package commands

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/aliases"
	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/hostkeys"
	"golang.org/x/crypto/ssh"
)

func configWithTrustedKey(t *testing.T, extra string) string {
	t.Helper()
	dir := configWithKey(t, extra)
	store, err := hostkeys.Open(filepath.Join(dir, config.KnownHostsFileName))
	if err != nil {
		t.Fatal(err)
	}
	key := commandHostKey(t)
	if err := store.Put("main", 22, key); err != nil {
		t.Fatal(err)
	}
	wasScan := scanHostKey
	scanHostKey = func(context.Context, config.Machine) (ssh.PublicKey, string, error) {
		return key, "203.0.113.10", nil
	}
	t.Cleanup(func() { scanHostKey = wasScan })
	return dir
}

func TestAliasesPrintsTheBlockAndWritesNothing(t *testing.T) {
	dir := configWithTrustedKey(t, "workspaces:\n  - name: alice\n    machine: main\n")

	out, err := execute(t, "--config", dir, "aliases")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{aliases.Begin, "Host alice-devmachine", "HostKeyAlias main-devmachine", aliases.End} {
		if !strings.Contains(out, want) {
			t.Fatalf("output is missing %q: %q", want, out)
		}
	}
}

func TestAliasesWritesOnlyItsOwnBlock(t *testing.T) {
	dir := configWithTrustedKey(t, "workspaces:\n  - name: alice\n    machine: main\n")
	path := filepath.Join(t.TempDir(), "ssh_config")
	if err := os.WriteFile(path, []byte("Host work-jump\n    HostName jump.example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := execute(t, "--config", dir, "aliases", "--write", "--path", path, "--yes"); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"work-jump", "alice-devmachine"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("lost %q:\n%s", want, body)
		}
	}
}

func TestAliasesAsksBeforeTouchingTheFile(t *testing.T) {
	dir := configWithTrustedKey(t, "workspaces:\n  - name: alice\n    machine: main\n")
	path := filepath.Join(t.TempDir(), "ssh_config")

	out, err := executeWithInput(t, "n\n", "--config", dir, "aliases", "--write", "--path", path)
	if !errors.Is(err, errDeclined) {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(out, path) {
		t.Fatalf("the question does not name the file: %q", out)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("a no still wrote the file")
	}
}

func TestAliasesCheckWritesNothing(t *testing.T) {
	dir := configWithTrustedKey(t, "workspaces:\n  - name: alice\n    machine: main\n")
	path := filepath.Join(t.TempDir(), "ssh_config")

	out, err := execute(t, "--config", dir, "aliases", "--write", "--path", path, "--check")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "would") {
		t.Fatalf("a dry run should say what it would do: %q", out)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("a dry run wrote the file")
	}
}

func TestAliasesPathWithoutWriteIsRefused(t *testing.T) {
	dir := configWithTrustedKey(t, "workspaces:\n  - name: alice\n    machine: main\n")

	_, err := execute(t, "--config", dir, "aliases", "--path", filepath.Join(t.TempDir(), "ssh_config"))
	if err == nil {
		t.Fatal("it took a path and wrote nowhere")
	}
	if !strings.Contains(err.Error(), "--write") {
		t.Fatalf("the error does not say what is missing: %v", err)
	}
}

func TestAliasesAsJSONCarriesEachEntry(t *testing.T) {
	dir := configWithTrustedKey(t, "workspaces:\n  - name: alice\n    machine: main\n")

	out, err := execute(t, "--config", dir, "--format", "json", "aliases")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Aliases []aliases.Alias `json:"aliases"`
		Written bool            `json:"written"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output was not JSON: %v (%q)", err, out)
	}
	if len(got.Aliases) != 1 || got.Aliases[0].Name != "alice-devmachine" {
		t.Fatalf("got %#v", got.Aliases)
	}
	if got.Written {
		t.Fatal("printing is not writing")
	}
}

func TestAliasesSaysWhenThereIsNoWorkspace(t *testing.T) {
	dir := configWithKey(t, "")

	out, err := execute(t, "--config", dir, "aliases")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "workspaces new") {
		t.Fatalf("it should say how to get one: %q", out)
	}
}
