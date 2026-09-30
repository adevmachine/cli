# The network package contract

A network package makes a private network reachable through devmachine
without the CLI knowing the product. It declares a
[`network:` block](package-format.md#network) and ships up to three
scripts. This page says what each one gets and what it must give back.

Every script is Python 3 with no dependencies beyond the standard library,
and must run on Python 3.9: that is what macOS ships, and `resolve` runs on
your computer. Every script gets the machine's settings for its package as
one JSON object in the `DEVMACHINE_SETTINGS` variable: each variable the
package declares, with its default, and any `<package>.<name>` setting of
the machine on top.

```json
{"exit_node": false, "login_server": "https://net.example.com"}
```

## `resolve`

Runs **on your computer**, every time something needs the machine's
addresses: a connection, `devmachine resolve`, an SSH alias.

- **Argument:** the name, as written after the prefix. `tailscale:main`
  gives `main`. It is an argument, never part of a shell line.
- **Output:** the machine's IP addresses on standard output, one per
  line, in the order to try them. Anything that is not an IP address is
  refused.
- **Exit 0:** the addresses are good.
- **Exit 3:** the network is not reachable from here — the app is not
  installed, not running, or not signed in. Print why on standard error
  in a few words (`tailscale is not running`). The entry is skipped, not
  reported as a failure.
- **Any other exit:** something is wrong with the script or the network.
  The entry is skipped too, and standard error is shown as the reason.
- **Time:** 5 seconds, then it is stopped and skipped.

It runs from the package's own folder on your computer: the pinned
release a sync downloaded, or `<config>/packages/<name>/`.

## `join`

Runs **on the machine**, as its admin account, for
`devmachine login <package>`, in a real terminal (`ssh -t`). It does
whatever signing in to the network takes: printing a URL to open, reading
an auth key, calling the network's own command. Exit 0 when the machine
joined.

It runs from where `sync` put the package:
`/opt/devmachine/roles/<package>/` for a published package, or
`/opt/devmachine/roles.local/<package>/` for one from your own
`packages/`. So the package has to be synced before `login`.

## `self_name`

Runs **on the machine**, as its admin, right after `join` succeeds. It
prints the name the machine answers to on the network, on one line: letters,
digits, dots, dashes and underscores. devmachine adds `<prefix>:<name>` as
the first entry of the machine's `hosts` in `config.yml`, unless it is
already there. The public address stays as a fallback.

If it fails, or prints something that cannot be a host entry, the login
still counts, and devmachine prints the `hosts:` block to paste by hand.

## An example

The `tailscale` package in the packages repository is the reference:
`resolve` reads `tailscale status --json` on your computer, `join` runs
`tailscale up` with `--login-server` when `tailscale.login_server` is set,
and `self_name` reads the machine's own `tailscale status --json`.
