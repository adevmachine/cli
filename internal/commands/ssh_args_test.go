package commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/hostkeys"
	"github.com/adevmachine/cli/internal/remote"
	"golang.org/x/crypto/ssh"
)

func pinnedMachine(t *testing.T, port int, spaced bool) (config.Machine, ssh.PublicKey) {
	t.Helper()
	dir := t.TempDir()
	if spaced {
		dir = filepath.Join(dir, "config with spaces")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	m := config.Machine{
		Name: "main", Hosts: []config.Host{{Address: "203.0.113.10"}}, Port: port,
		Key: "/keys/id with spaces", KnownHostsFile: filepath.Join(dir, "known hosts"),
	}
	key := commandHostKey(t)
	store, err := hostkeys.Open(m.KnownHostsFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(m.Name, m.Port, key); err != nil {
		t.Fatal(err)
	}
	return m, key
}

func TestStrictSSHArgsPinEveryIdentitySource(t *testing.T) {
	for _, port := range []int{22, 2222} {
		m, _ := pinnedMachine(t, port, true)
		args := strictSSHArgs(m)
		joined := strings.Join(args, " ")
		for _, want := range []string{
			"StrictHostKeyChecking=yes",
			"UserKnownHostsFile=" + m.KnownHostsFile,
			"GlobalKnownHostsFile=/dev/null",
			"HostKeyAlias=" + hostkeys.Lookup(m.Name, m.Port),
			"UpdateHostKeys=no", "CheckHostIP=no", "VerifyHostKeyDNS=no",
			"KnownHostsCommand=none", "HostKeyAlgorithms=ssh-ed25519",
		} {
			if !strings.Contains(joined, want) {
				t.Fatalf("port %d: missing %q in %#v", port, want, args)
			}
		}
		for _, forbidden := range []string{"StrictHostKeyChecking=no", "accept-new", "UserKnownHostsFile=/dev/null"} {
			if strings.Contains(joined, forbidden) {
				t.Fatalf("port %d: unsafe %q in %#v", port, forbidden, args)
			}
		}
		identity := slices.Index(args, "-i")
		lastStrict := slices.Index(args, "HostKeyAlgorithms=ssh-ed25519")
		if identity < lastStrict {
			t.Fatalf("identity option precedes strict options: %#v", args)
		}
	}
}

func TestStrictSSHCommandQuotesNestedMoshArguments(t *testing.T) {
	m, _ := pinnedMachine(t, 2222, true)
	got := strictSSHCommand(m)
	for _, want := range []string{
		"'UserKnownHostsFile=" + m.KnownHostsFile + "'",
		"'-i' '/keys/id with spaces'",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestSystemSSHPreflightReturnsChangedKeyAndNeverExecutes(t *testing.T) {
	m, _ := pinnedMachine(t, 22, false)
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n")
	m.KnownHostsFile = filepath.Join(dir, config.KnownHostsFileName)
	store, err := hostkeys.Open(m.KnownHostsFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(m.Name, m.Port, commandHostKey(t)); err != nil {
		t.Fatal(err)
	}

	presented := commandHostKey(t)
	wasScan := scanHostKey
	scanHostKey = func(context.Context, config.Machine) (ssh.PublicKey, string, error) {
		return presented, "203.0.113.10", nil
	}
	launched := false
	runInteractive = func(string, ...string) error { launched = true; return nil }
	lookPath = func(string) (string, error) { return "/usr/bin/ssh", nil }
	verifySystemHost = realVerifySystemHost
	t.Cleanup(func() {
		scanHostKey = wasScan
		runInteractive = realExecCommand
		lookPath = realLookPath
		verifySystemHost = realVerifySystemHost
	})

	_, err = execute(t, "--config", dir, "ssh")
	if !errors.Is(err, remote.ErrHostKeyChanged) {
		t.Fatalf("got %v, want ErrHostKeyChanged", err)
	}
	if launched {
		t.Fatal("system ssh ran after a host-key mismatch")
	}
}
