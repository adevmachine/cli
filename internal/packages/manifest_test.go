package packages

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseManifestReadsEveryField(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "package.yml"), `
format: 1
name: sharing
scope: workspace
summary: File sharing, uploads by API key and downloads by public link.
requires:
  cli: ">= 0.2.0"
needs: [docker]
extends:
  caddy.sites.d: files/sharing.caddy
variables:
  port:
    summary: The port the container listens on.
    default: 53842
credentials:
  - name: sharing_api_key
    kind: secret
    scope: workspace
    env: SHARING_API_KEY
`)

	m, err := ParseManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Format != 1 {
		t.Fatalf("format is %d", m.Format)
	}
	if m.Name != "sharing" || m.Scope != ScopeWorkspace {
		t.Fatalf("got %#v", m)
	}
	if m.Requires.CLI != ">= 0.2.0" {
		t.Fatalf("requires.cli is %q", m.Requires.CLI)
	}
	if len(m.Needs) != 1 || m.Needs[0] != "docker" {
		t.Fatalf("needs %#v", m.Needs)
	}
	if m.Extends["caddy.sites.d"] != "files/sharing.caddy" {
		t.Fatalf("extends %#v", m.Extends)
	}
	if m.Variables["port"].Default != 53842 {
		t.Fatalf("default is %#v", m.Variables["port"].Default)
	}
	if len(m.Credentials) != 1 || m.Credentials[0].Env != "SHARING_API_KEY" {
		t.Fatalf("credentials %#v", m.Credentials)
	}
	if m.Credentials[0].Kind != KindSecret {
		t.Fatalf("kind is %q", m.Credentials[0].Kind)
	}
	if m.Path != dir {
		t.Fatalf("path is %q", m.Path)
	}
}

func TestParseManifestRemembersWhereEachFieldWas(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "package.yml"), "format: 1\nname: caddy\nscope: machine\nsummary: A proxy.\n")

	m, err := ParseManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Lines["scope"] != 3 {
		t.Fatalf("scope was on line %d, want 3", m.Lines["scope"])
	}
}

func TestParseManifestSaysWhichFileIsMalformed(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "package.yml"), "format: 1\nname: [unclosed\n")

	_, err := ParseManifest(dir)
	if err == nil {
		t.Fatal("malformed YAML was accepted")
	}
	if !strings.Contains(err.Error(), filepath.Join(dir, "package.yml")) {
		t.Fatalf("the error does not name the file: %v", err)
	}
}

func TestParseManifestSaysWhichFileIsMissing(t *testing.T) {
	dir := t.TempDir()

	_, err := ParseManifest(dir)
	if err == nil {
		t.Fatal("a directory with no manifest was accepted")
	}
	if !strings.Contains(err.Error(), FileName) {
		t.Fatalf("the error does not name the file: %v", err)
	}
}
