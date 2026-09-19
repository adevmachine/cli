package packages

import (
	"os"
	"path/filepath"
	"strings"
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

// writePackage creates a package directory named dirName and writes body as
// its manifest. The directory name is explicit because one rule under test is
// that a package's name has to match the directory it lives in.
func writePackage(t *testing.T, dirName, body string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), dirName)
	write(t, ManifestPath(dir), body)
	return dir
}

// problemAbout returns the first problem whose message carries want.
func problemAbout(t *testing.T, problems []Problem, want string) Problem {
	t.Helper()
	for _, p := range problems {
		if strings.Contains(p.What, want) {
			return p
		}
	}
	t.Fatalf("no problem about %q in %#v", want, problems)
	return Problem{}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
