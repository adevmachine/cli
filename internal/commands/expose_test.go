package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/packages"
	"github.com/adevmachine/cli/internal/remote"
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
// entrypoint (zones/upsert), without ever touching a real machine.
type exposeClient struct {
	files map[string]string // sites.d file name -> its content
	// zones is what a locked DNS provider claims to hold, empty meaning no
	// provider answers at all.
	zones   []string
	upserts int
	lastCmd string
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
		return "", fmt.Errorf("no such file: %s", command)
	case strings.Contains(command, "rm -f "):
		for name := range c.files {
			if strings.Contains(command, name) {
				delete(c.files, name)
			}
		}
		return "", nil
	case strings.HasSuffix(strings.TrimSpace(command), " zones"):
		body, _ := json.Marshal(struct {
			Zones []string `json:"zones"`
		}{c.zones})
		return string(body), nil
	case strings.Contains(command, " upsert "):
		c.upserts++
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
	orig := dial
	dial = func(context.Context, config.Machine, string) (remote.Client, string, error) {
		return client, "203.0.113.10", nil
	}
	t.Cleanup(func() { dial = orig })
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

func TestExposeListReadsWhatIsThere(t *testing.T) {
	client := &exposeClient{files: map[string]string{
		"app.example.com.caddy": "app.example.com {\n\treverse_proxy 127.0.0.1:8080\n}\n# workspace: alice\n",
	}}
	dialExpose(t, client)
	dir := configWithCaddy(t)

	out, err := execute(t, "--config", dir, "expose", "list")
	if err != nil {
		t.Fatalf("expose list returned %v", err)
	}
	if !strings.Contains(out, "app.example.com") || !strings.Contains(out, "8080") {
		t.Fatalf("got %q", out)
	}
}

func TestExposeAddRecordsTheRouteAndTouchesNoMachine(t *testing.T) {
	client := &exposeClient{}
	dialExpose(t, client)
	dir := configWithCaddy(t)

	out, err := execute(t, "--config", dir, "expose", "add", "alice", "8080",
		"--host", "app.example.com", "--publish")
	if err != nil {
		t.Fatal(err)
	}
	if len(client.files) != 0 {
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
	orig := dial
	dial = func(context.Context, config.Machine, string) (remote.Client, string, error) {
		return nil, "", errors.New("no route to host")
	}
	t.Cleanup(func() { dial = orig })
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
	dialExpose(t, &exposeClient{})
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
}

func TestExposeRmForgetsTheRouteAndTouchesNoMachine(t *testing.T) {
	client := &exposeClient{files: map[string]string{"alice-routes.caddy": "app.example.com {\n}\n"}}
	dialExpose(t, client)
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [caddy]\n"+
		"workspaces:\n  - name: alice\n    routes: [{host: app.example.com, port: 8080}]\n")
	writeCaddyPackage(t, dir)

	out, err := execute(t, "--config", dir, "expose", "rm", "app.example.com", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if len(client.files) != 1 {
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

func TestExposeRmOfAnUnmanagedHostSaysHowToAdoptOrRemoveIt(t *testing.T) {
	dialExpose(t, &exposeClient{})
	dir := configWithCaddy(t)
	_, err := execute(t, "--config", dir, "expose", "rm", "old.example.com", "--yes")
	if err == nil || !strings.Contains(err.Error(), "expose add") || !strings.Contains(err.Error(), "old.example.com.caddy") {
		t.Fatalf("got %v", err)
	}
}
