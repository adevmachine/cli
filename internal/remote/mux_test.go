package remote

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mydevmachine/devmachine/internal/config"
)

func muxTestMachine(t *testing.T) config.Machine {
	t.Helper()
	m := config.Machine{
		Name:           "main",
		Hosts:          []config.Host{{Address: "203.0.113.10"}},
		Port:           2222,
		Key:            throwawayKey(t),
		KnownHostsFile: filepath.Join(t.TempDir(), "known_hosts"),
	}
	trustMachine(t, m, newHostKey(t).PublicKey())
	return m
}

func fakeCacheDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := userCacheDir
	userCacheDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userCacheDir = orig })
	return dir
}

func TestDialMuxBuildsTheSameHostKeyPinningAsTheSystemSSHBuilder(t *testing.T) {
	cache := fakeCacheDir(t)
	m := muxTestMachine(t)

	client, address, err := DialMux(context.Background(), m, "")
	if err != nil {
		t.Fatalf("DialMux returned %v", err)
	}
	if address != m.Hosts[0].Address {
		t.Fatalf("address = %q, want %q", address, m.Hosts[0].Address)
	}
	mux, ok := client.(*muxClient)
	if !ok {
		t.Fatalf("got %T, want *muxClient", client)
	}

	want := StrictSSHArgs(m)
	joined := strings.Join(mux.args, " ")
	for _, arg := range want {
		if !strings.Contains(joined, arg) {
			t.Fatalf("missing pinning option %q in %#v", arg, mux.args)
		}
	}

	controlDir := filepath.Join(cache, "devmachine", "cm")
	for _, want := range []string{
		"ControlMaster=auto",
		"ControlPath=" + filepath.Join(controlDir, "%C"),
		"ControlPersist=5m",
		"BatchMode=yes",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing multiplexing option %q in %#v", want, mux.args)
		}
	}
}

func TestDialMuxCreatesTheControlPathDirectoryMode0700(t *testing.T) {
	cache := fakeCacheDir(t)
	m := muxTestMachine(t)

	if _, _, err := DialMux(context.Background(), m, ""); err != nil {
		t.Fatalf("DialMux returned %v", err)
	}

	info, err := os.Stat(filepath.Join(cache, "devmachine", "cm"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("the control path is not a directory")
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("mode = %o, want 0700", perm)
	}
}

func TestDialMuxFallsBackToDialWhenSSHIsMissing(t *testing.T) {
	fakeCacheDir(t)
	m := muxTestMachine(t)

	origLookPath := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { lookPath = origLookPath })

	origFallback := dialFallback
	called := false
	dialFallback = func(_ context.Context, gotMachine config.Machine, gotUser string) (Client, string, error) {
		called = true
		if gotMachine.Name != m.Name || gotUser != "alice" {
			t.Fatalf("fallback got (%q, %q)", gotMachine.Name, gotUser)
		}
		return &localClient{}, "fell back", nil
	}
	t.Cleanup(func() { dialFallback = origFallback })

	client, address, err := DialMux(context.Background(), m, "alice")
	if err != nil {
		t.Fatalf("DialMux returned %v", err)
	}
	if !called {
		t.Fatal("DialMux did not fall back to Dial when ssh is missing")
	}
	if address != "fell back" {
		t.Fatalf("address = %q", address)
	}
	if _, ok := client.(*localClient); !ok {
		t.Fatalf("got %T, want the fallback's client", client)
	}
}

func TestDialMuxKeepsUsingTheLocalClientForASelfMachine(t *testing.T) {
	fakeCacheDir(t)
	m := config.Machine{Name: "mac", Self: true}

	client, address, err := DialMux(context.Background(), m, "")
	if err != nil {
		t.Fatalf("DialMux returned %v", err)
	}
	if address != SelfAddress {
		t.Fatalf("address = %q, want %q", address, SelfAddress)
	}
	if _, ok := client.(*localClient); !ok {
		t.Fatalf("got %T, want *localClient", client)
	}
}

