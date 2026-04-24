package tape_test

import (
	"encoding/json"
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

func TestReplayParquetTickSummary(t *testing.T) {
	engine := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode})

	summary, err := engine.RunFile(filepath.Join("..", "testdata", "ticks_5_rows.parquet"))
	if err != nil {
		t.Fatalf("run file: %v", err)
	}

	if summary.Events != 5 {
		t.Fatalf("events = %d, want 5", summary.Events)
	}
	if summary.EventTypes["tick"] != 5 {
		t.Fatalf("tick count = %d, want 5", summary.EventTypes["tick"])
	}
	if summary.Symbols["ERICB"] != 5 {
		t.Fatalf("symbol count = %d, want 5", summary.Symbols["ERICB"])
	}
}

func TestReplayParquetBarSummary(t *testing.T) {
	engine := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode})

	summary, err := engine.RunFile(filepath.Join("..", "testdata", "bars_5_rows.parquet"))
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
	engine.AddSink(recorder)
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

func TestRecorderCloseWritesSessionIndex(t *testing.T) {
	recordingPath := filepath.Join(t.TempDir(), "session.tape")

	recorder, err := tape.NewRecorder(recordingPath)
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}

	if err := recorder.Record(0, tape.Tick{
		Time:  time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC),
		Sym:   "ERICB",
		Price: 93.12,
		Seq:   1,
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := recorder.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}

	if _, err := os.Stat(recordingPath + ".idx"); err != nil {
		t.Fatalf("stat index: %v", err)
	}
}

func TestRunFileSupportsStartAtWithoutSessionIndex(t *testing.T) {
	recordingPath := filepath.Join(t.TempDir(), "session.tape")

	recorder, err := tape.NewRecorder(recordingPath)
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}

	engine := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode})
	engine.AddSink(recorder)
	if _, err := engine.RunFile(filepath.Join("..", "testdata", "ticks_5_rows.csv")); err != nil {
		t.Fatalf("run file: %v", err)
	}
	if err := recorder.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}
	if err := os.Remove(recordingPath + ".idx"); err != nil {
		t.Fatalf("remove index: %v", err)
	}

	replay := tape.NewEngine(tape.Config{
		Mode: tape.MaxSpeedMode,
		StartAt: tape.StartAt{
			Sequence: 4,
		},
	})
	summary, err := replay.RunFile(recordingPath)
	if err != nil {
		t.Fatalf("run file: %v", err)
	}
	if summary.Events != 2 {
		t.Fatalf("events = %d, want 2", summary.Events)
	}
}

func TestRunFileSupportsStartAtWithValidSessionIndex(t *testing.T) {
	recordingPath := filepath.Join(t.TempDir(), "session.tape")

	recorder, err := tape.NewRecorder(recordingPath)
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}

	engine := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode})
	engine.AddSink(recorder)
	if _, err := engine.RunFile(filepath.Join("..", "testdata", "ticks_5_rows.csv")); err != nil {
		t.Fatalf("run file: %v", err)
	}
	if err := recorder.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}

	replay := tape.NewEngine(tape.Config{
		Mode: tape.MaxSpeedMode,
		StartAt: tape.StartAt{
			Sequence: 4,
		},
	})
	summary, err := replay.RunFile(recordingPath)
	if err != nil {
		t.Fatalf("run file: %v", err)
	}
	if summary.Events != 2 {
		t.Fatalf("events = %d, want 2", summary.Events)
	}
}

