# Configuration

Your configuration is a folder of plain files that says what machines you
have, what runs on them, and who logs in. It's yours, not the CLI's, and
no copy of it ever lives in this repository.

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

It prints the rule too, so "which configuration am I using" never needs a
guess. Running a second, test setup next to a real one is one variable:

```
DEVMACHINE_CONFIG=~/.config/devmachine-test devmachine doctor
```

The two never mix.

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

Defaults: `user` is `root`, `port` is `22`. Everything else is what you
wrote. Nothing here is a secret — values live in `devmachine secrets` and
get delivered to the machine. This file only says which packages, which
settings, and which logins you want.

## Settings

A package declares the values it reads, each with a default. `settings:`
overrides one, on a machine or a workspace, written `<package>.<name>`:

```yaml
machines:
  - name: main
    packages: [base, caddy]
    settings:
      base.timezone: America/Sao_Paulo
      caddy.email: someone@example.com

workspaces:
  - name: alice
    packages: [workspace, dev, zsh, claude-plugins]
    settings:
      claude-plugins.marketplace: example.com/their-plugins
      claude-plugins.plugins: [their-plugin]
```

Only the first dot is the split, so `claude-plugins.marketplace.url` is
the package `claude-plugins` and the setting `marketplace.url`.

Two settings are refused, not just ignored: one with no `<package>.`
prefix, and one for a package the target doesn't have. That second case
matters most — a mistyped package name would otherwise do nothing, and the
machine would quietly not match what you wrote.

## Credentials

A package says how a login is obtained, and whether a copy of it works on
another account (`shareable`). Whether you *want* it copied is your call,
set per workspace:

```yaml
credentials:
  gh: machine          # the default for this setup

workspaces:
  - name: alice
  - name: bob
    credentials:
      gh: own          # bob logs in for himself
```

`machine` means one login, stored under `/etc/devmachine/<name>/` and
copied into every workspace that wants it. `own` means that workspace
signs in for itself, and nothing is ever copied over it.

First match wins:

| Order | Where |
| --- | --- |
| 1 | the workspace's `credentials:` |
| 2 | the configuration's `credentials:` |
| 3 | the package's own recommended setting |

A package marked "not shareable" overrides all three: asking for
`machine` on one is refused by name. See
[Sharing a login](../how-it-works/sharing-a-login.md) for what the copy
actually does.

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
worked, and the command. It answers "what did that session do to my
machine" — over plain `ssh` there's no such answer, since shell history
stays on the machine itself.

It's a record, nothing more. It stops nothing and checks nothing, and no
command reads it back. If the file can't be written, the line is just
dropped and the command carries on. The format is in
[commands](../reference/commands.md#the-command-log).

## Secrets

Tokens don't go in `config.yml`. They live in your operating system's
keychain:

```
devmachine secrets set cloudflare_token     # asks, without echoing
devmachine secrets list                     # names only, never a value
devmachine secrets rm cloudflare_token
```

With no keychain — a headless server, a locked-down container — the value
falls back to `secrets.json`, readable by nobody else. The name is always
recorded, even when the keychain holds the value, since a keychain can't
be listed by name and `secrets list` would otherwise show nothing.
