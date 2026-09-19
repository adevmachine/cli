package provision

import (
	"strings"
	"testing"
)

func TestSummaryNamesEveryTargetAndPackage(t *testing.T) {
	plan := planWith(t, "main",
		[]string{"base", "docker"},
		map[string][]string{"alice": {"claude-code"}})

	lines := strings.Join(Summary(plan), "\n")
	for _, want := range []string{"main", "base", "docker", "alice", "claude-code"} {
		if !strings.Contains(lines, want) {
			t.Fatalf("the summary leaves out %q:\n%s", want, lines)
		}
	}
}

// A local package silently replacing a published one is the surprise this
// mechanism is most likely to produce, and the plan is where somebody would
// catch it.
func TestSummarySaysWhenAPackageComesFromTheOperatorsOwnConfiguration(t *testing.T) {
	plan := planWithLocalPackage(t, "main", "caddy")

	lines := strings.Join(Summary(plan), "\n")
	if !strings.Contains(lines, "local") {
		t.Fatalf("an overridden package is not marked:\n%s", lines)
	}
}

// Only the copy that won is marked. A published package carries no note,
// because "everything is normal" printed seven times is noise.
func TestSummaryLeavesAPublishedPackageUnmarked(t *testing.T) {
	plan := planWith(t, "main", []string{"docker"}, nil)

	for _, line := range Summary(plan) {
		if strings.Contains(line, "docker") && strings.Contains(line, "local") {
			t.Fatalf("a published package was marked as local: %q", line)
		}
	}
}

func TestSummaryNamesTheExtensionsAndWhereTheyLand(t *testing.T) {
	lines := strings.Join(Summary(planWithExtension(t)), "\n")

	for _, want := range []string{"sharing", "caddy.sites.d", "/etc/caddy/sites.d"} {
		if !strings.Contains(lines, want) {
			t.Fatalf("the summary leaves out %q:\n%s", want, lines)
		}
	}
}

func TestSummarySaysSoWhenThereIsNothingToDo(t *testing.T) {
	lines := Summary(planWith(t, "main", nil, nil))

	if len(lines) != 1 || !strings.Contains(lines[0], "nothing") {
		t.Fatalf("got %#v", lines)
	}
}
