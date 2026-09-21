# Configuration

The configuration is yours, not the CLI's. It lives in a directory you choose,
and no copy of it is ever part of this repository.

## Where it is

Four rules, first hit wins:

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

It prints the rule as well as the path. "Which configuration am I actually
using" is the first question when something behaves unexpectedly, and answering
it should not require guessing.

Running a second setup alongside a real one is therefore a matter of one
variable:

```
DEVMACHINE_CONFIG=~/.config/devmachine-test devmachine doctor
```

The two can never cross.

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
    user: root             # the administrative login
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

# What a new workspace gets when no flag says otherwise. `setup` seeds it.
defaults:
  workspace: [workspace, dev, zsh, mise]

# Whether a login is shared across the machine. A workspace may override it.
credentials:
  gh: machine

packages: v0.0.1           # the pinned release the recipes come from
domain: example.com
```

Defaults: `user` is `root`, `port` is `22`. Everything else is what you wrote.

Nothing in this file is a secret. Values live in `devmachine secrets` and are
delivered to the machine; what is written here is which packages, which
settings, and which logins you want shared.

## Settings

A package declares the values it reads under `variables`, each with a default.
`settings:` is where a target overrides one. It sits on a machine or on a
workspace, and a key is written `<package>.<name>`:

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

Only the first dot is the split, so a package may use a dotted name of its own:
`claude-plugins.marketplace.url` is the package `claude-plugins` and the name
`marketplace.url`.

Two settings are refused rather than ignored:

- one with no `<package>.` prefix, because nothing could tell what reads it
- one for a package that target does not install

The second is the one that matters. A mistyped package name would otherwise
cost nothing and do nothing: the value reaches no recipe, the recipe keeps its
default, and the machine quietly is not what the configuration says it is. If
the package arrives through another package's `needs`, name it in `packages:`
as well — you are configuring it, so say you want it.

## Credentials

A package says how a credential is obtained, and whether a copy of it works on
another account (`shareable`). Whether you *want* it copied is your call, and
it is per workspace:

```yaml
credentials:
  gh: machine          # the default for this setup

workspaces:
  - name: alice
  - name: bob
    credentials:
      gh: own          # bob logs in for himself
```

`machine` means one login, stored under `/etc/devmachine/<name>/` and copied
into every workspace that wants it. `own` means that workspace logs in by
itself and nothing is ever copied over it.

Which answer applies, first hit wins:

| Order | Where |
| --- | --- |
| 1 | the workspace's `credentials:` |
| 2 | the configuration's `credentials:` |
| 3 | the package's own `scope:` |

A package that says `shareable: false` beats all three: asking for `machine` on
one is refused, naming the package, rather than copying something that would
not work.

What the copying actually does, and why the CLI generates it rather than any
package, is in [Sharing a login](../how-it-works/sharing-a-login.md).

This block holds no values and no method. The package still says how a login
happens and whether it travels; this says only what you want done about it.

## What is validated

`config show` and every command that touches a machine refuse a configuration
that cannot work, and say what to change:

- no machine at all
- a machine with no name, no address, or a port outside 1–65535
- two machines, or two workspaces, with the same name
- a workspace on a machine that is not configured
- a workspace with no machine when there are several
- a setting with no `<package>.` prefix, or for a package the target does not
  install
- a credential answer that is neither `machine` nor `own`

## The command log

Every command that reaches a machine appends a line to `<config>/history.log`:
when, which workspace or machine, whether it worked, and the command. It exists
so that "what did that session do to my machine" has an answer — over `ssh` it
does not, because the shell history stays on the machine, under whichever
account was used.

It is a record, and nothing more. It stops nothing, it checks nothing, and no
command reads it. Writing it can never fail a command: if the file cannot be
opened, the line is dropped and the command carries on. Calling a log a
safeguard is how a safeguard stops being built, so this one says plainly that
it is not one. The format is in
[commands](../reference/commands.md#the-command-log).

## Secrets

Tokens do not go in `config.yml`. They live in the operating system's keychain:

```
devmachine secrets set cloudflare_token     # asks, without echoing
devmachine secrets list                     # names only, never a value
devmachine secrets rm cloudflare_token
```

Where there is no keychain — a headless server, a locked-down container — the
value falls back to `secrets.json`, readable by nobody else. The fallback exists
so the CLI works everywhere, not because a file is a good place for a secret.

The name is always recorded, even when the keychain holds the value. A keychain
cannot be listed by service, so without that index `secrets list` would show
nothing and you would have no way to know what is stored.
