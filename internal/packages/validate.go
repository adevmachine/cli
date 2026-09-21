package packages

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Problem is one thing wrong with a package, and where it is.
type Problem struct {
	// File is relative to the package directory.
	File string `json:"file"`
	// Line is zero when the problem is not on a line.
	Line int    `json:"line,omitempty"`
	What string `json:"what"`
}

func (p Problem) Error() string {
	if p.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", p.File, p.Line, p.What)
	}
	return fmt.Sprintf("%s: %s", p.File, p.What)
}

// aptModule matches a task that calls apt directly.
//
// The rule that a package works beyond Debian stops being a convention
// somebody has to remember and becomes a validation error.
var aptModule = regexp.MustCompile(`^\s*(-\s*)?(ansible\.builtin\.)?(apt|apt_key|apt_repository)\s*:`)

var packageName = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// The one contract a package can answer so far, and the commands it owes.
const kindDNS = "dns"

var dnsCommands = []string{"zones", "list", "upsert", "delete", "help"}

// AnyCommand is what a package writes when its entrypoint takes anything.
const AnyCommand = "*"

// Validate returns everything wrong with the package in dir.
//
// It returns every problem rather than the first, because the caller is often
// a model correcting its own work: four problems in one answer is one round
// trip, four answers is four.
func Validate(dir string) ([]Problem, error) {
	m, err := ParseManifest(dir)
	if err != nil {
		return nil, err
	}

	var problems []Problem
	at := func(field, what string) {
		problems = append(problems, Problem{File: FileName, Line: m.Lines[field], What: what})
	}

	// The format is checked before anything else, and nothing else is reported
	// on top of it. Every other message assumes the fields mean what this CLI
	// thinks they mean, and that assumption is exactly what a format mismatch
	// breaks.
	switch {
	case m.Format == 0:
		at("format", fmt.Sprintf("every package needs `format`, and this CLI reads %s",
			joinInts(ReadableFormats)))
		return problems, nil
	case !slices.Contains(ReadableFormats, m.Format):
		at("format", fmt.Sprintf("format %d, and this CLI reads %s. Upgrade with `brew upgrade devmachine`",
			m.Format, joinInts(ReadableFormats)))
		return problems, nil
	}

	switch {
	case m.Name == "":
		at("name", "every package needs a `name`")
	case !packageName.MatchString(m.Name):
		at("name", fmt.Sprintf("name %q: use lower case letters, digits, dashes and underscores", m.Name))
	case m.Name != filepath.Base(dir):
		at("name", fmt.Sprintf("name is %q but the directory is %q: a package is found by its directory",
			m.Name, filepath.Base(dir)))
	}

	if m.Scope != ScopeMachine && m.Scope != ScopeWorkspace {
		at("scope", fmt.Sprintf("scope must be %q or %q, got %q", ScopeMachine, ScopeWorkspace, m.Scope))
	}
	if strings.TrimSpace(m.Summary) == "" {
		at("summary", "every package needs a one-line `summary`: it is what `packages list` prints")
	}
	if m.Requires.CLI != "" {
		if _, err := ParseConstraint(m.Requires.CLI); err != nil {
			at("requires", fmt.Sprintf("requires.cli %q: %v", m.Requires.CLI, err))
		}
	}

	problems = append(problems, validateEntrypoint(dir, m)...)
	problems = append(problems, validateCredentials(m)...)

	for _, point := range sortedKeys(m.Extends) {
		source := m.Extends[point]
		if !strings.Contains(point, ".") {
			at("extends", fmt.Sprintf(
				"extends %q: an extension point is written <package>.<place>, for example caddy.sites.d", point))
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, source)); err != nil {
			at("extends", fmt.Sprintf("extends %q points at %s, which is not in the package", point, source))
		}
	}

	for _, place := range sortedKeys(m.Provides) {
		if path := m.Provides[place]; !filepath.IsAbs(path) {
			at("provides", fmt.Sprintf("provides %q is %q: an extension point is an absolute path on the machine",
				place, path))
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "tasks", "main.yml")); err != nil {
		problems = append(problems, Problem{
			File: filepath.Join("tasks", "main.yml"),
			What: "a package is an Ansible role, so it needs tasks/main.yml",
		})
	}

	taskProblems, err := scanForApt(dir)
	if err != nil {
		return nil, err
	}
	return append(problems, taskProblems...), nil
}

