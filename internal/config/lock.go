package config

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// LockFileName is the file every writer of config.yml locks, inside the
// cache folder, which a configuration repository already ignores.
const LockFileName = "config.lock"

// Lock holds the configuration's write lock until the returned function is
// called, waiting while another command holds it.
//
// Every writer reads config.yml, changes it and writes it back. Two at once —
// two `machines add` from an app, a sync recording a route while an edit
// runs — would each write back what they read, and the slower one would
// silently drop the other's change. The lock is not reentrant: a writer that
// holds it must not call another writer.
func Lock(dir string) (func(), error) {
	path := filepath.Join(dir, "cache", LockFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("creating the folder for %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("locking %s: %w", path, err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

// locked runs write while holding the configuration's lock.
func locked(dir string, write func() error) error {
	unlock, err := Lock(dir)
	if err != nil {
		return err
	}
	defer unlock()
	return write()
}
