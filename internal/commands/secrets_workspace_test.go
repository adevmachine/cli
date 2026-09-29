package commands

import (
	"strings"
	"testing"

	"github.com/mydevmachine/devmachine/internal/credentials"
	"github.com/mydevmachine/devmachine/internal/secrets"
)

// configWithAWorkspace is one machine and one workspace, with no packages:
// enough for a workspace's own secret, which is not tied to any package.
func configWithAWorkspace(t *testing.T) string {
	t.Helper()
	return configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n"+
		"workspaces:\n  - name: alice\n")
}

func TestSecretsSetWithWorkspaceStoresUnderTheWorkspaceKey(t *testing.T) {
	dir := configWithAWorkspace(t)
	t.Cleanup(func() { _ = secrets.Delete(dir, "alice/API_KEY") })

	if _, err := execute(t, "--config", dir, "secrets", "set", "API_KEY", "value", "--workspace", "alice"); err != nil {
		t.Fatalf("secrets set returned %v", err)
	}

	got, err := secrets.Get(dir, "alice/API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if got != "value" {
		t.Fatalf("got %q", got)
	}
}

func TestSecretsSetWithWorkspaceRecordsTheDefaultTarget(t *testing.T) {
	dir := configWithAWorkspace(t)
	t.Cleanup(func() { _ = secrets.Delete(dir, "alice/API_KEY") })

	if _, err := execute(t, "--config", dir, "secrets", "set", "API_KEY", "value", "--workspace", "alice"); err != nil {
		t.Fatal(err)
	}

	target, ok, err := secrets.FindTarget(dir, "alice", "API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("no target was recorded")
	}
	if target.EnvFile != "" {
		t.Fatalf("got %#v, want the default (empty EnvFile)", target)
	}
}

func TestSecretsSetWithEnvFileRecordsThatTarget(t *testing.T) {
	dir := configWithAWorkspace(t)
	t.Cleanup(func() { _ = secrets.Delete(dir, "alice/API_KEY") })

	if _, err := execute(t, "--config", dir, "secrets", "set", "API_KEY", "value",
		"--workspace", "alice", "--env-file", "app/.env"); err != nil {
		t.Fatal(err)
	}

	target, ok, err := secrets.FindTarget(dir, "alice", "API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || target.EnvFile != "app/.env" {
		t.Fatalf("got %#v", target)
	}
}

func TestSecretsSetWithEnvFileRefusesAPathThatEscapesTheHome(t *testing.T) {
	dir := configWithAWorkspace(t)

	_, err := execute(t, "--config", dir, "secrets", "set", "API_KEY", "value",
		"--workspace", "alice", "--env-file", "../outside")
	if err == nil {
		t.Fatal("expected an error for a path that escapes the workspace's home")
	}
	if _, ok, _ := secrets.FindTarget(dir, "alice", "API_KEY"); ok {
		t.Fatal("a target was recorded despite the refusal")
	}
}

func TestSecretsSetEnvFileWithoutWorkspaceIsRefused(t *testing.T) {
	dir := configWithAWorkspace(t)

	_, err := execute(t, "--config", dir, "secrets", "set", "API_KEY", "value", "--env-file", "app/.env")
	if err == nil || !strings.Contains(err.Error(), "--workspace") {
		t.Fatalf("got %v", err)
	}
}

func TestSecretsSetPushWithoutWorkspaceIsRefused(t *testing.T) {
	dir := configWithAWorkspace(t)

	_, err := execute(t, "--config", dir, "secrets", "set", "API_KEY", "value", "--push")
	if err == nil || !strings.Contains(err.Error(), "--workspace") {
		t.Fatalf("got %v", err)
	}
}

