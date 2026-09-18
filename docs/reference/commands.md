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

## version, help

```
devmachine version
devmachine help [command] [--json]
```
