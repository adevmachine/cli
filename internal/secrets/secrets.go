// Package secrets stores the tokens a provider needs.
//
// The operating system's keychain holds them when there is one. When there is
// not — a headless Linux box, a locked-down container — they fall back to a
// file in the configuration directory, readable by nobody else. The fallback
// exists so the CLI works everywhere, not because a file is a good place for a
// secret.
package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/zalando/go-keyring"
)

// service is how these secrets are labelled in the OS keychain.
const service = "devmachine"

// fallbackFile is the index of secret names, and the store of the values the
// keychain could not take.
const fallbackFile = "secrets.json"

// entry is one secret in the index.
//
// A value is present only when the keychain refused it. The name is always
// recorded: a keychain cannot be enumerated by service, so without this index
// a secret stored there would be invisible to List.
type entry struct {
	Value string `json:"value,omitempty"`
}

// Seams, so the tests never touch the real keychain.
var (
	realKeyringSet    = keyring.Set
	realKeyringGet    = keyring.Get
	realKeyringDelete = keyring.Delete

	keyringSet    = realKeyringSet
	keyringGet    = realKeyringGet
	keyringDelete = realKeyringDelete
)

// Set stores a secret, preferring the keychain, and records its name either
// way so List can see it.
func Set(dir, name, value string) error {
	stored, err := fileLoad(dir)
	if err != nil {
		return err
	}
	if err := keyringSet(service, name, value); err == nil {
		stored[name] = entry{}
	} else {
		stored[name] = entry{Value: value}
	}
	return fileSave(dir, stored)
}

// Get returns a secret. The keychain wins when both stores hold the name.
func Get(dir, name string) (string, error) {
	if v, err := keyringGet(service, name); err == nil {
		return v, nil
	}
	stored, err := fileLoad(dir)
	if err != nil {
		return "", err
	}
	e, ok := stored[name]
	if !ok {
		return "", fmt.Errorf("no secret named %q", name)
	}
	if e.Value == "" {
		return "", fmt.Errorf("secret %q is in the OS keychain but it cannot be read right now", name)
	}
	return e.Value, nil
}

// List returns the names that are stored, never a value.
func List(dir string) ([]string, error) {
	stored, err := fileLoad(dir)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(stored))
	for name := range stored {
		names = append(names, name)
	}
	slices.Sort(names)
	return names, nil
}

// Delete removes a secret from both stores.
func Delete(dir, name string) error {
	keyringErr := keyringDelete(service, name)

	stored, err := fileLoad(dir)
	if err != nil {
		return err
	}
	_, known := stored[name]
	if known {
		delete(stored, name)
		if err := fileSave(dir, stored); err != nil {
			return err
		}
	}

	if keyringErr != nil && !known {
		return fmt.Errorf("no secret named %q", name)
	}
	return nil
}

func fileLoad(dir string) (map[string]entry, error) {
	body, err := os.ReadFile(filepath.Join(dir, fallbackFile))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]entry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the stored secrets: %w", err)
	}

	stored := map[string]entry{}
	if err := json.Unmarshal(body, &stored); err != nil {
		return nil, fmt.Errorf("reading the stored secrets: %w", err)
	}
	return stored, nil
}

// fileSave writes through a temporary file and renames it, so an interrupted
// write cannot leave the store truncated.
func fileSave(dir string, stored map[string]entry) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	body, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("rendering the secrets: %w", err)
	}

	path := filepath.Join(dir, fallbackFile)
	tmp, err := os.CreateTemp(dir, fallbackFile+".*")
	if err != nil {
		return fmt.Errorf("writing the secrets: %w", err)
	}
	defer os.Remove(tmp.Name())

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("securing the secrets file: %w", err)
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return fmt.Errorf("writing the secrets: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing the secrets: %w", err)
	}
	return os.Rename(tmp.Name(), path)
}
