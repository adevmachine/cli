package commands

import (
	"errors"
	"strings"
	"testing"
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
