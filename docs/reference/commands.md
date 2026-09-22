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
devmachine setup [--force] [--no-harden]
```

Takes a machine over. It asks for a machine name, an address, the
administrative login, a port and a domain, then how the CLI should log in:

1. a key of its own, kept in `<config>/keys/<machine>` and used for nothing
   else — the recommendation, and the answer if you have no opinion;
2. a key file already on this computer;
3. a key your SSH agent holds, offered by fingerprint and comment.

The third is how a key kept in a password manager works: it never touches the
disk, so there is no file to point at. Picking it leaves `key` out of
`config.yml`, which is what tells the CLI to ask the agent every time.

It then writes `config.yml` and starts on the machine. In order:

1. it tries the key. **If that already works, no password is asked for** —
   many servers arrive with a key pasted in at the provider, and their owner
   has no root password to give;
2. only when the key is refused does it ask for the password, without echoing
   it when there is a terminal;
3. it installs the key over that one connection;
4. it **proves the key by opening a new connection with the key alone**;
5. it turns password login off;
6. it installs Ansible.

Step 6 installs the `ansible` package, not `ansible-core`: the core package
leaves out `community.general`, which the firewall package needs, and that only
shows up much later inside a play. It is the last thing done by hand — from
there on everything the CLI does to a machine is a play.

Ansible is installed on **Debian and Ubuntu**, the two this has been run on.
Anywhere else `setup` stops and names your distribution rather than guessing at
a package manager: install `ansible` by hand and run it again.

Step 4 is why the order cannot change. Installing a key does not prove it
works — a wrong mode on `authorized_keys`, an `AuthorizedKeysFile` pointing
elsewhere, or SELinux all let it install and still refuse it. **If the proof
fails, nothing is hardened and password login stays on**, so you can still get
in; the error says where to look.

The password is used once, on one connection, and is written nowhere: not to
`config.yml`, not to the lock, not to the log.

| Flag | Meaning |
| --- | --- |
| `--force` | overwrite a configuration that already exists |
| `--no-harden` | leave password login on; the key is still installed and proved |

Running it again on a machine it already owns is safe: it offers the key it
made last time, finds that the key works, and skips straight past the
password.

## setup git

```
devmachine setup git [--yes] [--check]
```

Makes the configuration directory a git repository, so it can be pushed to a
private remote. Only `config.yml` and `packages.lock` are ever committed.

In order:

1. writes `<config>/.gitignore`, **before** `git init` — there is no moment
   where `git add -A` could pick up a key or a secret;
2. `git init -b main`;
3. commits `.gitignore`, `config.yml` and `packages.lock`;
4. checks what is tracked. A directory that became a repository by hand
   before this command existed can already be tracking a key — that is
   **refused**, with the fix (`git rm --cached <path>`) and a reminder that
   untracking a file does not remove it from a commit that already has it, so
   the key must be rotated too;
5. with no remote yet: if `gh` is on `PATH` and logged in, offers to create a
   **private** repository and push to it; otherwise prints the two commands to
   run by hand.

The remote must be private, and the command says so: `config.yml` holds real
hostnames and usernames.

| Flag | Meaning |
| --- | --- |
| `--yes` | create the private remote and push without asking |
| `--check` | say what would happen, and write nothing |

Running it again on a directory that is already a repository skips straight to
the tracked-files check — it never re-writes `.gitignore` or re-runs `git init`.

## doctor

```
devmachine doctor [--machine m]
```

Four checks, in order: the configuration, the connection, the operating system,
and whether Ansible is installed. Then one check per credential the installed
packages declare, named `credential: <key>`, saying what is missing and the
command that delivers it. Then one check per installed DNS provider, named
`dns: <provider>`, asking it what it holds — a stale token is better found
here than while creating a subdomain. Exits non-zero if any failed.

A machine whose packages declare no credential reports none, and a machine
with no DNS provider installed reports none of those either: neither is a
failure, both are a choice.

A check that could not run reports `skip` and why. A credential whose package
never said where it is kept reports `skip` too: there is nowhere to look, and
"I cannot tell" is not "it is not there".

## config

```
devmachine config path      the directory in use, and the rule that chose it
devmachine config show      machines, workspaces and their effective values
```

`show` validates, so it is also the quickest way to find out what is wrong with
a configuration.

## machines

```
devmachine machines list                  each machine, its addresses, port and workspaces
devmachine machines add [--no-harden]     take over another machine and record it
devmachine machines rm <name> [--yes]     forget a machine; the server keeps running
devmachine machines create-local <name>   a machine on this computer
devmachine machines start <name>          start a local machine
devmachine machines stop <name>           stop a local machine
devmachine machines delete-local <name> [--yes]   destroy it and everything on it
```

`setup` writes the first machine; `add` writes every one after it. It asks the
same questions, minus the domain, and runs the same bootstrap: the key first, a
password only if the key is refused, the proof on a connection of its own, then
hardening and Ansible. See [setup](#setup) for what each step is for.

`rm` takes a machine out of `config.yml` and **does nothing at all to the
server** — it keeps running, with everything on it, and the key still gets in.
It asks first unless `--yes` is given, and it refuses to leave a workspace
pointing at a machine that is no longer there.

**`rm` is not `delete-local`.** They look alike and only one destroys anything:
`rm` forgets a server, `delete-local` erases a machine on this computer.

`create-local` builds a machine on this computer and hands it back as an
ordinary machine: an address, a port and an admin login. It comes up the way a
bought server arrives — root reachable over SSH with a password and **no key
installed** — so `devmachine setup` has the same work to do on it as on
anything else. The root password is `devmachine`, and it is public on purpose:
the VM holds no data and exists to be destroyed.

It writes nothing to your configuration. `setup` does that, and running it
against the new machine is the point.

`start`, `stop` and `delete-local` act on a local machine only. A bought server
is not the CLI's to switch on or off. `delete-local` asks first.

Two limits, both from what a machine on your own computer is:

- It needs [Lima](https://lima-vm.io) (`brew install lima`), which runs on
  macOS and Linux. **There is no Windows path.**
- It is not reachable from the internet, so `dns`, TLS and subdomains do not
  work on it. Everything else does: `setup`, `doctor`, `run`, `ssh`, `sync`,
  packages.

## workspaces

```
devmachine workspaces list
devmachine workspaces new <name> [--machine m] [--like w] [--packages a,b] [--user u] [--check] [--yes]
devmachine workspaces edit <name> [--machine m] [--user u] [--add p] [--rm p] [--set k=v] [--check] [--yes]
devmachine workspaces rm <name> [--yes]
```

A workspace is one Linux account on one machine. These commands edit
`config.yml` and touch no machine — `devmachine sync` is what creates or
changes the account.

`new` takes its package list from `defaults.workspace` in your configuration:

```yaml
defaults:
  workspace: [workspace, dev, zsh, mise]
