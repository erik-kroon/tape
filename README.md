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

## Step through a session

```bash
tape replay testdata/ticks_5_rows.csv --step
```

## Record to a session file

```bash
tape replay testdata/ticks_5_rows.csv --record sessions/opening-bell.tape
```

## Inspect a recording

```bash
tape inspect sessions/opening-bell.tape
```

## Run a determinism check

```bash
tape check testdata/bars_5_rows.csv --runs 5
```

## Run a synthetic benchmark

```bash
tape bench --events 1000000 --symbols 100
```

## Use as a library

```go
package main

import (
	"fmt"
	"log"

	"github.com/erik-kroon/tape"
)

func main() {
	engine := tape.NewEngine(tape.Config{
		Mode:  tape.AcceleratedMode,
		Speed: 100,
	})

	engine.OnBar(func(ctx tape.Context, bar tape.Bar) error {
		fmt.Println(bar.Symbol(), bar.Close)
		return nil
	})

	if _, err := engine.RunFile("testdata/bars_5_rows.csv"); err != nil {
		log.Fatal(err)
	}
}
```

## Current scope

This repository is an MVP scaffold aligned to the PRD. It focuses on deterministic local replay primitives and a usable CLI, not a full exchange simulator or backtesting framework.