func TestRunFileFallsBackWhenSessionIndexIsStale(t *testing.T) {
	recordingPath := filepath.Join(t.TempDir(), "session.tape")

	recorder, err := tape.NewRecorder(recordingPath)
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}

	engine := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode})
	engine.AddSink(recorder)
	if _, err := engine.RunFile(filepath.Join("..", "testdata", "ticks_5_rows.csv")); err != nil {
		t.Fatalf("run file: %v", err)
	}
	if err := recorder.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}

	contents, err := os.ReadFile(recordingPath)
	if err != nil {
		t.Fatalf("read recording: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(contents)), "\n")
	reduced := strings.Join(lines[:3], "\n") + "\n"
	if err := os.WriteFile(recordingPath, []byte(reduced), 0o600); err != nil {
		t.Fatalf("rewrite recording: %v", err)
	}

	replay := tape.NewEngine(tape.Config{
		Mode: tape.MaxSpeedMode,
		StartAt: tape.StartAt{
			Sequence: 4,
		},
	})
	summary, err := replay.RunFile(recordingPath)
	if err != nil {
		t.Fatalf("run file: %v", err)
	}
	if summary.Events != 0 {
		t.Fatalf("events = %d, want 0", summary.Events)
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

func TestCheckDeterminismParquet(t *testing.T) {
	result, err := tape.CheckDeterminism(filepath.Join("..", "testdata", "bars_5_rows.parquet"), tape.Config{
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

func TestRecorderRoundTripWithCustomCodec(t *testing.T) {
	recordingPath := filepath.Join(t.TempDir(), "custom-session.tape")
	codec := headlineEventCodec()

	recorder, err := tape.NewRecorderWithCodecs(recordingPath, codec)
	if err != nil {
		t.Fatalf("new recorder with codecs: %v", err)
	}

	source := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode})
	source.AddSink(recorder)
	summary, err := source.Run(newEventStream(
		headlineEvent{
			Time:     time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC),
			Sym:      "ERICB",
			Headline: "Opening auction imbalance cleared",
			Seq:      1,
		},
		headlineEvent{
			Time:     time.Date(2026, 4, 24, 9, 30, 1, 0, time.UTC),
			Sym:      "ERICB",
			Headline: "Guidance reaffirmed",
			Seq:      2,
		},
	))
	if err != nil {
		t.Fatalf("run custom stream: %v", err)
	}
	closeErr := recorder.Close()
	if closeErr != nil {
		t.Fatalf("close recorder: %v", closeErr)
	}
	if summary.Events != 2 {
		t.Fatalf("events = %d, want 2", summary.Events)
	}

	var replayed []headlineEvent
	replay := tape.NewEngine(tape.Config{
		Mode:        tape.MaxSpeedMode,
		EventCodecs: []tape.EventCodec{codec},
	})
	replay.OnEvent(func(ctx tape.Context, event tape.Event) error {
		headline, ok := event.(headlineEvent)
		if !ok {
			t.Fatalf("event type = %T, want headlineEvent", event)
		}
		replayed = append(replayed, headline)
		return nil
	})

	replayedSummary, err := replay.RunFile(recordingPath)
	if err != nil {
		t.Fatalf("replay custom recording: %v", err)
	}
	if replayedSummary.Events != 2 {
		t.Fatalf("replayed events = %d, want 2", replayedSummary.Events)
	}
	if replayedSummary.EventTypes["headline"] != 2 {
		t.Fatalf("headline count = %d, want 2", replayedSummary.EventTypes["headline"])
	}
	if len(replayed) != 2 {
		t.Fatalf("replayed len = %d, want 2", len(replayed))
	}
	if replayed[0].Headline != "Opening auction imbalance cleared" {
		t.Fatalf("headline[0] = %q, want %q", replayed[0].Headline, "Opening auction imbalance cleared")
	}
	if replayed[1].Headline != "Guidance reaffirmed" {
		t.Fatalf("headline[1] = %q, want %q", replayed[1].Headline, "Guidance reaffirmed")
	}
}

func TestCheckDeterminismWithCustomCodec(t *testing.T) {
	recordingPath := filepath.Join(t.TempDir(), "custom-session.tape")
	codec := headlineEventCodec()

	recorder, err := tape.NewRecorderWithCodecs(recordingPath, codec)
	if err != nil {
		t.Fatalf("new recorder with codecs: %v", err)
	}

	engine := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode})
	engine.Use(recorder.Middleware())
	_, err = engine.Run(newEventStream(
		headlineEvent{
			Time:     time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC),
			Sym:      "ERICB",
			Headline: "Opening auction imbalance cleared",
			Seq:      1,
		},
		headlineEvent{
			Time:     time.Date(2026, 4, 24, 9, 30, 1, 0, time.UTC),
			Sym:      "ERICB",
			Headline: "Guidance reaffirmed",
			Seq:      2,
		},
	))
	closeErr := recorder.Close()
	if err != nil {
		t.Fatalf("run custom stream: %v", err)
	}
	if closeErr != nil {
		t.Fatalf("close recorder: %v", closeErr)
	}

	result, err := tape.CheckDeterminism(recordingPath, tape.Config{
		Mode:        tape.MaxSpeedMode,
		EventCodecs: []tape.EventCodec{codec},
	}, 3)
	if err != nil {
		t.Fatalf("check determinism with codecs: %v", err)
	}
	if result.Runs != 3 {
		t.Fatalf("runs = %d, want 3", result.Runs)
	}
	if result.Events != 2 {
		t.Fatalf("events = %d, want 2", result.Events)
	}
	if len(result.Hash) != 64 {
		t.Fatalf("hash length = %d, want 64", len(result.Hash))
	}
}

