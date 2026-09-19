package remote

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// recordingClient keeps what was asked of the machine, so a test can read the
// shell the bootstrap writes instead of needing a machine to run it on.
type recordingClient struct {
	commands []string
	inputs   []string
	// failOn makes any command holding this text fail, which is how the
	// "sshd refused the config" branch is reached.
	failOn string
	output map[string]string
}

func (c *recordingClient) Run(_ context.Context, command string) (string, error) {
	c.commands = append(c.commands, command)
	if c.failOn != "" && strings.Contains(command, c.failOn) {
		return "", fmt.Errorf("running %q: exit status 255", command)
	}
	return c.output[command], nil
}

func (c *recordingClient) RunInput(ctx context.Context, command string, stdin io.Reader) (string, error) {
	body, err := io.ReadAll(stdin)
	if err != nil {
		return "", err
	}
	c.inputs = append(c.inputs, string(body))
	return c.Run(ctx, command)
}

func (c *recordingClient) Stream(ctx context.Context, command string, _, _ io.Writer) error {
	_, err := c.Run(ctx, command)
	return err
}

func (c *recordingClient) Upload(context.Context, string, io.Reader) error { return nil }
func (c *recordingClient) Close() error                                    { return nil }

func (c *recordingClient) transcript() string { return strings.Join(c.commands, "\n") }

// publicKeyLine returns an authorized_keys line for a key nothing else uses.
func publicKeyLine(t *testing.T, comment string) string {
	t.Helper()

	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer))) + " " + comment
}

func TestInstallKeyIsIdempotent(t *testing.T) {
	c := &recordingClient{}
	if err := InstallKey(context.Background(), c, publicKeyLine(t, "test")); err != nil {
		t.Fatal(err)
	}
	joined := c.transcript()
	// Running setup twice must not leave the key twice.
	if !strings.Contains(joined, "grep") {
		t.Fatalf("it appends without looking: %s", joined)
	}
	for _, want := range []string{"0700", "0600", ".ssh"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("it does not set %q: %s", want, joined)
		}
	}
}

func TestInstallKeyNeverPutsTheKeyInAnArgument(t *testing.T) {
	c := &recordingClient{}
	if err := InstallKey(context.Background(), c, "ssh-ed25519 SECRETLOOKING test"); err != nil {
		t.Fatal(err)
	}
	// A public key is not a secret, but the habit is: values go in on stdin.
	// An argument is in `ps` for every account on the machine to read.
	if strings.Contains(c.transcript(), "SECRETLOOKING") {
		t.Fatal("the key reached the command line")
	}
	if len(c.inputs) != 1 || !strings.Contains(c.inputs[0], "SECRETLOOKING") {
		t.Fatalf("the key did not go in on stdin: %#v", c.inputs)
	}
}

func TestInstallKeyRefusesAnEmptyKey(t *testing.T) {
	c := &recordingClient{}
	if err := InstallKey(context.Background(), c, "   \n"); err == nil {
		t.Fatal("an empty key was installed")
	}
	if len(c.commands) != 0 {
		t.Fatalf("it touched the machine anyway: %#v", c.commands)
	}
}

func TestProveKeyOpensANewConnection(t *testing.T) {
	// Reusing the session that installed the key proves nothing: that session
	// was authenticated by a password.
	server := sshServerAccepting(t, "publickey")
	m := machineAt(t, server.addr)

	if err := ProveKey(context.Background(), m, "root", throwawayKey(t)); err != nil {
		t.Fatal(err)
	}
	if server.connectionCount() != 1 {
		t.Fatalf("made %d connections", server.connectionCount())
	}
	if got := server.methodsOffered(); len(got) != 1 || got[0] != "publickey" {
		t.Fatalf("the proof did not use the key alone: %v", got)
	}
}

func TestProveKeySaysWhatToCheckWhenItFails(t *testing.T) {
	server := sshServerRejecting(t)
	m := machineAt(t, server.addr)

	err := ProveKey(context.Background(), m, "root", throwawayKey(t))
	if err == nil {
		t.Fatal("a refused key was reported as proved")
	}
	// This failure is the one that matters: hardening is about to be skipped
	// because of it, and the person needs to know why.
	for _, want := range []string{"authorized_keys", "AuthorizedKeysFile"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the error does not say where to look: %v", err)
		}
	}
	if !errors.Is(err, ErrAuthRefused) {
		t.Fatalf("a refused key is not reported as a refused login: %v", err)
	}
}

// TestInstallKeyAgainstTheThrowawayMachine runs the shell on a real sshd host.
// A script that is idempotent in a string comparison and not on a machine is
// the failure this whole task exists to catch.
func TestInstallKeyAgainstTheThrowawayMachine(t *testing.T) {
	m := testMachine(t)

	client, _, err := Dial(context.Background(), m, "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	key := publicKeyLine(t, "devmachine-install-key-test")
	defer client.Run(context.Background(),
		"sed -i '/devmachine-install-key-test/d' \"$HOME/.ssh/authorized_keys\"")

	for range 2 {
		if err := InstallKey(context.Background(), client, key); err != nil {
			t.Fatal(err)
		}
	}

	out, err := client.Run(context.Background(),
		"grep -c devmachine-install-key-test \"$HOME/.ssh/authorized_keys\"")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "1" {
		t.Fatalf("the key is in authorized_keys %s times", strings.TrimSpace(out))
	}

	modes, err := client.Run(context.Background(),
		"stat -c %a \"$HOME/.ssh\" \"$HOME/.ssh/authorized_keys\"")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Fields(modes)[0] != "700" || strings.Fields(modes)[1] != "600" {
		t.Fatalf("got modes %q", strings.TrimSpace(modes))
	}
}

func TestProveKeyAgainstTheThrowawayMachine(t *testing.T) {
	m := testMachine(t)

	if err := ProveKey(context.Background(), m, m.User, m.Key); err != nil {
		t.Fatal(err)
	}

	absent := filepath.Join(t.TempDir(), "id_ed25519")
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(private, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absent, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ProveKey(context.Background(), m, m.User, absent); err == nil {
		t.Fatal("a key the machine does not trust was reported as proved")
	}
}
