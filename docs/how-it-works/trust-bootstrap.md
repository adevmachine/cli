# Setting up a server for the first time

Setting up a server you have never logged into before is the hardest
moment this CLI handles, and the first thing a new user hits. This page
explains what `devmachine setup` does, and why it does the steps in this
order.

## It does not ask which situation you are in

Most providers let you paste an SSH key in when you create the server. So
a lot of people arrive with a key that already works — and asking them for
a root password would be asking for something they never had.

So `setup` finds out for itself:

1. Where the machine is, which account to use, and which port.
2. Which key to use: make a new one, point at a file, or pick one your SSH
   agent already holds.
3. **Try the key.** If it works, there is nothing to install — skip ahead
   to locking the server down.
4. Only if the key does not work does it ask for the password.

## The proof is a new connection

Installing a key does not mean the key works. A wrong file permission, a
home folder anyone can write to, a setting pointing somewhere else,
SELinux — any of these can let the key install cleanly and still refuse
every login.

The connection that installed the key cannot prove it works, because that
connection used the password, not the key. So `setup` closes it, opens a
**brand new connection using only the key**, and only counts that as
proof.

## Locking down comes after the proof, never with it

Turning the password login off before proving the key works would be like
locking the door with the key still inside — if the key does not work, you
are locked out too.

If the proof fails, `setup` stops, tells you what to check, and says
plainly:

```
password login is still on: you can still get in with the password
```

That is the whole reason the steps are separate. `--no-harden` skips
locking the server down; the key is still installed and still proved to
work.

Locking down means writing one file,
`/etc/ssh/sshd_config.d/00-devmachine-hardening.conf`, that turns password
login off. devmachine checks the file is valid before it applies it, and
only reloads SSH if the check passes. **If the file is invalid, devmachine
deletes it** instead of leaving it behind — a bad file left on disk would
break the next SSH reload, even a routine reboot.

### Why the file name starts with `00`

SSH reads its settings files in order and uses the **first** value it
finds for each one. A drop-in file only wins if it sorts before the
others.

A cloud provider's image often ships its own settings file, named
something like `60-cloudimg-settings.conf`. A file named `99-` would be
read after it and **silently do nothing** — no error, no warning, and
password login still on even though the run said it succeeded.

## The password is never stored

It lives in memory for one connection, then it is gone. It is never
written to `config.yml`, the lock file, the log file, or anywhere else.
There is no command to store it, because there is nothing to keep.

When you type it at a terminal, it is never shown on screen.

## Why this does not just run the `ssh` command

If your SSH agent holds several keys, a password login can fail **before
you ever see the password prompt**: the agent offers key after key, and
the server cuts the connection after too many tries. On a server you are
connecting to for the first time, that happens every time.

`setup` talks to SSH directly in code, instead of running the `ssh`
command, so it can offer exactly one thing at a time — the key, or the
password, never both. Sessions you use yourself, like `ssh` and `mosh`,
still run the real `ssh` command, because there you want your own
terminal, your own agent, your own tmux.

## After this step

`setup` installs `ansible` and `git`. That is the last thing it does by
hand. From then on, every change to the machine goes through `devmachine
sync`.

It installs the `ansible` package rather than `ansible-core`: the smaller
package leaves out `community.general`, which the `firewall` package
needs.
