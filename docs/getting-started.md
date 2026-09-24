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

## Get a machine

Either buy a server — any provider, any distribution the CLI supports — or make
one on this computer:

```
devmachine machines create-local dev
```

That needs [Lima](https://lima-vm.io) (`brew install lima`), which rules out
Windows, and the machine it makes is not reachable from the internet, so DNS,
TLS and public hostnames do not work on it. Everything else does.

It leaves the machine **exactly as a bought server arrives**: root reachable
over SSH with a password and no key. That is deliberate — it is the same
starting point `setup` is built for, so the path you take is the path the tests
take.

## Take it over

```
devmachine setup
```

This is the only step that needs anything by hand, and it needs it once.

It asks where the machine is, who the administrative account is, and which key
to use — a new one, a file you already have, or one your SSH agent is holding.
Then it works out the rest:

- If the key already works, it says so and moves on.
- If it does not, it asks for the root password, installs the key, and **proves
  it on a fresh connection** before trusting it.
- Only once the key is proved does it turn password authentication off.
- Finally it installs Ansible, which is the last thing done by hand.

If the proof fails it stops and leaves password login on, so you can still get
in. [The trust bootstrap](how-it-works/trust-bootstrap.md) explains why each of
those steps is where it is.

The password is used once and never stored.

If the configuration already exists, `devmachine setup` resumes this step: it
uses the configured host-key pin and authentication and ensures Ansible is
installed without rewriting the configuration or changing SSH policy.

## Build the machine

```
devmachine doctor      # configuration, host key, connection, OS, Ansible
devmachine sync        # fetch the recipes, send them, converge
```

`sync` prints what it would do and asks before doing it. `--check` is a dry run
and `--yes` skips the question.

A second `sync` changes nothing. That is the point of it: it describes a state
rather than a series of steps.

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
