package remote

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adevmachine/cli/internal/config"
	"golang.org/x/crypto/ssh"
)

// throwawayKey writes a private key nothing will ever authenticate with.
//
// A test about which addresses were tried must not depend on whether the
// machine running it happens to have an SSH agent. Without this it passes on a
// developer's laptop and fails on a runner, for a reason that has nothing to do
// with what it is checking.
func throwawayKey(t *testing.T) string {
	t.Helper()

	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(private, "")
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveKeepsALiteralAddress(t *testing.T) {
	m := config.Machine{Name: "main", Hosts: []config.Host{{Address: "203.0.113.10"}}}

	got, err := Resolve(m)
	if err != nil {
		t.Fatalf("Resolve returned %v", err)
	}
	if len(got) != 1 || got[0] != "203.0.113.10" {
		t.Fatalf("got %#v", got)
	}
}

func TestResolveKeepsTheConfiguredOrder(t *testing.T) {
	m := config.Machine{Name: "main", Hosts: []config.Host{
		{Address: "100.64.0.5"},
		{Address: "203.0.113.10"},
	}}

	got, _ := Resolve(m)
	if len(got) != 2 || got[0] != "100.64.0.5" || got[1] != "203.0.113.10" {
		t.Fatalf("the order was not preserved: %#v", got)
	}
}

func TestResolveDropsTailscaleWhenTheBinaryIsMissing(t *testing.T) {
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { lookPath = realLookPath })

	m := config.Machine{Name: "main", Hosts: []config.Host{
		{Address: "tailscale:vps"},
		{Address: "203.0.113.10"},
	}}

	got, err := Resolve(m)
	if err != nil {
		t.Fatalf("Resolve returned %v", err)
	}
	if len(got) != 1 || got[0] != "203.0.113.10" {
		t.Fatalf("the tailscale entry was not dropped: %#v", got)
	}
}

func TestResolveExpandsTailscaleWhenItIsAvailable(t *testing.T) {
	lookPath = func(string) (string, error) { return "/usr/bin/tailscale", nil }
	tailscaleIP = func(string) (string, error) { return "100.64.0.5", nil }
	t.Cleanup(func() { lookPath = realLookPath; tailscaleIP = realTailscaleIP })

	m := config.Machine{Name: "main", Hosts: []config.Host{{Address: "tailscale:vps"}}}

	got, _ := Resolve(m)
	if len(got) != 1 || got[0] != "100.64.0.5" {
		t.Fatalf("got %#v", got)
	}
}

func TestResolveDropsTailscaleWhenThePeerIsUnknown(t *testing.T) {
	lookPath = func(string) (string, error) { return "/usr/bin/tailscale", nil }
	tailscaleIP = func(string) (string, error) { return "", errors.New("no such peer") }
	t.Cleanup(func() { lookPath = realLookPath; tailscaleIP = realTailscaleIP })

	m := config.Machine{Name: "main", Hosts: []config.Host{
		{Address: "tailscale:vps"},
		{Address: "203.0.113.10"},
	}}

	got, _ := Resolve(m)
	if len(got) != 1 || got[0] != "203.0.113.10" {
		t.Fatalf("got %#v", got)
	}
}

func TestResolveFailsWhenEveryAddressWasDropped(t *testing.T) {
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { lookPath = realLookPath })

	m := config.Machine{Name: "main", Hosts: []config.Host{{Address: "tailscale:vps"}}}

	_, err := Resolve(m)
	if err == nil {
		t.Fatal("expected an error when nothing is left to try")
	}
	if !strings.Contains(err.Error(), "tailscale") {
		t.Fatalf("the error does not say what was dropped or why: %v", err)
	}
}

