package commands

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mydevmachine/devmachine/internal/packages"
)

func TestMachinesListWithNoConfigurationIsEmpty(t *testing.T) {
	dir := t.TempDir()

	out, err := execute(t, "--config", dir, "--format", "json", "machines", "list")
	if err != nil {
		t.Fatalf("machines list returned %v (%s)", err, out)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("got %q, want an empty list", out)
	}

	out, err = execute(t, "--config", dir, "machines", "list")
	if err != nil {
		t.Fatalf("machines list returned %v (%s)", err, out)
	}
	if !strings.Contains(out, "devmachine setup") {
		t.Fatalf("the table does not say how to add the first machine: %q", out)
	}
}

func TestConfigShowWithNoConfigurationIsEmpty(t *testing.T) {
	dir := t.TempDir()

	out, err := execute(t, "--config", dir, "--format", "json", "config", "show")
	if err != nil {
		t.Fatalf("config show returned %v (%s)", err, out)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v (%q)", err, out)
	}
	if string(got["machines"]) != "[]" || string(got["workspaces"]) != "[]" {
		t.Fatalf("got %s", out)
	}

	out, err = execute(t, "--config", dir, "config", "show")
	if err != nil {
		t.Fatalf("config show returned %v (%s)", err, out)
	}
	if !strings.Contains(out, "no configuration") {
		t.Fatalf("the table does not say there is no configuration: %q", out)
	}
}

func TestConfigShowJSONListsAreNeverNull(t *testing.T) {
	dir := writeConfigDir(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n")

	out, err := execute(t, "--config", dir, "--format", "json", "config", "show")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"workspaces": []`) {
		t.Fatalf("an empty workspace list is not []: %s", out)
	}
}

func TestPackagesListWithNoConfigurationReadsTheLatestRelease(t *testing.T) {
	dir := t.TempDir()
	writeLocalPackage(t, dir, "docker", packages.ScopeMachine)
	t.Cleanup(swap(&latestPackagesRelease, func(context.Context) (string, error) { return "v9", nil }))
	var opened string
	t.Cleanup(swap(&openStore, func(ctx context.Context, configDir, version string) (*packages.Store, error) {
		opened = version
		return packages.Open(ctx, configDir, "")
	}))

	out, err := execute(t, "--config", dir, "--format", "json", "packages", "list")
	if err != nil {
		t.Fatalf("packages list returned %v (%s)", err, out)
	}
	if opened != "v9" {
		t.Fatalf("it opened release %q, want the latest, v9", opened)
	}
	var got struct {
		Release  string `json:"release"`
		Packages []struct {
			Name      string   `json:"name"`
			Installed []string `json:"installed_on"`
		} `json:"packages"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v (%q)", err, out)
	}
	if got.Release != "v9" || len(got.Packages) != 1 || got.Packages[0].Name != "docker" ||
		got.Packages[0].Installed == nil || len(got.Packages[0].Installed) != 0 {
		t.Fatalf("got %s", out)
	}
}

func TestPackagesListWithNoConfigurationAndNoNetworkSaysWhy(t *testing.T) {
	dir := t.TempDir()

	_, err := execute(t, "--config", dir, "packages", "list")
	if err == nil || !strings.Contains(err.Error(), "no configuration") ||
		!strings.Contains(err.Error(), "latest packages release") {
		t.Fatalf("got %v", err)
	}
}

func TestPackagesListReportsThePinnedRelease(t *testing.T) {
	dir := configWithPackagesInstalled(t)

	out, err := execute(t, "--config", dir, "--format", "json", "packages", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"release": ""`) {
		t.Fatalf("an unpinned configuration does not say so: %s", out)
	}
}

func TestMachinesListJSONCarriesEachMachinesPackages(t *testing.T) {
	dir := writeConfigDir(t, `machines:
  - name: main
    hosts: [203.0.113.10]
    packages: [essentials, docker]
  - name: bare
    hosts: [203.0.113.11]
`)

	out, err := execute(t, "--config", dir, "--format", "json", "machines", "list")
	if err != nil {
		t.Fatal(err)
	}
	var got []struct {
		Name     string   `json:"name"`
		Packages []string `json:"packages"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v (%q)", err, out)
	}
	if len(got) != 2 || strings.Join(got[0].Packages, ",") != "essentials,docker" {
		t.Fatalf("got %s", out)
	}
	if got[1].Packages == nil || len(got[1].Packages) != 0 {
		t.Fatalf("a machine with no packages is not []: %s", out)
	}
}
