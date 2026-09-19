# The trust bootstrap

Taking over a server you have never logged into is the hardest path this CLI
has, and the one a stranger hits first. This is what `devmachine setup` does and
why the order is what it is.

## It does not ask which situation you are in

Most providers let you paste an SSH key when you create the server. A large
share of people therefore arrive with a key that already works — and asking
them for a root password would be asking for something they never had.

So `setup` finds out instead:

1. Where the machine is, who the administrative account is, which port.
2. Which key to use: make a new one, point at a file, or pick one your SSH
   agent already holds.
3. **Try the key.** If it works, there is nothing to install — skip to
   hardening.
4. Only if it does not, ask for the password.

## The proof is a new connection

Installing a key does not mean the key works. `authorized_keys` with the wrong
mode, a home directory that is group-writable, an `AuthorizedKeysFile` pointing
somewhere else, SELinux — every one of those installs cleanly and then refuses
every login.

The session that installed the key cannot prove anything about it: that session
was authenticated by a password. So `setup` closes it, opens a **new
connection offering the key and nothing else**, and only counts that as proof.

## Hardening comes after the proof, never with it

Turning password authentication off before proving the key is locking the door
with the key still inside.

If the proof fails, `setup` stops, names what to check, and says plainly:

```
password login is still on: you can still get in with the password
```

That is the whole reason the steps are separate. `--no-harden` skips hardening
entirely; the key is still installed and still proved.

Hardening itself writes `/etc/ssh/sshd_config.d/00-devmachine-hardening.conf`,
validates it with `sshd -t`, and reloads only if sshd accepted it. **A file sshd
rejects is removed rather than left behind** — left on disk it would break the
next reload by anything at all, a reboot included.

### Why the name starts with `00`

sshd uses the **first** value it finds for each directive, and the `Include` of
`sshd_config.d` sits at the top of the main configuration. So a drop-in only
wins if it sorts before the others.

A cloud image ships its own settings in a file like
`60-cloudimg-settings.conf`. A drop-in named `99-` would be read second and
**silently do nothing** — no error, no warning, and password login still on
after a run that reported success.

## The password is never stored

It lives in process memory, is used once, and is gone. It is not in
`config.yml`, not in `packages.lock`, not in `history.log`, not in a log line.
There is no `devmachine secrets set` for it, because there is nothing to keep.

When the input is a terminal it is read without echo.

## Why this does not shell out to `ssh`

An SSH agent holding many keys makes a password login fail **before the prompt
appears**: the client offers key after key and the server cuts the connection at
`MaxAuthTries`. On a machine you are meeting for the first time, that is every
time.

`setup` uses `golang.org/x/crypto/ssh` so it can offer exactly one method — the
key, or the password — and never a list. Interactive sessions (`ssh`, `mosh`)
still exec the system binaries, because there you want your terminal, your
agent and your tmux.

## After the bootstrap

`setup` installs `ansible` and `git`, and that is the last thing done by hand.
Everything after it is `devmachine sync`.

It installs `ansible` rather than `ansible-core`: the core package alone does
not carry `community.general`, which the `firewall` package needs.
