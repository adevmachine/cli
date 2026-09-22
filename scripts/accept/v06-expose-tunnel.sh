#!/usr/bin/env bash
# The v0.6 expose, HTTPS, and tunnel acceptance proof.
set -uo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
source "$ROOT/scripts/accept/common.sh"

if [ -z "${ACCEPT_RUN_DIR:-}" ]; then
  make_run_dir >/dev/null
  ACCEPT_OWNS_RUN_DIR=1
else
  ACCEPT_OWNS_RUN_DIR=0
fi

VM="devmachine-accept-${ACCEPT_RUN_ID:?ACCEPT_RUN_ID is required}-v06"
require_accept_vm "$VM" || exit 1

SCENARIO_DIR=$(mktemp -d "$ACCEPT_RUN_DIR/v06.XXXXXX")
DEVMACHINE_CONFIG=$(mktemp -d "$SCENARIO_DIR/config.XXXXXX")
SCENARIO_LOG_DIR="$SCENARIO_DIR/logs"
mkdir -p "$SCENARIO_LOG_DIR"
export DEVMACHINE_CONFIG

TUNNEL_PID=
BUSY_PID=

cleanup_v06() {
  if [ -n "$TUNNEL_PID" ]; then
    kill "$TUNNEL_PID" 2>/dev/null || true
    wait "$TUNNEL_PID" 2>/dev/null || true
  fi
  if [ -n "$BUSY_PID" ]; then
    kill "$BUSY_PID" 2>/dev/null || true
    wait "$BUSY_PID" 2>/dev/null || true
  fi
  destroy_accept_vm "$VM" || true
  rm -rf "$SCENARIO_DIR"
  if [ "$ACCEPT_OWNS_RUN_DIR" -eq 1 ]; then
    rm -rf "$ACCEPT_RUN_DIR"
  fi
}
trap cleanup_v06 EXIT INT TERM

die() {
  echo "$1" >&2
  exit 1
}

if [ -z "${DEVMACHINE_ACCEPT_BIN:-}" ] || [ ! -x "$DEVMACHINE_ACCEPT_BIN" ]; then
  die "DEVMACHINE_ACCEPT_BIN must name the local acceptance CLI"
fi
if [ -z "${PACKAGES:-}" ] || [ ! -d "$PACKAGES/packages" ]; then
  die "PACKAGES must name a packages checkout"
fi

free_port() {
  python3 - <<'PY'
import socket

sock = socket.socket()
sock.bind(("127.0.0.1", 0))
print(sock.getsockname()[1])
sock.close()
PY
}

deadline_after() {
  python3 - "$1" <<'PY'
import sys
import time

print(int(time.time()) + int(sys.argv[1]))
PY
}

before_deadline() {
  [ "$(date +%s)" -lt "$1" ]
}

copy_package() {
  package=$1
  [ -d "$PACKAGES/packages/$package" ] || die "package fixture is missing: $package"
  cp -R "$PACKAGES/packages/$package" "$DEVMACHINE_CONFIG/packages/"
}

"$DEVMACHINE_ACCEPT_BIN" machines create-local "$VM" \
  >"$SCENARIO_LOG_DIR/create.log" 2>&1 || die "could not create $VM"
CLOUD_INIT=$(limactl shell "$VM" -- cloud-init status --wait 2>&1 || true)
printf '%s\n' "$CLOUD_INIT" > "$SCENARIO_LOG_DIR/cloud-init.log"
[ "$CLOUD_INIT" = "status: done" ] || die "$VM did not finish cloud-init: $CLOUD_INIT"

PORT=$(limactl list --format '{{.SSHLocalPort}}' "$VM")
[ -n "$PORT" ] || die "$VM never received an SSH port"
printf '%s\n' "$VM" "127.0.0.1" "root" "$PORT" "example.com" "1" "devmachine" \
  | "$DEVMACHINE_ACCEPT_BIN" setup >"$SCENARIO_LOG_DIR/setup.log" 2>&1 \
  || die "could not set up $VM"
"$DEVMACHINE_ACCEPT_BIN" run --machine "$VM" -- true \
  >"$SCENARIO_LOG_DIR/connect.log" 2>&1 || die "$VM does not answer"

