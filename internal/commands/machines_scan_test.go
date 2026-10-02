package commands

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/hostkeys"
	"golang.org/x/crypto/ssh"
)

func stubScan(t *testing.T) (ssh.PublicKey, *config.Machine) {
	t.Helper()
	key := commandHostKey(t)
	scanned := &config.Machine{}
	t.Cleanup(swap(&scanHostKey, func(_ context.Context, m config.Machine) (ssh.PublicKey, string, error) {
		*scanned = m
		return key, m.Hosts[0].Address, nil
	}))
	return key, scanned
}

func TestMachinesScanReportsTheHostKeyAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	key, scanned := stubScan(t)

	out, err := execute(t, "--config", dir, "--format", "json", "machines", "scan",
		"--address", "203.0.113.20", "--port", "2222")
	if err != nil {
		t.Fatalf("machines scan returned %v (%s)", err, out)
	}
	if scanned.Port != 2222 || scanned.Hosts[0].Address != "203.0.113.20" || scanned.KnownHostsFile != "" {
		t.Fatalf("it scanned %#v", scanned)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v (%q)", err, out)
	}
	want := map[string]any{
		"address":     "203.0.113.20",
		"port":        float64(2222),
		"key_type":    "ssh-ed25519",
		"fingerprint": hostkeys.Fingerprint(key),
		"verify":      "ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub",
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %#v, want %#v (%s)", k, got[k], v, out)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("scan wrote %v", entries)
	}
	if _, err := os.Stat(filepath.Join(dir, config.KnownHostsFileName)); !os.IsNotExist(err) {
		t.Fatal("scan wrote known_hosts")
	}
}

func TestMachinesScanAsATableSaysWhatToPassToAdd(t *testing.T) {
	key, _ := stubScan(t)

	out, err := execute(t, "--config", t.TempDir(), "machines", "scan", "--address", "203.0.113.20")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, hostkeys.Fingerprint(key)) || !strings.Contains(out, "--fingerprint") {
		t.Fatalf("got %q", out)
	}
}

func TestMachinesScanNeedsAnAddress(t *testing.T) {
	stubScan(t)
	if _, err := execute(t, "--config", t.TempDir(), "machines", "scan"); err == nil ||
		!strings.Contains(err.Error(), "--address") {
		t.Fatalf("got %v", err)
	}
}
