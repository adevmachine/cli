#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
source "$ROOT/scripts/accept/common.sh"

accept_reset
equals "active" "active" "whole value matches" >/dev/null
[ "$ACCEPT_CHECKS" -eq 1 ]
[ "$ACCEPT_FAILURES" -eq 0 ]

accept_reset
equals "inactive" "active" "substring is not equality" >/dev/null || true
[ "$ACCEPT_CHECKS" -eq 1 ]
[ "$ACCEPT_FAILURES" -eq 1 ]

accept_reset
equals "2 00" "200" "internal whitespace stays significant" >/dev/null || true
[ "$ACCEPT_FAILURES" -eq 1 ]

accept_reset
equals " active" "active" "leading spaces stay significant" >/dev/null || true
[ "$ACCEPT_FAILURES" -eq 1 ]

accept_reset
equals "active " "active" "trailing spaces stay significant" >/dev/null || true
[ "$ACCEPT_FAILURES" -eq 1 ]

accept_reset
equals $'\tactive' "active" "leading tabs stay significant" >/dev/null || true
[ "$ACCEPT_FAILURES" -eq 1 ]

cleanup_test_dir=$(mktemp -d "${TMPDIR:-/tmp}/devmachine-accept-common.XXXXXX")
cleanup_marker="$cleanup_test_dir/path-fallback-used"
cat > "$cleanup_test_dir/devmachine" <<EOF
#!/usr/bin/env bash
touch "$cleanup_marker"
EOF
chmod +x "$cleanup_test_dir/devmachine"
if DEVMACHINE_ACCEPT_BIN= DEVMACHINE_BIN= PATH="$cleanup_test_dir:$PATH" KEEP_ACCEPT_VM=0 \
    destroy_accept_vm "devmachine-accept-test" >/dev/null 2>&1; then
  echo "cleanup accepted an implicit PATH CLI" >&2
  exit 1
fi
[ ! -f "$cleanup_marker" ]

if DEVMACHINE_ACCEPT_BIN="$cleanup_test_dir/devmachine" KEEP_ACCEPT_VM=0 \
    destroy_accept_vm "devmachine-accept-test" >/dev/null 2>&1; then
  echo "cleanup accepted an executable outside the repository" >&2
  exit 1
fi

local_cleanup_bin="$ROOT/scripts/accept/.accept-test-bin"
cp "$cleanup_test_dir/devmachine" "$local_cleanup_bin"
chmod +x "$local_cleanup_bin"
DEVMACHINE_ACCEPT_BIN="$local_cleanup_bin" KEEP_ACCEPT_VM=0 \
  destroy_accept_vm "devmachine-accept-test" >/dev/null 2>&1
[ -f "$cleanup_marker" ]
rm -f "$local_cleanup_bin"
rm -rf "$cleanup_test_dir"

require_accept_vm "devmachine-accept-123-v05"
if require_accept_vm "fakevps" 2>/dev/null; then
  echo "fakevps passed the acceptance VM guard" >&2
  exit 1
fi
if require_accept_vm "main" 2>/dev/null; then
  echo "an arbitrary machine passed the acceptance VM guard" >&2
  exit 1
fi

[ -x "$ROOT/scripts/accept/run.sh" ]
[ -x "$ROOT/scripts/accept/setup-git.sh" ]
[ -x "$ROOT/scripts/accept/v05-packages-credentials-dns.sh" ]
[ -x "$ROOT/scripts/accept/v06-expose-tunnel.sh" ]
[ -x "$ROOT/scripts/accept/host-key-pinning.sh" ]
grep -q '^accept:' "$ROOT/Makefile"
grep -q 'make accept' "$ROOT/docs/development.md"
grep -q 'KEEP_ACCEPT_VM=1' "$ROOT/docs/development.md"
grep -q 'make accept' "$ROOT/docs/releasing.md"
