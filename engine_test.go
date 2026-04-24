package tape_test

import (
	"path/filepath"
	"testing"

	"github.com/erik-kroon/tape"
)

func TestReplayCSVBarSummary(t *testing.T) {
	engine := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode})

	summary, err := engine.RunFile(filepath.Join("testdata", "bars_5_rows.csv"))
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
}

func TestRecorderRoundTrip(t *testing.T) {
	recordingPath := filepath.Join(t.TempDir(), "session.tape")

	recorder, err := tape.NewRecorder(recordingPath)
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}

	engine := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode})
	engine.Use(recorder.Middleware())
	summary, err := engine.RunFile(filepath.Join("testdata", "ticks_5_rows.csv"))
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
	result, err := tape.CheckDeterminism(filepath.Join("testdata", "bars_5_rows.csv"), tape.Config{
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
