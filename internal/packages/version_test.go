package packages

import (
	"strings"
	"testing"
)

func TestConstraintAllows(t *testing.T) {
	cases := []struct {
		constraint, version string
		want                bool
	}{
		{">= 0.2.0", "0.2.0", true},
		{">= 0.2.0", "0.2.1", true},
		{">= 0.2.0", "0.10.0", true},
		{">= 0.2.0", "0.1.9", false},
		{">= 0.2.0", "1.0.0", true},
		{"> 0.2.0", "0.2.0", false},
		{"= 0.2.0", "0.2.0", true},
		{"= 0.2.0", "0.2.1", false},
		{">= 0.2.0", "v0.2.0", true},
		// A development build is not a released version, and refusing every
		// recipe while working on the CLI would be useless.
		{">= 9.9.9", "dev", true},
	}
	for _, c := range cases {
		constraint, err := ParseConstraint(c.constraint)
		if err != nil {
			t.Fatalf("ParseConstraint(%q) returned %v", c.constraint, err)
		}
		if got := constraint.Allows(c.version); got != c.want {
			t.Errorf("%q allows %q = %v, want %v", c.constraint, c.version, got, c.want)
		}
	}
}

func TestConstraintKeepsWhatWasWritten(t *testing.T) {
	c, err := ParseConstraint(">= 0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if c.String() != ">= 0.2.0" {
		t.Fatalf("String() is %q", c.String())
	}
}

func TestParseConstraintRefusesWhatItCannotRead(t *testing.T) {
	for _, s := range []string{"~> 0.2", "^1.0.0", "0.2", "latest", ""} {
		if _, err := ParseConstraint(s); err == nil {
			t.Errorf("ParseConstraint(%q) was accepted", s)
		}
	}
}

func TestParseConstraintSaysHowToWriteOne(t *testing.T) {
	_, err := ParseConstraint("latest")
	if err == nil {
		t.Fatal("latest was accepted")
	}
	if !strings.Contains(err.Error(), ">= 0.2.0") {
		t.Fatalf("the error does not show the shape: %v", err)
	}
}
