package provision

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/packages"
)

// pin is the release every fixture store is opened at.
const pin = "v1"

// catalogue is the fixture set of recipes. It is small on purpose: `needs`
// deep enough to order (caddy -> firewall -> base), one extension point, one
// package that extends it, and one package nothing asks for.
var catalogue = map[string]string{
	"base": `format: 1
name: base
scope: machine
summary: The shared base every machine gets.
`,
	"firewall": `format: 1
name: firewall
scope: machine
summary: A firewall that denies by default.
needs: [base]
`,
	"docker": `format: 1
name: docker
scope: machine
summary: The container engine.
needs: [base]
`,
	"caddy": `format: 1
name: caddy
scope: machine
summary: A proxy with automatic certificates.
needs: [firewall]
provides:
  sites.d: /etc/caddy/sites.d
`,
	"acme": `format: 1
name: acme
scope: machine
summary: A second recipe that also reads an address, so two can collide.
`,
	"unused": `format: 1
name: unused
scope: machine
summary: A recipe in the store that nothing asks for.
`,
	"dev": `format: 1
name: dev
scope: workspace
summary: The GitHub CLI, and the login one account normally shares.
credentials:
  - name: gh
    kind: manual
    scope: machine
    shareable: true
    command: gh auth login
    stored_at: ~/.config/gh/hosts.yml
`,
	"vendor-cli": `format: 1
name: vendor-cli
scope: workspace
summary: A tool whose session is bound to the account that made it.
credentials:
  - name: vendor
    kind: manual
    scope: workspace
    command: vendor login
    stored_at: ~/.vendor/session
`,
	"claude-code": `format: 1
name: claude-code
scope: workspace
summary: Claude Code, logged in per workspace.
`,
	"global-skills": `format: 1
name: global-skills
scope: workspace
summary: Shared Agent Skills.
skills:
  path: skills
`,
	"sharing": `format: 1
name: sharing
scope: workspace
summary: File sharing behind the proxy.
extends:
  caddy.sites.d: files/sharing.caddy
`,
	"callable": `format: 1
name: callable
scope: machine
summary: A recipe that also ships an executable.
kind: dns
entrypoint: bin/provider
commands: ["*"]
`,
}

// extraFiles are the files a fixture recipe needs beyond its tasks.
var extraFiles = map[string]map[string]string{
	"sharing":  {"files/sharing.caddy": "example.com {\n  respond \"ok\"\n}\n"},
	"callable": {"bin/provider": "#!/usr/bin/env python3\n"},
	"global-skills": {
		"skills/workflow/SKILL.md": "---\nname: workflow\ndescription: Shared workflow.\n---\n\n# Workflow\n",
	},
}

// writeRecipe lays a fixture package out as a role: the manifest, tasks, and
// whatever else it declares.
func writeRecipe(t *testing.T, dir, name string) {
	t.Helper()
	write(t, filepath.Join(dir, "package.yml"), catalogue[name])
	write(t, filepath.Join(dir, "tasks", "main.yml"),
		"---\n- name: Install "+name+"\n  package: {name: "+name+", state: present}\n")
	for path, body := range extraFiles[name] {
		mode := fs.FileMode(0o644)
		if strings.HasPrefix(path, "bin/") {
			mode = 0o755
		}
		writeMode(t, filepath.Join(dir, path), body, mode)
	}
}

