package commands

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/dns"
	"github.com/mydevmachine/devmachine/internal/expose"
	"github.com/mydevmachine/devmachine/internal/packages"
	"github.com/mydevmachine/devmachine/internal/remote"
)

// writeCaddyPackage writes a fixture caddy package that declares the same
// sites.d extension point the real one does, so `expose` can find it without
// touching the packages repository.
func writeCaddyPackage(t *testing.T, configDir string) {
	t.Helper()
	pkgDir := filepath.Join(packages.LocalDir(configDir), "caddy")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "format: 1\nname: caddy\nscope: machine\nsummary: A reverse proxy.\n" +
		"provides:\n  sites.d: /etc/caddy/sites.d\n"
	if err := os.WriteFile(packages.ManifestPath(pkgDir), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

// configWithCaddy is a one-machine, one-workspace configuration with caddy on
// the machine's own package list, the ordinary state after `packages add
// caddy` and `sync`.
func configWithCaddy(t *testing.T) string {
	t.Helper()
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [caddy]\n"+
		"workspaces:\n  - name: alice\n")
	writeCaddyPackage(t, dir)
	return dir
}

// configWithoutCaddy is the same shape, minus caddy: the state before it was
// ever added.
func configWithoutCaddy(t *testing.T) string {
	t.Helper()
	return configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n"+
		"workspaces:\n  - name: alice\n")
}

// exposeClient is a remote.Client a test drives by hand: it answers `expose`'s
// own shell (mkdir/cat/ls/rm/systemctl) and, when asked, a DNS provider's
// entrypoint (zones/list/upsert/delete), without ever touching a real machine.
type exposeClient struct {
	files map[string]string // sites.d file name -> its content
	// zones is what a locked DNS provider claims to hold, empty meaning no
	// provider answers at all.
	zones   []string
	upserts int
	lastCmd string
	// scripts is every script `expose` sent to apply a routes file, and
	// verdict what the machine answers them; empty means it applied.
	scripts []string
	verdict string
	// probe is what the machine answers when asked whether it reaches
	// another machine's port; empty means it does.
	probe string
	// upsertErr is what the DNS provider answers a write with, nil when it
	// takes it.
	upsertErr error
	// records is what the DNS provider lists for its zone, and lists how many
	// times it was asked.
	records []dns.Record
	lists   int
	listErr error
	// deleted is every record the DNS provider was asked to delete, as the
	// command line carried it; deleteErr is what it answers with.
	deleted   []string
	deleteErr error
}

var (
	encodedContent = regexp.MustCompile(`printf '%s' '([A-Za-z0-9+/=]*)'`)
	routesTarget   = regexp.MustCompile(`restore\(\) \{ rm -f '[^']+' '([^']+)'`)
)

// apply does to files what the script asks, as far as a test cares: the
// content it carries lands in the routes file, or the file goes.
func (c *exposeClient) apply(script string) (string, error) {
	c.scripts = append(c.scripts, script)
	verdict := c.verdict
	if verdict == "" {
		verdict = "devmachine-apply: applied"
	}
	if verdict != "devmachine-apply: applied" {
		return verdict + "\n", nil
	}
	if c.files == nil {
		c.files = map[string]string{}
	}
	name := filepath.Base(routesTarget.FindStringSubmatch(script)[1])
	if strings.Contains(script, "base64 -d") {
		m := encodedContent.FindStringSubmatch(script)
		body, err := base64.StdEncoding.DecodeString(m[1])
		if err != nil {
			return "", err
		}
		c.files[name] = string(body)
	} else {
		delete(c.files, name)
	}
	return verdict + "\n", nil
}

