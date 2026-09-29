# Upgrade

devmachine has three things that can be out of date: the CLI on your
computer, the packages your server uses, and the skills that teach your
coding agent the CLI. Each is upgraded separately, and none of it touches a
server until you run `sync`.

## The CLI

```
brew update && brew upgrade mydevmachine/tap/devmachine
devmachine version
```

On Linux, get the newest binary from the
[releases page](https://github.com/mydevmachine/devmachine/releases).

## The packages

Packages are versioned separately from the CLI, as a release of their own.
Move to the newest one:

```
devmachine packages pin
```

With no argument, this pins the latest published release — see the
[releases page](https://github.com/mydevmachine/packages/releases) for
what changed. Pinning only edits `config.yml`; nothing is installed yet.

Check what would change, per machine:

```
devmachine sync --check
```

then apply it:

```
devmachine sync
```

With more than one machine, add `--machine <name>` to check or apply one at
a time, or run `sync` without it if you want every machine your
configuration knows about. The lock file records exactly which package
release each machine is running, so `sync --check` always compares against
what is really there, not against what you last typed.

## The skills

```
devmachine skills update
```

refreshes every skill your coding agents use from its current package
source. Skills only affect new sessions — one already running keeps using
what it loaded at the start.

## Adding the essentials to an older machine

A machine set up before the `essentials` package existed has none of it:
`base`, `git`, `firewall`, `ssh_hardening`, `caddy`. Add it without
rebuilding anything — what is already installed stays as it is:

```
devmachine packages pin
devmachine packages add essentials
devmachine sync
```

## If you like, check the server's identity first

```
devmachine machines trust <name> --check
```

compares the SSH host key devmachine expects against the one the server
presents, without writing anything. Worth running before a `sync` you are
not fully expecting, on a server you have not touched in a while.

## Nothing changes on a server until you run sync

`packages pin`, `packages add`, and `workspaces edit` all edit
`config.yml` only. The server keeps running exactly what it was running
until `devmachine sync` applies the change — so pinning a new package
release today and running `sync` next week is fine; nothing happens in
between.

Source: [devmachine releases](https://github.com/mydevmachine/devmachine/releases),
[packages releases](https://github.com/mydevmachine/packages/releases)
