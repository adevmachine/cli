package remote

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
)

// controlSocketSuffix is what ssh adds to a ControlPath while it creates the
// master: a dot and 16 random characters for the temporary socket it renames
// into place, plus the terminating NUL the kernel counts.
const controlSocketSuffix = 18

// controlSocketNameLen keeps the socket name short. 16 hex characters still
// leave a collision between two of one person's connections out of reach.
const controlSocketNameLen = 16

// unixSocketPathLimit is the size of sun_path, NUL included: 104 bytes on
// macOS and the BSDs, 108 on Linux.
func unixSocketPathLimit() int {
	if runtime.GOOS == "linux" {
		return 108
	}
	return 104
}

// controlSocketDir picks where the ControlMaster sockets live: the cache
// directory, unless a socket there plus ssh's temporary suffix would not fit
// in a Unix socket path, as under a long home directory. Then a short
// per-account directory under /tmp.
func controlSocketDir(cacheDir string, uid, limit int) (string, error) {
	need := func(dir string) int { return len(dir) + 1 + controlSocketNameLen + controlSocketSuffix }
	dir := filepath.Join(cacheDir, "devmachine", "cm")
	if need(dir) <= limit {
		return dir, nil
	}
	fallback := "/tmp/dm-" + strconv.Itoa(uid)
	if need(fallback) <= limit {
		return fallback, nil
	}
	return "", fmt.Errorf("no directory keeps a connection socket under the %d-byte Unix socket limit", limit)
}

// controlSocketPath names one connection's socket after what tells two
// connections apart, as ssh's own %C does, but in 16 characters instead of 40.
func controlSocketPath(dir, user, address string, port int) string {
	sum := sha256.Sum256([]byte(user + "@" + address + ":" + strconv.Itoa(port)))
	return filepath.Join(dir, hex.EncodeToString(sum[:])[:controlSocketNameLen])
}

// ensurePrivateDir creates dir mode 0700, or accepts it only if it already is
// a real directory this account owns and nobody else can enter. /tmp is shared:
// a directory another account prepared there could hand ssh a socket it
// controls.
func ensurePrivateDir(dir string) error {
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("checking %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory, so it cannot hold the connection sockets: remove it", dir)
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("%s is owned by another account (uid %d), so it is not safe for the connection sockets: remove it", dir, st.Uid)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Errorf("%s has mode %o, so other accounts can reach the connection sockets: run chmod 700 %s", dir, perm, dir)
	}
	return nil
}