// applied is the content the last script carried, or "" when it removed the
// file.
func (c *exposeClient) applied(t *testing.T) string {
	t.Helper()
	if len(c.scripts) == 0 {
		t.Fatal("nothing was applied")
	}
	m := encodedContent.FindStringSubmatch(c.scripts[len(c.scripts)-1])
	if m == nil {
		return ""
	}
	body, err := base64.StdEncoding.DecodeString(m[1])
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func (c *exposeClient) Run(_ context.Context, command string) (string, error) {
	c.lastCmd = command
	switch {
	case strings.Contains(command, "ls -1 "):
		names := make([]string, 0, len(c.files))
		for name := range c.files {
			names = append(names, name)
		}
		return strings.Join(names, "\n"), nil
	case strings.HasPrefix(strings.TrimSpace(command), "cat "):
		for name, body := range c.files {
			if strings.Contains(command, name) {
				return body, nil
			}
		}
		if strings.Contains(command, "|| true") {
			return "", nil
		}
		return "", fmt.Errorf("no such file: %s", command)
	case strings.Contains(command, "rm -f "):
		for name := range c.files {
			if strings.Contains(command, name) {
				delete(c.files, name)
			}
		}
		return "", nil
	case strings.Contains(command, "/dev/tcp/"):
		if c.probe == "" {
			return "reachable\n", nil
		}
		return c.probe + "\n", nil
	case strings.HasSuffix(strings.TrimSpace(command), " zones"):
		body, _ := json.Marshal(struct {
			Zones []string `json:"zones"`
		}{c.zones})
		return string(body), nil
	case strings.Contains(command, " list "):
		c.lists++
		if c.listErr != nil {
			return `{"error":{"kind":"unauthenticated","message":"request failed (HTTP 403)"}}`, c.listErr
		}
		body, _ := json.Marshal(struct {
			Records []dns.Record `json:"records"`
		}{c.records})
		return string(body), nil
	case strings.Contains(command, " delete "):
		c.deleted = append(c.deleted, command)
		if c.deleteErr != nil {
			return `{"error":{"kind":"forbidden","message":"request failed (HTTP 403)"}}`, c.deleteErr
		}
		return "{}", nil
	case strings.Contains(command, " upsert "):
		c.upserts++
		if c.upsertErr != nil {
			return `{"error":{"kind":"unauthenticated","message":"request failed (HTTP 403)"}}`, c.upsertErr
		}
		return "{}", nil
	default:
		return "", nil
	}
}

func (c *exposeClient) RunInput(ctx context.Context, command string, stdin io.Reader) (string, error) {
	c.lastCmd = command
	body, err := io.ReadAll(stdin)
	if err != nil {
		return "", err
	}
	if command == expose.ApplyCommand {
		return c.apply(string(body))
	}
	// writeSite's script is `mkdir -p <dir> && cat > <path> && systemctl reload caddy`.
	if strings.Contains(command, "cat > ") {
		if c.files == nil {
			c.files = map[string]string{}
		}
		base := filepath.Base(strings.Fields(strings.Split(command, "cat > ")[1])[0])
		c.files[base] = string(body)
		return "", nil
	}
	return c.Run(ctx, command)
}

func (c *exposeClient) Stream(ctx context.Context, command string, stdout, _ io.Writer) error {
	out, err := c.Run(ctx, command)
	if _, werr := io.WriteString(stdout, out); werr != nil {
		return werr
	}
	return err
}

func (c *exposeClient) Upload(context.Context, string, io.Reader) error { return nil }
func (c *exposeClient) Close() error                                    { return nil }

func dialExpose(t *testing.T, client *exposeClient) {
	t.Helper()
	stub := func(context.Context, config.Machine, string) (remote.Client, string, error) {
		return client, "203.0.113.10", nil
	}
	origDial, origMux := dial, dialMux
	dial, dialMux = stub, stub
	t.Cleanup(func() { dial, dialMux = origDial, origMux })
}

func machineDown(t *testing.T) {
	t.Helper()
	stub := func(context.Context, config.Machine, string) (remote.Client, string, error) {
		return nil, "", errors.New("no route to host")
	}
	origDial, origMux := dial, dialMux
	dial, dialMux = stub, stub
	t.Cleanup(func() { dial, dialMux = origDial, origMux })
}

func TestExposeAddWarnsThatAnybodyCanReachIt(t *testing.T) {
	dialExpose(t, &exposeClient{})
	dir := configWithCaddy(t)

	out, err := executeWithInput(t, "n\n", "--config", dir, "expose", "add", "alice", "8080", "--host", "app.example.com")
	if err == nil {
		t.Fatal("declining should not publish anything")
	}
	// An admin panel speaks HTTP and still must not be published. The
	// question is the last place to say so.
	if !strings.Contains(out, "anybody") {
		t.Fatalf("the question does not say who can reach it: %q", out)
	}
}

func TestExposeAddYesDoesNotPublish(t *testing.T) {
	client := &exposeClient{}
	dialExpose(t, client)
	dir := configWithCaddy(t)

	out, err := execute(t, "--config", dir, "expose", "add", "alice", "8080",
		"--host", "app.example.com", "--yes")
	if !errors.Is(err, errDeclined) {
		t.Fatalf("expose add --yes returned %v, want a declined publication", err)
	}
	if len(client.files) != 0 {
		t.Fatalf("--yes published files: %#v", client.files)
	}
	if !strings.Contains(out, "[y/N]") {
		t.Fatalf("--yes skipped the publication question: %q", out)
	}
}

func TestExposeAddRefusesWhenCaddyIsNotInstalled(t *testing.T) {
	dialExpose(t, &exposeClient{})
	dir := configWithoutCaddy(t)

	_, err := executeWithInput(t, "", "--config", dir, "expose", "add", "alice", "8080",
		"--host", "app.example.com", "--yes")
	if err == nil {
		t.Fatal("it wrote a Caddy block on a machine with no Caddy")
	}
	if !strings.Contains(err.Error(), "caddy") {
		t.Fatalf("the error does not name what is missing: %v", err)
	}
}

func TestExposeAddSaysWhatToCreateWhenNobodyHoldsTheZone(t *testing.T) {
	// With no DNS provider installed, it prints the record rather than
	// refusing to publish.
	client := &exposeClient{}
	dialExpose(t, client)
	dir := configWithCaddy(t)

	out, err := execute(t, "--config", dir, "expose", "add", "alice", "8080",
		"--host", "app.example.com", "--publish")
	if err != nil {
		t.Fatalf("expose add returned %v", err)
	}
	if !strings.Contains(out, "app.example.com") || !strings.Contains(out, "A") {
		t.Fatalf("it did not print the record to create by hand: %q", out)
	}
}

func TestExposeAddOffersToCreateTheRecord(t *testing.T) {
	// The name has to point at the machine before Caddy can get a
	// certificate for it. Failing later, in a log, is the alternative.
	client := &exposeClient{zones: []string{"example.com"}}
	dialExpose(t, client)
	dir := configWithCaddy(t)
	writeDNSPackage(t, dir, "hostinger", nil, "print('ok')")
	lockOnto(t, dir, "main", "hostinger")

	if _, err := execute(t, "--config", dir, "expose", "add", "alice", "8080",
		"--host", "app.example.com", "--publish"); err != nil {
		t.Fatalf("expose add returned %v", err)
	}
	if client.upserts != 1 {
		t.Fatalf("the record was not written through the installed provider: %d upserts", client.upserts)
	}
}

func TestExposeAddRefusesAHostThatWouldEscapeTheFileName(t *testing.T) {
	dialExpose(t, &exposeClient{})
	dir := configWithCaddy(t)

	_, err := execute(t, "--config", dir, "expose", "add", "alice", "8080", "--host", "../evil", "--yes")
	if err == nil {
		t.Fatal("expected an error for an unusable hostname")
	}
}

func TestExposeListReportsEveryDisagreement(t *testing.T) {
	client := &exposeClient{files: map[string]string{
		"alice-routes.caddy":    expose.RenderWorkspace("alice", []expose.Site{{Host: "app.example.com", Port: 8080}}),
		"old.example.com.caddy": expose.Render(expose.Site{Host: "old.example.com", Port: 3000, Workspace: "bob"}),
		"bob-routes.caddy":      expose.RenderWorkspace("bob", []expose.Site{{Host: "moved.example.com", Port: 1}}),
	}}
	dialExpose(t, client)
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [caddy]\n"+
		"workspaces:\n  - name: alice\n    routes: [{host: app.example.com, port: 8080}, {host: new.example.com, port: 8082}]\n"+
		"  - name: bob\n    routes: [{host: moved.example.com, port: 2}]\n")
	writeCaddyPackage(t, dir)

	out, err := execute(t, "--config", dir, "--format", "json", "expose", "list")
	if err != nil {
		t.Fatal(err)
	}
	var rows []site
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatal(err, out)
	}
	got := map[string]site{}
	for _, r := range rows {
		got[r.Host] = r
	}
	if got["app.example.com"].Status != "published" {
		t.Fatalf("%+v", got["app.example.com"])
	}
	if r := got["new.example.com"]; r.Status != "pending" || !strings.Contains(r.Note, "devmachine sync") {
		t.Fatalf("%+v", r)
	}
	if r := got["old.example.com"]; r.Status != "unmanaged" || !strings.Contains(r.Note, "expose add bob 3000 --host old.example.com") {
		t.Fatalf("%+v", r)
	}
	if r := got["moved.example.com"]; r.Status != "differs" || !strings.Contains(r.Note, "machine has port 1") {
		t.Fatalf("%+v", r)
	}
}