func TestRunFileSupportsLegacySessionRecords(t *testing.T) {
	recordingPath := filepath.Join(t.TempDir(), "legacy-session.tape")
	record := `{"type":"tick","tick":{"time":"2026-04-24T09:30:00Z","symbol":"ERICB","price":93.12,"size":10,"seq":1},"index":0}` + "\n"
	if err := os.WriteFile(recordingPath, []byte(record), 0o600); err != nil {
		t.Fatalf("write legacy record: %v", err)
	}

	engine := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode})
	summary, err := engine.RunFile(recordingPath)
	if err != nil {
		t.Fatalf("run legacy recording: %v", err)
	}
	if summary.Events != 1 {
		t.Fatalf("events = %d, want 1", summary.Events)
	}
	if summary.EventTypes["tick"] != 1 {
		t.Fatalf("tick count = %d, want 1", summary.EventTypes["tick"])
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

func TestOutputSinksReceiveReplayedEventsBeforeHandlers(t *testing.T) {
	engine := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode})

	var calls []string
	engine.AddSink(tape.OutputSinkFunc(func(ctx tape.Context, event tape.Event) error {
		calls = append(calls, fmt.Sprintf("sink:%d:%s", ctx.Index, event.Symbol()))
		return nil
	}))
	engine.OnEvent(func(ctx tape.Context, event tape.Event) error {
		calls = append(calls, fmt.Sprintf("handler:%d:%s", ctx.Index, event.Symbol()))
		return nil
	})

	summary, err := engine.Run(newEventStream(
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC), Sym: "ERICB", Price: 93.12, Seq: 1},
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 30, 1, 0, time.UTC), Sym: "VOLV", Price: 301.00, Seq: 2},
	))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if summary.Events != 2 {
		t.Fatalf("events = %d, want 2", summary.Events)
	}

	want := []string{
		"sink:0:ERICB",
		"handler:0:ERICB",
		"sink:1:VOLV",
		"handler:1:VOLV",
	}
	if !slices.Equal(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
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

func TestSummaryRecoversFromHandlerPanic(t *testing.T) {
	engine := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode})
	engine.OnEvent(func(ctx tape.Context, event tape.Event) error {
		panic("kaboom")
	})

	summary, err := engine.Run(newEventStream(
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC), Sym: "ERICB", Price: 93.12, Seq: 1},
	))
	if !errors.Is(err, tape.ErrHandlerPanic) {
		t.Fatalf("err = %v, want %v", err, tape.ErrHandlerPanic)
	}
	if !strings.Contains(err.Error(), "kaboom") {
		t.Fatalf("err = %q, want panic value included", err.Error())
	}
	if summary.Events != 0 {
		t.Fatalf("events = %d, want 0", summary.Events)
	}
	if summary.ErrorCount != 1 {
		t.Fatalf("error count = %d, want 1", summary.ErrorCount)
	}
}

func TestSummaryTracksHandlerDuration(t *testing.T) {
	engine := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode})
	engine.OnEvent(func(ctx tape.Context, event tape.Event) error {
		time.Sleep(2 * time.Millisecond)
		return nil
	})

	summary, err := engine.Run(newEventStream(
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC), Sym: "ERICB", Price: 93.12, Seq: 1},
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 30, 1, 0, time.UTC), Sym: "ERICB", Price: 93.20, Seq: 2},
	))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if summary.HandlerDuration < 4*time.Millisecond {
		t.Fatalf("handler duration = %s, want at least 4ms", summary.HandlerDuration)
	}
	if summary.HandlerDuration > summary.WallDuration {
		t.Fatalf("handler duration = %s, want <= wall duration %s", summary.HandlerDuration, summary.WallDuration)
	}
}

