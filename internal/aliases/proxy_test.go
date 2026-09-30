package aliases

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mydevmachine/devmachine/internal/config"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const testCLI = "/opt/homebrew/bin/devmachine"

// golden compares a rendered block with testdata/<name>, with the throwaway
// trust-store directory written as $DIR so the file does not move per run.
func golden(t *testing.T, name, got string, cfg config.Config) {
	t.Helper()
	got = strings.ReplaceAll(got, filepath.Dir(cfg.Machines[0].KnownHostsFile), "$DIR")
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("%s differs, got:\n%s\nwant:\n%s", name, got, want)
	}
}

func withConfigDir(cfg config.Config, dir string) config.Config {
	for i := range cfg.Machines {
		cfg.Machines[i].ConfigDir = dir
	}
	return cfg
}

func TestRenderProxyForm(t *testing.T) {
	cfg := withConfigDir(twoAddresses(t), "/Users/alice/.config/devmachine")

	block, err := Render(cfg, Options{CLI: testCLI})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "proxy.golden", block, cfg)
}

func TestRenderLiteralForm(t *testing.T) {
	cfg := twoAddresses(t)

	block, err := Render(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "literal.golden", block, cfg)
}

func TestProxyFormResolvesNothingWhenItIsWritten(t *testing.T) {
	cfg := twoAddresses(t)
	withResolver(t, map[string][]string{})

	block, err := Render(cfg, Options{CLI: testCLI})
	if err != nil {
		t.Fatalf("the proxy form needed an address to be written: %v", err)
	}
	if strings.Contains(block, "100.64.0.5") {
		t.Fatalf("a resolved address was written into the proxy form:\n%s", block)
	}
}

func TestProxyFormKeepsThePinnedKeyWhateverTheAddress(t *testing.T) {
	cfg := twoAddresses(t)

	found, err := List(cfg, Options{CLI: testCLI})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range found {
		if a.HostKeyAlias != "main-devmachine" || a.UserKnownHostsFile != cfg.Machines[0].KnownHostsFile {
			t.Fatalf("%s does not check the pinned key: %#v", a.Name, a)
		}
	}
}

func TestProxyCommandQuotesWhatTheShellAndSSHWouldRead(t *testing.T) {
	cfg := withConfigDir(oneAddress(t), "/Users/alice/My Config/100%")

	found, err := List(cfg, Options{CLI: "/Applications/Dev Machine.app/devmachine"})
	if err != nil {
		t.Fatal(err)
	}
	want := `'/Applications/Dev Machine.app/devmachine' --config '/Users/alice/My Config/100%%' ssh-proxy main %p`
	if found[0].ProxyCommand != want {
		t.Fatalf("got  %s\nwant %s", found[0].ProxyCommand, want)
	}
}

func TestProxyCommandLeavesOutADefaultConfigDirectoryItDoesNotKnow(t *testing.T) {
	found, err := List(oneAddress(t), Options{CLI: testCLI})
	if err != nil {
		t.Fatal(err)
	}
	if found[0].ProxyCommand != testCLI+" ssh-proxy main %p" {
		t.Fatalf("got %s", found[0].ProxyCommand)
	}
}

func TestProxyFormWritesNoPubAliasForOneAddress(t *testing.T) {
	block, err := Render(oneAddress(t), Options{CLI: testCLI})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(block, "-pub") {
		t.Fatalf("got:\n%s", block)
	}
}

func TestProxyFormPubAliasGoesStraightToThePublicAddress(t *testing.T) {
	block, err := Render(twoAddresses(t), Options{CLI: testCLI})
	if err != nil {
		t.Fatal(err)
	}
	after := block[strings.Index(block, "Host alice-devmachine-pub"):]
	if !strings.Contains(after, "HostName 203.0.113.10") || strings.Contains(after, "ProxyCommand") {
		t.Fatalf("the -pub alias does not skip the network:\n%s", block)
	}
}

func TestCLIPathFindsDevmachineOnThePath(t *testing.T) {
	bin := t.TempDir()
	path := filepath.Join(bin, "devmachine")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	if got := CLIPath(); got != path {
		t.Fatalf("got %q", got)
	}
}

func TestCLIPathIsEmptyWhenDevmachineIsNotOnThePath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if got := CLIPath(); got != "" {
		t.Fatalf("got %q", got)
	}
}
