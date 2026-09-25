package commands

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/hostkeys"
	"github.com/adevmachine/cli/internal/keys"
	"github.com/adevmachine/cli/internal/remote"
	"golang.org/x/crypto/ssh"
)

// nopClient stands in for a machine. Every step of the bootstrap is stubbed,
// so no test reads what a command returned.
type nopClient struct{}

func (nopClient) Run(context.Context, string) (string, error)                 { return "", nil }
func (nopClient) RunInput(context.Context, string, io.Reader) (string, error) { return "", nil }
func (nopClient) Stream(context.Context, string, io.Writer, io.Writer) error  { return nil }
func (nopClient) Upload(context.Context, string, io.Reader) error             { return nil }
func (nopClient) Close() error                                                { return nil }

// swap replaces a seam and returns the function that puts it back.
func swap[T any](p *T, v T) func() {
	old := *p
	*p = v
	return func() { *p = old }
}

// bootstrapStubs says what the machine on the other end does.
type bootstrapStubs struct {
	keyWorks   bool
	proofFails bool
	agent      []keys.Offered
}

// bootstrapSteps records the decisions the flow took, so a test reads those
// rather than guessing from the output.
type bootstrapSteps struct {
	events           []string
	askedForPassword bool
	password         string
	installedKey     bool
	publicKey        string
	proved           bool
	provedWith       remote.Auth
	hardened         bool
	ansible          bool
	hostKey          ssh.PublicKey
}

func stubBootstrap(t *testing.T, s bootstrapStubs) *bootstrapSteps {
	t.Helper()
	steps := &bootstrapSteps{}
	steps.hostKey = commandHostKey(t)
	trustRecorded := false

	t.Cleanup(swap(&scanHostKey, func(_ context.Context, m config.Machine) (ssh.PublicKey, string, error) {
		steps.events = append(steps.events, "scan host key")
		return steps.hostKey, m.Hosts[0].Address, nil
	}))
	t.Cleanup(swap(&confirmHostKey, func(_ io.Reader, _ io.Writer, _ string) (bool, error) {
		steps.events = append(steps.events, "ask trust")
		return true, nil
	}))

	t.Cleanup(swap(&dialWith, func(_ context.Context, m config.Machine, _ string, a remote.Auth) (remote.Client, string, error) {
		address := m.Hosts[0].Address
		if !trustRecorded {
			store, err := hostkeys.Open(m.KnownHostsFile)
			if err != nil {
				t.Errorf("opening trust before authentication: %v", err)
			} else if err := store.Check(m.Name, m.Port, steps.hostKey); err != nil {
				t.Errorf("host key was not written before authentication: %v", err)
			}
			steps.events = append(steps.events, "write trust")
			trustRecorded = true
		}
		if a.Password != "" {
			steps.events = append(steps.events, "optional password authentication")
			steps.askedForPassword = true
			steps.password = a.Password
			return nopClient{}, address, nil
		}
		steps.events = append(steps.events, "attempt key authentication")
		if !s.keyWorks {
			return nil, "", fmt.Errorf("%w: the machine refused the key", remote.ErrAuthRefused)
		}
		return nopClient{}, address, nil
	}))
	t.Cleanup(swap(&installKey, func(_ context.Context, _ remote.Client, publicKey string) error {
		steps.installedKey = true
		steps.publicKey = publicKey
		return nil
	}))
	t.Cleanup(swap(&proveAuth, func(_ context.Context, _ config.Machine, _ string, a remote.Auth) (remote.Client, error) {
		if s.proofFails {
			return nil, errors.New("the key is installed but does not log in: check authorized_keys and AuthorizedKeysFile")
		}
		steps.proved = true
		steps.events = append(steps.events, "prove key")
		steps.provedWith = a
		return nopClient{}, nil
	}))
	t.Cleanup(swap(&installAnsible, func(context.Context, remote.Client, io.Writer) error {
		steps.ansible = true
		steps.events = append(steps.events, "install ansible")
		return nil
	}))
	t.Cleanup(swap(&harden, func(context.Context, remote.Client) error {
		steps.hardened = true
		steps.events = append(steps.events, "harden")
		return nil
	}))
	t.Cleanup(swap(&agentKeys, func() ([]keys.Offered, error) { return s.agent, nil }))

	return steps
}

func commandHostKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// answers replies to setup's questions, in order: machine name, address,
// administrative login, port, domain, then how to log in.
func answers(lines ...string) io.Reader {
	return strings.NewReader(strings.Join(lines, "\n") + "\n")
}

// runSetupIn runs the whole flow against a configuration directory.
func runSetupIn(t *testing.T, dir string, in io.Reader, o setupOptions) (string, error) {
	t.Helper()
	out := &strings.Builder{}
	err := runSetup(context.Background(), dir, in, out, o)
	return out.String(), err
}

func TestSetupWithExistingConfigurationOnlyPreparesTheSelectedMachine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, config.FileName)
	original := []byte("machines:\n  - name: main\n    hosts: [203.0.113.10]\n  - name: sandbox\n    hosts: [198.51.100.7]\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	var dialed string
	t.Cleanup(swap(&dial, func(_ context.Context, m config.Machine, user string) (remote.Client, string, error) {
		dialed = m.Name + ":" + user
		return nopClient{}, m.Hosts[0].Address, nil
	}))
	ansible := false
	t.Cleanup(swap(&installAnsible, func(context.Context, remote.Client, io.Writer) error {
		ansible = true
		return nil
	}))
	t.Cleanup(swap(&scanHostKey, func(context.Context, config.Machine) (ssh.PublicKey, string, error) {
		t.Fatal("setup scanned a new host key for an existing configuration")
		return nil, "", nil
	}))
	t.Cleanup(swap(&installKey, func(context.Context, remote.Client, string) error {
		t.Fatal("setup installed a key for an existing configuration")
		return nil
	}))
	t.Cleanup(swap(&harden, func(context.Context, remote.Client) error {
		t.Fatal("setup hardened an existing machine")
		return nil
	}))

	out, err := execute(t, "--config", dir, "--machine", "sandbox", "setup")
	if err != nil {
		t.Fatalf("runSetup returned %v", err)
	}
	if dialed != "sandbox:root" || !ansible {
		t.Fatalf("dialed %q, installed Ansible %v", dialed, ansible)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(after, original) {
		t.Fatalf("setup rewrote config.yml:\n%s", after)
	}
	if !strings.Contains(out, "without rewriting") || !strings.Contains(out, "Ansible") {
		t.Fatalf("setup did not explain the resume path: %q", out)
	}
}

func TestSetupWithoutAPackageReleasePrintsTheSkillsFollowUp(t *testing.T) {
	stubBootstrap(t, bootstrapStubs{keyWorks: true})

	out, err := runSetupIn(t, t.TempDir(),
		answers("main", "203.0.113.10", "root", "22", "", "1"), setupOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "devmachine skills add") || !strings.Contains(out, "package release") {
		t.Fatalf("missing skills follow-up: %q", out)
	}
}

func TestSetupWithAPackageReleaseOffersLocalSkills(t *testing.T) {
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\npackages: v9\n")
	t.Cleanup(swap(&dial, func(context.Context, config.Machine, string) (remote.Client, string, error) {
		return nopClient{}, "203.0.113.10", nil
	}))
	t.Cleanup(swap(&installAnsible, func(context.Context, remote.Client, io.Writer) error { return nil }))
	called := false
	t.Cleanup(swap(&setupSkills, func(_ context.Context, gotDir string, _ io.Reader, _ io.Writer) error {
		called = gotDir == dir
		return nil
	}))

	if _, err := runSetupIn(t, dir, strings.NewReader(""), setupOptions{}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("setup did not offer local skills after preparing a pinned configuration")
	}
}

func TestSetupWithAWorkingKeySkipsThePassword(t *testing.T) {
	steps := stubBootstrap(t, bootstrapStubs{keyWorks: true})

	out, err := runSetupIn(t, t.TempDir(),
		answers("main", "203.0.113.10", "root", "22", "example.com", "1"), setupOptions{})
	if err != nil {
		t.Fatalf("runSetup returned %v", err)
	}

	if steps.askedForPassword {
		t.Fatal("it asked for a password it did not need")
	}
	if !steps.hardened {
		t.Fatal("it did not harden")
	}
	if !strings.Contains(out, "already") {
		t.Fatalf("it does not say the key already worked: %q", out)
	}
}

func TestSetupFallsBackToThePassword(t *testing.T) {
	steps := stubBootstrap(t, bootstrapStubs{keyWorks: false})

	if _, err := runSetupIn(t, t.TempDir(),
		answers("main", "203.0.113.10", "root", "22", "example.com", "1", "devmachine"),
		setupOptions{}); err != nil {
		t.Fatalf("runSetup returned %v", err)
	}

	if !steps.askedForPassword || !steps.installedKey || !steps.proved || !steps.hardened {
		t.Fatalf("got %#v", steps)
	}
	if steps.password != "devmachine" {
		t.Fatalf("password = %q", steps.password)
	}
}

func TestSetupTrustsTheHostBeforeAnyAuthentication(t *testing.T) {
	steps := stubBootstrap(t, bootstrapStubs{keyWorks: false})

	if _, err := runSetupIn(t, t.TempDir(),
		answers("main", "203.0.113.10", "root", "22", "", "1", "devmachine"),
		setupOptions{}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"scan host key",
		"ask trust",
		"write trust",
		"attempt key authentication",
		"optional password authentication",
		"prove key",
		"harden",
		"install ansible",
	}
	if !slices.Equal(steps.events, want) {
		t.Fatalf("events = %#v, want %#v", steps.events, want)
	}
}

func TestSetupHostTrustRefusalLeavesNoFilesOrAuthentication(t *testing.T) {
	dir := t.TempDir()
	steps := stubBootstrap(t, bootstrapStubs{keyWorks: false})
	t.Cleanup(swap(&confirmHostKey, func(_ io.Reader, _ io.Writer, _ string) (bool, error) {
		steps.events = append(steps.events, "ask trust")
		return false, nil
	}))

	_, err := runSetupIn(t, dir,
		answers("main", "203.0.113.10", "root", "22", ""), setupOptions{})
	if !errors.Is(err, errDeclined) {
		t.Fatalf("runSetup error = %v, want declined", err)
	}
	for _, path := range []string{
		filepath.Join(dir, config.FileName),
		filepath.Join(dir, config.KnownHostsFileName),
		keys.Dir(dir),
	} {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("refusal left %s", path)
		}
	}
	if !slices.Equal(steps.events, []string{"scan host key", "ask trust"}) {
		t.Fatalf("refusal events = %#v", steps.events)
	}
	if steps.installedKey || steps.proved || steps.hardened || steps.ansible {
		t.Fatalf("refusal bootstrapped: %#v", steps)
	}
}

