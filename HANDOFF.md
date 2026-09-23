# Handoff — where the CLI stands, 2026-09-23

Read this first, then `docs/superpowers/specs/2026-09-17-devmachine-cli-design.md`
in `~/dev/devmachine` (the living spec). This file is the state; that file is the
reasoning.

## Released today

| | |
| --- | --- |
| `github.com/adevmachine/cli` | **v0.6.0**, on the Homebrew tap |
| `github.com/adevmachine/packages` | **v3** |

v0.5.0 shipped DNS providers as packages, `tailscale`, `git-key` and
`base.swap`. v0.6.0 shipped `expose`, `tunnel`, `machine setup|doctor` and
`setup git`.

## Unreleased on `main`

- SSH host keys are pinned in `<config>/known_hosts` before authentication and
  verified by every Go/system-SSH path. Existing v0.6 configurations migrate
  with `devmachine machines trust <name>`; replacement is explicit.
- The permanent `make accept` gate now covers setup/Git, the v0.5 and v0.6
  machine proofs, and host-key rejection plus deliberate rotation.
- `dns add` and `expose add` require `--publish` for non-interactive public
  writes; `--yes` alone never publishes.
- The complete local gate, disposable-VM acceptance and GitHub CI are green.

## The one ordering rule that is not written anywhere else

**The CLI releases before the packages repository.** `packages`' CI downloads the
*latest CLI release* to run `scripts/validate.sh`. A manifest field the current
release does not understand fails there, so `main` on `packages` goes red and
nobody notices until they look. It happened twice: `kind: manual` and `kind: vpn`.

Order: tag the CLI → wait for its release workflow → re-run `packages` CI → tag
`packages`.

## What is left to do

### Blocked on registrar credentials

Hostinger's current OpenAPI contract now explicitly says `overwrite: true`
replaces the existing records with the payload. The provider's two-call path —
`DELETE` the RRset, then `PUT` with `overwrite: false` — remains deliberate; a
single-call replacement would erase every other RRset in the zone.

Provider discovery has been proved on `fakevps` with Hostinger and Cloudflare
installed together: a configured Hostinger zone was selected even while the
other provider failed authentication. Completing the external two-provider
proof still needs a Hostinger token with DNS write permission and a valid
Cloudflare token. The available Hostinger token reads DNS but returns 403 on
writes, and the local Cloudflare login is expired.

**Until the external writes run, the Hostinger MCP server stays in `~/dev/devmachine/.mcp.json`
and the `add-subdomain` skill keeps calling it.** Replacing a path that works
with one nobody has watched is not a retirement.

### Three smaller things, in the order I would do them

1. **`packages validate` cannot catch a manifest that lies.** Two defects
   reached a real machine this week that a deeper validation would have caught
   in milliseconds: `commands:` listing a command the entrypoint refuses, and a
   `variables:` entry with a `default:` and no matching `defaults/main.yml`
   behind it (the play died on an undefined variable). A `validate --deep` that
   invokes `help` and compares, and that checks every declared variable has a
   default, would close both.

2. **`sync` runs the workspace packages twice** whenever a machine has a shared
   login, which is most of them. That is correct — a package that *reads* a
   copied credential sees nothing on the run that delivered it, and a `sync`
   that ends with the machine needing another `sync` is the one thing
   convergence must not do — but it is blunt. Re-running only the packages that
   declare they consume a credential would be cheaper.

### Not started

**Converting `~/dev/devmachine` itself.** The spec is
`docs/superpowers/specs/2026-09-21-adopting-the-real-machine-design.md` in that
repo, written and **not committed**, with five open questions. It was blocked on
`setup git`, which now exists. Read it before touching the real VPS: it is an
*adoption*, not a provision — the machine already holds nine accounts, ten live
subdomains, a staging database and a VPN, and a convergence that is merely
correct for a new machine takes those down.

## What acceptance scripts are for, and why they matter here

Unit tests sat at 83% and green through **every** defect found this week. The
things that caught them were the acceptance scripts: a bash script that builds a
throwaway Lima VM from nothing and asserts on what the machine actually does.

Two of them were themselves wrong in a way worth remembering:

- `grep -q "1"` matched the `1` inside `no address answered: 127.0.0.1`
- `grep -q "active"` matched the `active` inside `inactive`

Both reported PASS against a machine that was not running. Both now compare whole
values, through an `is` helper that says why in a comment.

And a v0.6 run passed on a VM whose `/etc/caddy/Caddyfile` had been hand-edited
to get a certificate — the exact failure the script exists to prevent, arriving
through the script. The accommodation was real (no public name resolves to a Lima
VM, so the ACME challenge can never succeed), so it became `caddy.local_certs`, a
setting, off by default. **An accommodation that is declared can be reviewed; one
made by hand inside the machine cannot.**

The scripts now live in `scripts/accept/` and run through `make accept` as part
of the release gate. Each machine scenario owns one uniquely named disposable
Lima VM, reuses it for all assertions in that scenario, and destroys it on exit.
`fakevps` and the real VPS are outside that gate.

## House rules that are easy to break

- **Nothing runs against the real VPS**, not even `--check`, not one `ssh`, unless
  the operator names the command in that turn. Everything is proved on a throwaway
  Lima VM (`devmachine machines create-local`). There is also a VM named
  `fakevps` used by `make test-vps` — leave it alone.
- **Everything written to disk is English.** Code, comments, test names, commit
  messages, docs, fixtures. The chat is Portuguese.
- **No personal data in either public repo.** No real hostname, IP, domain,
  username, email or token. Fixtures use `alice`, `bob`, `example.com`,
  `203.0.113.x`, `100.64.0.x`. `scripts/check-no-real-data.sh` enforces it.
- Conventional Commits, one line, 120 characters. Never `Co-Authored-By`, never
  any AI attribution.
- Commit and push need
  `export SSH_AUTH_SOCK="$HOME/Library/Group Containers/2BUA8C4S2C.com.1password/t/agent.sock"`.
- The gate before every commit:
  `make fmt && make test && make lint && make cover && make surface && make docs`
  plus `scripts/check-no-real-data.sh`. Coverage floor is 80%; it sits at 82.5%.

## Two things that bit me, so they do not bite twice

**A test that never runs is worse than no test.** Four Python tests were added
after a file's `if __name__ == "__main__":` block and were silently never
collected — the suite stayed green and the assertions never ran. After adding
tests, confirm the count went up.

**CI has no git identity and no signing key.** `setup git` makes real commits, so
the workflow now sets `user.email`/`user.name`, and `internal/repo`'s `Commit` no
longer forces `-S`: whether a commit is signed is the operator's own git
configuration, and forcing it broke every machine without a signing key while
overriding a setting that was never the CLI's to override.
