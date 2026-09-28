# devmachine

devmachine turns a VPS into workspaces you develop in: one Linux account per
project, each with its own tools, logins and coding agent.

You need a Debian or Ubuntu VPS you can reach as root over SSH, and a Mac or
Linux computer.

```
brew install adevmachine/tap/devmachine
devmachine setup
devmachine workspaces new alice && devmachine sync
devmachine ssh alice
```

`setup` takes the machine over: it checks the host key with you, installs a
key, and turns password login off. `workspaces new` adds a workspace to your
configuration, and `sync` builds it on the machine.

**Documentation: https://adevmachine.github.io/docs/** — getting started,
concepts, the command reference and troubleshooting. The same pages are in
[`docs/`](docs/index.md), and an LLM can read them all from
[llms-full.txt](https://adevmachine.github.io/docs/llms-full.txt).

## The model

- A **machine** is a server the CLI reaches over SSH, or your computer itself
  (`self: true`).
- A **workspace** is one Linux account on one machine. You name it in a
  command and never type an address.
- A **package** is a recipe: the coding agent, Docker, a GitHub login, a
  reverse proxy. `sync` applies the packages your configuration asks for. The
  published ones live in [adevmachine/packages](https://github.com/adevmachine/packages),
  and you can write your own.

Your configuration lives in a directory you control, `~/.config/devmachine`;
nothing personal is ever part of this repository. Nothing runs on the machine
between commands: no agent, no daemon.

## From source

```
git clone https://github.com/adevmachine/cli.git
cd cli && make build && ./devmachine help
```

Working on the CLI itself: [development](docs/development.md) and
[releasing](docs/releasing.md).

## Licence

MIT. See [LICENSE](LICENSE).
