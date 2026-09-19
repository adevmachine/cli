// Package packages fetches, verifies, reads and checks recipes.
//
// A package is an Ansible role with one extra file beside it. Nothing is
// translated: what is written is what runs, so a failure points at the line
// somebody wrote rather than at generated YAML they have never seen.
package packages

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// FileName is the one file a role needs beside it to be a package.
const FileName = "package.yml"

// The scopes a package can declare. Scope is a property of the software, not a
// preference: Docker is installed once and serves everyone, Claude Code has a
// login per person.
const (
	ScopeMachine   = "machine"
	ScopeWorkspace = "workspace"
)

// The kinds of credential, and the only hard one is the first.
const (
	// KindLogin cannot be automated: a browser or a device code, and a person.
	KindLogin = "login"
	// KindSecret is a value somebody hands over once.
	KindSecret = "secret"
	// KindFile is a file somebody drops on the machine.
	KindFile = "file"
)

// ReadableFormats are the shapes of package.yml this CLI can read. A set, not
// a number: adding a field is not a new format, so most releases add nothing
// here, and the ones that do keep reading the old shape for as long as it is
// worth doing.
var ReadableFormats = []int{1}

// Variable is a value a package reads, and what it falls back to.
type Variable struct {
	Summary string `yaml:"summary"`
	Default any    `yaml:"default"`
}

// Credential is something a package's tool cannot work without, and how it is
// obtained.
//
// How belongs here rather than in the operator's configuration, because the
// package is the only thing that knows it: nobody else knows that this tool
// logs in with one command and leaves its session in one file.
type Credential struct {
	Name  string `yaml:"name"`
	Kind  string `yaml:"kind"`
	Scope string `yaml:"scope"`
	// Command is the interactive login to run, for KindLogin.
	Command string `yaml:"command"`
	// StoredAt is where the tool keeps the result, so doctor can look and a
	// later version can copy it. It is a claim, not a guarantee.
	StoredAt string `yaml:"stored_at"`
	// Env and Path are where a KindSecret or KindFile value is delivered.
	Env  string `yaml:"env"`
	Path string `yaml:"path"`
}

// Manifest is what package.yml holds.
type Manifest struct {
	// Format is the shape of this file, and it is required. Without it the
	// first change to the format would make every existing recipe fail in a
	// different way, none of them saying why.
	Format   int    `yaml:"format"`
	Name     string `yaml:"name"`
	Scope    string `yaml:"scope"`
	Summary  string `yaml:"summary"`
	Requires struct {
		CLI string `yaml:"cli"`
	} `yaml:"requires"`
	// Needs is ordering, declared. Never implied by the order of a list:
	// implied ordering is what made the original repository impossible to
	// reason about.
	Needs []string `yaml:"needs"`
	// Provides names places other packages may write into.
	Provides map[string]string `yaml:"provides"`
	// Extends is a contribution to a place another package said may be
	// written to. It is not a patch, and it cannot reach anywhere else.
	Extends       map[string]string   `yaml:"extends"`
	Variables     map[string]Variable `yaml:"variables"`
	Credentials   []Credential        `yaml:"credentials"`
	RequiresFiles []string            `yaml:"requires_files"`

	// Kind, Entrypoint and Commands make a package callable: the contract it
	// answers, the executable to call on the machine, and what that executable
	// accepts.
	//
	// Commands is a list, or the single entry "*" for anything. Nothing calls
	// them in this version; validating them now is what stops the first one
	// inventing its own shape.
	Kind       string   `yaml:"kind"`
	Entrypoint string   `yaml:"entrypoint"`
	Commands   []string `yaml:"commands"`

	// Path is the directory the manifest was read from, and Lines maps a
	// top-level field name onto the line it was written on.
	Path  string         `yaml:"-"`
	Lines map[string]int `yaml:"-"`
}

// ManifestPath is where a package's manifest lives.
func ManifestPath(dir string) string { return filepath.Join(dir, FileName) }

// ParseManifest reads the package.yml in dir.
func ParseManifest(dir string) (Manifest, error) {
	var m Manifest
	path := ManifestPath(dir)

	body, err := os.ReadFile(path)
	if err != nil {
		return m, fmt.Errorf("reading %s: %w", path, err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(body, &root); err != nil {
		return m, fmt.Errorf("parsing %s: %w", path, err)
	}
	if err := root.Decode(&m); err != nil {
		return m, fmt.Errorf("parsing %s: %w", path, err)
	}

	m.Path = dir
	m.Lines = topLevelLines(&root)
	return m, nil
}

// topLevelLines maps each top-level key onto the line it was written on, so a
// validation message can point at it.
func topLevelLines(root *yaml.Node) map[string]int {
	lines := map[string]int{}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return lines
	}
	pairs := root.Content[0].Content
	for i := 0; i+1 < len(pairs); i += 2 {
		lines[pairs[i].Value] = pairs[i].Line
	}
	return lines
}
