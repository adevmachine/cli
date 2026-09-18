package commands

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureInteractive records what would have been launched instead of
// launching it, so these tests never open a session.
func captureInteractive(t *testing.T) *[]string {
	t.Helper()
	var got []string
	runInteractive = func(name string, args ...string) error {
		got = append([]string{name}, args...)
		return nil
	}
	lookPath = func(string) (string, error) { return "/usr/bin/fake", nil }
	t.Cleanup(func() { runInteractive = realExecCommand; lookPath = realLookPath })
	return &got
}

func configWith(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSSHUsesTheConfiguredUserPortAndKey(t *testing.T) {
	got := captureInteractive(t)
	dir := configWith(t, "hosts:\n  - 203.0.113.10\nuser: alice\nport: 2222\nkey: /keys/id_ed25519\n")

	if _, err := execute(t, "--config", dir, "ssh"); err != nil {
		t.Fatalf("ssh returned %v", err)
	}

	line := strings.Join(*got, " ")
	for _, want := range []string{"ssh", "-p 2222", "-i /keys/id_ed25519", "alice@203.0.113.10"} {
		if !strings.Contains(line, want) {
			t.Fatalf("the command is missing %q: %q", want, line)
		}
	}
}

func TestSSHTakesAUserFromTheArgument(t *testing.T) {
	got := captureInteractive(t)
	dir := configWith(t, "hosts:\n  - 203.0.113.10\nuser: root\n")

	if _, err := execute(t, "--config", dir, "ssh", "bob"); err != nil {
		t.Fatalf("ssh returned %v", err)
	}

	line := strings.Join(*got, " ")
	if !strings.Contains(line, "bob@203.0.113.10") {
		t.Fatalf("the argument did not override the configured user: %q", line)
	}
}

func TestMoshPassesThePortThroughItsSSHOption(t *testing.T) {
	got := captureInteractive(t)
	dir := configWith(t, "hosts:\n  - 203.0.113.10\nport: 2222\nkey: /keys/id_ed25519\n")

	if _, err := execute(t, "--config", dir, "mosh"); err != nil {
		t.Fatalf("mosh returned %v", err)
	}

	line := strings.Join(*got, " ")
	// mosh does not take -p or -i itself; they have to travel inside --ssh.
	if !strings.Contains(line, "--ssh=ssh -p 2222 -i /keys/id_ed25519") {
		t.Fatalf("the port and key did not reach mosh's ssh command: %q", line)
	}
	if !strings.Contains(line, "root@203.0.113.10") {
		t.Fatalf("the target is missing: %q", line)
	}
}

func TestAnInteractiveCommandSaysWhenTheBinaryIsMissing(t *testing.T) {
	runInteractive = func(string, ...string) error { return nil }
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { runInteractive = realExecCommand; lookPath = realLookPath })

	dir := configWith(t, "hosts:\n  - 203.0.113.10\n")

	_, err := execute(t, "--config", dir, "mosh")
	if err == nil {
		t.Fatal("expected an error when mosh is not installed")
	}
	if !strings.Contains(err.Error(), "mosh") {
		t.Fatalf("the error does not name the missing binary: %v", err)
	}
}

func TestAnInteractiveCommandRefusesAnInvalidConfig(t *testing.T) {
	got := captureInteractive(t)
	dir := configWith(t, "domain: example.com\n")

	if _, err := execute(t, "--config", dir, "ssh"); err == nil {
		t.Fatal("expected an error when there is no host")
	}
	if len(*got) != 0 {
		t.Fatalf("it launched a session with an invalid configuration: %v", *got)
	}
}

func TestHumanBytesReadsLikeAPersonWouldWriteIt(t *testing.T) {
	cases := map[uint64]string{
		0:          "0B",
		512:        "512B",
		1024:       "1.0K",
		1536:       "1.5K",
		1073741824: "1.0G",
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Fatalf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
