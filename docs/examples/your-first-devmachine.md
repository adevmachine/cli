# Your first devmachine, explained

The first commands, one at a time: what each asks and what it does. At the
end you have a workspace called `alice` on your server.

**You need:** a Debian or Ubuntu VPS you can reach as root over SSH, and a
Mac or Linux computer.

## By hand

### 1. Install the CLI

```
brew install mydevmachine/tap/devmachine
```

On Linux without Homebrew, get the binary from the
[releases page](https://github.com/mydevmachine/devmachine/releases).

### 2. Connect the server

```
devmachine setup
```

It asks for the server's address and login, then shows its **fingerprint**.
Check it against your provider's dashboard before you say yes: that is how
you know it is your server. Then it installs a key, proves the key works,
and turns password logins off. See
[how setup locks the server](../how-it-works/trust-bootstrap.md).

### 3. Teach your agent the CLI

```
devmachine skills add
```

Any new Claude Code or Codex session can now run devmachine for you.

### 4. Add the essentials and a workspace

```
devmachine packages add essentials
devmachine workspaces new alice
devmachine sync
```

`essentials` gives the server base tools, git, a firewall and Caddy.
`workspaces new` adds `alice` to your configuration. `sync` builds both on the
server: `alice` is its own account, with git, the GitHub CLI, Node, bun and
zsh. It shows the plan and asks before changing anything.

### 5. Open it

```
devmachine ssh alice
```

You land inside tmux, so what you start keeps running after you close the
terminal. See [keep your sessions running](keep-sessions-running.md).

## With your agent

Install the CLI (step 1), then say to Claude Code or Codex on your computer:

```text
Set up devmachine for me with a workspace called alice:
read https://mydevmachine.sh/agent-setup.md and follow it.
```

It asks for what it needs, one question at a time. You still check the
fingerprint in `devmachine setup`, and you approve the `sync`.

**Check it:** `devmachine doctor` passes, and `devmachine ssh alice` opens a
shell on the server.
