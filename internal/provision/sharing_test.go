package provision

import (
	"strings"
	"testing"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/packages"
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

	copies := sharedTasks(t, files["site.yml"], "shell")
	if len(copies) != 1 {
		t.Fatalf("got %d tasks copying the shared login, want 1:\n%s", len(copies), files["site.yml"])
	}
	// The copy overwrites stored_at. Reaching bob would destroy the account he
	// logged into, on the next sync, with nothing saying why.
	users := loopUsers(t, copies[0])
	if len(users) != 1 || users[0] != "alice" {
		t.Fatalf("the shared login reached %#v:\n%s", users, files["site.yml"])
	}
}

func TestGenerateCopiesASharedLoginToEveryWorkspaceThatWantsIt(t *testing.T) {
	files, err := Generate(planSharing(t))
	if err != nil {
		t.Fatal(err)
	}

	copies := sharedTasks(t, files["site.yml"], "shell")
	if len(copies) != 1 {
		t.Fatalf("got %d tasks, want 1:\n%s", len(copies), files["site.yml"])
	}
	users := loopUsers(t, copies[0])
	if len(users) != 2 || users[0] != "alice" || users[1] != "bob" {
		t.Fatalf("got %#v", users)
	}
	for _, entry := range copies[0]["loop"].([]any) {
		if got := asString(entry.(map[string]any)["path"]); got != ".config/gh/hosts.yml" {
			t.Fatalf("the path inside the home is %q", got)
		}
	}

	script := asString(copies[0]["shell"])
	if !strings.Contains(script, "< '/etc/devmachine/gh/hosts.yml'") {
		t.Fatalf("the master copy is not what the copy reads:\n%s", script)
	}
}

func TestGenerateNeverLetsRootWriteIntoAWorkspaceHome(t *testing.T) {
	files, err := Generate(planSharing(t))
	if err != nil {
		t.Fatal(err)
	}

	// A workspace owns its home and can put a link anywhere in it. Root's
	// file or copy module would follow that link out of the home.
	for _, module := range []string{"file", "copy"} {
		if found := sharedTasks(t, files["site.yml"], module); len(found) != 0 {
			t.Fatalf("a %s task runs as root inside a workspace home: %#v", module, found)
		}
	}
	copies := sharedTasks(t, files["site.yml"], "shell")
	if len(copies) != 1 {
		t.Fatalf("got %d tasks, want 1:\n%s", len(copies), files["site.yml"])
	}
	if script := asString(copies[0]["shell"]); !strings.Contains(script, `runuser -u "$user" --`) {
		t.Fatalf("the copy does not run as the workspace account:\n%s", script)
	}
	if copies[0]["changed_when"] == nil {
		t.Fatalf("a copy that changed nothing has to say so: %#v", copies[0])
	}
}

func TestSharedHomePathRefusesAPathOutsideTheHome(t *testing.T) {
	for _, storedAt := range []string{"/var/lib/tool/state", "~/../other/state", "~/"} {
		if _, err := sharedHomePath(storedAt); err == nil {
			t.Fatalf("%q was accepted", storedAt)
		}
	}
	got, err := sharedHomePath("~/.config/gh/hosts.yml")
	if err != nil || got != ".config/gh/hosts.yml" {
		t.Fatalf("got %q, %v", got, err)
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

	for _, task := range sharedTasks(t, files["site.yml"], "shell") {
		if condition, ok := task["when"].(string); !ok || !strings.Contains(condition, "stat.exists") {
			t.Fatalf("the copy runs whether or not the login is there: %#v", task)
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