mkdir -p "$DEVMACHINE_CONFIG/packages"
for package in base docker firewall caddy workspace; do
  copy_package "$package"
done

python3 - <<'PY' || die "could not configure the v0.6 fixture"
import os
from pathlib import Path

path = Path(os.environ["DEVMACHINE_CONFIG"]) / "config.yml"
text = path.read_text()
text = text.replace(
    "      key: ",
    "      packages: [base, docker, firewall, caddy]\n"
    "      settings:\n"
    "        caddy.local_certs: true\n"
    "      key: ",
    1,
)
path.write_text(text)
PY

"$DEVMACHINE_ACCEPT_BIN" workspaces new alice --packages workspace --yes \
  >"$SCENARIO_LOG_DIR/workspace.log" 2>&1 || die "could not add alice"
"$DEVMACHINE_ACCEPT_BIN" sync --yes >"$SCENARIO_LOG_DIR/sync.log" 2>&1 \
  || die "could not sync the v0.6 fixture"

PROBE=$("$DEVMACHINE_ACCEPT_BIN" run --machine "$VM" -- \
  'docker run -d --name accept-v06-probe -p 127.0.0.1:8080:80 nginx:alpine' 2>&1) \
  || die "could not start the nginx fixture: $PROBE"
printf '%s\n' "$PROBE" > "$SCENARIO_LOG_DIR/probe.log"
CONTAINER_DEADLINE=$(deadline_after 60)
DIRECT_CODE=
while before_deadline "$CONTAINER_DEADLINE"; do
  DIRECT_CODE=$("$DEVMACHINE_ACCEPT_BIN" run --machine "$VM" -- \
    'curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8080' 2>&1 || true)
  if [ "$DIRECT_CODE" = "200" ]; then
    break
  fi
  sleep 1
done
[ "$DIRECT_CODE" = "200" ] || die "the nginx fixture never served directly: $DIRECT_CODE"

ASKED=$(printf 'n\n' | "$DEVMACHINE_ACCEPT_BIN" expose add alice 8080 --host app.example.com 2>&1)
contains "$ASKED" "anybody" "expose says anybody will reach it" || true

NOCADDY_DIR=$(mktemp -d "$SCENARIO_DIR/no-caddy.XXXXXX")
cp -R "$DEVMACHINE_CONFIG/." "$NOCADDY_DIR/"
python3 - "$NOCADDY_DIR/config.yml" <<'PY' || die "could not configure the no-caddy fixture"
import re
import sys
from pathlib import Path

path = Path(sys.argv[1])
text = path.read_text()
text, packages = re.subn(r",\s*caddy\s*\]", "]", text, count=1)
text, settings = re.subn(
    r"(?m)^[ \t]*settings:\n[ \t]*caddy\.local_certs: true\n", "", text, count=1)
if packages != 1 or settings != 1:
    raise SystemExit("the no-caddy fixture did not remove caddy and its setting")
path.write_text(text)
PY
NOCADDY=$(DEVMACHINE_CONFIG="$NOCADDY_DIR" "$DEVMACHINE_ACCEPT_BIN" expose add alice 8080 \
  --host app.example.com --yes 2>&1)
printf '%s\n' "$NOCADDY" > "$SCENARIO_LOG_DIR/no-caddy.log"
contains "$NOCADDY" "caddy is not on" "expose refuses without caddy, and names it" || true

"$DEVMACHINE_ACCEPT_BIN" expose add alice 8080 --host app.example.com --yes \
  >"$SCENARIO_LOG_DIR/expose-add.log" 2>&1 || die "could not expose app.example.com"
BLOCK=$("$DEVMACHINE_ACCEPT_BIN" run --machine "$VM" -- \
  'cat /etc/caddy/sites.d/app.example.com.caddy' 2>&1) \
  || die "could not read the Caddy site block"
contains "$BLOCK" "reverse_proxy 127.0.0.1:8080" "the block points at the port" || true
contains "$BLOCK" "devmachine expose" "and says what wrote it" || true

