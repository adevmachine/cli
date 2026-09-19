package packages

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writePackageAt writes a manifest into an exact directory.
func writePackageAt(t *testing.T, dir, body string) {
	t.Helper()
	write(t, ManifestPath(dir), body)
}

// markCached makes a release look already fetched, so opening a store needs no
// network.
func markCached(t *testing.T, configDir, version string) {
	t.Helper()
	write(t, filepath.Join(CacheDir(configDir, version), checksumFile), "abc123\n")
}

func TestStorePrefersALocalPackageOfTheSameName(t *testing.T) {
	configDir := t.TempDir()
	writePackageAt(t, filepath.Join(LocalDir(configDir), "caddy"),
		"format: 1\nname: caddy\nscope: machine\nsummary: The operator's own caddy.\n")
	writePackageAt(t, filepath.Join(CacheDir(configDir, "v1"), "packages", "caddy"),
		"format: 1\nname: caddy\nscope: machine\nsummary: The published caddy.\n")
	markCached(t, configDir, "v1")

	store, err := Open(context.Background(), configDir, "v1")
	if err != nil {
		t.Fatal(err)
	}
	found, err := store.Get("caddy")
	if err != nil {
		t.Fatal(err)
	}
	if found.Source != SourceLocal {
		t.Fatalf("source is %q", found.Source)
	}
	if !strings.Contains(found.Manifest.Summary, "operator") {
		t.Fatalf("got the published one: %q", found.Manifest.Summary)
	}
}

func TestStoreFallsBackToTheRelease(t *testing.T) {
	configDir := t.TempDir()
	writePackageAt(t, filepath.Join(CacheDir(configDir, "v1"), "packages", "docker"),
		"format: 1\nname: docker\nscope: machine\nsummary: Docker Engine.\n")
	markCached(t, configDir, "v1")

	store, err := Open(context.Background(), configDir, "v1")
	if err != nil {
		t.Fatal(err)
	}
	found, err := store.Get("docker")
	if err != nil {
		t.Fatal(err)
	}
	if found.Source != SourceRelease {
		t.Fatalf("source is %q", found.Source)
	}
	if store.Version() != "v1" || store.Checksum() != "abc123" {
		t.Fatalf("the store does not carry what it opened: %q %q", store.Version(), store.Checksum())
	}
}

func TestStoreSaysWhatIsAvailableWhenAPackageIsMissing(t *testing.T) {
	configDir := t.TempDir()
	writePackageAt(t, filepath.Join(CacheDir(configDir, "v1"), "packages", "docker"),
		"format: 1\nname: docker\nscope: machine\nsummary: Docker Engine.\n")
	markCached(t, configDir, "v1")

	store, err := Open(context.Background(), configDir, "v1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Get("kaddy")
	if err == nil {
		t.Fatal("a package that does not exist was found")
	}
	if !strings.Contains(err.Error(), "docker") {
		t.Fatalf("the error does not say what does exist: %v", err)
	}
}

func TestStoreAllListsBothSourcesOnce(t *testing.T) {
	configDir := t.TempDir()
	writePackageAt(t, filepath.Join(LocalDir(configDir), "caddy"), "format: 1\nname: caddy\nscope: machine\nsummary: a\n")
	writePackageAt(t, filepath.Join(CacheDir(configDir, "v1"), "packages", "caddy"), "format: 1\nname: caddy\nscope: machine\nsummary: b\n")
	writePackageAt(t, filepath.Join(CacheDir(configDir, "v1"), "packages", "git"), "format: 1\nname: git\nscope: machine\nsummary: c\n")
	markCached(t, configDir, "v1")

	store, err := Open(context.Background(), configDir, "v1")
	if err != nil {
		t.Fatal(err)
	}
	all, err := store.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d packages, want 2 (caddy once, git once)", len(all))
	}
	if all[0].Manifest.Name != "caddy" || all[1].Manifest.Name != "git" {
		t.Fatalf("the listing is not in name order: %#v", all)
	}
	if all[0].Source != SourceLocal {
		t.Fatalf("the overriding copy should be the one listed: %q", all[0].Source)
	}
}

// With no pin there is no release to fetch, and the operator's own packages
// are all there is. That is the ordinary state of a machine being set up.
func TestStoreWithNoPinUsesOnlyTheLocalPackages(t *testing.T) {
	configDir := t.TempDir()
	writePackageAt(t, filepath.Join(LocalDir(configDir), "caddy"), "format: 1\nname: caddy\nscope: machine\nsummary: a\n")

	store, err := Open(context.Background(), configDir, "")
	if err != nil {
		t.Fatal(err)
	}
	all, err := store.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Source != SourceLocal {
		t.Fatalf("got %#v", all)
	}
}

func TestStoreIgnoresADirectoryWithNoManifest(t *testing.T) {
	configDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(LocalDir(configDir), "notes"), 0o755); err != nil {
		t.Fatal(err)
	}

	store, err := Open(context.Background(), configDir, "")
	if err != nil {
		t.Fatal(err)
	}
	all, err := store.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("got %#v", all)
	}
}

func TestLocalDirIsUnderTheConfigurationDirectory(t *testing.T) {
	if got := LocalDir("/tmp/conf"); got != filepath.Join("/tmp/conf", "packages") {
		t.Fatalf("got %q", got)
	}
}
