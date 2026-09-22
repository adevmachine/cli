package commands

import (
	"errors"
	"net"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func configWithWorkspace(t *testing.T) string {
	t.Helper()
	return configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    port: 2222\n"+
		"    key: /keys/id_ed25519\nworkspaces:\n  - name: alice\n")
}

func TestTunnelExecsSshWithALocalForward(t *testing.T) {
	got := captureInteractive(t)
	dir := configWithWorkspace(t)
	port := strconv.Itoa(freePort(t))

	if _, err := execute(t, "--config", dir, "tunnel", "alice", port); err != nil {
		t.Fatalf("tunnel returned %v", err)
	}

	joined := strings.Join(*got, " ")
	if !strings.Contains(joined, "-L "+port+":127.0.0.1:"+port) {
		t.Fatalf("got %#v", *got)
	}
	// -N: no remote command. The point is the forward, not a shell.
	if !slices.Contains(*got, "-N") {
		t.Fatalf("it asks for a shell as well: %#v", *got)
	}
}

func TestTunnelUsesTheLocalPortWhenGiven(t *testing.T) {
	got := captureInteractive(t)
	dir := configWithWorkspace(t)
	remote := strconv.Itoa(freePort(t))
	local := strconv.Itoa(freePort(t))

	if _, err := execute(t, "--config", dir, "tunnel", "alice", remote, "--local", local); err != nil {
		t.Fatalf("tunnel returned %v", err)
	}
	if !strings.Contains(strings.Join(*got, " "), "-L "+local+":127.0.0.1:"+remote) {
		t.Fatalf("got %#v", *got)
	}
}

func TestTunnelSaysWhichPortIsBusyAndOffersAFreeOne(t *testing.T) {
	// Somebody running something else locally cannot bind that port again,
	// and a bind error nobody reads is the worst way to find out.
	captureInteractive(t)
	dir := configWithWorkspace(t)

	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	busyPort := busy.Addr().(*net.TCPAddr).Port

	_, err = execute(t, "--config", dir, "tunnel", "alice", strconv.Itoa(freePort(t)), "--local", strconv.Itoa(busyPort))
	if err == nil {
		t.Fatal("it tried to bind a port already in use")
	}
	if !strings.Contains(err.Error(), "--local") {
		t.Fatalf("the error does not offer the way out: %v", err)
	}
}

func TestTunnelSaysWhereItIsAndHowToClose(t *testing.T) {
	captureInteractive(t)
	dir := configWithWorkspace(t)
	port := strconv.Itoa(freePort(t))

	out, err := execute(t, "--config", dir, "tunnel", "alice", port)
	if err != nil {
		t.Fatalf("tunnel returned %v", err)
	}
	for _, want := range []string{"localhost:" + port, "Ctrl-C"} {
		if !strings.Contains(out, want) {
			t.Fatalf("got %q", out)
		}
	}
}

func TestTunnelSaysWhenSshIsMissing(t *testing.T) {
	runInteractive = func(string, ...string) error { return nil }
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { runInteractive = realExecCommand; lookPath = realLookPath })
	dir := configWithWorkspace(t)

	_, err := execute(t, "--config", dir, "tunnel", "alice", strconv.Itoa(freePort(t)))
	if err == nil {
		t.Fatal("expected an error when ssh is not installed")
	}
	if !strings.Contains(err.Error(), "ssh") {
		t.Fatalf("the error does not name what is missing: %v", err)
	}
}

func TestTunnelTrustsTheMachineTheSameWayEveryOtherCommandDoes(t *testing.T) {
	// `tunnel` execs the system ssh, which reads the operator's own
	// known_hosts. A machine the CLI built minutes ago is in nobody's, so
	// `tunnel` failed on every machine the CLI creates. `login` had exactly
	// this defect; the Go client pins no host key either. Whether the CLI
	// pins host keys is one decision for the whole CLI, and until it is
	// taken no command may answer it differently from the rest.
	got := captureInteractive(t)
	dir := configWithWorkspace(t)
	port := strconv.Itoa(freePort(t))

	if _, err := execute(t, "--config", dir, "tunnel", "alice", port); err != nil {
		t.Fatalf("tunnel returned %v", err)
	}

	joined := strings.Join(*got, " ")
	for _, want := range []string{"StrictHostKeyChecking=no", "UserKnownHostsFile=/dev/null"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("tunnel cannot reach a machine the CLI just made: %#v", *got)
		}
	}
}