SERVED=$("$DEVMACHINE_ACCEPT_BIN" run --machine "$VM" -- \
  'curl -sk -o /dev/null -w "%{http_code}" --resolve app.example.com:443:127.0.0.1 https://app.example.com' 2>&1)
equals "$SERVED" "200" "Caddy proxies the host to the container" || true

LIST=$("$DEVMACHINE_ACCEPT_BIN" expose list 2>&1)
contains "$LIST" "app.example.com" "expose list shows it" || true
"$DEVMACHINE_ACCEPT_BIN" expose rm app.example.com --yes \
  >"$SCENARIO_LOG_DIR/expose-rm.log" 2>&1 || die "could not remove app.example.com"
GONE=$("$DEVMACHINE_ACCEPT_BIN" run --machine "$VM" -- 'ls /etc/caddy/sites.d' 2>&1)
refutes "$GONE" "app.example.com" "expose rm removed the file" || true

LOCAL_PORT=$(free_port)
"$DEVMACHINE_ACCEPT_BIN" tunnel alice 8080 --local "$LOCAL_PORT" \
  >"$SCENARIO_LOG_DIR/tunnel.log" 2>&1 &
TUNNEL_PID=$!
OPEN_DEADLINE=$(deadline_after 30)
TUNNEL_CODE=
while before_deadline "$OPEN_DEADLINE"; do
  TUNNEL_CODE=$(curl -s -o /dev/null -w '%{http_code}' --max-time 2 \
    "http://127.0.0.1:$LOCAL_PORT" 2>/dev/null || true)
  if [ "$TUNNEL_CODE" = "200" ]; then
    break
  fi
  sleep 1
done
equals "$TUNNEL_CODE" "200" "tunnel reaches the port from this computer" || true
kill "$TUNNEL_PID" 2>/dev/null || true
wait "$TUNNEL_PID" 2>/dev/null || true
CLOSED_DEADLINE=$(deadline_after 15)
AFTER=
while before_deadline "$CLOSED_DEADLINE"; do
  AFTER=$(curl -s -o /dev/null -w '%{http_code}' --max-time 2 \
    "http://127.0.0.1:$LOCAL_PORT" 2>/dev/null || true)
  if [ "$AFTER" = "000" ]; then
    break
  fi
  sleep 1
done
equals "$AFTER" "000" "and closing it closes the tunnel" || true
if kill -0 "$TUNNEL_PID" 2>/dev/null; then
  fail "no ssh is left behind" || true
else
  pass "no ssh is left behind"
fi
TUNNEL_PID=

BUSY_PORT=$(free_port)
python3 - "$BUSY_PORT" >"$SCENARIO_LOG_DIR/busy-port.log" 2>&1 <<'PY' &
import socket
import sys
import time

sock = socket.socket()
sock.bind(("127.0.0.1", int(sys.argv[1])))
sock.listen()
time.sleep(120)
PY
BUSY_PID=$!
BUSY_DEADLINE=$(deadline_after 15)
while before_deadline "$BUSY_DEADLINE"; do
  if kill -0 "$BUSY_PID" 2>/dev/null; then
    BUSY_READY=$(python3 - "$BUSY_PORT" <<'PY'
import socket
import sys

sock = socket.socket()
try:
    sock.bind(("127.0.0.1", int(sys.argv[1])))
except OSError:
    print("busy")
finally:
    sock.close()
PY
)
    [ "$BUSY_READY" = "busy" ] && break
  fi
  sleep 1
done
[ "${BUSY_READY:-}" = "busy" ] || die "the busy-port fixture did not bind"
BUSYOUT=$("$DEVMACHINE_ACCEPT_BIN" tunnel alice 8080 --local "$BUSY_PORT" 2>&1 || true)
contains "$BUSYOUT" "--local" "a busy local port is explained, not a bind error" || true
kill "$BUSY_PID" 2>/dev/null || true
wait "$BUSY_PID" 2>/dev/null || true
BUSY_PID=

MD=$("$DEVMACHINE_ACCEPT_BIN" machine doctor 2>&1)
contains "$MD" "ssh" "machine doctor checks this computer" || true

scenario_done 12 "v0.6"
