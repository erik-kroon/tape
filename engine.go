package tape

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"time"
)

type Context struct {
	Index     int
	StartedAt time.Time
}

type EventHandler func(Context, Event) error
type Middleware func(EventHandler) EventHandler

type Summary struct {
	Events         int
	StartedAt      time.Time
	FinishedAt     time.Time
	WallDuration   time.Duration
	FirstEventTime time.Time
	LastEventTime  time.Time
	EventTypes     map[string]int
	Symbols        map[string]int
}

type Engine struct {
	config     Config
	handlers   []EventHandler
	middleware []Middleware
}

func NewEngine(config Config) *Engine {
	return &Engine{config: config.normalized()}
}

func (e *Engine) Use(middleware Middleware) {
	e.middleware = append(e.middleware, middleware)
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
	stream, err := OpenStream(path)
	if err != nil {
		return Summary{}, err
	}
	defer stream.Close()

	return e.Run(stream)
}

func (e *Engine) Run(stream Stream) (Summary, error) {
	summary := Summary{
		StartedAt:  time.Now(),
		EventTypes: map[string]int{},
		Symbols:    map[string]int{},
	}

	handler := e.chainHandlers()
	clock := newReplayClock(e.config)
	var prev Event

	for {
		event, err := stream.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return summary, err
		}

		if err := validateOrdering(prev, event, e.config.Permissive); err != nil {
			return summary, err
		}

		if err := clock.Wait(summary.Events, event.Timestamp()); err != nil {
			return summary, err
		}

		if summary.Events == 0 {
			summary.FirstEventTime = event.Timestamp()
		}
		summary.LastEventTime = event.Timestamp()
		summary.EventTypes[event.Type()]++
		if symbol := event.Symbol(); symbol != "" {
			summary.Symbols[symbol]++
		}

		ctx := Context{
			Index:     summary.Events,
			StartedAt: summary.StartedAt,
		}
		if err := handler(ctx, event); err != nil {
			return summary, err
		}

		summary.Events++
		prev = event
	}

	summary.FinishedAt = time.Now()
	summary.WallDuration = summary.FinishedAt.Sub(summary.StartedAt)
	return summary, nil
}

func (e *Engine) chainHandlers() EventHandler {
	handler := func(ctx Context, event Event) error {
		for _, registered := range e.handlers {
			if err := registered(ctx, event); err != nil {
				return err
			}
		}
		return nil
	}

	for index := len(e.middleware) - 1; index >= 0; index-- {
		handler = e.middleware[index](handler)
	}

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
