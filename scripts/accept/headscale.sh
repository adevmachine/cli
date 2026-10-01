#!/usr/bin/env bash
# The Headscale proof: a real Headscale server, a real Tailscale client and the
# packages checkout's `tailscale` package, on three disposable VMs that share
# one private network. The CLI under test is built for Linux and runs only
# inside the client VM, which plays "your computer"; nothing here touches the
# Tailscale, SSH or devmachine configuration of the computer running it.
#
# Opt-in: it boots three VMs, so it is not in run.sh's default list.
set -uo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
source "$ROOT/scripts/accept/common.sh"

if [ -z "${ACCEPT_RUN_DIR:-}" ]; then
  make_run_dir >/dev/null
  ACCEPT_OWNS_RUN_DIR=1
else
  ACCEPT_OWNS_RUN_DIR=0
fi

HEADSCALE_VERSION=${HEADSCALE_VERSION:-0.29.4}
PREFIX="devmachine-accept-${ACCEPT_RUN_ID:?ACCEPT_RUN_ID is required}"
SERVER="$PREFIX-hs-server"
BOX="$PREFIX-hs-box"
CLIENT="$PREFIX-hs-client"
for vm in "$SERVER" "$BOX" "$CLIENT"; do
  require_accept_vm "$vm" || exit 1
done

SCENARIO_DIR=$(mktemp -d "$ACCEPT_RUN_DIR/headscale.XXXXXX")
SCENARIO_LOG_DIR="$SCENARIO_DIR/logs"
mkdir -p "$SCENARIO_LOG_DIR"

destroy_headscale_vms() {
  if [ "${KEEP_ACCEPT_VM:-0}" = 1 ]; then
    printf 'keeping acceptance VMs %s, %s, %s\n' "$SERVER" "$BOX" "$CLIENT" >&2
    return 0
  fi
  for vm in "$SERVER" "$BOX" "$CLIENT"; do
    require_accept_vm "$vm" || continue
    limactl delete --force "$vm" >/dev/null 2>&1 || true
  done
}

cleanup_headscale() {
  destroy_headscale_vms
  rm -rf "$SCENARIO_DIR"
  if [ "$ACCEPT_OWNS_RUN_DIR" -eq 1 ]; then
    rm -rf "$ACCEPT_RUN_DIR"
  fi
}
trap cleanup_headscale EXIT INT TERM

