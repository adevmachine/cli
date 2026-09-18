package commands

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureInteractive records what would have been launched instead of
// launching it, so these tests never open a session.
func captureInteractive(t *testing.T) *[]string {
	t.Helper()
	var got []string
	runInteractive = func(name string, args ...string) error {
		got = append([]string{name}, args...)
		return nil
	}
	lookPath = func(string) (string, error) { return "/usr/bin/fake", nil }
	t.Cleanup(func() { runInteractive = realExecCommand; lookPath = realLookPath })
	return &got
}

func configWith(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSSHUsesTheConfiguredUserPortAndKey(t *testing.T) {
	got := captureInteractive(t)
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    user: alice\n    port: 2222\n    key: /keys/id_ed25519\n")

	if _, err := execute(t, "--config", dir, "ssh"); err != nil {
		t.Fatalf("ssh returned %v", err)
	}

	line := strings.Join(*got, " ")
	for _, want := range []string{"ssh", "-p 2222", "-i /keys/id_ed25519", "alice@203.0.113.10"} {
		if !strings.Contains(line, want) {
			t.Fatalf("the command is missing %q: %q", want, line)
		}
	}
}

func TestSSHTakesAUserFromTheArgument(t *testing.T) {
	got := captureInteractive(t)
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    user: root\nworkspaces:\n  - name: bob\n")

	if _, err := execute(t, "--config", dir, "ssh", "bob"); err != nil {
		t.Fatalf("ssh returned %v", err)
	}

	line := strings.Join(*got, " ")
	if !strings.Contains(line, "bob@203.0.113.10") {
		t.Fatalf("the argument did not override the configured user: %q", line)
	}
}

func TestMoshPassesThePortThroughItsSSHOption(t *testing.T) {
	got := captureInteractive(t)
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    port: 2222\n    key: /keys/id_ed25519\n")

	if _, err := execute(t, "--config", dir, "mosh"); err != nil {
		t.Fatalf("mosh returned %v", err)
	}

	line := strings.Join(*got, " ")
	// mosh does not take -p or -i itself; they have to travel inside --ssh.
	if !strings.Contains(line, "--ssh=ssh -p 2222 -i /keys/id_ed25519") {
		t.Fatalf("the port and key did not reach mosh's ssh command: %q", line)
	}
	if !strings.Contains(line, "root@203.0.113.10") {
		t.Fatalf("the target is missing: %q", line)
	}
}

func TestAnInteractiveCommandSaysWhenTheBinaryIsMissing(t *testing.T) {
	runInteractive = func(string, ...string) error { return nil }
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { runInteractive = realExecCommand; lookPath = realLookPath })

	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n")

	_, err := execute(t, "--config", dir, "mosh")
	if err == nil {
		t.Fatal("expected an error when mosh is not installed")
	}
	if !strings.Contains(err.Error(), "mosh") {
		t.Fatalf("the error does not name the missing binary: %v", err)
	}
}

func TestAnInteractiveCommandRefusesAnInvalidConfig(t *testing.T) {
	got := captureInteractive(t)
	dir := configWith(t, "domain: example.com\n")

	if _, err := execute(t, "--config", dir, "ssh"); err == nil {
		t.Fatal("expected an error when there is no host")
	}
	if len(*got) != 0 {
		t.Fatalf("it launched a session with an invalid configuration: %v", *got)
	}
}

