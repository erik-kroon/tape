# Tape

Tape is a deterministic market-event replay engine for Go.

It is built for replaying historical ticks and bars through trading or analytics code with repeatable timing and consistent ordering. Tape can read CSV files, flat Parquet files, synthetic event streams, and recorded JSONL sessions stored as `.tape` files.

Tape does not execute trades and does not provide financial advice.

## What Tape Includes

- A Go library with `Tick`, `Bar`, and shared event interfaces
- A replay engine with max-speed, real-time, accelerated, and step modes
- CLI commands for replaying, sampling, benchmarking, validating, and indexing data
- CSV and Parquet readers for tick and OHLCV bar data
- Session recording and replay through `.tape` files
- Determinism checks and test fixtures for the core replay loop

## Install

```bash
go install github.com/erik-kroon/tape/cmd/tape@latest
```

Tape currently targets `go 1.24.9`, matching the floor required by `github.com/parquet-go/parquet-go`.

## Quick Start

Replay a CSV file:

```bash
tape replay testdata/bars_5_rows.csv --speed 100x --metrics
```

Replay a Parquet file:

```bash
tape replay testdata/bars_5_rows.parquet --speed max --metrics
```

Replay only a filtered slice:

```bash
tape replay testdata/bars_5_rows.csv \
  --symbol ERICB \
  --event-type bar \
  --from 2026-04-24T09:31:00Z \
  --to 2026-04-24T09:33:00Z
```

Replay a merged stream from multiple files:

```bash
tape replay sessions/quotes.tape sessions/trades.tape --speed max --metrics
```

Step through events manually:

```bash
tape replay testdata/ticks_5_rows.csv --step
```

Seek to a starting position:

```bash
tape replay testdata/ticks_5_rows.csv --start-at 2026-04-24T09:30:00.500Z
tape inspect testdata/ticks_5_rows.csv --start-at 4 --sample 2
```

## CLI Commands

### `replay`

Replays events from a file through the engine.

```bash
tape replay <path> [<path>...] [--speed max|realtime|100x] [--step] [--symbol SYM1,SYM2] [--event-type tick,bar] [--from RFC3339] [--to RFC3339] [--start-at RFC3339|YYYY-MM-DD|SEQ]
```

Use `--record` to capture the replay into a `.tape` session:

```bash
tape replay testdata/ticks_5_rows.csv --record sessions/opening-bell.tape
```

Runnable example:

```bash
go run ./examples/replay
go run ./examples/record_replay
```

### `inspect`

Prints a small sample of events from a source without replaying the full stream.

```bash
tape inspect <path> [<path>...] [--sample N] [--symbol SYM1,SYM2] [--event-type tick,bar] [--from RFC3339] [--to RFC3339] [--start-at RFC3339|YYYY-MM-DD|SEQ]
```

Example:

```bash
tape inspect sessions/opening-bell.tape --sample 5
```

### `check`

Runs the same input multiple times and verifies deterministic output.

```bash
tape check <path> [<path>...] [--runs N] [--symbol SYM1,SYM2] [--event-type tick,bar] [--from RFC3339] [--to RFC3339] [--start-at RFC3339|YYYY-MM-DD|SEQ]
```

Examples:

```bash
tape check testdata/bars_5_rows.csv --runs 5
tape check testdata/bars_5_rows.parquet --runs 5
```

### `bench`

Generates synthetic events and reports replay performance.

```bash
tape bench [--events N] [--symbols N]
```

Example:

```bash
tape bench --events 1000000 --symbols 100
go run ./examples/benchmark
```

### `index`

Builds or rebuilds a session index for `.tape` files.

```bash
tape index <path>
```

Example:

```bash
tape index sessions/opening-bell.tape
```

## Filters and Start Positions

`replay`, `inspect`, and `check` all support one or more input paths plus the same inclusive filters:

- `--symbol` for one or more comma-separated symbols
- `--event-type` for one or more comma-separated event types
- `--from` and `--to` for RFC3339 time bounds

Those commands also support `--start-at` to seek before processing begins. `--start-at` accepts:

- An RFC3339 timestamp
- A `YYYY-MM-DD` date
- A non-negative sequence number

Processing starts from the first event at or after that position.

## Session Files

Tape can record event streams into `.tape` files and replay them later with the same engine and filters used for CSV or Parquet input.

When a recording closes, Tape writes a `.idx` sidecar automatically. That index allows direct seeks into large `.tape` and `.jsonl` sessions when `--start-at` is used. If the index is missing or stale, Tape falls back to a linear scan.

## Use as a Library

```go
package main

import (
	"fmt"
	"log"

	tape "github.com/erik-kroon/tape/src"
)

func main() {
	engine := tape.NewEngine(tape.Config{
		Mode:  tape.AcceleratedMode,
		Speed: 100,
	})

	engine.AddSink(tape.OutputSinkFunc(func(ctx tape.Context, event tape.Event) error {
		fmt.Println("sink", ctx.Index, event.Symbol())
		return nil
	}))

	engine.OnBar(func(ctx tape.Context, bar tape.Bar) error {
		fmt.Println(ctx.Clock().Now(), bar.Symbol(), bar.Close)
		return nil
	})

	if _, err := engine.RunFile("testdata/bars_5_rows.csv"); err != nil {
		log.Fatal(err)
	}
}
```

Use `RunFiles` to merge multiple ordered sources into one replay:

```go
if _, err := engine.RunFiles("sessions/quotes.tape", "sessions/trades.tape"); err != nil {
	log.Fatal(err)
}
```

Replay summaries include event totals, elapsed wall time, throughput, allocation volume, and error counts.

