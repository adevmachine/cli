package commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/hostkeys"
	"github.com/mydevmachine/devmachine/internal/packages"
	"golang.org/x/crypto/ssh"
)

// configWithNetwork is a machine that installs the acme-net network package,
// with its host key pinned so a login can open a session.
func configWithNetwork(t *testing.T, machineExtra string) string {
	t.Helper()
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [acme-net]\n"+machineExtra)
	localNetworkPackage(t, dir, "acme-net", "acme", "exit 3\n")
	pinForLogin(t, dir)
	return dir
}

func pinForLogin(t *testing.T, dir string) {
	t.Helper()
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
}

func hostsOf(t *testing.T, dir string) []string {
	t.Helper()
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := cfg.Machine("main")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, h := range m.Hosts {
		out = append(out, h.Address)
	}
	return out
}

func TestLoginToANetworkRunsItsJoinOnTheMachineInATerminal(t *testing.T) {
	launched := captureInteractive(t)
	dialing(t, &recordingRemote{out: "main-abc\n"})
	dir := configWithNetwork(t, "")

	if _, err := execute(t, "--config", dir, "login", "acme-net"); err != nil {
		t.Fatal(err)
	}
	line := strings.Join(*launched, " ")
	if !strings.HasPrefix(line, "ssh -t ") || !strings.Contains(line, "root@203.0.113.10") {
		t.Fatalf("the join did not get a terminal on the machine: %q", line)
	}
	if !strings.Contains(line, "/opt/devmachine/roles.local/acme-net/bin/join") {
		t.Fatalf("the package's own join was not run: %q", line)
	}
}

func TestLoginToANetworkHandsJoinTheMachinesSettings(t *testing.T) {
	launched := captureInteractive(t)
	dialing(t, &recordingRemote{out: "main-abc\n"})
	dir := configWithNetwork(t, "    settings:\n      acme-net.login_server: https://net.example.com\n")

	if _, err := execute(t, "--config", dir, "login", "acme-net"); err != nil {
		t.Fatal(err)
	}
	line := strings.Join(*launched, " ")
	if !strings.Contains(line, `DEVMACHINE_SETTINGS='{"login_server":"https://net.example.com"}'`) {
		t.Fatalf("the settings did not reach join: %q", line)
	}
}

func TestLoginToANetworkAddsItsNameAboveTheOtherHosts(t *testing.T) {
	captureInteractive(t)
	client := &recordingRemote{out: "main-abc\n"}
	dialing(t, client)
	dir := configWithNetwork(t, "")

	out, err := execute(t, "--config", dir, "login", "acme-net")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "acme:main-abc") {
		t.Fatalf("did not say what it added: %q", out)
	}
	if got := strings.Join(hostsOf(t, dir), ","); got != "acme:main-abc,203.0.113.10" {
		t.Fatalf("hosts = %s", got)
	}
	if len(client.ran) != 1 || !strings.Contains(client.ran[0], "/opt/devmachine/roles.local/acme-net/bin/self-name") {
		t.Fatalf("self_name was not asked: %#v", client.ran)
	}
	body, err := os.ReadFile(filepath.Join(dir, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "    hosts:\n      - acme:main-abc\n      - 203.0.113.10\n") {
		t.Fatalf("hosts was not written as a block list:\n%s", body)
	}
}

func TestLoginToANetworkLeavesAnEntryAlreadyThere(t *testing.T) {
	captureInteractive(t)
	dialing(t, &recordingRemote{out: "main-abc\n"})
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [acme:main-abc, 203.0.113.10]\n    packages: [acme-net]\n")
	localNetworkPackage(t, dir, "acme-net", "acme", "exit 3\n")
	pinForLogin(t, dir)

	out, err := execute(t, "--config", dir, "login", "acme-net")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "already in") {
		t.Fatalf("did not say it was already there: %q", out)
	}
	if got := strings.Join(hostsOf(t, dir), ","); got != "acme:main-abc,203.0.113.10" {
		t.Fatalf("hosts = %s", got)
	}
}

