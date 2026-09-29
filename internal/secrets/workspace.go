package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// workspaceSecretsFile is the index of where a workspace's own secret is
// delivered. It holds no values — those stay in Set and Get's own store —
// only what `secrets set --env-file` and `secrets rm --from-file` need to
// remember between runs.
const workspaceSecretsFile = "workspace-secrets.json"

// Target is where one workspace's own secret is delivered.
type Target struct {
	Workspace string `json:"workspace"`
	Name      string `json:"name"`
	// EnvFile is the destination, relative to the workspace's home. Empty
	// means the default: ~/.devmachine/env, a file the workspace's shell
	// sources.
	EnvFile string `json:"env_file,omitempty"`
	// PendingRemoval means the next `credentials push` removes Name from
	// EnvFile, then forgets this target.
	PendingRemoval bool `json:"pending_removal,omitempty"`
}

// Key is how a target is named in the index and in `secrets list`.
func (t Target) Key() string { return targetKey(t.Workspace, t.Name) }

func targetKey(workspace, name string) string { return workspace + "/" + name }

// SetTarget records where a workspace secret is delivered, replacing
// whatever was recorded for it before.
func SetTarget(dir string, t Target) error {
	stored, err := targetsLoad(dir)
	if err != nil {
		return err
	}
	stored[t.Key()] = t
	return targetsSave(dir, stored)
}

// FindTarget returns the recorded target for one workspace secret, and
// whether there was one.
func FindTarget(dir, workspace, name string) (Target, bool, error) {
	stored, err := targetsLoad(dir)
	if err != nil {
		return Target{}, false, err
	}
	t, ok := stored[targetKey(workspace, name)]
	return t, ok, nil
}

// Targets returns every recorded workspace secret target, sorted by
// workspace and then by name.
func Targets(dir string) ([]Target, error) {
	stored, err := targetsLoad(dir)
	if err != nil {
		return nil, err
	}
	out := make([]Target, 0, len(stored))
	for _, t := range stored {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Workspace != out[j].Workspace {
			return out[i].Workspace < out[j].Workspace
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// RemoveTarget forgets where a workspace secret was delivered. It never
// touches the value: Delete does that.
func RemoveTarget(dir, workspace, name string) error {
	stored, err := targetsLoad(dir)
	if err != nil {
		return err
	}
	delete(stored, targetKey(workspace, name))
	return targetsSave(dir, stored)
}

func targetsLoad(dir string) (map[string]Target, error) {
	body, err := os.ReadFile(filepath.Join(dir, workspaceSecretsFile))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Target{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the workspace secret targets: %w", err)
	}

	stored := map[string]Target{}
	if err := json.Unmarshal(body, &stored); err != nil {
		return nil, fmt.Errorf("reading the workspace secret targets: %w", err)
	}
	return stored, nil
}

// targetsSave writes through a temporary file and renames it, so an
// interrupted write cannot leave the index truncated.
func targetsSave(dir string, stored map[string]Target) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	body, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("rendering the workspace secret targets: %w", err)
	}

	path := filepath.Join(dir, workspaceSecretsFile)
	tmp, err := os.CreateTemp(dir, workspaceSecretsFile+".*")
	if err != nil {
		return fmt.Errorf("writing the workspace secret targets: %w", err)
	}
	defer os.Remove(tmp.Name())

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("securing the workspace secret targets file: %w", err)
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return fmt.Errorf("writing the workspace secret targets: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing the workspace secret targets: %w", err)
	}
	return os.Rename(tmp.Name(), path)
}
