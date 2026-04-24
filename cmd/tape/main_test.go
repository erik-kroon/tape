package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	for _, fragment := range []string{
		"Events:      5",
		"Sample:",
		"seq=1",
		"seq=2",
		"price=",
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
