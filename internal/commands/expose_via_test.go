package commands

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/dns"
	"github.com/mydevmachine/devmachine/internal/expose"
	"github.com/mydevmachine/devmachine/internal/packages"
	"github.com/mydevmachine/devmachine/internal/remote"
)

// configWithEdge is a machine the internet reaches, with caddy, and one only
// a private network reaches, without it: alice lives on the second.
func configWithEdge(t *testing.T, routes string) string {
	t.Helper()
	dir := configWith(t, "machines:\n"+
		"  - name: edge\n    hosts: [203.0.113.10]\n    packages: [caddy]\n"+
		"  - name: lab\n    hosts: [100.64.0.7]\n"+
		"workspaces:\n  - name: alice\n    machine: lab\n"+routes)
	writeCaddyPackage(t, dir)
	return dir
}

// dialMachines answers each machine with its own client, so a test sees which
// machine a command acted on.
func dialMachines(t *testing.T, clients map[string]*exposeClient) {
	t.Helper()
	stub := func(_ context.Context, m config.Machine, _ string) (remote.Client, string, error) {
		c, ok := clients[m.Name]
		if !ok {
			return nil, "", errors.New("no route to host")
		}
		return c, "", nil
	}
	origDial, origMux := dial, dialMux
	dial, dialMux = stub, stub
	t.Cleanup(func() { dial, dialMux = origDial, origMux })
}

func addJSON(t *testing.T, dir string, args ...string) exposeResult {
	t.Helper()
	out, err := execute(t, append([]string{"--config", dir, "--format", "json", "expose", "add"}, args...)...)
	if err != nil {
		t.Fatal(err, out)
	}
	var got exposeResult
	if err := json.Unmarshal([]byte(lastJSON(out)), &got); err != nil {
		t.Fatal(err, out)
	}
	return got
}

