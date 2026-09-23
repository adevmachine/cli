package commands

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/hostkeys"
	"golang.org/x/crypto/ssh"
)

func TestMachinesTrustMissingKeyDeclineWritesNothing(t *testing.T) {
	dir, presented := trustCommandFixture(t)
	out, err := executeWithInput(t, "n\n", "--config", dir, "machines", "trust", "main")
	if !errors.Is(err, errDeclined) {
		t.Fatalf("error = %v, output = %q", err, out)
	}
	if _, statErr := os.Stat(filepath.Join(dir, config.KnownHostsFileName)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("decline created known_hosts")
	}
	if !strings.Contains(out, hostkeys.Fingerprint(presented)) {
		t.Fatalf("prompt did not show fingerprint: %q", out)
	}
}

func TestMachinesTrustMissingKeyWithYesAddsIt(t *testing.T) {
	dir, presented := trustCommandFixture(t)
	dir = gitRepoDir(t, dir)
	out, err := execute(t, "--config", dir, "machines", "trust", "main", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	assertTrustedKey(t, dir, presented)
	if !strings.Contains(out, "trusted") {
		t.Fatalf("output = %q", out)
	}
	if got := lastCommitMessage(t, dir); got != "chore(config): trust host key for main" {
		t.Fatalf("commit message = %q", got)
	}
}

func TestMachinesTrustMatchingKeyDoesNotPromptOrWrite(t *testing.T) {
	dir, presented := trustCommandFixture(t)
	seedTrust(t, dir, presented)
	before := trustChecksum(t, dir)

	out, err := executeWithInput(t, "", "--config", dir, "machines", "trust", "main")
	if err != nil {
		t.Fatal(err)
	}
	if after := trustChecksum(t, dir); after != before {
		t.Fatal("matching key rewrote the trust store")
	}
	if !strings.Contains(out, "already matches") {
		t.Fatalf("output = %q", out)
	}
}

func TestMachinesTrustChangedKeyNeedsReplace(t *testing.T) {
	dir, presented := trustCommandFixture(t)
	old := commandHostKey(t)
	seedTrust(t, dir, old)

	_, err := execute(t, "--config", dir, "machines", "trust", "main", "--yes")
	if err == nil {
		t.Fatal("changed key was accepted without --replace")
	}
	for _, want := range []string{hostkeys.Fingerprint(old), hostkeys.Fingerprint(presented), "--replace"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error does not contain %q: %v", want, err)
		}
	}
}

func TestMachinesTrustChangedKeyReplacementCanBeDeclined(t *testing.T) {
	dir, presented := trustCommandFixture(t)
	old := commandHostKey(t)
	seedTrust(t, dir, old)
	before := trustChecksum(t, dir)

	_, err := executeWithInput(t, "n\n", "--config", dir, "machines", "trust", "main", "--replace")
	if !errors.Is(err, errDeclined) {
		t.Fatalf("error = %v", err)
	}
	if after := trustChecksum(t, dir); after != before {
		t.Fatal("declined replacement changed the file")
	}
	assertTrustedKey(t, dir, old)
	_ = presented
}

func TestMachinesTrustCheckReportsChangeWithoutWriting(t *testing.T) {
	dir, _ := trustCommandFixture(t)
	seedTrust(t, dir, commandHostKey(t))
	before := trustChecksum(t, dir)

	out, err := execute(t, "--format", "json", "--config", dir, "machines", "trust", "main", "--replace", "--check", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if after := trustChecksum(t, dir); after != before {
		t.Fatal("--check changed the file")
	}
	var result trustResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("stdout was not only JSON: %v (%q)", err, out)
	}
	if result.Status != "changed" || !result.Check || result.Changed {
		t.Fatalf("result = %#v", result)
	}
}

func TestMachinesTrustChangedKeyWithReplaceCommitsIt(t *testing.T) {
	dir, presented := trustCommandFixture(t)
	dir = gitRepoDir(t, dir)
	seedTrust(t, dir, commandHostKey(t))

	if _, err := execute(t, "--config", dir, "machines", "trust", "main", "--replace", "--yes"); err != nil {
		t.Fatal(err)
	}
	assertTrustedKey(t, dir, presented)
	if got := lastCommitMessage(t, dir); got != "chore(config): replace host key for main" {
		t.Fatalf("commit message = %q", got)
	}
}

func TestMachinesTrustJSONHasStableFieldsAndNoDiagnostics(t *testing.T) {
	dir, _ := trustCommandFixture(t)
	out, err := execute(t, "--format", "json", "--config", dir, "machines", "trust", "main", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("stdout was not only JSON: %v (%q)", err, out)
	}
	want := []string{"machine", "address", "status", "key_type", "presented_fingerprint", "check", "changed"}
	for _, field := range want {
		if _, ok := raw[field]; !ok {
			t.Fatalf("JSON is missing %q: %v", field, raw)
		}
	}
	if _, ok := raw["current_fingerprint"]; ok {
		t.Fatalf("new trust unexpectedly has current_fingerprint: %v", raw)
	}
}

func TestMachinesTrustPositionalAndFlagMustAgree(t *testing.T) {
	dir, _ := trustCommandFixture(t)
	_, err := execute(t, "--config", dir, "--machine", "other", "machines", "trust", "main", "--yes")
	if err == nil || !strings.Contains(err.Error(), "different machines") {
		t.Fatalf("error = %v", err)
	}
}

func trustCommandFixture(t *testing.T) (string, ssh.PublicKey) {
	t.Helper()
	dir := writeConfigDir(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n")
	presented := commandHostKey(t)
	t.Cleanup(swap(&scanHostKey, func(_ context.Context, _ config.Machine) (ssh.PublicKey, string, error) {
		return presented, "203.0.113.10", nil
	}))
	return dir, presented
}

func seedTrust(t *testing.T, dir string, key ssh.PublicKey) {
	t.Helper()
	store, err := hostkeys.Open(filepath.Join(dir, config.KnownHostsFileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("main", 22, key); err != nil {
		t.Fatal(err)
	}
}

func assertTrustedKey(t *testing.T, dir string, key ssh.PublicKey) {
	t.Helper()
	store, err := hostkeys.Open(filepath.Join(dir, config.KnownHostsFileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Check("main", 22, key); err != nil {
		t.Fatalf("key is not trusted: %v", err)
	}
}

func trustChecksum(t *testing.T, dir string) [sha256.Size]byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, config.KnownHostsFileName))
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(body)
}
