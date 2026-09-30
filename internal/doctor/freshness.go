package doctor

import (
	"fmt"

	"github.com/mydevmachine/devmachine/internal/release"
)

// The freshness checks. They look at your computer and GitHub, not at a
// machine, so they come after every machine check.
const (
	CheckCLI         = "cli"
	CheckPackagesPin = "packages pin"
)

const updateFix = "run `devmachine update`"

// CLICheck says whether the running CLI is the newest release.
//
// Old is a warning and never a failure: an old CLI still works. Not being
// able to ask is a skip — being offline says nothing about the CLI.
func CLICheck(current, latest string, lookupErr error) Check {
	if lookupErr != nil {
		return Check{Name: CheckCLI, Status: StatusSkip,
			Detail: fmt.Sprintf("could not find the latest release: %v", lookupErr)}
	}
	if _, ok := release.Compare(current, latest); !ok {
		return Check{Name: CheckCLI, Status: StatusSkip,
			Detail: fmt.Sprintf("%q is a development build, not a release", current)}
	}
	if release.Newer(latest, current) {
		return Check{Name: CheckCLI, Status: StatusWarn,
			Detail: fmt.Sprintf("this is %s, and %s is out: %s", current, latest, updateFix)}
	}
	return Check{Name: CheckCLI, Status: StatusPass, Detail: current + ", the latest"}
}

// PackagesPinCheck says whether the configuration pins the newest packages
// release.
func PackagesPinCheck(pinned, latest string, lookupErr error) Check {
	if pinned == "" {
		return Check{Name: CheckPackagesPin, Status: StatusSkip,
			Detail: "no packages release is pinned; `devmachine packages pin` pins the latest"}
	}
	if lookupErr != nil {
		return Check{Name: CheckPackagesPin, Status: StatusSkip,
			Detail: fmt.Sprintf("could not find the latest packages release: %v", lookupErr)}
	}
	if release.Newer(latest, pinned) {
		return Check{Name: CheckPackagesPin, Status: StatusWarn,
			Detail: fmt.Sprintf("pinned %s, and %s is out: %s", pinned, latest, updateFix)}
	}
	return Check{Name: CheckPackagesPin, Status: StatusPass, Detail: pinned + ", the latest"}
}