## Strategy Harness

For project-level strategy code, use the `strategy` package to bind user hooks to Tape without wiring `Engine` callbacks manually each time.

```go
package main

import (
	"fmt"
	"log"

	"github.com/erik-kroon/tape/strategy"
	tape "github.com/erik-kroon/tape/src"
)

func main() {
	summary, err := strategy.RunFile("testdata/bars_5_rows.csv", tape.Config{
		Mode: tape.MaxSpeedMode,
	}, strategy.Hooks{
		OnStart: func(ctx strategy.RunContext) error {
			fmt.Println("starting", ctx.Path)
			return nil
		},
		OnEvent: func(ctx tape.Context, event tape.Event) error {
			bar, ok := event.(tape.Bar)
			if !ok {
				return nil
			}
			fmt.Println(ctx.Index, bar.Symbol(), bar.Close)
			return nil
		},
		OnEnd: func(result strategy.RunResult) error {
			fmt.Println("finished", result.Summary.Events, "events")
			return nil
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("events:", summary.Events)
}
```

Use `strategy.NewRunner` when you want to attach Tape middleware or sinks once and reuse the same harness across runs.

For replay-and-verify tests, capture your strategy outputs and assert them against checked-in snapshots:

```go
type Signal struct {
	Index  int       `json:"index"`
	Time   time.Time `json:"time"`
	Symbol string    `json:"symbol"`
	Side   string    `json:"side"`
}

func TestSignals(t *testing.T) {
	capture := strategy.NewOutputCapture[Signal]()
	warmupStart := time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC)
	measuredStart := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	measuredEnd := time.Date(2026, 4, 24, 11, 0, 0, 0, time.UTC)

	_, err := strategy.RunFile("testdata/bars_5_rows.csv", tape.Config{
		Mode: tape.MaxSpeedMode,
	}, strategy.Hooks{
		OnEvent: func(ctx tape.Context, event tape.Event) error {
			bar, ok := event.(tape.Bar)
			if !ok || bar.Close < 93.10 {
				return nil
			}
			return capture.RecordMeasured(ctx, Signal{
				Index:  ctx.MeasuredIndex,
				Time:   bar.Time,
				Symbol: bar.Symbol(),
				Side:   "buy",
			})
		},
	}, strategy.WithReplayRanges(strategy.ReplayRanges{
		Warmup: strategy.ReplayRange{
			Start: warmupStart,
			End:   measuredStart,
		},
		Measured: strategy.ReplayRange{
			Start: measuredStart,
			End:   measuredEnd,
		},
	}))
	if err != nil {
		t.Fatalf("run file: %v", err)
	}

	capture.AssertMatchesFile(t, "testdata/signals.jsonl")
}
```

`strategy.WithReplayRanges` expands the replay filter to cover both windows, while `ctx.Measured` and `ctx.MeasuredIndex` let your handler distinguish assertion events from warmup events. By default `OutputCapture` writes one JSON object per line, and `AssertMatchesFile` works with `.jsonl` or `.golden` snapshots. Use `RecordMeasured` when snapshots should ignore warmup output, `WithOutputMarshal` when you want a custom single-line format for human-readable goldens, or `WriteFile` when you want to persist a fresh capture outside the test assertion path.

## Supported Parquet Schemas

Tape currently supports flat Parquet files for two event families:

- Tick rows: a timestamp column named `timestamp`, `time`, or `ts`; a symbol column named `symbol`, `ticker`, or `instrument`; a price column named `price` or `last`; optional size columns named `size`, `qty`, `quantity`, or `volume`; and an optional sequence column named `seq` or `sequence`
- Bar rows: a timestamp column named `timestamp`, `time`, or `ts`; a symbol column named `symbol`, `ticker`, or `instrument`; required `open`, `high`, `low`, and `close` columns; an optional `volume` column; and an optional sequence column named `seq` or `sequence`

Timestamp columns must use the Parquet `TIMESTAMP` logical type.

The current adapter expects a single flat file with top-level columns. It does not interpret nested schemas or partitioned dataset layouts.

## Parquet Notes

Parquet input is read sequentially and converted into Tape events row by row. That works well for replaying larger historical files without converting them to CSV first, but it is still a replay pipeline rather than a query engine.

- Tape does not currently use predicate pushdown
- Tape does not currently use column projection
- Replay still validates ordering and applies filters after decoding

## Custom Event Codecs

Session recording, `.tape` replay, and determinism checks can be extended with custom codecs:

```go
codec := tape.EventCodec{
	Type: "headline",
	Encode: func(event tape.Event) (json.RawMessage, error) {
		headline, ok := event.(Headline)
		if !ok {
			return nil, fmt.Errorf("headline codec expected Headline, got %T", event)
		}
		return json.Marshal(headline)
	},
	Decode: func(payload json.RawMessage) (tape.Event, error) {
		var headline Headline
		if err := json.Unmarshal(payload, &headline); err != nil {
			return nil, err
		}
		return headline, nil
	},
}

recorder, err := tape.NewRecorderWithCodecs("session.tape", codec)
if err != nil {
	log.Fatal(err)
}

engine := tape.NewEngine(tape.Config{
	Mode:        tape.MaxSpeedMode,
	EventCodecs: []tape.EventCodec{codec},
})
```

## Development

CLI output regressions are covered with golden files in `cmd/tape/testdata`, and strategy-level output regressions can be captured with `strategy.OutputCapture`.

Run tests:

```bash
go test ./...
```

Refresh golden files only when an output change is intentional:

```bash
UPDATE_GOLDEN=1 go test ./...
```
