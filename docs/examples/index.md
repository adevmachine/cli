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
- [OpenClaw, in its own sandbox](openclaw-in-its-own-sandbox.md) — an
  autonomous agent, always on, in a sandbox of its own.
- [Keep your sessions running](keep-sessions-running.md) — close the laptop,
  and Claude Code keeps working on the server.
- [A Claude that never sleeps](claude-that-never-sleeps.md) — `/rc` by hand,
  then drive the session from your phone or Claude Desktop.
- [One consultant, three startups](one-consultant-three-startups.md) — a
  sandbox per client, each with its own stack and its own logins.
- [Hermes Agent, in its own sandbox](hermes-agent-in-its-own-sandbox.md) —
  Nous Research's agent, always on, in a sandbox of its own.
- [Agno AgentOS with a control plane](agno-agentos-with-control-plane.md) —
  an agent API and its dashboard, each on its own subdomain.
- [wuzapi as your own package](wuzapi-as-your-own-package.md) — a WhatsApp API
  written once as a package, running on two machines.
- [Log in with your 1Password SSH key](log-in-with-1password.md) — keep the
  key in your vault, and approve each use with Touch ID.
- [Pi as your coding agent](pi-coding-agent.md) — install Pi in a workspace
  and use it for everyday work.
- [SSH or mosh?](ssh-or-mosh.md) — which one to use, and what to install
  where.
- [Claude 24/7 in Telegram](claude-24-7-in-telegram.md) — message Claude
  Code from Telegram, while it runs on your server around the clock.
- [Logins and secrets](logins-and-secrets.md) — see what is missing, sign in,
  and hand over tokens with `credentials`, `login` and `secrets`.
- [Point your domain with Cloudflare](cloudflare-dns.md) — a scoped API token,
  and devmachine points your names for you.
- [Contribute to a Ruby on Rails project](ruby-on-rails.md) — a Rails
  workspace for an open source gem, from fork to running tests.
- [Your own Tailscale with Headscale](headscale.md) — a private network with
  a control server you host yourself.