// TestSetupProvesTheKeyBeforeHardening: the proof is the only thing that says
// the door still opens once the password is gone.
func TestSetupProvesWithTheKeyAloneAndInstallsWhatItProves(t *testing.T) {
	dir := t.TempDir()
	steps := stubBootstrap(t, bootstrapStubs{keyWorks: false})

	if _, err := runSetupIn(t, dir,
		answers("main", "203.0.113.10", "root", "22", "", "1", "devmachine"),
		setupOptions{}); err != nil {
		t.Fatalf("runSetup returned %v", err)
	}

	generated := filepath.Join(keys.Dir(dir), "main")
	if steps.provedWith.KeyPath != generated || steps.provedWith.Password != "" {
		t.Fatalf("the proof did not use the key alone: %#v", steps.provedWith)
	}
	public, err := os.ReadFile(generated + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(public)) != steps.publicKey {
		t.Fatalf("it installed %q, not the key it generated", steps.publicKey)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Machines[0].Key != generated {
		t.Fatalf("the configuration does not point at the key it made: %q", cfg.Machines[0].Key)
	}
}

func TestSetupDoesNotHardenWhenTheProofFails(t *testing.T) {
	steps := stubBootstrap(t, bootstrapStubs{keyWorks: false, proofFails: true})

	out, err := runSetupIn(t, t.TempDir(),
		answers("main", "203.0.113.10", "root", "22", "", "1", "devmachine"), setupOptions{})
	if err == nil {
		t.Fatal("a failed proof was reported as success")
	}

	// Turning passwords off after a failed proof locks the door with the key
	// still inside.
	if steps.hardened {
		t.Fatal("it hardened without proving the key")
	}
	if !strings.Contains(out, "password login is still on") {
		t.Fatalf("it does not say the machine is still reachable: %q", out)
	}
}

func TestSetupNeverWritesThePasswordAnywhere(t *testing.T) {
	dir := t.TempDir()
	stubBootstrap(t, bootstrapStubs{keyWorks: false})

	if _, err := runSetupIn(t, dir,
		answers("main", "203.0.113.10", "root", "22", "example.com", "1", "hunter2"),
		setupOptions{}); err != nil {
		t.Fatalf("runSetup returned %v", err)
	}

	// Not in config.yml, not in the lock, not in the history log. The password
	// lives in memory, is used once, and is never the CLI's to keep.
	found := false
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(string(body), "hunter2") {
			t.Errorf("the password reached %s", path)
			found = true
		}
		return nil
	})
	if found {
		t.FailNow()
	}
}

