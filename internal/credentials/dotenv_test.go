package credentials

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestMergeDotenvAppendsWhenTheNameIsNotThere(t *testing.T) {
	got := MergeDotenv([]byte("FOO=bar\n"), "BAZ", "qux")
	want := "FOO=bar\nBAZ='qux'\n"
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestMergeDotenvReplacesInPlace(t *testing.T) {
	got := MergeDotenv([]byte("FOO=bar\nBAZ=old\nQUX=1\n"), "BAZ", "new")
	want := "FOO=bar\nBAZ='new'\nQUX=1\n"
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestMergeDotenvKeepsCommentsAndBlankLines(t *testing.T) {
	original := "# a comment\n\nFOO=bar\n# BAZ=commented out\n"
	got := MergeDotenv([]byte(original), "BAZ", "value")
	want := "# a comment\n\nFOO=bar\n# BAZ=commented out\nBAZ='value'\n"
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestMergeDotenvOnAnEmptyFile(t *testing.T) {
	got := MergeDotenv(nil, "FOO", "bar")
	if string(got) != "FOO='bar'\n" {
		t.Fatalf("got %q", got)
	}
}

func TestMergeDotenvDoesNotAddATrailingNewlineWhenTheOriginalHadNone(t *testing.T) {
	got := MergeDotenv([]byte("FOO=bar"), "BAZ", "qux")
	want := "FOO=bar\nBAZ='qux'\n"
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestMergeDotenvQuotesASpaceAQuoteAndAHash(t *testing.T) {
	got := MergeDotenv(nil, "TOKEN", awkward)
	if got := sourceAndEcho(t, string(got), "TOKEN"); got != awkward {
		t.Fatalf("the round trip gave %q, want %q", got, awkward)
	}
}

func TestRemoveDotenvLineDropsOnlyTheNamedOne(t *testing.T) {
	original := "FOO=bar\nBAZ=old\nQUX=1\n"
	out, changed := removeDotenvLine([]byte(original), "BAZ")
	if !changed {
		t.Fatal("expected a change")
	}
	if string(out) != "FOO=bar\nQUX=1\n" {
		t.Fatalf("got %q", out)
	}
}

func TestRemoveDotenvLineReportsNoChangeWhenTheNameIsNotThere(t *testing.T) {
	original := []byte("FOO=bar\n")
	out, changed := removeDotenvLine(original, "MISSING")
	if changed {
		t.Fatal("expected no change")
	}
	if string(out) != string(original) {
		t.Fatalf("the untouched file changed: %q", out)
	}
}

func TestSafeWorkspacePathAcceptsAnOrdinaryRelativePath(t *testing.T) {
	got, err := SafeWorkspacePath("app/.env")
	if err != nil {
		t.Fatal(err)
	}
	if got != "app/.env" {
		t.Fatalf("got %q", got)
	}
}

func TestSafeWorkspacePathRefusesAnAbsolutePath(t *testing.T) {
	if _, err := SafeWorkspacePath("/etc/passwd"); err == nil {
		t.Fatal("expected an error for an absolute path")
	}
}

func TestSafeWorkspacePathRefusesEscapingTheHome(t *testing.T) {
	for _, p := range []string{"../outside", "app/../../outside", "..", "../../etc/passwd"} {
		if _, err := SafeWorkspacePath(p); err == nil {
			t.Fatalf("%q escaped the workspace's home without an error", p)
		}
	}
}

func TestSafeWorkspacePathRefusesAnEmptyPath(t *testing.T) {
	if _, err := SafeWorkspacePath(""); err == nil {
		t.Fatal("expected an error for an empty path")
	}
}

func TestSafeWorkspacePathCleansADotSegment(t *testing.T) {
	got, err := SafeWorkspacePath("./app/.env")
	if err != nil {
		t.Fatal(err)
	}
	if got != "app/.env" {
		t.Fatalf("got %q", got)
	}
}

func TestPushWorkspaceEnvWritesIntoTheDefaultFile(t *testing.T) {
	c := &recordingClient{answer: "MISSING\n"}

	if err := PushWorkspaceEnv(context.Background(), c, "alice", DefaultEnvFile, "API_KEY", "secret", false); err != nil {
		t.Fatal(err)
	}
	if len(c.inputs) != 1 {
		t.Fatalf("nothing was written: %#v", c.inputs)
	}
	if !strings.Contains(c.inputs[0], "API_KEY='secret'") {
		t.Fatalf("got %q", c.inputs[0])
	}
	joined := strings.Join(c.commands, "\n")
	if !strings.Contains(joined, DefaultEnvFile) {
		t.Fatalf("the default file is nowhere in the commands: %q", joined)
	}
	if strings.Contains(joined, "secret") {
		t.Fatalf("the value reached a command: %q", joined)
	}
}

func TestPushWorkspaceEnvMergesWithWhatWasAlreadyThere(t *testing.T) {
	readCommand := fmt.Sprintf(dotenvReadScript, shellQuote("alice"), shellQuote(DefaultEnvFile))
	c := &recordingClient{output: map[string]string{
		readCommand: "EXISTS\nOTHER=kept\n",
	}}

	if err := PushWorkspaceEnv(context.Background(), c, "alice", DefaultEnvFile, "API_KEY", "secret", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.inputs[0], "OTHER=kept") || !strings.Contains(c.inputs[0], "API_KEY='secret'") {
		t.Fatalf("got %q", c.inputs[0])
	}
}

func TestPushWorkspaceEnvNeverBacksUpTheDefaultFile(t *testing.T) {
	c := &recordingClient{answer: "MISSING\n"}

	if err := PushWorkspaceEnv(context.Background(), c, "alice", DefaultEnvFile, "API_KEY", "secret", false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(c.commands, "\n"), `backup=1`) {
		t.Fatal("the default file was backed up")
	}
}

func TestPushWorkspaceEnvBacksUpACustomFileTheFirstTime(t *testing.T) {
	c := &recordingClient{answer: "MISSING\n"}

	if err := PushWorkspaceEnv(context.Background(), c, "alice", "app/.env", "API_KEY", "secret", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(c.commands, "\n"), `backup=1`) {
		t.Fatal("a custom file was not asked to back up")
	}
}

func TestPushWorkspaceEnvRefusesAnEmptyValue(t *testing.T) {
	c := &recordingClient{}
	if err := PushWorkspaceEnv(context.Background(), c, "alice", DefaultEnvFile, "API_KEY", "", false); err == nil {
		t.Fatal("an empty value was delivered")
	}
	if len(c.commands) != 0 {
		t.Fatal("it touched the machine anyway")
	}
}

func TestRemoveWorkspaceEnvDropsOnlyTheNamedLine(t *testing.T) {
	readCommand := fmt.Sprintf(dotenvReadScript, shellQuote("alice"), shellQuote(DefaultEnvFile))
	c := &recordingClient{output: map[string]string{
		readCommand: "EXISTS\nAPI_KEY='secret'\nOTHER=kept\n",
	}}

	if err := RemoveWorkspaceEnv(context.Background(), c, "alice", DefaultEnvFile, "API_KEY", false); err != nil {
		t.Fatal(err)
	}
	if len(c.inputs) != 1 || c.inputs[0] != "OTHER=kept\n" {
		t.Fatalf("got %#v", c.inputs)
	}
}

func TestRemoveWorkspaceEnvWritesNothingWhenTheFileDoesNotExist(t *testing.T) {
	c := &recordingClient{output: map[string]string{}, answer: "MISSING\n"}

	if err := RemoveWorkspaceEnv(context.Background(), c, "alice", DefaultEnvFile, "API_KEY", false); err != nil {
		t.Fatal(err)
	}
	if len(c.inputs) != 0 {
		t.Fatal("it wrote to a file that does not exist")
	}
}
