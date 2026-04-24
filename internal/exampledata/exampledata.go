package exampledata

import (
	"path/filepath"
	"runtime"
)

// Path resolves a path from the repository root so examples can be run from any directory.
func Path(parts ...string) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join(parts...)
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	segments := append([]string{repoRoot}, parts...)
	return filepath.Join(segments...)
}
