package provision

import (
	"flag"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func assertOrderInText(t *testing.T, text string, want ...string) {
	t.Helper()
	at := -1
	for _, s := range want {
		next := strings.Index(text, s)
		if next < 0 {
			t.Fatalf("%q is missing from:\n%s", s, text)
		}
		if next < at {
			t.Fatalf("%q comes too early in:\n%s", s, text)
		}
		at = next
	}
}

func TestGenerateWritesAnsibleCfgWithBothRolePaths(t *testing.T) {
	files, err := Generate(planWith(t, "main", []string{"base"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	cfg := string(files["ansible.cfg"])
	// The operator's own packages come first. That is the whole overlay
	// mechanism; there is no separate concept.
	if !strings.Contains(cfg, "roles_path = /opt/devmachine/roles.local:/opt/devmachine/roles") {
		t.Fatalf("got:\n%s", cfg)
	}
}

func TestGenerateWritesALocalInventory(t *testing.T) {
	files, err := Generate(planWith(t, "main", []string{"base"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	inventory := string(files["inventory.ini"])
	if !strings.Contains(inventory, "devmachine ansible_connection=local") {
		t.Fatalf("got:\n%s", inventory)
	}
	// A group named after the host makes Ansible warn on every single run.
	if strings.Contains(inventory, "[devmachine]") {
		t.Fatalf("the host has a group of its own name:\n%s", inventory)
	}
}

func TestGeneratePlaybookRunsMachinePackagesInOrder(t *testing.T) {
	plan := planWith(t, "main", []string{"caddy"}, nil) // caddy needs firewall needs base
	files, err := Generate(plan)
	if err != nil {
		t.Fatal(err)
	}
	assertOrderInText(t, string(files["site.yml"]), "base", "firewall", "caddy")
}

func TestGeneratePlaybookLoopsAWorkspacePackageOverItsWorkspacesOnly(t *testing.T) {
	plan := planWith(t, "main", nil, map[string][]string{
		"alice": {"claude-code"},
		"bob":   {},
	})
	files, err := Generate(plan)
	if err != nil {
		t.Fatal(err)
	}
	playbook := string(files["site.yml"])
	if !strings.Contains(playbook, "alice") {
		t.Fatalf("alice is missing:\n%s", playbook)
	}
	if strings.Contains(playbook, "bob") {
		t.Fatalf("bob declares nothing and should not appear:\n%s", playbook)
	}
}

func TestGenerateNamesTheLoopVariableRatherThanUsingItem(t *testing.T) {
	plan := planWith(t, "main", nil, map[string][]string{"alice": {"claude-code"}})
	files, err := Generate(plan)
	if err != nil {
		t.Fatal(err)
	}
	playbook := string(files["site.yml"])

	// A role is free to have loops of its own, and any of them rebinds `item`.
	// A workspace package written that way would silently act on whatever the
	// inner loop was iterating — it fails with "'AnsibleUnsafeText' object has
	// no attribute 'user'", which names nothing useful.
	if !strings.Contains(playbook, "loop_var: devmachine_workspace") {
		t.Fatalf("the loop variable is not named:\n%s", playbook)
	}
	if strings.Contains(playbook, `devmachine_workspace: "{{ item }}"`) {
		t.Fatalf("the workspace is still bound through item:\n%s", playbook)
	}
}

func TestGenerateTagsEveryPackageWithItsOwnName(t *testing.T) {
	files, err := Generate(planWith(t, "main", []string{"docker"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(files["site.yml"]), "tags: [docker]") {
		t.Fatalf("got:\n%s", files["site.yml"])
	}
}

// Without the apply block the inner tasks of a dynamic include_role do not
// inherit the tag, so `--tags docker` includes the role and then skips
// everything in it.
func TestGenerateAppliesTheTagInsideEveryIncludedRole(t *testing.T) {
	plan := planWith(t, "main", []string{"caddy"}, map[string][]string{"alice": {"claude-code"}})
	files, err := Generate(plan)
	if err != nil {
		t.Fatal(err)
	}

	for _, task := range tasksIn(t, files["site.yml"]) {
		include, ok := task["include_role"].(map[string]any)
		if !ok {
			continue
		}
		apply, ok := include["apply"].(map[string]any)
		if !ok {
			t.Fatalf("the role %v is included with no apply block: %#v", include["name"], task)
		}
		if !reflect.DeepEqual(apply["tags"], task["tags"]) {
			t.Fatalf("the role %v applies %#v but is tagged %#v", include["name"], apply["tags"], task["tags"])
		}
	}
}

// Every shipped recipe reads ansible_facts, so a playbook that turns fact
// gathering off breaks all of them.
func TestGenerateLeavesFactGatheringOn(t *testing.T) {
	files, err := Generate(planWith(t, "main", []string{"base"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(files["site.yml"]), "gather_facts") {
		t.Fatalf("the playbook says something about facts:\n%s", files["site.yml"])
	}
}

func TestGenerateWritesAnExtensionAsACopyIntoTheProvidedPath(t *testing.T) {
	plan := planWithExtension(t) // sharing extends caddy.sites.d
	files, err := Generate(plan)
	if err != nil {
		t.Fatal(err)
	}
	playbook := string(files["site.yml"])
	if !strings.Contains(playbook, "/etc/caddy/sites.d") {
		t.Fatalf("the extension does not write where caddy said:\n%s", playbook)
	}
	// An extension runs after the package it extends, or it writes into a
	// directory that does not exist yet.
	assertOrderInText(t, playbook, "caddy", "/etc/caddy/sites.d")
}

// Two workspaces installing the same package contribute the same file name,
// and one silently overwriting the other is the kind of defect nobody finds.
func TestGenerateKeepsTwoWorkspaceExtensionsApart(t *testing.T) {
	plan := planWith(t, "main", []string{"caddy"}, map[string][]string{
		"alice": {"sharing"},
		"bob":   {"sharing"},
	})
	files, err := Generate(plan)
	if err != nil {
		t.Fatal(err)
	}

	var destinations []string
	for _, task := range tasksIn(t, files["site.yml"]) {
		if copyTask, ok := task["copy"].(map[string]any); ok {
			destinations = append(destinations, copyTask["dest"].(string))
		}
	}
	if len(destinations) != 2 {
		t.Fatalf("got %v", destinations)
	}
	if destinations[0] == destinations[1] {
		t.Fatalf("both workspaces write to %s", destinations[0])
	}
}

// The source of an extension follows the copy that won, or a local override
// would run its own tasks and ship the published package's file.
func TestGenerateReadsAnExtensionFromTheCopyThatWon(t *testing.T) {
	store := storeWithCatalogue(t, "sharing")
	plan := planFor(t, store, "main", []string{"caddy"}, map[string][]string{"alice": {"sharing"}})

	files, err := Generate(plan)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(files["site.yml"]), "/opt/devmachine/roles.local/sharing/files/sharing.caddy") {
		t.Fatalf("got:\n%s", files["site.yml"])
	}
}

func TestGenerateHostVarsCarryTheWorkspaces(t *testing.T) {
	files, err := Generate(planWith(t, "main", nil, map[string][]string{"alice": {"claude-code"}}))
	if err != nil {
		t.Fatal(err)
	}
	vars := string(files["host_vars/devmachine.yml"])
	if !strings.Contains(vars, "alice") {
		t.Fatalf("got:\n%s", vars)
	}
}

// kind, entrypoint and commands are inert until v0.4. A generator that started
// treating them specially would be a surprise nobody asked for.
func TestGenerateTreatsACallablePackageAsAnOrdinaryRole(t *testing.T) {
	callable, err := Generate(planWith(t, "main", []string{"callable"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Generate(planWith(t, "main", []string{"unused"}, nil))
	if err != nil {
		t.Fatal(err)
	}

	got := strings.ReplaceAll(string(callable["site.yml"]), "callable", "unused")
	if got != string(plain["site.yml"]) {
		t.Fatalf("a callable package generates something else:\n%s", callable["site.yml"])
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	plan := planWith(t, "main", []string{"caddy", "docker"}, map[string][]string{"alice": {"claude-code"}})
	first, err := Generate(plan)
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		again, err := Generate(plan)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first, again) {
			t.Fatal("two runs produced different files; a diff nobody can read is a diff nobody reads")
		}
	}
}

func TestGenerateCarriesAMachineSetting(t *testing.T) {
	files, err := Generate(planWithSettings(t, map[string]any{"base.timezone": "UTC"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	vars := string(files["host_vars/devmachine.yml"])
	if !strings.Contains(vars, "devmachine_base_timezone: UTC") {
		t.Fatalf("got:\n%s", vars)
	}
}

func TestGenerateNamespacesASettingByItsPackage(t *testing.T) {
	// Two packages may both want `email`. Ansible has one variable namespace,
	// so the package name is what keeps them apart.
	files, err := Generate(planWithSettings(t, map[string]any{
		"caddy.email": "a@example.com", "acme.email": "b@example.com",
	}, nil))
	if err != nil {
		t.Fatal(err)
	}
	vars := string(files["host_vars/devmachine.yml"])
	for _, want := range []string{"devmachine_caddy_email:", "devmachine_acme_email:"} {
		if !strings.Contains(vars, want) {
			t.Fatalf("got:\n%s", vars)
		}
	}
}

func TestGenerateTurnsADashIntoAnUnderscore(t *testing.T) {
	// `claude-code` is a fine package name and an invalid Ansible variable.
	files, err := Generate(planWithSettings(t, nil,
		map[string]map[string]any{"alice": {"claude-code.model": "x"}}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(files["site.yml"]), "devmachine_claude_code_model: x") {
		t.Fatalf("got:\n%s", files["site.yml"])
	}
}

func TestGenerateKeepsOnlyTheFirstDotAsThePackageName(t *testing.T) {
	// A package is free to use a dotted name of its own, and the whole of it
	// is still one Ansible variable.
	files, err := Generate(planWithSettings(t, nil,
		map[string]map[string]any{"alice": {"claude-code.marketplace.url": "example.com"}}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(files["site.yml"]), "devmachine_claude_code_marketplace_url: example.com") {
		t.Fatalf("got:\n%s", files["site.yml"])
	}
}

func TestGenerateCarriesASettingThatIsNotAString(t *testing.T) {
	files, err := Generate(planWithSettings(t, map[string]any{
		"firewall.http":     false,
		"firewall.ports":    []string{"80", "443"},
		"firewall.maxretry": 5,
	}, nil))
	if err != nil {
		t.Fatal(err)
	}
	vars := string(files["host_vars/devmachine.yml"])
	for _, want := range []string{
		"devmachine_firewall_http: false",
		"devmachine_firewall_maxretry: 5",
		"- \"80\"",
	} {
		if !strings.Contains(vars, want) {
			t.Fatalf("%q is missing from:\n%s", want, vars)
		}
	}
}

func TestGenerateGivesEachWorkspaceItsOwnSettings(t *testing.T) {
	// alice and bob may install the same package with different values, and
	// the loop has to carry each one's.
	plan := planWithSettings(t, nil, map[string]map[string]any{
		"alice": {"claude-code.model": "one"},
		"bob":   {"claude-code.model": "two"},
	})
	files, err := Generate(plan)
	if err != nil {
		t.Fatal(err)
	}

	playbook := string(files["site.yml"])
	for _, task := range tasksIn(t, files["site.yml"]) {
		vars, ok := task["vars"].(map[string]any)
		if !ok || vars["devmachine_claude_code_model"] == nil {
			continue
		}
		loop, ok := task["loop"].([]any)
		if !ok || len(loop) != 1 {
			t.Fatalf("a setting reached %d workspaces at once:\n%s", len(loop), playbook)
		}
		who := loop[0].(map[string]any)["name"]
		want := map[any]string{"alice": "one", "bob": "two"}[who]
		if vars["devmachine_claude_code_model"] != want {
			t.Fatalf("%v got %v, not %q:\n%s", who, vars["devmachine_claude_code_model"], want, playbook)
		}
	}
}

func TestGenerateLeavesAWorkspaceWithoutSettingsInTheSharedLoop(t *testing.T) {
	// Only a workspace that overrides something needs a task of its own;
	// everybody else keeps sharing one.
	plan := planWith(t, "main", nil, map[string][]string{
		"alice": {"claude-code"},
		"bob":   {"claude-code"},
	})
	plan.Workspaces[0].Target.Settings = map[string]any{"claude-code.model": "one"}

	files, err := Generate(plan)
	if err != nil {
		t.Fatal(err)
	}
	playbook := string(files["site.yml"])
	if !strings.Contains(playbook, "claude-code for each workspace that declares it") {
		t.Fatalf("bob lost the shared loop:\n%s", playbook)
	}
	if strings.Count(playbook, "include_role") != 2 {
		t.Fatalf("got:\n%s", playbook)
	}
}

func TestGenerateRefusesTwoSettingsWithTheSameVariableName(t *testing.T) {
	// `a-b` and `a.b` are two settings and one Ansible variable. Letting one
	// win silently is the failure this whole feature exists to prevent.
	_, err := Generate(planWithSettings(t, map[string]any{
		"base.a-b": "one",
		"base.a.b": "two",
	}, nil))
	if err == nil {
		t.Fatal("two settings collapsed into one variable without a word")
	}
	if !strings.Contains(err.Error(), "devmachine_base_a_b") {
		t.Fatalf("the error does not name the variable: %v", err)
	}
}

func TestGenerateMatchesTheGoldenFiles(t *testing.T) {
	plan := planWith(t, "main", []string{"caddy", "docker"}, map[string][]string{
		"alice": {"claude-code", "sharing"},
		"bob":   {},
	})
	plan.Machine.Settings = map[string]any{"caddy.email": "someone@example.com"}
	plan.Workspaces[0].Target.Settings = map[string]any{"claude-code.plugins": []string{"one", "two"}}

	files, err := Generate(plan)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range slices.Sorted(maps.Keys(files)) {
		golden := filepath.Join("testdata", "generate", name)
		if *update {
			write(t, golden, string(files[name]))
			continue
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != string(files[name]) {
			t.Errorf("%s changed:\n--- want ---\n%s\n--- got ---\n%s", name, want, files[name])
		}
	}
}

// tasksIn parses the generated playbook, which also proves it is YAML.
func tasksIn(t *testing.T, playbook []byte) []map[string]any {
	t.Helper()
	var plays []struct {
		Hosts  string           `yaml:"hosts"`
		Become bool             `yaml:"become"`
		Tasks  []map[string]any `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(playbook, &plays); err != nil {
		t.Fatalf("the playbook is not YAML: %v\n%s", err, playbook)
	}
	if len(plays) != 1 {
		t.Fatalf("got %d plays", len(plays))
	}
	if plays[0].Hosts != "devmachine" || !plays[0].Become {
		t.Fatalf("got hosts %q, become %v", plays[0].Hosts, plays[0].Become)
	}
	return plays[0].Tasks
}