```

`setup` seeds that list, and changing one line there changes every workspace
made afterwards. `--packages` overrides it for one workspace; `--like <name>`
copies another workspace's list instead.

**`--like` copies the packages and nothing else.** Not the Linux account, which
would collide, and not the machine, which would put one workspace wherever
another happens to be.

**`new` refuses on a machine with no key.** A workspace is reachable because
the administrative key is copied into it; with nothing to copy, the account
would be created with no way in. The fix is `devmachine setup`, which gives the
machine a key. A machine with no `key:` in `config.yml` is served by your SSH
agent, so an agent holding nothing is the same situation.

With several machines configured, `new` refuses to guess: pass `--machine`.

`edit` changes one workspace. `--add` and `--rm` take a package name each and
may be repeated. `--set <package>.<name>=<value>` writes into the workspace's
`settings:`, which is how a package's variables are set — see
[packages](../concepts/packages.md). The value is read as YAML, so
`--set claude-plugins.plugins=[one, two]` sets a list; an empty value,
`--set zsh.theme=`, takes the setting out again.

`--share <credential>=own` keeps this workspace's own login instead of the one
shared across the machine — which is how one workspace signs in to a different
account. `=machine` puts it back, and an empty value falls back to whatever the
configuration says.

A workspace with `own` is left out of the copying on purpose, so it never loses
the account it logged in with.

A setting for a package the workspace does not install is refused. It would
reach nothing: the recipe would quietly keep its default, and the machine would
not be what the configuration says it is.

**Changing `--machine` does not move a workspace.** It looks like it does. The
next `sync` creates the account on the new machine, and the old one keeps
everything it had — its home, its files, its account. The command says so.

**`rm` leaves the Linux account, its home and its files on the machine.**
Deleting a home is not something a configuration edit should do, and `sync`
could not put it back. Remove them there by hand if you really want them gone.

## aliases

```
devmachine aliases [--write] [--path p] [--check] [--yes]
```

Prints one SSH `Host` entry per workspace, so `ssh alice-devmachine` and
`mosh alice-devmachine` work from an ordinary terminal — no CLI in the way, no
package on the machine, nothing to install.

```
# >>> devmachine — generated, do not edit
Host alice-devmachine
    HostName 100.64.0.5
    User alice
    Port 22
    IdentityFile /home/you/.config/devmachine/keys/main
    IdentitiesOnly yes
    HostKeyAlias main-devmachine
