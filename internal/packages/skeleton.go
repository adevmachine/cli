package packages

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// SchemaField is one field of package.yml, as the CLI publishes it.
type SchemaField struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Summary  string `json:"summary"`
}

// SchemaDoc is the whole format, answered by `packages schema`.
//
// It is published by the binary rather than by a page, because a page drifts
// the moment the format changes and the validator cannot.
type SchemaDoc struct {
	Formats []int         `json:"formats"`
	Fields  []SchemaField `json:"fields"`
}

// schemaFields is the one table. A test asserts every yaml tag on Manifest
// appears here, so a new field cannot arrive undocumented.
var schemaFields = []SchemaField{
	{"format", true, "The shape of this file. This CLI reads format 1."},
	{"name", true, "The package's name, which has to be the directory it lives in."},
	{"scope", true, `Where it is installed: "machine" or "workspace".`},
	{"summary", true, "One line saying what it installs. It is what `packages list` prints."},
	{"requires", false, `Which CLI can run it, written as requires.cli: ">= 0.2.0".`},
	{"needs", false, "Packages that have to run before this one. It is the only thing that decides order."},
	{"provides", false, "Places other packages may write into, as <place>: <absolute path on the machine>."},
	{"extends", false, "Contributions to another package's place, as <package>.<place>: <path inside this package>."},
	{"variables", false, "Values this package reads, each with a summary and a default."},
	{"credentials", false, "What its tool cannot work without, and how each one is obtained."},
	{"requires_files", false, "Files that have to be on the machine before it runs."},
	{"kind", false, `The contract an entrypoint answers. The only one so far is "dns".`},
	{"entrypoint", false, "An executable in the package the CLI can call on the machine."},
	{"commands", false, `What the entrypoint accepts: a list, or ["*"] for anything.`},
}

// Schema returns the format this CLI reads.
func Schema() SchemaDoc {
	return SchemaDoc{Formats: ReadableFormats, Fields: schemaFields}
}

// skeletonManifest and the two files beside it are a package that already
// validates and already does something. A skeleton full of "add tasks here"
// teaches the wrong thing on the first run.
const skeletonManifest = `format: 1
name: {{.Name}}
scope: {{.Scope}}
summary: Replace this line with what {{.Name}} installs.
`

const skeletonTasks = `---
- name: Install what {{.Name}} needs
  package:
    name: "{{"{{"}} {{.Var}}_packages {{"}}"}}"
    state: present
  when: {{.Var}}_packages | length > 0
`

const skeletonDefaults = `---
{{.Var}}_packages: []
`

// WriteSkeleton writes a new package into dir.
//
// It refuses to write over one that is already there: `packages new` is how a
// package starts, never how one is edited.
func WriteSkeleton(dir, name, scope string) error {
	if !packageName.MatchString(name) {
		return fmt.Errorf("name %q: use lower case letters, digits, dashes and underscores", name)
	}
	if scope != ScopeMachine && scope != ScopeWorkspace {
		return fmt.Errorf("scope %q: a package is installed on a %s or in a %s, and there is no third place",
			scope, ScopeMachine, ScopeWorkspace)
	}
	if _, err := os.Stat(ManifestPath(dir)); err == nil {
		return fmt.Errorf("%s already exists", ManifestPath(dir))
	}

	data := struct{ Name, Scope, Var string }{name, scope, ansibleVariable(name)}
	for path, body := range map[string]string{
		FileName:                              skeletonManifest,
		filepath.Join("tasks", "main.yml"):    skeletonTasks,
		filepath.Join("defaults", "main.yml"): skeletonDefaults,
	} {
		rendered, err := render(body, data)
		if err != nil {
			return err
		}
		target := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(rendered), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func render(body string, data any) (string, error) {
	parsed, err := template.New("skeleton").Parse(body)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	if err := parsed.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}

// ansibleVariable turns a package name into something Ansible accepts: a dash
// is fine in a name and never fine in a variable.
func ansibleVariable(name string) string { return strings.ReplaceAll(name, "-", "_") }