func TestExposeListWithTheMachineDownStillListsTheConfiguration(t *testing.T) {
	machineDown(t)
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [caddy]\n"+
		"workspaces:\n  - name: alice\n    routes: [{host: app.example.com, port: 8080}]\n")
	writeCaddyPackage(t, dir)

	out, err := execute(t, "--config", dir, "expose", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "app.example.com") || !strings.Contains(out, "unknown") {
		t.Fatalf("%q", out)
	}
}

func TestExposeListSaysNothingWhenBothSidesAreEmpty(t *testing.T) {
	dialExpose(t, &exposeClient{})
	dir := configWithCaddy(t)
	out, err := execute(t, "--config", dir, "expose", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Nothing is published") {
		t.Fatalf("%q", out)
	}
}

func TestExposeAddNoApplyRecordsTheRouteAndTouchesNoMachine(t *testing.T) {
	client := &exposeClient{}
	dialExpose(t, client)
	dir := configWithCaddy(t)

	out, err := execute(t, "--config", dir, "expose", "add", "alice", "8080",
		"--host", "app.example.com", "--publish", "--no-apply")
	if err != nil {
		t.Fatal(err)
	}
	if len(client.files) != 0 || len(client.scripts) != 0 {
		t.Fatalf("add wrote on the machine: %v", client.files)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	w, r, ok := cfg.RouteOwner("app.example.com")
	if !ok || w.Name != "alice" || r.Port != 8080 {
		t.Fatalf("route not recorded: %v %+v", ok, r)
	}
	if !strings.Contains(out, "devmachine sync") {
		t.Fatalf("the answer must say sync publishes it: %q", out)
	}
}

func TestExposeAddAppliesTheRouteToCaddyWithoutASync(t *testing.T) {
	client := &exposeClient{}
	dialExpose(t, client)
	dir := configWithCaddy(t)

	out, err := execute(t, "--config", dir, "expose", "add", "alice", "8080",
		"--host", "app.example.com", "--publish")
	if err != nil {
		t.Fatal(err)
	}
	want := expose.RenderWorkspace("alice", []expose.Site{{Host: "app.example.com", Port: 8080, Workspace: "alice"}})
	if got := client.applied(t); got != want {
		t.Fatalf("applied\n%s\nwant what sync writes\n%s", got, want)
	}
	if !strings.Contains(client.scripts[0], "'/etc/caddy/sites.d/alice-routes.caddy'") {
		t.Fatalf("the script does not write alice's routes file:\n%s", client.scripts[0])
	}
	if !strings.Contains(out, "published: https://app.example.com -> alice:8080") {
		t.Fatalf("%q", out)
	}
	if strings.Contains(out, "Run `devmachine sync`") {
		t.Fatalf("a published route needs no sync: %q", out)
	}

	listed, err := execute(t, "--config", dir, "--format", "json", "expose", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listed, `"status": "published"`) {
		t.Fatalf("expose list after the fast path: %s", listed)
	}
}

func TestExposeAddKeepsTheOtherRoutesOfTheWorkspace(t *testing.T) {
	client := &exposeClient{}
	dialExpose(t, client)
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [caddy]\n"+
		"workspaces:\n  - name: alice\n    routes: [{host: api.example.com, port: 8081}]\n")
	writeCaddyPackage(t, dir)

	if _, err := execute(t, "--config", dir, "expose", "add", "alice", "8080",
		"--host", "app.example.com", "--publish"); err != nil {
		t.Fatal(err)
	}
	got := client.applied(t)
	if !strings.Contains(got, "api.example.com {") || !strings.Contains(got, "app.example.com {") {
		t.Fatalf("the file must carry every route of alice:\n%s", got)
	}
}