# <<< devmachine
```

`--write` puts the block in `~/.ssh/config`, or in the file `--path` names. It
asks first. **Only the block between the two markers is replaced.** That file
holds hosts this CLI knows nothing about — a work jump host, a sandbox, a
client's bastion — and rewriting the whole file deletes them.

Three things the entries do on purpose:

- **`HostKeyAlias` is the same for every alias of one machine.** `known_hosts`
  is indexed by address, so a machine on two addresses gets two entries and
  switching between them gives `Host key verification failed`. This indexes by
  name instead.
- **`HostName` is the first address that resolves.** A `tailscale:` entry is
  turned into an address here, the same way every other command does it.
- **`IdentitiesOnly yes` goes with `IdentityFile`.** Without it ssh offers
  every key the agent holds first, and a server can cut the connection at
  `MaxAuthTries` before the one that works is tried. A machine with no `key:`
  is served by the agent, so neither line is written for it.

A `-pub` alias is written only when there is a second, literal address to fall
back to and the first one did not already resolve to it. With one address, or
with the tailnet already down, a second entry would only repeat the first.

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
devmachine run --package <name> -- <command> [args...]
```

Runs one command and prints its output. Without `--workspace` it runs as the
machine's admin. The exit code is the command's.

The output of a command that failed is printed before the error, because that
output is usually the explanation.

`--package` reaches an installed package's entrypoint directly, instead of a
shell command — everything after `--` is what the package receives. This is
the generic door onto a machine: it removes the need to know an address, an
account, a port or a key, and it does not limit what a shell can do.
`commands:` in the package's manifest is what limits, and only for a package
that asked to be limited — `run --package` refuses anything the manifest does
not list, and says what it does accept. `--package` and `--workspace` name
two different targets and cannot be combined. See [how a package declares an
entrypoint](../concepts/packages.md).

## dns

```
devmachine dns status [host]
devmachine dns providers
devmachine dns list [zone] [--dns-provider p] [--zone z]
devmachine dns check <name> [--dns-provider p] [--zone z]
devmachine dns add <name> <type> <value> [--dns-provider p] [--zone z] [--check] [--yes]
devmachine dns rm  <name> <type> [value] [--dns-provider p] [--zone z] [--check] [--yes]
```

`status` checks a name from outside: it resolves, its certificate is
accepted, and it answers a request. With no host it uses the configured
`domain`. A 4xx counts as serving — the server answered. A 5xx does not.

`providers` lists every installed DNS provider and the zones it can see. It
is the one read that shows the whole picture, and the first thing to run when
a DNS command did not do what was expected. A provider that cannot be asked
(a stale token, for example) is reported with its error rather than failing
the whole command — that is what this command exists to show. With none
installed, it says to run `devmachine packages list`.

`list` and `check` ask the registrar, through whichever installed provider
holds the zone — a different question from `dns status`, which asks the
public internet. A record can exist at the registrar and not have propagated
yet, and it can resolve while sitting in a zone nobody manages through this
CLI. `list` prints every record for a zone; with no zone it falls back to the
configured `domain`. `check` says whether one name is pointed, and exits
non-zero when it is not — readable from a script, not only from the words.

`--dns-provider` acts through a named provider instead of asking which
installed one holds the zone. `--zone` is rarely needed: it exists for a zone
a provider's token cannot list, when `--dns-provider` alone leaves the CLI
unable to tell where the label ends and the zone begins.

Which provider answered, and why, is printed to stderr on every one of these
— stdout is the data, and which provider was asked is diagnostics.

