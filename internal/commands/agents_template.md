# AGENTS.md

This folder is a devmachine configuration: `config.yml`, `packages/`,
`packages.lock`, `known_hosts`. Anything under `keys/` and `secrets.json` is
private and never committed.

## Rules for an agent working here

- Use the `devmachine` CLI for every change, and every question about
  machine state. Never hand-edit `config.yml` when a command exists for it.
- Never answer "what is published" or "what DNS records exist" by reading
  files. Run `devmachine expose list`, `devmachine dns list`, or
  `devmachine config show`.
- Read `devmachine --help`, the installed `use-devmachine` skill, or
  https://mydevmachine.sh/llms-full.txt for the full command set.
- Ask the person before any command that contacts a machine: `sync`,
  `doctor`, `run`, `ssh`, `dns`, `expose`, and the like. Show `--check`
  first when the command has one.
- Logins and secrets are the person's to type. Never ask for a password or
  a token, and never type one into a command.
- Your own packages go in `packages/`. Use `devmachine packages new` to
  start one and `devmachine packages validate` to check it.

## Your notes

<!-- Add what your agent should know about this setup: which machine is
production, what to ask before touching it, notes about your network. -->
