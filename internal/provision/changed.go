package provision

import (
	"regexp"
	"slices"
	"strings"
)

var taskHeader = regexp.MustCompile(`^(?:TASK|RUNNING HANDLER) \[(.+)\] \*+\s*$`)

// ChangedTasks names every task that reported a change, in the order they
// ran, from what Ansible's default output printed.
//
// It is how `update` says what a `sync --check` would change without
// replaying the whole run: the recap only counts changes, and the count
// alone does not tell anybody whether to say yes.
func ChangedTasks(output string) []string {
	var (
		current string
		out     []string
	)
	for _, line := range strings.Split(output, "\n") {
		if m := taskHeader.FindStringSubmatch(strings.TrimRight(line, "\r")); m != nil {
			current = m[1]
			continue
		}
		if current != "" && strings.HasPrefix(line, "changed: ") && !slices.Contains(out, current) {
			out = append(out, current)
		}
	}
	return out
}
