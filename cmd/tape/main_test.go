package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erik-kroon/tape/internal/testutil/golden"
)

func TestRunInspectShowsSampleEvents(t *testing.T) {
	output := captureStdout(t, func() {
		err := run([]string{
			"inspect",
			filepath.Join("..", "..", "testdata", "ticks_5_rows.csv"),
			"--sample", "2",
		})
		if err != nil {
			t.Fatalf("run inspect: %v", err)
		}
	})

	golden.Assert(t, filepath.Join("testdata", "inspect_sample_ticks_5_rows.golden"), output)
}

func TestRunReplayPrintsEvents(t *testing.T) {
	output := captureStdout(t, func() {
		err := run([]string{
			"replay",
			"--print",
			"--metrics=false",
			filepath.Join("..", "..", "testdata", "ticks_5_rows.csv"),
		})
		if err != nil {
			t.Fatalf("run replay: %v", err)
		}
	})

	golden.Assert(t, filepath.Join("testdata", "replay_print_ticks_5_rows.golden"), output)
}

func TestRunReplayPrintsParquetEvents(t *testing.T) {
	output := captureStdout(t, func() {
		err := run([]string{
			"replay",
			"--print",
			"--metrics=false",
			filepath.Join("..", "..", "testdata", "ticks_5_rows.parquet"),
		})
		if err != nil {
			t.Fatalf("run replay: %v", err)
		}
	})

	for _, fragment := range []string{
		"seq=1 time=2026-04-24T09:30:00Z symbol=ERICB price=92.5000 size=100.0000",
		"seq=5 time=2026-04-24T09:30:01Z symbol=ERICB price=92.5800 size=90.0000",
	} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("output missing %q\n%s", fragment, output)
		}
	}
}

func TestRunReplayRecordsEvents(t *testing.T) {
	recordPath := filepath.Join(t.TempDir(), "session.tape")

	err := run([]string{
		"replay",
		"--metrics=false",
		"--record", recordPath,
		filepath.Join("..", "..", "testdata", "ticks_5_rows.csv"),
	})
	if err != nil {
		t.Fatalf("run replay: %v", err)
	}

	contents, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read recording: %v", err)
	}
	if count := strings.Count(string(contents), "\n"); count != 5 {
		t.Fatalf("record lines = %d, want 5", count)
	}
	if !strings.Contains(string(contents), `"type":"tick"`) {
		t.Fatalf("recording missing tick payloads\n%s", string(contents))
	}
}

func TestRunReplayMergesMultipleSourcesInOrder(t *testing.T) {
	quotesPath := filepath.Join(t.TempDir(), "quotes.tape")
	tradesPath := filepath.Join(t.TempDir(), "trades.tape")
	writeSessionFile(t, quotesPath, []string{
		`{"type":"tick","payload":{"time":"2026-04-24T09:30:00Z","symbol":"ERICB","price":92.50,"size":100,"seq":1},"index":0}`,
		`{"type":"tick","payload":{"time":"2026-04-24T09:30:01Z","symbol":"ERICB","price":92.53,"size":80,"seq":3},"index":1}`,
	})
	writeSessionFile(t, tradesPath, []string{
		`{"type":"tick","payload":{"time":"2026-04-24T09:30:00Z","symbol":"ERICB","price":92.51,"size":10,"seq":1},"index":0}`,
		`{"type":"tick","payload":{"time":"2026-04-24T09:30:00.5Z","symbol":"ERICB","price":92.52,"size":12,"seq":2},"index":1}`,
	})

	output := captureStdout(t, func() {
		err := run([]string{
			"replay",
			"--print",
			"--metrics=false",
			quotesPath,
			tradesPath,
		})
		if err != nil {
			t.Fatalf("run replay: %v", err)
		}
	})

	want := []string{
		"seq=1 time=2026-04-24T09:30:00Z symbol=ERICB price=92.5000 size=100.0000",
		"seq=2 time=2026-04-24T09:30:00Z symbol=ERICB price=92.5100 size=10.0000",
		"seq=3 time=2026-04-24T09:30:00.5Z symbol=ERICB price=92.5200 size=12.0000",
		"seq=4 time=2026-04-24T09:30:01Z symbol=ERICB price=92.5300 size=80.0000",
	}
	for _, fragment := range want {
		if !strings.Contains(output, fragment) {
			t.Fatalf("output missing %q\n%s", fragment, output)
		}
	}
}

