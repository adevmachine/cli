# Try it on your own computer first

Run a virtual machine on your computer and use it exactly like a VPS: set it
up, make workspaces, add packages, open them. Nothing to buy, and you can
throw it away when you are done.

**You need:** a Mac or Linux computer and [Lima](https://lima-vm.io), which
runs the virtual machine: `brew install lima`.

## Before you start: machine, skills, workspace

A local VM is not a bought server, so it is made and added differently, but
everything after that is the same.

```
brew install mydevmachine/tap/devmachine
devmachine machines create-local sandbox
devmachine setup
devmachine skills add
devmachine workspaces new alice --machine sandbox
devmachine sync
```

`create-local` prints the machine's address, its port and the root
password:

```
sandbox      127.0.0.1                    port 60022  admin: root
It has no key on it yet, and the root password is "devmachine".
```

The machine arrives the way a bought server does: reachable as root with a
password, and no key yet. The password is public on purpose — this machine
holds nothing real. Run `devmachine setup` if this is your first machine;
if you already have one configured, run `devmachine machines add` instead,
next to it. Either way, answer with what `create-local` printed: address
`127.0.0.1`, login `root`, the port it showed, and the password when asked.
Everything else is the same as on a real server — see
[getting started](../getting-started.md) for what each command does.

## By hand

### 1. Use it like a VPS

```
devmachine packages add claude-code --workspace alice
devmachine sync --machine sandbox
devmachine ssh alice
```

`--machine sandbox` is needed only when you have more than one machine.

### 2. What does not work on a local machine

The internet cannot reach a virtual machine on your computer, so `dns`,
HTTPS certificates and `expose` do not work there — see
[publishing](../concepts/publishing.md). Everything else does. To see an app
you run in the VM, use a tunnel instead of a URL:

```
devmachine tunnel alice 3000
```

### 3. Stop it, start it, throw it away

```
devmachine machines stop sandbox
devmachine machines start sandbox
devmachine machines delete-local sandbox
```

`delete-local` destroys the VM and everything in it. Then take it out of
your configuration: first its workspaces with `devmachine workspaces rm
alice`, then the machine with `devmachine machines rm sandbox` — it refuses
while a workspace still points at it.

## With your agent

Open a session on your own computer (`devmachine skills add` taught it the
CLI) and say:

```text
Create a local devmachine VM called sandbox and set it up as a machine,
then create a workspace alice on it with claude-code.
```

The agent runs `machines create-local`, then `setup` or `machines add`. You
still answer the fingerprint check yourself, even on a local VM — the agent
tells you the address, port and password to use, but does not skip that
step. After that, it creates the workspace, adds the package, and asks
before running `sync`.

**Check it:** `devmachine doctor --machine sandbox` passes every line after
setup.

Source: [Lima](https://lima-vm.io/docs/)
