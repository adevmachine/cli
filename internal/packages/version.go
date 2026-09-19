package packages

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// constraintPattern is deliberately small. Three operators cover what a recipe
// needs to say; a full range grammar is a dependency and a support burden for
// a question nobody has asked.
var constraintPattern = regexp.MustCompile(`^(>=|>|=)\s*v?(\d+)\.(\d+)\.(\d+)$`)

// Constraint is what a recipe says about the CLI that can run it.
//
// It answers a different question from `format`. A format says whether this
// CLI can read the file at all; a constraint says whether it can run what the
// file describes.
type Constraint struct {
	op       string
	major    int
	minor    int
	patch    int
	original string
}

// ParseConstraint reads a constraint like ">= 0.2.0".
func ParseConstraint(s string) (Constraint, error) {
	match := constraintPattern.FindStringSubmatch(strings.TrimSpace(s))
	if match == nil {
		return Constraint{}, fmt.Errorf(`write it as ">= 0.2.0", "> 0.2.0" or "= 0.2.0"`)
	}
	major, _ := strconv.Atoi(match[2])
	minor, _ := strconv.Atoi(match[3])
	patch, _ := strconv.Atoi(match[4])
	return Constraint{op: match[1], major: major, minor: minor, patch: patch, original: strings.TrimSpace(s)}, nil
}

func (c Constraint) String() string { return c.original }

// Allows reports whether this CLI version satisfies the constraint.
//
// An unparseable version is allowed: that is what a build from source calls
// itself, and refusing every recipe while the CLI is being worked on would
// help nobody.
func (c Constraint) Allows(version string) bool {
	major, minor, patch, ok := parseVersion(version)
	if !ok {
		return true
	}

	got := [3]int{major, minor, patch}
	want := [3]int{c.major, c.minor, c.patch}
	switch c.op {
	case "=":
		return got == want
	case ">":
		return compareVersions(got, want) > 0
	default:
		return compareVersions(got, want) >= 0
	}
}

var versionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)

func parseVersion(s string) (int, int, int, bool) {
	match := versionPattern.FindStringSubmatch(strings.TrimSpace(s))
	if match == nil {
		return 0, 0, 0, false
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	return major, minor, patch, true
}

func compareVersions(a, b [3]int) int {
	for i := range a {
		if a[i] != b[i] {
			return a[i] - b[i]
		}
	}
	return 0
}
