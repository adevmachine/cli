# Packages

A package adds something to your server or to a workspace: Claude Code,
Docker, your GitHub login.

Add one with `devmachine packages add <name>`, then run `devmachine sync`
to install it. `devmachine packages list` shows what exists. The
published packages live at `github.com/mydevmachine/packages`, and you can
write your own.

## Which packages exist

```
devmachine packages list
```

lists every package your configuration can use, with a short description
of what each one does. `devmachine packages help <name>` prints what a
package accepts, straight from the package itself.

## Server packages and workspace packages

A **server package** installs once and serves everyone on the machine —
Docker, the firewall, Caddy. A **workspace package** belongs to one person
— Claude Code, `zsh`, a dotfiles setup — and can go on as many workspaces
as you want.

```
devmachine packages add docker --machine main
devmachine packages add claude-code --workspace alice
```

Adding the wrong kind to the wrong target is refused, and says why.

## Changing a package's settings

A package reads its own settings, each with a default. Change one on a
workspace with:

```
devmachine workspaces edit alice --set caddy.email=you@example.com
```

An empty value, `--set caddy.email=`, removes the override. On a machine,
edit `settings:` directly in `config.yml` — see
[Configuration](configuration.md#settings).

## Writing your own

`<config>/packages/` holds packages you write yourself, in the same
format as the published ones, and a package there with the same name
replaces an official one.

A package is an Ansible role plus a `package.yml` file — see
[the package format](../reference/package-format.md) for what goes in
it, and
[why packages work this way](../how-it-works/why-nothing-is-embedded.md)
for the reasoning behind it.
