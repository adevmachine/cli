# devmachine documentation

Set up and operate a personal development VPS.

## Start here

- [Getting started](getting-started.md) — install it, point it at a machine, see
  that it works.

## Concepts

- [Machines and workspaces](concepts/machines-and-workspaces.md) — the two ideas
  everything else is built on.
- [Configuration](concepts/configuration.md) — where it lives, what it holds,
  and how the CLI finds it.

## How it works

The reasoning behind decisions that are not obvious from the outside. Read these
when something behaves in a way that surprises you.

- [SSH and authentication](how-it-works/ssh-and-authentication.md) — why the CLI
  offers one key and never the whole agent.
- [Addresses and fallback](how-it-works/addresses-and-fallback.md) — how a
  machine with several addresses is reached, and what is dropped silently.
- [Choosing a target](how-it-works/choosing-a-target.md) — why a command asks
  instead of guessing which machine it runs against.

## Reference

- [Commands](reference/commands.md) — every command, its flags and its output.
- [The package format](reference/package-format.md) — every `package.yml`
  field, and the message behind every validation rule.

## Fixing things

- [Troubleshooting](troubleshooting.md) — errors you are likely to meet, and what
  each one really means.

## Contributing

- [Development](development.md) — building, testing against a real machine, and
  the rules this repository holds itself to.
- [Releasing](releasing.md) — how a version reaches Homebrew.
