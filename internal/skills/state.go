package skills

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Installed is the persisted ownership of skills installed from one source.
type Installed struct {
	Source string   `json:"source"`
	Root   string   `json:"root"`
	Skills []string `json:"skills"`
	Agents []Agent  `json:"agents"`
}

func (i Installer) stateDirectory() string {
	if i.StateDir != "" {
		return i.StateDir
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "devmachine", "skills")
	}
	return filepath.Join(i.Home, ".local", "state", "devmachine", "skills")
}

func stateName(source string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(source)) + ".json"
}

func (i Installer) statePath(source string) string {
	return filepath.Join(i.stateDirectory(), stateName(source))
}

func (i Installer) records() ([]Installed, error) {
	dir := i.stateDirectory()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading skill ownership from %s: %w", dir, err)
	}
	var records []Installed
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading skill ownership %s: %w", entry.Name(), err)
		}
		var record Installed
		if err := json.Unmarshal(body, &record); err != nil {
			return nil, fmt.Errorf("parsing skill ownership %s: %w", entry.Name(), err)
		}
		if record.Source == "" {
			return nil, fmt.Errorf("skill ownership %s has no source", entry.Name())
		}
		records = append(records, record)
	}
	sort.Slice(records, func(a, b int) bool { return records[a].Source < records[b].Source })
	return records, nil
}

func (i Installer) writeRecord(record Installed) error {
	dir := i.stateDirectory()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating skill ownership directory: %w", err)
	}
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	tmp, err := os.CreateTemp(dir, ".ownership-*.json")
	if err != nil {
		return fmt.Errorf("creating skill ownership record: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, i.statePath(record.Source)); err != nil {
		return fmt.Errorf("saving skill ownership for %s: %w", record.Source, err)
	}
	return nil
}

func (i Installer) removeRecord(source string) error {
	err := os.Remove(i.statePath(source))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// List returns all skill sources managed by Devmachine.
func (i Installer) List() ([]Installed, error) {
	return i.records()
}
