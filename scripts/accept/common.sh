#!/usr/bin/env bash

# Shared assertions and safety guards for the release acceptance scenarios.
# Keep this file compatible with the Bash shipped by macOS (3.2).

accept_reset() {
  ACCEPT_CHECKS=0
  ACCEPT_FAILURES=0
}

pass() {
  ACCEPT_CHECKS=$((ACCEPT_CHECKS + 1))
  printf '  PASS  %s\n' "$1"
}

fail() {
  ACCEPT_CHECKS=$((ACCEPT_CHECKS + 1))
  ACCEPT_FAILURES=$((ACCEPT_FAILURES + 1))
  printf '  FAIL  %s\n' "$1"
  return 1
}

# Remove whitespace surrounding a value, without changing whitespace inside
# it. In particular, spaces inside a short value remain significant ("2 00"
# is not "200").
_accept_trim_line_endings() {
  ACCEPT_TRIMMED=$1
  while :; do
    [ -n "$ACCEPT_TRIMMED" ] || break
    ACCEPT_FIRST=${ACCEPT_TRIMMED%"${ACCEPT_TRIMMED#?}"}
    case "$ACCEPT_FIRST" in
      ' '|$'\t'|$'\n'|$'\r') ACCEPT_TRIMMED=${ACCEPT_TRIMMED#?} ;;
      *) break ;;
    esac
  done
  while :; do
    [ -n "$ACCEPT_TRIMMED" ] || break
    ACCEPT_LAST=${ACCEPT_TRIMMED#"${ACCEPT_TRIMMED%?}"}
    case "$ACCEPT_LAST" in
      ' '|$'\t'|$'\n'|$'\r') ACCEPT_TRIMMED=${ACCEPT_TRIMMED%?} ;;
      *) break ;;
    esac
  done
}

equals() {
  _accept_trim_line_endings "$1"
  accept_left=$ACCEPT_TRIMMED
  _accept_trim_line_endings "$2"
  accept_right=$ACCEPT_TRIMMED
  if [ "$accept_left" = "$accept_right" ]; then
    pass "$3"
  else
    fail "$3"
  fi
}

contains() {
  if printf '%s' "$1" | grep -Fq -- "$2"; then
    pass "$3"
  else
    fail "$3"
  fi
}

refutes() {
  if printf '%s' "$1" | grep -Fq -- "$2"; then
    fail "$3"
  else
    pass "$3"
  fi
}

scenario_done() {
  expected=$1
  name=$2
  [ "$ACCEPT_CHECKS" -eq "$expected" ] || fail "$name ran $ACCEPT_CHECKS checks; expected $expected"
  [ "$ACCEPT_FAILURES" -eq 0 ]
}

require_accept_vm() {
  accept_vm=$1
  case "$accept_vm" in
    fakevps)
      echo "refusing to operate on protected VM: fakevps" >&2
      return 1
      ;;
    devmachine-accept-*)
      return 0
      ;;
    *)
      echo "refusing non-acceptance VM: $accept_vm" >&2
      return 1
      ;;
  esac
}

make_run_dir() {
  ACCEPT_RUN_DIR=$(mktemp -d "${TMPDIR:-/tmp}/devmachine-accept.XXXXXX")
  export ACCEPT_RUN_DIR
  printf '%s\n' "$ACCEPT_RUN_DIR"
}

destroy_accept_vm() {
  accept_vm=$1
  require_accept_vm "$accept_vm" || return 1
  ACCEPT_KEEP_VM=${KEEP_ACCEPT_VM:-0}
  export ACCEPT_KEEP_VM
  if [ "$ACCEPT_KEEP_VM" = 1 ]; then
    printf 'keeping acceptance VM %s\n' "$accept_vm" >&2
    return 0
  fi
  if [ -n "${DEVMACHINE_BIN:-}" ]; then
    "$DEVMACHINE_BIN" machines delete-local --yes "$accept_vm"
  else
    devmachine machines delete-local --yes "$accept_vm"
  fi
}

accept_reset