func TestEngineAppliesSymbolTypeAndTimeFilters(t *testing.T) {
	end := time.Date(2026, 4, 24, 9, 32, 0, 0, time.UTC)
	engine := tape.NewEngine(tape.Config{
		Mode: tape.MaxSpeedMode,
		Filter: tape.Filter{
			Symbols:    []string{"ericb"},
			EventTypes: []string{"tick"},
			StartTime:  time.Date(2026, 4, 24, 9, 31, 0, 0, time.UTC),
			EndTime:    end,
		},
	})

	var got []tape.Event
	var indices []int
	engine.OnEvent(func(ctx tape.Context, event tape.Event) error {
		got = append(got, event)
		indices = append(indices, ctx.Index)
		return nil
	})

	summary, err := engine.Run(newEventStream(
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC), Sym: "ERICB", Price: 93.12, Seq: 1},
		tape.Bar{Time: time.Date(2026, 4, 24, 9, 31, 0, 0, time.UTC), Sym: "ERICB", Open: 93.10, High: 93.30, Low: 93.00, Close: 93.20, Seq: 2},
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 31, 30, 0, time.UTC), Sym: "VOLV", Price: 301.00, Seq: 3},
		tape.Tick{Time: end, Sym: "ERICB", Price: 93.25, Seq: 4},
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 33, 0, 0, time.UTC), Sym: "ERICB", Price: 93.30, Seq: 5},
	))
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if summary.Events != 1 {
		t.Fatalf("events = %d, want 1", summary.Events)
	}
	if summary.EventTypes["tick"] != 1 {
		t.Fatalf("tick count = %d, want 1", summary.EventTypes["tick"])
	}
	if summary.Symbols["ERICB"] != 1 {
		t.Fatalf("ERICB count = %d, want 1", summary.Symbols["ERICB"])
	}
	if !summary.FirstEventTime.Equal(end) {
		t.Fatalf("first event = %s, want %s", summary.FirstEventTime, end)
	}
	if !summary.LastEventTime.Equal(end) {
		t.Fatalf("last event = %s, want %s", summary.LastEventTime, end)
	}
	if len(got) != 1 {
		t.Fatalf("handler events = %d, want 1", len(got))
	}
	if tick, ok := got[0].(tape.Tick); !ok || tick.Sym != "ERICB" || !tick.Time.Equal(end) {
		t.Fatalf("handler event = %#v, want ERICB tick at %s", got[0], end)
	}
	if len(indices) != 1 || indices[0] != 0 {
		t.Fatalf("indices = %v, want [0]", indices)
	}
}

func TestEngineRejectsInvalidFilterTimeRange(t *testing.T) {
	engine := tape.NewEngine(tape.Config{
		Mode: tape.MaxSpeedMode,
		Filter: tape.Filter{
			StartTime: time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 4, 24, 9, 0, 0, 0, time.UTC),
		},
	})

	_, err := engine.Run(newEventStream(
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC), Sym: "ERICB", Price: 93.12, Seq: 1},
	))
	if err == nil {
		t.Fatal("run error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "filter start time") {
		t.Fatalf("error = %v, want filter time-range error", err)
	}
}

