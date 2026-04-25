package strategy_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tape "github.com/erik-kroon/tape/src"
	"github.com/erik-kroon/tape/strategy"
)

func TestRunFileInvokesHooksInOrder(t *testing.T) {
	var calls []string
	summary, err := strategy.RunFile(filepath.Join("..", "testdata", "bars_5_rows.csv"), tape.Config{
		Mode: tape.MaxSpeedMode,
	}, strategy.Hooks{
		OnStart: func(ctx strategy.RunContext) error {
			calls = append(calls, "start:"+filepath.Base(ctx.Path))
			return nil
		},
		OnEvent: func(ctx tape.Context, event tape.Event) error {
			calls = append(calls, fmt.Sprintf("event:%d:%s", ctx.Index, event.Symbol()))
			return nil
		},
		OnEnd: func(result strategy.RunResult) error {
			calls = append(calls, fmt.Sprintf("end:%d:%v", result.Summary.Events, result.Err == nil))
			return nil
		},
	})
	if err != nil {
		t.Fatalf("run file: %v", err)
	}

	want := []string{
		"start:bars_5_rows.csv",
		"event:0:ERICB",
		"event:1:ERICB",
		"event:2:ERICB",
		"event:3:ERICB",
		"event:4:ERICB",
		"end:5:true",
	}
	if !slices.Equal(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	if summary.Events != 5 {
		t.Fatalf("events = %d, want 5", summary.Events)
	}
}

func TestRunFilesExposeMergedPaths(t *testing.T) {
	quotesPath := filepath.Join(t.TempDir(), "quotes.tape")
	tradesPath := filepath.Join(t.TempDir(), "trades.tape")
	writeSessionFile(t, quotesPath, []string{
		`{"type":"tick","payload":{"time":"2026-04-24T09:30:00Z","symbol":"ERICB","price":92.50,"size":100,"seq":1},"index":0}`,
	})
	writeSessionFile(t, tradesPath, []string{
		`{"type":"tick","payload":{"time":"2026-04-24T09:30:00.5Z","symbol":"ERICB","price":92.52,"size":12,"seq":2},"index":0}`,
	})

	var started strategy.RunContext
	var ended strategy.RunResult
	summary, err := strategy.RunFiles([]string{quotesPath, tradesPath}, tape.Config{
		Mode: tape.MaxSpeedMode,
	}, strategy.Hooks{
		OnStart: func(ctx strategy.RunContext) error {
			started = ctx
			return nil
		},
		OnEvent: func(ctx tape.Context, event tape.Event) error {
			return nil
		},
		OnEnd: func(result strategy.RunResult) error {
			ended = result
			return nil
		},
	})
	if err != nil {
		t.Fatalf("run files: %v", err)
	}
	if summary.Events != 2 {
		t.Fatalf("events = %d, want 2", summary.Events)
	}
	if started.Path != "" {
		t.Fatalf("start path = %q, want empty for multi-source run", started.Path)
	}
	if got := strings.Join(started.Paths, ","); got != quotesPath+","+tradesPath {
		t.Fatalf("start paths = %q, want %q", got, quotesPath+","+tradesPath)
	}
	if got := strings.Join(ended.Paths, ","); got != quotesPath+","+tradesPath {
		t.Fatalf("end paths = %q, want %q", got, quotesPath+","+tradesPath)
	}
}

func TestRunCallsOnEndWhenStartFails(t *testing.T) {
	wantErr := errors.New("start failed")
	onEndCalls := 0

	summary, err := strategy.Run(newEventStream(
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC), Sym: "ERICB", Price: 93.12, Seq: 1},
	), tape.Config{Mode: tape.MaxSpeedMode}, strategy.Hooks{
		OnStart: func(ctx strategy.RunContext) error {
			return wantErr
		},
		OnEvent: func(ctx tape.Context, event tape.Event) error {
			t.Fatal("OnEvent called, want no calls after start failure")
			return nil
		},
		OnEnd: func(result strategy.RunResult) error {
			onEndCalls++
			if !errors.Is(result.Err, wantErr) {
				t.Fatalf("result err = %v, want %v", result.Err, wantErr)
			}
			if result.Summary.Events != 0 {
				t.Fatalf("summary events = %d, want 0", result.Summary.Events)
			}
			return nil
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if summary.Events != 0 {
		t.Fatalf("summary events = %d, want 0", summary.Events)
	}
	if onEndCalls != 1 {
		t.Fatalf("OnEnd calls = %d, want 1", onEndCalls)
	}
}

func TestRunCallsOnEndWhenRunFails(t *testing.T) {
	wantErr := errors.New("handler failed")
	onEndCalls := 0

	summary, err := strategy.Run(newEventStream(
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC), Sym: "ERICB", Price: 93.12, Seq: 1},
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 30, 1, 0, time.UTC), Sym: "ERICB", Price: 93.20, Seq: 2},
	), tape.Config{Mode: tape.MaxSpeedMode}, strategy.Hooks{
		OnEvent: func(ctx tape.Context, event tape.Event) error {
			if ctx.Index == 1 {
				return wantErr
			}
			return nil
		},
		OnEnd: func(result strategy.RunResult) error {
			onEndCalls++
			if !errors.Is(result.Err, wantErr) {
				t.Fatalf("result err = %v, want %v", result.Err, wantErr)
			}
			if result.Summary.Events != 1 {
				t.Fatalf("summary events = %d, want 1", result.Summary.Events)
			}
			return nil
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if summary.Events != 1 {
		t.Fatalf("summary events = %d, want 1", summary.Events)
	}
	if onEndCalls != 1 {
		t.Fatalf("OnEnd calls = %d, want 1", onEndCalls)
	}
}

func TestRunnerOptionsApplyMiddlewareAndSinks(t *testing.T) {
	var calls []string
	runner := strategy.NewRunner(tape.Config{Mode: tape.MaxSpeedMode},
		strategy.WithMiddleware(func(next tape.EventHandler) tape.EventHandler {
			return func(ctx tape.Context, event tape.Event) error {
				calls = append(calls, "middleware:before")
				err := next(ctx, event)
				calls = append(calls, "middleware:after")
				return err
			}
		}),
		strategy.WithSinks(tape.OutputSinkFunc(func(ctx tape.Context, event tape.Event) error {
			calls = append(calls, "sink")
			return nil
		})),
	)

	summary, err := runner.Run(newEventStream(
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC), Sym: "ERICB", Price: 93.12, Seq: 1},
	), strategy.Hooks{
		OnStart: func(ctx strategy.RunContext) error {
			calls = append(calls, "start")
			return nil
		},
		OnEvent: func(ctx tape.Context, event tape.Event) error {
			calls = append(calls, "handler")
			return nil
		},
		OnEnd: func(result strategy.RunResult) error {
			calls = append(calls, "end")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if summary.Events != 1 {
		t.Fatalf("summary events = %d, want 1", summary.Events)
	}

	want := []string{"start", "middleware:before", "sink", "handler", "middleware:after", "end"}
	if !slices.Equal(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestRunRejectsMissingOnEventHook(t *testing.T) {
	onEndCalls := 0

	_, err := strategy.Run(newEventStream(), tape.Config{Mode: tape.MaxSpeedMode}, strategy.Hooks{
		OnEnd: func(result strategy.RunResult) error {
			onEndCalls++
			if result.Err == nil {
				t.Fatal("result err = nil, want missing hook error")
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("err = nil, want missing hook error")
	}
	if err.Error() != "strategy: OnEvent hook is required" {
		t.Fatalf("err = %q, want missing hook error", err.Error())
	}
	if onEndCalls != 1 {
		t.Fatalf("OnEnd calls = %d, want 1", onEndCalls)
	}
}

type eventStream struct {
	events []tape.Event
	index  int
}

func newEventStream(events ...tape.Event) *eventStream {
	return &eventStream{events: events}
}

func (s *eventStream) Next() (tape.Event, error) {
	if s.index >= len(s.events) {
		return nil, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}

func (s *eventStream) Close() error {
	return nil
}

func writeSessionFile(t *testing.T, path string, records []string) {
	t.Helper()

	data := strings.Join(records, "\n") + "\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write session file: %v", err)
	}
}