// fakeUnreachableSSH is a script that always exits 255: OpenSSH's own code
// for "never reached the machine", which is what tells muxClient.exec to try
// the next address instead of reporting the remote command's own failure.
func fakeUnreachableSSH(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 255\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMuxClientRunReturnsAnErrorTheSameShapeAsTheProgrammaticClient(t *testing.T) {
	fakeCacheDir(t)
	m := muxTestMachine(t)
	fakeSSH := fakeUnreachableSSH(t)

	origLookPath := lookPath
	lookPath = func(name string) (string, error) {
		if name == "ssh" {
			return fakeSSH, nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { lookPath = origLookPath })

	client, _, err := DialMux(context.Background(), m, "")
	if err != nil {
		t.Fatalf("DialMux returned %v", err)
	}

	_, err = client.Run(context.Background(), "true")
	if err == nil {
		t.Fatal("expected an error: no address can ever answer in this test")
	}
	if !strings.Contains(err.Error(), m.Name) {
		t.Fatalf("error does not name the machine: %v", err)
	}
}

// TestDialMuxReusesTheControlSocket runs `run` twice against the disposable
// Lima VM (see scripts/fake-vps.sh) and proves the second call finds the
// first one's control socket still alive, which is the whole point of
// DialMux: `ssh -O check` against the very same ControlPath only succeeds
// when a master connection from an earlier process is still there.
func TestDialMuxReusesTheControlSocket(t *testing.T) {
	// The real cache directory, not a t.TempDir(): macOS temp paths run long
	// enough on their own to blow the ~104-byte Unix socket limit once %C's
	// hash is appended, and that is exactly the failure DialMux exists to
	// avoid in production.
	m := testMachine(t)

	client, _, err := DialMux(context.Background(), m, "")
	if err != nil {
		t.Fatalf("DialMux returned %v", err)
	}
	if _, err := client.Run(context.Background(), "true"); err != nil {
		t.Fatalf("the first run returned %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close returned %v", err)
	}

	client2, _, err := DialMux(context.Background(), m, "")
	if err != nil {
		t.Fatalf("DialMux returned %v", err)
	}
	if _, err := client2.Run(context.Background(), "true"); err != nil {
		t.Fatalf("the second run returned %v", err)
	}
	t.Cleanup(func() { _ = client2.Close() })

	controlDir, err := controlPathDir()
	if err != nil {
		t.Fatal(err)
	}
	user := m.User
	args := append(StrictSSHArgs(m),
		"-o", "ControlPath="+filepath.Join(controlDir, "%C"),
		"-O", "check", user+"@"+m.Hosts[0].Address)
	check := exec.CommandContext(context.Background(), "ssh", args...)
	out, err := check.CombinedOutput()
	if err != nil {
		t.Fatalf("ssh -O check found no live master: %v: %s", err, out)
	}
}

func TestMuxClientUploadReturnsAClearError(t *testing.T) {
	fakeCacheDir(t)
	m := muxTestMachine(t)

	client, _, err := DialMux(context.Background(), m, "")
	if err != nil {
		t.Fatalf("DialMux returned %v", err)
	}

	if err := client.Upload(context.Background(), "/tmp/x", strings.NewReader("")); err == nil {
		t.Fatal("expected Upload to refuse")
	}
}

func TestMuxClientStopsAtARefusedHostKeyAndSaysSo(t *testing.T) {
	fakeCacheDir(t)
	m := muxTestMachine(t)
	m.Hosts = append(m.Hosts, config.Host{Address: "203.0.113.11"})
	calls := filepath.Join(t.TempDir(), "calls")
	fakeSSH := filepath.Join(t.TempDir(), "ssh")
	script := "#!/bin/sh\necho x >> " + calls + "\necho 'Host key verification failed.' >&2\nexit 255\n"
	if err := os.WriteFile(fakeSSH, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	origLookPath := lookPath
	lookPath = func(string) (string, error) { return fakeSSH, nil }
	t.Cleanup(func() { lookPath = origLookPath })

	client, _, err := DialMux(context.Background(), m, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Run(context.Background(), "true")
	if !errors.Is(err, ErrHostKeyRejected) {
		t.Fatalf("got %v, want ErrHostKeyRejected", err)
	}
	body, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(body), "x"); n != 1 {
		t.Fatalf("ssh ran %d times, want 1: a refused key must not fall through to the next address", n)
	}
}
