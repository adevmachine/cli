package commands

import (
	"errors"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/config"
)

// withoutLima empties PATH, which is the only honest way to meet the machine
// that has no Lima on it from inside a test.
func withoutLima(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", "")
}

func TestCreateLocalSaysHowToInstallLima(t *testing.T) {
	withoutLima(t)

	_, err := execute(t, "machines", "create-local", "alpha")
	if err == nil {
		t.Fatal("it reported a machine where there is no lima")
	}
	if !strings.Contains(err.Error(), "brew install lima") {
		t.Fatalf("got %v", err)
	}
}

func TestLocalMachineCommandsNeedAName(t *testing.T) {
	for _, command := range []string{"create-local", "start", "stop", "delete-local"} {
		if _, err := execute(t, "machines", command); err == nil {
			t.Fatalf("%s ran with no name", command)
		}
	}
}

func TestDeleteLocalAsksBeforeDestroying(t *testing.T) {
	withoutLima(t)

	// The question comes first, so a no costs nothing — not even the lima
	// that is not installed here.
	out, err := executeWithInput(t, "n\n", "machines", "delete-local", "alpha")
	if !errors.Is(err, errDeclined) {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(out, "alpha") {
		t.Fatalf("the question does not name the machine: %q", out)
	}
}

func TestDeleteLocalWithYesDoesNotAsk(t *testing.T) {
	withoutLima(t)

	_, err := executeWithInput(t, "", "machines", "delete-local", "--yes", "alpha")
	if err == nil || errors.Is(err, errDeclined) {
		t.Fatalf("it asked, or did nothing: %v", err)
	}
	if !strings.Contains(err.Error(), "brew install lima") {
		t.Fatalf("got %v", err)
	}
}

func TestMachinesAddBootstrapsAndWritesTheNewMachine(t *testing.T) {
	dir := writeConfigDir(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\ndomain: example.com\n")
	steps := stubBootstrap(t, bootstrapStubs{keyWorks: false})

	// The same questions as setup, minus the domain, then the same bootstrap.
	out, err := executeWithInput(t, "sandbox\n198.51.100.7\nroot\n2222\n1\ndevmachine\n",
		"--config", dir, "machines", "add")
	if err != nil {
		t.Fatalf("machines add returned %v (%s)", err, out)
	}

	if !steps.installedKey || !steps.proved || !steps.hardened || !steps.ansible {
		t.Fatalf("it did not run the bootstrap: %#v", steps)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Machines) != 2 || cfg.Machines[1].Name != "sandbox" || cfg.Machines[1].Port != 2222 {
		t.Fatalf("machines = %#v", cfg.Machines)
	}
	if cfg.Domain != "example.com" {
		t.Fatalf("the rest of the configuration changed: %q", cfg.Domain)
	}
}

func TestMachinesAddRefusesANameThatIsTaken(t *testing.T) {
	dir := writeConfigDir(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n")
	steps := stubBootstrap(t, bootstrapStubs{keyWorks: true})

	_, err := executeWithInput(t, "main\n198.51.100.7\nroot\n22\n1\n", "--config", dir, "machines", "add")
	if err == nil {
		t.Fatal("two machines were allowed the same name")
	}
	// The name is refused before anything is done to a server.
	if steps.hardened {
		t.Fatal("it hardened a machine it was never going to record")
	}
}

func TestMachinesRmForgetsTheMachineAndSaysTheServerIsUntouched(t *testing.T) {
	dir := writeConfigDir(t, `machines:
  - name: main
    hosts: [203.0.113.10]
  - name: sandbox
    hosts: [198.51.100.7]
`)

	out, err := executeWithInput(t, "", "--config", dir, "machines", "rm", "--yes", "sandbox")
	if err != nil {
		t.Fatalf("machines rm returned %v", err)
	}

	// `rm` and `delete-local` are one letter apart in a person's head. Saying
	// plainly that the server keeps running is what keeps them apart.
	for _, want := range []string{"still running", "delete-local"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the output does not say what it did not do: %q", out)
		}
	}

	cfg, _ := config.Load(dir)
	if len(cfg.Machines) != 1 || cfg.Machines[0].Name != "main" {
		t.Fatalf("machines = %#v", cfg.Machines)
	}
}

func TestMachinesRmAsksFirst(t *testing.T) {
	dir := writeConfigDir(t, `machines:
  - name: main
    hosts: [203.0.113.10]
  - name: sandbox
    hosts: [198.51.100.7]
`)

	out, err := executeWithInput(t, "n\n", "--config", dir, "machines", "rm", "sandbox")
	if !errors.Is(err, errDeclined) {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(out, "sandbox") {
		t.Fatalf("the question does not name the machine: %q", out)
	}

	cfg, _ := config.Load(dir)
	if len(cfg.Machines) != 2 {
		t.Fatal("a no removed it anyway")
	}
}

func TestMachinesRmRefusesToOrphanAWorkspace(t *testing.T) {
	dir := writeConfigDir(t, `machines:
  - name: main
    hosts: [203.0.113.10]
  - name: sandbox
    hosts: [198.51.100.7]
workspaces:
  - name: alice
    machine: sandbox
`)

	_, err := executeWithInput(t, "", "--config", dir, "machines", "rm", "--yes", "sandbox")
	if err == nil {
		t.Fatal("it left a workspace pointing at nothing")
	}
	if !strings.Contains(err.Error(), "alice") {
		t.Fatalf("the error does not name the workspace: %v", err)
	}
}

func TestMachinesRmIsNotDeleteLocal(t *testing.T) {
	// Two commands whose names look alike, one of which destroys a machine.
	// The help of each has to say which is which.
	out, err := execute(t, "machines", "rm", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "delete-local") {
		t.Fatalf("rm's help does not point at the destructive one: %q", out)
	}

	out, err = execute(t, "machines", "delete-local", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "machines rm") {
		t.Fatalf("delete-local's help does not point at the harmless one: %q", out)
	}
}
