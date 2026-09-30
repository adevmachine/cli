package commands

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mydevmachine/devmachine/internal/packages"
)

// localNetworkPackage writes a network package into the configuration's own
// packages/, with trivial scripts: resolve runs on this computer in the test,
// join and self_name only ever run on a machine.
func localNetworkPackage(t *testing.T, dir, name, prefix, resolveBody string) {
	t.Helper()
	pkg := filepath.Join(packages.LocalDir(dir), name)
	files := map[string]string{
		"package.yml": "format: 1\nname: " + name + "\nscope: machine\nkind: vpn\nsummary: A private network.\n" +
			"variables:\n  login_server:\n    summary: Where it signs in.\n    default: ''\n" +
			"network:\n  prefix: " + prefix + "\n  resolve: bin/resolve\n  join: bin/join\n  self_name: bin/self-name\n",
		"tasks/main.yml": "---\n[]\n",
		"bin/resolve":    "#!/bin/sh\n" + resolveBody,
		"bin/join":       "#!/bin/sh\nexit 0\n",
		"bin/self-name":  "#!/bin/sh\necho main\n",
	}
	for rel, body := range files {
		path := filepath.Join(pkg, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func fakeDNS(t *testing.T, answers map[string][]string) {
	t.Helper()
	lookupIP = func(_ context.Context, host string) ([]string, error) {
		if ips, ok := answers[host]; ok {
			return ips, nil
		}
		return nil, errors.New("no such host")
	}
	t.Cleanup(func() { lookupIP = realLookupIP })
}

func TestResolvePrintsTheAddressesInTheOrderTheyAreTried(t *testing.T) {
	fakeDNS(t, map[string][]string{"vps.example.com": {"203.0.113.20"}})
	dir := configWith(t, "machines:\n  - name: main\n    packages: [acme-net]\n"+
		"    hosts: [acme:main, zerotier:main, vps.example.com, 203.0.113.10]\n")
	localNetworkPackage(t, dir, "acme-net", "acme", "echo 100.64.0.7\n")

	out, err := execute(t, "--config", dir, "resolve")
	if err != nil {
		t.Fatal(err)
	}
	order := []string{"100.64.0.7", "acme:main", "acme-net", "203.0.113.20", "vps.example.com", "203.0.113.10",
		"zerotier:main", "no package declares"}
	at := -1
	for _, want := range order {
		i := strings.Index(out[at+1:], want)
		if i < 0 {
			t.Fatalf("%q missing or out of order in:\n%s", want, out)
		}
		at += i + 1
	}
}

func TestResolveJSONIsTheShapeTheAppReads(t *testing.T) {
	fakeDNS(t, nil)
	dir := configWith(t, "machines:\n  - name: main\n    packages: [tailscale]\n"+
		"    hosts: [tailscale:main, tailscale:vps, 203.0.113.10]\n")
	localNetworkPackage(t, dir, "tailscale", "tailscale", `case "$1" in
main) echo 100.64.0.5 ;;
*) echo 'tailscale is not running' >&2; exit 3 ;;
esac
`)

	out, err := execute(t, "--config", dir, "resolve", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	var got any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v:\n%s", err, out)
	}
	var want any
	if err := json.Unmarshal([]byte(`{"machine":"main",
		"addresses":[{"address":"100.64.0.5","source":"tailscale:main","package":"tailscale"},
		             {"address":"203.0.113.10","source":"203.0.113.10","package":""}],
		"dropped":[{"source":"tailscale:vps","reason":"tailscale is not running"}]}`), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%s", out)
	}
}

func TestResolveJSONKeepsEmptyListsAsLists(t *testing.T) {
	fakeDNS(t, nil)
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n")

	out, err := execute(t, "--config", dir, "resolve", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"dropped": []`) {
		t.Fatalf("an empty list is not []:\n%s", out)
	}
}

func TestResolveDropsANameDNSCannotAnswer(t *testing.T) {
	fakeDNS(t, nil)
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [gone.example.com, 203.0.113.10]\n")

	out, err := execute(t, "--config", dir, "resolve", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"source": "gone.example.com"`) || !strings.Contains(out, "no such host") {
		t.Fatalf("the unresolvable name was not dropped with its reason:\n%s", out)
	}
}

func TestResolveRefusesYourOwnComputer(t *testing.T) {
	dir := configWith(t, "machines:\n  - name: laptop\n    self: true\n")

	if _, err := execute(t, "--config", dir, "resolve"); err == nil || !strings.Contains(err.Error(), "self: true") {
		t.Fatalf("got %v", err)
	}
}

func TestSSHProxyTriesTheAddressesInOrder(t *testing.T) {
	var tried []string
	proxy = func(_ context.Context, targets []string, _ io.Reader, _ io.Writer) (string, error) {
		tried = targets
		return targets[0], nil
	}
	t.Cleanup(func() { proxy = realProxy })
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [acme:main, 203.0.113.10]\n")
	localNetworkPackage(t, dir, "acme-net", "acme", "echo 100.64.0.7\n")

	if _, err := execute(t, "--config", dir, "ssh-proxy", "main", "2222"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(tried, ",") != "100.64.0.7:2222,203.0.113.10:2222" {
		t.Fatalf("tried %v", tried)
	}
}

func TestSSHProxyFallsThroughADroppedAddressToOneThatAnswers(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		conn, err := l.Accept()
		if err == nil {
			_, _ = conn.Write([]byte("SSH-2.0-fake\r\n"))
			_ = conn.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(l.Addr().String())
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [acme:main, 127.0.0.1]\n")
	localNetworkPackage(t, dir, "acme-net", "acme", "exit 3\n")

	out, err := executeWithInput(t, "", "--config", dir, "ssh-proxy", "main", port)
	if err != nil {
		t.Fatal(err)
	}
	if out != "SSH-2.0-fake\r\n" {
		t.Fatalf("got %q", out)
	}
}

func TestSSHProxyRefusesAPortThatIsNotANumber(t *testing.T) {
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n")

	if _, err := execute(t, "--config", dir, "ssh-proxy", "main", "ssh"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestSSHProxyIsNotPartOfTheSurface(t *testing.T) {
	var b strings.Builder
	if err := WriteSurface(&b); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "ssh-proxy") {
		t.Fatal("ssh-proxy is for ssh to call, not for people")
	}
	if !strings.Contains(b.String(), "devmachine resolve\n") {
		t.Fatal("resolve is missing from the surface")
	}
}
