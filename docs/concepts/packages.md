# Packages

Everything a machine gets is a package, including the things every machine gets.
There is no second mechanism and no privileged set compiled into the binary.

That is the whole idea: **the CLI knows how to fetch a recipe, send it to a
machine and run it. It does not know what Docker is.** Fixing how Docker is
installed is a push to the packages repository, not a new release of the CLI,
and not something you upgrade a binary to receive.

## What a package is

A recipe, not an installer for one binary. A package can configure as many
things as the job needs: a service, its systemd unit, its routes, a file in
somebody's home. A VPN package that touches OpenVPN, systemd, routing tables and
firewall rules is one package, because installing it is one decision.

A package **is** an Ansible role, with one small file beside it:

```
packages/caddy/
  package.yml        what Ansible has no concept of
  tasks/main.yml     an ordinary role
  handlers/  files/  templates/  defaults/  vars/
```

Nothing is translated. What is written is what runs, so a failure points at the
line somebody wrote rather than at generated YAML they have never seen.

The fields, and what each one is for, are in
[the package format](../reference/package-format.md).

## Scope is the package's to declare, the targets are yours to choose

A recipe says whether it belongs to a **machine** or to a **workspace**, because
that is a property of the software rather than a preference. Docker is installed
once and serves everyone. Claude Code has a login and a credential per person,
so it belongs to a workspace.

A workspace package goes on as many workspaces as you want — the same tool on
five of nine is the ordinary case. Each machine and each workspace lists what it
has, so reading one tells you what it is:

```yaml
machines:
  - name: main
    packages: [base, docker, caddy, firewall, fail2ban, ssh_hardening]

workspaces:
  - name: alice
    machine: main
    packages: [claude-code]
```

Naming the wrong kind of target is an error that says so, rather than installing
something where it cannot work.

## Ordering is declared, never implied

A package names what must already be on the same target:

```yaml
needs: [firewall]
```

`caddy` needs `firewall` because getting a certificate over HTTP-01 needs port
80 open. That is written down, so installing `caddy` on its own works, and so
that reading the recipe tells you the constraint.

The alternative — ordering by where something sits in a list — is how a
provisioning repository rots. The result starts depending on what ran first, the
coupling becomes invisible, and nobody can predict the final state by reading.

## A package may override another, but never patch one

A package in your own configuration directory replaces an official one of the
same name. No syntax is needed: **the name is the override.**

What a package may *not* do is modify what another package installed. The
legitimate need underneath is real, though — adding a block to Caddy is the
obvious case. The answer is a declared extension point, not a patch:

```yaml
# packages/caddy/package.yml
provides:
  sites.d: /etc/caddy/sites.d

# packages/sharing/package.yml
extends:
  caddy.sites.d: files/sharing.caddy
```

The extended package owns a place and says others may write there; the extender
writes in that place. The coupling ends up in both files, where somebody reads
it instead of discovering it three hours into a debugging session.

`devmachine sync` refuses an extension whose provider is not installed on the
same machine.

## Your own configuration is a package source

There is no separate overlay mechanism. `<config>/packages/` holds packages, in
the same format, with the same `package.yml`. The only difference is where they
came from: your disk rather than a release.

A private client's VPN and a personal bot are packages like any other. Nothing
about them is a special case, and publishing one later means moving a directory
rather than rewriting it.

That makes a whole personal configuration three things: **machines, workspaces,
and packages.**

## A default set, not a privileged layer

A machine with no packages is a bare server, so `setup` writes the set that makes
a machine a machine — `base`, `docker`, `caddy`, `firewall`, `fail2ban`,
`ssh_hardening`, `git`. Any of them can be removed afterwards.

They are ordinary packages. They arrive by default because a new machine without
them is not useful, not because the CLI treats them differently.

## Where a recipe comes from

One repository, `github.com/adevmachine/packages`. On a tag, its CI builds a
tarball and attaches it to the release along with a `checksums.txt`. The CLI
makes two plain HTTP requests, verifies the checksum and extracts.

Your configuration pins which release to use:

```yaml
packages: v1
```

**A pinned version, never a branch.** `v1` is a set that was tested together.
Moving to a newer one is a deliberate act with a diff to read, not something
that happens inside a routine `sync` because somebody pushed an hour ago.

**A release asset rather than the tag's archive**, because a tag can be moved
and its archive will silently change underneath you. The asset's checksum goes
into `packages.lock`, so content that changes is refused and named.

A recipe is cached once fetched, and copied to the machine alongside everything
else `sync` sends. After the first fetch, converging needs no network at all —
see [why nothing is embedded](../how-it-works/why-nothing-is-embedded.md) for
what that costs.

## What `packages.lock` is for

It records what is installed, where, and at which version. That is what makes
two machines reproducible from the same configuration, and what answers "what is
actually on this machine" without logging in to look.

## A package can declare an entrypoint

A package may add `kind`, `entrypoint` and `commands` to its manifest: an
executable the CLI can call on the machine, in a shape it understands. `kind:
dns` is the only kind so far — see [the DNS provider
contract](../reference/dns-provider-contract.md). `commands` is what that
entrypoint accepts, a list, or `["*"]` for anything.

`devmachine run --package <name> -- <command>` reaches it directly:

```
devmachine run --package cloudflare -- zones
```

This is the generic door onto a machine — it removes the need to know an
address, an account, a port or a key, and it does not limit what a shell can
do. `commands:` is what limits, and only for a package that asked to be
limited: `run --package` refuses anything the manifest does not list.

`devmachine packages help <name>` asks the package what it accepts, by
running its own `help` and printing what comes back. The package is the
source of truth about itself — nothing here is written in the CLI or in a
document somebody has to keep in sync.

Say plainly what this is not: `run` is not a permission system, and an
entrypoint with `commands: ["*"]` is exactly as capable as a shell on that
machine. It is a name for a thing that already runs there, not a fence around
it.

`run --package` is the way out when no verb fits yet. A week of
`~/.config/devmachine/history.log` is the list of verbs still missing — a
better backlog than guessing at one.
