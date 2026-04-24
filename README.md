# Tape

Tape is a deterministic market-event replay engine for Go.

It helps developers replay, inspect, test, and benchmark trading pipelines using historical CSV data, Parquet datasets, synthetic event streams, or recorded JSONL sessions.

Tape does not execute trades and does not provide financial advice.

## What is included

- A small Go library with `Tick` and `Bar` event types
- A replay engine with max-speed, real-time, accelerated, and step modes
- Output sinks for replayed events
- CSV and Parquet readers for tick and OHLCV bar data
- JSONL session recording and replay through `.tape` files
- A CLI with `replay`, `inspect`, `bench`, and `check`
- Tests and fixture data for the core replay loop

## Install

```bash
go install github.com/erik-kroon/tape/cmd/tape@latest
```

Parquet support is powered by [`github.com/parquet-go/parquet-go`](https://github.com/parquet-go/parquet-go), so the module now tracks that dependency's current Go floor: `go 1.24.9`.

## Replay a CSV file

```bash
tape replay testdata/bars_5_rows.csv --speed 100x --metrics
```

## Replay a Parquet file

```bash
tape replay testdata/bars_5_rows.parquet --speed max --metrics
```

Runnable example:

```bash
go run ./examples/replay
```

## Replay a filtered subset

```bash
tape replay testdata/bars_5_rows.csv --symbol ERICB --event-type bar --from 2026-04-24T09:31:00Z --to 2026-04-24T09:33:00Z
```

## Seek to a starting position

```bash
tape replay testdata/ticks_5_rows.csv --start-at 2026-04-24T09:30:00.500Z
tape inspect testdata/ticks_5_rows.csv --start-at 4 --sample 2
```

## Step through a session

```bash
tape replay testdata/ticks_5_rows.csv --step
```

## Record to a session file

```bash
tape replay testdata/ticks_5_rows.csv --record sessions/opening-bell.tape
```

Runnable example:

```bash
go run ./examples/record_replay
```

## Inspect a recording

```bash
tape inspect sessions/opening-bell.tape --sample 5
```

Tape recordings automatically write a `.idx` sidecar on close so `--start-at` can jump directly into large sessions. Existing recordings can be indexed or reindexed with:

```bash
tape index sessions/opening-bell.tape
```

## Run a determinism check

```bash
tape check testdata/bars_5_rows.csv --runs 5
tape check testdata/bars_5_rows.parquet --runs 5
```

## Run a synthetic benchmark

```bash
tape bench --events 1000000 --symbols 100
```

Runnable example:

```bash
go run ./examples/benchmark
```

## Golden replay tests

CLI output regressions are pinned with golden files in `cmd/tape/testdata`.

Run the package tests normally:

```bash
go test ./cmd/tape
```

Refresh the golden files only when an output change is intentional:

```bash
UPDATE_GOLDEN=1 go test ./cmd/tape
```

## Use as a library

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

## Current scope

This repository is an MVP scaffold aligned to the PRD. It focuses on deterministic local replay primitives and a usable CLI, not a full exchange simulator or backtesting framework.

Replay summaries report event totals, wall-clock elapsed time, throughput, allocation volume, and error counts. The inspect command prints a short event sample to make recorded sessions easier to sanity-check from the terminal.

`replay`, `inspect`, and `check` all support the same inclusive filters: `--symbol` for one or more comma-separated symbols, `--event-type` for one or more comma-separated event types, and `--from` / `--to` for RFC3339 time bounds.

Those commands also support `--start-at` to seek before replay begins. `--start-at` accepts an RFC3339 timestamp, a `YYYY-MM-DD` date, or a sequence number, and starts from the first event at or after that position.

When a valid `.tape.idx` sidecar is present, Tape uses it automatically for `.tape` and `.jsonl` sessions. Missing or stale indexes fall back to the normal linear scan.

## Supported Parquet Schemas

Tape currently supports flat Parquet files for two event families:

- Tick rows: a timestamp column named `timestamp`, `time`, or `ts`; a symbol column named `symbol`, `ticker`, or `instrument`; a price column named `price` or `last`; optional size columns named `size`, `qty`, `quantity`, or `volume`; and an optional sequence column named `seq` or `sequence`.
- Bar rows: a timestamp column named `timestamp`, `time`, or `ts`; a symbol column named `symbol`, `ticker`, or `instrument`; required `open`, `high`, `low`, and `close` columns; an optional `volume` column; and an optional sequence column named `seq` or `sequence`.

The timestamp columns must be stored as Parquet `TIMESTAMP` logical types. The current adapter is intentionally narrow: it expects a single flat file with top-level columns and does not try to interpret nested schemas or partitioned dataset layouts.

## Parquet Performance Notes

Parquet input is read sequentially and converted back into Tape events row by row. This is a good fit for replaying larger historical files without first converting them to CSV, but it is not yet a query engine:

- Tape does not currently use Parquet predicate pushdown or column projection to skip work.
- Replay still validates event ordering and applies filters after row decode, just like CSV and `.tape` inputs.
- You should expect better storage efficiency than CSV and similar replay semantics, but not warehouse-style scan optimizations.

## Custom event codecs

Session recording, `.tape` replay, and determinism checks can be extended with custom event codecs:

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
