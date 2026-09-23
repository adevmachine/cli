#!/usr/bin/env bash
# Proves strict rejection and explicit host-key rotation against one disposable VM.
set -uo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
source "$ROOT/scripts/accept/common.sh"

if [ -z "${ACCEPT_RUN_DIR:-}" ]; then
  make_run_dir >/dev/null
  ACCEPT_OWNS_RUN_DIR=1
else
  ACCEPT_OWNS_RUN_DIR=0
fi

VM="devmachine-accept-${ACCEPT_RUN_ID:?ACCEPT_RUN_ID is required}-hostkey"
require_accept_vm "$VM" || exit 1

SCENARIO_DIR=$(mktemp -d "$ACCEPT_RUN_DIR/hostkey.XXXXXX")
DEVMACHINE_CONFIG=$(mktemp -d "$SCENARIO_DIR/config.XXXXXX")
export DEVMACHINE_CONFIG

cleanup_hostkey() {
  destroy_accept_vm "$VM" || true
  rm -rf "$SCENARIO_DIR"
  if [ "$ACCEPT_OWNS_RUN_DIR" -eq 1 ]; then
    rm -rf "$ACCEPT_RUN_DIR"
  fi
}
trap cleanup_hostkey EXIT INT TERM

die() {
  echo "$1" >&2
  exit 1
}

if [ -z "${DEVMACHINE_ACCEPT_BIN:-}" ] || [ ! -x "$DEVMACHINE_ACCEPT_BIN" ]; then
  die "DEVMACHINE_ACCEPT_BIN must name the local acceptance CLI"
fi

"$DEVMACHINE_ACCEPT_BIN" machines create-local "$VM" \
  >"$SCENARIO_DIR/create.log" 2>&1 || die "could not create $VM"
CLOUD_INIT=$(limactl shell "$VM" -- cloud-init status --wait 2>&1 || true)
printf '%s\n' "$CLOUD_INIT" > "$SCENARIO_DIR/cloud-init.log"
[ "$CLOUD_INIT" = "status: done" ] || die "$VM did not finish cloud-init: $CLOUD_INIT"

PORT=$(limactl list --format '{{.SSHLocalPort}}' "$VM")
[ -n "$PORT" ] || die "$VM never received an SSH port"
SETUP=$(printf '%s\n' "$VM" "127.0.0.1" "root" "$PORT" "example.com" "y" "1" "devmachine" \
  | "$DEVMACHINE_ACCEPT_BIN" setup 2>&1) || die "could not set up $VM: $SETUP"
printf '%s\n' "$SETUP" > "$SCENARIO_DIR/setup.log"
contains "$SETUP" "SHA256:" "setup prints the presented SHA256 fingerprint" || true

KNOWN_HOSTS="$DEVMACHINE_CONFIG/known_hosts"
[ -f "$KNOWN_HOSTS" ] || die "setup did not create $KNOWN_HOSTS"
TRUST=$(cat "$KNOWN_HOSTS")
contains "$TRUST" "$VM-devmachine" "setup stores the stable machine alias" || true

"$DEVMACHINE_ACCEPT_BIN" run --machine "$VM" -- true \
  >"$SCENARIO_DIR/run-before.log" 2>&1 \
  && pass "a normal command succeeds with the stored key" \
  || die "the initial trusted connection failed"

BEFORE=$(cksum "$KNOWN_HOSTS" | awk '{print $1 ":" $2}')
limactl shell "$VM" -- sudo sh -eu -c '
  tmp=/tmp/devmachine-accept-host-key
  rm -f "$tmp" "$tmp.pub"
  ssh-keygen -q -t ed25519 -N "" -f "$tmp"
  install -m 0600 "$tmp" /etc/ssh/ssh_host_ed25519_key
  install -m 0644 "$tmp.pub" /etc/ssh/ssh_host_ed25519_key.pub
  rm -f "$tmp" "$tmp.pub"
  systemctl restart ssh
' >"$SCENARIO_DIR/rotate-server.log" 2>&1 || die "could not rotate the disposable VM host key"

CHANGED=$("$DEVMACHINE_ACCEPT_BIN" run --machine "$VM" -- true 2>&1)
CHANGED_STATUS=$?
printf '%s\n' "$CHANGED" > "$SCENARIO_DIR/run-changed.log"
if [ "$CHANGED_STATUS" -ne 0 ]; then
  pass "a changed server key is rejected"
else
  fail "a changed server key is rejected" || true
fi
contains "$CHANGED" "SSH host key changed" "the rejection names the changed host key" || true
contains "$CHANGED" "--replace" "the rejection names the explicit recovery" || true

CHECK=$("$DEVMACHINE_ACCEPT_BIN" --format json machines trust "$VM" --check --replace --yes 2>&1)
CHECK_STATUS=$?
printf '%s\n' "$CHECK" > "$SCENARIO_DIR/check.log"
[ "$CHECK_STATUS" -eq 0 ] || die "host-key check failed: $CHECK"
contains "$CHECK" '"current_fingerprint": "SHA256:' "check reports the old fingerprint" || true
contains "$CHECK" '"presented_fingerprint": "SHA256:' "check reports the presented fingerprint" || true
contains "$CHECK" '"check": true' "check reports that it did not write" || true
AFTER_CHECK=$(cksum "$KNOWN_HOSTS" | awk '{print $1 ":" $2}')
equals "$AFTER_CHECK" "$BEFORE" "check leaves known_hosts unchanged" || true

REPLACED=$("$DEVMACHINE_ACCEPT_BIN" --format json machines trust "$VM" --replace --yes 2>&1)
REPLACE_STATUS=$?
printf '%s\n' "$REPLACED" > "$SCENARIO_DIR/replace.log"
[ "$REPLACE_STATUS" -eq 0 ] || die "host-key replacement failed: $REPLACED"
contains "$REPLACED" '"changed": true' "explicit replacement reports a change" || true
AFTER_REPLACE=$(cksum "$KNOWN_HOSTS" | awk '{print $1 ":" $2}')
if [ "$AFTER_REPLACE" != "$BEFORE" ]; then
  pass "explicit replacement changes known_hosts"
else
  fail "explicit replacement changes known_hosts" || true
fi

"$DEVMACHINE_ACCEPT_BIN" run --machine "$VM" -- true \
  >"$SCENARIO_DIR/run-after.log" 2>&1 \
  && pass "a normal command succeeds only after explicit replacement" \
  || die "the connection did not recover after explicit replacement"

scenario_done 13 "host-key pinning"