func TestSetupOffersTheKeysTheAgentHolds(t *testing.T) {
	// Somebody whose key lives in a password manager has no file to point at,
	// and the CLI must not require one.
	dir := t.TempDir()
	steps := stubBootstrap(t, bootstrapStubs{keyWorks: false, agent: []keys.Offered{
		{Fingerprint: "SHA256:aaaa", Comment: "alice laptop", PublicKey: "ssh-ed25519 AAAAagent alice laptop"},
	}})

	out, err := runSetupIn(t, dir,
		answers("main", "203.0.113.10", "root", "22", "", "3", "devmachine"), setupOptions{})
	if err != nil {
		t.Fatalf("runSetup returned %v", err)
	}

	if steps.publicKey != "ssh-ed25519 AAAAagent alice laptop" {
		t.Fatalf("it installed %q, not the key the agent holds", steps.publicKey)
	}

	for _, want := range []string{"SHA256:aaaa", "alice laptop"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the agent's key was not offered: %q", out)
		}
	}
	if !steps.provedWith.Agent || steps.provedWith.KeyPath != "" {
		t.Fatalf("it did not log in through the agent: %#v", steps.provedWith)
	}

	// An agent key has no path, so the configuration says nothing about a key
	// file: an empty `key` is what makes the CLI ask the agent later.
	cfg, _ := config.Load(dir)
	if cfg.Machines[0].Key != "" {
		t.Fatalf("key = %q, want empty", cfg.Machines[0].Key)
	}
}

func TestSetupWithNoAgentStillOffersTheOtherTwoWays(t *testing.T) {
	// No agent is one of the two ordinary cases, not a failure to report.
	stubBootstrap(t, bootstrapStubs{keyWorks: true})

	out, err := runSetupIn(t, t.TempDir(),
		answers("main", "203.0.113.10", "root", "22", "", "1"), setupOptions{})
	if err != nil {
		t.Fatalf("runSetup returned %v", err)
	}
	if strings.Contains(strings.ToLower(out), "no ssh agent") {
		t.Fatalf("an absent agent was reported as a problem: %q", out)
	}
}

func TestSetupUsesAKeyFileAlreadyOnDisk(t *testing.T) {
	dir := t.TempDir()
	elsewhere := t.TempDir()
	path, public, err := keys.Generate(elsewhere, "id_ed25519")
	if err != nil {
		t.Fatal(err)
	}
	steps := stubBootstrap(t, bootstrapStubs{keyWorks: false})

	if _, err := runSetupIn(t, dir,
		answers("main", "203.0.113.10", "root", "22", "", "2", path, "devmachine"),
		setupOptions{}); err != nil {
		t.Fatalf("runSetup returned %v", err)
	}

	if steps.publicKey != public {
		t.Fatalf("it installed %q, not the key it was pointed at", steps.publicKey)
	}
	cfg, _ := config.Load(dir)
	if cfg.Machines[0].Key != path {
		t.Fatalf("key = %q", cfg.Machines[0].Key)
	}
}

func TestSetupReusesTheKeyItAlreadyMadeForThatMachine(t *testing.T) {
	// Running setup again on a machine it already owns is the common case, and
	// generating over the old key would lock it out of that machine forever.
	dir := t.TempDir()
	path, public, err := keys.Generate(keys.Dir(dir), "main")
	if err != nil {
		t.Fatal(err)
	}
	steps := stubBootstrap(t, bootstrapStubs{keyWorks: false})

	if _, err := runSetupIn(t, dir,
		answers("main", "203.0.113.10", "root", "22", "", "1", "devmachine"),
		setupOptions{}); err != nil {
		t.Fatalf("runSetup returned %v", err)
	}

	if steps.publicKey != public {
		t.Fatalf("it did not reuse the key at %s: %q", path, steps.publicKey)
	}
}