func TestExposeAddViaPublishesOnTheOtherMachine(t *testing.T) {
	edge, lab := &exposeClient{}, &exposeClient{}
	dialMachines(t, map[string]*exposeClient{"edge": edge, "lab": lab})
	dir := configWithEdge(t, "")

	got := addJSON(t, dir, "alice", "8080", "--host", "app.example.com", "--via", "edge", "--publish")

	if !got.Applied || got.Status != "published" || got.Via != "edge" || got.Note != "" {
		t.Fatalf("%+v", got)
	}
	if body := edge.files["alice-routes.caddy"]; !strings.Contains(body, "reverse_proxy 100.64.0.7:8080") {
		t.Fatalf("edge's routes file:\n%s", body)
	}
	if len(lab.scripts) != 0 {
		t.Fatal("nothing is written on lab, which has no caddy")
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, r, _ := cfg.RouteOwner("app.example.com"); r.Via != "edge" {
		t.Fatalf("recorded %+v", r)
	}
}

func TestExposeAddWithoutCaddyNamesTheMachinesThatHaveIt(t *testing.T) {
	dialMachines(t, map[string]*exposeClient{"edge": {}, "lab": {}})
	dir := configWithEdge(t, "")

	_, err := execute(t, "--config", dir, "expose", "add", "alice", "8080", "--host", "app.example.com", "--publish")
	if err == nil || !strings.Contains(err.Error(), "--via edge") {
		t.Fatalf("want the --via the configuration offers, got %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, config.FileName))
	if strings.Contains(string(body), "app.example.com") {
		t.Fatal("a refused route must not be recorded")
	}
}

func TestExposeAddViaARefusedMachineSaysWhy(t *testing.T) {
	dialMachines(t, map[string]*exposeClient{"edge": {}, "lab": {}})
	dir := configWithEdge(t, "")

	_, err := execute(t, "--config", dir, "expose", "add", "alice", "8080", "--host", "app.example.com",
		"--via", "lab", "--publish")
	if err == nil || !strings.Contains(err.Error(), "its own machine") {
		t.Fatalf("got %v", err)
	}
}

func TestExposeAddViaSaysWhenTheOtherMachineCannotReachThePort(t *testing.T) {
	edge := &exposeClient{probe: "unreachable"}
	dialMachines(t, map[string]*exposeClient{"edge": edge})
	dir := configWithEdge(t, "")

	got := addJSON(t, dir, "alice", "8080", "--host", "app.example.com", "--via", "edge", "--publish")

	if got.Status != "published" || !strings.Contains(got.Note, "100.64.0.7:8080") {
		t.Fatalf("the site is on Caddy, and the note says what does not answer: %+v", got)
	}
}

func TestExposeAddViaWithNoAddressForTheMachineStaysPending(t *testing.T) {
	edge := &exposeClient{}
	dialMachines(t, map[string]*exposeClient{"edge": edge})
	dir := configWith(t, "machines:\n"+
		"  - name: edge\n    hosts: [203.0.113.10]\n    packages: [caddy]\n"+
		"  - name: lab\n    hosts: [127.0.0.1]\n"+
		"workspaces:\n  - name: alice\n    machine: lab\n")
	writeCaddyPackage(t, dir)

	got := addJSON(t, dir, "alice", "8080", "--host", "app.example.com", "--via", "edge", "--publish")

	if got.Applied || got.Status != "pending" || !strings.Contains(got.Note, "lab") {
		t.Fatalf("%+v", got)
	}
	if len(edge.scripts) != 0 {
		t.Fatal("nothing can be written without lab's address")
	}
}

func TestExposeRmTakesTheSiteOffTheMachineThatServesIt(t *testing.T) {
	edge := &exposeClient{files: map[string]string{
		"alice-routes.caddy": expose.RenderWorkspace("alice",
			[]expose.Site{{Host: "app.example.com", Port: 8080, Upstream: "100.64.0.7"}}),
	}}
	lab := &exposeClient{}
	dialMachines(t, map[string]*exposeClient{"edge": edge, "lab": lab})
	dir := configWithEdge(t, "    routes: [{host: app.example.com, port: 8080, via: edge}]\n")

	if out, err := execute(t, "--config", dir, "expose", "rm", "app.example.com", "--yes"); err != nil {
		t.Fatal(err, out)
	}
	if _, ok := edge.files["alice-routes.caddy"]; ok {
		t.Fatal("the routes file is still on edge")
	}
	if len(lab.scripts) != 0 {
		t.Fatal("lab serves nothing and must not be touched")
	}
}

func TestExposeListOnTheServingMachineShowsRoutesSentThere(t *testing.T) {
	edge := &exposeClient{files: map[string]string{
		"alice-routes.caddy": expose.RenderWorkspace("alice",
			[]expose.Site{{Host: "app.example.com", Port: 8080, Upstream: "100.64.0.7"}}),
	}}
	dialMachines(t, map[string]*exposeClient{"edge": edge})
	dir := configWithEdge(t, "    routes: [{host: app.example.com, port: 8080, via: edge}]\n")

	out, err := execute(t, "--config", dir, "--machine", "edge", "--format", "json", "expose", "list")
	if err != nil {
		t.Fatal(err, out)
	}
	var rows []site
	if err := json.Unmarshal([]byte(out[strings.Index(out, "["):]), &rows); err != nil {
		t.Fatal(err, out)
	}
	if len(rows) != 1 || rows[0].Status != "published" || rows[0].From != "lab" {
		t.Fatalf("%+v", rows)
	}
}

func TestExposeListSaysWhenTheServingMachineProxiesToAnOldAddress(t *testing.T) {
	edge := &exposeClient{files: map[string]string{
		"alice-routes.caddy": expose.RenderWorkspace("alice",
			[]expose.Site{{Host: "app.example.com", Port: 8080, Upstream: "100.64.0.99"}}),
	}}
	dialMachines(t, map[string]*exposeClient{"edge": edge})
	dir := configWithEdge(t, "    routes: [{host: app.example.com, port: 8080, via: edge}]\n")

	out, err := execute(t, "--config", dir, "--machine", "edge", "--format", "json", "expose", "list")
	if err != nil {
		t.Fatal(err, out)
	}
	if !strings.Contains(out, `"differs"`) || !strings.Contains(out, "100.64.0.7") {
		t.Fatalf("an old address must read as differs and name the new one:\n%s", out)
	}
}

// makeCaddyARole gives the fixture caddy package the tasks a sync checks
// every local package for.
func makeCaddyARole(t *testing.T, dir string) {
	t.Helper()
	tasks := filepath.Join(packages.LocalDir(dir), "caddy", "tasks")
	if err := os.MkdirAll(tasks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tasks, "main.yml"), []byte("---\n[]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSyncOfTheServingMachineCarriesTheOtherMachinesAddress(t *testing.T) {
	stub := stubSync(t)
	dir := configWithEdge(t, "    routes: [{host: app.example.com, port: 8080, via: edge}]\n")
	makeCaddyARole(t, dir)

	if out, err := execute(t, "--config", dir, "--machine", "edge", "sync", "--yes"); err != nil {
		t.Fatal(err, out)
	}
	if len(stub.plan.Routes) != 1 || stub.plan.Routes[0].Upstream != "100.64.0.7" || stub.plan.Routes[0].From != "lab" {
		t.Fatalf("%+v", stub.plan.Routes)
	}
}

func TestSyncSaysWhichSiteItLeavesAloneWithoutAnAddress(t *testing.T) {
	stubSync(t)
	dir := configWith(t, "machines:\n"+
		"  - name: edge\n    hosts: [203.0.113.10]\n    packages: [caddy]\n"+
		"  - name: lab\n    hosts: [127.0.0.1]\n"+
		"workspaces:\n  - name: alice\n    machine: lab\n"+
		"    routes: [{host: app.example.com, port: 8080, via: edge}]\n")
	writeCaddyPackage(t, dir)
	makeCaddyARole(t, dir)

	out, err := execute(t, "--config", dir, "--machine", "edge", "sync", "--yes")
	if err != nil {
		t.Fatal(err, out)
	}
	if !strings.Contains(out, "app.example.com") || !strings.Contains(out, "left as it is") {
		t.Fatalf("the plan must say the site is left alone and why:\n%s", out)
	}
}

func dialDestroyMachines(t *testing.T, clients map[string]*destroyClient) {
	t.Helper()
	orig := dial
	dial = func(_ context.Context, m config.Machine, _ string) (remote.Client, string, error) {
		c, ok := clients[m.Name]
		if !ok {
			return nil, "", errors.New("no route to host")
		}
		return c, "", nil
	}
	t.Cleanup(func() { dial = orig })
}

func TestWorkspacesDestroyTakesItsSitesOffTheMachineThatServesThem(t *testing.T) {
	edge, lab := &destroyClient{}, &destroyClient{userExists: true}
	dialDestroyMachines(t, map[string]*destroyClient{"edge": edge, "lab": lab})
	dir := configWithEdge(t, "    routes: [{host: app.example.com, port: 8080, via: edge}]\n")

	if out, err := execute(t, "--config", dir, "workspaces", "destroy", "alice", "--confirm", "alice"); err != nil {
		t.Fatal(err, out)
	}
	if !strings.Contains(lab.lastScript, "userdel") || strings.Contains(lab.lastScript, "routes.caddy") {
		t.Fatalf("lab removes the account and has no routes file:\n%s", lab.lastScript)
	}
	if !strings.Contains(edge.lastScript, "alice-routes.caddy") || !strings.Contains(edge.lastScript, "reload caddy") {
		t.Fatalf("edge must drop alice's routes file:\n%s", edge.lastScript)
	}
}

func TestWorkspacesDestroyRefusesWhileTheServingMachineIsOutOfReach(t *testing.T) {
	lab := &destroyClient{userExists: true}
	dialDestroyMachines(t, map[string]*destroyClient{"lab": lab})
	dir := configWithEdge(t, "    routes: [{host: app.example.com, port: 8080, via: edge}]\n")

	_, err := execute(t, "--config", dir, "workspaces", "destroy", "alice", "--confirm", "alice")
	if err == nil || !strings.Contains(err.Error(), "edge") || !strings.Contains(err.Error(), "expose rm") {
		t.Fatalf("got %v", err)
	}
	if lab.lastScript != "" {
		t.Fatal("nothing may be destroyed while a site would be left behind")
	}
}

func TestWorkspacesRmRefusesWhileAnotherMachineServesItsSites(t *testing.T) {
	dir := configWithEdge(t, "    routes: [{host: app.example.com, port: 8080, via: edge}]\n")

	_, err := execute(t, "--config", dir, "workspaces", "rm", "alice", "--yes")
	if err == nil || !strings.Contains(err.Error(), "expose rm app.example.com") {
		t.Fatalf("got %v", err)
	}
}

func TestWorkspacesDestroyStopsWhenTheServingMachineKeepsTheFile(t *testing.T) {
	edge, lab := &destroyClient{runErr: errors.New("sudo: a password is required")}, &destroyClient{userExists: true}
	dialDestroyMachines(t, map[string]*destroyClient{"edge": edge, "lab": lab})
	dir := configWithEdge(t, "    routes: [{host: app.example.com, port: 8080, via: edge}]\n")

	_, err := execute(t, "--config", dir, "workspaces", "destroy", "alice", "--confirm", "alice")
	if err == nil || !strings.Contains(err.Error(), "edge") {
		t.Fatalf("got %v", err)
	}
	if lab.lastScript != "" {
		t.Fatal("the account must stay while edge still serves its site")
	}
}

func TestExposeListReadsALoopbackProxyForASiteFromElsewhereAsDiffering(t *testing.T) {
	edge := &exposeClient{files: map[string]string{
		"alice-routes.caddy": expose.RenderWorkspace("alice", []expose.Site{{Host: "app.example.com", Port: 8080}}),
	}}
	dialMachines(t, map[string]*exposeClient{"edge": edge})
	dir := configWithEdge(t, "    routes: [{host: app.example.com, port: 8080, via: edge}]\n")
	orig := upstreamOf
	upstreamOf = func(context.Context, config.Machine) (string, error) {
		return "", errors.New("tailscale is not running")
	}
	t.Cleanup(func() { upstreamOf = orig })

	out, err := execute(t, "--config", dir, "--machine", "edge", "expose", "list")
	if err != nil {
		t.Fatal(err, out)
	}
	if !strings.Contains(out, "differs") || !strings.Contains(out, "from lab") {
		t.Fatalf("a site from lab proxied to edge itself differs, and the row names lab:\n%s", out)
	}
}

func TestExposeAddKeepsAnErrorThatIsNotAboutCaddy(t *testing.T) {
	dialMachines(t, map[string]*exposeClient{"edge": {}, "lab": {}})
	dir := configWith(t, "machines:\n"+
		"  - name: edge\n    hosts: [203.0.113.10]\n    packages: [caddy]\n"+
		"  - name: lab\n    hosts: [100.64.0.7]\n    packages: [nonexistent]\n"+
		"workspaces:\n  - name: alice\n    machine: lab\n")
	writeCaddyPackage(t, dir)

	_, err := execute(t, "--config", dir, "expose", "add", "alice", "8080", "--host", "app.example.com", "--publish")
	if err == nil || strings.Contains(err.Error(), "--via") {
		t.Fatalf("an error that is not a missing caddy must not suggest --via, got %v", err)
	}
}

func TestSyncBlamesEachSiteOnItsOwnMachine(t *testing.T) {
	stubSync(t)
	dir := configWith(t, "machines:\n"+
		"  - name: edge\n    hosts: [203.0.113.10]\n    packages: [caddy]\n"+
		"  - name: lab\n    hosts: [127.0.0.1]\n"+
		"  - name: lab2\n    hosts: [127.0.0.2]\n"+
		"workspaces:\n"+
		"  - name: alice\n    machine: lab\n    routes: [{host: a.example.com, port: 1, via: edge}]\n"+
		"  - name: bob\n    machine: lab2\n    routes: [{host: b.example.com, port: 2, via: edge}]\n")
	writeCaddyPackage(t, dir)
	makeCaddyARole(t, dir)

	out, err := execute(t, "--config", dir, "--machine", "edge", "sync", "--yes")
	if err != nil {
		t.Fatal(err, out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "no address of lab2") && !strings.Contains(line, "b.example.com") {
			t.Fatalf("lab2's failure is shown apart from lab2's own site:\n%s", out)
		}
	}
}

func TestTheNameGetsThePublicAddressWhenAPrivateOneComesFirst(t *testing.T) {
	for _, tc := range []struct {
		in   []string
		want string
	}{
		{[]string{"100.64.0.7", "203.0.113.10"}, "203.0.113.10"},
		{[]string{"192.168.1.4", "10.0.0.2", "198.51.100.3"}, "198.51.100.3"},
		{[]string{"fd7a::1", "203.0.113.10"}, "203.0.113.10"},
		{[]string{"100.64.0.7"}, "100.64.0.7"},
		{[]string{"vps.example.com", "100.64.0.7"}, "vps.example.com"},
	} {
		got, private := publicAddress(tc.in)
		if got != tc.want {
			t.Fatalf("%v: got %q", tc.in, got)
		}
		if want := tc.want == "100.64.0.7"; private != want {
			t.Fatalf("%v: private %v", tc.in, private)
		}
	}
}

func TestExposeAddStillPublishesWhenTheProviderRefusesTheRecord(t *testing.T) {
	client := &exposeClient{zones: []string{"example.com"}, upsertErr: errors.New("exit status 1")}
	dialExpose(t, client)
	dir := configWithCaddy(t)
	writeDNSPackage(t, dir, "hostinger", nil, "print('ok')")
	lockOnto(t, dir, "main", "hostinger")

	out, err := execute(t, "--config", dir, "--format", "json", "expose", "add", "alice", "8080",
		"--host", "app.example.com", "--publish")
	if err != nil {
		t.Fatalf("a refused record must not stop the publish: %v\n%s", err, out)
	}
	var got exposeResult
	if err := json.Unmarshal([]byte(lastJSON(out)), &got); err != nil {
		t.Fatal(err, out)
	}
	if got.Status != "published" || !got.Applied {
		t.Fatalf("Caddy still gets the route: %+v", got)
	}
	if !strings.Contains(out, "Create this record by hand") || !strings.Contains(out, "203.0.113.10") {
		t.Fatalf("the output must say what to create by hand:\n%s", out)
	}
	if !strings.Contains(got.DNSError, "hostinger refused it") {
		t.Fatalf("the JSON must say the record was not written: %+v", got)
	}
	if !strings.Contains(out, "app (in example.com)\tA\t203.0.113.10") {
		t.Fatalf("the record is named inside the provider's zone:\n%s", out)
	}
	if !strings.Contains(out, "hostinger") {
		t.Fatalf("and which provider refused, and why:\n%s", out)
	}
}

func TestExposeRmViaRemovesTheRecordThatPointsAtTheServingMachine(t *testing.T) {
	edge := &exposeClient{zones: []string{"example.com"},
		records: []dns.Record{{Name: "app", Type: "A", Value: "203.0.113.10"}}}
	lab := &exposeClient{zones: []string{"example.com"},
		records: []dns.Record{{Name: "app", Type: "A", Value: "100.64.0.7"}}}
	dialMachines(t, map[string]*exposeClient{"edge": edge, "lab": lab})
	dir := configWithEdge(t, "    routes: [{host: app.example.com, port: 8080, via: edge}]\n")
	writeDNSPackage(t, dir, "hostinger", nil, "print('ok')")
	lockOnto(t, dir, "edge", "hostinger")

	if out, err := execute(t, "--config", dir, "expose", "rm", "app.example.com", "--yes"); err != nil {
		t.Fatal(err, out)
	}
	if len(edge.deleted) != 1 || !strings.Contains(edge.deleted[0], `"value":"203.0.113.10"`) {
		t.Fatalf("the record pointing at edge is not the one deleted: %q", edge.deleted)
	}
	if len(lab.deleted) != 0 || lab.lists != 0 {
		t.Fatal("lab serves nothing, so its address is never the record's")
	}
}

func TestExposeRmRemovesTheRecordOfThePublicAddressWhenAPrivateOneComesFirst(t *testing.T) {
	client := &exposeClient{zones: []string{"example.com"},
		records: []dns.Record{{Name: "app", Type: "A", Value: "203.0.113.10"}}}
	dialExpose(t, client)
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [100.64.0.7, 203.0.113.10]\n    packages: [caddy]\n"+
		"workspaces:\n  - name: alice\n    routes: [{host: app.example.com, port: 8080}]\n")
	writeCaddyPackage(t, dir)
	writeDNSPackage(t, dir, "hostinger", nil, "print('ok')")
	lockOnto(t, dir, "main", "hostinger")

	if out, err := execute(t, "--config", dir, "expose", "rm", "app.example.com", "--yes"); err != nil {
		t.Fatal(err, out)
	}
	if len(client.deleted) != 1 || !strings.Contains(client.deleted[0], `"value":"203.0.113.10"`) {
		t.Fatalf("%q", client.deleted)
	}
}

func TestWorkspacesDestroyRemovesTheRecordThatPointsAtTheServingMachine(t *testing.T) {
	edge := &exposeClient{zones: []string{"example.com"},
		records: []dns.Record{{Name: "app", Type: "A", Value: "203.0.113.10"}}}
	lab := &exposeClient{}
	dialMachines(t, map[string]*exposeClient{"edge": edge, "lab": lab})
	dir := configWithEdge(t, "    routes: [{host: app.example.com, port: 8080, via: edge}]\n")
	writeDNSPackage(t, dir, "hostinger", nil, "print('ok')")
	lockOnto(t, dir, "edge", "hostinger")

	if out, err := execute(t, "--config", dir, "workspaces", "destroy", "alice", "--confirm", "alice"); err != nil {
		t.Fatal(err, out)
	}
	if len(edge.deleted) != 1 || !strings.Contains(edge.deleted[0], `"value":"203.0.113.10"`) {
		t.Fatalf("the record pointing at edge is not the one deleted: %q", edge.deleted)
	}
}
