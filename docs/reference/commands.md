# Commands

Every command accepts:

| Flag | Meaning |
| --- | --- |
| `--config <dir>` | the configuration directory to use |
| `--format table\|json` | how to print; JSON is the stable contract |
| `--machine <name>` | which machine to act on, for commands that act on a server |
| `--help` | what this command does |

`devmachine help --json` prints the whole tree, including every flag, in
one document. That is what a script or an agent should read instead of
parsing help text.

## setup

```
devmachine setup [--force] [--no-harden]
```

Connects to your server for the first time and gets it ready to use.

With no `config.yml` yet, it asks for a machine name, an address, the
admin account, a port and a domain, then how to log in:

1. a key devmachine makes for itself, kept in `<config>/keys/<machine>`
   and used for nothing else — the recommended choice, and the answer if
   you have no opinion;
2. a key file already on your computer;
3. a key your SSH agent holds, offered by fingerprint and comment.

Pick the third if you keep your key in a password manager: it never
touches disk, so there is no file to point at. `config.yml` then leaves
`key` out, which tells devmachine to ask the agent every time.

First it shows the server's fingerprint, a code that identifies the
server. Check it matches the one in your provider's dashboard, so you
know you are talking to your own server. Say no and nothing happens: no
password or key is sent, and nothing changes on the server.

After you approve it, `setup` saves the fingerprint to
`<config>/known_hosts`, writes `config.yml`, and gets the server ready:

1. it tries the key first. **If that works, no password is asked for** —
   many servers already have a key from the provider, and their owner has
   no root password to give;
2. only if the key fails does it ask for the password, without showing it
   on screen;
3. it installs the key;
4. it opens a **new connection using only the key**, to prove the key
   really works;
5. it turns password login off;
6. it installs Ansible, the tool devmachine uses to apply changes.

This works on **Debian and Ubuntu**, the only distributions this has been
tested on. Anywhere else, `setup` stops and names your distribution
instead of guessing: install `ansible` yourself and run `setup` again.

**If the proof in step 4 fails, nothing is locked down and password login
stays on**, so you can still get in; the error tells you where to look.
See [setting up a server for the first time](../how-it-works/trust-bootstrap.md)
for why the steps run in this order.

The password is used once, for one connection, and is written nowhere.

| Flag | Meaning |
| --- | --- |
| `--force` | discard the existing configuration and start over from scratch |
| `--no-harden` | leave password login on; the key is still installed and proved |

Run it again with a configuration already in place, and it resumes
getting the machine ready instead of starting over: it connects with the
pinned key or SSH agent, and installs Ansible if needed. It never
rewrites `config.yml`, installs a new key, or changes SSH settings. Use
`--force` only to discard the configuration and start the questions
again.

