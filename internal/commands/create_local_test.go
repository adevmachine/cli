package commands

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/keys"
	"github.com/mydevmachine/devmachine/internal/local"
)

func stubCreateLocal(t *testing.T) *[]string {
	t.Helper()
	created := &[]string{}
	t.Cleanup(swap(&createLocal, func(_ context.Context, name string, _ io.Writer) (config.Machine, error) {
		*created = append(*created, name)
		return config.Machine{
			Name: name, Hosts: []config.Host{{Address: local.Address}}, User: local.AdminUser, Port: 60022,
		}, nil
	}))
	return created
}

func TestCreateLocalWithAddAddsTheMachineWithItsPassword(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "devmachine")
	stubCreateLocal(t)
	steps := stubBootstrap(t, bootstrapStubs{keyWorks: false})

	out, err := executeWithInput(t, "", "--config", dir, "--format", "json",
		"machines", "create-local", "sandbox", "--add", "--no-aliases")
	if err != nil {
		t.Fatalf("create-local --add returned %v (%s)", err, out)
	}
	if steps.password != local.Password || !steps.installedKey || !steps.proved {
		t.Fatalf("it did not install the key with the local password: %q", steps.events)
	}
	if slices.Contains(steps.events, "ask trust") {
		t.Fatalf("it asked to trust the host key: %q", steps.events)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("no configuration: %v", err)
	}
	m, err := cfg.Machine("sandbox")
	if err != nil {
		t.Fatal(err)
	}
	if m.Port != 60022 || m.Hosts[0].Address != local.Address || m.Key != filepath.Join(keys.Dir(dir), "sandbox") {
		t.Fatalf("machine = %#v", m)
	}

	start := strings.Index(out, "{")
	var got machineJSON
	if start < 0 || json.Unmarshal([]byte(out[start:]), &got) != nil {
		t.Fatalf("no JSON at the end: %q", out)
	}
	if got.Name != "sandbox" || got.Key != m.Key || got.Port != 60022 {
		t.Fatalf("reported %#v", got)
	}
}

func TestCreateLocalWithAddRefusesATakenNameBeforeCreatingAnything(t *testing.T) {
	dir := writeConfigDir(t, "machines:\n  - name: sandbox\n    hosts: [203.0.113.10]\n")
	created := stubCreateLocal(t)
	stubBootstrap(t, bootstrapStubs{keyWorks: true})

	_, err := executeWithInput(t, "", "--config", dir, "machines", "create-local", "sandbox", "--add")
	if err == nil || !strings.Contains(err.Error(), "already configured") {
		t.Fatalf("got %v", err)
	}
	if len(*created) != 0 {
		t.Fatal("it created a VM it was never going to add")
	}
}

func TestCreateLocalWithoutAddWritesNoConfiguration(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "devmachine")
	stubCreateLocal(t)
	steps := stubBootstrap(t, bootstrapStubs{keyWorks: true})

	out, err := executeWithInput(t, "", "--config", dir, "machines", "create-local", "sandbox")
	if err != nil {
		t.Fatal(err)
	}
	if len(steps.events) != 0 {
		t.Fatalf("it reached the machine: %q", steps.events)
	}
	if _, err := config.Load(dir); err == nil {
		t.Fatal("it wrote a configuration")
	}
	if !strings.Contains(out, "--add") {
		t.Fatalf("it does not say how to add it in one step: %q", out)
	}
}

func TestCreateLocalRefusesAddFlagsWithoutAdd(t *testing.T) {
	created := stubCreateLocal(t)

	_, err := execute(t, "--config", t.TempDir(), "machines", "create-local", "sandbox", "--no-aliases")
	if err == nil || !strings.Contains(err.Error(), "--add") || len(*created) != 0 {
		t.Fatalf("got %v, created %v", err, *created)
	}
}
