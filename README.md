# devmachine

A CLI that sets up and operates a personal development VPS.

You buy a server, point the CLI at it, and it does the rest: the first SSH
handshake, hardening, users, Docker, a reverse proxy with automatic TLS,
subdomains and DNS. Your own configuration lives in a directory you control;
everything generic lives in here.

## Status

Early. Everything here reads; nothing writes to a machine yet. Provisioning
(`sync`), workspaces and DNS records come next.

## Install

```
brew install adevmachine/tap/devmachine
devmachine --help
```

The tap installs `advm` as a short alias of the same binary.

From source:

```
git clone https://github.com/adevmachine/cli.git
cd cli && make build && ./devmachine help
```

## Documentation

The manual is in [`docs/`](docs/index.md): getting started, the concepts, how
the awkward parts really work, a command reference and troubleshooting.

Working on the CLI itself: [development](docs/development.md) and
[releasing](docs/releasing.md).

## The model

Two ideas carry everything.

A **machine** is a server the CLI can reach. There can be several.

A **workspace** is an environment: normally one Linux user on one machine. It is
what you work in and what you name in a command. Where it runs is a property of
the workspace, so you never type an address.

```yaml
machines:
  - name: main
    hosts:
      - tailscale:vps        # tried first
      - 203.0.113.10         # the fallback
  - name: sandbox
    hosts: [198.51.100.7]
    port: 2222
    key: /keys/sandbox

workspaces:
  - name: alice
    machine: main
  - name: bob
    machine: sandbox         # runs somewhere else entirely

domain: example.com
```

Then `devmachine ssh bob` lands on the sandbox and `devmachine ssh alice` on the
main server, and neither command mentions a host.

A workspace's Linux account is its own name, unless `user:` says otherwise.
`machine:` may be left out when there is only one machine.

## What works today

```
devmachine setup                     write the configuration by answering a few questions
devmachine doctor [--machine m]      is the config usable, is the machine reachable and ready
devmachine config path|show          which configuration is in effect, and what it says
devmachine machines list             the machines and the workspaces on each
devmachine stats [--machine m]       memory, swap, disk, load
devmachine ssh|mosh [workspace]      an interactive session
devmachine run --workspace w "cmd"   one command, as that workspace
devmachine dns status [host]         DNS, TLS and one request, checked from outside
devmachine secrets set|list|rm       provider tokens in the OS keychain
devmachine help --json               the whole command surface, for a script or an agent
```

Rules that hold everywhere: `--format json` is the stable contract, stdout is
data and stderr is diagnostics, and `--help` exists on every command. A command
that would act on a server nobody named asks instead of guessing.

## Coming next

`sync` (provisioning through embedded Ansible), `workspaces new|rm`, DNS
providers and subdomains, the first-contact bootstrap for a brand new server,
and creating a machine locally for people who have no server at all.

## Design

- **Ansible converges; the CLI is in charge.** Ansible already handles several
  machines through its inventory. What it has no idea of is a workspace, so the
  CLI owns that model and generates the inventory from it. The playbooks will be
  embedded in the binary and run on the machine, so nothing has to be installed
  locally beyond this binary and `ssh`.
- **Your configuration is yours.** It lives in a directory you choose and is
  never part of this repository.
- **No vendor in the core.** DNS sits behind an interface; a manual provider
  always works.
- **Nothing runs on the machine.** No agent, no daemon. The CLI is a client.

## Development

```
make build     # compile
make test      # tests with -race
make fmt       # gofmt
make lint      # golangci-lint
```

## Licence

MIT. See [LICENSE](LICENSE).
