# Configuration

Your configuration is a folder of files that lists your machines, your
workspaces, and what's installed where. Look at it with:

```
devmachine config show
```

## Where it is

First match wins:

| Order | Rule |
| --- | --- |
| 1 | `--config <path>` |
| 2 | `DEVMACHINE_CONFIG` |
| 3 | `$XDG_CONFIG_HOME/devmachine` |
| 4 | `~/.config/devmachine` |

```
$ devmachine config path
/Users/alice/.config/devmachine (from default)
```

Run a second, test setup next to a real one with one variable:

```
DEVMACHINE_CONFIG=~/.config/devmachine-test devmachine doctor
```

## What it holds

```
<config>/config.yml     machines, workspaces, domain, DNS provider
<config>/secrets.json   names of stored secrets, and values the keychain refused
<config>/keys/          keys the CLI generated, when it generated any
<config>/history.log    one line per command that reached a machine
```

## config.yml

```yaml
machines:
  - name: main
    hosts:
      - tailscale:vps      # tried first
      - 203.0.113.10       # the fallback
    user: root             # the account it logs in as
    port: 22
    key: /keys/main        # optional; without it the SSH agent serves
    packages: [base, docker, caddy, firewall, fail2ban, ssh_hardening, git]
    settings:
      base.timezone: Europe/Lisbon
      caddy.email: someone@example.com

workspaces:
  - name: alice
    machine: main
    packages: [workspace, dev, zsh, mise]
  - name: bob
    machine: sandbox
    user: bob-dev          # optional; the name is used by default
    packages: [workspace, dev]
    credentials:
      gh: own              # this one signs in to its own account

# What a new workspace gets when no flag says otherwise. `setup` sets this.
defaults:
  workspace: [workspace, dev, zsh, mise]

# Whether a login is shared across the machine. A workspace may override it.
credentials:
  gh: machine

packages: v0.0.1           # the pinned release the packages come from
domain: example.com
```

`user` defaults to `root`, `port` to `22`. Nothing here is a secret — a
token goes in `devmachine secrets`, never in this file.

## Settings

A package reads its own settings, each with a default. `settings:`
overrides one, on a machine (as shown above) or on a workspace, written
`<package>.<name>`:

```yaml
workspaces:
  - name: alice
    packages: [workspace, dev, zsh, claude-plugins]
    settings:
      claude-plugins.marketplace: example.com/their-plugins
      claude-plugins.plugins: [their-plugin]
```

Only the first dot is the split, so `claude-plugins.marketplace.url` is
the package `claude-plugins` and the setting `marketplace.url`.

A setting is refused, not just ignored, when it has no `<package>.`
prefix, or names a package the target doesn't have — so a typo never
silently does nothing.

## Credentials

A package says how its login works, and whether a copy of it can be
shared across a machine. Whether you *want* it shared is your call, set
per workspace:

```yaml
credentials:
  gh: machine          # the default for this setup

workspaces:
  - name: alice
  - name: bob
    credentials:
      gh: own          # bob logs in for himself
```

`machine` means one login, copied into every workspace that wants it.
`own` means that workspace signs in for itself. See
[Credentials](credentials.md) for how each kind works, and
[Sharing a login](../how-it-works/sharing-a-login.md) for what the copy
does.

## What is validated

`config show`, and every command that touches a machine, refuse a
configuration that can't work, and say what to fix:

- no machine at all
- a machine with no name, no address, or a port outside 1–65535
- two machines, or two workspaces, with the same name
- a workspace on a machine that isn't configured
- a workspace with no machine when there are several
- a setting with no `<package>.` prefix, or for a package the target
  doesn't have
- a credential answer that is neither `machine` nor `own`

## The command log

Every command that reaches a machine appends a line to
`<config>/history.log`: when, which workspace or machine, whether it
worked, and the command. It's the answer to "what did that session do to
my machine" — a plain `ssh` session leaves no such trail here. The format
is in [commands](../reference/commands.md#the-command-log).

## Secrets

Tokens don't go in `config.yml`. They live in your operating system's
keychain:

```
devmachine secrets set cloudflare_token     # asks, without echoing
devmachine secrets list                     # names only, never a value
devmachine secrets rm cloudflare_token
```

With no keychain — a headless server, a locked-down container — the value
falls back to `secrets.json`, readable by nobody else.
