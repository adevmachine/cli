package commands

import (
	"slices"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/config"
)

// loggingIn drives a login without opening a session and without a machine: it
// returns what would have been launched, and the client the copy runs through.
func loggingIn(t *testing.T) (*[]string, *recordingRemote) {
	t.Helper()
	launched := captureInteractive(t)
	client := &recordingRemote{}
	dialing(t, client)
	return launched, client
}

func TestLoginRunsThePackagesOwnCommand(t *testing.T) {
	launched, _ := loggingIn(t)
	dir := configWithCredentials(t, "probe-cmd")

	if _, err := execute(t, "--config", dir, "login", "gh"); err != nil {
		t.Fatalf("login returned %v", err)
	}

	// The CLI reads the command out of the declaration. It knows nothing
	// about gh.
	line := strings.Join(*launched, " ")
	if !strings.Contains(line, "gh auth login") {
		t.Fatalf("the package's own command was not run: %q", line)
	}
}

func TestLoginOpensARealTerminal(t *testing.T) {
	launched, _ := loggingIn(t)
	dir := configWithCredentials(t, "probe-tty")

	if _, err := execute(t, "--config", dir, "login", "gh"); err != nil {
		t.Fatalf("login returned %v", err)
	}

	// A device code and a browser prompt reach a person or they reach nobody.
	line := strings.Join(*launched, " ")
	if !strings.HasPrefix(line, "ssh -t ") {
		t.Fatalf("the session has no terminal: %q", line)
	}
}

func TestLoginForAWorkspaceLogsInAsThatWorkspace(t *testing.T) {
	launched, _ := loggingIn(t)
	dir := configWithCredentials(t, "probe-as")

	if _, err := execute(t, "--config", dir, "login", "claude", "--workspace", "alice"); err != nil {
		t.Fatalf("login returned %v", err)
	}

	line := strings.Join(*launched, " ")
	if !strings.Contains(line, "alice@203.0.113.10") {
		t.Fatalf("it did not log in as the workspace: %q", line)
	}
	if !strings.Contains(line, "claude /login") {
		t.Fatalf("the workspace's own command was not run: %q", line)
	}
}

func TestLoginForAWorkspaceCredentialWithoutAWorkspaceRefuses(t *testing.T) {
	launched, _ := loggingIn(t)
	dir := configWithCredentials(t, "probe-which")

	_, err := execute(t, "--config", dir, "login", "claude")
	if err == nil {
		t.Fatal("it guessed which workspace to log in as")
	}
	if !strings.Contains(err.Error(), "--workspace") {
		t.Fatalf("the error does not name the flag: %v", err)
	}
	if len(*launched) > 0 {
		t.Fatalf("it opened a session anyway: %q", *launched)
	}
}

func TestLoginRefusesASecret(t *testing.T) {
	loggingIn(t)
	dir := configWithCredentials(t, "probe-secret")

	_, err := execute(t, "--config", dir, "login", "probe-secret", "--workspace", "alice")
	if err == nil {
		t.Fatal("it tried to log into a secret")
	}
	if !strings.Contains(err.Error(), "devmachine secrets set probe-secret") {
		t.Fatalf("the error does not say what to run instead: %v", err)
	}
}

func TestLoginKeepsAMachineResultWhereTheWorkspacesWillFindIt(t *testing.T) {
	_, client := loggingIn(t)
	dir := configWithCredentials(t, "probe-shared")

	if _, err := execute(t, "--config", dir, "login", "gh"); err != nil {
		t.Fatalf("login returned %v", err)
	}

	// One login for the machine, kept where the next sync copies it into
	// every workspace that declares the package.
	ran := strings.Join(client.ran, "\n")
	for _, want := range []string{"/etc/devmachine/gh", ".config/gh/hosts.yml"} {
		if !strings.Contains(ran, want) {
			t.Fatalf("the result was not kept at %q:\n%s", want, ran)
		}
	}
}

func TestLoginForAWorkspaceKeepsNothingOnTheMachine(t *testing.T) {
	_, client := loggingIn(t)
	dir := configWithCredentials(t, "probe-own")

	if _, err := execute(t, "--config", dir, "login", "claude", "--workspace", "alice"); err != nil {
		t.Fatalf("login returned %v", err)
	}

	// Accounts differ between workspaces, so there is no master to copy.
	if strings.Contains(strings.Join(client.ran, "\n"), "/etc/devmachine") {
		t.Fatalf("a workspace login was copied to the machine: %#v", client.ran)
	}
}

func TestLoginRefusesANameNobodyDeclared(t *testing.T) {
	loggingIn(t)
	dir := configWithCredentials(t, "probe-absent")

	_, err := execute(t, "--config", dir, "login", "nothing")
	if err == nil {
		t.Fatal("it tried to log into a credential nobody declared")
	}
	if !strings.Contains(err.Error(), "credentials list") {
		t.Fatalf("the error does not say where to look: %v", err)
	}
}

func TestLoginRefusesAWorkspaceThatDoesNotDeclareIt(t *testing.T) {
	loggingIn(t)
	dir := configWithCredentials(t, "probe-elsewhere")

	_, err := execute(t, "--config", dir, "login", "claude", "--workspace", "bob")
	if err == nil {
		t.Fatal("it logged in for a workspace that does not want it")
	}
	if !strings.Contains(err.Error(), "bob") {
		t.Fatalf("the error does not name the workspace: %v", err)
	}
}

func TestLoginRecordsWhatWasAskedOfTheMachine(t *testing.T) {
	loggingIn(t)
	dir := configWithCredentials(t, "probe-log")

	if _, err := execute(t, "--config", dir, "login", "gh"); err != nil {
		t.Fatalf("login returned %v", err)
	}

	line := historyLines(t, dir)[0]
	if !strings.Contains(line, "login gh") {
		t.Fatalf("the log line leaves the login out: %q", line)
	}
}

func TestLoginTrustsTheMachineTheSameWayEveryOtherCommandDoes(t *testing.T) {
	// `login` hands the connection to the system ssh, which reads the
	// operator's own known_hosts. Every other command dials with the Go
	// client, which pins nothing (remote.go). A machine the CLI created
	// minutes ago is in nobody's known_hosts, so leaving the default made
	// `login` fail on every machine the CLI builds, with "Host key
	// verification failed". Whether the CLI pins host keys at all is one
	// decision for the whole CLI; until it is made, one command may not
	// answer it differently from the rest.
	args := loginArgs(config.Machine{Port: 2222}, "root", "127.0.0.1", "gh auth login")

	joined := strings.Join(args, " ")
	for _, want := range []string{"StrictHostKeyChecking=no", "UserKnownHostsFile=/dev/null"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("login does not reach a machine the CLI just made: %#v", args)
		}
	}
}

func TestLoginStillAsksForATerminal(t *testing.T) {
	// The whole point is that a person types into the session.
	args := loginArgs(config.Machine{Port: 22}, "root", "127.0.0.1", "gh auth login")
	if !slices.Contains(args, "-t") {
		t.Fatalf("got %#v", args)
	}
}