func TestExposeAddThatCaddyRefusesStaysPending(t *testing.T) {
	client := &exposeClient{verdict: "Error: ambiguous site definition: app.example.com\ndevmachine-apply: invalid"}
	dialExpose(t, client)
	dir := configWithCaddy(t)

	_, err := execute(t, "--config", dir, "expose", "add", "alice", "8080",
		"--host", "app.example.com", "--publish")
	if err == nil {
		t.Fatal("a refused file must not read as published")
	}
	for _, want := range []string{"ambiguous site definition", "pending", "devmachine sync"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in %v", want, err)
		}
	}
	cfg, _ := config.Load(dir)
	if _, _, ok := cfg.RouteOwner("app.example.com"); !ok {
		t.Fatal("the route must stay recorded, to fix and sync")
	}
}

func TestExposeAddOnAMachineWhoseCaddyIsNotInstalledYetStaysPending(t *testing.T) {
	client := &exposeClient{verdict: "devmachine-apply: no-caddy"}
	dialExpose(t, client)
	dir := configWithCaddy(t)

	out, err := execute(t, "--config", dir, "expose", "add", "alice", "8080",
		"--host", "app.example.com", "--publish")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "pending") || !strings.Contains(out, "devmachine sync") {
		t.Fatalf("%q", out)
	}
}

func TestExposeAddJSONSaysWhetherItApplied(t *testing.T) {
	dialExpose(t, &exposeClient{})
	dir := configWithCaddy(t)

	out, err := execute(t, "--config", dir, "--format", "json", "expose", "add", "alice", "8080",
		"--host", "app.example.com", "--publish")
	if err != nil {
		t.Fatal(err)
	}
	var got exposeResult
	if err := json.Unmarshal([]byte(lastJSON(out)), &got); err != nil {
		t.Fatal(err, out)
	}
	if !got.Applied || got.Status != "published" || got.Host != "app.example.com" || got.Workspace != "alice" {
		t.Fatalf("%+v", got)
	}
}

