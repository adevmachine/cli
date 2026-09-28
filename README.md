# devmachine

devmachine sets up a VPS for you to code on. Each project gets its own
account on the server, called a workspace, with its own tools, logins and
coding agent.

You need a Debian or Ubuntu VPS you can reach as root over SSH, and a Mac or
Linux computer.

```
brew install mydevmachine/tap/devmachine
devmachine setup
devmachine workspaces new alice && devmachine sync
devmachine ssh alice
```

`setup` connects to the server and locks it down: it checks who you are
talking to, installs a key, and turns off password logins. `workspaces new`
adds a workspace to your configuration. `sync` builds it on the server.

**Documentation: https://mydevmachine.github.io/docs/** — getting started,
how it all works, every command, and fixes for common errors. The same pages
are in [`docs/`](docs/index.md), and an LLM can read them all from
[llms-full.txt](https://mydevmachine.github.io/docs/llms-full.txt).

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
