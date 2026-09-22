package commands

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/aliases"
)

var errNotFound = errors.New("not found")

func stubTools(t *testing.T, present map[string]bool) {
	t.Helper()
	orig := lookPath
	lookPath = func(name string) (string, error) {
		if present[name] {
			return "/usr/bin/" + name, nil
		}
		return "", errNotFound
	}
	t.Cleanup(func() { lookPath = orig })
}

func TestMachineDoctorReportsAMissingMosh(t *testing.T) {
	// mosh is a nicety. Reporting it as a failure would make a working
	// computer look broken.
	stubTools(t, map[string]bool{"ssh": true})
	t.Setenv("SSH_AUTH_SOCK", "/tmp/agent.sock")
	t.Setenv("HOME", t.TempDir())
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n")

	out, err := execute(t, "--config", dir, "machine", "doctor")
	if err != nil {
		t.Fatalf("a missing mosh should not fail the whole check: %v", err)
	}
	if !strings.Contains(out, "mosh") {
		t.Fatalf("mosh is not reported: %q", out)
	}
	if !strings.Contains(out, machineWarn) {
		t.Fatalf("a missing mosh should be a warning, not a failure: %q", out)
	}
}

func TestMachineDoctorSaysTheAliasesAreStale(t *testing.T) {
	// A workspace added after the last `aliases --write` is not reachable by
	// name, and nothing else would tell you.
	stubTools(t, map[string]bool{"ssh": true, "mosh": true})
	t.Setenv("SSH_AUTH_SOCK", "/tmp/agent.sock")
	home := t.TempDir()
	t.Setenv("HOME", home)

	path := filepath.Join(home, ".ssh", "config")
	if err := aliases.Write(path, "Host bob-devmachine\n    HostName old.invalid\n"); err != nil {
		t.Fatal(err)
	}

	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n"+
		"workspaces:\n  - name: alice\n")

	out, err := execute(t, "--config", dir, "machine", "doctor")
	if err == nil {
		t.Fatal("a stale alias file should fail the check")
	}
	if !strings.Contains(out, "aliases") || !strings.Contains(out, "stale") {
		t.Fatalf("it does not say the aliases are stale: %q", out)
	}
}

func TestMachineDoctorPassesWithNoWorkspaceConfigured(t *testing.T) {
	stubTools(t, map[string]bool{"ssh": true, "mosh": true})
	t.Setenv("SSH_AUTH_SOCK", "/tmp/agent.sock")
	t.Setenv("HOME", t.TempDir())
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n")

	out, err := execute(t, "--config", dir, "machine", "doctor")
	if err != nil {
		t.Fatalf("machine doctor returned %v: %q", err, out)
	}
}

func TestMachineSetupOnLinuxSaysWhatToInstall(t *testing.T) {
	// Guessing a package manager on somebody's own laptop is worse than
	// telling them what is missing.
	origOS := hostOS
	hostOS = "linux"
	t.Cleanup(func() { hostOS = origOS })
	stubTools(t, map[string]bool{})

	out, err := execute(t, "machine", "setup")
	if err != nil {
		t.Fatalf("machine setup returned %v", err)
	}
	for _, want := range []string{"ssh", "mosh", "yourself"} {
		if !strings.Contains(out, want) {
			t.Fatalf("got %q", out)
		}
	}
}

func TestMachineSetupSaysWhenNothingIsMissing(t *testing.T) {
	stubTools(t, map[string]bool{"ssh": true, "mosh": true})

	out, err := execute(t, "machine", "setup")
	if err != nil {
		t.Fatalf("machine setup returned %v", err)
	}
	if !strings.Contains(out, "Nothing to install") {
		t.Fatalf("got %q", out)
	}
}

func TestMachineSetupOnAMacWithNoHomebrewRefuses(t *testing.T) {
	origOS := hostOS
	hostOS = "darwin"
	t.Cleanup(func() { hostOS = origOS })
	stubTools(t, map[string]bool{})

	_, err := execute(t, "machine", "setup", "--yes")
	if err == nil {
		t.Fatal("expected an error with no Homebrew on a Mac")
	}
	if !strings.Contains(err.Error(), "homebrew") {
		t.Fatalf("the error does not name what is missing: %v", err)
	}
}