func TestSecretsSetPushDeliversImmediately(t *testing.T) {
	dir := configWithAWorkspace(t)
	t.Cleanup(func() { _ = secrets.Delete(dir, "alice/API_KEY") })
	client := &recordingRemote{out: "MISSING\n"}
	dialing(t, client)

	out, err := execute(t, "--config", dir, "secrets", "set", "API_KEY", "value",
		"--workspace", "alice", "--push")
	if err != nil {
		t.Fatalf("secrets set --push returned %v", err)
	}
	if !strings.Contains(out, credentials.DefaultEnvFile) {
		t.Fatalf("the report does not say where it went: %q", out)
	}
	if !client.wrote() {
		t.Fatal("nothing was delivered to the machine")
	}
	if !strings.Contains(strings.Join(client.delivered, "\n"), "API_KEY='value'") {
		t.Fatalf("the value did not reach the file: %#v", client.delivered)
	}
}

func TestSecretsListShowsAWorkspaceSecretAndItsTarget(t *testing.T) {
	dir := configWithAWorkspace(t)
	t.Cleanup(func() { _ = secrets.Delete(dir, "alice/API_KEY") })
	execute(t, "--config", dir, "secrets", "set", "API_KEY", "value",
		"--workspace", "alice", "--env-file", "app/.env")

	out, err := execute(t, "--config", dir, "secrets", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "alice/API_KEY") || !strings.Contains(out, "app/.env") {
		t.Fatalf("got %q", out)
	}
	if strings.Contains(out, "value") {
		t.Fatalf("the value leaked: %q", out)
	}
}

func TestSecretsListWithWorkspaceFiltersToThatWorkspace(t *testing.T) {
	dir := configWithAWorkspace(t)
	t.Cleanup(func() {
		_ = secrets.Delete(dir, "alice/API_KEY")
		_ = secrets.Delete(dir, "machine_token")
	})
	execute(t, "--config", dir, "secrets", "set", "API_KEY", "value", "--workspace", "alice")
	execute(t, "--config", dir, "secrets", "set", "machine_token", "value")

	out, err := execute(t, "--config", dir, "secrets", "list", "--workspace", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "alice/API_KEY") {
		t.Fatalf("the workspace secret is missing: %q", out)
	}
	if strings.Contains(out, "machine_token") {
		t.Fatalf("a secret from another scope leaked in: %q", out)
	}
}

func TestSecretsRmWithoutFromFileOnlyForgetsTheTarget(t *testing.T) {
	dir := configWithAWorkspace(t)
	execute(t, "--config", dir, "secrets", "set", "API_KEY", "value",
		"--workspace", "alice", "--env-file", "app/.env")

	if _, err := execute(t, "--config", dir, "secrets", "rm", "API_KEY", "--workspace", "alice"); err != nil {
		t.Fatal(err)
	}

	if _, ok, _ := secrets.FindTarget(dir, "alice", "API_KEY"); ok {
		t.Fatal("the target survived a plain rm")
	}
	if _, err := secrets.Get(dir, "alice/API_KEY"); err == nil {
		t.Fatal("the value survived rm")
	}
}

func TestSecretsRmWithFromFileMarksTheTargetForRemoval(t *testing.T) {
	dir := configWithAWorkspace(t)
	execute(t, "--config", dir, "secrets", "set", "API_KEY", "value",
		"--workspace", "alice", "--env-file", "app/.env")

	if _, err := execute(t, "--config", dir, "secrets", "rm", "API_KEY",
		"--workspace", "alice", "--from-file"); err != nil {
		t.Fatal(err)
	}

	target, ok, err := secrets.FindTarget(dir, "alice", "API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || !target.PendingRemoval {
		t.Fatalf("got %#v, ok=%v", target, ok)
	}
}

func TestSecretsRmFromFileWithNoTargetIsRefused(t *testing.T) {
	dir := configWithAWorkspace(t)

	_, err := execute(t, "--config", dir, "secrets", "rm", "API_KEY", "--workspace", "alice", "--from-file")
	if err == nil {
		t.Fatal("expected an error: there is nothing to remove from a file")
	}
}

