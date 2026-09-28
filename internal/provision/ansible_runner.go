package provision

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/mydevmachine/cli/internal/packages"
	"github.com/mydevmachine/cli/internal/remote"
)

// Ansible provisions a machine by running ansible-playbook on it.
//
// Running it there means the operator's computer needs only this binary and
// ssh, and a run costs no round trip per task.
type Ansible struct {
	Client remote.Client
}

// Apply sends everything the plan needs and runs it.
//
// The machine's output is streamed as it arrives and copied at the same time,
// so the recap is read from what the person already watched rather than from a
// second run.
func (a *Ansible) Apply(ctx context.Context, plan packages.MachinePlan, opts Options) (Result, error) {
	base, err := Base(plan.Machine)
	if err != nil {
		return Result{}, err
	}

	files, err := GenerateAt(plan, base)
	if err != nil {
		return Result{}, err
	}

	tarball, err := Tar(files, roleDirs(plan))
	if err != nil {
		return Result{}, err
	}
	if base == "" || base == "/" {
		return Result{}, fmt.Errorf("refusing to empty %q as the bundle directory", base)
	}
	// Unpacking never deletes, and Ansible searches roles.local before roles:
	// a copy an earlier sync left would keep running instead of this one.
	if _, err := a.Client.Run(ctx, "rm -rf "+base); err != nil {
		return Result{}, fmt.Errorf("emptying %s: %w", base, err)
	}
	if err := a.Client.Upload(ctx, base, tarball); err != nil {
		return Result{}, err
	}

	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	var seen bytes.Buffer
	watched := io.MultiWriter(&seen, out)

	command := playbookCommand(opts, base)
	runErr := a.Client.Stream(ctx, command, watched, watched)
	result := readRecap(seen.String())

	if runErr != nil {
		if result.Failed > 0 {
			return result, fmt.Errorf("%d task(s) failed on %s: %w", result.Failed, plan.Machine.Name, runErr)
		}
		return result, runErr
	}
	return result, nil
}

// roleDirs maps each package in the plan onto the directory it is unpacked
// into, which is what decides whether the operator's copy or the published one
// runs.
//
// Only the packages the plan names are sent. Having a recipe in the cache that
// nothing asks for is the ordinary state, and sending the whole cache to every
// machine would make that state cost bandwidth and confusion.
func roleDirs(plan packages.MachinePlan) map[string]string {
	dirs := map[string]string{}
	for _, resolved := range append([]packages.Resolved{plan.OnMachine}, plan.Workspaces...) {
		for _, found := range resolved.Ordered {
			dirs[path.Join(rolesDir(found.Source), found.Manifest.Name)] = found.Manifest.Path
		}
	}
	return dirs
}

func playbookCommand(opts Options, base string) string {
	// Ansible searches a playbook-adjacent directory named roles before the
	// configured roles_path. Running a copied playbook from a fresh directory
	// keeps base/roles from silently beating base/roles.local.
	command := fmt.Sprintf("run=$(mktemp -d) && trap 'rm -rf \"$run\"' EXIT && "+
		"cp %[1]s/site.yml \"$run/site.yml\" && cd \"$run\" && "+
		"ANSIBLE_CONFIG=%[1]s/ansible.cfg ansible-playbook -i %[1]s/inventory.ini site.yml",
		base)
	if opts.Check {
		command += " --check"
	}
	if len(opts.Tags) > 0 {
		command += " --tags " + strings.Join(opts.Tags, ",")
	}
	return command
}

// recapLine matches the play recap, which is the only line that opens with a
// host name and then a run of key=number counters.
var recapLine = regexp.MustCompile(`(?m)^\S+\s*:\s+(ok=\d+.*)$`)

var counter = regexp.MustCompile(`(\w+)=(\d+)`)

// readRecap turns the recap into counts. A run with no recap at all — one that
// died before the play started — leaves a zero Result, and the error from the
// run is what says what happened.
func readRecap(output string) Result {
	matches := recapLine.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return Result{}
	}

	var result Result
	for _, field := range counter.FindAllStringSubmatch(matches[len(matches)-1][1], -1) {
		value, err := strconv.Atoi(field[2])
		if err != nil {
			continue
		}
		switch field[1] {
		case "ok":
			result.Ok = value
		case "changed":
			result.Changed = value
		case "failed":
			result.Failed = value
		}
	}
	return result
}
