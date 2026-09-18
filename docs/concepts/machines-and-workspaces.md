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
