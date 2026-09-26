package commands

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/remote"
)

// destroyClient is a remote.Client a test drives by hand: it plays the shell
// script `workspaces destroy` sends, without touching a real machine.
type destroyClient struct {
	userExists bool
	lastScript string
	runErr     error
}

func (c *destroyClient) Run(_ context.Context, script string) (string, error) {
	c.lastScript = script
	if c.runErr != nil {
		return "", c.runErr
	}
	if c.userExists {
		return "removed", nil
	}
	return "absent", nil
}

func (c *destroyClient) RunInput(ctx context.Context, command string, _ io.Reader) (string, error) {
	return c.Run(ctx, command)
}

func (c *destroyClient) Stream(ctx context.Context, command string, stdout, _ io.Writer) error {
	out, err := c.Run(ctx, command)
	if _, werr := io.WriteString(stdout, out); werr != nil {
		return werr
	}
	return err
}

func (c *destroyClient) Upload(context.Context, string, io.Reader) error { return nil }
func (c *destroyClient) Close() error                                    { return nil }

func dialDestroy(t *testing.T, client *destroyClient) {
	t.Helper()
	orig := dial
	dial = func(context.Context, config.Machine, string) (remote.Client, string, error) {
		return client, "203.0.113.10", nil
	}
	t.Cleanup(func() { dial = orig })
}

func TestWorkspacesDestroyDeclinesOnTheWrongTypedName(t *testing.T) {
	client := &destroyClient{userExists: true}
	dialDestroy(t, client)
	dir := configWithKey(t, "workspaces:\n  - name: alice\n    machine: main\n")

	_, err := executeWithInput(t, "not-alice\n", "--config", dir, "workspaces", "destroy", "alice")
	if !errors.Is(err, errDeclined) {
		t.Fatalf("got %v", err)
	}
	if client.lastScript != "" {
		t.Fatalf("it ran something on the machine: %q", client.lastScript)
	}
	cfg, _ := config.Load(dir)
	if len(cfg.Workspaces) != 1 {
		t.Fatalf("config changed: %#v", cfg.Workspaces)
	}
}

func TestWorkspacesDestroyConfirmRunsOneScriptAndRemovesTheRoutesFile(t *testing.T) {
	client := &destroyClient{userExists: true}
	dialDestroy(t, client)
	dir := configWithCaddy(t)
	// configWithCaddy already declares workspace alice; add a route for it.
	if err := config.AddRoute(dir, "alice", config.Route{Host: "app.example.com", Port: 8080}); err != nil {
		t.Fatal(err)
	}

	out, err := execute(t, "--config", dir, "workspaces", "destroy", "alice", "--confirm", "alice")
	if err != nil {
		t.Fatalf("got %v (%s)", err, out)
	}
	for _, want := range []string{"loginctl disable-linger", "terminate-user", "userdel --force --remove", "'alice'", "alice-routes.caddy"} {
		if !strings.Contains(client.lastScript, want) {
			t.Fatalf("script does not contain %q: %s", want, client.lastScript)
		}
	}
	cfg, _ := config.Load(dir)
	if _, err := cfg.Workspace("alice"); err == nil {
		t.Fatal("alice is still configured")
	}
}

func TestWorkspacesDestroyListsEachRouteBeforeAsking(t *testing.T) {
	client := &destroyClient{userExists: true}
	dialDestroy(t, client)
	dir := configWithKey(t, "workspaces:\n  - name: alice\n    machine: main\n"+
		"    routes: [{host: app.example.com, port: 8080}, {host: api.example.com, port: 9090}]\n")

	out, _ := executeWithInput(t, "\n", "--config", dir, "workspaces", "destroy", "alice")
	for _, want := range []string{"https://app.example.com", "https://api.example.com"} {
		if !strings.Contains(out, want) {
			t.Fatalf("it did not list the route %q: %q", want, out)
		}
	}
}

func TestWorkspacesDestroyCheckRunsNothing(t *testing.T) {
	client := &destroyClient{userExists: true}
	dialDestroy(t, client)
	dir := configWithKey(t, "workspaces:\n  - name: alice\n    machine: main\n")

	out, err := execute(t, "--config", dir, "workspaces", "destroy", "alice", "--check")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "would destroy alice") {
		t.Fatalf("got %q", out)
	}
	if client.lastScript != "" {
		t.Fatalf("--check ran something on the machine: %q", client.lastScript)
	}
	cfg, _ := config.Load(dir)
	if len(cfg.Workspaces) != 1 {
		t.Fatalf("--check changed the configuration: %#v", cfg.Workspaces)
	}
}

