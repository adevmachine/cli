package packages

import (
	"path/filepath"
	"strings"
	"testing"
)

const networkManifest = `format: 1
name: acme-net
scope: machine
kind: vpn
summary: Joins the machine to a private network.
network:
  prefix: acme
  resolve: bin/resolve
  join: bin/join
  self_name: bin/self-name
`

func networkPackage(t *testing.T, manifest string, scripts ...string) string {
	t.Helper()
	dir := writePackage(t, "acme-net", manifest)
	write(t, filepath.Join(dir, "tasks", "main.yml"), "---\n[]\n")
	for _, script := range scripts {
		writeMode(t, filepath.Join(dir, script), "#!/usr/bin/env python3\n", 0o755)
	}
	return dir
}

func TestParseManifestReadsTheNetworkBlock(t *testing.T) {
	dir := networkPackage(t, networkManifest, "bin/resolve", "bin/join", "bin/self-name")

	m, err := ParseManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := Network{Prefix: "acme", Resolve: "bin/resolve", Join: "bin/join", SelfName: "bin/self-name"}
	if m.Network == nil || *m.Network != want {
		t.Fatalf("got %#v", m.Network)
	}
}

func TestValidateAcceptsANetworkPackage(t *testing.T) {
	dir := networkPackage(t, networkManifest, "bin/resolve", "bin/join", "bin/self-name")

	problems, err := Validate(dir)
	if err != nil || len(problems) != 0 {
		t.Fatalf("%v %#v", err, problems)
	}
}

func TestValidateAcceptsANetworkThatOnlyResolves(t *testing.T) {
	manifest := strings.Replace(networkManifest, "  join: bin/join\n  self_name: bin/self-name\n", "", 1)
	dir := networkPackage(t, manifest, "bin/resolve")

	problems, err := Validate(dir)
	if err != nil || len(problems) != 0 {
		t.Fatalf("%v %#v", err, problems)
	}
}

func TestValidateRefusesANetworkWithNoPrefix(t *testing.T) {
	dir := networkPackage(t, strings.Replace(networkManifest, "  prefix: acme\n", "", 1),
		"bin/resolve", "bin/join", "bin/self-name")

	problems, err := Validate(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := problemAbout(t, problems, "network.prefix")
	if p.Line == 0 {
		t.Fatal("the problem does not point at the network block")
	}
}

func TestValidateRefusesAPrefixThatCannotBeWrittenInHosts(t *testing.T) {
	for _, prefix := range []string{"Acme", "ac me", "acme:", "1acme", "a.b"} {
		manifest := strings.Replace(networkManifest, "prefix: acme", "prefix: "+quote(prefix), 1)
		dir := networkPackage(t, manifest, "bin/resolve", "bin/join", "bin/self-name")

		problems, err := Validate(dir)
		if err != nil {
			t.Fatal(err)
		}
		problemAbout(t, problems, "network.prefix")
	}
}

func TestValidateRefusesANetworkWithNoResolve(t *testing.T) {
	dir := networkPackage(t, strings.Replace(networkManifest, "  resolve: bin/resolve\n", "", 1),
		"bin/join", "bin/self-name")

	problems, err := Validate(dir)
	if err != nil {
		t.Fatal(err)
	}
	problemAbout(t, problems, "network.resolve")
}

func TestValidateRefusesAJoinWithoutASelfName(t *testing.T) {
	dir := networkPackage(t, strings.Replace(networkManifest, "  self_name: bin/self-name\n", "", 1),
		"bin/resolve", "bin/join")

	problems, err := Validate(dir)
	if err != nil {
		t.Fatal(err)
	}
	problemAbout(t, problems, "network.self_name")
}

func TestValidateRefusesASelfNameWithoutAJoin(t *testing.T) {
	dir := networkPackage(t, strings.Replace(networkManifest, "  join: bin/join\n", "", 1),
		"bin/resolve", "bin/self-name")

	problems, err := Validate(dir)
	if err != nil {
		t.Fatal(err)
	}
	problemAbout(t, problems, "network.join")
}

func TestValidateRefusesANetworkOnAWorkspacePackage(t *testing.T) {
	manifest := strings.Replace(networkManifest, "scope: machine", "scope: workspace", 1)
	dir := networkPackage(t, manifest, "bin/resolve", "bin/join", "bin/self-name")

	problems, err := Validate(dir)
	if err != nil {
		t.Fatal(err)
	}
	problemAbout(t, problems, "machine package")
}

func TestValidateRefusesANetworkScriptThatIsNotThere(t *testing.T) {
	dir := networkPackage(t, networkManifest, "bin/resolve", "bin/join")

	problems, err := Validate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(problemAbout(t, problems, "is not in the package").What, "network.self_name") {
		t.Fatal("the message does not name the field")
	}
}

func TestValidateRefusesANetworkScriptThatIsNotExecutable(t *testing.T) {
	dir := networkPackage(t, networkManifest, "bin/join", "bin/self-name")
	writeMode(t, filepath.Join(dir, "bin", "resolve"), "#!/usr/bin/env python3\n", 0o644)

	problems, err := Validate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(problemAbout(t, problems, "network.resolve").What, "chmod") {
		t.Fatal("the message does not say how to fix it")
	}
}

func TestValidateRefusesANetworkScriptThatIsNotPython(t *testing.T) {
	dir := networkPackage(t, networkManifest, "bin/resolve", "bin/join")
	writeMode(t, filepath.Join(dir, "bin", "self-name"), "#!/bin/sh\n", 0o755)

	problems, err := Validate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(problemAbout(t, problems, "network.self_name").What, "python3") {
		t.Fatal("the message does not say what is expected")
	}
}

func TestValidateRefusesANetworkScriptOutsideThePackage(t *testing.T) {
	for _, path := range []string{"../resolve", "/usr/bin/resolve"} {
		manifest := strings.Replace(networkManifest, "resolve: bin/resolve", "resolve: "+path, 1)
		dir := networkPackage(t, manifest, "bin/join", "bin/self-name")

		problems, err := Validate(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(problemAbout(t, problems, "network.resolve").What, "inside the package") {
			t.Fatalf("%s: the message does not say where it has to be", path)
		}
	}
}

func quote(s string) string { return `"` + s + `"` }
