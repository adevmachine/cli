package credentials

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/packages"
	"github.com/adevmachine/cli/internal/remote"
)

// awkward is what a naive implementation gets wrong: a space, a single quote
// and a dollar are all ordinary characters in a token.
const awkward = `a b'c$d"e\f`

func TestEnvBodySurvivesAShellRoundTrip(t *testing.T) {
	body := EnvBody(secret("probe", "PROBE_TOKEN"), awkward)

	// The file is sourced by a shell. Eyeballing the quoting proves nothing,
	// so a real shell reads it back.
	if got := sourceAndEcho(t, body, "PROBE_TOKEN"); got != awkward {
		t.Fatalf("the round trip gave %q, want %q", got, awkward)
	}
}

func TestEnvBodyExportsTheValue(t *testing.T) {
	// A provider's entrypoint sources the file with `set -a`, but a person
	// reading it should still see a variable it can export.
	body := EnvBody(secret("probe", "PROBE_TOKEN"), "plain")
	if !strings.Contains(body, "PROBE_TOKEN=") {
		t.Fatalf("got:\n%s", body)
	}
	if strings.Contains(body, "plain\nplain") {
		t.Fatalf("the value was written twice:\n%s", body)
	}
}

func TestEnvBodySurvivesANewlineAndATrailingQuote(t *testing.T) {
	for _, value := range []string{"line one\nline two", "ends with a quote'", "'", "$(rm -rf /)"} {
		body := EnvBody(secret("probe", "PROBE_TOKEN"), value)
		if got := sourceAndEcho(t, body, "PROBE_TOKEN"); got != value {
			t.Fatalf("the round trip gave %q, want %q", got, value)
		}
	}
}

func TestPushNeverPutsTheValueInAnArgument(t *testing.T) {
	c := &recordingClient{}

	if err := Push(context.Background(), c, secret("probe", "PROBE_TOKEN"), awkward); err != nil {
		t.Fatal(err)
	}
	// An argument is readable in `ps` by every account on the machine for as
	// long as the command runs.
	if strings.Contains(strings.Join(c.commands, "\n"), awkward) {
		t.Fatalf("the value reached the command line:\n%s", strings.Join(c.commands, "\n"))
	}
	if len(c.inputs) != 1 {
		t.Fatalf("the value did not go in on stdin: %#v", c.inputs)
	}
	if got := sourceAndEcho(t, c.inputs[0], "PROBE_TOKEN"); got != awkward {
		t.Fatalf("what went in on stdin reads back as %q", got)
	}
}

func TestPushWritesModeSixHundredOwnedByRoot(t *testing.T) {
	c := &recordingClient{}

	if err := Push(context.Background(), c, secret("probe", "PROBE_TOKEN"), "value"); err != nil {
		t.Fatal(err)
	}
	command := strings.Join(c.commands, "\n")
	for _, want := range []string{"/etc/devmachine/probe/env", "0700", "0600", "root"} {
		if !strings.Contains(command, want) {
			t.Fatalf("the command does not set %q:\n%s", want, command)
		}
	}
}

func TestPushPutsAWorkspaceFileInThatWorkspacesHome(t *testing.T) {
	c := &recordingClient{}
	d := secret("sentry", "SENTRY_TOKEN")
	d.Scope = packages.ScopeWorkspace
	d.Workspace, d.LinuxUser = "alice", "alice"

	if err := Push(context.Background(), c, d, "value"); err != nil {
		t.Fatal(err)
	}
	command := strings.Join(c.commands, "\n")
	// Only the CLI knows whose home `~` is, and root writing into a home
	// leaves a file the workspace cannot read unless the owner is set.
	if !strings.Contains(command, "alice") {
		t.Fatalf("the workspace account is nowhere in it:\n%s", command)
	}
	if strings.Contains(command, "/etc/devmachine") {
		t.Fatalf("a workspace credential went to the machine's directory:\n%s", command)
	}
}

func TestPushRefusesALoginKind(t *testing.T) {
	c := &recordingClient{}
	d := Declared{
		Credential: packages.Credential{
			Name: "gh", Kind: packages.KindLogin, Scope: packages.ScopeMachine,
			Command: "gh auth login", StoredAt: "~/.config/gh/hosts.yml",
		},
		Package: "gh-login",
	}

	err := Push(context.Background(), c, d, "value")
	if err == nil {
		t.Fatal("a browser login was pushed")
	}
	// Nobody can push a login, so the error has to hand over the command that
	// does work.
	if !strings.Contains(err.Error(), "devmachine login gh") {
		t.Fatalf("the error does not say what to run instead: %v", err)
	}
	if len(c.commands) != 0 {
		t.Fatalf("it touched the machine anyway: %#v", c.commands)
	}
}

func TestPushRefusesALoginForAWorkspaceNamingTheWorkspace(t *testing.T) {
	d := Declared{
		Credential: packages.Credential{
			Name: "claude", Kind: packages.KindLogin, Scope: packages.ScopeWorkspace,
			Command: "claude /login", StoredAt: "~/.claude/.credentials.json",
		},
		Package: "claude-code", Workspace: "alice", LinuxUser: "alice",
	}

	err := Push(context.Background(), &recordingClient{}, d, "value")
	if err == nil || !strings.Contains(err.Error(), "devmachine login claude --workspace alice") {
		t.Fatalf("got %v", err)
	}
}

