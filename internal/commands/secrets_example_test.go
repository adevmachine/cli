package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/packages"
)

func configWithSecretCredential(t *testing.T) string {
	t.Helper()
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [cloudflare]\n")
	writeCredentialPackage(t, dir, "cloudflare", packages.ScopeMachine,
		"  - name: cloudflare\n    kind: secret\n    scope: machine\n    env: CLOUDFLARE_TOKEN\n")
	return dir
}

func TestSecretsExampleListsNamesAndNoValues(t *testing.T) {
	dir := configWithSecretCredential(t)
	storeSecret(t, dir, "cloudflare", "s3cret")

	out, err := execute(t, "--config", dir, "secrets", "example")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "CLOUDFLARE_TOKEN=") {
		t.Fatalf("it does not list the name: %q", out)
	}
	if strings.Contains(out, "s3cret") {
		t.Fatal("it printed the value")
	}
}

func TestSecretsExampleWritesToStdoutNotToAFile(t *testing.T) {
	// A file named .env.example one typo away from .env, in a directory that
	// is about to be a git repository, is a trap. The operator redirects it
	// wherever they want it.
	dir := configWithSecretCredential(t)

	if _, err := execute(t, "--config", dir, "secrets", "example"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.example")); err == nil {
		t.Fatal("it wrote a file")
	}
}

func TestSecretsExampleSkipsCredentialsThatAreNotSecrets(t *testing.T) {
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [gh-login]\n")
	writeCredentialPackage(t, dir, "gh-login", packages.ScopeMachine,
		"  - name: gh\n    kind: manual\n    scope: machine\n"+
			"    command: gh auth login\n    stored_at: ~/.config/gh/hosts.yml\n")

	out, err := execute(t, "--config", dir, "secrets", "example")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("a manual login has no value to list: got %q", out)
	}
}
