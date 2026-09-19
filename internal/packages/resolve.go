package packages

import (
	"fmt"
	"slices"
	"strings"

	"github.com/adevmachine/cli/internal/config"
)

// Target is one place packages are installed: a machine, or one workspace on
// it.
type Target struct {
	// Kind is ScopeMachine or ScopeWorkspace.
	Kind string `json:"kind"`
	Name string `json:"name"`
	// LinuxUser is empty for a machine.
	LinuxUser string   `json:"linux_user,omitempty"`
	Packages  []string `json:"packages,omitempty"`
}

// Resolved is everything a target gets, in the order it has to run.
type Resolved struct {
	Target  Target
	Ordered []Found
}

// Extension is one package's contribution to a place another package opened.
type Extension struct {
	// From is the extending package, and Target is the machine or workspace
	// that asked for it.
	From   string `json:"from"`
	Target string `json:"target"`
	// Point is written <package>.<place>, such as caddy.sites.d.
	Point string `json:"point"`
	// Source is a path inside the extending package.
	Source string `json:"source"`
	// Into is the absolute path on the machine the extended package provides.
	Into string `json:"into"`
}

// MachinePlan is everything that has to happen on one machine.
type MachinePlan struct {
	Machine config.Machine
	// OnMachine is what the machine itself gets; Workspaces is one entry per
	// workspace that lives on it.
	OnMachine  Resolved
	Workspaces []Resolved
	Extensions []Extension
}

// ResolveMachine works out the machine's own packages, each of its
// workspaces' packages, the order `needs` puts them in, and where every
// declared extension writes.
func ResolveMachine(store *Store, cfg config.Config, machine config.Machine, cliVersion string) (MachinePlan, error) {
	plan := MachinePlan{Machine: machine}

	resolved, err := resolveTarget(store, Target{
		Kind: ScopeMachine, Name: machine.Name, Packages: machine.Packages,
	}, cliVersion)
	if err != nil {
		return plan, err
	}
	plan.OnMachine = resolved

	for _, w := range cfg.WorkspacesOn(machine.Name) {
		r, err := resolveTarget(store, Target{
			Kind: ScopeWorkspace, Name: w.Name, LinuxUser: w.LinuxUser(), Packages: w.Packages,
		}, cliVersion)
		if err != nil {
			return plan, err
		}
		plan.Workspaces = append(plan.Workspaces, r)
	}

	extensions, err := wireExtensions(plan)
	if err != nil {
		return plan, err
	}
	plan.Extensions = extensions
	return plan, nil
}

// resolveTarget expands one target's list through `needs` and orders it.
//
// The roots are visited in name order, so two configurations that list the
// same packages differently produce the same run: `needs` is the only thing
// that may decide an order.
func resolveTarget(store *Store, target Target, cliVersion string) (Resolved, error) {
	out := Resolved{Target: target}

	var (
		done     = map[string]bool{}
		visiting = map[string]bool{}
		visit    func(name string, chain []string) error
	)
	visit = func(name string, chain []string) error {
		if done[name] {
			return nil
		}
		if visiting[name] {
			return fmt.Errorf("the packages %s need each other in a circle",
				strings.Join(append(chain, name), " -> "))
		}
		visiting[name] = true

		found, err := store.Get(name)
		if err != nil {
			return err
		}
		if found.Manifest.Scope != target.Kind {
			return scopeError(found.Manifest, target)
		}
		if c := found.Manifest.Requires.CLI; c != "" {
			constraint, err := ParseConstraint(c)
			if err != nil {
				return fmt.Errorf("package %q: requires.cli %q: %w", name, c, err)
			}
			if !constraint.Allows(cliVersion) {
				return fmt.Errorf(
					"package %q needs a CLI %s, and this one is %s: upgrade with `brew upgrade devmachine`",
					name, constraint, cliVersion)
			}
		}

		for _, need := range found.Manifest.Needs {
			if err := visit(need, append(chain, name)); err != nil {
				return err
			}
		}

		visiting[name] = false
		done[name] = true
		out.Ordered = append(out.Ordered, found)
		return nil
	}

	roots := slices.Clone(target.Packages)
	slices.Sort(roots)
	for _, name := range roots {
		if err := visit(name, nil); err != nil {
			return out, err
		}
	}
	return out, nil
}

// scopeError says what to do, not just what was wrong. Naming the wrong kind
// of target is a mistake anybody makes once.
func scopeError(m Manifest, target Target) error {
	if m.Scope == ScopeWorkspace {
		return fmt.Errorf(
			"package %q is a workspace package, and %q is a machine: "+
				"add it with `devmachine packages add %s --workspace <name>`",
			m.Name, target.Name, m.Name)
	}
	return fmt.Errorf(
		"package %q is a machine package, and %q is a workspace: "+
			"add it with `devmachine packages add %s --machine <name>`",
		m.Name, target.Name, m.Name)
}

// wireExtensions resolves each `extends` onto the absolute path the extended
// package declared, and refuses one whose provider is not on this machine.
func wireExtensions(plan MachinePlan) ([]Extension, error) {
	provided := map[string]string{}
	for _, f := range plan.OnMachine.Ordered {
		for place, path := range f.Manifest.Provides {
			provided[f.Manifest.Name+"."+place] = path
		}
	}

	var out []Extension
	for _, resolved := range append([]Resolved{plan.OnMachine}, plan.Workspaces...) {
		for _, f := range resolved.Ordered {
			for _, point := range sortedKeys(f.Manifest.Extends) {
				into, ok := provided[point]
				if !ok {
					owner, _, _ := strings.Cut(point, ".")
					return nil, fmt.Errorf(
						"package %q extends %q, but %s is not installed on machine %q, or it does not provide %q",
						f.Manifest.Name, point, owner, plan.Machine.Name, point)
				}
				out = append(out, Extension{
					From:   f.Manifest.Name,
					Target: resolved.Target.Name,
					Point:  point,
					Source: f.Manifest.Extends[point],
					Into:   into,
				})
			}
		}
	}
	return out, nil
}
