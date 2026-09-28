package commands

import (
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/config"
)

func TestPackagesPinWithNoReleasePinsTheLatest(t *testing.T) {
	dir := configWith(t, "machines:\n  - name: main  # the server\n    hosts: [203.0.113.10]\n")
	defer stubLatestPackagesRelease(t, "v8")()

	out, err := execute(t, "--config", dir, "packages", "pin")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Packages != "v8" || !strings.Contains(out, "v8") {
		t.Fatalf("pinned %q, said %q", cfg.Packages, out)
	}
}

func TestPackagesPinTakesANamedRelease(t *testing.T) {
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\npackages: v6\n")
	defer stubLatestPackagesRelease(t, "v9")()

	if _, err := execute(t, "--config", dir, "packages", "pin", "v7"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.Load(dir)
	if cfg.Packages != "v7" {
		t.Fatalf("got %q", cfg.Packages)
	}
}

func TestPackagesPinRefusesABranchName(t *testing.T) {
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n")
	if _, err := execute(t, "--config", dir, "packages", "pin", "main"); err == nil {
		t.Fatal("a branch is not a release")
	}
}

func TestSyncWithNoPinSaysHowToPin(t *testing.T) {
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n"+
		"workspaces:\n  - name: alice\n    packages: [workspace]\n")
	forbidDial(t)

	_, err := execute(t, "--config", dir, "sync", "--check", "--yes")
	if err == nil || !strings.Contains(err.Error(), "devmachine packages pin") {
		t.Fatalf("got %v", err)
	}
}
