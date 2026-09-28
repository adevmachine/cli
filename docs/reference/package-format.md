# The package format

A package is an Ansible role plus one extra file, `package.yml`. Nothing
is translated on the way to the machine: what you write is what runs, so
a failure points at the exact line you wrote.

This page and the validator agree. The page exists because people read
before they write; the validator is what enforces the rules. When they
disagree, trust the validator — ask it with `devmachine packages schema
--json`.

## The layout

```
<name>/
  package.yml
  tasks/main.yml
  defaults/main.yml
  handlers/, files/, templates/, vars/   (optional, as in any role)
```

The directory name **is** the package name. `devmachine packages new
<name>` writes a starting skeleton that already passes `devmachine
packages validate`.

## The fields

### `format` (required)

The shape of the file. This CLI reads format `1`.

It comes first because everything else depends on it. Without it, a
change to the format would break every existing package in a different
way, with no message explaining why. A validator that meets a format it
cannot read says so and stops, instead of misreading fields it does not
understand.

### `name` (required)

Lower case letters, digits, dashes and underscores. It must match the
directory name, because a package is found by its directory.

### `scope` (required)

`machine` or `workspace`. Nothing else is valid.

Scope is a fact about the software, not a preference. Docker installs
once and serves everyone, so it is `machine`. A tool with a login per
person is `workspace`, and it runs once for each workspace that asks for
it.

### `summary` (required)

One line saying what the package installs. `packages list` prints it.

### `requires.cli`

Which version of the CLI can run this package: `">= 0.2.0"`, `"> 0.2.0"`
or `"= 0.2.0"`.

This is different from `format`. `format` says whether the CLI can *read*
the file. `requires.cli` says whether it can *run* what the file
describes. The CLI binary and the packages are released on their own
schedules, so the two can disagree.

A CLI built from source calls itself `dev`, and every constraint allows
it.

### `needs`

Packages that must run before this one:

```yaml
needs: [base, firewall]
```

This is the only thing that decides run order. The order packages happen
to be listed in your own configuration means nothing.

A circular dependency is refused, and the error names the packages
involved.

### `provides`

Places other packages may write into, as a name and an absolute path on
the machine:

```yaml
provides:
  sites.d: /etc/caddy/sites.d
```

### `extends`

Adds a file to a place another package opened:

```yaml
extends:
  caddy.sites.d: files/sharing.caddy
```

The key is `<package>.<place>`, and the value is a path inside this
package. It can only add a file there, not change what is already there,
and it cannot reach anywhere else. Extending a place nobody provides is
refused while devmachine works out the plan, before anything runs.

The file lands as `<extending package>-<basename>`, so two packages
adding a file with the same name never collide.

### `variables`

Values the package reads, each with a summary and a default:

```yaml
variables:
  port:
    summary: The port the container listens on.
    default: 53842
```

The package's Ansible role reads `devmachine_<package>_<name>`, so a
package called `tunnel` that declares `port` uses
`devmachine_tunnel_port`. The package name is part of the variable
because Ansible has one shared namespace, and two packages might both
want a `port`.

That is the same name used for a target's
[settings](../concepts/configuration.md#settings): a setting is just a
default someone overrode. The package does not know or care where the
value came from.

A dash works in a package name but never in a variable name, so `-`
becomes `_`, and so does the `.` a package name may contain. Two names
that collide this way are refused, rather than letting one silently win.

### `credentials`

What the package's tool needs to log in or authenticate, **and how to
get it**. How belongs in the package, because the package is the only
thing that knows.

Each entry has a `name`, a `kind`, and a `scope` (`machine` or
`workspace`), plus what its kind needs:

| `kind` | also needs | what it means |
| --- | --- | --- |
| `manual` | `command`, `stored_at` | A person runs `command`; the tool leaves its session at `stored_at`. |
| `secret` | `env` or `path` | A value handed over once, delivered there. |
| `file` | `path` | A file placed on the machine at that path. |

```yaml
credentials:
  - name: claude
    kind: manual
    scope: workspace
    command: claude /login
    stored_at: ~/.claude/.credentials.json
```

`stored_at` is a claim, not a guarantee. It is what lets `doctor` check
whether the login worked.

A `manual` credential can also say `shareable: true`: a copy of
`stored_at` works on another account, the way one GitHub login can serve
every workspace. This is a fact about the tool, found by testing it: a
session file copies fine, a token tied to one device or browser does not.
Leave it out and it defaults to `false`, so the CLI never copies it
anywhere.

A `manual` credential that recommends `scope: machine` must also say
`shareable: true`, because `scope: machine` means exactly "one login,
copied into every workspace." Recommending both without `shareable` would
ask for something the package itself says cannot work.

Only a `manual` credential can be `shareable`. A `secret` or a `file` is
delivered fresh to each place that needs it, never copied from one place
to another, so setting `shareable` on either is refused.

`scope` here is only a recommendation. Whether a shareable credential is
actually shared is the operator's own choice, made per workspace in their
configuration — see [Configuration](../concepts/configuration.md).

### `requires_files`

Files that must already be on the machine before the package runs.

### `skills.path`

A package can ship complete Agent Skill directories:

```yaml
skills:
  path: skills
```

The path is relative to the package root, and cannot contain `..`, be
absolute, or escape through a symlink. Each direct child must be one
lower-case, dash-separated skill directory with a `SKILL.md`, whose
frontmatter `name` matches the directory name and whose `description` is
not empty. Scripts, references and assets under a valid skill directory
are included as part of that skill.

This does not replace the Ansible role. A package with skills still has
`tasks/main.yml`, and may also have defaults, handlers, files and
templates.

### `kind`, `entrypoint`, `commands`

A package can ship an executable the CLI calls on the machine:

```yaml
kind: dns
entrypoint: bin/provider
commands: [zones, list, upsert, delete, help]
```

- `entrypoint` is a path inside the package. It must exist, be
  executable, and start with `#!/usr/bin/env python3` — Ansible already
  needs Python on any machine this CLI sets up, so an entrypoint with no
  extra dependencies always works.
- `commands` lists what it accepts: a list of names, or `["*"]` for
  anything. Mixing `"*"` with named commands is refused, since it says
  two contradictory things.
- `kind` is a contract. The only one so far is `dns`, which must accept
  `zones`, `list`, `upsert`, `delete` and `help`.

Nothing calls an entrypoint in this version yet. It is validated now so
the first real use cannot invent its own shape later.

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
| a `machine` login is not `shareable` | ``credential "X" recommends `scope: machine`, so it needs `shareable: true` `` |
| a `secret` or a `file` is `shareable` | ``credential "X" is a secret, so it cannot be `shareable` `` |
| an entrypoint is not executable | `entrypoint "X" is not executable: chmod +x it` |
| an entrypoint is not Python 3 | `an entrypoint is Python 3 and starts with #!/usr/bin/env python3` |
| `kind` or `commands` with no entrypoint | ``kind` and `commands` describe an `entrypoint`, and this package declares none`` |

`devmachine packages validate` reports every problem at once, not just
the first. Fixing one at a time would take several round trips for what
should be one.

## Why `apt` is refused

A package that calls `apt` only works on Debian. `package:` picks the
machine's own package manager instead, so the same package keeps working
even when the machine underneath it changes. This turns a rule people
have to remember into an error the validator catches.
