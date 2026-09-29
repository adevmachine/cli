# Your first devmachine, explained

This walks through the six commands on the [home page](../getting-started.md)
one at a time, slower, so you know what each one asks and what you see on
screen. If you already ran them and it worked, you do not need this page.

**You need:** a Debian or Ubuntu VPS you can reach as root over SSH, and a
Mac or Linux computer.

## By hand

### 1. Install the CLI

```
brew install mydevmachine/tap/devmachine
```

On Linux without Homebrew, get the binary from the
[releases page](https://github.com/mydevmachine/devmachine/releases) instead.
Nothing on the server changes yet — this only puts `devmachine` on your own
computer.

### 2. `devmachine setup`

```
devmachine setup
```

It asks for your server's address, the admin account (usually `root`), the
port, and a domain (empty is fine). Then it shows a code called the
**fingerprint** and asks if it matches what your provider's dashboard shows
for that server. Check it before you say yes — that is the only way
`devmachine` knows it is talking to your server and not someone else's.

Say yes, and it does four things in order: makes a key and installs it on the
server, opens a brand new connection using only that key to prove it works,
turns password logins off, and installs Ansible. If the proof step fails,
nothing is locked down, and the error tells you where to look. See
[setting up a server for the first time](../how-it-works/trust-bootstrap.md)
for why the order matters.

### 3. `devmachine skills add`

```
devmachine skills add
```

Teaches your coding agent (Claude Code, Codex, or another) the whole
`devmachine` CLI, so it can run these commands for you correctly. It only
takes effect in a new agent session, so run it once, early. You can run it
before `setup` too.

### 4. `devmachine workspaces new alice`

```
devmachine workspaces new alice
```

Creates a workspace called `alice` in your configuration. Nothing happens on
the server yet — this only writes to a file on your own computer. A
workspace is its own Linux account with its own tools, files, and logins;
see [machines and workspaces](../concepts/machines-and-workspaces.md).

### 5. `devmachine sync`

```
devmachine sync
```

Builds the workspace on the server: creates the Linux account and installs
its default packages — git, the GitHub CLI, Node LTS, bun, and zsh with Oh My
Zsh. It shows the plan first and asks before it changes anything. The first
run takes a few minutes.

### 6. `devmachine ssh alice`

```
devmachine ssh alice
```

Opens a real SSH session as `alice`. It lands inside a tmux session, so if
your connection drops or you close the terminal, whatever you started keeps
running — `devmachine ssh alice` again puts you right back. See
[keep your sessions running](keep-sessions-running.md).

## With your agent

Open a session on your own computer, with any coding agent, and say:

```text
Set up devmachine for me: read https://mydevmachine.sh/agent-setup.md and
follow it.
```

The agent installs the CLI, runs `devmachine skills add`, then walks through
the same steps above, asking you one question at a time. You still do the
parts only you can do: confirming the fingerprint against your provider's
dashboard, typing the server's root password if it needs one, approving
`devmachine sync` once the agent shows you the plan, and any browser or
device sign-in such as `devmachine login gh`. See
[set up with a coding agent](../agent-setup.md) for the full page the agent
follows.

**Check it:** `devmachine doctor` reports the machine healthy, and
`devmachine ssh alice` drops you into a working shell.

Source: [getting started](../getting-started.md),
[set up with a coding agent](../agent-setup.md),
[setting up a server for the first time](../how-it-works/trust-bootstrap.md)
