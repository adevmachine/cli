# Setting up a server for the first time

Setting up a server you have never logged into is the hardest moment this
CLI handles, and the first thing a new user hits. This page explains what
`devmachine setup` does, and why the steps run in this order.

## It does not ask which situation you are in

Most providers let you paste an SSH key in when you create the server, so
a lot of people arrive with a key that already works — asking them for a
root password would ask for something they never had. So `setup` finds
out for itself: where the machine is, which account and port, which key
to use, then **tries the key first**. Only if that fails does it ask for
the password.

## The proof is a new connection

Installing a key does not mean it works — a wrong file permission, a
world-writable home folder, SELinux, any of these can let a key install
cleanly and still refuse every login. The connection that installed the
key cannot prove it works either, since that connection used the
password. So `setup` closes it, opens a **brand new connection using only
the key**, and only counts that as proof.

## Locking down comes after the proof, never with it

Turning password login off before proving the key works is locking the
door with the key still inside. If the proof fails, `setup` stops and
says plainly:

```
password login is still on: you can still get in with the password
```

`--no-harden` skips locking the server down; the key is still installed
and proved.

Locking down means writing one file that turns password login off,
checking it is valid, and only reloading SSH if it passes — **an invalid
file is deleted**, since a bad file left behind would break the next SSH
reload, even a routine reboot.

### Why the file name starts with `00`

SSH uses the **first** value it finds for each setting, so a drop-in file
only wins if it sorts before others. A cloud image often ships its own
settings as `60-cloudimg-settings.conf`; a file named `99-` would read
after it and **silently do nothing** — no warning, password login still
on despite a successful run.

## The password is never stored

It lives in memory for one connection, then is gone — never written to
`config.yml`, the lock file, or the log. Typed at a terminal, it is never
shown on screen.

## Why this does not just run the `ssh` command

If your SSH agent holds several keys, a password login can fail **before
you see the prompt**: the agent offers key after key, and the server cuts
the connection after too many tries — on a server you are meeting for the
first time, that happens every time.

`setup` talks to SSH directly in code, so it offers exactly one thing at
a time — the key, or the password, never both. Sessions you use
yourself, `ssh` and `mosh`, still run the real command, since there you
want your own terminal and agent.

## After this step

`setup` installs `ansible` and `git` — the last thing done by hand. From
then on, every change goes through `devmachine sync`. It installs
`ansible` rather than `ansible-core`, since the smaller package leaves
out a piece the `firewall` package needs.
