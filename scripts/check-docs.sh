#!/usr/bin/env bash
# Fails when the manual has a broken link, or a page nothing links to.
#
# A page nobody can reach is worse than a missing one: it looks maintained and
# is never read.
set -uo pipefail

cd "$(dirname "$0")/.."

# Prints every relative markdown link in a file, one per line, with any anchor
# stripped. External links are excluded by their scheme, not by their first
# letter — "how-it-works/" starts with an h, and excluding the letter silently
# hid every page in that folder the first time this was written.
links_in() {
  grep -ohE '\]\([^)]+\.md(#[^)]*)?\)' "$1" 2>/dev/null \
    | sed -E 's/^\]\(//; s/\)$//; s/#.*$//' \
    | grep -vE '^[a-z][a-z0-9+.-]*://'
}

problems=0

while IFS= read -r page; do
  dir=$(dirname "$page")
  while IFS= read -r target; do
    [ -z "$target" ] && continue
    if [ ! -e "$dir/$target" ]; then
      echo "broken link in $page: $target"
      problems=1
    fi
  done < <(links_in "$page")
done < <(find docs -name '*.md' | sort)

linked=$(links_in docs/index.md | sort -u)

while IFS= read -r page; do
  relative=${page#docs/}
  [ "$relative" = "index.md" ] && continue
  if ! printf '%s\n' "$linked" | grep -qxF "$relative"; then
    echo "not linked from docs/index.md: $relative"
    problems=1
  fi
done < <(find docs -name '*.md' | sort)

if [ "$problems" -ne 0 ]; then
  echo
  echo "The manual does not hold together."
  exit 1
fi
echo "Documentation links are fine."
