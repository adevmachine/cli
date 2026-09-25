# Handoff — where the CLI stands, 2026-09-25

Read this first, then the current specs and plans in
`~/dev/devmachine/docs/superpowers/`. This file records state; those documents
record the design and implementation reasoning.

## Released and pending

| Repository | Released | Pending branch |
| --- | --- | --- |
| `github.com/adevmachine/cli` | `v0.6.3` | `da-agent-skills` |
| `github.com/adevmachine/packages` | `v5` | `da-agent-skills` |
| Devmachine macOS app | current `main` | `da-expose-cli` |

The pending CLI implements native agent skills, workspace package/default
editing, additive ownership, Claude compatibility links, and check-mode-safe
convergence. The pending package release adds the public `devmachine-skills`
package. The app branch delegates publication and listing to
`devmachine expose`, with an explicit workspace picker, and no longer edits
Caddy, DNS, Ansible, or the old repository itself.

## Evidence already collected

- CLI: race tests, lint, 80.9% coverage, generated surface, docs, and the
  no-real-data check are green.
- CLI integration: `make test-vps` is green against the existing disposable
  Lima machine. The machine was stopped afterward and its disk was preserved.
- Full acceptance: the skill-package scenario passed dry-run, apply, ownership,
  support-file, workspace-isolation, and second-sync idempotence checks on one
  reused fake machine.
- Packages: validation, Ansible syntax, and no-real-data checks are green.
- App: 617 tests, build, and no-real-data checks are green.
- The private configuration repository is clean and locally committed. Existing
  workspaces and future defaults select the shared skill packages. The public
  package intentionally reports `missing` until its next package release.
- No command was run against the real VPS.

## Release order

The CLI must release before the packages repository. Package CI downloads the
latest CLI release, so publishing the package first makes its new `skills:`
manifest field fail against the old validator.

After the feature branches are integrated:

1. Push CLI `main`, wait for CI, tag `v0.7.0`, push the tag, and wait for the
   release and Homebrew update.
2. Re-run package checks with the released CLI, push package `main`, wait for
   CI, tag `v6`, push the tag, and wait for its release.
3. Install or upgrade the released CLI locally, then install the public and
   private skill packages for the desired local agent harnesses.
4. Only with the operator's explicit approval, run a real-machine dry-run and
   inspect it before a separate approval for apply.

## Commands that are safe without the real VPS

```sh
make fmt
make test
make lint
make cover
make surface
make docs
scripts/check-no-real-data.sh
make test-vps
```

`make test-vps` uses the disposable Lima machine through
`scripts/fake-vps.sh`; it does not target the configured production machine.

## Commands that require explicit operator approval

Any command that can resolve the production configuration and connect to its
machine requires approval in that turn. This includes `sync --check`, `doctor`,
`run`, `ssh`, `expose`, `dns`, login delivery, and the app's live refresh or
publish actions.

Never infer approval for apply from approval for dry-run. Present the exact
command first, run it once, and inspect the result before asking for the next
write.

## House rules

- Nothing contacts the real VPS without exact-command approval.
- Use one reusable fake machine for related integration checks; do not create a
  fleet of fresh machines.
- Everything written to disk is English; chat may be Portuguese.
- Public repositories contain no personal hostname, IP, domain, username,
  email, token, or private workspace name.
- Use Conventional Commits with no AI attribution or co-author trailer.
- Preserve unmanaged remote files. Skill convergence only owns paths recorded
  for a package and refuses unmanaged collisions.
- Before release, follow `docs/releasing.md`; a release is a tag and the CI
  workflow produces binaries and updates Homebrew.
