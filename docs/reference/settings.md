# Settings

Every variable the published packages accept, and what each one defaults to.

A setting is written `<package>.<name>` under a machine's or a workspace's
`settings:`, and it reaches the recipe as the variable it already reads.
**A setting is a default somebody overrode, not a new mechanism** — the recipe
cannot tell the difference and does not have to.

```yaml
machines:
  - name: main
    packages: [base, caddy]
    settings:
      base.timezone: Europe/Lisbon
      caddy.email: someone@example.com
```

Or without opening the file:

```
devmachine workspaces edit alice --set zsh.tmux_config=false
```

A setting for a package the target does not install is refused. A typo in a
package name would otherwise be silent: the value would reach nothing, the
recipe would keep its default, and the machine would not be what the
configuration says it is.

**This page is generated** by `make settings`. Do not edit it by hand.

| Setting | What it does | Default |
| --- | --- | --- |
| `base.hostname` | The machine's hostname. Empty leaves the one it already has. | *(empty)* |
| `base.timezone` | The machine's timezone, as tzdata spells it. Empty leaves whatever the machine came with. | *(empty)* |
| `base.upgrade` | Upgrade every package already installed. Off, because that is the owner's decision and not a side effect of installing base tools. | `false` |
| `caddy.email` | The address the certificate authority writes to about an expiring certificate. Empty means an anonymous account. | *(empty)* |
| `claude-code.diff_sidebar` | Open the /diff panel. Unset leaves whatever Claude Code has. It is a preference Claude Code keeps in its own state file, so it is amended only where that file, and that preference, already exist. | *(none)* |
| `claude-code.env` | Environment variables every Claude Code session runs with, as a map. It is how a model-specific or terminal-specific workaround is turned on without this package having an opinion about it. | `map[]` |
| `claude-code.expanded_todos` | Show the task list under the footer. Unset leaves whatever Claude Code has, and the same condition applies. | *(none)* |
| `claude-code.home` | Where the account's home is. | `/home/<the account>` |
| `claude-code.remote_control_at_startup` | Connect every session to Remote Control as it opens, instead of waiting for somebody to type /rc. | `false` |
| `claude-code.session_name_prefix` | What each Remote Control session is called in the phone app. Empty means the workspace's name, which is what tells two workspaces' sessions apart. | *(empty)* |
| `claude-code.status_line` | The command Claude Code runs to draw its status line. Empty leaves the status line alone. It runs outside a login shell, so give it an absolute path. | *(empty)* |
| `claude-plugins.claude` | Where the Claude Code CLI is. It is installed into the account's own ~/.local/bin, so only a workspace that put it somewhere else says so. | `<the home>/.local/bin/claude` |
| `claude-plugins.home` | Where the account's home is. | `/home/<the account>` |
| `claude-plugins.marketplace` | Where the plugins come from: a GitHub repository written owner/name, a git URL, or a path on the machine. Empty installs nothing. | *(empty)* |
| `claude-plugins.plugins` | Which plugins to install, each written <plugin>@<marketplace>. The marketplace half is the name the marketplace gives itself, which is what Claude Code installs and reports against. | `[]` |
| `claude-plugins.update` | Re-fetch the marketplace and update every plugin on each run. It is off by default because it cannot tell whether anything moved, so a run with it on always reports a change. | `false` |
| `claude-remote-control.home` | Where the account's home is. | `/home/<the account>` |
| `claude-remote-control.restart_seconds` | How long to wait before reconnecting. The server gives up on its own after about ten minutes with no network, so the unit restarts for good. | `30` |
| `claude-remote-control.session_name` | What this session is called in the phone app. Empty means the workspace's name, which is what tells two workspaces' sessions apart. | *(empty)* |
| `claude-remote-control.working_directory` | The directory a remote session starts in. | `<the home>/dev` |
| `dev.gh_version` | Which GitHub CLI release to install. It is a release asset rather than a distribution package because most distributions do not carry gh at all. | `2.63.2` |
| `dev.home` | Where the account's home is. | `/home/<the account>` |
| `dev.node_version` | Which Node mise installs globally. | `lts` |
| `fail2ban.bantime` | How long a ban lasts, in seconds. | `3600` |
| `fail2ban.ignoreip` | The addresses that are never banned, space separated. Loopback only by default: a wider range exempts everyone who shares it. | `127.0.0.1/8 ::1` |
| `fail2ban.maxretry` | How many failures from one address before it is banned. | `5` |
| `firewall.http` | Open 80 and 443. A machine that hosts nothing can turn this off. | `true` |
| `firewall.mosh_interface` | The one interface mosh's UDP range is opened on. Empty means it is not opened at all, which keeps the range off a public edge. | *(empty)* |
| `firewall.mosh_ports` | The UDP range mosh is given on that interface. | `60000:61000` |
| `glab.home` | Where the account's home is. | `/home/<the account>` |
| `glab.version` | Which GitLab CLI release to install. It is a release asset rather than a distribution package because no distribution carries glab. | `1.111.0` |
| `mise.home` | Where the account's home is. | `/home/<the account>` |
| `sentry.home` | Where the account's home is. | `/home/<the account>` |
| `ssh_hardening.service` | What systemd calls sshd. Empty means the name this distribution family uses, which is `ssh` on Debian and Ubuntu and `sshd` elsewhere. | *(empty)* |
| `workspace.admin_home` | The home of the account the CLI provisions with. Whatever reaches that account over SSH is what reaches this workspace. | `/root` |
| `workspace.generate_key` | Give the account an SSH key of its own when it has none. | `true` |
| `workspace.git_email` | The address on this workspace's commits. | *(empty)* |
| `workspace.git_name` | The name on this workspace's commits. | *(empty)* |
| `workspace.groups` | Extra Linux groups the account joins. The docker group is one of them and it is effectively root, so nobody joins it by accident. | `[]` |
| `workspace.home` | Where the account's home is. Debian and Ubuntu put it under /home; a machine whose useradd is configured otherwise says so here. | `/home/<the account>` |
| `workspace.known_hosts` | The hosts whose SSH host key is trusted in advance, so the first clone does not stop to ask a question nobody is there to answer. | `[github.com]` |
| `workspace.shell` | The login shell. Empty means whatever useradd would pick, and the package that installs a shell is the one that sets it. | *(empty)* |
| `workspace.sign_commits` | Sign every commit and rebase with the account's own SSH key. | `true` |
| `zsh.home` | Where the account's home is. | `/home/<the account>` |
| `zsh.tmux_auto_attach` | Open a tmux session on every SSH login, so a dropped connection loses nothing. A second connection while the first is live gets a session of its own instead of a second view of the same one. | `true` |
| `zsh.tmux_config` | Write the account's ~/.tmux.conf. Turn it off to keep a config of your own. | `true` |
