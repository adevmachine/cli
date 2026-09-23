#!/usr/bin/env bash
set -uo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
source "$ROOT/scripts/accept/common.sh"

if [ -z "${ACCEPT_RUN_DIR:-}" ]; then
  make_run_dir >/dev/null
  ACCEPT_OWNS_RUN_DIR=1
else
  ACCEPT_OWNS_RUN_DIR=0
fi

SCENARIO_DIR=$(mktemp -d "$ACCEPT_RUN_DIR/setup-git.XXXXXX")

cleanup_setup_git() {
  rm -rf "$SCENARIO_DIR"
  if [ "$ACCEPT_OWNS_RUN_DIR" -eq 1 ]; then
    rm -rf "$ACCEPT_RUN_DIR"
  fi
}
trap cleanup_setup_git EXIT INT TERM

if [ -z "${DEVMACHINE_ACCEPT_BIN:-}" ] || [ ! -x "$DEVMACHINE_ACCEPT_BIN" ]; then
  echo "DEVMACHINE_ACCEPT_BIN must name the local acceptance CLI" >&2
  exit 1
fi

export GIT_CONFIG_NOSYSTEM=1
export GIT_CONFIG_GLOBAL="$SCENARIO_DIR/gitconfig"
printf '[user]\n\tname = Acceptance Fixture\n\temail = acceptance@example.invalid\n[commit]\n\tgpgsign = false\n' \
  > "$GIT_CONFIG_GLOBAL"

ACCEPT_GH_LOG="$SCENARIO_DIR/gh.log"
export ACCEPT_GH_LOG
mkdir -p "$SCENARIO_DIR/bin"
printf '%s\n' '#!/usr/bin/env bash' 'printf "%s\\n" "$*" >> "$ACCEPT_GH_LOG"' 'exit 1' \
  > "$SCENARIO_DIR/bin/gh"
chmod +x "$SCENARIO_DIR/bin/gh"
export PATH="$SCENARIO_DIR/bin:$PATH"

DEVMACHINE_CONFIG="$SCENARIO_DIR/config"
export DEVMACHINE_CONFIG
mkdir -p "$DEVMACHINE_CONFIG/keys" "$DEVMACHINE_CONFIG/cache/packages"
printf 'machines:\n  - name: main\n    hosts: [203.0.113.10]\npackages: v2\n' > "$DEVMACHINE_CONFIG/config.yml"
printf '{}\n' > "$DEVMACHINE_CONFIG/packages.lock"
printf 'main-devmachine ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAcceptancePublicKey\n' \
  > "$DEVMACHINE_CONFIG/known_hosts"
printf 'PRIVATE KEY\n' > "$DEVMACHINE_CONFIG/keys/id_ed25519"
printf 'PUBLIC KEY\n' > "$DEVMACHINE_CONFIG/keys/id_ed25519.pub"
printf '{"t":"s3cret"}\n' > "$DEVMACHINE_CONFIG/secrets.json"
printf 'ran something\n' > "$DEVMACHINE_CONFIG/history.log"
printf 'TOKEN=s3cret\n' > "$DEVMACHINE_CONFIG/cloudflare.env"
printf 'cached\n' > "$DEVMACHINE_CONFIG/cache/packages/x"

OUT=$("$DEVMACHINE_ACCEPT_BIN" setup git --yes 2>&1)
contains "$OUT" "private" "setup git says the remote must be private" || true

TRACKED=$(git -C "$DEVMACHINE_CONFIG" ls-files)
for path in config.yml packages.lock known_hosts .gitignore; do
  contains "$TRACKED" "$path" "tracks $path" || true
done
for path in keys/id_ed25519 secrets.json history.log cloudflare.env cache/; do
  refutes "$TRACKED" "$path" "does not track $path" || true
done

BLOBS=$(git -C "$DEVMACHINE_CONFIG" rev-list --objects --all | awk '{print $2}')
for path in keys/id_ed25519 secrets.json cloudflare.env; do
  refutes "$BLOBS" "$path" "$path never existed in any commit" || true
done
CONTENT=$(git -C "$DEVMACHINE_CONFIG" rev-list --objects --all \
  | awk '{print $1}' | xargs -n1 git -C "$DEVMACHINE_CONFIG" cat-file -p 2>/dev/null)
refutes "$CONTENT" "s3cret" "no secret value is in any blob" || true
refutes "$CONTENT" "PRIVATE KEY" "no private key is in any blob" || true

FIRST=$(git -C "$DEVMACHINE_CONFIG" rev-list --max-parents=0 HEAD)
FIRST_FILES=$(git -C "$DEVMACHINE_CONFIG" show --name-only --format= "$FIRST")
contains "$FIRST_FILES" ".gitignore" "the very first commit already carries .gitignore" || true

