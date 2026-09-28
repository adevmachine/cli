# Machines and workspaces

Two ideas carry everything else. A **machine** is a server the CLI can
reach — you can have several, each with its own address, login, port, and
key. A **workspace** is an environment on one of them: normally one Linux
user on one machine, and what you name in a command.

## Your computer

**Your computer** is the one you run `devmachine` on — usually your
laptop. In this manual, the VPS is never "your computer". Your computer
can also be listed as a machine the CLI manages, with `self: true`:

```yaml
machines:
  - name: mac
    self: true
```

It has no `hosts`, `user`, `port`, or `key`: there's no address for the
computer you're sitting at. `sync` just writes its files to a folder here
and runs Ansible directly, no SSH involved. Everything else works the same
way.

This isn't the same as the Lima VM from `machines create-local`. That VM
has its own address, its own key, its own `setup` — a machine that happens
to live on your computer, but reached like any other. The two are kept
apart on purpose: see
[Your computer as a machine](../how-it-works/your-computer-as-a-machine.md)
for why.

**A workspace can never run on a self machine.** A workspace is a Linux
account reached over SSH, and a self machine has no account and no SSH
server to reach. Add one with `devmachine machines add --self <name>`,
set it up with `devmachine setup`, and use it for things that should run
on your own Mac — see [commands](../reference/commands.md#machines) for
what it refuses to do, and why.

## Why a workspace, and not just a user

Where a workspace runs is a property of the workspace, so you never repeat
it in a command:

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

Neither command names an address, a port, or a key. Moving `bob` to
another machine is one line of configuration, and nothing you type
changes.

## The Linux account

A workspace's account is its own name: `alice` owns the Linux user
`alice`. `user:` overrides that, for when the name is already taken on
that machine, or two workspaces would collide:

```yaml
workspaces:
  - name: bob
    machine: sandbox
    user: bob-dev
```

## Leaving the machine out

`machine:` is optional when you have exactly one machine — there's only
one place a workspace could live. With several machines it's required: a
workspace that doesn't say where it runs is an error, not a guess.

## Where Ansible fits

Ansible already handles several machines. What it has no idea of is a
workspace — to Ansible, workspaces are just items in a loop, with no sense
of which machine each belongs to. So the CLI keeps track of workspaces
itself, and builds the machine list and settings Ansible needs from them.
Ansible still does the actual work of setting each machine up.

| Layer | Owner |
| --- | --- |
| workspace → machine | the CLI |
| machine list and settings | the CLI generates them |
| setting up each machine | Ansible |
| sessions, stats, DNS, diagnosis | the CLI, straight over SSH |

## Making one

```
devmachine workspaces new alice
devmachine workspaces new bob --like alice
devmachine sync
```

`new` writes a line in `config.yml` and **touches no machine**. `sync` is
what creates the account — stopping after `new` leaves a workspace that
exists only on paper.

A new workspace gets, unless you say otherwise, `defaults.workspace` from
your configuration — set by `setup`, and yours to change:

```yaml
defaults:
  workspace: [workspace, dev, zsh, mise]
```

Change that line, and every workspace made afterwards is different.
`--like bob` copies another workspace's package list instead — nothing
else, since a copied account name would collide and a copied machine
would put the new workspace in the wrong place.

**A workspace can't be created on a machine with no key.** Reaching it
needs a copy of that key, and with none there's nothing to copy. Run
`devmachine setup` first.

## Removing one

```
devmachine workspaces rm alice
```

This removes the entry from `config.yml`. **The Linux account, its home
folder, and its files stay on the machine**, and the command says so.
Deleting a home folder isn't something a configuration edit should do,
and `sync` couldn't undo it. If you want the account gone, remove it on
the machine yourself, on purpose.

## Reaching one by name

```
devmachine aliases --write
mosh alice-devmachine
```

The CLI already knows every workspace, its machine, its address, its
port, and its key, so it can write SSH shortcuts straight from your
configuration — no machine involved.

It replaces a **marked block**, never the whole file:

```
# >>> devmachine — generated, do not edit
Host alice-devmachine
    HostName 100.64.0.5
    User alice
    HostKeyAlias main-devmachine
# <<< devmachine
```

Your `~/.ssh/config` holds other hosts this CLI knows nothing about — a
work jump host, a sandbox. Rewriting the whole file would erase them.

`HostKeyAlias` stays the same across every shortcut for one machine. SSH
normally tracks a machine by its address, so the same machine at two
addresses looks like two different ones, and switching between them
triggers a false `Host key verification failed` warning. `HostKeyAlias`
tracks it by name instead, so both addresses count as the same machine.

A `-pub` shortcut appears only when there's a second, fallback address.
With one address, you never see one.
