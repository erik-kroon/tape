package tape_test

import (
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	tape "github.com/erik-kroon/tape/src"
)

func TestReplayCSVBarSummary(t *testing.T) {
	engine := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode})

	summary, err := engine.RunFile(filepath.Join("..", "testdata", "bars_5_rows.csv"))
	if err != nil {
		t.Fatalf("run file: %v", err)
	}

	if summary.Events != 5 {
		t.Fatalf("events = %d, want 5", summary.Events)
	}
	if summary.EventTypes["bar"] != 5 {
		t.Fatalf("bar count = %d, want 5", summary.EventTypes["bar"])
	}
	if summary.Symbols["ERICB"] != 5 {
		t.Fatalf("symbol count = %d, want 5", summary.Symbols["ERICB"])
	}
	if summary.ErrorCount != 0 {
		t.Fatalf("error count = %d, want 0", summary.ErrorCount)
	}
	if summary.Throughput <= 0 {
		t.Fatalf("throughput = %f, want > 0", summary.Throughput)
	}
	if summary.AllocBytes == 0 {
		t.Fatalf("alloc bytes = %d, want > 0", summary.AllocBytes)
	}
}

func TestRecorderRoundTrip(t *testing.T) {
	recordingPath := filepath.Join(t.TempDir(), "session.tape")

	recorder, err := tape.NewRecorder(recordingPath)
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}

	engine := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode})
	engine.Use(recorder.Middleware())
	summary, err := engine.RunFile(filepath.Join("..", "testdata", "ticks_5_rows.csv"))
	closeErr := recorder.Close()
	if err != nil {
		t.Fatalf("run file: %v", err)
	}
	if closeErr != nil {
		t.Fatalf("close recorder: %v", closeErr)
	}
	if summary.Events != 5 {
		t.Fatalf("events = %d, want 5", summary.Events)
	}

	replay := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode})
	replayed, err := replay.RunFile(recordingPath)
	if err != nil {
		t.Fatalf("replay recording: %v", err)
	}
	if replayed.Events != 5 {
		t.Fatalf("replayed events = %d, want 5", replayed.Events)
	}
	if replayed.EventTypes["tick"] != 5 {
		t.Fatalf("tick count = %d, want 5", replayed.EventTypes["tick"])
	}
}

func TestCheckDeterminism(t *testing.T) {
	result, err := tape.CheckDeterminism(filepath.Join("..", "testdata", "bars_5_rows.csv"), tape.Config{
		Mode: tape.MaxSpeedMode,
	}, 3)
	if err != nil {
		t.Fatalf("check determinism: %v", err)
	}

	if result.Runs != 3 {
		t.Fatalf("runs = %d, want 3", result.Runs)
	}
	if result.Events != 5 {
		t.Fatalf("events = %d, want 5", result.Events)
	}
	if len(result.Hash) != 64 {
		t.Fatalf("hash length = %d, want 64", len(result.Hash))
	}
}

func TestContextClockExposesReplayTime(t *testing.T) {
	timestamps := []time.Time{
		time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC),
		time.Date(2026, 4, 24, 9, 30, 1, 0, time.UTC),
	}
	engine := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode})

	var got []time.Time
	engine.OnEvent(func(ctx tape.Context, event tape.Event) error {
		got = append(got, ctx.Clock().Now())
		if replayTime := ctx.ReplayTime(); !replayTime.Equal(event.Timestamp()) {
			t.Fatalf("replay time = %s, want %s", replayTime, event.Timestamp())
		}
		return nil
	})

	_, err := engine.Run(newEventStream(
		tape.Tick{Time: timestamps[0], Sym: "ERICB", Price: 93.12, Seq: 1},
		tape.Tick{Time: timestamps[1], Sym: "ERICB", Price: 93.20, Seq: 2},
	))
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(got) != len(timestamps) {
		t.Fatalf("got %d replay timestamps, want %d", len(got), len(timestamps))
	}
	for index := range timestamps {
		if !got[index].Equal(timestamps[index]) {
			t.Fatalf("timestamp[%d] = %s, want %s", index, got[index], timestamps[index])
		}
	}
}

func TestSummaryTracksErrorsOnHandlerFailure(t *testing.T) {
	engine := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode})
	wantErr := errors.New("boom")
	callCount := 0
	engine.OnEvent(func(ctx tape.Context, event tape.Event) error {
		callCount++
		if callCount == 2 {
			return wantErr
		}
		return nil
	})

	summary, err := engine.Run(newEventStream(
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC), Sym: "ERICB", Price: 93.12, Seq: 1},
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 30, 1, 0, time.UTC), Sym: "ERICB", Price: 93.20, Seq: 2},
	))
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if summary.Events != 1 {
		t.Fatalf("events = %d, want 1", summary.Events)
	}
	if summary.ErrorCount != 1 {
		t.Fatalf("error count = %d, want 1", summary.ErrorCount)
	}
	if summary.FinishedAt.IsZero() {
		t.Fatal("finished at is zero, want non-zero")
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
