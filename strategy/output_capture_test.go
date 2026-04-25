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

func TestOutputCaptureRecordMeasuredSkipsWarmupOutputs(t *testing.T) {
	capture := strategy.NewOutputCapture[capturedSignal]()
	timestamps := []time.Time{
		time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC),
		time.Date(2026, 4, 24, 9, 31, 0, 0, time.UTC),
		time.Date(2026, 4, 24, 9, 32, 0, 0, time.UTC),
		time.Date(2026, 4, 24, 9, 33, 0, 0, time.UTC),
	}

	_, err := strategy.Run(newEventStream(
		tape.Bar{Time: timestamps[0], Sym: "ERICB", Open: 93.00, High: 93.10, Low: 92.90, Close: 93.05, Seq: 1},
		tape.Bar{Time: timestamps[1], Sym: "ERICB", Open: 93.05, High: 93.20, Low: 93.00, Close: 93.15, Seq: 2},
		tape.Bar{Time: timestamps[2], Sym: "ERICB", Open: 93.15, High: 93.30, Low: 93.10, Close: 93.25, Seq: 3},
		tape.Bar{Time: timestamps[3], Sym: "ERICB", Open: 93.25, High: 93.40, Low: 93.20, Close: 93.35, Seq: 4},
	), tape.Config{Mode: tape.MaxSpeedMode}, strategy.Hooks{
		OnEvent: func(ctx tape.Context, event tape.Event) error {
			bar := event.(tape.Bar)
			return capture.RecordMeasured(ctx, capturedSignal{
				Index:  ctx.MeasuredIndex,
				Time:   bar.Time,
				Symbol: bar.Sym,
				Side:   "buy",
				Close:  bar.Close,
			})
		},
	}, strategy.WithReplayRanges(strategy.ReplayRanges{
		Warmup: strategy.ReplayRange{
			Start: timestamps[0],
			End:   timestamps[1],
		},
		Measured: strategy.ReplayRange{
			Start: timestamps[2],
			End:   timestamps[3],
		},
	}))
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	want := "" +
		"{\"index\":0,\"time\":\"2026-04-24T09:32:00Z\",\"symbol\":\"ERICB\",\"side\":\"buy\",\"close\":93.25}\n" +
		"{\"index\":1,\"time\":\"2026-04-24T09:33:00Z\",\"symbol\":\"ERICB\",\"side\":\"buy\",\"close\":93.35}\n"
	if capture.String() != want {
		t.Fatalf("capture = %q, want %q", capture.String(), want)
	}
}