func TestExposeAddJSONWithTheMachineDownSaysPending(t *testing.T) {
	machineDown(t)
	dir := configWithCaddy(t)

	out, err := execute(t, "--config", dir, "--format", "json", "expose", "add", "alice", "8080",
		"--host", "app.example.com", "--publish")
	if err != nil {
		t.Fatal(err)
	}
	var got exposeResult
	if err := json.Unmarshal([]byte(lastJSON(out)), &got); err != nil {
		t.Fatal(err, out)
	}
	if got.Applied || got.Status != "pending" {
		t.Fatalf("%+v", got)
	}
}

// lastJSON is the JSON document at the end of the combined output, after
// whatever the command said on stderr.
func lastJSON(out string) string {
	return out[strings.LastIndex(out, "{\n"):]
}

func TestExposeAddStillPointsTheName(t *testing.T) {
	client := &exposeClient{zones: []string{"example.com"}}
	dialExpose(t, client)
	dir := configWithCaddy(t)
	writeDNSPackage(t, dir, "hostinger", nil, "print('ok')")
	lockOnto(t, dir, "main", "hostinger")

	if _, err := execute(t, "--config", dir, "expose", "add", "alice", "8080",
		"--host", "app.example.com", "--publish"); err != nil {
		t.Fatal(err)
	}
	if client.upserts != 1 {
		t.Fatalf("the name was not pointed: %d upserts", client.upserts)
	}
}

func TestExposeAddRecordsEvenWhenTheMachineIsDown(t *testing.T) {
	machineDown(t)
	dir := configWithCaddy(t)

	out, err := execute(t, "--config", dir, "expose", "add", "alice", "8080",
		"--host", "app.example.com", "--publish")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.Load(dir)
	if _, _, ok := cfg.RouteOwner("app.example.com"); !ok {
		t.Fatal("an unreachable machine must not lose the route")
	}
	if !strings.Contains(out, "203.0.113.10") {
		t.Fatalf("the record to create by hand is missing: %q", out)
	}
	if !strings.Contains(out, "pending") || !strings.Contains(out, "devmachine sync") {
		t.Fatalf("it must say the route waits for a sync: %q", out)
	}
}

func TestExposeAddRefusesWithoutCaddy(t *testing.T) {
	dialExpose(t, &exposeClient{})
	dir := configWithoutCaddy(t)
	_, err := execute(t, "--config", dir, "expose", "add", "alice", "8080",
		"--host", "app.example.com", "--publish")
	if err == nil || !strings.Contains(err.Error(), "caddy") {
		t.Fatalf("got %v", err)
	}
	cfg, _ := config.Load(dir)
	if _, _, ok := cfg.RouteOwner("app.example.com"); ok {
		t.Fatal("a refused add must record nothing")
	}
}

func TestExposeAddCheckWritesNothing(t *testing.T) {
	client := &exposeClient{}
	dialExpose(t, client)
	dir := configWithCaddy(t)
	out, err := execute(t, "--config", dir, "expose", "add", "alice", "8080",
		"--host", "app.example.com", "--check")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "would ") {
		t.Fatal(out)
	}
	cfg, _ := config.Load(dir)
	if _, _, ok := cfg.RouteOwner("app.example.com"); ok {
		t.Fatal("--check recorded a route")
	}
	if len(client.scripts) != 0 {
		t.Fatal("--check applied something")
	}
}

func TestExposeAddCheckShowsTheConfigurationAndTheCaddyFile(t *testing.T) {
	client := &exposeClient{files: map[string]string{
		"alice-routes.caddy": expose.RenderWorkspace("alice", []expose.Site{{Host: "api.example.com", Port: 8081}}),
	}}
	dialExpose(t, client)
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [caddy]\n"+
		"workspaces:\n  - name: alice\n    routes: [{host: api.example.com, port: 8081}]\n")
	writeCaddyPackage(t, dir)

	out, err := execute(t, "--config", dir, "expose", "add", "alice", "8080",
		"--host", "app.example.com", "--check")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"config.yml",
		"+ {host: app.example.com, port: 8080}",
		"/etc/caddy/sites.d/alice-routes.caddy",
		" api.example.com {",
		"+app.example.com {",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestExposeRmTakesTheRouteOffCaddyWithoutASync(t *testing.T) {
	client := &exposeClient{files: map[string]string{
		"alice-routes.caddy": expose.RenderWorkspace("alice", []expose.Site{{Host: "app.example.com", Port: 8080}}),
	}}
	dialExpose(t, client)
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [caddy]\n"+
		"workspaces:\n  - name: alice\n    routes: [{host: app.example.com, port: 8080}]\n")
	writeCaddyPackage(t, dir)

	out, err := execute(t, "--config", dir, "expose", "rm", "app.example.com", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if got := client.applied(t); got != "" {
		t.Fatalf("alice has no route left, yet the file carries:\n%s", got)
	}
	if _, ok := client.files["alice-routes.caddy"]; ok {
		t.Fatal("the routes file is still on the machine")
	}
	if !strings.Contains(out, "no longer published") {
		t.Fatalf("%q", out)
	}
}

