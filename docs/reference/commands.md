# Commands

Every command accepts:

| Flag | Meaning |
| --- | --- |
| `--config <dir>` | the configuration directory to use |
| `--format table\|json` | how to print; JSON is the stable contract |
| `--machine <name>` | which machine to act on, for commands that act on a server |
| `--help` | what this command does |

`devmachine help --json` prints the whole tree, including every flag, in
one document — read this instead of parsing help text.

## setup

```
devmachine setup [--force] [--no-harden]
```

Connects to your server for the first time and gets it ready to use.

With no `config.yml` yet, it asks for a machine name, an address, the
admin account, a port, a domain, and how to log in: a key devmachine
makes for itself (recommended), a key file already on your computer, or
one your SSH agent holds (for a key in a password manager).

It shows the server's fingerprint first and asks you to check it against
your provider's dashboard. Say no and nothing is sent or changed.

Then it gets the server ready, in order: try the key; if that fails, ask
for the password (never shown on screen); install the key; open a **new
connection using only the key** to prove it works; turn password login
off; install Ansible. If the proof step fails, nothing is locked down and
the error says where to look. See [setting up a server for the first
time](../how-it-works/trust-bootstrap.md) for why the order matters.

Works on **Debian and Ubuntu** only; elsewhere it names your distro and
stops. The password is used once and written nowhere.

| Flag | Meaning |
| --- | --- |
| `--force` | discard the existing configuration and start over |
| `--no-harden` | leave password login on; the key is still installed and proved |

Run again with a configuration in place, and it just makes sure Ansible
is installed — it never rewrites `config.yml`, a key, or SSH settings.