func TestLoginToANetworkWithoutANameSaysHowToAddItByHand(t *testing.T) {
	captureInteractive(t)
	dialing(t, &recordingRemote{err: errors.New("exit status 1")})
	dir := configWithNetwork(t, "")

	out, err := execute(t, "--config", dir, "login", "acme-net")
	if err != nil {
		t.Fatal(err)
	}
	want := "    machines:\n      - name: main\n        hosts:\n          - acme:<name>\n          - 203.0.113.10\n"
	if !strings.Contains(out, want) {
		t.Fatalf("did not print the YAML to paste, want:\n%s\ngot:\n%s", want, out)
	}
	if got := strings.Join(hostsOf(t, dir), ","); got != "203.0.113.10" {
		t.Fatalf("hosts = %s, want nothing added", got)
	}
}

func TestLoginToANetworkRefusesANameThatCannotBeAHostEntry(t *testing.T) {
	captureInteractive(t)
	dialing(t, &recordingRemote{out: "main abc; rm -rf /\n"})
	dir := configWithNetwork(t, "")

	out, err := execute(t, "--config", dir, "login", "acme-net")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "acme:<name>") {
		t.Fatalf("did not fall back to the by-hand instructions: %q", out)
	}
	if got := strings.Join(hostsOf(t, dir), ","); got != "203.0.113.10" {
		t.Fatalf("hosts = %s", got)
	}
}

func TestLoginToANetworkBelongsToTheMachine(t *testing.T) {
	captureInteractive(t)
	dialing(t, &recordingRemote{})
	dir := configWithNetwork(t, "")

	_, err := execute(t, "--config", dir, "login", "acme-net", "--workspace", "alice")
	if err == nil || !strings.Contains(err.Error(), "no --workspace") {
		t.Fatalf("got %v", err)
	}
}

func TestLoginToANetworkThatFailsSaysToSyncFirst(t *testing.T) {
	captureInteractive(t)
	runInteractive = func(string, ...string) error { return errors.New("exit status 127") }
	dialing(t, &recordingRemote{})
	dir := configWithNetwork(t, "")

	_, err := execute(t, "--config", dir, "login", "acme-net")
	if err == nil || !strings.Contains(err.Error(), "devmachine sync") {
		t.Fatalf("got %v", err)
	}
}

func TestLoginToANetworkIsRecorded(t *testing.T) {
	captureInteractive(t)
	dialing(t, &recordingRemote{out: "main-abc\n"})
	dir := configWithNetwork(t, "")

	if _, err := execute(t, "--config", dir, "login", "acme-net"); err != nil {
		t.Fatal(err)
	}
	if line := historyLines(t, dir)[0]; !strings.Contains(line, "login acme-net") {
		t.Fatalf("the log line leaves the login out: %q", line)
	}
}

func TestLoginTailscaleUsesThePackagesJoinWhenItDeclaresANetwork(t *testing.T) {
	launched := captureInteractive(t)
	dialing(t, &recordingRemote{out: "main-abc\n"})
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [tailscale]\n")
	localNetworkPackage(t, dir, "tailscale", "tailscale", "exit 3\n")
	manifest := filepath.Join(packages.LocalDir(dir), "tailscale", "package.yml")
	body, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	withCredential := string(body) + "credentials:\n  - name: tailscale\n    kind: manual\n    scope: machine\n" +
		"    command: tailscale up\n    stored_at: /var/lib/tailscale/tailscaled.state\n"
	if err := os.WriteFile(manifest, []byte(withCredential), 0o644); err != nil {
		t.Fatal(err)
	}
	pinForLogin(t, dir)

	if _, err := execute(t, "--config", dir, "login", "tailscale"); err != nil {
		t.Fatal(err)
	}
	line := strings.Join(*launched, " ")
	if strings.HasSuffix(line, "tailscale up") || !strings.Contains(line, "/tailscale/bin/join") {
		t.Fatalf("the declared command ran instead of the package's join: %q", line)
	}
	if got := strings.Join(hostsOf(t, dir), ","); got != "tailscale:main-abc,203.0.113.10" {
		t.Fatalf("hosts = %s", got)
	}
}
