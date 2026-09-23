# devmachine documentation

Set up and operate a personal development VPS.

Three commands take a server nobody has logged into and make it yours:

```
devmachine setup      get in, install a key, prove it, harden, install Ansible
devmachine sync       fetch the recipes and converge the machine
devmachine workspaces new alice && devmachine sync
```

No server? `devmachine machines create-local dev` makes one on this computer,
as a bought one arrives.

## Start here

- [Getting started](getting-started.md) — install it, point it at a machine, see
  that it works.

## Concepts

- [Machines and workspaces](concepts/machines-and-workspaces.md) — the two ideas
  everything else is built on.
- [Configuration](concepts/configuration.md) — where it lives, what it holds,
  and how the CLI finds it.
- [Packages](concepts/packages.md) — everything a machine gets, including the
  things every machine gets.
- [Credentials](concepts/credentials.md) — what a workspace has to be signed in
  to, and which part of that a machine can do for you.
- [DNS](concepts/dns.md) — zones, records, how a provider is chosen, and why
  `dns status` asks a different question from `dns list`.
- [Publishing](concepts/publishing.md) — `expose` and `tunnel`, and why the
  question is who should reach a port rather than which protocol it speaks.

## How it works

The reasoning behind decisions that are not obvious from the outside. Read these
when something behaves in a way that surprises you.

- [SSH and authentication](how-it-works/ssh-and-authentication.md) — why the CLI
  offers one key and never the whole agent.
- [SSH host keys](how-it-works/ssh-host-keys.md) — how first trust, strict
  verification and deliberate rotation protect every connection.
- [Addresses and fallback](how-it-works/addresses-and-fallback.md) — how a
  machine with several addresses is reached, and what is dropped silently.
- [Choosing a target](how-it-works/choosing-a-target.md) — why a command asks
  instead of guessing which machine it runs against.
- [Why nothing is embedded](how-it-works/why-nothing-is-embedded.md) — what
  fetching every recipe buys, and the one thing it costs.
- [Why a login cannot be automated](how-it-works/why-a-login-cannot-be-automated.md)
  — the first question everybody asks.
- [Sharing a login](how-it-works/sharing-a-login.md) — why the CLI generates the
  copying itself, and what happens to a workspace that keeps its own account.
- [DNS providers](how-it-works/dns-providers.md) — what each shipped provider
  does, and the mistakes its API design invites.
- [The trust bootstrap](how-it-works/trust-bootstrap.md) — how a server you have
  never logged into becomes one the CLI owns, and why the order matters.
- [Versioning your configuration](how-it-works/versioning-your-configuration.md)
  — what `devmachine setup git` commits, why the remote must be private, and
  why the `.gitignore` comes first.

## Reference

- [Commands](reference/commands.md) — every command, its flags and its output.
- [The package format](reference/package-format.md) — every `package.yml`
  field, and the message behind every validation rule.
- [The DNS provider contract](reference/dns-provider-contract.md) — how a DNS
  provider's entrypoint is called, and what it must answer.
- [Settings](reference/settings.md) — every variable the published packages
  accept, generated from their own manifests.

## Fixing things

- [Troubleshooting](troubleshooting.md) — errors you are likely to meet, and what
  each one really means.

## Contributing

- [Development](development.md) — building, testing against a real machine, and
  the rules this repository holds itself to.
- [Releasing](releasing.md) — how a version reaches Homebrew.
