package commands

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mydevmachine/devmachine/internal/packages"
)

func stubLatestCLI(t *testing.T, tag string, err error) *int {
	t.Helper()
	calls := 0
	previous := latestCLIRelease
	latestCLIRelease = func(context.Context) (string, error) {
		calls++
		return tag, err
	}
	t.Cleanup(func() { latestCLIRelease = previous })
	return &calls
}

func runningVersion(t *testing.T, v string) {
	t.Helper()
	SetVersion(v)
	t.Cleanup(func() { SetVersion("dev") })
}

func pinnedConfig(t *testing.T, release string) string {
	t.Helper()
	dir := configWithTrustedKey(t, "packages: "+release+"\n")
	writeCommandFile(t, filepath.Join(packages.CacheDir(dir, release), ".checksum"), "fixture\n")
	return dir
}

func TestDoctorWarnsWhenTheCLIAndThePinAreBehind(t *testing.T) {
	runningVersion(t, "0.7.17")
	stubLatestCLI(t, "v0.7.18", nil)
	defer stubLatestPackagesRelease(t, "v17")()
	dir := pinnedConfig(t, "v16")
	answering(t, "ID=ubuntu")

	out, err := execute(t, "--config", dir, "doctor")
	if err != nil {
		t.Fatalf("a warning failed doctor: %v\n%s", err, out)
	}
	cli := lineWith(t, out, "  cli ")
	pin := lineWith(t, out, "packages pin")
	for _, line := range []string{cli, pin} {
		if !strings.HasPrefix(line, "warn") || !strings.Contains(line, "devmachine update") {
			t.Fatalf("not a warning with the fix: %q", line)
		}
	}
	if !strings.Contains(pin, "v16") || !strings.Contains(pin, "v17") {
		t.Fatalf("the pin check does not name both releases: %q", pin)
	}
}

func TestDoctorPassesWhenEverythingIsLatest(t *testing.T) {
	runningVersion(t, "0.7.18")
	stubLatestCLI(t, "v0.7.18", nil)
	defer stubLatestPackagesRelease(t, "v17")()
	dir := pinnedConfig(t, "v17")
	answering(t, "ID=ubuntu")

	out, err := execute(t, "--config", dir, "doctor")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, name := range []string{"  cli ", "packages pin"} {
		if line := lineWith(t, out, name); !strings.HasPrefix(line, "pass") {
			t.Fatalf("not a pass: %q", line)
		}
	}
}

func TestDoctorSkipsTheFreshnessChecksOffline(t *testing.T) {
	runningVersion(t, "0.7.17")
	stubLatestCLI(t, "", errors.New("no route to host"))
	previous := latestPackagesRelease
	latestPackagesRelease = func(context.Context) (string, error) { return "", errors.New("no route to host") }
	t.Cleanup(func() { latestPackagesRelease = previous })
	dir := pinnedConfig(t, "v16")
	answering(t, "ID=ubuntu")

	out, err := execute(t, "--config", dir, "doctor")
	if err != nil {
		t.Fatalf("being offline failed doctor: %v\n%s", err, out)
	}
	for _, name := range []string{"  cli ", "packages pin"} {
		if line := lineWith(t, out, name); !strings.HasPrefix(line, "skip") {
			t.Fatalf("not a skip: %q", line)
		}
	}
}

func TestDoctorRemembersTheLatestVersions(t *testing.T) {
	runningVersion(t, "0.7.18")
	calls := stubLatestCLI(t, "v0.7.18", nil)
	cache := t.TempDir()
	previous := releaseCacheDir
	releaseCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { releaseCacheDir = previous })
	defer stubLatestPackagesRelease(t, "v17")()
	dir := pinnedConfig(t, "v17")
	answering(t, "ID=ubuntu")

	for range 2 {
		if out, err := execute(t, "--config", dir, "doctor"); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}
	if *calls != 1 {
		t.Fatalf("asked GitHub %d times; the second doctor should read the cache", *calls)
	}
}
