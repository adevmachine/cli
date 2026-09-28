# Try it on your own computer first

Run a virtual machine on your computer and use it exactly like a VPS: set it
up, make workspaces, add packages, open them. Nothing to buy, and you can
throw it away when you are done.

**You need:** a Mac or Linux computer and [Lima](https://lima-vm.io), which
runs the virtual machine: `brew install lima`.

## 1. Create the machine

```
devmachine machines create-local sandbox
```

It prints the machine's address, its port and the root password:

```
sandbox      127.0.0.1                    port 60022  admin: root
It has no key on it yet, and the root password is "devmachine".
```

The machine arrives the way a bought server does: reachable as root with a
password, and no key yet. The password is public on purpose — this machine
holds nothing real.

## 2. Set it up

If this is your first machine:

```
devmachine setup
```

If you already have a machine configured, add this one next to it:

```
devmachine machines add
```

Answer with what step 1 printed: address `127.0.0.1`, login `root`, the port
it showed, and the password when asked. Everything else is the same as on a
real server: the fingerprint check, the key, password logins turned off.

## 3. Use it like a VPS

```
devmachine workspaces new alice --machine sandbox
devmachine packages add claude-code --workspace alice
devmachine sync --machine sandbox
devmachine ssh alice
```

`--machine sandbox` is needed only when you have more than one machine.

## What does not work on a local machine

The internet cannot reach a virtual machine on your computer, so `dns`,
HTTPS certificates and `expose` do not work there. Everything else does. To
see an app you run in the VM, use a tunnel instead of a URL:

```
devmachine tunnel alice 3000
```

## Stop it, start it, throw it away

```
devmachine machines stop sandbox
devmachine machines start sandbox
devmachine machines delete-local sandbox
```

`delete-local` destroys the VM and everything in it. Then take it out of your
configuration: first its workspaces with `devmachine workspaces rm alice`,
then the machine with `devmachine machines rm sandbox` — it refuses while a
workspace still points at it.

**Check it:** `devmachine doctor --machine sandbox` passes every line after
step 2.

Source: [Lima](https://lima-vm.io/docs/)
