package commands

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/config"
)

func TestSetupWritesAConfigurationThatLoads(t *testing.T) {
	dir := t.TempDir()
	answers := strings.NewReader("sandbox\n198.51.100.7\nroot\n2222\nexample.com\n")

	if err := runSetup(dir, answers, io.Discard, false); err != nil {
		t.Fatalf("runSetup returned %v", err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("the configuration it wrote does not load: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("the configuration it wrote is invalid: %v", err)
	}

	if len(cfg.Machines) != 1 {
		t.Fatalf("machines = %#v", cfg.Machines)
	}
	m := cfg.Machines[0]
	if m.Name != "sandbox" || m.Hosts[0].Address != "198.51.100.7" || m.Port != 2222 {
		t.Fatalf("machine = %#v", m)
	}
	if cfg.Domain != "example.com" {
		t.Fatalf("domain = %q", cfg.Domain)
	}
}

func TestSetupTakesTheDefaultsOnEmptyAnswers(t *testing.T) {
	dir := t.TempDir()
	// Only the address is typed; everything else is left blank.
	answers := strings.NewReader("\n203.0.113.10\n\n\n\n")

	if err := runSetup(dir, answers, io.Discard, false); err != nil {
		t.Fatalf("runSetup returned %v", err)
	}

	cfg, _ := config.Load(dir)
	m := cfg.Machines[0]
	if m.Name != "main" || m.User != config.DefaultAdminUser || m.Port != config.DefaultPort {
		t.Fatalf("the defaults were not applied: %#v", m)
	}
}

func TestSetupRefusesAnAddressThatWasNotGiven(t *testing.T) {
	dir := t.TempDir()
	answers := strings.NewReader("main\n\nroot\n22\n\n")

	if err := runSetup(dir, answers, io.Discard, false); err == nil {
		t.Fatal("expected an error when no address was given")
	}
	if _, err := os.Stat(filepath.Join(dir, config.FileName)); err == nil {
		t.Fatal("it wrote a configuration it had already refused")
	}
}

func TestSetupRefusesAPortThatIsNotANumber(t *testing.T) {
	dir := t.TempDir()
	answers := strings.NewReader("main\n203.0.113.10\nroot\nnot-a-port\n\n")

	if err := runSetup(dir, answers, io.Discard, false); err == nil {
		t.Fatal("expected an error for a port that is not a number")
	}
}

func TestSetupRefusesToOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	existing := "machines:\n  - name: keep-me\n    hosts: [203.0.113.99]\n"
	os.WriteFile(filepath.Join(dir, config.FileName), []byte(existing), 0o600)

	answers := strings.NewReader("main\n203.0.113.10\nroot\n22\n\n")
	err := runSetup(dir, answers, io.Discard, false)
	if err == nil {
		t.Fatal("expected setup to refuse to overwrite")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("the error does not say how to proceed: %v", err)
	}

	cfg, _ := config.Load(dir)
	if cfg.Machines[0].Name != "keep-me" {
		t.Fatal("the existing configuration was overwritten anyway")
	}
}

func TestSetupOverwritesWithForce(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, config.FileName),
		[]byte("machines:\n  - name: old\n    hosts: [203.0.113.99]\n"), 0o600)

	answers := strings.NewReader("new\n203.0.113.10\nroot\n22\n\n")
	if err := runSetup(dir, answers, io.Discard, true); err != nil {
		t.Fatalf("runSetup returned %v", err)
	}

	cfg, _ := config.Load(dir)
	if cfg.Machines[0].Name != "new" {
		t.Fatalf("machine = %q", cfg.Machines[0].Name)
	}
}

func TestSetupSaysItChangedNothingOnTheMachine(t *testing.T) {
	dir := t.TempDir()
	out := &strings.Builder{}

	runSetup(dir, strings.NewReader("main\n203.0.113.10\n\n\n\n"), out, false)

	// Somebody running a command called "setup" reasonably expects a server to
	// have been prepared. Saying plainly that nothing happened is the point.
	if !strings.Contains(out.String(), "Nothing on the machine has changed") {
		t.Fatalf("the output does not say what it did not do: %q", out.String())
	}
}

func TestTheConfigurationFileIsNotReadableByOthers(t *testing.T) {
	dir := t.TempDir()

	runSetup(dir, strings.NewReader("main\n203.0.113.10\n\n\n\n"), io.Discard, false)

	info, err := os.Stat(filepath.Join(dir, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("mode = %o, want 600", perm)
	}
}
