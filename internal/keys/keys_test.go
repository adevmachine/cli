package keys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateWritesAPairOnlyTheOwnerCanRead(t *testing.T) {
	dir := t.TempDir()
	path, public, err := Generate(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("private key is %o", info.Mode().Perm())
	}
	if !strings.HasPrefix(public, "ssh-ed25519 ") {
		t.Fatalf("public key is %q", public)
	}
}

func TestGenerateWritesThePublicHalfBesideThePrivateOne(t *testing.T) {
	dir := t.TempDir()
	path, public, err := Generate(dir, "main")
	if err != nil {
		t.Fatal(err)
	}

	// Somebody pasting the key into a provider's web form looks for a file, not
	// for something the CLI printed once.
	body, err := os.ReadFile(path + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != public {
		t.Fatalf("the file holds %q, Generate returned %q", body, public)
	}
}

func TestGenerateRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := Generate(dir, "main"); err != nil {
		t.Fatal(err)
	}
	// Overwriting a key locks you out of every machine that trusts it, and
	// nothing brings it back.
	if _, _, err := Generate(dir, "main"); err == nil {
		t.Fatal("it overwrote a key")
	}
}

func TestGenerateRefusesANameThatEscapesTheDirectory(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"", "..", "../escape", "a/b"} {
		if _, _, err := Generate(dir, name); err == nil {
			t.Fatalf("name %q was accepted", name)
		}
	}
}

func TestGenerateCreatesTheDirectoryItWritesInto(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keys")
	if _, _, err := Generate(dir, "main"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("the directory is %o", info.Mode().Perm())
	}
}

func TestGenerateProducesAKeyDialCanRead(t *testing.T) {
	dir := t.TempDir()
	path, _, err := Generate(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ssh.ParsePrivateKey(body); err != nil {
		t.Fatalf("the CLI cannot read the key it wrote: %v", err)
	}
}

func TestGeneratePublicKeyMatchesThePrivateOne(t *testing.T) {
	dir := t.TempDir()
	path, public, err := Generate(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.ParsePrivateKey(body)
	if err != nil {
		t.Fatal(err)
	}

	// A public key that belongs to another private key installs cleanly and
	// then refuses every connection, with nothing to look at.
	marshalled := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	if !strings.HasPrefix(public, marshalled) {
		t.Fatalf("public key %q does not belong to %q", public, marshalled)
	}
}

func TestGenerateNamesTheKeyInItsComment(t *testing.T) {
	dir := t.TempDir()
	_, public, err := Generate(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	// authorized_keys on a shared machine ends up holding several keys, and the
	// comment is the only thing saying which one this is.
	if !strings.HasSuffix(public, " devmachine-main") {
		t.Fatalf("public key %q carries no comment", public)
	}
}

func TestDirIsInsideTheConfiguration(t *testing.T) {
	if got := Dir("/c"); got != filepath.Join("/c", "keys") {
		t.Fatalf("got %q", got)
	}
}
