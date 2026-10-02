package remote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestControlSocketDirKeepsTheCacheDirectoryWhenTheSocketFits(t *testing.T) {
	dir, err := controlSocketDir("/Users/alice/Library/Caches", 501, 104)
	if err != nil {
		t.Fatal(err)
	}
	if want := "/Users/alice/Library/Caches/devmachine/cm"; dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}
}

func TestControlSocketDirFallsBackToAShortDirectoryUnderAVeryLongHome(t *testing.T) {
	home := "/Users/" + strings.Repeat("a", 60) + "/Library/Caches"

	dir, err := controlSocketDir(home, 501, 104)
	if err != nil {
		t.Fatal(err)
	}
	if want := "/tmp/dm-501"; dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}
	socket := controlSocketPath(dir, "alice", "203.0.113.10", 22)
	if n := len(socket) + controlSocketSuffix; n > 104 {
		t.Fatalf("%q plus ssh's temporary suffix is %d bytes, over the 104-byte limit", socket, n)
	}
}

func TestControlSocketDirFitsWithTheSuffixSshAddsWhileCreatingTheMaster(t *testing.T) {
	// 12-character user: the path that broke with ssh's own 40-character %C.
	home := "/Users/abcdefghijkl/Library/Caches"
	dir, err := controlSocketDir(home, 501, 104)
	if err != nil {
		t.Fatal(err)
	}
	socket := controlSocketPath(dir, "root", "127.0.0.1", 60022)
	if n := len(socket) + controlSocketSuffix; n > 104 {
		t.Fatalf("%q plus ssh's temporary suffix is %d bytes, over the 104-byte limit", socket, n)
	}
}

func TestControlSocketDirUsesTheLongerLinuxLimit(t *testing.T) {
	cache := "/home/" + strings.Repeat("b", 44) + "/.cache"
	if got, _ := controlSocketDir(cache, 1000, 104); got != "/tmp/dm-1000" {
		t.Fatalf("with a 104-byte limit dir = %q, want the fallback", got)
	}
	if got, _ := controlSocketDir(cache, 1000, 108); got != filepath.Join(cache, "devmachine", "cm") {
		t.Fatalf("with a 108-byte limit dir = %q, want the cache directory", got)
	}
}

func TestControlSocketPathIsShortAndDiffersPerUserAddressAndPort(t *testing.T) {
	base := controlSocketPath("/d", "alice", "203.0.113.10", 22)
	if name := filepath.Base(base); len(name) != 16 {
		t.Fatalf("socket name %q is %d bytes, want 16", name, len(name))
	}
	if base != controlSocketPath("/d", "alice", "203.0.113.10", 22) {
		t.Fatal("the same connection got two different socket names")
	}
	for _, other := range []string{
		controlSocketPath("/d", "bob", "203.0.113.10", 22),
		controlSocketPath("/d", "alice", "203.0.113.11", 22),
		controlSocketPath("/d", "alice", "203.0.113.10", 2222),
	} {
		if other == base {
			t.Fatalf("two different connections share the socket %q", base)
		}
	}
}

func TestEnsurePrivateDirCreatesItMode0700(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dm-test")
	if err := ensurePrivateDir(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("mode = %o, want 0700", perm)
	}
	if err := ensurePrivateDir(dir); err != nil {
		t.Fatalf("an existing private directory was refused: %v", err)
	}
}

func TestEnsurePrivateDirRefusesADirectoryOthersCanWriteTo(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dm-test")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateDir(dir); err == nil {
		t.Fatal("a directory every account can write to was accepted")
	}
}

func TestEnsurePrivateDirRefusesASymbolicLink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "dm-test")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateDir(link); err == nil {
		t.Fatal("a symbolic link was accepted as the socket directory")
	}
}

func TestEnsurePrivateDirRefusesADirectoryAnotherAccountOwns(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root owns every directory it could test against")
	}
	if _, err := os.Stat("/"); err != nil {
		t.Skip(err)
	}
	err := ensurePrivateDir("/")
	if err == nil || !strings.Contains(err.Error(), "owned by") {
		t.Fatalf("got %v, want a refusal naming the owner", err)
	}
}
