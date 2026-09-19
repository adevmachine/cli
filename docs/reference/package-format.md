# The package format

A package is an Ansible role with one extra file beside it, `package.yml`.
Nothing is translated on the way to the machine: what is written is what runs,
so a failure points at the line somebody wrote.

This page and the validator say the same thing. The page exists because people
read before they write; the validator is what enforces it. When they disagree,
the validator is right — ask it with `devmachine packages schema --json`.

## The layout

```
<name>/
  package.yml
  tasks/main.yml
  defaults/main.yml
  handlers/, files/, templates/, vars/   (optional, as in any role)
```

The directory name **is** the package name. `devmachine packages new <name>`
writes a skeleton that already passes `devmachine packages validate`.

## The fields

### `format` (required)

The shape of the file. This CLI reads format `1`.

It comes first because everything else depends on it. Without it, the first
change to the format would make every existing recipe fail in a different way,
none of them saying why. A validator that meets a format it cannot read reports
that and stops: the other messages all assume the fields mean what this CLI
thinks they mean.

### `name` (required)

Lower case letters, digits, dashes and underscores. It has to match the
directory, because a package is found by its directory.

### `scope` (required)

`machine` or `workspace`. There is no third scope.

Scope is a property of the software, not a preference. Docker is installed once
and serves everyone, so it is `machine`. A tool with a login per person is
`workspace`, and it runs once per workspace that asked for it.

### `summary` (required)

One line saying what the package installs. It is what `packages list` prints.

### `requires.cli`

Which CLI can run this recipe, written as `">= 0.2.0"`, `"> 0.2.0"` or
`"= 0.2.0"`.

This answers a different question from `format`. A format says whether this CLI
can *read* the file; `requires.cli` says whether it can *run* what the file
describes. The binary and the recipes are released on their own schedules,
which is the point, and it also means they can disagree.

A CLI built from source calls itself `dev`, and every constraint allows it.

### `needs`

Packages that have to run before this one:

```yaml
needs: [base, firewall]
```

It is the only thing that decides order. The order of a machine's own list
means nothing — two configurations listing the same packages differently
produce the same run.

A circle is refused, and the message names it.

### `provides`

Places other packages may write into, as a name and an absolute path on the
machine:

```yaml
provides:
  sites.d: /etc/caddy/sites.d
```

### `extends`

A contribution to a place another package opened:

```yaml
extends:
  caddy.sites.d: files/sharing.caddy
```

The key is `<package>.<place>`, and the value is a path inside this package.
It is not a patch, and it cannot reach anywhere else. Extending a place nobody
provides is refused when the machine is resolved, not when it is applied.

The file lands as `<extending package>-<basename>`, so two packages
contributing the same file name cannot collide.

### `variables`

Values the package reads, each with a summary and a default:

```yaml
variables:
  port:
    summary: The port the container listens on.
    default: 53842
```

The recipe reads `devmachine_<package>_<name>`, so a package called `tunnel`
declaring `port` puts `devmachine_tunnel_port` in its own `defaults/main.yml`.
The package name is in the variable because Ansible has one namespace for all
of them, and two packages are free to both want a `port`.

That is the same name a target's
[settings](../concepts/configuration.md#settings) are written under, and it is
the whole mechanism: a setting is a default somebody overrode. The recipe
cannot tell where the value came from, and does not have to.

A dash is fine in a package name and never fine in a variable, so `-` becomes
`_` on the way in, as does the `.` a package may use inside a name of its own.
Two names that collide that way are refused rather than one of them winning.

### `credentials`

What the package's tool cannot work without, **and how each one is obtained**.
How belongs in the package because the package is the only thing that knows it.

Each entry has a `name`, a `kind` and a `scope` (`machine` or `workspace`),
then what its kind needs:

| `kind` | also needs | what it means |
| --- | --- | --- |
| `login` | `command`, `stored_at` | A person runs `command`; the tool leaves its session at `stored_at`. |
| `secret` | `env` or `path` | A value handed over once, delivered there. |
| `file` | `path` | A file dropped on the machine at that path. |

```yaml
credentials:
  - name: claude
    kind: login
    scope: workspace
    command: claude /login
    stored_at: ~/.claude/.credentials.json
```

`stored_at` is a claim, not a guarantee. It is what lets `doctor` look and say
whether the login worked.

### `requires_files`

Files that have to be on the machine before the package runs.

### `kind`, `entrypoint`, `commands`

A package may carry an executable the CLI calls on the machine:

```yaml
kind: dns
entrypoint: bin/provider
commands: [zones, list, upsert, delete, help]
```

- `entrypoint` is a path inside the package. It has to exist, be executable,
  and start with `#!/usr/bin/env python3` — Ansible already requires Python on
  any machine this CLI provisions, so an entrypoint with no dependencies cannot
  break on install.
- `commands` is what it accepts: a list, or `["*"]` for anything. `"*"` beside
  other commands says two things at once and is refused.
- `kind` is a contract. The only one so far is `dns`, and a `dns` package has
  to accept `zones`, `list`, `upsert`, `delete` and `help`.

Nothing calls an entrypoint in this version. It is validated now so the first
one cannot invent its own shape.

## The rules, and what each one says

| Rule | The message |
| --- | --- |
| `format` missing | ``every package needs `format`, and this CLI reads 1`` |
| `format` unreadable | `format 99, and this CLI reads 1. Upgrade with brew upgrade devmachine` |
| `name` missing | ``every package needs a `name` `` |
| `name` malformed | `name "X": use lower case letters, digits, dashes and underscores` |
| `name` is not the directory | `name is "X" but the directory is "Y": a package is found by its directory` |
| `scope` unknown | `scope must be "machine" or "workspace", got "X"` |
| `summary` missing | ``every package needs a one-line `summary` `` |
| `requires.cli` unreadable | `requires.cli "X": write it as ">= 0.2.0", "> 0.2.0" or "= 0.2.0"` |
| `extends` key has no dot | `an extension point is written <package>.<place>` |
| `extends` file is not there | `extends "X" points at Y, which is not in the package` |
| `provides` path is relative | `an extension point is an absolute path on the machine` |
| no `tasks/main.yml` | `a package is an Ansible role, so it needs tasks/main.yml` |
| the `apt` module is used | ``the apt module is not allowed; use `package` so this works beyond Debian`` |
| a credential says too little | one line per credential, naming it and what it is missing |
| an entrypoint is not executable | `entrypoint "X" is not executable: chmod +x it` |
| an entrypoint is not Python 3 | `an entrypoint is Python 3 and starts with #!/usr/bin/env python3` |
| `kind` or `commands` with no entrypoint | ``kind` and `commands` describe an `entrypoint`, and this package declares none`` |

`devmachine packages validate` reports every problem at once, not the first.
Correcting one at a time is four round trips for one answer's worth of work.

## Why `apt` is refused

A package that calls `apt` works on Debian and nowhere else. `package:` picks
the machine's own package manager, so the same recipe keeps working when the
machine is not what it was. The rule stops being a convention somebody has to
remember and becomes a validation error.