func TestExposeRmKeepsTheOtherRoutesOfTheWorkspace(t *testing.T) {
	client := &exposeClient{}
	dialExpose(t, client)
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [caddy]\n"+
		"workspaces:\n  - name: alice\n    routes: [{host: app.example.com, port: 8080}, {host: api.example.com, port: 8081}]\n")
	writeCaddyPackage(t, dir)

	if _, err := execute(t, "--config", dir, "expose", "rm", "app.example.com", "--yes"); err != nil {
		t.Fatal(err)
	}
	want := expose.RenderWorkspace("alice", []expose.Site{{Host: "api.example.com", Port: 8081, Workspace: "alice"}})
	if got := client.applied(t); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestExposeRmCheckShowsBothSidesAndChangesNothing(t *testing.T) {
	client := &exposeClient{files: map[string]string{
		"alice-routes.caddy": expose.RenderWorkspace("alice", []expose.Site{{Host: "app.example.com", Port: 8080}}),
	}}
	dialExpose(t, client)
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [caddy]\n"+
		"workspaces:\n  - name: alice\n    routes: [{host: app.example.com, port: 8080}]\n")
	writeCaddyPackage(t, dir)

	out, err := execute(t, "--config", dir, "expose", "rm", "app.example.com", "--check")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"- {host: app.example.com, port: 8080}", "-app.example.com {", "removed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if len(client.scripts) != 0 {
		t.Fatal("--check applied something")
	}
	cfg, _ := config.Load(dir)
	if _, _, ok := cfg.RouteOwner("app.example.com"); !ok {
		t.Fatal("--check forgot the route")
	}
}

func TestExposeRmNoApplyForgetsTheRouteAndTouchesNoMachine(t *testing.T) {
	client := &exposeClient{files: map[string]string{"alice-routes.caddy": "app.example.com {\n}\n"}}
	dialExpose(t, client)
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [caddy]\n"+
		"workspaces:\n  - name: alice\n    routes: [{host: app.example.com, port: 8080}]\n")
	writeCaddyPackage(t, dir)

	out, err := execute(t, "--config", dir, "expose", "rm", "app.example.com", "--yes", "--no-apply")
	if err != nil {
		t.Fatal(err)
	}
	if len(client.files) != 1 || len(client.scripts) != 0 {
		t.Fatal("rm touched the machine")
	}
	cfg, _ := config.Load(dir)
	if _, _, ok := cfg.RouteOwner("app.example.com"); ok {
		t.Fatal("the route is still recorded")
	}
	if !strings.Contains(out, "devmachine sync") {
		t.Fatalf("%q", out)
	}
}

func TestExposeRmWithTheMachineDownSaysItIsStillServed(t *testing.T) {
	machineDown(t)
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [caddy]\n"+
		"workspaces:\n  - name: alice\n    routes: [{host: app.example.com, port: 8080}]\n")
	writeCaddyPackage(t, dir)

	out, err := execute(t, "--config", dir, "expose", "rm", "app.example.com", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "devmachine sync") {
		t.Fatalf("%q", out)
	}
	cfg, _ := config.Load(dir)
	if _, _, ok := cfg.RouteOwner("app.example.com"); ok {
		t.Fatal("the route is still recorded")
	}
}

func TestExposeRmOfAnUnmanagedHostSaysHowToAdoptOrRemoveIt(t *testing.T) {
	dialExpose(t, &exposeClient{})
	dir := configWithCaddy(t)
	_, err := execute(t, "--config", dir, "expose", "rm", "old.example.com", "--yes")
	if err == nil || !strings.Contains(err.Error(), "expose add") || !strings.Contains(err.Error(), "old.example.com.caddy") {
		t.Fatalf("got %v", err)
	}
}

