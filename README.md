# Tape

Tape is a deterministic market-event replay engine for Go.

It helps developers replay, inspect, test, and benchmark trading pipelines using historical CSV data, synthetic event streams, or recorded JSONL sessions.

Tape does not execute trades and does not provide financial advice.

## What is included

- A small Go library with `Tick` and `Bar` event types
- A replay engine with max-speed, real-time, accelerated, and step modes
- CSV readers for tick and OHLCV bar data
- JSONL session recording and replay through `.tape` files
- A CLI with `replay`, `inspect`, `bench`, and `check`
- Tests and fixture data for the core replay loop

## Install

```bash
go install github.com/erik-kroon/tape/cmd/tape@latest
```

## Replay a CSV file

```bash
tape replay testdata/bars_5_rows.csv --speed 100x --metrics
```

Runnable example:

```bash
go run ./examples/replay
```

## Replay a filtered subset

```bash
tape replay testdata/bars_5_rows.csv --symbol ERICB --event-type bar --from 2026-04-24T09:31:00Z --to 2026-04-24T09:33:00Z
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

## Run a determinism check

```bash
tape check testdata/bars_5_rows.csv --runs 5
```

## Run a synthetic benchmark

```bash
tape bench --events 1000000 --symbols 100
```

Runnable example:

```bash
go run ./examples/benchmark
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