func TestWorkspacesDestroyConfirmMustMatchTheName(t *testing.T) {
	client := &destroyClient{userExists: true}
	dialDestroy(t, client)
	dir := configWithKey(t, "workspaces:\n  - name: alice\n    machine: main\n")

	_, err := execute(t, "--config", dir, "workspaces", "destroy", "alice", "--confirm", "bob")
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("got %v", err)
	}
	if client.lastScript != "" {
		t.Fatal("it dialed the machine despite the mismatch")
	}
}

func TestWorkspacesDestroyDialFailureLeavesConfigAlone(t *testing.T) {
	orig := dial
	dial = func(context.Context, config.Machine, string) (remote.Client, string, error) {
		return nil, "", errors.New("no route to host")
	}
	t.Cleanup(func() { dial = orig })
	dir := configWithKey(t, "workspaces:\n  - name: alice\n    machine: main\n")

	_, err := execute(t, "--config", dir, "workspaces", "destroy", "alice", "--confirm", "alice")
	if err == nil || !strings.Contains(err.Error(), "workspaces rm") {
		t.Fatalf("got %v", err)
	}
	cfg, _ := config.Load(dir)
	if len(cfg.Workspaces) != 1 {
		t.Fatalf("config changed: %#v", cfg.Workspaces)
	}
}

func TestWorkspacesDestroyScriptFailureLeavesConfigAlone(t *testing.T) {
	client := &destroyClient{userExists: true, runErr: errors.New("permission denied")}
	dialDestroy(t, client)
	dir := configWithKey(t, "workspaces:\n  - name: alice\n    machine: main\n")

	_, err := execute(t, "--config", dir, "workspaces", "destroy", "alice", "--confirm", "alice")
	if err == nil {
		t.Fatal("expected the script failure to surface")
	}
	cfg, _ := config.Load(dir)
	if len(cfg.Workspaces) != 1 {
		t.Fatalf("config changed: %#v", cfg.Workspaces)
	}
}

func TestWorkspacesDestroyAbsentAccountStillSucceeds(t *testing.T) {
	client := &destroyClient{userExists: false}
	dialDestroy(t, client)
	dir := configWithKey(t, "workspaces:\n  - name: alice\n    machine: main\n")

	out, err := execute(t, "--config", dir, "workspaces", "destroy", "alice", "--confirm", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "had no account") {
		t.Fatalf("got %q", out)
	}
	cfg, _ := config.Load(dir)
	if _, err := cfg.Workspace("alice"); err == nil {
		t.Fatal("alice is still configured")
	}
}

func TestWorkspacesDestroyRefusesTheAdminAccount(t *testing.T) {
	client := &destroyClient{}
	dialDestroy(t, client)
	dir := configWithKey(t, "workspaces:\n  - name: alice\n    machine: main\n    user: root\n")

	_, err := execute(t, "--config", dir, "workspaces", "destroy", "alice", "--confirm", "alice")
	if err == nil || !strings.Contains(err.Error(), "administrative account") {
		t.Fatalf("got %v", err)
	}
	if client.lastScript != "" {
		t.Fatal("it dialed the machine for the admin account")
	}
}

func TestWorkspacesDestroyRefusesWhenItCannotFindWhereItsRoutesLive(t *testing.T) {
	client := &destroyClient{userExists: true}
	dialDestroy(t, client)
	dir := configWithKey(t, "workspaces:\n  - name: alice\n    machine: main\n"+
		"    routes: [{host: app.example.com, port: 8080}]\n")

	_, err := execute(t, "--config", dir, "workspaces", "destroy", "alice", "--confirm", "alice")
	if err == nil || !strings.Contains(err.Error(), "app.example.com") || !strings.Contains(err.Error(), "expose rm") {
		t.Fatalf("got %v", err)
	}
	if client.lastScript != "" {
		t.Fatalf("ran on the machine: %q", client.lastScript)
	}
	cfg, _ := config.Load(dir)
	if len(cfg.Workspaces) != 1 {
		t.Fatal("config changed")
	}
}
