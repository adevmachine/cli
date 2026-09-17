# devmachine

A CLI that sets up and operates a personal development VPS.

You buy a server, point the CLI at it, and it does the rest: the first SSH
handshake, hardening, users, Docker, a reverse proxy with automatic TLS,
subdomains and DNS. Your own configuration lives in a directory you control;
everything generic lives in here.

## Status

Very early. The repository holds the skeleton only — `version` and `help`. The
commands below are the plan, not a promise.

## Install

Not published yet. To build from source:

```
git clone https://github.com/adevmachine/cli.git
cd cli
make build
./devmachine help
```

## Planned commands

```
devmachine setup                      wizard: address, domain, key, bootstrap
devmachine doctor                     config, SSH, VPS and DNS checks
devmachine config path|show|set       the effective configuration
devmachine secrets set|list|rm        provider tokens in the OS keychain

devmachine sync [--check] [--tags x]  converge the machine
devmachine ssh | mosh | run           reach the machine
devmachine stats                      memory, CPU, disk, sessions

devmachine users list|new|edit|rm     context users, with presets
devmachine dns status|list|add|rm     DNS through the configured provider
devmachine site list|add|rm           expose a port as a subdomain
```

Rules that hold for every command: JSON output is the stable contract, stdout is
data and stderr is diagnostics, anything that writes supports `--check` and
`--yes`, and `--help` exists everywhere.

## Design

- **Ansible is the convergence engine, the CLI is in charge.** The playbooks are
  embedded in the binary, copied to the machine and run there, so nothing has to
  be installed locally beyond this binary and `ssh`.
- **Your configuration is yours.** It lives in a directory (by default
  `~/.config/devmachine`, overridable) and is never part of this repository.
- **No vendor in the core.** DNS sits behind an interface with several
  providers; a manual provider always works.
- **Nothing runs on the machine.** No agent, no daemon. The CLI is a client.

## Development

```
make build     # compile
make test      # tests with -race
make fmt       # gofmt
make lint      # golangci-lint
```

## Licence

To be decided.