`add` makes a name hold **exactly** one value: `upsert` replaces whatever was
already there for that name and type, it does not add to it. Before writing,
the confirmation names the zone, the provider and whatever value it is about
to replace — writing the right record into the wrong account is the mistake
this command can make, and that is the last chance to catch it. `--check`
prints what would change and writes nothing; `--yes` skips the question.

`rm` removes one value, or with none given, every value at that name and
type. Removing one value out of several is not atomic on every registrar, and
the command warns before doing it. A name is always the full name (`www.example.com`,
or the zone itself for the apex) — the CLI turns it into the label the
provider expects.

## secrets

```
devmachine secrets set <name> [value] [--stdin]
devmachine secrets list
devmachine secrets rm <name>
devmachine secrets example
```

With no value, `set` asks without echoing, so the secret never reaches your
shell history. `list` prints names only.

`example` lists what a machine's packages need as `<NAME>=`, one per line, with
no value — the record of **which** secrets a machine needs, never what they
are. It never reads a stored value, and it always writes to standard output,
never to a file: a file named `.env.example` sits one typo away from `.env`,
in a directory that may be a git repository, and that is a trap the operator
sets themselves, by redirecting the output where they want it.

## login

```
devmachine login <credential> [--workspace w] [--machine m]
```

Runs the login a package declared, in the place that credential belongs. The
CLI reads the command out of the declaration; it knows nothing about any
particular tool.

The session is a real terminal (`ssh -t`, the same path `devmachine ssh`
takes), because a device code or a browser prompt reaches a person or it
reaches nobody.

A workspace credential is logged into as that workspace, and needs
`--workspace`: accounts differ between workspaces, so there is no master
session to copy and the CLI will not pick one for you.

A machine credential is logged into once, as the machine's admin, and what the
tool wrote is copied into `/etc/devmachine/<name>/`. The next `devmachine sync`
is what spreads it to the workspaces that declare the package.

A `kind: secret` is refused: nobody logs into a value. Use `devmachine secrets
set`, then `devmachine credentials push`.

The system `ssh` is what opens the session, so the first login to a machine it
has never seen asks you to accept the host key. See
[the troubleshooting page](../troubleshooting.md).

## credentials

```
devmachine credentials list [--machine m]
devmachine credentials push [--machine m] [--check] [--yes]
```

What the packages installed on a machine and its workspaces cannot work
without, and what is missing.

Every row says the command that fixes it: a missing login says
`devmachine login <name>`, and a missing secret says `devmachine secrets set
<name>`, then `devmachine credentials push`. A secret you have already stored
asks only for the push.

A row reads `unknown` when the package never said where its tool keeps the
result. There is nowhere to look, and "I cannot tell" is not "it is not there".

The report never prints a value, in either format.

`push` delivers the values the machine is missing, and only those. A login is
skipped — nobody can push a browser session — and a credential you never stored
a value for is named, with the `secrets set` that fixes it, because a push that
quietly does nothing is the failure this command exists to prevent. It exits
non-zero when it found one.

A value is read from the secret named after the credential. A workspace that
needs its own value stores it as `<workspace>/<name>`, and that wins over the
shared one.

`--check` says what it would write and writes nothing. A value is never
printed, in either format, and the command log records the push without it.

## packages

```
devmachine packages list
devmachine packages add <name> [--machine m | --workspace w] [--check] [--yes]
devmachine packages rm  <name> [--machine m | --workspace w] [--check] [--yes]
devmachine packages new <name> [--scope machine|workspace] [--into <dir>]
devmachine packages validate <dir>
devmachine packages schema [--json]
devmachine packages help <name> [--json]
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

`help` asks an installed package what it accepts, by running its own `help`
and printing the answer — a table by default, the raw shape under `--json`.
It refuses a package that declares no `entrypoint`: a package that cannot be
called has nothing to ask.

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

Beside a tag per package there is one more: `credentials`, which runs only the
copying of the shared logins. `devmachine sync --tags credentials` is what to
run after `devmachine login`, instead of the whole machine again.

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

`run` and `sync` append one line each to `<config>/history.log`, mode `0600`:

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

**`setup` and `machines add` are not logged**, and that is deliberate. They are
the two commands that handle a root password, and a log is the last place that
should ever come close to one. They also run once per machine, so there is
little to look back at.

`login` is not logged either: what it runs is a command the package declared,
in a terminal you are watching.
