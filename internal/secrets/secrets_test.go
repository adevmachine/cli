package secrets

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// withoutKeyring makes every keyring call fail, which is how the file fallback
// gets exercised on a machine that has a working keychain.
func withoutKeyring(t *testing.T) {
	t.Helper()
	fail := errors.New("no keyring here")
	keyringSet = func(string, string, string) error { return fail }
	keyringGet = func(string, string) (string, error) { return "", fail }
	keyringDelete = func(string, string) error { return fail }
	t.Cleanup(func() {
		keyringSet, keyringGet, keyringDelete = realKeyringSet, realKeyringGet, realKeyringDelete
	})
}

func TestSetThenGetReturnsTheValue(t *testing.T) {
	withoutKeyring(t)
	dir := t.TempDir()

	if err := Set(dir, "cloudflare_token", "s3cret"); err != nil {
		t.Fatalf("Set returned %v", err)
	}
	got, err := Get(dir, "cloudflare_token")
	if err != nil {
		t.Fatalf("Get returned %v", err)
	}
	if got != "s3cret" {
		t.Fatalf("got %q", got)
	}
}

func TestSetOverwritesAValueThatWasAlreadyThere(t *testing.T) {
	withoutKeyring(t)
	dir := t.TempDir()

	Set(dir, "token", "first")
	if err := Set(dir, "token", "second"); err != nil {
		t.Fatalf("Set returned %v", err)
	}
	if got, _ := Get(dir, "token"); got != "second" {
		t.Fatalf("got %q, want the new value", got)
	}
}

func TestGetSaysWhichSecretIsMissing(t *testing.T) {
	withoutKeyring(t)

	_, err := Get(t.TempDir(), "absent")
	if err == nil {
		t.Fatal("expected an error for a secret that was never set")
	}
	if !strings.Contains(err.Error(), "absent") {
		t.Fatalf("the error does not name the secret: %v", err)
	}
}

func TestListReturnsNamesInOrder(t *testing.T) {
	withoutKeyring(t)
	dir := t.TempDir()

	Set(dir, "zeta", "v")
	Set(dir, "alpha", "v")

	got, err := List(dir)
	if err != nil {
		t.Fatalf("List returned %v", err)
	}
	if !slices.Equal(got, []string{"alpha", "zeta"}) {
		t.Fatalf("got %#v, want them sorted", got)
	}
}

func TestListNeverReturnsAValue(t *testing.T) {
	withoutKeyring(t)
	dir := t.TempDir()

	Set(dir, "token", "s3cret")

	names, _ := List(dir)
	for _, n := range names {
		if strings.Contains(n, "s3cret") {
			t.Fatal("List leaked a secret's value")
		}
	}
}

func TestListOnAFreshDirectoryIsEmptyAndNotAnError(t *testing.T) {
	withoutKeyring(t)

	got, err := List(t.TempDir())
	if err != nil {
		t.Fatalf("List returned %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %#v", got)
	}
}

func TestDeleteRemovesIt(t *testing.T) {
	withoutKeyring(t)
	dir := t.TempDir()

	Set(dir, "token", "s3cret")
	if err := Delete(dir, "token"); err != nil {
		t.Fatalf("Delete returned %v", err)
	}
	if _, err := Get(dir, "token"); err == nil {
		t.Fatal("the secret survived Delete")
	}
}

func TestDeleteSaysWhenThereWasNothingToDelete(t *testing.T) {
	withoutKeyring(t)

	if err := Delete(t.TempDir(), "absent"); err == nil {
		t.Fatal("expected an error for a secret that does not exist")
	}
}

func TestTheFallbackFileIsNotReadableByOthers(t *testing.T) {
	withoutKeyring(t)
	dir := t.TempDir()

	Set(dir, "token", "s3cret")

	info, err := os.Stat(filepath.Join(dir, fallbackFile))
	if err != nil {
		t.Fatalf("the fallback file is missing: %v", err)
	}
	if perm := info.Mode().Perm(); perm != fs.FileMode(0o600) {
		t.Fatalf("mode = %o, want 600: a secret must not be readable by others", perm)
	}
}

func TestTheKeyringIsPreferredOverTheFile(t *testing.T) {
	stored := map[string]string{}
	keyringSet = func(_, name, value string) error { stored[name] = value; return nil }
	keyringGet = func(_, name string) (string, error) {
		v, ok := stored[name]
		if !ok {
			return "", errors.New("not found")
		}
		return v, nil
	}
	keyringDelete = func(_, name string) error { delete(stored, name); return nil }
	t.Cleanup(func() {
		keyringSet, keyringGet, keyringDelete = realKeyringSet, realKeyringGet, realKeyringDelete
	})

	dir := t.TempDir()
	if err := Set(dir, "token", "s3cret"); err != nil {
		t.Fatalf("Set returned %v", err)
	}

	// The value belongs in the keychain and must not reach the disk. The name
	// still has to, or List would show nothing.
	body, err := os.ReadFile(filepath.Join(dir, fallbackFile))
	if err != nil {
		t.Fatalf("the index is missing, so List would show nothing: %v", err)
	}
	if strings.Contains(string(body), "s3cret") {
		t.Fatalf("the value was written to disk: %s", body)
	}
	if got, _ := Get(dir, "token"); got != "s3cret" {
		t.Fatalf("got %q", got)
	}
}

func TestListShowsASecretThatOnlyTheKeychainHolds(t *testing.T) {
	stored := map[string]string{}
	keyringSet = func(_, name, value string) error { stored[name] = value; return nil }
	keyringGet = func(_, name string) (string, error) {
		v, ok := stored[name]
		if !ok {
			return "", errors.New("not found")
		}
		return v, nil
	}
	keyringDelete = func(_, name string) error { delete(stored, name); return nil }
	t.Cleanup(func() {
		keyringSet, keyringGet, keyringDelete = realKeyringSet, realKeyringGet, realKeyringDelete
	})

	dir := t.TempDir()
	Set(dir, "token", "s3cret")

	// A keychain cannot be listed by service, so without an index of names a
	// secret stored there would be invisible.
	names, err := List(dir)
	if err != nil {
		t.Fatalf("List returned %v", err)
	}
	if len(names) != 1 || names[0] != "token" {
		t.Fatalf("got %#v, want the keychain-held name", names)
	}
}

func TestListJoinsTheKeyringAndTheFileWithoutRepeating(t *testing.T) {
	// A name can exist in both when the keyring started working after a
	// fallback write. It should still be listed once.
	withoutKeyring(t)
	dir := t.TempDir()
	Set(dir, "token", "from-file")

	keyringGet = func(_, name string) (string, error) {
		if name == "token" {
			return "from-keyring", nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { keyringGet = realKeyringGet })

	names, _ := List(dir)
	if len(names) != 1 || names[0] != "token" {
		t.Fatalf("got %#v, want one entry", names)
	}
	// The keyring is the preferred store, so its value is the one that counts.
	if got, _ := Get(dir, "token"); got != "from-keyring" {
		t.Fatalf("got %q, want the keyring's value", got)
	}
}
