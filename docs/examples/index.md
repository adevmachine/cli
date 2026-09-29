# Examples

Real setups, done in a few steps. Each one links to a concept page for the
why; here it is only the commands.

**Whatever you start keeps running.** Every login to a workspace opens inside
a tmux session. Start an app or an agent, close the terminal, and it keeps
running on the server. `devmachine ssh alice` puts you back in the same
session. Only a reboot stops it.

- [Your first devmachine, explained](your-first-devmachine.md) — the first
  commands done slowly, with what each one asks and does.
- [Try it on your own computer first](a-local-vm-with-lima.md) — a local virtual machine with Lima, used exactly like a VPS.
- [Express site with TLS](express-site-with-tls.md) — an Express app, live at
  your own domain with HTTPS.
- [Docker site on 8080](docker-site-on-8080.md) — a container, live at your
  own domain with HTTPS.
- [Claude Code, controlled from your phone](claude-code-remote-control.md) —
  start a session on your server, drive it from the Claude app.
- [Start Claude from your phone](start-claude-from-your-phone.md) — a session
  that stays up on the server, so you never have to SSH in first.
- [An agent in its own workspace](an-agent-in-its-own-workspace.md) — an
  autonomous agent, kept in its own account.
- [Keep your sessions running](keep-sessions-running.md) — close the laptop,
  and Claude Code keeps working on the server.
- [A Claude that never sleeps](claude-that-never-sleeps.md) — `/rc` by hand,
  then drive the session from your phone or Claude Desktop.
- [One consultant, three startups](one-consultant-three-startups.md) — a
  workspace per client, each with its own stack and its own logins.
