package provision

import (
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/packages"
)

// sharedTasks are the generated tasks that put one login where a workspace
// expects it: the directories on the way, and the copy itself.
func sharedTasks(t *testing.T, playbook []byte, module string) []map[string]any {
	t.Helper()

	var out []map[string]any
	for _, task := range tasksIn(t, playbook) {
		if _, ok := task[module]; !ok {
			continue
		}
		if !strings.Contains(asString(task["name"]), "gh login") {
			continue
		}
		out = append(out, task)
	}
	return out
}

func asString(value any) string {
	s, _ := value.(string)
	return s
}

// loopUsers is every account one task's loop touches.
func loopUsers(t *testing.T, task map[string]any) []string {
	t.Helper()

	entries, ok := task["loop"].([]any)
	if !ok {
		t.Fatalf("the task has no loop: %#v", task)
	}
	var out []string
	for _, entry := range entries {
		item, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("a loop entry is not a mapping: %#v", entry)
		}
		out = append(out, asString(item["user"]))
	}
	return out
}

func planSharing(t *testing.T, optOut ...string) packages.MachinePlan {
	t.Helper()

	plan := planWith(t, "main", nil, map[string][]string{
		"alice": {"dev"},
		"bob":   {"dev"},
	})
	for _, name := range optOut {
		for i := range plan.Workspaces {
			if plan.Workspaces[i].Target.Name == name {
				plan.Workspaces[i].Target.Credentials = map[string]string{"gh": config.CredentialOwn}
			}
		}
	}
	return plan
}

func TestGenerateSkipsAWorkspaceThatOptedOutOfASharedLogin(t *testing.T) {
	files, err := Generate(planSharing(t, "bob"))
	if err != nil {
		t.Fatal(err)
	}

	copies := sharedTasks(t, files["site.yml"], "copy")
	if len(copies) != 1 {
		t.Fatalf("got %d tasks copying the shared login, want 1:\n%s", len(copies), files["site.yml"])
	}
	// The copy overwrites stored_at. Reaching bob would destroy the account he
	// logged into, on the next sync, with nothing saying why.
	users := loopUsers(t, copies[0])
	if len(users) != 1 || users[0] != "alice" {
		t.Fatalf("the shared login reached %#v:\n%s", users, files["site.yml"])
	}
	if strings.Contains(string(files["site.yml"]), "~bob/.config/gh") {
		t.Fatalf("bob is named on the way to the shared login:\n%s", files["site.yml"])
	}
}

func TestGenerateCopiesASharedLoginToEveryWorkspaceThatWantsIt(t *testing.T) {
	files, err := Generate(planSharing(t))
	if err != nil {
		t.Fatal(err)
	}

	copies := sharedTasks(t, files["site.yml"], "copy")
	if len(copies) != 1 {
		t.Fatalf("got %d tasks, want 1:\n%s", len(copies), files["site.yml"])
	}
	users := loopUsers(t, copies[0])
	if len(users) != 2 || users[0] != "alice" || users[1] != "bob" {
		t.Fatalf("got %#v", users)
	}

	options := copies[0]["copy"].(map[string]any)
	if options["src"] != "/etc/devmachine/gh/hosts.yml" {
		t.Fatalf("the master copy is %q", options["src"])
	}
	if options["remote_src"] != true {
		t.Fatalf("the copy is between two places on the machine: %#v", options)
	}
	if options["mode"] != "0600" {
		t.Fatalf("a login is readable by its owner alone, got %v", options["mode"])
	}
	if options["owner"] != "{{ devmachine_shared.user }}" || options["group"] != "{{ devmachine_shared.user }}" {
		t.Fatalf("got owner %v group %v", options["owner"], options["group"])
	}
}

func TestGenerateOwnsEveryDirectoryOnTheWayToASharedLogin(t *testing.T) {
	files, err := Generate(planSharing(t, "bob"))
	if err != nil {
		t.Fatal(err)
	}

	directories := sharedTasks(t, files["site.yml"], "file")
	if len(directories) != 1 {
		t.Fatalf("got %d tasks making the directories, want 1:\n%s", len(directories), files["site.yml"])
	}

	entries, ok := directories[0]["loop"].([]any)
	if !ok {
		t.Fatalf("the task has no loop: %#v", directories[0])
	}
	// Root making ~/.config on the way would leave a root-owned directory in
	// a workspace's home, and that workspace's own tools would start failing
	// for a reason nobody would connect to this. Every level is named.
	var paths []string
	for _, entry := range entries {
		paths = append(paths, asString(entry.(map[string]any)["path"]))
	}
	if len(paths) != 2 || paths[0] != "~alice/.config" || paths[1] != "~alice/.config/gh" {
		t.Fatalf("got %#v", paths)
	}

	options := directories[0]["file"].(map[string]any)
	if options["owner"] != "{{ devmachine_shared.user }}" || options["state"] != "directory" {
		t.Fatalf("got %#v", options)
	}
}

func TestGenerateWaitsForTheLoginBeforeCopyingIt(t *testing.T) {
	files, err := Generate(planSharing(t))
	if err != nil {
		t.Fatal(err)
	}

	// Nobody can automate the login, so a sync before it has happened is
	// ordinary. It has to skip the copy, not fail the run.
	looked := 0
	for _, task := range tasksIn(t, files["site.yml"]) {
		if options, ok := task["stat"].(map[string]any); ok &&
			options["path"] == "/etc/devmachine/gh/hosts.yml" {
			looked++
		}
	}
	if looked != 1 {
		t.Fatalf("got %d tasks looking for the master copy, want 1:\n%s", looked, files["site.yml"])
	}

	for _, module := range []string{"copy", "file"} {
		for _, task := range sharedTasks(t, files["site.yml"], module) {
			if condition, ok := task["when"].(string); !ok || !strings.Contains(condition, "stat.exists") {
				t.Fatalf("a %s task runs whether or not the login is there: %#v", module, task)
			}
		}
	}
}

func TestGenerateRefusesToShareALoginThatCannotTravel(t *testing.T) {
	plan := planWith(t, "main", nil, map[string][]string{"alice": {"vendor-cli"}})
	plan.Workspaces[0].Target.Credentials = map[string]string{"vendor": config.CredentialMachine}

	if _, err := Generate(plan); err == nil {
		t.Fatal("generating a copy that would not work has to be refused")
	}
}
