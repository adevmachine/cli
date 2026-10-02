package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/hostkeys"
)

const oneMachine = "machines:\n  - name: main\n    hosts: [203.0.113.10]\n"

func readConfigFile(t *testing.T, dir string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// A machine is written only once the key is proved, so a failed run leaves
// nothing behind that a second run would trip over.
func TestMachinesAddWithFlagsWritesNothingUntilTheKeyIsProved(t *testing.T) {
	dir := writeConfigDir(t, oneMachine)
	before := readConfigFile(t, dir)
	steps := stubBootstrap(t, bootstrapStubs{keyWorks: false})
	args := []string{"--config", dir, "machines", "add", "--name", "sandbox", "--address", "100.64.0.7",
		"--fingerprint", hostkeys.Fingerprint(steps.hostKey), "--no-aliases"}

	if _, err := executeWithInput(t, "", args...); err == nil {
		t.Fatal("a key that does not log in, and no password, was accepted")
	}
	if after := readConfigFile(t, dir); after != before {
		t.Fatalf("a failed add changed config.yml:\n%s", after)
	}

	steps = stubBootstrap(t, bootstrapStubs{keyWorks: true})
	args[len(args)-2] = hostkeys.Fingerprint(steps.hostKey)
	if err := os.Remove(filepath.Join(dir, config.KnownHostsFileName)); err != nil {
		t.Fatal(err)
	}
	if out, err := executeWithInput(t, "", args...); err != nil {
		t.Fatalf("the second run failed: %v (%s)", err, out)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.Machine("sandbox"); err != nil {
		t.Fatalf("the second run did not add it: %v", err)
	}
}

func TestMachinesAddWritesNothingWhenTheProofFails(t *testing.T) {
	dir := writeConfigDir(t, oneMachine)
	before := readConfigFile(t, dir)
	stubBootstrap(t, bootstrapStubs{keyWorks: false, proofFails: true})

	_, err := executeWithInput(t, "sandbox\n198.51.100.7\nroot\n22\n1\ndevmachine\n",
		"--config", dir, "machines", "add")
	if err == nil {
		t.Fatal("an unproved key was accepted")
	}
	if after := readConfigFile(t, dir); after != before {
		t.Fatalf("a failed add changed config.yml:\n%s", after)
	}
}
