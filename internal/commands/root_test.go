package commands

import "testing"

func TestRunUnknownCommandFails(t *testing.T) {
	if err := run([]string{"nope"}); err == nil {
		t.Fatal("expected an error for an unknown command")
	}
}

func TestRunVersionSucceeds(t *testing.T) {
	SetVersion("1.2.3")
	if err := run([]string{"version"}); err != nil {
		t.Fatalf("version returned %v", err)
	}
}

func TestRunNoArgsSucceeds(t *testing.T) {
	if err := run(nil); err != nil {
		t.Fatalf("no args returned %v", err)
	}
}
