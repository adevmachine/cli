# Commands

Every command accepts:

| Flag | Meaning |
| --- | --- |
| `--config <dir>` | the configuration directory to use |
| `--format table\|json` | how to print; JSON is the stable contract |
| `--machine <name>` | which machine to act on, for commands that act on a server |
| `--help` | what this command does |

`devmachine help --json` prints the whole tree, including every flag, in one
document. That is what a script or an agent should read instead of parsing help
text.

## setup

```
devmachine setup [--force]
```

Asks for a machine name, an address, the administrative login, a port and a
domain, then writes `config.yml`.

It changes nothing on any machine. It refuses to overwrite a configuration that
already exists unless `--force` is given.

## doctor

```
devmachine doctor [--machine m]
```

Four checks, in order: the configuration, the connection, the operating system,
and whether Ansible is installed. Exits non-zero if any failed.

A check that could not run reports `skip` and why.

## config

```
devmachine config path      the directory in use, and the rule that chose it
devmachine config show      machines, workspaces and their effective values
```

`show` validates, so it is also the quickest way to find out what is wrong with
a configuration.

## machines

```
devmachine machines list    each machine, its addresses, port and workspaces
```

## stats

```
devmachine stats [--machine m]
```

Memory, swap, disk and load. The table rounds for a person; the JSON carries
raw byte counts.

## ssh, mosh

```
devmachine ssh [workspace]
devmachine mosh [workspace]
```

An interactive session. With a workspace, it lands in that environment on
whichever machine it lives. Without one, on the machine as its admin.

Both run the system binary, so your terminal, agent and tmux behave normally.
`mosh` survives a link that drops or roams, and needs mosh on both sides.

## run

```
devmachine run "<command>" [--workspace w]
```

Runs one command and prints its output. Without `--workspace` it runs as the
machine's admin. The exit code is the command's.

The output of a command that failed is printed before the error, because that
output is usually the explanation.

## dns

```
devmachine dns status [host]
```

Checks a name from outside: it resolves, its certificate is accepted, and it
answers a request. With no host it uses the configured `domain`.

A 4xx counts as serving — the server answered. A 5xx does not.

## secrets

```
devmachine secrets set <name> [value] [--stdin]
devmachine secrets list
devmachine secrets rm <name>
```

With no value, `set` asks without echoing, so the secret never reaches your
shell history. `list` prints names only.

## packages

```
devmachine packages list
devmachine packages add <name> [--machine m | --workspace w] [--check] [--yes]
devmachine packages rm  <name> [--machine m | --workspace w] [--check] [--yes]
devmachine packages new <name> [--scope machine|workspace] [--into <dir>]
devmachine packages validate <dir>
devmachine packages schema [--json]
```

`list` reads the recipes and the configuration together, so a package appears
once with every machine and workspace that asks for it. A name a target asks
for and nothing provides is listed as `missing`, rather than left out for
`sync` to find.

`add` and `rm` edit `config.yml` and touch no machine — `devmachine sync`
applies the change. Pass one of `--machine` or `--workspace`; with neither, and
one configured machine, that machine is the target. Adding a package a target
already has changes nothing and says so.

Comments in `config.yml` survive: only the package lists and the pin are
rewritten, everything else is left as you wrote it.

`new` writes a package that already passes `validate` and already installs
something. It refuses to write over one that is there.

`validate` reports every problem at once, each with the file and the line, and
exits 1 when it found any.

`schema` prints the `package.yml` format this binary reads. It is the answer
that cannot drift, because the validator is what enforces it. The whole format
is in [the package format](package-format.md).

## sync

```
devmachine sync [--machine m] [--check] [--yes] [--tags a,b]
```

Puts a machine into the state the configuration describes.

In order: it reads and judges the configuration, makes the pinned release
available (fetching and verifying it the first time), resolves what the machine
and each of its workspaces get and in what order, checks your own packages,
prints the plan, asks, then sends everything to the machine and runs Ansible
**there**, streaming the output as it arrives.

| Flag | Meaning |
| --- | --- |
| `--check` | a dry run: the machine reports what would change and changes nothing |
| `--yes` | apply without asking |
| `--tags a,b` | only the packages named, by name |

`--check` never asks, and never writes the lock: a dry run that recorded
itself as applied would make the lock claim something nobody did.

Only your own packages are checked before the run. A published one was checked
when it was released; one in `<config>/packages/` has never been checked by
anybody.

With `--format json` the document on stdout is the result, and the plan, the
prompt and the machine's own output go to stderr.

On success, `<config>/packages.lock` records what was applied to that machine
and its workspaces, at which release and checksum. Only the machine that was
synced is rewritten — syncing one machine says nothing about another.

## version, help

```
devmachine version
devmachine help [command] [--json]
```

## The command log

Every command that reaches a machine appends one line to
`<config>/history.log`, mode `0600`:

```
2026-09-18T12:00:00Z  workspace alice   ok      "docker ps"
2026-09-18T12:01:00Z  machine main      failed  "sync --tags caddy"
```

The time is UTC, the target is the workspace or the machine the command
resolved to, then whether it succeeded, then the command itself. The command is
quoted, so one carrying a newline stays on one line.

The CLI only writes it. Read it with `tail` and `grep`, rotate it with
`logrotate`, delete it whenever you like — see
[configuration](../concepts/configuration.md#the-command-log) for what it is
not.
