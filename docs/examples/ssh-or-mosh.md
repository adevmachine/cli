# SSH or mosh?

`devmachine ssh` and `devmachine mosh` both open a terminal on your
workspace. They pick the same tmux session, so it makes no difference which
one you used last time. This page is about which to use when, and the one
setting mosh needs before it works.

**You need:** a workspace — see [getting started](../getting-started.md).

## Before you start: machine, skills, workspace

```
brew install mydevmachine/tap/devmachine
devmachine setup
devmachine skills add
devmachine workspaces new alice
devmachine sync
```

Already have a machine? Skip `setup`. Already have the workspace? Skip the
last two. See [getting started](../getting-started.md) for what each command
does.

## What each one is good for

`ssh` works everywhere, on any network, with no setup beyond what `setup`
already did. It's the default, and it's enough most of the time.

`mosh` is worth it when your connection is not steady: it survives a change
of Wi-Fi, your laptop going to sleep, roaming between networks, and a link
that drops packets. It also shows what you type right away, even on a slow
connection, instead of waiting for the server to echo it back.

Neither replaces tmux. `mosh` can't scroll back through old output — tmux
already gives you that inside the session both commands land in.

## What has to be installed where

- **On the server:** the `base` package, part of `essentials`, installs
  `mosh`. A machine set up with `devmachine setup` already has it. A machine
  started with `--no-essentials` does not, until you add `base`.
- **On your computer:** `devmachine machine setup` installs `ssh` and `mosh`
  through Homebrew on a Mac (on Linux it tells you what to install
  instead). `devmachine machine doctor` says what's missing — a missing
  `mosh` is only a warning, since `ssh` still works.
- devmachine does not bundle either program. `ssh` and `mosh` run the real
  system binaries, so your terminal, your SSH agent, and tmux all behave
  normally.

```
devmachine machine doctor
devmachine machine setup
```

## Open the UDP range for mosh

Unlike `ssh`, mosh needs a UDP port range (60000–61000 by default), not just
one TCP port. With the `firewall` package — part of `essentials` — that
range is closed by default, on every interface. `mosh` will fail until you
open it on one interface with `firewall.mosh_interface`.

The recommended way is over [Tailscale](../concepts/reaching-your-server.md):
that keeps the range off the public internet, reachable only from your own
devices.

```
devmachine packages add tailscale
devmachine login tailscale
devmachine sync
```

Then, in `config.yml`, set the interface under the machine's `settings:`:

```yaml
machines:
  - name: main
    settings:
      firewall.mosh_interface: tailscale0
```

```
devmachine sync
```

## By hand

### 1. Check your computer has both

```
devmachine machine doctor
```

If `mosh` is missing, install it:

```
devmachine machine setup
```

### 2. Connect with ssh

```
devmachine ssh alice
```

Always works, and needs nothing extra.

### 3. Connect with mosh

```
devmachine mosh alice
```

Needs `firewall.mosh_interface` set on the machine, as shown above. Both
commands check `<config>/known_hosts` first, the same as any other login.

## With your agent

Open a session on your own computer (`devmachine skills add` taught it the
CLI) and say:

```text
Open the mosh UDP range on my devmachine machine, over Tailscale only, then
sync.
```

The agent adds `firewall.mosh_interface: tailscale0` to `config.yml` and
runs `devmachine sync --check` first, then asks before applying it. Signing
the server into Tailscale (`devmachine login tailscale`) is a browser step
that's yours to do.

**Check it:** run `devmachine ssh alice` and `devmachine mosh alice` from
two different terminals — both land in the same tmux session, so anything
you see in one shows up in the other.

Source: [docs/reference/commands.md — ssh, mosh](../reference/commands.md),
[docs/how-it-works/ssh-and-authentication.md](../how-it-works/ssh-and-authentication.md),
[docs/concepts/reaching-your-server.md](../concepts/reaching-your-server.md)