**On a self machine** (`self: true` in `config.yml`, your own computer —
see [`machines`](#machines) below), setup is much shorter: there is no
address, so no fingerprint, no key and no password are involved. It checks
that Homebrew is on `PATH` — if not, it prints the official one-line
install command from [brew.sh](https://brew.sh) and stops, since that
command needs `sudo` and running it for you is not this CLI's place — then
installs Ansible with `brew install ansible` if it is not already there.
If it is already set up, it says so and changes nothing.

## setup git

```
devmachine setup git [--yes] [--check]
```

Turns your configuration directory into a git repository, so you can push
it to a private remote.

`config.yml`, `packages.lock` and the public `known_hosts` are
committed — see
[versioning your configuration](../how-it-works/versioning-your-configuration.md)
for the full table of what is kept and what never is.

In order:

1. writes `<config>/.gitignore`, **before** `git init` — so `git add -A`
   can never pick up a key or a secret;
2. `git init -b main`;
3. commits `.gitignore`, `config.yml`, `packages.lock` and `known_hosts`
   when present;
4. checks what got tracked. If the directory was already a repository
   before this command existed, it may already be tracking a key — that is
   **refused**, with the fix (`git rm --cached <path>`). Untracking a file
   does not remove it from a past commit, so rotate the key too;
5. with no remote yet: if `gh` is on `PATH` and logged in, offers to
   create a **private** repository and push to it; otherwise prints the
   two commands to run yourself.

The remote must be private, and the command says so: `config.yml` holds
real hostnames and usernames.

| Flag | Meaning |
| --- | --- |
| `--yes` | skip questions about local writes; never create or push a remote |
| `--check` | say what would happen, and write nothing |

Running it again on a directory that is already a repository skips
straight to the tracked-files check — it never rewrites `.gitignore` or
runs `git init` again.

## doctor

```
devmachine doctor [--machine m]
```

Checks whether a machine is healthy and reports what is wrong.

Five checks run in order: the configuration, the SSH fingerprint, the
login, the operating system, and whether Ansible is installed. A missing,
broken or changed fingerprint stops the checks before login is even
tried. Then one check per credential the installed packages need, named
`credential: <key>`, saying what is missing and the command that fixes
it. Then one check per installed DNS provider, named `dns: <provider>`,
asking it what it holds — better to catch a stale token here than while
creating a subdomain. `doctor` exits non-zero if anything failed.

A machine whose packages need no credential reports none, and a machine
with no DNS provider reports none of those either: neither is a
failure, both are just how that machine is set up.

A check that could not run reports `skip` and why. A credential whose
package never said where it keeps its login also reports `skip`: there is
nowhere to look, and "I cannot tell" is not the same as "it is not
there".

## config

```
devmachine config path      the directory in use, and the rule that chose it
devmachine config show      machines, workspaces and their effective values
```

Shows where your configuration lives and what is in it.

`show` checks your configuration for errors as it prints it, so it is
also the quickest way to find out what is wrong.

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
`admin_user`, `port`, `key` and `workspaces`, and `self: true` on the one
that is your own computer — this is how a script finds the machine to run
something locally.

**Your computer is never picked by default.** With one server and a
`self` machine also configured, a command with no `--machine` still acts
on the server, exactly as before you added your computer; the `self`
machine is only reached by naming it. With two or more servers, a command
still refuses to guess.

`setup` sets up the first machine; `add` sets up every one after it. It
asks the same questions, minus the domain, and follows the same steps: the
key first, a password only if the key fails, proof on a fresh connection,
then locking down and installing Ansible. See [setup](#setup) for what
each step does.

`add --self <name>` is different: it names the computer devmachine itself
runs on, not a server. It asks nothing about an address, a port or a
key — there is none — and writes `- name: <name>` and `self: true` into
`config.yml`. It refuses if a self machine is already configured, or the
name is taken. See [your computer as a machine](../how-it-works/your-computer-as-a-machine.md)
for what makes a self machine different from `machines create-local`.

A self machine has no `hosts`, `user`, `port` or `key`. Any command that
needs a real address over SSH — `ssh`, `mosh`, `tunnel`, `login`,
`expose`, `dns` when it points a record at the machine, `machines trust`,
and `aliases` — refuses on it with `<name> is your computer (self:
true): <command> needs a machine it reaches over SSH`. A workspace can
never run on a self machine.

`trust` reads the server's public fingerprint without logging in. If
devmachine has no fingerprint on file, it asks before saving one; if the
fingerprint matches what is saved, nothing changes; if it has changed,
`trust` refuses unless you pass `--replace`. `--check` compares without
writing, and `--yes` skips only the local confirmation. Naming a machine
both positionally and with `--machine` is fine only when the two names
match.

With `--format json`, the stable fields are `machine`, `address`,
`status`, `key_type`, optional `current_fingerprint`,
`presented_fingerprint`, `check` and `changed`.

`rm` takes a machine out of `config.yml` and **does nothing to the server
itself** — it keeps running, with everything on it, and the key still
gets in. It asks first unless you pass `--yes`, and it refuses to leave a
workspace pointing at a machine that no longer exists.

**`rm` is not `delete-local`.** They sound alike but only one destroys
anything: `rm` just forgets a server, `delete-local` erases a machine on
your computer.

`create-local` builds a machine on your computer and hands it back like an
ordinary machine: an address, a port and an admin login. It arrives the
way a bought server does — root reachable over SSH with a password and
**no key installed** — so `devmachine setup` still has to run against it.
The root password is `devmachine`, public on purpose: this VM holds no
real data and exists to be thrown away.

It writes nothing to your configuration. `setup` does that.

`start`, `stop` and `delete-local` only act on a local machine. A bought
server is not something this CLI turns on or off. `delete-local` asks
first.

Two limits, both from what a machine on your own computer is:

- It needs [Lima](https://lima-vm.io) (`brew install lima`), which runs on
  macOS and Linux. **There is no Windows path.**
- It is not reachable from the internet, so `dns`, HTTPS and subdomains do
  not work on it. Everything else does: `setup`, `doctor`, `run`, `ssh`,
  `sync`, packages.

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
[workspaces](../concepts/machines-and-workspaces.md) for what one is used
for.

These commands only edit `config.yml` — none of them touch a machine.
`devmachine sync` is what actually creates or changes the account.

`new` takes its package list from `defaults.workspace` in your
configuration:

```yaml
defaults:
  workspace: [workspace, dev, zsh, mise]
```

`setup` fills that list in, and changing one line there changes every
workspace made afterwards. `--packages` overrides it for one workspace;
`--like <name>` copies another workspace's list instead.

**`--like` copies only the package list.** Not the Linux account, which
would collide, and not the machine, which would put the new workspace
wherever the copied one happens to live.

**`new` refuses on a machine with no key.** A workspace is only reachable
because the admin key gets copied into it. Run `devmachine setup` first.
A machine with no `key:` in `config.yml` relies on your SSH agent
instead, so an empty agent hits the same problem.

With several machines configured, `new` refuses to guess: pass
`--machine`.

`edit` changes one workspace. `--add` and `--rm` each take a package name
and can be repeated. `--set <package>.<name>=<value>` writes into the
workspace's `settings:`, which is how you set a package's own options —
see [packages](../concepts/packages.md). The value is read as YAML, so
`--set claude-plugins.plugins=[one, two]` sets a list; an empty value,
`--set zsh.theme=`, removes the setting again.

`--share <credential>=own` keeps this workspace's own login instead of the
one shared across the machine — how one workspace signs in to a different
account. `=machine` puts it back, and an empty value falls back to
whatever the configuration says.

A workspace set to `own` is left out of the copying on purpose, so it
never loses the account it logged in with.

Setting an option for a package the workspace does not install is
refused. It would reach nothing: the package would quietly keep its
default, and the machine would not match what the configuration says.

**Changing `--machine` does not move a workspace.** It looks like it
should. The next `sync` creates the account on the new machine, and the
old one keeps everything it had — its files, its account, all of it. The
command says so.

**`rm` leaves the Linux account, its home and its files on the machine.**
Deleting a home directory is not something a configuration edit should
do, and `sync` could not put it back. Remove them on the machine by hand
if you really want them gone.

**`destroy` is the one that deletes for real.** It removes the Linux
account, everything under its home, its Caddy routes file, and its entry
in `config.yml` — on the machine, for real. It asks you to type the
workspace's name again before doing anything, because there is no undo;
`--confirm <name>` answers that from a script, and must match exactly.
DNS records for its routes are left alone — take those down by hand too if
they should go. If the machine step fails, the configuration is left
untouched, so you can retry `destroy`. It needs the machine to be
reachable; use `rm` instead to forget a workspace on a machine that is
gone.

`defaults` only changes `defaults.workspace`, which new workspaces
inherit. Existing workspaces are unchanged. Like the other workspace
commands, it only edits the configuration and touches no machine.

## skills

```text
devmachine skills add [--package name] [--agent claude|codex|pi|opencode] [--yes]
devmachine skills list
devmachine skills update [--yes]
devmachine skills remove <name> [--yes]
```

Manages Agent Skills on your own computer. These commands never touch a
configured machine.

Bare `add` installs `devmachine-skills` from the pinned packages release,
even if a local package has the same name. Before `setup` has ever run,
there is no pinned release yet, so it uses the latest published release
instead — the same one `setup` would pin for a new configuration.
`--package` only accepts a local package. Without `--agent`, interactive
use detects installed agent tools and asks which ones to enable.

`list` reports each managed source, its skills and which agent tools use
them. `update` reinstalls every recorded source from the current release
or local package. `remove` takes one exact skill name and refuses a path
it does not manage or does not own.

The real copy lives at `~/.agents/skills/<name>`. Claude's copy is a link:
`~/.claude/skills/<name> -> ../../.agents/skills/<name>`.

## aliases

```
devmachine aliases [--write] [--path p] [--check] [--yes]
```

Prints one SSH `Host` entry per workspace, so `ssh alice-devmachine` and
`mosh alice-devmachine` work from an ordinary terminal — no devmachine
involved, nothing to install on the machine.

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

`--write` puts this block in `~/.ssh/config`, or the file `--path` names.
It asks first. The block goes at the top of the file because OpenSSH uses
the first value it reads for each setting. **Only the text between the two
markers is ever replaced.** The rest of that file may hold hosts devmachine
knows nothing about — a work jump host, a sandbox, a client's server — and
rewriting the whole file would delete them.

Three things worth knowing about these entries:

- **`HostKeyAlias` is the same for every alias of one machine.**
  `known_hosts` is indexed by address, so a machine reachable on two
  addresses would need two entries, and switching between them would show
  `Host key verification failed`. Using the name instead avoids that.
- **`HostName` is the first address that resolves,** the same way every
  other devmachine command picks an address. A `tailscale:` entry is
  turned into a real address here too.
- **`IdentitiesOnly yes` always goes with `IdentityFile`.** Without it,
  ssh offers every key your agent holds first, and the server can cut the
  connection before it tries the one that works. A machine with no `key:`
  relies on the agent, so neither line is written for it.

A `-pub` alias is only written when there is a second address to fall
back to, and the first one did not already resolve to it. With one
address, or with Tailscale already down, a second entry would just repeat
the first.

## stats

```
devmachine stats [--machine m]
```

Prints memory, swap, disk and load for a machine. The table rounds the
numbers for reading; the JSON gives raw byte counts.

## ssh, mosh

```
devmachine ssh [workspace]
devmachine mosh [workspace]
```

Opens an interactive session on a machine. With a workspace named, it
lands in that account on whichever machine it lives on. Without one, it
logs in as the machine's admin.

Both run the real `ssh` or `mosh` program, so your terminal, agent and
tmux behave the way they always do. `mosh` survives a connection that
drops or roams, and needs mosh installed on both sides. Both check
`<config>/known_hosts` before starting, and keep strict checking on in
the underlying `ssh`/`mosh` process too.

## run

```
devmachine run "<command>" [--workspace w]
devmachine run --package <name> [--workspace w] -- <command> [args...]
```

Runs one command on a machine and prints its output. Without
`--workspace`, it runs as the machine's admin. It exits with the same
code the remote command used.

If the command fails, its output is printed before the error, because
that output is usually what explains the failure.

`--package` calls an installed package's own entrypoint directly, instead
of running a shell command — everything after `--` is passed straight to
the package. This is the general-purpose door onto a machine: it removes
the need to know an address, an account, a port or a key, and it does not
limit what a shell can do. `commands:` in the package's manifest is what
limits it, and only for a package that asks to be limited — `run
--package` refuses anything not on that list and says what it does
accept. With `--workspace`, the package runs as that workspace's own
account on its own machine, instead of the machine's admin — how a
package offers a command that needs to read that account's own files or
its own logins, such as a per-workspace GitHub login. See [how a package
declares an entrypoint](../concepts/packages.md).

`run` keeps its SSH connection open for five minutes and reuses it across
calls, which is what lets a script call it every few seconds without
paying for a new handshake each time. It goes through the system OpenSSH
client, with the same pinned fingerprint `devmachine ssh` uses.

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
what each one does differently under the hood.

`status` checks a name from the outside: does it resolve, is its
certificate accepted, does it answer a request. With no host given, it
uses the configured `domain`. A 4xx response counts as serving — the
server did answer. A 5xx does not.

`providers` lists every installed DNS provider and the zones it can see.
This is the one command that shows the whole picture, and the first thing
to run when a DNS command did not do what you expected. A provider that
cannot be asked (a stale token, for example) is reported with its error
instead of failing the whole command. With none installed, it tells you
to run `devmachine packages list`.

`list` and `check` ask the registrar itself, through whichever installed
provider holds the zone — a different question from `dns status`, which
asks the public internet. A record can exist at the registrar and not
have spread yet, or it can resolve while sitting in a zone no provider
here manages. `list` prints every record in a zone; with none given, it
falls back to the configured `domain`. `check` says whether one name is
set, and exits non-zero when it is not — readable from a script, not just
from the words.

`--dns-provider` acts through a named provider instead of asking which
installed one holds the zone. `--zone` is rarely needed: only for a zone a
provider's token cannot list, when `--dns-provider` alone does not tell
devmachine where the label ends and the zone begins.

Which provider answered, and why, is always printed to stderr — stdout is
the data, and which provider was asked is a diagnostic detail.

`add` makes a name hold **exactly** one value: it replaces whatever was
already there for that name and type, rather than adding to it. Before
writing, it shows you the zone, the provider, and whatever value it is
about to replace — writing the right record into the wrong account is the
mistake this exists to catch. `--check` prints what would change and
writes nothing. `--publish` is explicit consent for a script to skip the
confirmation question; `--yes` never grants that consent on its own.

`rm` removes one value, or with none given, every value at that name and
type. Removing one value out of several is not always a single atomic
step on the registrar's side, and the command warns before doing it. A
name is always the full name (`www.example.com`, or the zone itself for
the apex — the root domain of your server) — devmachine turns it into the
label the provider expects.

## expose

**HTTPS only.** Caddy handles HTTPS for you; proxying raw TCP or UDP needs
a plugin and a custom build, which this project has chosen not to
maintain. Anything that is not plain HTTP, or that only you should reach,
uses [`devmachine tunnel`](#tunnel) instead of `expose`.

```
devmachine expose add <workspace> <port> --host <host> [--check] [--publish]
devmachine expose list
devmachine expose rm <host> [--check] [--yes]
```

Publishes a port on a workspace to the internet, over HTTPS, at a
hostname you choose.

`add` records the site in `config.yml`, under the workspace's `routes:`,
and touches no machine. `devmachine sync` writes the actual Caddy config,
through the `sites.d` extension point the `caddy` package provides —
never a path devmachine guesses itself. **If `caddy` is not on the
machine, `add` refuses and says so**, instead of recording a route
nothing can serve.

The configuration is the only record of what is published. A machine
rebuilt from it comes back with every site; a site set up on the machine
by hand does not. See
[why a published site lives in the configuration](../how-it-works/published-sites.md).

Before recording anything, `add` asks for confirmation — whatever is
behind the port becomes reachable by anyone who learns the hostname. See
[tunnel](#tunnel) for the kind of thing that should not answer that
question with yes. `--publish` is the only non-interactive way past the
question; `--yes` never publishes on its own. `--check` prints what would
happen and records nothing.

It also points the hostname at the machine, the same way `dns add` does:
with a provider installed, it writes the record; otherwise it prints the
exact record to create by hand. This runs here, not left for you to
remember separately, because a name that does not resolve yet fails
minutes later, buried in Caddy's certificate log.

`list` reads the configuration and the machine and prints every host with
its port, its workspace, and one of four words. `published` means both
agree. `pending` means the configuration has it and the machine does
not: run `sync`. `differs` means the machine has a different port or
owner for it: run `sync`. `unmanaged` means only the machine has it — a
site set up before this CLI tracked routes, or by hand — and it is gone
on a rebuild; the row shows the `expose add` command that would adopt it.
With the machine unreachable, or `caddy` no longer on it, the
configuration's own rows print as `unknown` rather than a guess; the
reason goes to stderr.

`rm` takes the site out of the configuration; the next `sync` removes its
config block and reloads Caddy. A host the configuration does not know
about is refused, with the two ways out: adopt it with `add`, or remove
the file on the machine by hand — `rm` never deletes what it does not
own.

See [Publishing](../concepts/publishing.md) for the three real cases this
question exists to catch.

## tunnel

```
devmachine tunnel <workspace> <port> [--local <port>]
```

Opens an SSH tunnel so a port on the server shows up as
`localhost:<port>` on your own computer. **Nothing is published**: no DNS
record, no certificate, no Caddy — the connection is encrypted by SSH
itself and only you can reach it.

Use this instead of `expose` for anything that is not plain HTTP, or that
only you should see: a database tool showing real data, an inbox showing
real mail, a queue dashboard that can drain a queue. All three speak
HTTP, none has its own login, and all three belong behind `tunnel`, not
`expose`.

| | Anyone | Only you |
| --- | --- | --- |
| **HTTP** | `expose` | `tunnel` |
| **Anything else** | nothing | `tunnel` |

`--local` picks the port on your computer, for when the remote port is
already taken here — a busy port is reported by name, with `--local`
offered as the fix, instead of a bind error nobody can read.

The command holds your terminal open while the tunnel is up. Closing it
(Ctrl-C) closes the tunnel: there is nothing left running afterward.

## machine

```
devmachine machine setup
devmachine machine doctor
```

Sets up and checks your own computer — the one thing here you still have
to install by hand, once.

`doctor` checks, in order: `ssh` on `PATH`; `mosh` on `PATH` (a warning,
not a failure — `ssh` alone is enough); an SSH agent or a key in the
configuration; and whether `~/.ssh/config` has the devmachine block, up to
date with what `devmachine aliases --write` would produce right now. A
workspace added after the last write is not reachable by name yet, and
this check is what tells you.

`setup` installs whatever is missing, through Homebrew on a Mac. On
Linux, it **tells you what to install** instead of guessing a package
manager.

**What this deliberately does not do:** it does not install an editor,
shell plugins, or language runtimes. Those are personal taste, and taste
stays in your own configuration, not in a machine this CLI sets up for
anyone.

## secrets

```
devmachine secrets set <name> [value] [--stdin]
devmachine secrets list
devmachine secrets rm <name>
devmachine secrets example
```

Stores values packages need but that are not logins — API keys, tokens.

With no value given, `set` asks without showing it on screen, so the
secret never reaches your shell history. `list` prints names only.

`example` lists what a machine's packages need as `<NAME>=`, one per
line, with no value — a record of **which** secrets a machine needs,
never what they are. It never reads a stored value, and it always writes
to standard output, never to a file: a file named `.env.example` sits one
typo away from `.env`, in a directory that might be a git repository, and
that is a mistake you make yourself, by choosing where to redirect the
output.

## login

```
devmachine login <credential> [--workspace w] [--machine m]
```

Runs the login a package declared, in the account it belongs to. devmachine
only reads the command out of the package's declaration; it knows
nothing about the tool itself.

This opens a real terminal session (`ssh -t`, the same path `devmachine
ssh` takes), because a device code or a browser prompt has to reach a
person, or it reaches nobody.

A workspace credential is logged into as that workspace, and needs
`--workspace`: accounts differ between workspaces, so there is no single
session to copy, and devmachine will not guess one for you.

A machine credential is logged into once, as the machine's admin, and
what the tool wrote is copied into `/etc/devmachine/<name>/`. The next
`devmachine sync` is what spreads it out to the workspaces that use that
package.

A `kind: secret` credential is refused: nobody logs into a plain value.
Use `devmachine secrets set`, then `devmachine credentials push`.

The real `ssh` opens the session, but with the same strict fingerprint
check as every other command. It never learns a key on the fly. See
[SSH host keys](../how-it-works/ssh-host-keys.md).

## credentials

```
devmachine credentials list [--machine m]
devmachine credentials push [--machine m] [--check] [--yes]
```

Shows what the packages on a machine and its workspaces need to log in or
authenticate, and what is missing.

Every row names the command that fixes it: a missing login says
`devmachine login <name>`, and a missing secret says `devmachine secrets
set <name>`, then `devmachine credentials push`. A secret you already
stored only asks for the push.

A row reads `unknown` when the package never said where its tool keeps
the result. There is nowhere to look, and "I cannot tell" is not the same
as "it is not there".

The report never prints a value, in either format.

`push` delivers the values a machine is missing, and only those. A login
is skipped — nobody can push a browser session — and a secret you never
stored a value for is named, along with the `secrets set` that fixes it,
because a push that quietly does nothing would defeat the point of this
command. It exits non-zero when it found one.

A value is read from the secret named after the credential. A workspace
that needs its own value stores it as `<workspace>/<name>`, which wins
over the shared one.

`--check` says what it would write and writes nothing. A value is never
printed, in either format, and the command log records the push without
it.

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

Manages what is installed on your machines and workspaces. See
[the package format](package-format.md) for how a package is put
together.

`list` reads the packages and the configuration together, so each one
appears once with every machine and workspace that uses it. A name a
target asks for that nothing provides is listed as `missing`, instead of
being left for `sync` to discover on its own.

`add` and `rm` only edit `config.yml` — `devmachine sync` is what applies
the change. Pass one of `--machine` or `--workspace`; with neither, and
one configured machine, that machine is the target. Adding a package a
target already has changes nothing and says so.

Comments in `config.yml` survive: only the package lists and the pin are
rewritten, everything else stays exactly as you wrote it.

`new` writes a package that already passes `validate` and already
installs something. It refuses to overwrite one that is already there.

`validate` reports every problem at once, each with its file and line,
and exits 1 if it found any.

`schema` prints the `package.yml` format this binary reads — the answer
that cannot drift, because the validator enforces it. The whole format is
in [the package format](package-format.md).

`pin` writes `packages: <release>` to `config.yml`: which release of the
packages repository every package is read from. With no release given,
it pins the latest one. `setup` pins the latest when it writes a new
configuration, so `pin` is for moving to a newer release, or for a
configuration written by hand. A release is a tag such as `v8`, never a
branch.

`help` asks an installed package what it accepts, by running its own
`help` and printing the answer — a table by default, the raw JSON under
`--json`. It refuses a package with no `entrypoint`: a package that
cannot be called has nothing to ask.

## sync

```
devmachine sync [--machine m] [--check] [--yes] [--tags a,b]
```

Applies your configuration to a machine — installs what is missing,
updates what changed.

In order: it reads and checks the configuration, makes the pinned release
available (fetching and verifying it the first time), works out what the
machine and each of its workspaces should get and in what order, checks
your own packages, prints the plan, asks for confirmation, then sends
everything to the machine and runs Ansible **there**, streaming the
output as it happens.

| Flag | Meaning |
| --- | --- |
| `--check` | a dry run: the machine reports what would change and changes nothing |
| `--yes` | apply without asking |
| `--tags a,b` | only the packages named, by name |

There is one tag beyond the package names: `credentials`, which only
copies shared logins. `devmachine sync --tags credentials` is what to run
after `devmachine login`, instead of syncing the whole machine again.

`--check` never asks, and never writes the lock file: a dry run that
recorded itself as applied would make the lock claim something that
never happened.

Only your own packages are checked before the run. A published one was
already checked when it was released; one in `<config>/packages/` has
never been checked by anyone else.

With `--format json`, the document on stdout is the result; the plan, the
prompt and the machine's own output all go to stderr.

On success, `<config>/packages.lock` records what was applied to that
machine and its workspaces, at which release and checksum. Only the
machine that was synced is rewritten — syncing one machine says nothing
about another.

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

The time is UTC, then the workspace or machine the command ran against,
then whether it succeeded, then the command itself. The command is
quoted, so one with a newline in it stays on one line.

The CLI only writes to this file. Read it with `tail` and `grep`, rotate
it with `logrotate`, delete it whenever you like — see
[configuration](../concepts/configuration.md#the-command-log) for what it
is not.

**`setup` and `machines add` are not logged**, on purpose. They are the
two commands that handle a root password, and a log is the last place
that should ever come near one. They also run once per machine, so there
is little to look back at.

`login` is not logged either: what it runs is a command the package
declared, in a terminal you are watching yourself.