func TestSetupNoHardenSkipsIt(t *testing.T) {
	steps := stubBootstrap(t, bootstrapStubs{keyWorks: false})

	out, err := runSetupIn(t, t.TempDir(),
		answers("main", "203.0.113.10", "root", "22", "", "1", "devmachine"),
		setupOptions{noHarden: true})
	if err != nil {
		t.Fatalf("runSetup returned %v", err)
	}

	if steps.hardened {
		t.Fatal("--no-harden hardened anyway")
	}
	if !steps.proved {
		t.Fatal("--no-harden also skipped the proof")
	}
	if !strings.Contains(out, "password login") {
		t.Fatalf("it does not say password login was left alone: %q", out)
	}
}

func TestSetupStopsWhenNothingAnswers(t *testing.T) {
	// A machine nobody can reach is not a machine whose password is worth
	// asking for.
	steps := &bootstrapSteps{}
	t.Cleanup(swap(&dialWith, func(context.Context, config.Machine, string, remote.Auth) (remote.Client, string, error) {
		return nil, "", errors.New("no address answered")
	}))
	t.Cleanup(swap(&agentKeys, func() ([]keys.Offered, error) { return nil, nil }))

	_, err := runSetupIn(t, t.TempDir(),
		answers("main", "203.0.113.10", "root", "22", "", "1"), setupOptions{})
	if err == nil {
		t.Fatal("an unreachable machine was reported as set up")
	}
	if steps.askedForPassword {
		t.Fatal("it asked for a password for a machine that never answered")
	}
}

func TestSetupWritesAConfigurationThatLoads(t *testing.T) {
	dir := t.TempDir()
	stubBootstrap(t, bootstrapStubs{keyWorks: true})

	if _, err := runSetupIn(t, dir,
		answers("sandbox", "198.51.100.7", "root", "2222", "example.com", "1"),
		setupOptions{}); err != nil {
		t.Fatalf("runSetup returned %v", err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("the configuration it wrote does not load: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("the configuration it wrote is invalid: %v", err)
	}

	if len(cfg.Machines) != 1 {
		t.Fatalf("machines = %#v", cfg.Machines)
	}
	m := cfg.Machines[0]
	if m.Name != "sandbox" || m.Hosts[0].Address != "198.51.100.7" || m.Port != 2222 {
		t.Fatalf("machine = %#v", m)
	}
	if cfg.Domain != "example.com" {
		t.Fatalf("domain = %q", cfg.Domain)
	}
}

func TestSetupTakesTheDefaultsOnEmptyAnswers(t *testing.T) {
	dir := t.TempDir()
	stubBootstrap(t, bootstrapStubs{keyWorks: true})

	// Only the address is typed; everything else is left blank.
	if _, err := runSetupIn(t, dir,
		answers("", "203.0.113.10", "", "", "", ""), setupOptions{}); err != nil {
		t.Fatalf("runSetup returned %v", err)
	}

	cfg, _ := config.Load(dir)
	m := cfg.Machines[0]
	if m.Name != "main" || m.User != config.DefaultAdminUser || m.Port != config.DefaultPort {
		t.Fatalf("the defaults were not applied: %#v", m)
	}
}

func TestSetupRefusesAnAddressThatWasNotGiven(t *testing.T) {
	dir := t.TempDir()

	if _, err := runSetupIn(t, dir, answers("main", "", "root", "22", ""), setupOptions{}); err == nil {
		t.Fatal("expected an error when no address was given")
	}
	if _, err := os.Stat(filepath.Join(dir, config.FileName)); err == nil {
		t.Fatal("it wrote a configuration it had already refused")
	}
}

func TestSetupRefusesAPortThatIsNotANumber(t *testing.T) {
	if _, err := runSetupIn(t, t.TempDir(),
		answers("main", "203.0.113.10", "root", "not-a-port", ""), setupOptions{}); err == nil {
		t.Fatal("expected an error for a port that is not a number")
	}
}

func TestSetupExistingDoesNotOverwriteWhenPreparationFails(t *testing.T) {
	dir := t.TempDir()
	existing := "machines:\n  - name: keep-me\n    hosts: [203.0.113.99]\n"
	if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(swap(&dial, func(context.Context, config.Machine, string) (remote.Client, string, error) {
		return nil, "", errors.New("unreachable")
	}))

	_, err := runSetupIn(t, dir, answers("main", "203.0.113.10", "root", "22", ""), setupOptions{})
	if err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Fatalf("error = %v, want connection failure", err)
	}

	after, readErr := os.ReadFile(filepath.Join(dir, config.FileName))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != existing {
		t.Fatal("the existing configuration was overwritten")
	}
}

func TestSetupOverwritesWithForce(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, config.FileName),
		[]byte("machines:\n  - name: old\n    hosts: [203.0.113.99]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stubBootstrap(t, bootstrapStubs{keyWorks: true})

	if _, err := runSetupIn(t, dir,
		answers("new", "203.0.113.10", "root", "22", "", "1"), setupOptions{force: true}); err != nil {
		t.Fatalf("runSetup returned %v", err)
	}

	cfg, _ := config.Load(dir)
	if cfg.Machines[0].Name != "new" {
		t.Fatalf("machine = %q", cfg.Machines[0].Name)
	}
}

func TestSetupSaysWhatItChangedOnTheMachine(t *testing.T) {
	stubBootstrap(t, bootstrapStubs{keyWorks: true})

	out, err := runSetupIn(t, t.TempDir(),
		answers("main", "203.0.113.10", "", "", "", "1"), setupOptions{})
	if err != nil {
		t.Fatalf("runSetup returned %v", err)
	}

	// setup now touches the server, and saying which parts it changed is the
	// difference between a command people trust and one they run scared.
	for _, want := range []string{"password login", "Next"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the output does not say what it did: %q", out)
		}
	}
}

