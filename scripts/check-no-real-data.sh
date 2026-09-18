#!/usr/bin/env bash
# Fails when the tree holds a value from the operator's private pattern list.
#
# The list lives outside the repository, because the list itself is the data.
# Point at it with DEVMACHINE_PRIVATE_PATTERNS, or keep it at the default path.
# With no list the check passes: a contributor who has none is not the person
# this guards against.
set -uo pipefail

cd "$(dirname "$0")/.."

LIST="${DEVMACHINE_PRIVATE_PATTERNS:-$HOME/.config/devmachine-app/private-patterns}"
if [ ! -r "$LIST" ]; then
  echo "No pattern list at $LIST — nothing to check against."
  exit 0
fi

hits=0
while IFS= read -r pattern; do
  [ -z "$pattern" ] && continue
  case "$pattern" in \#*) continue ;; esac

  # A word boundary is required. Without it a short pattern matches inside an
  # ordinary English word — "agno" lives inside "diagnostics" — and the guard
  # cries wolf until somebody stops reading it.
  if out=$(git ls-files -z | xargs -0 grep -HnIiE -- "\\b${pattern}\\b" 2>/dev/null); then
    printf '%s\n' "$out"
    hits=1
  fi
done < "$LIST"

if [ "$hits" -ne 0 ]; then
  echo
  echo "Real data found. It must never enter this repository."
  exit 1
fi
echo "No real data found."
