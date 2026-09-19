package credentials

import (
	"context"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/packages"
	"github.com/adevmachine/cli/internal/remote"
)

func TestPresentAsksOnce(t *testing.T) {
	c := &recordingClient{}

	// Three credentials must not be three connections: `doctor` and
	// `credentials list` both want this answer for everything at once.
	if _, err := Present(context.Background(), c, []Declared{
		machineLogin("gh"), workspaceLogin("alice", "claude"), workspaceLogin("bob", "claude"),
	}); err != nil {
		t.Fatal(err)
	}
	if len(c.commands) != 1 {
		t.Fatalf("it asked %d times: %#v", len(c.commands), c.commands)
	}
}

func TestPresentAsksNothingWhenNothingIsWanted(t *testing.T) {
	c := &recordingClient{}

	got, err := Present(context.Background(), c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 || len(c.commands) != 0 {
		t.Fatalf("got %#v after %#v", got, c.commands)
	}
}

func TestPresentReadsTheAnswerBack(t *testing.T) {
	c := &recordingClient{answer: "gh\tyes\nalice/claude\tno\n"}

	got, err := Present(context.Background(), c, []Declared{
		machineLogin("gh"), workspaceLogin("alice", "claude"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got["gh"] {
		t.Fatalf("got %#v", got)
	}
	if present, ok := got["alice/claude"]; !ok || present {
		t.Fatalf("got %#v", got)
	}
}

func TestPresentSendsTheWorkspaceAccountSoATildeCanBeExpanded(t *testing.T) {
	c := &recordingClient{}

	if _, err := Present(context.Background(), c, []Declared{workspaceLogin("alice", "claude")}); err != nil {
		t.Fatal(err)
	}
	// `~/.claude/.credentials.json` is written from the workspace's point of
	// view. Only the CLI knows whose home that is.
	if len(c.inputs) != 1 || !strings.Contains(c.inputs[0], "alice") {
		t.Fatalf("the account did not travel: %#v", c.inputs)
	}
	if !strings.Contains(c.inputs[0], "~/.claude/.credentials.json") {
		t.Fatalf("the place did not travel: %#v", c.inputs)
	}
}

func TestPresentLeavesOutACredentialWithNowhereToLook(t *testing.T) {
	// A package that declares a login and no `stored_at` cannot be checked.
	// Saying "missing" would be a claim this has no grounds for.
	d := machineLogin("gh")
	d.StoredAt = ""
	c := &recordingClient{}

	got, err := Present(context.Background(), c, []Declared{d})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["gh"]; ok {
		t.Fatalf("it reported on a credential it could not look for: %#v", got)
	}
	if len(c.commands) != 0 {
		t.Fatalf("it asked anyway: %#v", c.commands)
	}
}

func TestPlaceIsWhereTheValueWasDelivered(t *testing.T) {
	// A secret is not where its package said it keeps a session; it is where
	// this CLI put it.
	if got := Place(secret("hostinger", "HOSTINGER_TOKEN")); got != EnvFile("hostinger") {
		t.Fatalf("got %q", got)
	}
	if got := Place(machineLogin("gh")); got != "~/.config/gh/hosts.yml" {
		t.Fatalf("got %q", got)
	}
}

// TestPresentAgainstTheThrowawayMachine proves the expansion no unit test can:
// that `~` becomes the right account's home on a real machine.
func TestPresentAgainstTheThrowawayMachine(t *testing.T) {
	m := testMachine(t)
	ctx := context.Background()

	client, _, err := remote.Dial(ctx, m, "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	defer client.Run(ctx, "rm -rf "+MachineDir("probe"))

	if _, err := client.Run(ctx, "id alice >/dev/null 2>&1 || useradd -m alice"); err != nil {
		t.Skipf("the test machine cannot make an account: %v", err)
	}
	defer client.Run(ctx, "userdel -r alice")

	if err := Push(ctx, client, secret("probe", "PROBE_TOKEN"), "value"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Run(ctx,
		"install -d -o alice -m 0700 /home/alice/.claude && "+
			"install -o alice -m 0600 /dev/null /home/alice/.claude/.credentials.json"); err != nil {
		t.Fatal(err)
	}

	got, err := Present(ctx, client, []Declared{
		secret("probe", "PROBE_TOKEN"),
		workspaceLogin("alice", "claude"),
		workspaceLogin("bob", "claude"),
		machineLogin("gh"),
	})
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]bool{
		"probe": true, "alice/claude": true, "bob/claude": false, "gh": false,
	} {
		if got[key] != want {
			t.Fatalf("credential %q: got %v, want %v (whole answer %#v)", key, got[key], want, got)
		}
	}
}

func machineLogin(name string) Declared {
	return Declared{
		Credential: packages.Credential{
			Name: name, Kind: packages.KindLogin, Scope: packages.ScopeMachine,
			Command: name + " auth login", StoredAt: "~/.config/" + name + "/hosts.yml",
		},
		Package: name,
	}
}

func workspaceLogin(workspace, name string) Declared {
	return Declared{
		Credential: packages.Credential{
			Name: name, Kind: packages.KindLogin, Scope: packages.ScopeWorkspace,
			Command: name + " /login", StoredAt: "~/.claude/.credentials.json",
		},
		Package: name, Workspace: workspace, LinuxUser: workspace,
	}
}