func TestTheConfigurationFileIsNotReadableByOthers(t *testing.T) {
	dir := t.TempDir()
	stubBootstrap(t, bootstrapStubs{keyWorks: true})

	if _, err := runSetupIn(t, dir,
		answers("main", "203.0.113.10", "", "", "", "1"), setupOptions{}); err != nil {
		t.Fatalf("runSetup returned %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("mode = %o, want 600", perm)
	}
}

func TestSetupInstallsAnsible(t *testing.T) {
	// The imperative shell surface ends here. Everything after it is a play,
	// so a machine without Ansible is a machine `sync` cannot reach.
	steps := stubBootstrap(t, bootstrapStubs{keyWorks: false})

	if _, err := runSetupIn(t, t.TempDir(),
		answers("main", "203.0.113.10", "root", "22", "", "1", "devmachine"),
		setupOptions{}); err != nil {
		t.Fatalf("runSetup returned %v", err)
	}
	if !steps.ansible {
		t.Fatal("it left the machine without Ansible")
	}
}

func TestSetupInstallsAnsibleEvenWithNoHarden(t *testing.T) {
	// --no-harden is about password login, not about leaving the job half
	// done.
	steps := stubBootstrap(t, bootstrapStubs{keyWorks: true})

	if _, err := runSetupIn(t, t.TempDir(),
		answers("main", "203.0.113.10", "root", "22", "", "1"),
		setupOptions{noHarden: true}); err != nil {
		t.Fatalf("runSetup returned %v", err)
	}
	if !steps.ansible {
		t.Fatal("--no-harden also skipped Ansible")
	}
}

func TestSetupInstallsNothingWhenTheProofFails(t *testing.T) {
	steps := stubBootstrap(t, bootstrapStubs{keyWorks: false, proofFails: true})

	if _, err := runSetupIn(t, t.TempDir(),
		answers("main", "203.0.113.10", "root", "22", "", "1", "devmachine"),
		setupOptions{}); err == nil {
		t.Fatal("a failed proof was reported as success")
	}
	if steps.ansible {
		t.Fatal("it carried on installing on a machine it could not prove it owns")
	}
}

func TestSetupSeedsTheDefaultPackagesForAWorkspace(t *testing.T) {
	dir := t.TempDir()
	stubBootstrap(t, bootstrapStubs{keyWorks: true})

	if _, err := runSetupIn(t, dir,
		answers("main", "203.0.113.10", "root", "22", "example.com", "1"),
		setupOptions{}); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	// The default lives in the person's own file, so changing one line
	// changes every workspace made afterwards.
	if !slices.Equal(cfg.Defaults.Workspace, config.DefaultWorkspacePackages) {
		t.Fatalf("got %#v", cfg.Defaults.Workspace)
	}
}
