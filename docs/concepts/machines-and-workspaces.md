# Machines and workspaces

Two ideas carry everything else.

A **machine** is a server the CLI can reach. There can be several. Each has its
own addresses, administrative login, port and key.

A **workspace** is an environment: normally one Linux user on one machine. It is
what a person works in and what they name in a command.

## Why a workspace, and not just a user

Where a workspace runs is a property of the workspace, not something you repeat
in every command. That is the whole point:

```yaml
machines:
  - name: main
    hosts: [203.0.113.10]
  - name: sandbox
    hosts: [198.51.100.7]
    port: 2222
    key: /keys/sandbox

workspaces:
  - name: alice
    machine: main
  - name: bob
    machine: sandbox
```

```
devmachine ssh alice     # lands on main
devmachine ssh bob       # lands on the sandbox
```

Neither command mentions an address, a port or a key. Moving `bob` to another
machine is one line of configuration, and nothing anyone types changes.

## The Linux account

A workspace's account is its own name. `alice` owns the Linux user `alice`.

`user:` overrides that, for when the name cannot be the account — it is taken on
that machine, or two workspaces would collide:

```yaml
workspaces:
  - name: bob
    machine: sandbox
    user: bob-dev
```

## Leaving the machine out

`machine:` is optional when there is exactly one machine — a workspace can only
live in one place, so there is nothing to say.

With several machines it is required. A workspace that does not say where it
runs is an error, not a guess.

## Where Ansible fits

Ansible already handles several machines: that is what an inventory is. What it
has no idea of is a workspace — to Ansible those are items in a loop, with no
notion of which host each belongs to.

So the CLI owns the workspace model and generates the inventory and the per-host
variables from it. Ansible keeps converging the machines. Nothing about it is
reinvented.

| Layer | Owner |
| --- | --- |
| workspace → machine | the CLI |
| inventory and host variables | the CLI generates them |
| converging a machine | Ansible |
| sessions, stats, DNS, diagnosis | the CLI, straight over SSH |

## Making one

```
devmachine workspaces new alice
devmachine workspaces new bob --like alice
devmachine sync
```

`new` writes a line in `config.yml` and **touches no machine**. `sync` is what
creates the account. The command says so, because somebody who stops after the
first line has a workspace that exists only in their configuration.

What a new workspace gets, when no flag says otherwise, is `defaults.workspace`
in your own configuration — seeded by `setup`, and yours to change:

```yaml
defaults:
  workspace: [workspace, dev, zsh, mise]
```

Change that one line and every workspace made afterwards is different. `--like
bob` copies another workspace's packages instead, and **only its packages**: a
copied account name would collide, and a copied machine would put the new
workspace wherever the old one happens to be.

**A workspace cannot be created on a machine with no key.** It is reachable
because the administrative key is copied into it, and with no key there is
nothing to copy — the account would exist with no way in. Run `devmachine
setup` first.

## Removing one

```
devmachine workspaces rm alice
```

This removes the entry from `config.yml`. **The Linux account, its home and its
files stay on the machine**, and the command says so.

Deleting somebody's home is not something a configuration edit should do, and
`sync` could not put it back. If you want the account gone, remove it on the
machine yourself, deliberately.

## Reaching one by name

```
devmachine aliases --write
mosh alice-devmachine
```

The CLI already knows every workspace, its machine, its addresses, its port and
its key, so writing SSH aliases needs no Ansible and no package — it is a local
file, written from your configuration.

It replaces a **delimited block** and never the whole file:

```
# >>> devmachine — generated, do not edit
Host alice-devmachine
    HostName 100.64.0.5
    User alice
    HostKeyAlias main-devmachine
# <<< devmachine
```

Your `~/.ssh/config` holds hosts this CLI knows nothing about — a work jump
host, a sandbox. Rewriting the whole file would eat them.

**`HostKeyAlias` is the same on every alias for one machine**, and that is the
part worth understanding. `known_hosts` is indexed by address, so one machine
reached at two addresses gives two entries, and switching between them gives
`Host key verification failed` on a machine that is perfectly fine.
`HostKeyAlias` indexes by name instead, so the tailnet address and the public
one are the same host as far as SSH is concerned.

A `-pub` alias appears only when there is a second, different address to fall
back to. Somebody with one address never sees one.
