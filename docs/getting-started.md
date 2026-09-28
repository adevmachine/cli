# Getting started

devmachine turns a VPS into workspaces you develop in: one Linux account per
project or client, each with its own tools and logins. You need a Debian or
Ubuntu VPS you can reach as root over SSH, and a Mac or Linux computer.

Three commands:

```
brew install adevmachine/tap/devmachine
devmachine setup
devmachine workspaces new alice && devmachine sync
```

`devmachine skills add` teaches your coding agent devmachine, and it works
before `setup`.

Then work in it:

```
devmachine ssh alice
```

`setup` asks for the machine's address and shows its host key fingerprint
before trusting it — compare it with your provider's console. It installs a
key, checks the key works, and turns password login off. `workspaces new`
adds a Linux account to your configuration, and `sync` builds it on the
machine. On Linux without Homebrew, take the binary from the
[releases page](https://github.com/adevmachine/cli/releases).

A new workspace has git, the GitHub CLI, Node LTS through mise, bun and zsh.
The machine itself gets nothing beyond the SSH hardening until you add
packages to it.

Something failed? [Troubleshooting](troubleshooting.md) says what each error
really means.

## Set it up to develop

- **A coding agent:** `devmachine packages add claude-code --workspace alice`,
  then `devmachine sync`.
- **GitHub, signed in once for every workspace:** `devmachine login gh`, then
  `devmachine sync`. See [credentials](concepts/credentials.md).
- **Docker:** `devmachine packages add docker`, and let the workspace use it
  with `devmachine workspaces edit alice --set workspace.groups=[docker]`, then
  `devmachine sync`.
- **Anything else:** `devmachine packages list` shows what exists;
  [packages](concepts/packages.md) says how to add one or write your own.
- **Teach your coding agent this CLI:** `devmachine skills add`. See
  [agent skills](concepts/agent-skills.md).

## Extras

- **See an app you run in the workspace at a URL** — anything listening on a
  port, a dev server or a Docker container: add the reverse proxy once with
  `devmachine packages add caddy`, then
  `devmachine expose add alice 3000 --host app.example.com` and `devmachine sync`. To reach it only from your computer, without a URL,
  use `devmachine tunnel alice 3000`. See [publishing](concepts/publishing.md).
- **Keep your configuration in git:** `devmachine setup git`. See
  [versioning your configuration](how-it-works/versioning-your-configuration.md).

Real setups, step by step: [examples](examples/index.md).
