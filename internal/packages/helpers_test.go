package packages

import (
	"os"
	"path/filepath"
	"testing"
)

// write creates a file and every directory above it.
func write(t *testing.T, path, body string) {
	t.Helper()
	writeMode(t, path, body, 0o644)
}

func writeMode(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}