func TestRunIndexBuildsSidecar(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.tape")
	records := strings.Join([]string{
		`{"type":"tick","payload":{"time":"2026-04-24T09:30:00Z","symbol":"ERICB","price":93.12,"size":10,"seq":1},"index":0}`,
		`{"type":"tick","payload":{"time":"2026-04-24T09:30:01Z","symbol":"ERICB","price":93.20,"size":8,"seq":2},"index":1}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(records), 0o600); err != nil {
		t.Fatalf("write recording: %v", err)
	}

	output := captureStdout(t, func() {
		err := run([]string{"index", path})
		if err != nil {
			t.Fatalf("run index: %v", err)
		}
	})

	if !strings.Contains(output, "Index written: "+path+".idx") {
		t.Fatalf("output missing index path\n%s", output)
	}
	if _, err := os.Stat(path + ".idx"); err != nil {
		t.Fatalf("stat index: %v", err)
	}
}

func TestRunCheckAcceptsParquet(t *testing.T) {
	output := captureStdout(t, func() {
		err := run([]string{
			"check",
			"--runs", "3",
			filepath.Join("..", "..", "testdata", "bars_5_rows.parquet"),
		})
		if err != nil {
			t.Fatalf("run check: %v", err)
		}
	})

	for _, fragment := range []string{
		"Determinism check passed.",
		"Runs:             3",
		"Events processed: 5",
		"Output hash:",
	} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("output missing %q\n%s", fragment, output)
		}
	}
}

func TestRunInspectAppliesFiltersToSummaryAndSample(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mixed.tape")
	records := strings.Join([]string{
		`{"type":"tick","payload":{"time":"2026-04-24T09:30:00Z","symbol":"ERICB","price":93.12,"size":10,"seq":1},"index":0}`,
		`{"type":"bar","payload":{"time":"2026-04-24T09:31:00Z","symbol":"ERICB","open":93.00,"high":93.50,"low":92.90,"close":93.40,"volume":100,"seq":2},"index":1}`,
		`{"type":"tick","payload":{"time":"2026-04-24T09:32:00Z","symbol":"VOLV","price":301.00,"size":5,"seq":3},"index":2}`,
		`{"type":"tick","payload":{"time":"2026-04-24T09:33:00Z","symbol":"ERICB","price":93.55,"size":8,"seq":4},"index":3}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(records), 0o600); err != nil {
		t.Fatalf("write recording: %v", err)
	}

	output := captureStdout(t, func() {
		err := run([]string{
			"inspect",
			path,
			"--symbol", "ericb",
			"--event-type", "tick",
			"--from", "2026-04-24T09:32:30Z",
			"--to", "2026-04-24T09:33:00Z",
			"--sample", "2",
		})
		if err != nil {
			t.Fatalf("run inspect: %v", err)
		}
	})

	for _, fragment := range []string{
		"Events:      1",
		"Types:       tick=1",
		"Symbols:     ERICB=1",
		"Sample:",
		"symbol=ERICB",
		"price=",
	} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("output missing %q\n%s", fragment, output)
		}
	}
	for _, fragment := range []string{"VOLV", "open="} {
		if strings.Contains(output, fragment) {
			t.Fatalf("output unexpectedly contains %q\n%s", fragment, output)
		}
	}
}

