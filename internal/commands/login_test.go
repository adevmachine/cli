package commands

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/hostkeys"
	"github.com/mydevmachine/devmachine/internal/packages"
	"github.com/mydevmachine/devmachine/internal/remote"
	"golang.org/x/crypto/ssh"
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
	m, _ := pinnedMachine(t, 2222, true)
	args := loginArgs(m, "root", "127.0.0.1", "gh auth login")

	joined := strings.Join(args, " ")
	for _, want := range []string{"StrictHostKeyChecking=yes", "UserKnownHostsFile=" + m.KnownHostsFile, "HostKeyAlias=[main-devmachine]:2222"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("login does not enforce %q: %#v", want, args)
		}
	}
}

// configWithTailscaleCredential is a machine with a `tailscale` login of its
// own, so `login tailscale` has something to log into.
func configWithTailscaleCredential(t *testing.T) string {
	t.Helper()
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [tailscale-pkg]\n")
	writeCredentialPackage(t, dir, "tailscale-pkg", packages.ScopeMachine,
		"  - name: tailscale\n    kind: manual\n    scope: machine\n    command: tailscale up\n")
	key := commandHostKey(t)
	store, err := hostkeys.Open(filepath.Join(dir, config.KnownHostsFileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("main", 22, key); err != nil {
		t.Fatal(err)
	}
	wasScan := scanHostKey
	scanHostKey = func(context.Context, config.Machine) (ssh.PublicKey, string, error) {
		return key, "203.0.113.10", nil
	}
	t.Cleanup(func() { scanHostKey = wasScan })
	return dir
}

func TestLoginTailscalePrependsTheMachinesTailnetName(t *testing.T) {
	captureInteractive(t)
	client := &recordingRemote{out: `{"Self":{"HostName":"main-abc123"}}`}
	dialing(t, client)
	dir := configWithTailscaleCredential(t)

	out, err := execute(t, "--config", dir, "login", "tailscale")
	if err != nil {
		t.Fatalf("login returned %v", err)
	}
	if !strings.Contains(out, "tailscale:main-abc123") {
		t.Fatalf("did not say what it added: %q", out)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := cfg.Machine("main")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Hosts) != 2 || m.Hosts[0].Address != "tailscale:main-abc123" || m.Hosts[1].Address != "203.0.113.10" {
		t.Fatalf("hosts = %#v", m.Hosts)
	}
	body, err := os.ReadFile(filepath.Join(dir, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "    hosts:\n      - tailscale:main-abc123\n      - 203.0.113.10\n") {
		t.Fatalf("hosts was not written as a block list:\n%s", body)
	}
}

func TestLoginTailscaleWithoutAReadableNameSaysHowToAddItByHand(t *testing.T) {
	captureInteractive(t)
	client := &recordingRemote{out: `not json`}
	dialing(t, client)
	dir := configWithTailscaleCredential(t)

	out, err := execute(t, "--config", dir, "login", "tailscale")
	if err != nil {
		t.Fatalf("login returned %v", err)
	}
	want := "    machines:\n" +
		"      - name: main\n" +
		"        hosts:\n" +
		"          - tailscale:<name>\n" +
		"          - 203.0.113.10\n"
	if !strings.Contains(out, want) {
		t.Fatalf("did not print the YAML to paste, want:\n%s\ngot:\n%s", want, out)
	}
	if !strings.Contains(out, "tailscale status") {
		t.Fatalf("did not say where <name> comes from: %q", out)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := cfg.Machine("main")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Hosts) != 1 {
		t.Fatalf("hosts = %#v, want nothing added", m.Hosts)
	}
}

func TestLoginTailscaleSkipsAnAddressAlreadyPresent(t *testing.T) {
	captureInteractive(t)
	client := &recordingRemote{out: `{"Self":{"HostName":"main-abc123"}}`}
	dialing(t, client)
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [tailscale:main-abc123, 203.0.113.10]\n    packages: [tailscale-pkg]\n")
	writeCredentialPackage(t, dir, "tailscale-pkg", packages.ScopeMachine,
		"  - name: tailscale\n    kind: manual\n    scope: machine\n    command: tailscale up\n")
	key := commandHostKey(t)
	store, err := hostkeys.Open(filepath.Join(dir, config.KnownHostsFileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("main", 22, key); err != nil {
		t.Fatal(err)
	}
	scanHostKey = func(context.Context, config.Machine) (ssh.PublicKey, string, error) {
		return key, "203.0.113.10", nil
	}
	t.Cleanup(func() { scanHostKey = remote.ScanHostKey })

	out, err := execute(t, "--config", dir, "login", "tailscale")
	if err != nil {
		t.Fatalf("login returned %v", err)
	}
	if !strings.Contains(out, "already in") {
		t.Fatalf("did not say it was already there: %q", out)
	}
}

func TestLoginStillAsksForATerminal(t *testing.T) {
	// The whole point is that a person types into the session.
	args := loginArgs(config.Machine{Port: 22}, "root", "127.0.0.1", "gh auth login")
	if !slices.Contains(args, "-t") {
		t.Fatalf("got %#v", args)
	}
}
