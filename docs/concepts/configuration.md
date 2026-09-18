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

workspaces:
  - name: alice
    machine: main
  - name: bob
    machine: sandbox
    user: bob-dev          # optional; the name is used by default

domain: example.com
dns_provider: manual
```

Defaults: `user` is `root`, `port` is `22`. Everything else is what you wrote.

## What is validated

`config show` and every command that touches a machine refuse a configuration
that cannot work, and say what to change:

- no machine at all
- a machine with no name, no address, or a port outside 1–65535
- two machines, or two workspaces, with the same name
- a workspace on a machine that is not configured
- a workspace with no machine when there are several

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
