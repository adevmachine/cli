# Your computer as a machine

A machine can declare `self: true`: this means your own computer, the one
running the CLI. This page explains why that exists, why it is called
`self` and not `local`, and what changes about `sync` and `setup` when you
use it.

## Why `self`, not `local`

`machines create-local` already means something else: a small virtual
machine made on your computer, that stands in for a real server while you
learn or test the CLI. That VM has its own address, its own port, its own
root login, its own key — and its own `setup`, exactly like a real server.
It only happens to live on your computer.

`self` is a different thing: your own computer, right now, with no
address to dial and no key to install. Reusing the word "local" for both
would make two different ideas share one word, and the first time someone
typed the wrong one, they would find out the hard way. So this one got its
own word.

## Why no address fields

`hosts`, `user`, `port` and `key` describe how to reach a machine that is
not your computer. A self machine has none of that to describe — there is
nothing to dial, no login, no key to install — so devmachine refuses all
four if you try to set them, naming the field:

```
machine "mac" is your computer (self: true), so it has no hosts: remove it
```

The same reason is why a workspace can never live on a self machine. A
workspace is a Linux account on a server, reached over SSH with a key the
CLI installed. Your own computer has no account system like that and no
SSH server for a workspace to connect through.

## Why it never needs `sudo` for the work itself

A self machine's setup does not ask for extra permissions the way a
server's setup does: Homebrew, mise and Claude all live under your own
home folder on a Mac, and none of them need root. And it only runs on
macOS — nothing about this path has been tried anywhere else, so it stops
with a clear error on anything else.

## Why `setup` prints the Homebrew command instead of running it

Ansible, the tool devmachine uses to apply changes, cannot install
itself — something has to install it first. On a real server, that is what
`setup`'s whole first-time process does: install a key, prove it works,
lock the server down, then install Ansible. None of that applies to your
own computer — there is no address to reach — so `setup` on a self machine
only checks two things: is Homebrew on your `PATH`, and is Ansible.

If Homebrew is missing, `setup` prints the official one-line install
command from [brew.sh](https://brew.sh) and stops. It does not run that
command itself, because it asks for `sudo` — and asking for your password
on your own computer, without you typing it yourself, is not something
this CLI does. Once Homebrew is there, `setup` runs `brew install ansible`
itself and shows you the output, since that part needs no extra
permission.

`sync` follows the same rule: if `ansible-playbook` is not on your `PATH`,
it stops before touching anything and tells you to run `setup`, the same
way it would point out a missing requirement on a real server.

## Where files end up, and why not `/opt`

On a real server, devmachine's files live at `/opt/devmachine` — that is
fine there, because `setup` already has full access to a server it was
given control of. On a Mac, writing to `/opt` needs `sudo`, and asking for
that runs into the same rule as installing Homebrew: it is not this CLI's
place to ask.

So on a self machine, those files live under your own home folder
instead, next to everything else your other tools already put there:

```
$HOME/.local/share/devmachine/bundle
```