// testMachine describes the throwaway VPS from scripts/fake-vps.sh. Without it
// the integration tests skip, so `go test ./...` still works where there is no
// VM.
func testMachine(t *testing.T) config.Machine {
	t.Helper()

	host := os.Getenv("DEVMACHINE_TEST_HOST")
	port := os.Getenv("DEVMACHINE_TEST_PORT")
	key := os.Getenv("DEVMACHINE_TEST_KEY")
	if host == "" || port == "" || key == "" {
		t.Skip("no test VPS: run `eval \"$(scripts/fake-vps.sh env)\"` first")
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("DEVMACHINE_TEST_PORT is not a number: %v", err)
	}

	user := os.Getenv("DEVMACHINE_TEST_USER")
	if user == "" {
		user = "root"
	}
	return config.Machine{
		Name:  "sandbox",
		Hosts: []config.Host{{Address: host}},
		User:  user,
		Port:  n,
		Key:   key,
	}
}

func TestDialReachesTheTestMachine(t *testing.T) {
	m := testMachine(t)

	client, address, err := Dial(context.Background(), m, "")
	if err != nil {
		t.Fatalf("Dial returned %v", err)
	}
	defer client.Close()

	if address != m.Hosts[0].Address {
		t.Fatalf("connected through %q, want %q", address, m.Hosts[0].Address)
	}
}

func TestRunReturnsTheCommandOutput(t *testing.T) {
	m := testMachine(t)

	client, _, err := Dial(context.Background(), m, "")
	if err != nil {
		t.Fatalf("Dial returned %v", err)
	}
	defer client.Close()

	out, err := client.Run(context.Background(), "id -un")
	if err != nil {
		t.Fatalf("Run returned %v", err)
	}
	if strings.TrimSpace(out) != m.User {
		t.Fatalf("got %q, want %q", strings.TrimSpace(out), m.User)
	}
}

func TestRunReportsACommandThatFailed(t *testing.T) {
	m := testMachine(t)

	client, _, err := Dial(context.Background(), m, "")
	if err != nil {
		t.Fatalf("Dial returned %v", err)
	}
	defer client.Close()

	if _, err := client.Run(context.Background(), "exit 3"); err == nil {
		t.Fatal("expected an error from a command that exited non-zero")
	}
}

func TestDialFallsBackToTheNextAddress(t *testing.T) {
	m := testMachine(t)
	// An address in the documentation range never answers, so the fallback is
	// the only way this can succeed.
	m.Hosts = append([]config.Host{{Address: "203.0.113.1"}}, m.Hosts...)

	client, address, err := Dial(context.Background(), m, "")
	if err != nil {
		t.Fatalf("Dial returned %v", err)
	}
	defer client.Close()

	if address == "203.0.113.1" {
		t.Fatal("Dial claimed to reach an address that cannot answer")
	}
}

func TestDialSaysEveryAddressFailed(t *testing.T) {
	m := config.Machine{
		Name:  "main",
		Hosts: []config.Host{{Address: "203.0.113.1"}, {Address: "203.0.113.2"}},
		User:  "root",
		Port:  22,
		Key:   throwawayKey(t),
	}

	_, _, err := Dial(context.Background(), m, "")
	if err == nil {
		t.Fatal("expected an error when no address answers")
	}
	for _, want := range []string{"203.0.113.1", "203.0.113.2"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the error does not name %q: %v", want, err)
		}
	}
}

func TestDialLogsInAsTheUserItIsGiven(t *testing.T) {
	m := testMachine(t)

	// An account that does not exist cannot authenticate. Failing here is what
	// proves the override reached the server instead of being ignored in
	// favour of the machine's admin login, which would have succeeded.
	if _, _, err := Dial(context.Background(), m, "nobody-here"); err == nil {
		t.Fatal("Dial succeeded as a user that does not exist on the machine")
	}
}

func TestDialFallsBackToTheAdminUserWhenGivenNone(t *testing.T) {
	m := testMachine(t)

	client, _, err := Dial(context.Background(), m, "")
	if err != nil {
		t.Fatalf("Dial returned %v", err)
	}
	defer client.Close()

	out, err := client.Run(context.Background(), "id -un")
	if err != nil {
		t.Fatalf("Run returned %v", err)
	}
	if strings.TrimSpace(out) != m.User {
		t.Fatalf("logged in as %q, want the machine's admin user %q", strings.TrimSpace(out), m.User)
	}
}