func TestRunInspectRejectsInvalidTimeRange(t *testing.T) {
	err := run([]string{
		"inspect",
		filepath.Join("..", "..", "testdata", "ticks_5_rows.csv"),
		"--from", "2026-04-24T10:00:00Z",
		"--to", "2026-04-24T09:00:00Z",
	})
	if err == nil {
		t.Fatal("run inspect error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "filter start time") {
		t.Fatalf("error = %v, want filter time-range error", err)
	}
}

func TestRunInspectStartAtTimestampSkipsEarlierEvents(t *testing.T) {
	output := captureStdout(t, func() {
		err := run([]string{
			"inspect",
			filepath.Join("..", "..", "testdata", "ticks_5_rows.csv"),
			"--start-at", "2026-04-24T09:30:00.500Z",
			"--sample", "2",
		})
		if err != nil {
			t.Fatalf("run inspect: %v", err)
		}
	})

	for _, fragment := range []string{
		"Events:      3",
		"First event: 2026-04-24T09:30:00.5Z",
		"seq=1 time=2026-04-24T09:30:00.5Z symbol=ERICB",
		"seq=2 time=2026-04-24T09:30:00.75Z symbol=ERICB",
	} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("output missing %q\n%s", fragment, output)
		}
	}
	if strings.Contains(output, "09:30:00.25Z") || strings.Contains(output, "price=92.5000") {
		t.Fatalf("output unexpectedly contains events before start-at\n%s", output)
	}
}

func TestRunInspectStartAtSequenceSkipsEarlierEvents(t *testing.T) {
	output := captureStdout(t, func() {
		err := run([]string{
			"inspect",
			filepath.Join("..", "..", "testdata", "ticks_5_rows.csv"),
			"--start-at", "4",
			"--sample", "2",
		})
		if err != nil {
			t.Fatalf("run inspect: %v", err)
		}
	})

	for _, fragment := range []string{
		"Events:      2",
		"seq=1 time=2026-04-24T09:30:00.75Z symbol=ERICB",
		"seq=2 time=2026-04-24T09:30:01Z symbol=ERICB",
	} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("output missing %q\n%s", fragment, output)
		}
	}
	if strings.Contains(output, "09:30:00.5Z") {
		t.Fatalf("output unexpectedly contains sequence before start-at\n%s", output)
	}
}

func TestRunInspectRejectsInvalidStartAt(t *testing.T) {
	err := run([]string{
		"inspect",
		filepath.Join("..", "..", "testdata", "ticks_5_rows.csv"),
		"--start-at", "not-a-position",
	})
	if err == nil {
		t.Fatal("run inspect error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "start-at") {
		t.Fatalf("error = %v, want start-at error", err)
	}
}

func TestRunnableExamples(t *testing.T) {
	for _, tc := range []struct {
		name      string
		path      string
		fragments []string
	}{
		{
			name: "replay",
			path: "./examples/replay",
			fragments: []string{
				"09:30:00 ERICB close=92.70",
				"Replayed 5 bar events across 1 symbol(s)",
			},
		},
		{
			name: "record_replay",
			path: "./examples/record_replay",
			fragments: []string{
				"Recorded 5 tick events into opening-bell.tape",
				"replay[0] ERICB price=92.50 size=100",
				"Replayed 5 events from the recording",
			},
		},
		{
			name: "benchmark",
			path: "./examples/benchmark",
			fragments: []string{
				"Benchmark events: 50000",
				"Benchmark symbols: 8",
				"Throughput:",
			},
		},
		{
			name: "strategy",
			path: "./examples/strategy",
			fragments: []string{
				"Starting strategy on",
				"bar[0] ERICB close=92.70",
				"Strategy completed with 5 events",
				"Harness summary events=5 symbols=1",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := runGoProgram(t, tc.path)
			for _, fragment := range tc.fragments {
				if !strings.Contains(output, fragment) {
					t.Fatalf("output missing %q\n%s", fragment, output)
				}
			}
		})
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer reader.Close()

	os.Stdout = writer
	defer func() {
		os.Stdout = oldStdout
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	return string(output)
}

func runGoProgram(t *testing.T, path string) string {
	t.Helper()

	command := exec.Command("go", "run", path)
	command.Dir = filepath.Join("..", "..")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go run %s: %v\n%s", path, err, output)
	}
	return string(output)
}

func writeSessionFile(t *testing.T, path string, records []string) {
	t.Helper()

	data := strings.Join(records, "\n") + "\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write session file: %v", err)
	}
}
