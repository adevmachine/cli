package provision

import (
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/mydevmachine/devmachine/internal/packages"
	"gopkg.in/yaml.v3"
)

type routeCopyTask struct {
	Name string `yaml:"name"`
	Copy *struct {
		Owner string `yaml:"owner"`
		Group string `yaml:"group"`
		Mode  string `yaml:"mode"`
	} `yaml:"copy"`
	File *struct{} `yaml:"file"`
	Loop []struct {
		Src  string `yaml:"src"`
		Dest string `yaml:"dest"`
		Path string `yaml:"path"`
	} `yaml:"loop"`
}

func routeTasksOf(t *testing.T, site []byte) (copies, removals routeCopyTask) {
	t.Helper()
	var plays []struct {
		Tasks []routeCopyTask `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(site, &plays); err != nil {
		t.Fatal(err)
	}
	for _, task := range plays[0].Tasks {
		switch task.Name {
		case "routes from the configuration":
			copies = task
		case "files the configuration no longer owns":
			removals = task
		}
	}
	return copies, removals
}

func routesPlan(t *testing.T) packages.MachinePlan {
	t.Helper()
	plan := planWith(t, "main", []string{"caddy"}, map[string][]string{"alice": nil, "bob": nil})
	plan.SitesDir = "/etc/caddy/sites.d"
	plan.Routes = []packages.Route{
		{Workspace: "alice", Host: "app.example.com", Port: 8080},
		{Workspace: "alice", Host: "api.example.com", Port: 8081},
	}
	return plan
}

func TestWorkspaceRoutesIsByteForByteWhatSyncWrites(t *testing.T) {
	plan := routesPlan(t)
	files, err := Generate(plan)
	if err != nil {
		t.Fatal(err)
	}
	copies, _ := routeTasksOf(t, files["site.yml"])
	if copies.Copy == nil || len(copies.Loop) != 1 {
		t.Fatalf("the play has no routes copy: %+v", copies)
	}

	change, err := WorkspaceRoutes(plan, "alice")
	if err != nil {
		t.Fatal(err)
	}
	entry := copies.Loop[0]
	if change.Path != entry.Dest {
		t.Fatalf("fast path writes %s, sync writes %s", change.Path, entry.Dest)
	}
	bundled := files[strings.TrimPrefix(entry.Src, RemoteDir+"/")]
	if string(change.Content) != string(bundled) {
		t.Fatalf("fast path content differs from sync's:\n%s\n---\n%s", change.Content, bundled)
	}
	if change.Owner != copies.Copy.Owner || change.Group != copies.Copy.Group || change.Mode != copies.Copy.Mode {
		t.Fatalf("fast path gives %s:%s %s, sync gives %s:%s %s", change.Owner, change.Group, change.Mode,
			copies.Copy.Owner, copies.Copy.Group, copies.Copy.Mode)
	}
}

func TestWorkspaceRoutesRemovesTheOldOneHostFilesSyncRemoves(t *testing.T) {
	plan := routesPlan(t)
	files, err := Generate(plan)
	if err != nil {
		t.Fatal(err)
	}
	_, removals := routeTasksOf(t, files["site.yml"])
	var removed []string
	for _, e := range removals.Loop {
		removed = append(removed, e.Path)
	}

	change, err := WorkspaceRoutes(plan, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(change.Stale) != 2 {
		t.Fatalf("got %v", change.Stale)
	}
	for _, stale := range change.Stale {
		if !slices.Contains(removed, stale) {
			t.Fatalf("fast path removes %s, sync does not: %v", stale, removed)
		}
	}
}

func TestWorkspaceRoutesWithNoRouteLeftRemovesTheFile(t *testing.T) {
	plan := routesPlan(t)
	files, err := Generate(plan)
	if err != nil {
		t.Fatal(err)
	}
	_, removals := routeTasksOf(t, files["site.yml"])

	change, err := WorkspaceRoutes(plan, "bob")
	if err != nil {
		t.Fatal(err)
	}
	if change.Content != nil {
		t.Fatalf("bob has no route and still gets a file:\n%s", change.Content)
	}
	if change.Path != path.Join(plan.SitesDir, "bob-routes.caddy") {
		t.Fatal(change.Path)
	}
	found := false
	for _, e := range removals.Loop {
		found = found || e.Path == change.Path
	}
	if !found {
		t.Fatalf("sync does not remove %s", change.Path)
	}
}

func TestWorkspaceRoutesRefusesWithoutCaddyOrOnAnotherMachine(t *testing.T) {
	plan := routesPlan(t)
	if _, err := WorkspaceRoutes(plan, "carol"); err == nil {
		t.Fatal("carol is not on this machine")
	}
	plan.SitesDir = ""
	if _, err := WorkspaceRoutes(plan, "alice"); err == nil || !strings.Contains(err.Error(), "caddy") {
		t.Fatalf("got %v", err)
	}
}
