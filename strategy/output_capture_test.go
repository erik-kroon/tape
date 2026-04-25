package strategy_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	tape "github.com/erik-kroon/tape/src"
	"github.com/erik-kroon/tape/strategy"
)

type capturedSignal struct {
	Index  int       `json:"index"`
	Time   time.Time `json:"time"`
	Symbol string    `json:"symbol"`
	Side   string    `json:"side"`
	Close  float64   `json:"close"`
}

func TestOutputCaptureMatchesJSONLFile(t *testing.T) {
	capture := strategy.NewOutputCapture[capturedSignal]()

	summary, err := strategy.RunFile(filepath.Join("..", "testdata", "bars_5_rows.csv"), tape.Config{
		Mode: tape.MaxSpeedMode,
	}, strategy.Hooks{
		OnEvent: func(ctx tape.Context, event tape.Event) error {
			bar, ok := event.(tape.Bar)
			if !ok || bar.Close < 93.10 {
				return nil
			}
			return capture.Record(capturedSignal{
				Index:  ctx.Index,
				Time:   bar.Time,
				Symbol: bar.Sym,
				Side:   "buy",
				Close:  bar.Close,
			})
		},
	})
	if err != nil {
		t.Fatalf("run file: %v", err)
	}
	if summary.Events != 5 {
		t.Fatalf("events = %d, want 5", summary.Events)
	}

	capture.AssertMatchesFile(t, filepath.Join("testdata", "breakout_signals.jsonl"))
}

func TestOutputCaptureMatchesGoldenFileWithCustomFormatting(t *testing.T) {
	capture := strategy.NewOutputCapture[capturedSignal](strategy.WithOutputMarshal(func(signal capturedSignal) ([]byte, error) {
		return []byte(fmt.Sprintf(
			"index=%d time=%s symbol=%s side=%s close=%.2f",
			signal.Index,
			signal.Time.Format(time.RFC3339),
			signal.Symbol,
			signal.Side,
			signal.Close,
		)), nil
	}))

	summary, err := strategy.RunFile(filepath.Join("..", "testdata", "bars_5_rows.csv"), tape.Config{
		Mode: tape.MaxSpeedMode,
	}, strategy.Hooks{
		OnEvent: func(ctx tape.Context, event tape.Event) error {
			bar, ok := event.(tape.Bar)
			if !ok || bar.Close < 93.10 {
				return nil
			}
			return capture.Record(capturedSignal{
				Index:  ctx.Index,
				Time:   bar.Time,
				Symbol: bar.Sym,
				Side:   "buy",
				Close:  bar.Close,
			})
		},
	})
	if err != nil {
		t.Fatalf("run file: %v", err)
	}
	if summary.Events != 5 {
		t.Fatalf("events = %d, want 5", summary.Events)
	}

	capture.AssertMatchesFile(t, filepath.Join("testdata", "breakout_signals.golden"))
}

func TestOutputCaptureWriteFileWritesRecordedOutputs(t *testing.T) {
	capture := strategy.NewOutputCapture[capturedSignal]()
	if err := capture.Record(capturedSignal{
		Index:  1,
		Time:   time.Date(2026, 4, 24, 9, 31, 0, 0, time.UTC),
		Symbol: "ERICB",
		Side:   "buy",
		Close:  93.00,
	}); err != nil {
		t.Fatalf("record: %v", err)
	}

	path := filepath.Join(t.TempDir(), "signals.jsonl")
	if err := capture.WriteFile(path); err != nil {
		t.Fatalf("write file: %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(contents) != capture.String() {
		t.Fatalf("file contents = %q, want %q", string(contents), capture.String())
	}
}

func TestOutputCaptureRejectsMultiLineEncodedOutput(t *testing.T) {
	capture := strategy.NewOutputCapture[string](strategy.WithOutputMarshal(func(value string) ([]byte, error) {
		return []byte(value), nil
	}))

	err := capture.Record("buy\nsell")
	if err == nil {
		t.Fatal("record error = nil, want multiline output error")
	}
	if err.Error() != "strategy: captured output must encode to a single line" {
		t.Fatalf("record error = %q, want multiline output error", err.Error())
	}
}
