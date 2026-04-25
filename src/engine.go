package tape

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"runtime"
	"time"
)

type Clock interface {
	Now() time.Time
}

type Context struct {
	Index         int
	Measured      bool
	MeasuredIndex int
	StartedAt     time.Time
	SourcePaths   []string
	replayTime    time.Time
}

type EventHandler func(Context, Event) error
type Middleware func(EventHandler) EventHandler

func (c Context) Clock() Clock {
	return contextClock{now: c.replayTime}
}

func (c Context) ReplayTime() time.Time {
	return c.replayTime
}

type contextClock struct {
	now time.Time
}

func (c contextClock) Now() time.Time {
	return c.now
}

type Summary struct {
	Events          int
	ErrorCount      int
	StartedAt       time.Time
	FinishedAt      time.Time
	WallDuration    time.Duration
	HandlerDuration time.Duration
	Throughput      float64
	AllocBytes      uint64
	FirstEventTime  time.Time
	LastEventTime   time.Time
	EventTypes      map[string]int
	Symbols         map[string]int
}

type Engine struct {
	config     Config
	handlers   []EventHandler
	middleware []Middleware
	sinks      []OutputSink
}

func NewEngine(config Config) *Engine {
	return &Engine{config: config.normalized()}
}

func (e *Engine) Use(middleware Middleware) {
	e.middleware = append(e.middleware, middleware)
}

func (e *Engine) AddSink(sink OutputSink) {
	e.sinks = append(e.sinks, sink)
}

func (e *Engine) OnEvent(handler EventHandler) {
	e.handlers = append(e.handlers, handler)
}

func (e *Engine) OnTick(handler func(Context, Tick) error) {
	e.OnEvent(func(ctx Context, event Event) error {
		tick, ok := event.(Tick)
		if !ok {
			return nil
		}
		return handler(ctx, tick)
	})
}

func (e *Engine) OnBar(handler func(Context, Bar) error) {
	e.OnEvent(func(ctx Context, event Event) error {
		bar, ok := event.(Bar)
		if !ok {
			return nil
		}
		return handler(ctx, bar)
	})
}

func (e *Engine) RunFile(path string) (Summary, error) {
	return e.RunFiles(path)
}

func (e *Engine) RunFiles(paths ...string) (Summary, error) {
	if err := e.config.validate(); err != nil {
		return Summary{}, err
	}

	selection, err := OpenReplaySelectionPaths(paths, e.config)
	if err != nil {
		return Summary{}, err
	}
	defer selection.Close()

	return e.runSelection(selection, paths)
}

func (e *Engine) Run(stream Stream) (summary Summary, err error) {
	if err := e.config.validate(); err != nil {
		return Summary{}, err
	}

	selection, err := NewReplaySelection(stream, e.config)
	if err != nil {
		return Summary{}, err
	}

	return e.runSelection(selection, nil)
}

func (e *Engine) runSelection(selection *ReplaySelection, sourcePaths []string) (summary Summary, err error) {
	if err := e.config.validate(); err != nil {
		return Summary{}, err
	}

	summary = Summary{
		StartedAt:  time.Now(),
		EventTypes: map[string]int{},
		Symbols:    map[string]int{},
	}
	var startMem runtime.MemStats
	runtime.ReadMemStats(&startMem)
	defer finalizeSummary(&summary, startMem)

	timer := NewHandlerTimer()
	defer func() {
		summary.HandlerDuration = timer.Total()
	}()

	copiedSourcePaths := append([]string(nil), sourcePaths...)
	handler := e.chainHandlers(timer)
	clock := newReplayClock(e.config)

	for {
		selected, nextErr := selection.Next()
		err = nextErr
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
				break
			}
			summary.ErrorCount++
			return summary, err
		}

		event := selected.Event

		if err = clock.Wait(selected.Index, event.Timestamp()); err != nil {
			summary.ErrorCount++
			return summary, err
		}

		if selected.Index == 0 {
			summary.FirstEventTime = event.Timestamp()
		}
		summary.LastEventTime = event.Timestamp()
		summary.EventTypes[event.Type()]++
		if symbol := event.Symbol(); symbol != "" {
			summary.Symbols[symbol]++
		}

		ctx := Context{
			Index:         selected.Index,
			Measured:      true,
			MeasuredIndex: selected.Index,
			StartedAt:     summary.StartedAt,
			SourcePaths:   copiedSourcePaths,
			replayTime:    event.Timestamp(),
		}
		if err = handler(ctx, event); err != nil {
			summary.ErrorCount++
			return summary, err
		}

		summary.Events = selected.Index + 1
	}

	return summary, nil
}

func finalizeSummary(summary *Summary, startMem runtime.MemStats) {
	summary.FinishedAt = time.Now()
	summary.WallDuration = summary.FinishedAt.Sub(summary.StartedAt)

	var endMem runtime.MemStats
	runtime.ReadMemStats(&endMem)
	if endMem.TotalAlloc >= startMem.TotalAlloc {
		summary.AllocBytes = endMem.TotalAlloc - startMem.TotalAlloc
	}

	if summary.WallDuration > 0 {
		summary.Throughput = float64(summary.Events) / summary.WallDuration.Seconds()
	}
}

func (e *Engine) chainHandlers(timer *HandlerTimer) EventHandler {
	handler := func(ctx Context, event Event) error {
		for _, sink := range e.sinks {
			if err := sink.Write(ctx, event); err != nil {
				return err
			}
		}
		for _, registered := range e.handlers {
			if err := registered(ctx, event); err != nil {
				return err
			}
		}
		return nil
	}

	handler = timer.Middleware()(handler)
	for index := len(e.middleware) - 1; index >= 0; index-- {
		handler = e.middleware[index](handler)
	}
	handler = RecoverPanics()(handler)

	return handler
}

func validateOrdering(prev Event, current Event, permissive bool) error {
	if prev == nil {
		return nil
	}

	if current.Timestamp().Before(prev.Timestamp()) && !permissive {
		return fmt.Errorf("ordering error: %s before %s", current.Timestamp().Format(time.RFC3339Nano), prev.Timestamp().Format(time.RFC3339Nano))
	}

	if current.Timestamp().Equal(prev.Timestamp()) && current.Sequence() < prev.Sequence() && !permissive {
		return fmt.Errorf("ordering error: sequence %d before %d", current.Sequence(), prev.Sequence())
	}

	return nil
}

type replayClock struct {
	config        Config
	lastEventTime time.Time
	stepReader    *bufio.Reader
}

func newReplayClock(config Config) *replayClock {
	return &replayClock{
		config:     config,
		stepReader: bufio.NewReader(config.StepReader),
	}
}

func (c *replayClock) Wait(index int, current time.Time) error {
	if index == 0 {
		c.lastEventTime = current
		return nil
	}

	switch c.config.Mode {
	case MaxSpeedMode:
		c.lastEventTime = current
		return nil
	case StepMode:
		if _, err := fmt.Fprint(c.config.StepWriter, "Press Enter for next event...\n"); err != nil {
			return err
		}
		if _, err := c.stepReader.ReadString('\n'); err != nil {
			return err
		}
		c.lastEventTime = current
		return nil
	case RealTimeMode, AcceleratedMode:
		delay := current.Sub(c.lastEventTime)
		if delay < 0 {
			delay = 0
		}
		if c.config.Mode == AcceleratedMode && c.config.Speed > 0 {
			delay = time.Duration(float64(delay) / c.config.Speed)
		}
		time.Sleep(delay)
		c.lastEventTime = current
		return nil
	default:
		return fmt.Errorf("clock error: unknown mode %q", c.config.Mode)
	}
}
