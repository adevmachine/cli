// Package selfupdate replaces the running CLI with a newer release: through
// Homebrew when Homebrew installed it, and from the release's own archive
// otherwise.
package selfupdate

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/mydevmachine/devmachine/internal/release"
)

// Formula is the tap formula Homebrew installs the CLI from.
const Formula = "mydevmachine/tap/devmachine"

// ContinueFlag is the hidden `update` flag the new binary is started with
// after it replaced the old one. Its value is the version that was replaced.
const ContinueFlag = "continue-after-self-update"

var (
	latestAPIURL = "https://api.github.com/repos/mydevmachine/devmachine/releases/latest"
	// DownloadURL is where release assets live, one directory per tag.
	DownloadURL = "https://github.com/mydevmachine/devmachine/releases/download"
)

// Latest is the tag of the newest published CLI release.
func Latest(ctx context.Context) (string, error) {
	return release.Latest(ctx, latestAPIURL)
}

// knownPrefixes are where Homebrew lives by default on Apple silicon, on an
// Intel Mac and on Linux. /usr/local counts only through its Cellar: a
// binary copied into /usr/local/bin by hand is not Homebrew's to upgrade.
var knownPrefixes = []struct{ prefix, owned string }{
	{"/opt/homebrew", "/opt/homebrew/"},
	{"/usr/local", "/usr/local/Cellar/"},
	{"/home/linuxbrew/.linuxbrew", "/home/linuxbrew/.linuxbrew/Cellar/"},
}

// HomebrewPrefix returns the Homebrew prefix that owns the binary at path,
// or "" when Homebrew did not install it. brewPrefix is what `brew --prefix`
// said, empty when there is no brew; it covers a prefix in a custom place.
//
// The path is resolved first, because Homebrew installs a symlink on PATH
// that points into its Cellar.
func HomebrewPrefix(path, brewPrefix string) string {
	resolved := resolve(path)
	for _, p := range knownPrefixes {
		if strings.HasPrefix(resolved, p.owned) || strings.HasPrefix(path, p.owned) {
			return p.prefix
		}
	}
	if brewPrefix == "" {
		return ""
	}
	for _, prefix := range []string{brewPrefix, resolve(brewPrefix)} {
		if strings.HasPrefix(resolved, prefix+"/Cellar/") {
			return brewPrefix
		}
	}
	return ""
}

// BinaryUnder is where Homebrew links the CLI inside prefix. After an
// upgrade the Cellar path of the old version may be gone; this link always
// points at the current one.
func BinaryUnder(prefix string) string {
	return filepath.Join(prefix, "bin", "devmachine")
}

func resolve(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

// ContinueArgs is the argument list the new binary is started with: the
// same command line, with the continue flag naming the replaced version.
func ContinueArgs(args []string, previous string) []string {
	flag := "--" + ContinueFlag
	out := make([]string, 0, len(args)+1)
	for i := 0; i < len(args); i++ {
		switch {
		case strings.HasPrefix(args[i], flag+"="):
		case args[i] == flag:
			i++
		default:
			out = append(out, args[i])
		}
	}
	return append(out, flag+"="+previous)
}
