package packages

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// LockFile answers "what is actually on this machine" without logging in to
// look, and is what makes two machines reproducible from one configuration.
const LockFile = "packages.lock"

// LockEntry is one package, where its copy came from, and what pins it.
type LockEntry struct {
	Name   string `yaml:"name" json:"name"`
	Source string `yaml:"source" json:"source"`
	// Version and Checksum are empty for a local package: there is nothing to
	// pin, and pretending otherwise would be a lie in a file whose whole job
	// is being true.
	Version  string `yaml:"version,omitempty" json:"version,omitempty"`
	Checksum string `yaml:"checksum,omitempty" json:"checksum,omitempty"`
}

// Lock is what the last sync applied, per target.
type Lock struct {
	Release    string                 `yaml:"release,omitempty" json:"release,omitempty"`
	AppliedAt  string                 `yaml:"applied_at" json:"applied_at"`
	Machines   map[string][]LockEntry `yaml:"machines" json:"machines"`
	Workspaces map[string][]LockEntry `yaml:"workspaces" json:"workspaces"`
}

// LockPath is where the lock lives.
func LockPath(configDir string) string { return filepath.Join(configDir, LockFile) }

// LoadLock reads the lock. A missing one is the ordinary first state, not an
// error.
func LoadLock(configDir string) (Lock, error) {
	lock := Lock{Machines: map[string][]LockEntry{}, Workspaces: map[string][]LockEntry{}}

	path := LockPath(configDir)
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return lock, nil
	}
	if err != nil {
		return lock, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := yaml.Unmarshal(body, &lock); err != nil {
		return lock, fmt.Errorf("parsing %s: %w", path, err)
	}
	if lock.Machines == nil {
		lock.Machines = map[string][]LockEntry{}
	}
	if lock.Workspaces == nil {
		lock.Workspaces = map[string][]LockEntry{}
	}
	return lock, nil
}

// WithPlan returns the lock as it is after this plan was applied.
//
// Only the machine and the workspaces in the plan are replaced: syncing one
// machine says nothing about another, and the lock must not claim it does.
func (l Lock) WithPlan(plan MachinePlan, store *Store, now time.Time) Lock {
	out := Lock{
		Release:    store.Version(),
		AppliedAt:  now.UTC().Format(time.RFC3339),
		Machines:   maps.Clone(l.Machines),
		Workspaces: maps.Clone(l.Workspaces),
	}
	if out.Machines == nil {
		out.Machines = map[string][]LockEntry{}
	}
	if out.Workspaces == nil {
		out.Workspaces = map[string][]LockEntry{}
	}

	out.Machines[plan.Machine.Name] = lockEntries(plan.OnMachine.Ordered, store)
	for _, w := range plan.Workspaces {
		out.Workspaces[w.Target.Name] = lockEntries(w.Ordered, store)
	}
	return out
}

func lockEntries(found []Found, store *Store) []LockEntry {
	out := make([]LockEntry, 0, len(found))
	for _, f := range found {
		entry := LockEntry{Name: f.Manifest.Name, Source: f.Source}
		if f.Source == SourceRelease {
			entry.Version, entry.Checksum = store.Version(), store.Checksum()
		}
		out = append(out, entry)
	}
	return out
}

// SaveLock writes the lock atomically. A lock truncated by an interrupted
// write is worse than one that is out of date.
func SaveLock(configDir string, l Lock) error {
	body, err := yaml.Marshal(l)
	if err != nil {
		return fmt.Errorf("rendering %s: %w", LockFile, err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}

	temp, err := os.CreateTemp(configDir, LockFile+".*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())

	if _, err := temp.Write(body); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(temp.Name(), LockPath(configDir))
}
