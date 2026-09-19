package credentials

import (
	"context"
	"fmt"
	"strings"

	"github.com/adevmachine/cli/internal/packages"
	"github.com/adevmachine/cli/internal/remote"
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
	if d.Kind == packages.KindLogin {
		return d.StoredAt
	}
	return Destination(d)
}

// presentScript answers, for each line it reads, whether the place exists.
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
		printf '%s%syes\n' "$key" "$tab"
	else
		printf '%s%sno\n' "$key" "$tab"
	fi
done
`

// Present says which of the wanted credentials are already on the machine.
//
// A credential with nowhere to look — a login whose package never said where
// its tool keeps the session — is left out of the answer rather than reported
// missing. "I cannot tell" and "it is not there" are different things.
func Present(ctx context.Context, c remote.Client, wanted []Declared) (map[string]bool, error) {
	present := map[string]bool{}

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
		return present, nil
	}

	out, err := c.RunInput(ctx, presentScript, strings.NewReader(list.String()))
	if err != nil {
		return nil, fmt.Errorf("looking for the credentials on the machine: %w", err)
	}

	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		key, answer, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok {
			continue
		}
		present[key] = answer == "yes"
	}
	return present, nil
}