// A machine that lost caddy while a route still points at it is the one state
// where removing the route matters most, so it is the state these two must
// not refuse.
func TestExposeRmWorksWhenCaddyLeftTheMachine(t *testing.T) {
	dialExpose(t, &exposeClient{})
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n"+
		"workspaces:\n  - name: alice\n    routes: [{host: app.example.com, port: 8080}]\n")

	if _, err := execute(t, "--config", dir, "expose", "rm", "app.example.com", "--yes"); err != nil {
		t.Fatalf("rm must not need caddy on the machine: %v", err)
	}
	cfg, _ := config.Load(dir)
	if _, _, ok := cfg.RouteOwner("app.example.com"); ok {
		t.Fatal("the route is still recorded")
	}
}

func TestExposeListWorksWhenCaddyLeftTheMachine(t *testing.T) {
	dialExpose(t, &exposeClient{})
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n"+
		"workspaces:\n  - name: alice\n    routes: [{host: app.example.com, port: 8080}]\n")

	out, err := execute(t, "--config", dir, "expose", "list")
	if err != nil {
		t.Fatalf("list must not refuse: %v", err)
	}
	if !strings.Contains(out, "app.example.com") || !strings.Contains(out, "unknown") {
		t.Fatalf("the configuration's row must still print, as unknown: %q", out)
	}
}

// configPublishing is configWithCaddy with app.example.com already published
// by alice, and the hostinger provider installed on the machine.
func configPublishing(t *testing.T) string {
	t.Helper()
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [caddy]\n"+
		"workspaces:\n  - name: alice\n    routes: [{host: app.example.com, port: 8080}]\n")
	writeCaddyPackage(t, dir)
	writeDNSPackage(t, dir, "hostinger", nil, "print('ok')")
	lockOnto(t, dir, "main", "hostinger")
	return dir
}

func rmJSON(t *testing.T, dir string, args ...string) (exposeResult, string) {
	t.Helper()
	out, err := execute(t, append([]string{"--config", dir, "--format", "json", "expose", "rm"}, args...)...)
	if err != nil {
		t.Fatal(err, out)
	}
	var got exposeResult
	if err := json.Unmarshal([]byte(lastJSON(out)), &got); err != nil {
		t.Fatal(err, out)
	}
	return got, out
}

func TestExposeRmRemovesTheRecordAddPointed(t *testing.T) {
	client := &exposeClient{zones: []string{"example.com"},
		records: []dns.Record{{Name: "app", Type: "A", Value: "203.0.113.10"}, {Name: "www", Type: "A", Value: "203.0.113.10"}}}
	dialExpose(t, client)
	dir := configPublishing(t)

	got, out := rmJSON(t, dir, "app.example.com", "--yes")

	if got.Status != "removed" || got.DNSError != "" {
		t.Fatalf("%+v\n%s", got, out)
	}
	if len(client.deleted) != 1 || !strings.Contains(client.deleted[0], `{"name":"app","type":"A","value":"203.0.113.10"}`) {
		t.Fatalf("exactly app's A record pointing at the machine is deleted: %q", client.deleted)
	}
	if !strings.Contains(out, "removed app A 203.0.113.10 from example.com") {
		t.Fatalf("it does not say what it removed:\n%s", out)
	}
}

func TestExposeRmLeavesARecordThatPointsElsewhere(t *testing.T) {
	client := &exposeClient{zones: []string{"example.com"},
		records: []dns.Record{{Name: "app", Type: "A", Value: "198.51.100.7"}}}
	dialExpose(t, client)
	dir := configPublishing(t)

	got, out := rmJSON(t, dir, "app.example.com", "--yes")

	if len(client.deleted) != 0 {
		t.Fatalf("a name somebody repointed was deleted: %q", client.deleted)
	}
	if got.DNSError != "" || !strings.Contains(out, "points at 198.51.100.7, not at main (203.0.113.10)") {
		t.Fatalf("it does not say why the record stays: %+v\n%s", got, out)
	}
}

func TestExposeRmWithNoRecordLeftDeletesNothing(t *testing.T) {
	client := &exposeClient{zones: []string{"example.com"}}
	dialExpose(t, client)
	dir := configPublishing(t)

	got, _ := rmJSON(t, dir, "app.example.com", "--yes")

	if len(client.deleted) != 0 || got.DNSError != "" {
		t.Fatalf("nothing was there to delete: %q %+v", client.deleted, got)
	}
}

func TestExposeRmWithNoProviderPrintsTheRecordToRemoveByHand(t *testing.T) {
	client := &exposeClient{files: map[string]string{"alice-routes.caddy": "app.example.com {\n}\n"}}
	dialExpose(t, client)
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [caddy]\n"+
		"workspaces:\n  - name: alice\n    routes: [{host: app.example.com, port: 8080}]\n")
	writeCaddyPackage(t, dir)

	out, err := execute(t, "--config", dir, "expose", "rm", "app.example.com", "--yes")
	if err != nil {
		t.Fatal(err, out)
	}
	if !strings.Contains(out, "Remove this record by hand") || !strings.Contains(out, "app.example.com\tA\t203.0.113.10") {
		t.Fatalf("it does not print the record to remove by hand:\n%s", out)
	}
}