// validateEntrypoint checks a package that says it can be called.
//
// A package with no entrypoint is an ordinary role and none of this applies.
func validateEntrypoint(dir string, m Manifest) []Problem {
	if m.Entrypoint == "" {
		if m.Kind != "" || len(m.Commands) > 0 {
			return []Problem{{File: FileName, Line: m.Lines["kind"],
				What: "`kind` and `commands` describe an `entrypoint`, and this package declares none"}}
		}
		return nil
	}

	var problems []Problem
	at := func(field, what string) {
		problems = append(problems, Problem{File: FileName, Line: m.Lines[field], What: what})
	}

	// A kind is a contract: everything that declares one answers the same
	// commands, which is what lets a later `devmachine dns` find a registrar
	// nobody named.
	if m.Kind != "" && m.Kind != kindDNS {
		at("kind", fmt.Sprintf("the only kind so far is %q, got %q", kindDNS, m.Kind))
	}
	switch {
	case len(m.Commands) == 0:
		at("commands", `a package with an entrypoint says what it accepts: a list, or ["*"] for anything`)
	case len(m.Commands) > 1 && slices.Contains(m.Commands, AnyCommand):
		at("commands", `"*" means anything, so listing it beside other commands says two things at once`)
	}
	if m.Kind == kindDNS && !slices.Contains(m.Commands, AnyCommand) {
		for _, required := range dnsCommands {
			if !slices.Contains(m.Commands, required) {
				at("commands", fmt.Sprintf("a dns package has to accept %q", required))
			}
		}
	}

	path := filepath.Join(dir, m.Entrypoint)
	info, err := os.Stat(path)
	if err != nil {
		at("entrypoint", fmt.Sprintf("entrypoint %q is not in the package", m.Entrypoint))
		return problems
	}
	if info.Mode()&0o111 == 0 {
		at("entrypoint", fmt.Sprintf("entrypoint %q is not executable: chmod +x it", m.Entrypoint))
	}

	// Ansible already requires Python on any machine this CLI provisions, so
	// python3 is always there and an entrypoint with no dependencies cannot
	// break on install. Checking the shebang makes that a rule the CLI
	// enforces rather than one a contributor remembers.
	if first, err := firstLine(path); err == nil && first != pythonShebang {
		at("entrypoint", fmt.Sprintf(
			"entrypoint %q starts with %q: an entrypoint is Python 3 and starts with %s",
			m.Entrypoint, first, pythonShebang))
	}
	return problems
}

const pythonShebang = "#!/usr/bin/env python3"

// validateCredentials checks that each declared credential says enough to be
// acted on. Nothing here runs in this version; a later one is what reads it.
func validateCredentials(m Manifest) []Problem {
	var problems []Problem
	at := func(what string) {
		problems = append(problems, Problem{File: FileName, Line: m.Lines["credentials"], What: what})
	}

	for _, c := range m.Credentials {
		if c.Name == "" {
			at("every credential needs a `name`")
			continue
		}
		if c.Scope != ScopeMachine && c.Scope != ScopeWorkspace {
			at(fmt.Sprintf("credential %q: scope must be %q or %q, got %q",
				c.Name, ScopeMachine, ScopeWorkspace, c.Scope))
		}

		switch c.Kind {
		case KindLogin:
			if c.Command == "" {
				at(fmt.Sprintf("credential %q is a login, so it needs the `command` a person runs", c.Name))
			}
			if c.StoredAt == "" {
				at(fmt.Sprintf(
					"credential %q is a login, so it needs `stored_at`: without it nothing can check whether it worked",
					c.Name))
			}
		case KindSecret:
			if c.Shareable {
				at(fmt.Sprintf(
					"credential %q is a secret, so it cannot be `shareable`: a secret is delivered to each "+
						"place that wants it, never copied out of one of them", c.Name))
			}
			if c.Env == "" && c.Path == "" {
				at(fmt.Sprintf("credential %q is a secret, so it needs `env` or `path`: somewhere to deliver the value",
					c.Name))
			}
		case KindFile:
			if c.Shareable {
				at(fmt.Sprintf(
					"credential %q is a file, so it cannot be `shareable`: a file is delivered to each place "+
						"that wants it, never copied out of one of them", c.Name))
			}
			if c.Path == "" {
				at(fmt.Sprintf("credential %q is a file, so it needs the `path` it lands at", c.Name))
			}
		default:
			at(fmt.Sprintf("credential %q: kind must be %q, %q or %q, got %q",
				c.Name, KindLogin, KindSecret, KindFile, c.Kind))
		}
	}
	return problems
}

// scanForApt reports every direct use of apt in the package's YAML.
func scanForApt(dir string) ([]Problem, error) {
	var problems []Problem

	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if ext := filepath.Ext(path); ext != ".yml" && ext != ".yaml" {
			return nil
		}
		if filepath.Base(path) == FileName {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		scanner := bufio.NewScanner(file)
		for line := 1; scanner.Scan(); line++ {
			if aptModule.MatchString(scanner.Text()) {
				problems = append(problems, Problem{
					File: relative,
					Line: line,
					What: "the apt module is not allowed; use `package` so this works beyond Debian",
				})
			}
		}
		return scanner.Err()
	})
	return problems, err
}

func firstLine(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return "", scanner.Err()
	}
	return strings.TrimRight(scanner.Text(), "\r"), nil
}

func joinInts(values []int) string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, strconv.Itoa(v))
	}
	return strings.Join(out, ", ")
}

// sortedKeys keeps a message about a map from moving between runs.
func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
