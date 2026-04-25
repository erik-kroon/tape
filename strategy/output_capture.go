package strategy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erik-kroon/tape/internal/testutil/golden"
)

// OutputCapture records deterministic strategy outputs as single-line snapshots.
type OutputCapture[T any] struct {
	marshal func(T) ([]byte, error)
	lines   []string
}

// OutputCaptureOption configures how an OutputCapture serializes values.
type OutputCaptureOption[T any] func(*OutputCapture[T])

// NewOutputCapture returns a line-oriented capture for deterministic strategy outputs.
func NewOutputCapture[T any](options ...OutputCaptureOption[T]) *OutputCapture[T] {
	capture := &OutputCapture[T]{
		marshal: func(output T) ([]byte, error) {
			return json.Marshal(output)
		},
	}

	for _, option := range options {
		option(capture)
	}

	return capture
}

// WithOutputMarshal overrides the default JSON encoder for captured outputs.
func WithOutputMarshal[T any](marshal func(T) ([]byte, error)) OutputCaptureOption[T] {
	return func(capture *OutputCapture[T]) {
		capture.marshal = marshal
	}
}

// Record appends one encoded output line to the capture.
func (c *OutputCapture[T]) Record(output T) error {
	encoded, err := c.marshal(output)
	if err != nil {
		return err
	}
	if bytes.ContainsAny(encoded, "\r\n") {
		return fmt.Errorf("strategy: captured output must encode to a single line")
	}

	c.lines = append(c.lines, string(encoded))
	return nil
}

// String renders the capture as newline-delimited output with a trailing newline.
func (c *OutputCapture[T]) String() string {
	if len(c.lines) == 0 {
		return ""
	}
	return strings.Join(c.lines, "\n") + "\n"
}

// WriteFile persists the current capture to disk.
func (c *OutputCapture[T]) WriteFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(c.String()), 0o600)
}

// AssertMatchesFile compares the capture against a checked-in snapshot file.
func (c *OutputCapture[T]) AssertMatchesFile(t testing.TB, path string) {
	t.Helper()
	golden.Assert(t, path, c.String())
}