func TestPushRefusesAnEmptyValue(t *testing.T) {
	c := &recordingClient{}

	if err := Push(context.Background(), c, secret("probe", "PROBE_TOKEN"), ""); err == nil {
		t.Fatal("an empty value was delivered")
	}
	if len(c.commands) != 0 {
		t.Fatalf("it touched the machine anyway: %#v", c.commands)
	}
}

func TestPushWritesAFileKindWhereThePackageSaysItGoes(t *testing.T) {
	c := &recordingClient{}
	d := Declared{
		Credential: packages.Credential{
			Name: "robot", Kind: packages.KindFile, Scope: packages.ScopeWorkspace,
			Path: "~/.config/robot/key.json",
		},
		Package: "robot", Workspace: "alice", LinuxUser: "alice",
	}

	if err := Push(context.Background(), c, d, "{}"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(c.commands, "\n"), ".config/robot/key.json") {
		t.Fatalf("got:\n%s", strings.Join(c.commands, "\n"))
	}
	// A file is delivered as it was given: nothing wraps it in a variable.
	if len(c.inputs) != 1 || c.inputs[0] != "{}" {
		t.Fatalf("the file was rewritten on the way: %#v", c.inputs)
	}
}

func TestDestinationIsTheContractForAMachineSecret(t *testing.T) {
	if got := Destination(secret("hostinger", "HOSTINGER_TOKEN")); got != EnvFile("hostinger") {
		t.Fatalf("got %q", got)
	}
}

// sourceAndEcho writes body to a file, sources it the way a provider's
// entrypoint does, and prints one variable back.
func sourceAndEcho(t *testing.T, body, name string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "env")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `set -a; . "$1"; set +a; printf %s "$` + name + `"`
	out, err := exec.Command("sh", "-c", script, "sh", path).Output()
	if err != nil {
		t.Fatalf("sourcing the file failed: %v", err)
	}
	return string(out)
}

func secret(name, env string) Declared {
	return Declared{
		Credential: packages.Credential{
			Name: name, Kind: packages.KindSecret, Scope: packages.ScopeMachine, Env: env,
		},
		Package: name,
	}
}

// TestPushAgainstTheThrowawayMachine writes a real file on a real machine and
// reads the value back through a real shell. A round trip that works in a
// string comparison and not on a machine is the failure this whole task exists
// to catch, and v0.5 reads this file.
func TestPushAgainstTheThrowawayMachine(t *testing.T) {
	m := testMachine(t)
	ctx := context.Background()

	client, _, err := remote.Dial(ctx, m, "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	defer client.Run(ctx, "rm -rf "+MachineDir("probe"))

	d := secret("probe", "PROBE_TOKEN")
	// Delivering twice is the ordinary case: a push runs again on a machine
	// that already has the value.
	for range 2 {
		if err := Push(ctx, client, d, awkward); err != nil {
			t.Fatal(err)
		}
	}

	out, err := client.Run(ctx,
		`sh -c 'set -a; . `+EnvFile("probe")+`; set +a; printf %s "$PROBE_TOKEN"'`)
	if err != nil {
		t.Fatal(err)
	}
	if out != awkward {
		t.Fatalf("the machine reads the value back as %q, want %q", out, awkward)
	}

	modes, err := client.Run(ctx, "stat -c '%a %U' "+MachineDir("probe")+" "+EnvFile("probe"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Fields(modes); len(got) != 4 ||
		got[0] != "700" || got[1] != "root" || got[2] != "600" || got[3] != "root" {
		t.Fatalf("got %q", strings.TrimSpace(modes))
	}
}

// TestPushToAWorkspaceAgainstTheThrowawayMachine proves the part no unit test
// can: root writing into somebody else's home leaves everything it made owned
// by that account.
func TestPushToAWorkspaceAgainstTheThrowawayMachine(t *testing.T) {
	m := testMachine(t)
	ctx := context.Background()

	client, _, err := remote.Dial(ctx, m, "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if _, err := client.Run(ctx, "id alice >/dev/null 2>&1 || useradd -m alice"); err != nil {
		t.Skipf("the test machine cannot make an account: %v", err)
	}
	defer client.Run(ctx, "userdel -r alice")

	d := Declared{
		Credential: packages.Credential{
			Name: "robot", Kind: packages.KindFile, Scope: packages.ScopeWorkspace,
			Path: "~/.config/robot/key.json",
		},
		Package: "robot", Workspace: "alice", LinuxUser: "alice",
	}
	if err := Push(ctx, client, d, "{\"token\": \"value\"}\n"); err != nil {
		t.Fatal(err)
	}

	owners, err := client.Run(ctx,
		"stat -c '%a %U' /home/alice/.config /home/alice/.config/robot /home/alice/.config/robot/key.json")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(owners)
	if len(got) != 6 {
		t.Fatalf("got %q", strings.TrimSpace(owners))
	}
	// The directories on the way matter as much as the file: a root-owned
	// ~/.config would break the workspace's own tools later, for a reason
	// nobody would connect to a credential.
	for i, want := range []string{"700", "alice", "700", "alice", "600", "alice"} {
		if got[i] != want {
			t.Fatalf("got %q, want 700 alice 700 alice 600 alice", strings.TrimSpace(owners))
		}
	}

	body, err := client.Run(ctx, "cat /home/alice/.config/robot/key.json")
	if err != nil {
		t.Fatal(err)
	}
	if body != "{\"token\": \"value\"}\n" {
		t.Fatalf("the file arrived as %q", body)
	}
}
