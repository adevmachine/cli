#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
source "$ROOT/scripts/accept/common.sh"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "acceptance requires $1" >&2
    exit 1
  fi
}

for command in bash git ssh limactl python3 curl; do
  require_command "$command"
done

PACKAGES=${PACKAGES:-"$HOME/dev/packages"}
if [ ! -d "$PACKAGES/packages" ]; then
  echo "acceptance requires a packages checkout at $PACKAGES" >&2
  exit 1
fi
export PACKAGES

make_run_dir >/dev/null
ACCEPT_RUN_ID="$(date +%s)-$$"
export ACCEPT_RUN_ID

cleanup_run_dir() {
  rm -rf "$ACCEPT_RUN_DIR"
}
trap cleanup_run_dir EXIT INT TERM

make -C "$ROOT" build
DEVMACHINE_ACCEPT_BIN="$ROOT/devmachine"
if [ ! -f "$DEVMACHINE_ACCEPT_BIN" ] || [ ! -x "$DEVMACHINE_ACCEPT_BIN" ] || [ -L "$DEVMACHINE_ACCEPT_BIN" ]; then
  echo "acceptance build did not produce a regular executable: $DEVMACHINE_ACCEPT_BIN" >&2
  exit 1
fi
export DEVMACHINE_ACCEPT_BIN

if [ "$#" -eq 0 ]; then
  set -- setup-git
fi

for scenario in "$@"; do
  scenario_path="$ROOT/scripts/accept/$scenario.sh"
  if [ ! -x "$scenario_path" ]; then
    echo "unknown acceptance scenario: $scenario" >&2
    exit 1
  fi
  "$scenario_path"
done

echo "devmachine acceptance passed"
