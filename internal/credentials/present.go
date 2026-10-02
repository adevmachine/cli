package credentials

import (
	"context"
	"fmt"
	"strings"

	"github.com/mydevmachine/devmachine/internal/packages"
	"github.com/mydevmachine/devmachine/internal/remote"
)

// Place is where a credential is expected to be found on the machine.
//
// For a login it is `stored_at`, and that is a claim rather than a guarantee:
// it says where a tool keeps its session so this check can look there. A tool
// that changes where it writes makes this report say "missing" about something
// that is in fact present. It is worth having anyway — the alternative is no
// check at all — and the manual says so.
//
// For a secret or a file it is where this CLI delivered the value, which is
// not a claim about anybody.
func Place(d Declared) string {
	if d.Kind == packages.KindManual {
		return d.StoredAt
	}
	return Destination(d)
}

// presentScript answers, for each line it reads, whether the place exists
// and, when it is a file root can read, the SHA-256 of what it holds: enough
// to tell a rotated value from the one delivered, without the value ever
// leaving the machine.
//
// One command for every credential: three of them must not be three
// connections, because `doctor` and `credentials list` both ask about the lot.
// The list arrives on stdin, so a path never has to survive shell quoting.
const presentScript = `set -eu
tab=$(printf '\t')
# The unit separator, and not a tab, because a tab is IFS whitespace: two of
# them in a row count as one, and a machine credential — which has no account —
# would have its path read as its account.
sep=$(printf '\037')
while IFS="$sep" read -r key user place; do
	path=$place
	home=$HOME
	if [ -n "$user" ]; then
		home=$(getent passwd "$user" | cut -d: -f6)
	fi
	case "$path" in
	'~/'*) path="$home/${path#'~/'}" ;;
	esac
	if [ -n "$home" ] && [ -e "$path" ]; then
		sum=
		if [ -f "$path" ] && [ -r "$path" ]; then
			if command -v sha256sum >/dev/null 2>&1; then
				sum=$(sha256sum "$path" | cut -d' ' -f1)
			elif command -v shasum >/dev/null 2>&1; then
				sum=$(shasum -a 256 "$path" | cut -d' ' -f1)
			fi
		fi
		printf '%s%syes%s%s\n' "$key" "$tab" "$tab" "$sum"
	else
		printf '%s%sno%s\n' "$key" "$tab" "$tab"
	fi
done
`

// Present says which of the wanted credentials are already on the machine.
//
// A credential with nowhere to look — a login whose package never said where
// its tool keeps the session — is left out of the answer rather than reported
// missing. "I cannot tell" and "it is not there" are different things.
func Present(ctx context.Context, c remote.Client, wanted []Declared) (map[string]bool, error) {
	present, _, err := Look(ctx, c, wanted)
	return present, err
}

// Look is Present together with the SHA-256 of each file that is there. A
// digest is empty when the machine could not say: a directory, a file root
// cannot read, or no tool to hash with.
func Look(ctx context.Context, c remote.Client, wanted []Declared) (map[string]bool, map[string]string, error) {
	present := map[string]bool{}
	digests := map[string]string{}

	var list strings.Builder
	asked := 0
	for _, d := range wanted {
		place := Place(d)
		if place == "" {
			continue
		}
		fmt.Fprintf(&list, "%s\x1f%s\x1f%s\n", Key(d), d.LinuxUser, place)
		asked++
	}
	if asked == 0 {
		return present, digests, nil
	}

	out, err := c.RunInput(ctx, presentScript, strings.NewReader(list.String()))
	if err != nil {
		return nil, nil, fmt.Errorf("looking for the credentials on the machine: %w", err)
	}

	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		fields := strings.Split(strings.TrimSpace(line), "\t")
		if len(fields) < 2 {
			continue
		}
		present[fields[0]] = fields[1] == "yes"
		if len(fields) > 2 {
			digests[fields[0]] = fields[2]
		}
	}
	return present, digests, nil
}
