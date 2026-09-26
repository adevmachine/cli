package aliases

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/hostkeys"
	"golang.org/x/crypto/ssh"
)

// withResolver replaces the address lookup, so a test of what is written does
// not depend on whether this computer is on a tailnet.
func withResolver(t *testing.T, addresses map[string][]string) {
	t.Helper()
	was := resolve
	resolve = func(m config.Machine) ([]string, error) {
		if found, ok := addresses[m.Name]; ok {
			return found, nil
		}
		return nil, errNoAddress(m.Name)
	}
	t.Cleanup(func() { resolve = was })
}

func fileWith(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func read(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func pinMachine(t *testing.T, m *config.Machine, spaced bool) {
	t.Helper()
	dir := t.TempDir()
	if spaced {
		dir = filepath.Join(dir, "config with spaces")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	m.KnownHostsFile = filepath.Join(dir, "known hosts")
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(private.Public())
	if err != nil {
		t.Fatal(err)
	}
	store, err := hostkeys.Open(m.KnownHostsFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(m.Name, m.Port, key); err != nil {
		t.Fatal(err)
	}
}

// twoAddresses is a machine reached over a tailnet first and a public address
// when that is down.
func twoAddresses(t *testing.T) config.Config {
	t.Helper()
	withResolver(t, map[string][]string{"main": {"100.64.0.5"}})
	cfg := config.Config{
		Machines: []config.Machine{{
			Name:  "main",
			Hosts: []config.Host{{Address: "tailscale:main"}, {Address: "203.0.113.10"}},
			User:  "root", Port: 22, Key: "/keys/main",
		}},
		Workspaces: []config.Workspace{{Name: "alice", Machine: "main"}},
	}
	pinMachine(t, &cfg.Machines[0], false)
	return cfg
}

func oneAddress(t *testing.T) config.Config {
	t.Helper()
	withResolver(t, map[string][]string{"main": {"203.0.113.10"}})
	cfg := config.Config{
		Machines: []config.Machine{{
			Name: "main", Hosts: []config.Host{{Address: "203.0.113.10"}},
			User: "root", Port: 22,
		}},
		Workspaces: []config.Workspace{{Name: "alice", Machine: "main"}},
	}
	pinMachine(t, &cfg.Machines[0], false)
	return cfg
}

func TestWriteReplacesOnlyTheManagedBlock(t *testing.T) {
	path := fileWith(t, `Host work-jump
    HostName jump.example.com

`+Begin+`
Host old-devmachine
`+End+`

Host sandbox
    HostName 203.0.113.99
`)

	if err := Write(path, "Host alice-devmachine\n"); err != nil {
		t.Fatal(err)
	}

	body := read(t, path)
	// ~/.ssh/config holds hosts this CLI knows nothing about. Rewriting the
	// whole file deletes them.
	for _, want := range []string{"work-jump", "sandbox", "alice-devmachine"} {
		if !strings.Contains(body, want) {
			t.Fatalf("lost %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "old-devmachine") {
		t.Fatalf("the old block survived:\n%s", body)
	}
	if strings.Count(body, Begin) != 1 || strings.Count(body, End) != 1 {
		t.Fatalf("the markers were not kept to one pair:\n%s", body)
	}
}

func TestWritePrependsWhenThereIsNoBlockYet(t *testing.T) {
	path := fileWith(t, "Host work-jump\n    HostName jump.example.com\n")

	if err := Write(path, "Host alice-devmachine\n"); err != nil {
		t.Fatal(err)
	}

	body := read(t, path)
	if !strings.Contains(body, "work-jump") || !strings.Contains(body, "alice-devmachine") {
		t.Fatalf("got:\n%s", body)
	}
	if !strings.Contains(body, Begin) {
		t.Fatalf("the block is not marked:\n%s", body)
	}
	if strings.Index(body, Begin) > strings.Index(body, "Host work-jump") {
		t.Fatalf("managed defaults must come before user defaults:\n%s", body)
	}
}

func TestWriteMakesTheFileWhenThereIsNone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh", "config")

	if err := Write(path, "Host alice-devmachine\n"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, path), "alice-devmachine") {
		t.Fatal("it wrote nothing")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// ssh refuses to read a config anybody else can write.
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}

func TestWriteKeepsTheFilesOwnPermissions(t *testing.T) {
	path := fileWith(t, "Host work-jump\n")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Write(path, "Host alice-devmachine\n"); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}

func TestWriteTwiceLeavesOneBlock(t *testing.T) {
	path := fileWith(t, "Host work-jump\n")

	for i := 0; i < 2; i++ {
		if err := Write(path, "Host alice-devmachine\n"); err != nil {
			t.Fatal(err)
		}
	}
	if count := strings.Count(read(t, path), Begin); count != 1 {
		t.Fatalf("the block was written %d times:\n%s", count, read(t, path))
	}
}

func TestRenderGivesEveryAliasTheSameHostKeyAlias(t *testing.T) {
	block, err := Render(twoAddresses(t))
	if err != nil {
		t.Fatal(err)
	}
	// known_hosts is indexed by address. One machine on two addresses gives
	// two entries, and switching between them gives "Host key verification
	// failed". HostKeyAlias indexes by name instead.
	if strings.Count(block, "HostKeyAlias main-devmachine") != 2 {
		t.Fatalf("got:\n%s", block)
	}
}

func TestRenderPinsEveryHostKeySourceAndQuotesKnownHostsPath(t *testing.T) {
	cfg := oneAddress(t)
	pinMachine(t, &cfg.Machines[0], true)
	block, err := Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"StrictHostKeyChecking yes",
		`UserKnownHostsFile "` + cfg.Machines[0].KnownHostsFile + `"`,
		"GlobalKnownHostsFile /dev/null",
		"HostKeyAlias main-devmachine",
		"UpdateHostKeys no", "CheckHostIP no", "VerifyHostKeyDNS no",
		"KnownHostsCommand none", "HostKeyAlgorithms ssh-ed25519",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("missing %q:\n%s", want, block)
		}
	}
	for _, forbidden := range []string{"StrictHostKeyChecking no", "accept-new", "UserKnownHostsFile /dev/null"} {
		if strings.Contains(block, forbidden) {
			t.Fatalf("unsafe %q:\n%s", forbidden, block)
		}
	}
}

func TestRenderWritesAPubAliasOnlyWhenThereIsAFallback(t *testing.T) {
	// Somebody with one address never sees a `-pub` alias.
	block, err := Render(oneAddress(t))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(block, "-pub") {
		t.Fatalf("got:\n%s", block)
	}
}

func TestRenderPointsThePubAliasAtTheLiteralAddress(t *testing.T) {
	block, err := Render(twoAddresses(t))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(block, "Host alice-devmachine-pub") {
		t.Fatalf("got:\n%s", block)
	}
	after := block[strings.Index(block, "Host alice-devmachine-pub"):]
	if !strings.Contains(after, "HostName 203.0.113.10") {
		t.Fatalf("the fallback is not the public address:\n%s", block)
	}
}

func TestRenderLeavesOutAPubAliasThatWouldRepeatThePrimary(t *testing.T) {
	// The tailnet is down, so the primary already resolved to the public
	// address. A second alias for the same address helps nobody.
	withResolver(t, map[string][]string{"main": {"203.0.113.10"}})
	cfg := config.Config{
		Machines: []config.Machine{{
			Name:  "main",
			Hosts: []config.Host{{Address: "tailscale:main"}, {Address: "203.0.113.10"}},
			User:  "root", Port: 22,
		}},
		Workspaces: []config.Workspace{{Name: "alice", Machine: "main"}},
	}
	pinMachine(t, &cfg.Machines[0], false)

	block, err := Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(block, "-pub") {
		t.Fatalf("got:\n%s", block)
	}
}

func TestRenderNamesTheWorkspaceUser(t *testing.T) {
	withResolver(t, map[string][]string{"main": {"203.0.113.10"}})
	cfg := config.Config{
		Machines:   []config.Machine{{Name: "main", Hosts: []config.Host{{Address: "203.0.113.10"}}, User: "root", Port: 2222}},
		Workspaces: []config.Workspace{{Name: "alice", Machine: "main", User: "alice2"}},
	}
	pinMachine(t, &cfg.Machines[0], false)

	block, err := Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Host alice-devmachine", "User alice2", "Port 2222"} {
		if !strings.Contains(block, want) {
			t.Fatalf("the block is missing %q:\n%s", want, block)
		}
	}
}

func TestRenderOffersTheMachinesKeyAndOnlyThat(t *testing.T) {
	block, err := Render(twoAddresses(t))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(block, `IdentityFile "/keys/main"`) {
		t.Fatalf("got:\n%s", block)
	}
	// An agent holding many keys makes a server cut the connection at
	// MaxAuthTries before the one that works is ever offered.
	if !strings.Contains(block, "IdentitiesOnly yes") {
		t.Fatalf("got:\n%s", block)
	}
}

func TestRenderLeavesTheKeyToTheAgentWhenThereIsNoFile(t *testing.T) {
	block, err := Render(oneAddress(t))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(block, "IdentityFile") || strings.Contains(block, "IdentitiesOnly") {
		t.Fatalf("got:\n%s", block)
	}
}

func TestRenderWithNoWorkspaceIsEmpty(t *testing.T) {
	withResolver(t, map[string][]string{"main": {"203.0.113.10"}})
	cfg := config.Config{Machines: []config.Machine{{Name: "main", Hosts: []config.Host{{Address: "203.0.113.10"}}, User: "root", Port: 22}}}

	block, err := Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(block) != "" {
		t.Fatalf("got:\n%s", block)
	}
}

func TestRenderSaysWhichMachineHasNoAddressLeft(t *testing.T) {
	withResolver(t, map[string][]string{})
	cfg := config.Config{
		Machines:   []config.Machine{{Name: "main", Hosts: []config.Host{{Address: "tailscale:main"}}, User: "root", Port: 22}},
		Workspaces: []config.Workspace{{Name: "alice", Machine: "main"}},
	}

	_, err := Render(cfg)
	if err == nil {
		t.Fatal("it wrote an alias with nowhere to go")
	}
	if !strings.Contains(err.Error(), "main") {
		t.Fatalf("the error does not name the machine: %v", err)
	}
}

func TestListCarriesTheSameFactsAsTheBlock(t *testing.T) {
	found, err := List(twoAddresses(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatalf("got %#v", found)
	}
	if found[0].Name != "alice-devmachine" || found[0].Host != "100.64.0.5" {
		t.Fatalf("got %#v", found[0])
	}
	if found[1].Name != "alice-devmachine-pub" || found[1].Host != "203.0.113.10" {
		t.Fatalf("got %#v", found[1])
	}
	for _, a := range found {
		if a.HostKeyAlias != "main-devmachine" || a.User != "alice" || a.Port != 22 {
			t.Fatalf("got %#v", a)
		}
	}
}

func TestDefaultPathIsTheUsersOwnSSHConfig(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "config" || filepath.Base(filepath.Dir(path)) != ".ssh" {
		t.Fatalf("got %q", path)
	}
}

func TestWriteKeepsABlankLineBeforeWhatFollowsTheBlock(t *testing.T) {
	path := fileWith(t, Begin+"\nHost old-devmachine\n"+End+"\n\nHost sandbox\n    HostName 203.0.113.99\n")

	if err := Write(path, "Host alice-devmachine\n"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, path), End+"\n\nHost sandbox") {
		t.Fatalf("the host after the block was glued to it:\n%s", read(t, path))
	}
}

func TestListSkipsAWorkspaceOnASelfMachine(t *testing.T) {
	cfg := config.Config{
		Machines: []config.Machine{
			{Name: "mac", Self: true},
		},
		Workspaces: []config.Workspace{
			{Name: "alice", Machine: "mac"},
		},
	}
	found, err := List(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("a self machine's workspace produced an alias: %#v", found)
	}
}