func TestDialTellsARefusedLoginApartFromAnUnreachableMachine(t *testing.T) {
	m := testMachine(t)

	_, _, err := Dial(context.Background(), m, "nobody-here")
	if err == nil {
		t.Fatal("Dial succeeded as a user that does not exist")
	}
	// The machine answered; only the login failed. Saying "no address
	// answered" would send someone to check the network for nothing.
	if strings.Contains(err.Error(), "no address answered") {
		t.Fatalf("a refused login was reported as an unreachable machine: %v", err)
	}
	for _, want := range []string{"refused the login", "nobody-here"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the error does not say %q: %v", want, err)
		}
	}
}

func TestDialSaysWhatToDoWithNoKeyAndNoAgent(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")

	m := config.Machine{
		Name:  "main",
		Hosts: []config.Host{{Address: "203.0.113.1"}},
		User:  "root",
		Port:  22,
	}

	_, _, err := Dial(context.Background(), m, "")
	if err == nil {
		t.Fatal("expected an error with nothing to authenticate with")
	}
	for _, want := range []string{"key", "agent"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the error does not say what is missing (%q): %v", want, err)
		}
	}
}

func TestDialRejectsAKeyItCannotRead(t *testing.T) {
	m := config.Machine{
		Name:  "main",
		Hosts: []config.Host{{Address: "203.0.113.1"}},
		User:  "root",
		Port:  22,
		Key:   filepath.Join(t.TempDir(), "absent"),
	}

	_, _, err := Dial(context.Background(), m, "")
	if err == nil {
		t.Fatal("expected an error for a key file that does not exist")
	}
	if !strings.Contains(err.Error(), "absent") {
		t.Fatalf("the error does not name the file: %v", err)
	}
}

// tarballWith builds a gzipped tar in memory. The remote package cannot use
// provision.Tar for this: provision reaches for a client, so importing it back
// would be a cycle.
func tarballWith(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	zipped := gzip.NewWriter(&buf)
	archive := tar.NewWriter(zipped)
	for name, body := range files {
		header := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if err := archive.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zipped.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestUploadAndStreamAgainstTheThrowawayMachine(t *testing.T) {
	m := testMachine(t)

	client, _, err := Dial(context.Background(), m, "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	dir := "/tmp/devmachine-test-" + strconv.Itoa(os.Getpid())
	defer client.Run(context.Background(), "rm -rf "+dir)

	tarball := tarballWith(t, map[string]string{"hello.txt": "hi\n"})
	if err := client.Upload(context.Background(), dir, bytes.NewReader(tarball)); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := client.Stream(context.Background(), "cat "+dir+"/hello.txt", &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "hi" {
		t.Fatalf("got %q", out.String())
	}

	if err := client.Stream(context.Background(), "exit 3", io.Discard, io.Discard); err == nil {
		t.Fatal("a non-zero exit was reported as success")
	}
}

// TestStreamWritesWhileTheCommandIsStillRunning is the whole reason Stream
// exists: output collected and printed at the end makes a long run look stuck.
func TestStreamWritesWhileTheCommandIsStillRunning(t *testing.T) {
	m := testMachine(t)

	client, _, err := Dial(context.Background(), m, "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	first := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- client.Stream(context.Background(), "echo one; sleep 5; echo two", &firstWrite{ch: first}, io.Discard)
	}()

	select {
	case <-first:
	case err := <-done:
		t.Fatalf("the command finished before anything was written: %v", err)
	case <-time.After(4 * time.Second):
		t.Fatal("nothing was written in the first four seconds of a five second command")
	}

	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// firstWrite closes its channel the first time anything is written to it.
type firstWrite struct {
	ch   chan struct{}
	once sync.Once
}

func (w *firstWrite) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.ch) })
	return len(p), nil
}

func TestShellQuoteSurvivesAQuoteInThePath(t *testing.T) {
	if got := shellQuote("/opt/dev'machine"); got != `'/opt/dev'\''machine'` {
		t.Fatalf("got %s", got)
	}
}
