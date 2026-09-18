# Getting started

## Install

```
brew install adevmachine/tap/devmachine
```

The tap also installs `advm`, a short alias for the same binary. The full name
is what appears in documentation and error messages; the short one is for
typing.

From source:

```
git clone https://github.com/adevmachine/cli.git
cd cli && make build && ./devmachine help
```

## Point it at a machine

```
devmachine setup
```

It asks for a name, an address, the administrative login and a port, then writes
`config.yml`. It does not touch the machine: no key is installed, nothing is
configured, nothing is hardened. Setting a machine up from scratch comes in a
later version.

## Check that it works

```
devmachine doctor
```

```
pass  configuration       machine "main", 1 address(es), admin root, port 22, 0 workspace(s)
pass  connection          connected through 203.0.113.10
pass  operating system    ubuntu
fail  ansible             ansible-playbook is not on the machine; `devmachine setup` installs it
```

Every check that could not run because an earlier one failed is reported as
`skip`, with the reason. A wall of failures would bury the one that matters.

`doctor` exits non-zero when anything failed, so a script or an agent can act on
it.

## Look around

```
devmachine stats            what the machine is spending
devmachine machines list    the machines and the workspaces on each
devmachine ssh              a session on the machine
devmachine run "uptime"     one command
```

## Machine-readable output

Every command takes `--format json`, and that output is the stable contract:

```
devmachine --format json doctor
devmachine help --json        # the whole command surface
```

stdout carries data and stderr carries diagnostics, so a pipe never mixes the
two.