die() {
  echo "$1" >&2
  last=$(ls -t "$SCENARIO_LOG_DIR"/*.log 2>/dev/null | head -1)
  [ -n "$last" ] && { echo "--- last lines of $(basename "$last")" >&2; tail -40 "$last" >&2; }
  exit 1
}

if [ -z "${PACKAGES:-}" ] || [ ! -d "$PACKAGES/packages/tailscale" ]; then
  die "PACKAGES must name a packages checkout with the tailscale package"
fi
command -v go >/dev/null 2>&1 || die "the headscale scenario builds the CLI for Linux and needs go"

case "$(uname -m)" in
  arm64 | aarch64) GOARCH=arm64 ;;
  x86_64 | amd64) GOARCH=amd64 ;;
  *) die "no Linux build for $(uname -m)" ;;
esac
LINUX_BIN="$SCENARIO_DIR/devmachine-linux"
(cd "$ROOT" && GOOS=linux GOARCH=$GOARCH CGO_ENABLED=0 go build -ldflags "-X main.version=dev" \
  -o "$LINUX_BIN" ./cmd/devmachine) >"$SCENARIO_LOG_DIR/build.log" 2>&1 \
  || die "could not build the CLI for linux/$GOARCH; see $SCENARIO_LOG_DIR/build.log"

# vzNAT keeps two VMs from reaching each other, so the three share Lima's
# user-v2 network instead: one subnet, no root needed on the host.
create_vm() {
  vm=$1
  cpus=$2
  memory=$3
  limactl create --tty=false --name="$vm" \
    --set ".cpus = $cpus | .memory = \"$memory\" | .disk = \"10GiB\" | .mounts = [] | .networks = [{\"lima\": \"user-v2\"}] | .video.display = \"none\" | .containerd.system = false | .containerd.user = false" \
    template:ubuntu-24.04 >"$SCENARIO_LOG_DIR/create-$vm.log" 2>&1 \
    && limactl start --tty=false "$vm" >>"$SCENARIO_LOG_DIR/create-$vm.log" 2>&1
}

create_vm "$SERVER" 1 1GiB || die "could not create $SERVER"
create_vm "$BOX" 2 2GiB || die "could not create $BOX"
create_vm "$CLIENT" 2 2GiB || die "could not create $CLIENT"

on() {
  limactl shell --workdir / "$1" -- bash -c "$2" 2>&1
}

on_client() {
  on "$CLIENT" "export DEVMACHINE_KEYCHAIN=off; $1"
}

address_of() {
  on "$1" "ip -4 -o addr show dev eth0 | awk '{print \$4}' | cut -d/ -f1"
}

SERVER_IP=$(address_of "$SERVER")
BOX_IP=$(address_of "$BOX")
[ -n "$SERVER_IP" ] && [ -n "$BOX_IP" ] || die "the VMs have no address on the shared network"
LOGIN_SERVER="http://$SERVER_IP:8080"

# The box is a server as bought: root reachable with a password, no key.
on "$BOX" "sudo bash -c 'echo root:devmachine | chpasswd
printf \"PermitRootLogin yes\nPasswordAuthentication yes\n\" > /etc/ssh/sshd_config.d/01-devmachine-local.conf
rm -f /root/.ssh/authorized_keys
systemctl restart ssh'" >"$SCENARIO_LOG_DIR/box.log" || die "could not open root login on $BOX"
BOX_NAME=$(on "$BOX" hostname)

DEB="headscale_${HEADSCALE_VERSION}_linux_${GOARCH}.deb"
RELEASE="https://github.com/juanfont/headscale/releases/download/v$HEADSCALE_VERSION"
on "$SERVER" "set -e
cd /tmp
curl -fsSLo headscale.deb $RELEASE/$DEB
curl -fsSLo checksums.txt $RELEASE/checksums.txt
grep ' $DEB\$' checksums.txt | sed 's| $DEB\$| headscale.deb|' | sha256sum -c -
sudo apt-get install -y -qq ./headscale.deb
sudo sed -i -e 's|^server_url: .*|server_url: $LOGIN_SERVER|' \
  -e 's|^listen_addr: .*|listen_addr: 0.0.0.0:8080|' \
  -e 's|^  base_domain: .*|  base_domain: tail.example.com|' /etc/headscale/config.yaml
sudo systemctl enable headscale
sudo systemctl restart headscale
for i in \$(seq 30); do curl -fs http://127.0.0.1:8080/health && break; sleep 1; done
sudo headscale users create acme" >"$SCENARIO_LOG_DIR/headscale.log" \
  || die "could not install Headscale; see $SCENARIO_LOG_DIR/headscale.log"
contains "$(on "$SERVER" "headscale version")" "v$HEADSCALE_VERSION" "Headscale runs the pinned release" || true
AUTH_KEY=$(on "$SERVER" "sudo headscale preauthkeys create --user 1 --reusable --expiration 2h" | tail -1)
[ -n "$AUTH_KEY" ] || die "Headscale gave no pre-auth key"

equals "$(on_client "curl -s -o /dev/null -w '%{http_code}' $LOGIN_SERVER/health")" "200" \
  "the client reaches Headscale over the shared network" || true

limactl copy "$LINUX_BIN" "$CLIENT:/tmp/devmachine" >"$SCENARIO_LOG_DIR/copy.log" 2>&1 \
  || die "could not copy the CLI into $CLIENT"
on "$CLIENT" "set -e
sudo install -m 0755 /tmp/devmachine /usr/local/bin/devmachine
CODENAME=\$(. /etc/os-release && echo \$VERSION_CODENAME)
curl -fsSL https://pkgs.tailscale.com/stable/ubuntu/\$CODENAME.noarmor.gpg | sudo tee /usr/share/keyrings/tailscale-archive-keyring.gpg >/dev/null
curl -fsSL https://pkgs.tailscale.com/stable/ubuntu/\$CODENAME.tailscale-keyring.list | sudo tee /etc/apt/sources.list.d/tailscale.list >/dev/null
sudo apt-get update -qq
sudo apt-get install -y -qq tailscale expect
sudo tailscale up --login-server $LOGIN_SERVER --authkey $AUTH_KEY --hostname client" \
  >"$SCENARIO_LOG_DIR/client.log" || die "could not join the client to Headscale; see $SCENARIO_LOG_DIR/client.log"
contains "$(on_client "tailscale ip -4")" "100.64." "the client joined Headscale" || true

on_client "printf '%s\n' box $BOX_IP root 22 '' y 1 devmachine n | devmachine setup --no-essentials --no-aliases" \
  >"$SCENARIO_LOG_DIR/setup.log" || die "could not set up the box; see $SCENARIO_LOG_DIR/setup.log"

CONFIG=.config/devmachine
CLIENT_HOME=$(on "$CLIENT" 'mkdir -p ~/.config/devmachine/packages && echo $HOME')
limactl copy -r "$PACKAGES/packages/tailscale" "$PACKAGES/packages/workspace" "$CLIENT:$CLIENT_HOME/$CONFIG/packages/" \
  >"$SCENARIO_LOG_DIR/packages.log" 2>&1 || die "could not copy the packages into $CLIENT"
on_client "set -e
devmachine packages add tailscale --machine box --yes
python3 - ~/$CONFIG/config.yml <<'EOF'
import sys
path = sys.argv[1]
lines = open(path).read().splitlines(True)
at = lines.index('  - name: box\n')
lines.insert(at + 1, '    settings:\n      tailscale.login_server: $LOGIN_SERVER\n')
open(path, 'w').write(''.join(lines))
EOF
devmachine workspaces new acme --packages workspace --yes
devmachine sync --yes" >"$SCENARIO_LOG_DIR/sync.log" || die "could not sync; see $SCENARIO_LOG_DIR/sync.log"

# The join reads the key with getpass, which wants a real terminal and throws
# away anything typed before its prompt, so expect answers it.
on "$CLIENT" "cat > /tmp/login.exp <<'EOF'
set timeout 180
spawn devmachine login tailscale --machine box
expect {
  -re {Pre-auth key[^:]*: } { send \"\$env(AUTH_KEY)\r\" }
  timeout { exit 2 }
}
expect eof
catch wait result
exit [lindex \$result 3]
EOF" >/dev/null
LOGIN=$(on_client "AUTH_KEY=$AUTH_KEY expect /tmp/login.exp")
printf '%s\n' "$LOGIN" >"$SCENARIO_LOG_DIR/login.log"
contains "$LOGIN" "added tailscale:$BOX_NAME above the public address" "login joins the box to Headscale and names it" || true
HOSTS=$(on_client "sed -n '/- name: box/,/user:/p' ~/$CONFIG/config.yml | grep -A2 '^    hosts:'")
equals "$HOSTS" "    hosts:
      - tailscale:$BOX_NAME
      - $BOX_IP" "the Headscale entry is written first, the public address stays as a fallback" || true
equals "$(on "$BOX" "ls /tmp | grep -c '^tailscale-key-'")" "0" "the pre-auth key file is gone from the box" || true

first_address() {
  on_client "devmachine resolve --machine box --format json" \
    | python3 -c 'import json,sys; a=json.load(sys.stdin)["addresses"][0]; print(a["address"], a["package"])'
}
FIRST=$(first_address)
contains "$FIRST" "100.64." "resolve lists the box's Headscale address first" || true
contains "$FIRST" " tailscale" "the Headscale address comes from the tailscale package" || true

on "$CLIENT" "sudo iptables -I OUTPUT -d $BOX_IP -p tcp --dport 22 -j DROP" >/dev/null \
  || die "could not block the box's public address"
equals "$(on_client "devmachine run --machine box -- hostname")" "$BOX_NAME" \
  "run reaches the box over Headscale with its public address blocked" || true
contains "$(on_client "devmachine doctor --machine box")" "connected through 100.64." \
  "doctor connects over Headscale with its public address blocked" || true
on_client "devmachine aliases --write --path /tmp/aliases --yes" >"$SCENARIO_LOG_DIR/aliases.log" \
  || die "could not write the aliases"
alias_ssh() {
  on_client "ssh -F /tmp/aliases -o BatchMode=yes -o ConnectTimeout=30 acme-devmachine whoami"
}
equals "$(alias_ssh)" "acme" "an alias reaches the workspace over Headscale through ssh-proxy" || true

on "$CLIENT" "sudo tailscale down && sudo iptables -D OUTPUT -d $BOX_IP -p tcp --dport 22 -j DROP" >/dev/null \
  || die "could not stop Tailscale on the client"
DOWN=$(on_client "devmachine resolve --machine box --format json" \
  | python3 -c 'import json,sys; r=json.load(sys.stdin); print(r["dropped"][0]["source"], "|", r["dropped"][0]["reason"], "|", r["addresses"][0]["address"])')
equals "$DOWN" "tailscale:$BOX_NAME | tailscale is not running (it says Stopped) | $BOX_IP" \
  "with Tailscale down, the entry is skipped with its reason and the public address leads" || true
equals "$(on_client "devmachine run --machine box -- hostname")" "$BOX_NAME" \
  "run falls back to the public address" || true
equals "$(alias_ssh)" "acme" "the alias falls back to the public address" || true

on "$CLIENT" "sudo tailscale up --login-server $LOGIN_SERVER --hostname client" >/dev/null \
  || die "could not bring Tailscale back up"
contains "$(first_address)" "100.64." "with Tailscale back up, the Headscale address leads again" || true

scenario_done 15 "headscale"