func TestExposeRmStillTakesTheSiteOffCaddyWhenTheProviderRefuses(t *testing.T) {
	client := &exposeClient{zones: []string{"example.com"},
		records:   []dns.Record{{Name: "app", Type: "A", Value: "203.0.113.10"}},
		deleteErr: errors.New("exit status 1")}
	dialExpose(t, client)
	dir := configPublishing(t)

	got, out := rmJSON(t, dir, "app.example.com", "--yes")

	if got.Status != "removed" || !got.Applied {
		t.Fatalf("Caddy still drops the site: %+v", got)
	}
	if !strings.Contains(got.DNSError, "hostinger refused it") {
		t.Fatalf("the JSON must say the record was not removed: %+v", got)
	}
	if !strings.Contains(out, "Remove this record by hand") || !strings.Contains(out, "app (in example.com)\tA\t203.0.113.10") {
		t.Fatalf("it does not print the record to remove by hand:\n%s", out)
	}
}

func TestExposeRmDeletesNothingWhenTheProviderCannotList(t *testing.T) {
	client := &exposeClient{zones: []string{"example.com"}, listErr: errors.New("exit status 1")}
	dialExpose(t, client)
	dir := configPublishing(t)

	got, _ := rmJSON(t, dir, "app.example.com", "--yes")

	if len(client.deleted) != 0 {
		t.Fatalf("a record it could not see was deleted: %q", client.deleted)
	}
	if got.Status != "removed" || got.DNSError == "" {
		t.Fatalf("%+v", got)
	}
}

func TestExposeRmCheckShowsTheDNSRemovalAndDeletesNothing(t *testing.T) {
	client := &exposeClient{zones: []string{"example.com"},
		records: []dns.Record{{Name: "app", Type: "A", Value: "203.0.113.10"}}}
	dialExpose(t, client)
	dir := configPublishing(t)

	out, err := execute(t, "--config", dir, "expose", "rm", "app.example.com", "--check")
	if err != nil {
		t.Fatal(err, out)
	}
	if !strings.Contains(out, "would remove app A 203.0.113.10 from example.com (provider hostinger)") {
		t.Fatalf("--check does not show the DNS removal:\n%s", out)
	}
	if len(client.deleted) != 0 {
		t.Fatalf("--check deleted %q", client.deleted)
	}
}

func TestExposeRmNoApplyLeavesTheRecordAndSaysHowToRemoveIt(t *testing.T) {
	client := &exposeClient{zones: []string{"example.com"},
		records: []dns.Record{{Name: "app", Type: "A", Value: "203.0.113.10"}}}
	dialExpose(t, client)
	dir := configPublishing(t)

	out, err := execute(t, "--config", dir, "expose", "rm", "app.example.com", "--yes", "--no-apply")
	if err != nil {
		t.Fatal(err, out)
	}
	if len(client.deleted) != 0 || client.lists != 0 {
		t.Fatalf("--no-apply asked the DNS provider: %d lists, %q", client.lists, client.deleted)
	}
	if !strings.Contains(out, "devmachine dns rm app.example.com A 203.0.113.10 --machine main") {
		t.Fatalf("it does not say how to remove the record later:\n%s", out)
	}
}

func TestExposeRmWithTheMachineDownSaysWhyTheRecordIsPrinted(t *testing.T) {
	machineDown(t)
	dir := configPublishing(t)

	out, err := execute(t, "--config", dir, "expose", "rm", "app.example.com", "--yes")
	if err != nil {
		t.Fatal(err, out)
	}
	if !strings.Contains(out, "main could not be reached") || strings.Contains(out, "No installed provider holds") {
		t.Fatalf("the reason is the machine, not the providers:\n%s", out)
	}
	if !strings.Contains(out, "Remove this record by hand") || !strings.Contains(out, "app.example.com\tA\t203.0.113.10") {
		t.Fatalf("%s", out)
	}
}

func TestExposeAddWithTheMachineDownSaysWhyTheRecordIsPrinted(t *testing.T) {
	machineDown(t)
	dir := configWithCaddy(t)

	out, err := execute(t, "--config", dir, "expose", "add", "alice", "8080", "--host", "app.example.com", "--publish")
	if err != nil {
		t.Fatal(err, out)
	}
	if strings.Contains(out, "No installed provider holds") || !strings.Contains(out, "Create this record by hand") {
		t.Fatalf("the reason is the machine, not the providers:\n%s", out)
	}
}
