package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendWritesOneLinePerCommand(t *testing.T) {
	dir := t.TempDir()
	Append(dir, Entry{At: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC),
		Target: "workspace alice", Command: "docker ps", OK: true})
	Append(dir, Entry{At: time.Date(2026, 9, 18, 12, 1, 0, 0, time.UTC),
		Target: "machine main", Command: "sync", OK: false})

	body, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines:\n%s", len(lines), body)
	}
	for _, want := range []string{"2026-09-18T12:00:00Z", "workspace alice", "docker ps", "ok"} {
		if !strings.Contains(lines[0], want) {
			t.Fatalf("the first line leaves out %q: %q", want, lines[0])
		}
	}
	if !strings.Contains(lines[1], "failed") {
		t.Fatalf("a failure is not marked: %q", lines[1])
	}
}

func TestAppendNeverStopsTheCaller(t *testing.T) {
	// A log that can fail a command is a log somebody removes the first time
	// it costs them an afternoon.
	Append("/this/path/does/not/exist", Entry{At: time.Now(), Target: "machine main", Command: "sync"})
}

func TestAppendIsReadableByNobodyElse(t *testing.T) {
	dir := t.TempDir()
	Append(dir, Entry{At: time.Now(), Target: "machine main", Command: "sync", OK: true})

	info, err := os.Stat(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	// Commands carry hostnames, account names and sometimes an argument
	// somebody should not have typed.
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode is %o", info.Mode().Perm())
	}
}

func TestAppendKeepsACommandOnOneLine(t *testing.T) {
	dir := t.TempDir()
	Append(dir, Entry{At: time.Now(), Target: "workspace alice",
		Command: "printf 'a\nb'", OK: true})

	body, _ := os.ReadFile(filepath.Join(dir, FileName))
	if strings.Count(strings.TrimSpace(string(body)), "\n") != 0 {
		t.Fatalf("a command with a newline broke the format:\n%s", body)
	}
}

func TestAppendKeepsATargetOnOneLine(t *testing.T) {
	dir := t.TempDir()
	Append(dir, Entry{At: time.Now(), Target: "workspace a\nb", Command: "docker ps", OK: true})

	body, _ := os.ReadFile(filepath.Join(dir, FileName))
	if strings.Count(strings.TrimSpace(string(body)), "\n") != 0 {
		t.Fatalf("a target with a newline broke the format:\n%s", body)
	}
}

func TestAppendWritesTheTimeInUTC(t *testing.T) {
	dir := t.TempDir()
	somewhereElse := time.FixedZone("UTC+9", 9*60*60)
	Append(dir, Entry{At: time.Date(2026, 9, 18, 21, 0, 0, 0, somewhereElse),
		Target: "machine main", Command: "docker ps", OK: true})

	body, _ := os.ReadFile(filepath.Join(dir, FileName))
	if !strings.Contains(string(body), "2026-09-18T12:00:00Z") {
		t.Fatalf("the time was not written in UTC:\n%s", body)
	}
}
