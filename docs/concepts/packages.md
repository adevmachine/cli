# Packages

Everything a machine gets is a package, including the things every
machine gets by default. There's no second mechanism, and no special set
built into the binary.

That's the whole idea: **the CLI knows how to fetch a package, send it to
a machine, and run it. It doesn't know what Docker is.** Fixing how
Docker gets installed is a change to the packages repository, not a new
release of the CLI, and not something a binary upgrade gives you.

## What a package is

A set of setup steps, not an installer for one binary. A package can set
up as many things as the job needs — a service, its file in somebody's
home folder, its firewall rule. A VPN package that touches the VPN
software, routing, and firewall rules is still one package, because
installing it is one decision.

A package is an Ansible role — the standard unit Ansible uses to group
setup steps — with one small file beside it:

```
packages/caddy/
  package.yml        what Ansible has no concept of
  tasks/main.yml     an ordinary role
  handlers/  files/  templates/  defaults/  vars/
```

Nothing is translated. What's written is what runs, so a failure points
at the line somebody wrote, not at generated code nobody's seen. The
fields in `package.yml`, and what each is for, are in
[the package format](../reference/package-format.md).

## Scope is the package's to declare, the targets are yours to choose

A package says whether it belongs to a **machine** or a **workspace**,
because that's a fact about the software, not a preference. Docker is
installed once and serves everyone. Claude Code has a login per person,
so it belongs to a workspace — and can go on as many workspaces as you
want:

```yaml
machines:
  - name: main
    packages: [base, docker, caddy, firewall, fail2ban, ssh_hardening]

workspaces:
  - name: alice
    machine: main
    packages: [claude-code]
```

Naming the wrong kind of target is an error that says so, instead of
installing something where it can't work.

## Ordering is declared, never implied

A package names what must already be on the same target:

```yaml
needs: [firewall]
```

`caddy` needs `firewall`, because getting a TLS certificate needs a port
open. Writing that down means installing `caddy` alone still works, and
reading the package tells you why it needs the other one — instead of the
order in a list silently deciding what runs first.

## A package may override another, but never patch one

A package in your own configuration folder replaces an official one with
the same name — no extra syntax, the name alone is the override.

What a package may *not* do is change what another package installed.
Adding a block to Caddy's config is the obvious case where that's
tempting. The answer is a declared extension point, not a silent patch:

```yaml
# packages/caddy/package.yml
provides:
  sites.d: /etc/caddy/sites.d

# packages/sharing/package.yml
extends:
  caddy.sites.d: files/sharing.caddy
```

The extended package owns a place and says others may write there; the
extending package writes in that place. `devmachine sync` refuses an
extension whose target package isn't installed on the same machine.

## Your own configuration is a package source

There's no separate overlay system. `<config>/packages/` holds packages,
in the same format, with the same `package.yml` — the only difference is
that they came from your disk, not a release. A private client's VPN and
a personal bot are packages like any other. That makes a whole personal
configuration three things: **machines, workspaces, and packages.**

## A default set, not a special layer

A machine with no packages is a bare server, so `setup` writes the set
that makes a machine useful — `base`, `docker`, `caddy`, `firewall`,
`fail2ban`, `ssh_hardening`, `git`. They're ordinary packages: you can
remove any of them afterwards, and the CLI treats none of them
specially.

## Where a package comes from

One repository, `github.com/adevmachine/packages`. Each tagged release
gets a tarball with a checksum file attached; the CLI downloads it,
checks the sum, and extracts it. Your configuration pins which release to
use:

```yaml
packages: v1
```

**A pinned version, never a branch.** `v1` is a set that was tested
together — moving to a newer one is a deliberate step with a diff to
read, not something a routine `sync` picks up because somebody pushed an
hour ago. Pinning to a release file, rather than a moving tag, also means
the content can never silently change underneath you: its checksum is
recorded in `packages.lock`, and a mismatch is refused by name.

A package is cached after the first fetch and copied to the machine
alongside everything else `sync` sends, so after that, setup needs no
network at all — see
[why nothing is embedded](../how-it-works/why-nothing-is-embedded.md) for
what that trade-off costs.

`packages.lock` records what's installed, where, and at which version —
what makes two machines reproducible from the same configuration, and
what answers "what's actually on this machine" without logging in to
look.

## A package can declare an entrypoint

A package may add `kind`, `entrypoint`, and `commands` to its manifest: a
program the CLI can call on the machine. `kind: dns` is the only kind so
far — see
[the DNS provider contract](../reference/dns-provider-contract.md).
`commands` lists what that program accepts, or `["*"]` for anything.

```
devmachine run --package cloudflare -- zones
```

This is a generic door onto a machine, with no address, account, port, or
key to know. `commands:` is the only limit, and only for a package that
asked to be limited — `run --package` refuses anything the manifest
doesn't list. `devmachine packages help <name>` asks the package what it
accepts, by running its own `help`.

Said plainly: `run` is not a permission system. An entrypoint with
`commands: ["*"]` is exactly as capable as a shell on that machine — a
name for something that already runs there, not a fence around it.
`run --package` is the way out when no purpose-built command fits yet.
