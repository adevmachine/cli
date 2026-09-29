# devmachine

devmachine sets up a VPS for you to code on. Each project gets its own
account on the server, called a workspace, with its own tools, logins and
coding agent.

You need a Debian or Ubuntu VPS you can reach as root over SSH, and a Mac or
Linux computer.

```
brew install mydevmachine/tap/devmachine
devmachine setup
devmachine skills add
devmachine workspaces new acme && devmachine sync
devmachine ssh acme
```

`setup` connects to the server and locks it down: it checks who you are
talking to, installs a key, turns off password logins, and gives the server
the essentials: base tools, git, a firewall and Caddy. `workspaces new`
adds a workspace to your configuration. `sync` builds it on the server.
`skills add` teaches your coding agent the CLI, so you can ask for any of
this from any Claude Code or Codex session.

**Documentation: https://mydevmachine.sh/** — getting started,
how it all works, every command, and fixes for common errors. The same pages
are in [`docs/`](docs/index.md), and an LLM can read them all from
[llms-full.txt](https://mydevmachine.sh/llms-full.txt).

## The model

- A **machine** is a server the CLI reaches over SSH, or your own computer
  (`self: true`).
- A **workspace** is one account on one machine. You give it a name and use
  that name in every command — you never type an address.
- A **package** adds one thing: a coding agent, Docker, a GitHub login, a
  reverse proxy. `sync` installs the packages your configuration asks for.
  Browse the ready-made ones at
  [mydevmachine/packages](https://github.com/mydevmachine/packages), or write
  your own.

Your configuration lives in a folder you control, `~/.config/devmachine`.
Nothing personal is ever part of this repository, and nothing runs on the
server between commands.

## From source

```
git clone https://github.com/mydevmachine/devmachine.git
cd cli && make build && ./devmachine help
```

Working on the CLI itself: [development](docs/development.md) and
[releasing](docs/releasing.md).

## Licence

MIT. See [LICENSE](LICENSE).