// storeWithCatalogue seeds the release cache with every fixture recipe, and
// the operator's own directory with the ones named in local.
//
// The cache is written rather than fetched: Fetch trusts a cache that carries
// its checksum, so no test needs the network.
func storeWithCatalogue(t *testing.T, local ...string) *packages.Store {
	t.Helper()
	configDir := t.TempDir()

	release := filepath.Join(packages.CacheDir(configDir, pin), "packages")
	for name := range catalogue {
		writeRecipe(t, filepath.Join(release, name), name)
	}
	write(t, filepath.Join(packages.CacheDir(configDir, pin), ".checksum"),
		"0000000000000000000000000000000000000000000000000000000000000000\n")

	for _, name := range local {
		writeRecipe(t, filepath.Join(packages.LocalDir(configDir), name), name)
	}

	store, err := packages.Open(context.Background(), configDir, pin)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// planFor resolves one machine and the workspaces on it against a store.
func planFor(t *testing.T, store *packages.Store, machineName string,
	machinePackages []string, workspacePackages map[string][]string) packages.MachinePlan {
	t.Helper()

	machine := config.Machine{
		Name:     machineName,
		Hosts:    []config.Host{{Address: "203.0.113.10"}},
		Port:     22,
		Packages: machinePackages,
	}
	cfg := config.Config{Machines: []config.Machine{machine}, Packages: pin}

	workspaceNames := make([]string, 0, len(workspacePackages))
	for name := range workspacePackages {
		workspaceNames = append(workspaceNames, name)
	}
	sort.Strings(workspaceNames)
	for _, name := range workspaceNames {
		cfg.Workspaces = append(cfg.Workspaces, config.Workspace{
			Name:     name,
			Machine:  machineName,
			Packages: workspacePackages[name],
		})
	}

	plan, err := packages.ResolveMachine(store, cfg, machine, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

// planWithSettings resolves a plan whose targets carry settings. The packages
// are read out of the settings themselves: a setting for a package the target
// does not install is refused by the configuration, so a fixture that does
// that would be testing something that cannot happen.
func planWithSettings(t *testing.T, machine map[string]any,
	workspaces map[string]map[string]any) packages.MachinePlan {
	t.Helper()

	target := config.Machine{
		Name:     "main",
		Hosts:    []config.Host{{Address: "203.0.113.10"}},
		Port:     22,
		Packages: packagesNamedBy(machine),
		Settings: machine,
	}
	cfg := config.Config{Machines: []config.Machine{target}, Packages: pin}
	for _, name := range slices.Sorted(maps.Keys(workspaces)) {
		cfg.Workspaces = append(cfg.Workspaces, config.Workspace{
			Name:     name,
			Machine:  target.Name,
			Packages: packagesNamedBy(workspaces[name]),
			Settings: workspaces[name],
		})
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}

	plan, err := packages.ResolveMachine(storeWithCatalogue(t), cfg, target, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

// packagesNamedBy is every package a set of settings mentions.
func packagesNamedBy(settings map[string]any) []string {
	seen := map[string]bool{}
	var out []string
	for _, key := range slices.Sorted(maps.Keys(settings)) {
		name, _, _ := strings.Cut(key, ".")
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

func planWith(t *testing.T, machineName string,
	machinePackages []string, workspacePackages map[string][]string) packages.MachinePlan {
	t.Helper()
	return planFor(t, storeWithCatalogue(t), machineName, machinePackages, workspacePackages)
}

// planWithLocalPackage resolves a plan where one of the machine's packages is
// shadowed by a copy in the operator's own directory.
func planWithLocalPackage(t *testing.T, machineName, name string) packages.MachinePlan {
	t.Helper()
	return planFor(t, storeWithCatalogue(t, name), machineName, []string{name}, nil)
}

// planWithExtension resolves a plan where a workspace package contributes to a
// place a machine package opened.
func planWithExtension(t *testing.T) packages.MachinePlan {
	t.Helper()
	return planWith(t, "main", []string{"caddy"}, map[string][]string{"alice": {"sharing"}})
}

// archiveNames lists an archive's entries. It reports an error rather than
// failing a test, because its caller is a fake with no testing.T to fail.
func archiveNames(r io.Reader) ([]string, error) {
	unzipped, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer unzipped.Close()

	var names []string
	archive := tar.NewReader(unzipped)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			return names, nil
		}
		if err != nil {
			return nil, err
		}
		names = append(names, header.Name)
	}
}