func TestHumanBytesReadsLikeAPersonWouldWriteIt(t *testing.T) {
	cases := map[uint64]string{
		0:          "0B",
		512:        "512B",
		1024:       "1.0K",
		1536:       "1.5K",
		1073741824: "1.0G",
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Fatalf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

const twoMachineConfig = `
machines:
  - name: main
    hosts: [203.0.113.10]
    port: 22
  - name: sandbox
    hosts: [198.51.100.7]
    port: 2222
    key: /keys/sandbox
workspaces:
  - name: alice
    machine: main
  - name: bob
    machine: sandbox
    user: bob-dev
`

func TestSSHSendsAWorkspaceToItsOwnMachine(t *testing.T) {
	got := captureInteractive(t)
	dir := configWith(t, twoMachineConfig)

	if _, err := execute(t, "--config", dir, "ssh", "bob"); err != nil {
		t.Fatalf("ssh returned %v", err)
	}

	line := strings.Join(*got, " ")
	// bob lives on sandbox, so its address, port, key and Linux account are
	// the ones that have to appear — never main's.
	for _, want := range []string{"-p 2222", "-i /keys/sandbox", "bob-dev@198.51.100.7"} {
		if !strings.Contains(line, want) {
			t.Fatalf("the command is missing %q: %q", want, line)
		}
	}
	if strings.Contains(line, "203.0.113.10") {
		t.Fatalf("it reached the wrong machine: %q", line)
	}
}

func TestTwoWorkspacesReachDifferentMachines(t *testing.T) {
	dir := configWith(t, twoMachineConfig)

	got := captureInteractive(t)
	if _, err := execute(t, "--config", dir, "ssh", "alice"); err != nil {
		t.Fatalf("ssh alice returned %v", err)
	}
	alice := strings.Join(*got, " ")

	got = captureInteractive(t)
	if _, err := execute(t, "--config", dir, "ssh", "bob"); err != nil {
		t.Fatalf("ssh bob returned %v", err)
	}
	bob := strings.Join(*got, " ")

	if !strings.Contains(alice, "alice@203.0.113.10") {
		t.Fatalf("alice did not land on main: %q", alice)
	}
	if !strings.Contains(bob, "bob-dev@198.51.100.7") {
		t.Fatalf("bob did not land on sandbox: %q", bob)
	}
}

func TestSSHWithNoWorkspaceNeedsAMachineWhenThereAreSeveral(t *testing.T) {
	got := captureInteractive(t)
	dir := configWith(t, twoMachineConfig)

	_, err := execute(t, "--config", dir, "ssh")
	if err == nil {
		t.Fatal("expected an error rather than a guess between two machines")
	}
	if !strings.Contains(err.Error(), "--machine") {
		t.Fatalf("the error does not say how to choose: %v", err)
	}
	if len(*got) != 0 {
		t.Fatalf("it opened a session on a machine nobody named: %v", *got)
	}
}

func TestSSHWithNoWorkspaceUsesTheMachineFlag(t *testing.T) {
	got := captureInteractive(t)
	dir := configWith(t, twoMachineConfig)

	if _, err := execute(t, "--config", dir, "--machine", "sandbox", "ssh"); err != nil {
		t.Fatalf("ssh returned %v", err)
	}

	line := strings.Join(*got, " ")
	if !strings.Contains(line, "root@198.51.100.7") {
		t.Fatalf("it did not use the named machine's admin login: %q", line)
	}
}

func TestAnUnknownWorkspaceListsTheOnesThatExist(t *testing.T) {
	got := captureInteractive(t)
	dir := configWith(t, twoMachineConfig)

	_, err := execute(t, "--config", dir, "ssh", "carol")
	if err == nil {
		t.Fatal("expected an error for an unknown workspace")
	}
	for _, want := range []string{"alice", "bob"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the error does not list %q: %v", want, err)
		}
	}
	if len(*got) != 0 {
		t.Fatalf("it opened a session for a workspace that does not exist: %v", *got)
	}
}

func TestAWorkspaceUsesItsNameAsTheLinuxAccount(t *testing.T) {
	got := captureInteractive(t)
	dir := configWith(t, twoMachineConfig)

	if _, err := execute(t, "--config", dir, "ssh", "alice"); err != nil {
		t.Fatalf("ssh returned %v", err)
	}

	if line := strings.Join(*got, " "); !strings.Contains(line, "alice@") {
		t.Fatalf("the workspace name was not used as the account: %q", line)
	}
}
