package remote

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/hostkeys"
	"golang.org/x/crypto/ssh"
)

func TestScanHostKeyDoesNotAttemptAuthentication(t *testing.T) {
	server := startHostKeyServer(t, false)
	machine := server.machine(t)

	key, address, err := ScanHostKey(context.Background(), machine)
	if err != nil {
		t.Fatal(err)
	}
	if address != "127.0.0.1" {
		t.Fatalf("address = %q", address)
	}
	if !bytes.Equal(key.Marshal(), server.key.Marshal()) {
		t.Fatal("ScanHostKey returned a different key")
	}
	if got := server.authAttempts.Load(); got != 0 {
		t.Fatalf("scan attempted authentication %d times", got)
	}
}

func TestDialWithUnknownHostKeyFailsBeforeAuthentication(t *testing.T) {
	server := startHostKeyServer(t, false)
	machine := server.machine(t)

	_, _, err := DialWith(context.Background(), machine, "root", Auth{Password: "secret"})
	if !errors.Is(err, ErrHostKeyUnknown) {
		t.Fatalf("DialWith error = %v, want ErrHostKeyUnknown", err)
	}
	if got := server.authAttempts.Load(); got != 0 {
		t.Fatalf("unknown key attempted authentication %d times", got)
	}
}

func TestDialWithChangedHostKeyFailsBeforeAuthentication(t *testing.T) {
	server := startHostKeyServer(t, false)
	machine := server.machine(t)
	trustMachine(t, machine, newHostKey(t).PublicKey())

	_, _, err := DialWith(context.Background(), machine, "root", Auth{Password: "secret"})
	if !errors.Is(err, ErrHostKeyChanged) {
		t.Fatalf("DialWith error = %v, want ErrHostKeyChanged", err)
	}
	if got := server.authAttempts.Load(); got != 0 {
		t.Fatalf("changed key attempted authentication %d times", got)
	}
}

func TestDialWithMatchingHostKeyReachesAuthentication(t *testing.T) {
	server := startHostKeyServer(t, false)
	machine := server.machine(t)
	trustMachine(t, machine, server.key)

	_, _, err := DialWith(context.Background(), machine, "root", Auth{Password: "secret"})
	if !errors.Is(err, ErrAuthRefused) {
		t.Fatalf("DialWith error = %v, want ErrAuthRefused", err)
	}
	if got := server.authAttempts.Load(); got == 0 {
		t.Fatal("matching host key never reached authentication")
	}
}

func TestDialReportsMissingTrustBeforeMissingAgent(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	machine := config.Machine{
		Name:           "main",
		Hosts:          []config.Host{{Address: "203.0.113.10"}},
		User:           "root",
		Port:           22,
		KnownHostsFile: filepath.Join(t.TempDir(), "known_hosts"),
	}

	_, _, err := Dial(context.Background(), machine, "")
	if !errors.Is(err, ErrHostKeyUnknown) {
		t.Fatalf("Dial error = %v, want ErrHostKeyUnknown before agent error", err)
	}
}

func TestDialWithHostKeyMismatchDoesNotTryAFallbackAddress(t *testing.T) {
	machine := config.Machine{
		Name:           "main",
		Hosts:          []config.Host{{Address: "203.0.113.10"}, {Address: "203.0.113.11"}},
		User:           "root",
		Port:           22,
		KnownHostsFile: filepath.Join(t.TempDir(), "known_hosts"),
	}
	trustMachine(t, machine, newHostKey(t).PublicKey())
	presented := newHostKey(t).PublicKey()
	calls := 0
	dialSSH = func(_ context.Context, target string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
		calls++
		err := cfg.HostKeyCallback(target, &net.TCPAddr{IP: net.ParseIP("203.0.113.10"), Port: 22}, presented)
		return nil, err
	}
	t.Cleanup(func() { dialSSH = realDialContext })

	_, _, err := DialWith(context.Background(), machine, "root", Auth{Password: "secret"})
	if !errors.Is(err, ErrHostKeyChanged) {
		t.Fatalf("DialWith error = %v, want ErrHostKeyChanged", err)
	}
	if calls != 1 {
		t.Fatalf("identity mismatch tried %d addresses, want 1", calls)
	}
}

func TestDialWithNetworkFailureStillTriesAFallbackAddress(t *testing.T) {
	machine := config.Machine{
		Name:           "main",
		Hosts:          []config.Host{{Address: "203.0.113.10"}, {Address: "203.0.113.11"}},
		User:           "root",
		Port:           22,
		KnownHostsFile: filepath.Join(t.TempDir(), "known_hosts"),
	}
	trusted := newHostKey(t).PublicKey()
	trustMachine(t, machine, trusted)
	calls := 0
	dialSSH = func(_ context.Context, target string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("network unreachable")
		}
		if err := cfg.HostKeyCallback(target, &net.TCPAddr{IP: net.ParseIP("203.0.113.11"), Port: 22}, trusted); err != nil {
			return nil, err
		}
		return nil, nil
	}
	t.Cleanup(func() { dialSSH = realDialContext })

	_, address, err := DialWith(context.Background(), machine, "root", Auth{Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if address != "203.0.113.11" || calls != 2 {
		t.Fatalf("address = %q, calls = %d", address, calls)
	}
}

type hostKeyServer struct {
	listener     net.Listener
	key          ssh.PublicKey
	authAttempts atomic.Int32
}

func startHostKeyServer(t *testing.T, acceptPassword bool) *hostKeyServer {
	t.Helper()
	signer := newHostKey(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &hostKeyServer{listener: listener, key: signer.PublicKey()}
	config := &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, _ []byte) (*ssh.Permissions, error) {
			server.authAttempts.Add(1)
			if acceptPassword {
				return nil, nil
			}
			return nil, errors.New("password rejected")
		},
	}
	config.AddHostKey(signer)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(requests)
				for channel := range channels {
					channel.Reject(ssh.UnknownChannelType, "test server accepts no channels")
				}
			}(conn)
		}
	}()
	t.Cleanup(func() { listener.Close() })
	return server
}

func (s *hostKeyServer) machine(t *testing.T) config.Machine {
	t.Helper()
	host, rawPort, err := net.SplitHostPort(s.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatal(err)
	}
	return config.Machine{
		Name:           "main",
		Hosts:          []config.Host{{Address: host}},
		User:           "root",
		Port:           port,
		KnownHostsFile: filepath.Join(t.TempDir(), "known_hosts"),
	}
}

func newHostKey(t *testing.T) ssh.Signer {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func trustMachine(t *testing.T, machine config.Machine, key ssh.PublicKey) {
	t.Helper()
	store, err := hostkeys.Open(machine.KnownHostsFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(machine.Name, machine.Port, key); err != nil {
		t.Fatal(err)
	}
}
