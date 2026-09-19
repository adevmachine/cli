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

	"github.com/adevmachine/cli/internal/packages"
	"github.com/adevmachine/cli/internal/remote"
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
	files, err := Generate(plan)
	if err != nil {
		return Result{}, err
	}

	tarball, err := Tar(files, roleDirs(plan))
	if err != nil {
		return Result{}, err
	}
	if err := a.Client.Upload(ctx, RemoteDir, tarball); err != nil {
		return Result{}, err
	}

	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	var seen bytes.Buffer
	watched := io.MultiWriter(&seen, out)

	command := playbookCommand(opts)
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

func playbookCommand(opts Options) string {
	command := fmt.Sprintf("cd %[1]s && ANSIBLE_CONFIG=%[1]s/ansible.cfg ansible-playbook -i inventory.ini site.yml",
		RemoteDir)
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
