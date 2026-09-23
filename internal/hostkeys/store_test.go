package hostkeys

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestLookupUsesTheMachineAliasAndPort(t *testing.T) {
	if got := Lookup("main", 22); got != "main-devmachine" {
		t.Fatalf("port 22 lookup = %q", got)
	}
	if got := Lookup("main", 2222); got != "[main-devmachine]:2222" {
		t.Fatalf("port 2222 lookup = %q", got)
	}
}

func TestAlgorithmsStayOnThePinnedPublicKey(t *testing.T) {
	ed := ed25519Key(t)
	ecdsaKey := ecdsaKey(t)
	rsaKey := rsaKey(t)

	tests := []struct {
		name string
		key  ssh.PublicKey
		want []string
	}{
		{name: "ed25519", key: ed, want: []string{ssh.KeyAlgoED25519}},
		{name: "ecdsa", key: ecdsaKey, want: []string{ecdsaKey.Type()}},
		{name: "rsa", key: rsaKey, want: []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Algorithms(tt.key); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Algorithms(%s) = %#v, want %#v", tt.key.Type(), got, tt.want)
			}
		})
	}
}

func TestPutReplacesOneMachineAndPreservesTheOther(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	mainOld := ed25519Key(t)
	mainNew := ed25519Key(t)
	other := ecdsaKey(t)
	if err := store.Put("main", 22, mainOld); err != nil {
		t.Fatal(err)
	}
	if err := store.Put("other", 2222, other); err != nil {
		t.Fatal(err)
	}
	if err := store.Put("main", 22, mainNew); err != nil {
		t.Fatal(err)
	}

	callback, err := knownhosts.New(path)
	if err != nil {
		t.Fatal(err)
	}
	remote := &net.TCPAddr{IP: net.ParseIP("203.0.113.10"), Port: 22}
	if err := callback(net.JoinHostPort(Alias("main"), "22"), remote, mainNew); err != nil {
		t.Fatalf("replacement key was not trusted: %v", err)
	}
	if err := callback(net.JoinHostPort(Alias("other"), "2222"), remote, other); err != nil {
		t.Fatalf("other machine entry was not preserved: %v", err)
	}
	if err := callback(net.JoinHostPort(Alias("main"), "22"), remote, mainOld); err == nil {
		t.Fatal("old key remained trusted after replacement")
	}
}

func TestCheckDistinguishesMatchingAndDifferentKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	key := ed25519Key(t)
	if err := store.Put("main", 22, key); err != nil {
		t.Fatal(err)
	}
	if err := store.Check("main", 22, key); err != nil {
		t.Fatalf("matching key failed: %v", err)
	}
	if err := store.Check("main", 22, ed25519Key(t)); err == nil {
		t.Fatal("different key unexpectedly matched")
	}
}

func TestKeyReturnsThePinnedKeyAndReportsAnUnknownMachine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	want := ed25519Key(t)
	if err := store.Put("main", 22, want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Key("main", 22)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Marshal(), want.Marshal()) {
		t.Fatal("Key returned a different public key")
	}
	if _, err := store.Key("other", 22); err == nil {
		t.Fatal("Key returned no error for an unknown machine")
	}
}

func TestOpenReportsMalformedInputWithFileContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(path, []byte("main-devmachine not-a-public-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Open(path)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("Open() = %v, want error naming %s", err, path)
	}
}

func TestPutUsesMode0600AndAtomicRename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("main", 22, ed25519Key(t)); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("main", 22, ed25519Key(t)); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %04o, want 0600", after.Mode().Perm())
	}
	if os.SameFile(before, after) {
		t.Fatal("known_hosts inode did not change; write was not an atomic rename")
	}
}

func TestFingerprintUsesTheOpenSSHRepresentation(t *testing.T) {
	key := ed25519Key(t)
	if got, want := Fingerprint(key), ssh.FingerprintSHA256(key); got != want {
		t.Fatalf("Fingerprint() = %q, want %q", got, want)
	}
}

func ed25519Key(t *testing.T) ssh.PublicKey {
	t.Helper()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func ecdsaKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(&private.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func rsaKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(&private.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return key
}