DIRTY="$SCENARIO_DIR/dirty"
mkdir -p "$DIRTY/keys"
printf 'machines:\n  - name: main\n    hosts: [203.0.113.10]\npackages: v2\n' > "$DIRTY/config.yml"
printf 'PRIVATE KEY\n' > "$DIRTY/keys/id_ed25519"
git -C "$DIRTY" init -q -b main
git -C "$DIRTY" add -A && git -C "$DIRTY" commit -qm "by hand"
REFUSED=$(DEVMACHINE_CONFIG="$DIRTY" "$DEVMACHINE_ACCEPT_BIN" setup git --yes 2>&1)
REFUSED_CODE=$?
if [ "$REFUSED_CODE" -ne 0 ]; then
  pass "it refuses a directory that already tracks a key"
else
  fail "it refuses a directory that already tracks a key" || true
fi
contains "$REFUSED" "git rm --cached" "and says how to untrack it" || true
contains "$REFUSED" "rotate" "and says untracking is not enough" || true

"$DEVMACHINE_ACCEPT_BIN" packages add docker --machine main --yes > /dev/null 2>&1
MESSAGE=$(git -C "$DEVMACHINE_CONFIG" log -1 --format=%s)
equals "$MESSAGE" "chore(config): add package docker" "a write commits itself, with its own message" || true
LINES=$(git -C "$DEVMACHINE_CONFIG" log -1 --format=%B | grep -c .)
equals "$LINES" "1" "the message is one line" || true
BODY=$(git -C "$DEVMACHINE_CONFIG" log -1 --format=%B)
refutes "$BODY" "Co-Authored-By" "no Co-Authored-By" || true
refutes "$BODY" "Claude" "no AI attribution" || true

PLAIN="$SCENARIO_DIR/plain"
mkdir -p "$PLAIN"
printf 'machines:\n  - name: main\n    hosts: [203.0.113.10]\npackages: v2\n' > "$PLAIN/config.yml"
DEVMACHINE_CONFIG="$PLAIN" "$DEVMACHINE_ACCEPT_BIN" packages add docker --machine main --yes > /dev/null 2>&1
if [ -d "$PLAIN/.git" ]; then
  fail "it does not make a repository on its own" || true
else
  pass "it does not make a repository on its own"
fi

FRESH="$SCENARIO_DIR/fresh"
mkdir -p "$FRESH"
printf 'machines:\n  - name: main\n    hosts: [203.0.113.10]\npackages: v2\n' > "$FRESH/config.yml"
: > "$ACCEPT_GH_LOG"
PUBLISHED=$(DEVMACHINE_CONFIG="$FRESH" "$DEVMACHINE_ACCEPT_BIN" setup git --yes 2>&1)
if ! printf '%s' "$PUBLISHED" | grep -Fq -- "created a private repository" && [ ! -s "$ACCEPT_GH_LOG" ]; then
  pass "--yes does not create a remote"
else
  fail "--yes does not create a remote" || true
fi
contains "$PUBLISHED" "git remote add origin" "and says how to add one by hand" || true

OLD="$SCENARIO_DIR/old"
mkdir -p "$OLD/keys"
printf 'machines:\n  - name: main\n    hosts: [203.0.113.10]\npackages: v2\n' > "$OLD/config.yml"
printf 'PRIVATE KEY\n' > "$OLD/keys/id_ed25519"
git -C "$OLD" init -q -b main
DEVMACHINE_CONFIG="$OLD" "$DEVMACHINE_ACCEPT_BIN" setup git --yes > /dev/null 2>&1
IGNORE=$(cat "$OLD/.gitignore" 2>/dev/null)
contains "$IGNORE" "keys/" "a directory that was already a repository gets the ignore file" || true

DIRTY_TWO="$SCENARIO_DIR/dirty-two"
mkdir -p "$DIRTY_TWO"
printf 'machines:\n  - name: main\n    hosts: [203.0.113.10]\npackages: v2\n' > "$DIRTY_TWO/config.yml"
git -C "$DIRTY_TWO" init -q -b main
git -C "$DIRTY_TWO" add -A && git -C "$DIRTY_TWO" commit -qm "by hand"
mkdir -p "$DIRTY_TWO/keys"
printf 'PRIVATE KEY\n' > "$DIRTY_TWO/keys/id_ed25519"
DEVMACHINE_CONFIG="$DIRTY_TWO" "$DEVMACHINE_ACCEPT_BIN" packages add docker --machine main --yes > /dev/null 2>&1
STAGED=$(git -C "$DIRTY_TWO" diff --cached --name-only)
refutes "$STAGED" "keys/id_ed25519" "a refused commit leaves the key unstaged" || true

EXAMPLE=$("$DEVMACHINE_ACCEPT_BIN" secrets example 2>&1)
refutes "$EXAMPLE" "s3cret" "secrets example prints no value" || true

scenario_done 29 "setup git"
