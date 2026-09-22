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

require_accept_vm "devmachine-accept-123-v05"
if require_accept_vm "fakevps" 2>/dev/null; then
  echo "fakevps passed the acceptance VM guard" >&2
  exit 1
fi
if require_accept_vm "main" 2>/dev/null; then
  echo "an arbitrary machine passed the acceptance VM guard" >&2
  exit 1
fi