func TestEngineStartsAtTimestampBeforeApplyingFilters(t *testing.T) {
	startAt := time.Date(2026, 4, 24, 9, 31, 0, 0, time.UTC)
	engine := tape.NewEngine(tape.Config{
		Mode: tape.MaxSpeedMode,
		StartAt: tape.StartAt{
			Time: startAt,
		},
		Filter: tape.Filter{
			EventTypes: []string{"tick"},
		},
	})

	var got []tape.Event
	var indices []int
	engine.OnEvent(func(ctx tape.Context, event tape.Event) error {
		got = append(got, event)
		indices = append(indices, ctx.Index)
		return nil
	})

	summary, err := engine.Run(newEventStream(
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC), Sym: "ERICB", Price: 93.12, Seq: 1},
		tape.Bar{Time: startAt, Sym: "ERICB", Open: 93.10, High: 93.30, Low: 93.00, Close: 93.20, Seq: 2},
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 31, 30, 0, time.UTC), Sym: "ERICB", Price: 93.25, Seq: 3},
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 32, 0, 0, time.UTC), Sym: "VOLV", Price: 301.00, Seq: 4},
	))
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if summary.Events != 2 {
		t.Fatalf("events = %d, want 2", summary.Events)
	}
	if len(got) != 2 {
		t.Fatalf("handler events = %d, want 2", len(got))
	}
	if len(indices) != 2 || indices[0] != 0 || indices[1] != 1 {
		t.Fatalf("indices = %v, want [0 1]", indices)
	}

	first, ok := got[0].(tape.Tick)
	if !ok {
		t.Fatalf("event[0] type = %T, want tape.Tick", got[0])
	}
	if first.Seq != 3 {
		t.Fatalf("event[0] seq = %d, want 3", first.Seq)
	}
	if !summary.FirstEventTime.Equal(first.Time) {
		t.Fatalf("first event time = %s, want %s", summary.FirstEventTime, first.Time)
	}
}

func TestEngineStartsAtSequenceBeforeApplyingFilters(t *testing.T) {
	engine := tape.NewEngine(tape.Config{
		Mode: tape.MaxSpeedMode,
		StartAt: tape.StartAt{
			Sequence: 3,
		},
		Filter: tape.Filter{
			Symbols: []string{"ericb"},
		},
	})

	var got []tape.Event
	engine.OnEvent(func(ctx tape.Context, event tape.Event) error {
		got = append(got, event)
		return nil
	})

	summary, err := engine.Run(newEventStream(
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC), Sym: "ERICB", Price: 93.12, Seq: 1},
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 30, 30, 0, time.UTC), Sym: "VOLV", Price: 301.00, Seq: 2},
		tape.Bar{Time: time.Date(2026, 4, 24, 9, 31, 0, 0, time.UTC), Sym: "ERICB", Open: 93.10, High: 93.30, Low: 93.00, Close: 93.20, Seq: 3},
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 31, 30, 0, time.UTC), Sym: "ERICB", Price: 93.25, Seq: 4},
	))
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if summary.Events != 2 {
		t.Fatalf("events = %d, want 2", summary.Events)
	}
	if len(got) != 2 {
		t.Fatalf("handler events = %d, want 2", len(got))
	}
	first, ok := got[0].(tape.Bar)
	if !ok {
		t.Fatalf("event[0] type = %T, want tape.Bar", got[0])
	}
	if first.Seq != 3 {
		t.Fatalf("event[0] seq = %d, want 3", first.Seq)
	}
}

func TestEngineRejectsInvalidStartAt(t *testing.T) {
	engine := tape.NewEngine(tape.Config{
		Mode: tape.MaxSpeedMode,
		StartAt: tape.StartAt{
			Time:     time.Date(2026, 4, 24, 9, 31, 0, 0, time.UTC),
			Sequence: 3,
		},
	})

	_, err := engine.Run(newEventStream(
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC), Sym: "ERICB", Price: 93.12, Seq: 1},
	))
	if err == nil {
		t.Fatal("run error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "start-at") {
		t.Fatalf("error = %v, want start-at validation error", err)
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

type headlineEvent struct {
	Time     time.Time `json:"time"`
	Sym      string    `json:"symbol"`
	Headline string    `json:"headline"`
	Seq      int64     `json:"seq,omitempty"`
}

func (h headlineEvent) Type() string         { return "headline" }
func (h headlineEvent) Symbol() string       { return h.Sym }
func (h headlineEvent) Timestamp() time.Time { return h.Time }
func (h headlineEvent) Sequence() int64      { return h.Seq }

func headlineEventCodec() tape.EventCodec {
	return tape.EventCodec{
		Type: "headline",
		Encode: func(event tape.Event) (json.RawMessage, error) {
			headline, ok := event.(headlineEvent)
			if !ok {
				return nil, fmt.Errorf("headline codec expected headlineEvent, got %T", event)
			}
			return json.Marshal(headline)
		},
		Decode: func(payload json.RawMessage) (tape.Event, error) {
			var headline headlineEvent
			if err := json.Unmarshal(payload, &headline); err != nil {
				return nil, err
			}
			return headline, nil
		},
	}
}
