// Package hostkeys owns the configuration-scoped SSH host trust store.
package hostkeys

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Store is one OpenSSH known-hosts file.
type Store struct {
	path string
}

// Alias returns the stable SSH identity name for a configured machine.
func Alias(machine string) string { return machine + "-devmachine" }

// Lookup returns the exact token OpenSSH uses to index a machine and port.
func Lookup(machine string, port int) string {
	return knownhosts.Normalize(net.JoinHostPort(Alias(machine), strconv.Itoa(port)))
}

// Algorithms limits negotiation to signatures compatible with the pinned key.
func Algorithms(key ssh.PublicKey) []string {
	if key.Type() == ssh.KeyAlgoRSA {
		return []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256}
	}
	return []string{key.Type()}
}

// Fingerprint returns the public SHA256 fingerprint OpenSSH displays.
func Fingerprint(key ssh.PublicKey) string { return ssh.FingerprintSHA256(key) }

// Open validates path as an OpenSSH known-hosts file. A missing file is an
// empty store; it is created only by Put.
func Open(path string) (*Store, error) {
	store := &Store{path: path}
	if _, err := store.lines(); err != nil {
		return nil, err
	}
	return store, nil
}

// Check verifies key against the selected machine identity.
func (s *Store) Check(machine string, port int, key ssh.PublicKey) error {
	if _, err := s.lines(); err != nil {
		return err
	}
	callback, err := knownhosts.New(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return &knownhosts.KeyError{}
	}
	if err != nil {
		return fmt.Errorf("reading SSH host trust from %s: %w", s.path, err)
	}
	remote := &net.TCPAddr{Port: port}
	return callback(net.JoinHostPort(Alias(machine), strconv.Itoa(port)), remote, key)
}

// Put atomically replaces the selected machine entry while preserving every
// other line in the trust store.
func (s *Store) Put(machine string, port int, key ssh.PublicKey) error {
	lines, err := s.lines()
	if err != nil {
		return err
	}
	lookup := Lookup(machine, port)
	replacement := knownhosts.Line([]string{lookup}, key)
	out := make([]string, 0, len(lines)+1)
	inserted := false
	for _, line := range lines {
		matches, err := lineMatches(line, lookup)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", s.path, err)
		}
		if matches {
			if !inserted {
				out = append(out, replacement)
				inserted = true
			}
			continue
		}
		out = append(out, line)
	}
	if !inserted {
		out = append(out, replacement)
	}

	body := []byte(strings.Join(out, "\n") + "\n")
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".known_hosts-*")
	if err != nil {
		return fmt.Errorf("creating temporary trust store beside %s: %w", s.path, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("protecting temporary trust store for %s: %w", s.path, err)
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temporary trust store for %s: %w", s.path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("syncing temporary trust store for %s: %w", s.path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temporary trust store for %s: %w", s.path, err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("replacing %s: %w", s.path, err)
	}
	return nil
}

func (s *Store) lines() ([]string, error) {
	body, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", s.path, err)
	}
	trimmed := bytes.TrimSuffix(body, []byte("\n"))
	if len(trimmed) == 0 {
		return nil, nil
	}
	lines := strings.Split(string(trimmed), "\n")
	for i, line := range lines {
		if _, err := lineMatches(line, ""); err != nil {
			return nil, fmt.Errorf("parsing %s line %d: %w", s.path, i+1, err)
		}
	}
	return lines, nil
}

func lineMatches(line, lookup string) (bool, error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false, nil
	}
	_, hosts, _, _, rest, err := ssh.ParseKnownHosts([]byte(line + "\n"))
	if err != nil {
		return false, err
	}
	if len(bytes.TrimSpace(rest)) != 0 {
		return false, fmt.Errorf("unexpected trailing known-host data")
	}
	return slices.Contains(hosts, lookup), nil
}
