// Package keys makes and finds the SSH keys the CLI uses to reach a machine.
package keys

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// Dir is where generated keys live, under the configuration directory.
func Dir(configDir string) string {
	return filepath.Join(configDir, "keys")
}

// Generate writes an ed25519 pair into dir and returns the private key's path
// and the public key's line. It refuses to write over a key that is already
// there.
func Generate(dir, name string) (path, public string, err error) {
	if err := checkName(name); err != nil {
		return "", "", err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("making the key directory: %w", err)
	}

	path = filepath.Join(dir, name)
	publicPath := path + ".pub"
	for _, p := range []string{path, publicPath} {
		if _, err := os.Lstat(p); err == nil {
			// Writing over a key locks you out of every machine that already
			// trusts it, and nothing brings it back.
			return "", "", fmt.Errorf("%s already exists: remove it by hand if you really want a new key", p)
		} else if !os.IsNotExist(err) {
			return "", "", fmt.Errorf("looking at %s: %w", p, err)
		}
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generating the key: %w", err)
	}

	block, err := ssh.MarshalPrivateKey(privateKey, "")
	if err != nil {
		return "", "", fmt.Errorf("encoding the private key: %w", err)
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return "", "", fmt.Errorf("writing %s: %w", path, err)
	}

	wire, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return "", "", fmt.Errorf("encoding the public key: %w", err)
	}
	public = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(wire))) + " devmachine-" + name

	if err := os.WriteFile(publicPath, []byte(public+"\n"), 0o644); err != nil {
		return "", "", fmt.Errorf("writing %s: %w", publicPath, err)
	}

	return path, public, nil
}

func checkName(name string) error {
	if name == "" {
		return fmt.Errorf("a key needs a name")
	}
	if name != filepath.Base(name) || name == "." || name == ".." || strings.ContainsRune(name, filepath.Separator) {
		return fmt.Errorf("%q is not a key name: it would write outside the key directory", name)
	}
	return nil
}
