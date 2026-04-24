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
