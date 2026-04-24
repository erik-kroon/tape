package golden

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const updateEnvVar = "UPDATE_GOLDEN"

// Assert compares actual output against a golden file and can refresh it when requested.
func Assert(t *testing.T, path string, actual string) {
	t.Helper()

	actual = normalize(actual)
	if os.Getenv(updateEnvVar) == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create golden dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(actual), 0o600); err != nil {
			t.Fatalf("write golden file: %v", err)
		}
		return
	}

	expectedBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file %s: %v", path, err)
	}
	expected := normalize(string(expectedBytes))
	if expected == actual {
		return
	}

	t.Fatalf(
		"golden output mismatch for %s\n%s\nRefresh with `%s=1 go test ./cmd/tape`.",
		path,
		diff(expected, actual),
		updateEnvVar,
	)
}

func normalize(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}

func diff(expected string, actual string) string {
	expectedLines := strings.Split(expected, "\n")
	actualLines := strings.Split(actual, "\n")

	maxLines := max(len(expectedLines), len(actualLines))
	var b strings.Builder
	shown := 0

	for index := 0; index < maxLines; index++ {
		var want string
		if index < len(expectedLines) {
			want = expectedLines[index]
		}

		var got string
		if index < len(actualLines) {
			got = actualLines[index]
		}

		if want == got {
			continue
		}

		fmt.Fprintf(&b, "line %d\n", index+1)
		fmt.Fprintf(&b, "- want: %q\n", want)
		fmt.Fprintf(&b, "+ got:  %q\n", got)
		shown++
		if shown == 8 {
			remaining := maxLines - index - 1
			if remaining > 0 {
				fmt.Fprintf(&b, "... %d more differing line(s)\n", remaining)
			}
			break
		}
	}

	if shown == 0 {
		return "contents differ, but no differing lines were found"
	}

	return strings.TrimRight(b.String(), "\n")
}