func TestCredentialsPushDeliversAWorkspaceSecret(t *testing.T) {
	dir := configWithAWorkspace(t)
	t.Cleanup(func() { _ = secrets.Delete(dir, "alice/API_KEY") })
	execute(t, "--config", dir, "secrets", "set", "API_KEY", "s3cret", "--workspace", "alice")
	client := &recordingRemote{out: "MISSING\n"}
	dialing(t, client)

	out, err := execute(t, "--config", dir, "credentials", "push", "--yes")
	if err != nil {
		t.Fatalf("credentials push returned %v", err)
	}
	if !strings.Contains(out, "alice/API_KEY") {
		t.Fatalf("the report does not name it: %q", out)
	}
	if !client.wrote() {
		t.Fatal("nothing was delivered")
	}
	if strings.Contains(out, "s3cret") {
		t.Fatalf("the report carried a value: %q", out)
	}
}

func TestCredentialsPushNamesAWorkspaceSecretWithNoValue(t *testing.T) {
	dir := configWithAWorkspace(t)
	if err := secrets.SetTarget(dir, secrets.Target{Workspace: "alice", Name: "API_KEY"}); err != nil {
		t.Fatal(err)
	}
	client := &recordingRemote{out: "MISSING\n"}
	dialing(t, client)

	out, err := execute(t, "--config", dir, "credentials", "push", "--yes")
	if err == nil {
		t.Fatal("expected an error: nobody stored a value")
	}
	if !strings.Contains(out+err.Error(), "alice/API_KEY") {
		t.Fatalf("got %q / %v", out, err)
	}
	if client.wrote() {
		t.Fatal("it wrote something with no value stored")
	}
}

func TestCredentialsPushRemovesAWorkspaceSecretMarkedFromFile(t *testing.T) {
	dir := configWithAWorkspace(t)
	execute(t, "--config", dir, "secrets", "set", "API_KEY", "value",
		"--workspace", "alice", "--env-file", "app/.env")
	execute(t, "--config", dir, "secrets", "rm", "API_KEY", "--workspace", "alice", "--from-file")
	client := &recordingRemote{out: "EXISTS\nAPI_KEY='value'\nOTHER=kept\n"}
	dialing(t, client)

	out, err := execute(t, "--config", dir, "credentials", "push", "--yes")
	if err != nil {
		t.Fatalf("credentials push returned %v", err)
	}
	if !strings.Contains(out, "alice/API_KEY") {
		t.Fatalf("the report does not name it: %q", out)
	}
	if _, ok, _ := secrets.FindTarget(dir, "alice", "API_KEY"); ok {
		t.Fatal("the target was not forgotten after removal")
	}
}

func TestCredentialsPushCheckDoesNotDeliverAWorkspaceSecret(t *testing.T) {
	dir := configWithAWorkspace(t)
	t.Cleanup(func() { _ = secrets.Delete(dir, "alice/API_KEY") })
	execute(t, "--config", dir, "secrets", "set", "API_KEY", "s3cret", "--workspace", "alice")
	client := &recordingRemote{out: "MISSING\n"}
	dialing(t, client)

	out, err := execute(t, "--config", dir, "credentials", "push", "--check")
	if err != nil {
		t.Fatalf("credentials push --check returned %v", err)
	}
	if client.wrote() {
		t.Fatal("--check delivered a value")
	}
	if !strings.Contains(out, "alice/API_KEY") {
		t.Fatalf("a dry run should still say what it would deliver: %q", out)
	}
}

func TestCredentialsPushOnlyTouchesWorkspaceSecretsOnTheirOwnMachine(t *testing.T) {
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n"+
		"  - name: other\n    hosts: [203.0.113.20]\n"+
		"workspaces:\n  - name: alice\n    machine: other\n")
	t.Cleanup(func() { _ = secrets.Delete(dir, "alice/API_KEY") })
	execute(t, "--config", dir, "secrets", "set", "API_KEY", "value", "--workspace", "alice")
	client := &recordingRemote{out: "MISSING\n"}
	dialing(t, client)

	out, err := execute(t, "--config", dir, "--machine", "main", "credentials", "push", "--yes")
	if err != nil {
		t.Fatalf("credentials push returned %v", err)
	}
	if strings.Contains(out, "alice/API_KEY") || client.wrote() {
		t.Fatalf("a secret for a workspace on another machine was pushed: %q", out)
	}
}
