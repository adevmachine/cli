package keys

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// throwawayKey writes a private key nothing will ever authenticate with.
//
// A test about what an agent holds must never read the developer's own agent.
// That is how a test passes on one machine and fails on a runner, for a reason
// that has nothing to do with what it is checking.
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

// agentHolding runs a real SSH agent in this process, holding the keys it is
// given, and returns the socket to talk to it.
func agentHolding(t *testing.T, keyPath, comment string) string {
	t.Helper()

	body, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	private, err := ssh.ParseRawPrivateKey(body)
	if err != nil {
		t.Fatal(err)
	}

	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: private, Comment: comment}); err != nil {
		t.Fatal(err)
	}
	return serveAgent(t, keyring)
}

// serveAgent runs keyring behind a unix socket for the length of the test.
func serveAgent(t *testing.T, keyring agent.Agent) string {
	t.Helper()

	listener, sock := shortSocket(t)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = agent.ServeAgent(keyring, conn)
			}()
		}
	}()

	return sock
}

// shortSocket listens on a path a unix socket can hold. t.TempDir() builds its
// path out of the test's name, which here goes past the short hard limit.
func shortSocket(t *testing.T) (net.Listener, string) {
	t.Helper()

	dir, err := os.MkdirTemp("", "ag")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	sock := filepath.Join(dir, "s")
	listener, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })

	return listener, sock
}

func TestFromAgentWithNoAgentIsNotAnError(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	got, err := FromAgent()
	if err != nil {
		t.Fatalf("no agent is ordinary, not a failure: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %#v", got)
	}
}

func TestFromAgentListsFingerprintsAndComments(t *testing.T) {
	sock := agentHolding(t, throwawayKey(t), "a test key")
	t.Setenv("SSH_AUTH_SOCK", sock)

	got, err := FromAgent()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %#v", got)
	}
	// A person picks a key by recognising it, and a fingerprint plus the
	// comment is what they recognise.
	if !strings.HasPrefix(got[0].Fingerprint, "SHA256:") || got[0].Comment != "a test key" {
		t.Fatalf("got %#v", got[0])
	}
}

func TestFromAgentKeepsTheLineAKeyIsInstalledWith(t *testing.T) {
	path := throwawayKey(t)
	sock := agentHolding(t, path, "a test key")
	t.Setenv("SSH_AUTH_SOCK", sock)

	got, err := FromAgent()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %#v", got)
	}

	// A fingerprint identifies a key but cannot be installed. What goes into
	// authorized_keys is the line, so the caller needs it as well.
	body, _ := os.ReadFile(path)
	signer, err := ssh.ParsePrivateKey(body)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	if !strings.HasPrefix(got[0].PublicKey, want) {
		t.Fatalf("got %q, want the line for %q", got[0].PublicKey, want)
	}
}

func TestFromAgentWithAnUnreachableSocketIsNotAnError(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(t.TempDir(), "nothing"))
	got, err := FromAgent()
	if err != nil {
		t.Fatalf("a stale socket must not stop setup: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %#v", got)
	}
}

func TestFromAgentWithAnAgentHoldingNothingIsNotAnError(t *testing.T) {
	// An agent that is running but empty is as ordinary as no agent at all.
	t.Setenv("SSH_AUTH_SOCK", serveAgent(t, agent.NewKeyring()))

	got, err := FromAgent()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %#v", got)
	}
}

func TestFromAgentSaysSoWhenTheAgentAnswersBadly(t *testing.T) {
	// Something is listening and is not an agent. That is not the ordinary
	// "no agent" case, and swallowing it would leave a person wondering why
	// their key is not offered.
	listener, sock := shortSocket(t)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		conn.Close()
	}()

	t.Setenv("SSH_AUTH_SOCK", sock)
	if _, err := FromAgent(); err == nil {
		t.Fatal("an agent that cannot be talked to was reported as empty")
	} else if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the wrong failure was reported: %v", err)
	}
}