**On a self machine** (`self: true`, your own computer — see
[`machines`](#machines)), setup only checks Homebrew and installs Ansible;
no fingerprint, key or password involved.

## setup git

```
devmachine setup git [--yes] [--check]
```

Turns your configuration directory into a git repository, so you can push
it to a private remote. `config.yml`, `packages.lock` and `known_hosts`
are committed — see [versioning your
configuration](../how-it-works/versioning-your-configuration.md) for what
never is.

In order: writes `.gitignore` **before** `git init`, so a key can never
be picked up; `git init -b main`; commits the files above; checks what
got tracked and **refuses** if a key is already tracked, with the fix; if
`gh` is on `PATH` and logged in, offers to create a **private** repo and
push.

| Flag | Meaning |
| --- | --- |
| `--yes` | skip local-write questions; never create or push a remote |
| `--check` | say what would happen, write nothing |

Run again on an existing repository, and it skips straight to the
tracked-files check.

## doctor

```
devmachine doctor [--machine m]
```

Checks whether a machine is healthy and reports what is wrong.

Five checks in order: configuration, SSH fingerprint, login, operating
system, Ansible installed. A broken fingerprint stops the rest. Then one
check per needed credential (`credential: <key>`) and per installed DNS
provider (`dns: <provider>`). Exits non-zero if anything failed.

No credential or DNS provider needed is not a failure. A check that could
not run reports `skip` and why — "I cannot tell" is not "it is not
there".

## config

```
devmachine config path      the directory in use, and the rule that chose it
devmachine config show      machines, workspaces and their effective values
```

Shows where your configuration lives and what is in it. `show` validates
as it prints, so it is the quickest way to find what is wrong.

## machines

```
devmachine machines list                  each machine, its addresses, port and workspaces
devmachine machines add [--no-harden]     set up another server and record it
devmachine machines add --self <name>     add your computer as a machine, with no address
devmachine machines trust [name] [--check] [--replace] [--yes]   check or update its SSH fingerprint
devmachine machines rm <name> [--yes]     forget a machine; the server keeps running
devmachine machines create-local <name>   a machine on your computer
devmachine machines start <name>          start a local machine
devmachine machines stop <name>           stop a local machine
devmachine machines delete-local <name> [--yes]   destroy it and everything on it
```

Manages the list of machines devmachine knows about.

`list --format json` prints each machine with `name`, `hosts`,
`admin_user`, `port`, `key`, `workspaces`, and `self: true` on your own
computer. **Your computer is never picked by default** — a command with
no `--machine` still acts on the server, even with a self machine also
configured.

`add` sets up another server, same as [setup](#setup). `add --self
<name>` instead names the computer devmachine runs on: no address, port
or key. Refuses if a self machine already exists, or the name is taken.
See [your computer as a machine](../how-it-works/your-computer-as-a-machine.md)
for how this differs from `machines create-local`.

A self machine has no `hosts`, `user`, `port` or `key`, and no workspace
can live on one. Any command needing a real SSH address — `ssh`, `mosh`,
`tunnel`, `login`, `expose`, `dns`, `machines trust`, `aliases` — refuses
on it, naming the reason.

`trust` reads the server's public fingerprint without logging in: asks
before saving a new one, does nothing if it matches, refuses a changed
one unless `--replace`. `--check` compares without writing. JSON fields:
`machine`, `address`, `status`, `key_type`, optional
`current_fingerprint`, `presented_fingerprint`, `check`, `changed`.

`rm` takes a machine out of `config.yml` and **does nothing to the server
itself**. Asks first unless `--yes`; refuses to leave a workspace
pointing at a gone machine. **Not `delete-local`**: `rm` only forgets a
server, `delete-local` erases a machine on your computer.

`create-local` builds a machine on your computer, arriving password-only
like a bought server — `devmachine setup` still has to run against it.
Root password `devmachine`, public on purpose: this VM holds no real
data. `start`, `stop` and `delete-local` only act on a local machine.

Two limits: needs [Lima](https://lima-vm.io) (`brew install lima`), macOS
and Linux only; not reachable from the internet, so `dns`, HTTPS and
subdomains do not work on it.

## workspaces

```
devmachine workspaces list
devmachine workspaces new <name> [--machine m] [--like w] [--packages a,b] [--user u] [--check] [--yes]
devmachine workspaces edit <name> [--machine m] [--user u] [--add p] [--rm p] [--set k=v] [--check] [--yes]
devmachine workspaces defaults [--add p] [--rm p] [--check] [--yes]
devmachine workspaces rm <name> [--yes]
devmachine workspaces destroy <name> [--confirm <name>] [--check]
```

A workspace is one Linux account on one machine. See
[workspaces](../concepts/machines-and-workspaces.md).

These commands only edit `config.yml` — `devmachine sync` creates or
changes the account.

`new` takes its package list from `defaults.workspace: [pkg, ...]` in
`config.yml`. `--packages` overrides it for one workspace; `--like
<name>` copies another workspace's package list instead — packages only,
never the account or the machine.

**`new` refuses on a machine with no key**, since a workspace is reached
through the copied admin key. Run `devmachine setup` first. With several
machines, pass `--machine`.

`edit` changes one workspace. `--add`/`--rm` take a package name each,
repeatable. `--set <package>.<name>=<value>` writes a package option
(read as YAML — see [packages](../concepts/packages.md)); an empty value
removes it. `--share <credential>=own` keeps this workspace's own login
instead of the shared one; `=machine` shares it again. A package option
for a package the workspace does not install is refused.

**Changing `--machine` does not move a workspace.** The next `sync`
creates the account on the new machine; the old one keeps everything.

**`rm` leaves the Linux account, home and files on the machine** — remove
those by hand if you want them gone.

**`destroy` deletes for real**: the account, its home, its Caddy routes,
and its `config.yml` entry. Asks you to retype the name first (or
`--confirm <name>` from a script); DNS records are left alone. Needs the
machine reachable; use `rm` for one that is gone.

`defaults` only changes `defaults.workspace`, which new workspaces
inherit; existing ones are unchanged.

## skills

```text
devmachine skills add [--package name] [--agent claude|codex|pi|opencode] [--yes]
devmachine skills list
devmachine skills update [--yes]
devmachine skills remove <name> [--yes]
```

Manages Agent Skills on your own computer; never touches a machine.

Bare `add` installs `devmachine-skills` from the pinned release (or the
latest, before `setup` has pinned one). `--package` only accepts a local
package. Without `--agent`, it detects installed agent tools and asks.

`list` shows each source, its skills and agent tools. `update` reinstalls
every recorded source. `remove` takes one exact skill name.

Real copy: `~/.agents/skills/<name>`. Claude's is a link:
`~/.claude/skills/<name> -> ../../.agents/skills/<name>`.

## aliases

```
devmachine aliases [--write] [--path p] [--check] [--yes]
```

Prints one SSH `Host` entry per workspace, so `ssh alice-devmachine` and
`mosh alice-devmachine` work from an ordinary terminal.

```
# >>> devmachine — generated, do not edit
Host alice-devmachine
    HostName 100.64.0.5
    User alice
    Port 22
    IdentityFile /home/you/.config/devmachine/keys/main
    IdentitiesOnly yes
    HostKeyAlias main-devmachine
    StrictHostKeyChecking yes
    UserKnownHostsFile "/home/you/.config/devmachine/known_hosts"
    GlobalKnownHostsFile /dev/null
    UpdateHostKeys no
    CheckHostIP no
    VerifyHostKeyDNS no
    KnownHostsCommand none
    HostKeyAlgorithms ssh-ed25519
# <<< devmachine
```

`--write` puts this block in `~/.ssh/config`, or the `--path` file, after
asking first. **Only the text between the two markers is ever
replaced** — the rest of the file may hold hosts devmachine knows
nothing about.

- `HostKeyAlias` is the same for every alias of one machine, so switching
  addresses never trips `Host key verification failed`.
- `HostName` is the first address that resolves, same as every other
  command.
- `IdentitiesOnly yes` goes with `IdentityFile`, so ssh offers only this
  key and does not burn login attempts on others in your agent.

A `-pub` alias is only written when a second address exists and the
first did not already resolve to it.

## stats

```
devmachine stats [--machine m]
```

Prints memory, swap, disk and load. The table rounds; JSON gives raw byte
counts.

## ssh, mosh

```
devmachine ssh [workspace]
devmachine mosh [workspace]
```

Opens an interactive session: a workspace's account if named, the
machine's admin if not.

Both run the real `ssh`/`mosh` program, so your terminal, agent and tmux
behave normally. `mosh` survives a dropped or roaming connection and
needs mosh on both sides. Both check `<config>/known_hosts` first.

## run

```
devmachine run "<command>" [--workspace w]
devmachine run --package <name> [--workspace w] -- <command> [args...]
```

Runs one command on a machine and prints its output; exits with the same
code. A failed command's output prints before the error, since it usually
explains the failure.

`--package` calls an installed package's entrypoint directly — everything
after `--` goes to the package; `commands:` in its manifest can limit
what it accepts. With `--workspace`, it runs as that workspace's own
account, for a command that needs that account's own files or logins.
See [packages](../concepts/packages.md).

`run` keeps its SSH connection open for five minutes and reuses it, so a
script calling it every few seconds skips the handshake each time.

## dns

```
devmachine dns status [host]
devmachine dns providers
devmachine dns list [zone] [--dns-provider p] [--zone z]
devmachine dns check <name> [--dns-provider p] [--zone z]
devmachine dns add <name> <type> <value> [--dns-provider p] [--zone z] [--check] [--publish]
devmachine dns rm  <name> <type> [value] [--dns-provider p] [--zone z] [--check] [--yes]
```

Manages DNS records through an installed provider package (Hostinger,
Cloudflare). See [DNS providers](../how-it-works/dns-providers.md) for
what differs between them.

`status` checks a name from the outside — resolves, certificate accepted,
answers a request. Defaults to `domain`. A 4xx counts as serving; a 5xx
does not.

`providers` lists every installed provider and the zones it can see — run
first when a DNS command surprised you.

`list`/`check` ask the registrar, unlike `dns status` which asks the
public internet. `check` exits non-zero when a name is not set.
`--dns-provider` picks a provider directly; `--zone` only for a token
that cannot list its own zones. Which provider answered goes to stderr.

`add` makes a name hold **exactly** one value, replacing what was there,
after showing the zone, provider and value it replaces. `--check`
previews; `--publish` is the non-interactive consent (`--yes` alone never
grants it).

`rm` removes one value, or every value at that name and type with none
given — not always one atomic step on the registrar's side, so it warns
first. A name is always the full name (or the zone itself for the apex).

## expose

**HTTPS only.** Caddy handles HTTPS for you; anything else, or anything
only you should reach, uses [`devmachine tunnel`](#tunnel) instead.

```
devmachine expose add <workspace> <port> --host <host> [--check] [--publish]
devmachine expose list
devmachine expose rm <host> [--check] [--yes]
```

Publishes a workspace's port to the internet, over HTTPS, at a hostname
you choose.

`add` records the site in `config.yml`; `devmachine sync` writes the
Caddy config. **Refuses if `caddy` is not on the machine.** It asks for
confirmation first — the port becomes reachable by anyone who learns the
hostname; see [tunnel](#tunnel) for what should not get a yes.
`--publish` is the non-interactive way past that; `--check` previews. It
also points the hostname at the machine, same as `dns add`. See [why a
published site lives in the configuration](../how-it-works/published-sites.md).

`list` prints every host with its port, workspace, and one of four
words: `published` (both agree), `pending`/`differs` (needs `sync`),
`unmanaged` (only the machine has it — adopt with the `add` shown).
Unreachable machine or missing `caddy`: rows print `unknown`.

`rm` takes a host out of the configuration; the next `sync` removes it. A
host the configuration does not know is refused, with how to adopt or
remove it by hand. See [Publishing](../concepts/publishing.md) for the
cases this question exists to catch.

## tunnel

```
devmachine tunnel <workspace> <port> [--local <port>]
```

Opens an SSH tunnel so a remote port shows up as `localhost:<port>` on
your computer. **Nothing is published** — no DNS, no certificate, no
Caddy, only you can reach it.

Use this instead of `expose` for anything not plain HTTP, or that only
you should see: a database tool with real data, an inbox with real mail,
a queue dashboard that can drain a queue.

| | Anyone | Only you |
| --- | --- | --- |
| **HTTP** | `expose` | `tunnel` |
| **Anything else** | nothing | `tunnel` |

`--local` picks the port on your computer, if the remote one is already
taken here. Holds your terminal open while the tunnel is up; Ctrl-C
closes it, nothing left running.

## machine

```
devmachine machine setup
devmachine machine doctor
```

Sets up and checks your own computer — the one thing you still install by
hand, once.

`doctor` checks `ssh`/`mosh` on `PATH` (mosh is a warning only), an SSH
agent or configured key, and whether `~/.ssh/config` matches what
`devmachine aliases --write` would produce now.

`setup` installs what is missing via Homebrew on a Mac; on Linux it names
what to install instead of guessing. Never installs an editor, shell
plugins or language runtimes — that stays your choice.

## secrets

```
devmachine secrets set <name> [value] [--stdin]
devmachine secrets list
devmachine secrets rm <name>
devmachine secrets example
```

Stores values packages need that are not logins — API keys, tokens.

`set` with no value asks without echoing, so it never reaches your shell
history. `list` prints names only.

`example` lists which `<NAME>=` a machine's packages need, no values,
always to stdout — never to a file, since `.env.example` sits one typo
from `.env`.

## login

```
devmachine login <credential> [--workspace w] [--machine m]
```

Runs the login a package declared, in the account it belongs to, over a
real terminal session (`ssh -t`) — a device code or browser prompt has to
reach a person.

A workspace credential needs `--workspace`: no single session to copy
between accounts. A machine credential logs in once, as the admin, and
copies what the tool wrote to `/etc/devmachine/<name>/`; the next `sync`
spreads it to workspaces using that package. A `kind: secret` credential
is refused — use `devmachine secrets set`, then `devmachine credentials
push`.

Same strict fingerprint check as every other command. See [SSH host
keys](../how-it-works/ssh-host-keys.md).

## credentials

```
devmachine credentials list [--machine m]
devmachine credentials push [--machine m] [--check] [--yes]
```

Shows what a machine's packages need to authenticate, and what is
missing. Every row names the fixing command. `unknown` means the package
never said where its tool keeps the result — not the same as missing.
Never prints a value.

`push` delivers only missing values, skipping logins (nobody can push a
browser session) and naming any secret never stored; exits non-zero if
it found one. A workspace's own value (`<workspace>/<name>`) wins over
the shared one. `--check` previews and writes nothing.

## packages

```
devmachine packages list
devmachine packages add <name> [--machine m | --workspace w] [--check] [--yes]
devmachine packages rm  <name> [--machine m | --workspace w] [--check] [--yes]
devmachine packages new <name> [--scope machine|workspace] [--into <dir>]
devmachine packages validate <dir>
devmachine packages schema [--json]
devmachine packages help <name> [--json]
devmachine packages pin [release]
```

Manages what is installed on your machines and workspaces. See [the
package format](package-format.md).

`list` shows each package once with every machine and workspace that
uses it; one nothing provides is listed as `missing`.

`add`/`rm` only edit `config.yml` — `sync` applies the change. Pass
`--machine` or `--workspace`; with one configured machine, that is the
target. Comments in `config.yml` survive.

`new` writes a package that already passes `validate`; refuses to
overwrite one that exists. `validate` reports every problem at once, with
file and line. `schema` prints the `package.yml` format this binary
reads. `pin` writes `packages: <release>` — with none given, the latest;
a release is a tag such as `v8`, never a branch. `help` asks an installed
package what it accepts, by running its own `help`.

## sync

```
devmachine sync [--machine m] [--check] [--yes] [--tags a,b]
```

Applies your configuration to a machine — installs what is missing,
updates what changed.

In order: checks the configuration, fetches the pinned release if needed,
plans what the machine and its workspaces should get, checks your own
packages, prints the plan, asks, then runs Ansible on the machine,
streaming output.

| Flag | Meaning |
| --- | --- |
| `--check` | dry run: reports what would change, changes nothing |
| `--yes` | apply without asking |
| `--tags a,b` | only the packages named |

One tag beyond package names: `credentials`, which only copies shared
logins — run after `devmachine login` instead of a full sync.

`--check` never writes the lock file. Only your own packages (in
`<config>/packages/`) are checked before the run. With `--format json`,
stdout is the result; the plan and machine output go to stderr.

On success, `<config>/packages.lock` records what was applied, at which
release and checksum, for the machine synced.

## version, help

```
devmachine version
devmachine help [command] [--json]
```

## The command log

`run` and `sync` each append one line to `<config>/history.log`, mode
`0600`:

```
2026-09-18T12:00:00Z  workspace alice   ok      "docker ps"
2026-09-18T12:01:00Z  machine main      failed  "sync --tags caddy"
```

UTC time, target, success or failure, then the quoted command. See
[configuration](../concepts/configuration.md#the-command-log).

**`setup`, `machines add` and `login` are not logged** — the first two
handle a root password, and `login` runs a command you watch yourself.
