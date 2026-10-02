package commands

import (
	"strings"
	"testing"

	"github.com/mydevmachine/devmachine/internal/dns"
)

func configPublishingWithoutProvider(t *testing.T) string {
	t.Helper()
	dir := configWith(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n    packages: [caddy]\n"+
		"workspaces:\n  - name: alice\n    routes: [{host: app.example.com, port: 8080}]\n")
	writeCaddyPackage(t, dir)
	return dir
}

func TestExposeRmWithNoProviderSaysTheRecordWasNotRemoved(t *testing.T) {
	dialExpose(t, &exposeClient{files: map[string]string{"alice-routes.caddy": "app.example.com {\n}\n"}})
	dir := configPublishingWithoutProvider(t)

	got, out := rmJSON(t, dir, "app.example.com", "--yes")

	if !strings.Contains(got.DNSError, "No DNS provider is installed on main") {
		t.Fatalf("dns_error does not say the record is still there: %+v\n%s", got, out)
	}
}

func TestExposeRmWithTheMachineDownReportsTheRecordInDNSError(t *testing.T) {
	machineDown(t)
	dir := configPublishing(t)

	out, err := execute(t, "--config", dir, "--format", "json", "expose", "rm", "app.example.com", "--yes")
	if err != nil {
		t.Fatal(err, out)
	}
	if !strings.Contains(lastJSON(out), `"dns_error": "the DNS record was not removed: main could not be reached`) {
		t.Fatalf("%s", out)
	}
}

// A name that holds the machine's address and another one is not deleted:
// some providers replace the whole set to take one value out, and a failure
// halfway would take the other value down too.
func TestExposeRmLeavesANameThatAlsoHoldsAnotherValueToBeRemovedByHand(t *testing.T) {
	client := &exposeClient{zones: []string{"example.com"}, records: []dns.Record{
		{Name: "app", Type: "A", Value: "203.0.113.10"},
		{Name: "app", Type: "A", Value: "198.51.100.7"},
	}}
	dialExpose(t, client)
	dir := configPublishing(t)

	got, out := rmJSON(t, dir, "app.example.com", "--yes")

	if len(client.deleted) != 0 {
		t.Fatalf("a name with another value was deleted through the provider: %q", client.deleted)
	}
	if !strings.Contains(got.DNSError, "198.51.100.7") {
		t.Fatalf("dns_error does not say why: %+v", got)
	}
	if !strings.Contains(out, "Remove this record by hand") || !strings.Contains(out, "app (in example.com)\tA\t203.0.113.10") {
		t.Fatalf("it does not print the record to remove by hand:\n%s", out)
	}
}

func TestWorkspacesDestroyWithNoProviderWarnsTheRecordWasNotRemoved(t *testing.T) {
	dialExpose(t, &exposeClient{})
	dir := configPublishingWithoutProvider(t)

	out, err := execute(t, "--config", dir, "workspaces", "destroy", "alice", "--confirm", "alice")
	if err != nil {
		t.Fatal(err, out)
	}
	if !strings.Contains(out, "warning: the DNS record for app.example.com was not removed") {
		t.Fatalf("%s", out)
	}
}

func TestExposeRmAsksAboutTheDNSRecordToo(t *testing.T) {
	client := &exposeClient{zones: []string{"example.com"},
		records: []dns.Record{{Name: "app", Type: "A", Value: "203.0.113.10"}}}
	dialExpose(t, client)
	dir := configPublishing(t)

	out, err := executeWithInput(t, "n\n", "--config", dir, "expose", "rm", "app.example.com")
	if err == nil {
		t.Fatal("a no went ahead")
	}
	if !strings.Contains(out, "and remove its DNS record while it points at main") {
		t.Fatalf("the question does not mention the DNS record: %q", out)
	}
}

func TestExposeRmKeepDNSLeavesTheRecordAndSaysHowToRemoveIt(t *testing.T) {
	client := &exposeClient{zones: []string{"example.com"},
		records: []dns.Record{{Name: "app", Type: "A", Value: "203.0.113.10"}}}
	dialExpose(t, client)
	dir := configPublishing(t)

	out, err := executeWithInput(t, "y\n", "--config", dir, "expose", "rm", "app.example.com", "--keep-dns")
	if err != nil {
		t.Fatal(err, out)
	}
	if len(client.deleted) != 0 || client.lists != 0 {
		t.Fatalf("--keep-dns asked the DNS provider: %d lists, %q", client.lists, client.deleted)
	}
	if strings.Contains(out, "remove its DNS record") {
		t.Fatalf("the question still offers to remove the record: %q", out)
	}
	if !strings.Contains(out, "left alone (--keep-dns)") ||
		!strings.Contains(out, "devmachine dns rm app.example.com A 203.0.113.10 --machine main") {
		t.Fatalf("%s", out)
	}
}

func TestExposeRmCheckWithKeepDNSShowsNoDNSRemoval(t *testing.T) {
	client := &exposeClient{zones: []string{"example.com"},
		records: []dns.Record{{Name: "app", Type: "A", Value: "203.0.113.10"}}}
	dialExpose(t, client)
	dir := configPublishing(t)

	out, err := execute(t, "--config", dir, "expose", "rm", "app.example.com", "--check", "--keep-dns")
	if err != nil {
		t.Fatal(err, out)
	}
	if strings.Contains(out, "would remove app A") || !strings.Contains(out, "left alone (--keep-dns)") {
		t.Fatalf("%s", out)
	}
}

func TestWorkspacesDestroyKeepDNSLeavesTheRecords(t *testing.T) {
	client := &exposeClient{zones: []string{"example.com"},
		records: []dns.Record{{Name: "app", Type: "A", Value: "203.0.113.10"}}}
	dialExpose(t, client)
	dir := configDestroying(t)

	out, err := execute(t, "--config", dir, "workspaces", "destroy", "alice", "--confirm", "alice", "--keep-dns")
	if err != nil {
		t.Fatal(err, out)
	}
	if len(client.deleted) != 0 || client.lists != 0 {
		t.Fatalf("--keep-dns asked the DNS provider: %d lists, %q", client.lists, client.deleted)
	}
	if strings.Contains(out, "and its DNS record while") || !strings.Contains(out, "left alone (--keep-dns)") {
		t.Fatalf("%s", out)
	}
}
