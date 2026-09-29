# devmachine documentation

Set up and run your own development server.

Three commands turn a server nobody has logged into into one that is yours:

```
devmachine setup      connect to the server, lock it down, get it ready
devmachine sync       apply your packages to the server
devmachine workspaces new alice && devmachine sync
```

No server yet? `devmachine machines create-local dev` makes one on your own
computer, while you wait for a real one.

## Start here

- [Getting started](getting-started.md) — install devmachine, connect it to a
  server, and see that it works.
- [Set up with a coding agent](agent-setup.md) — hand this page to your coding
  agent. It asks what it needs and does the setup, except the steps only you
  can do.
- [Day to day](day-to-day.md) — ask your agent from any session, and when to
  open one in the configuration folder instead.

## Examples

Real setups, done in a few steps. Start at [the list](examples/index.md).

- [Your first devmachine, explained](examples/your-first-devmachine.md)
- [Try it on your own computer first](examples/a-local-vm-with-lima.md)
- [Express site with TLS](examples/express-site-with-tls.md)
- [Docker site on 8080](examples/docker-site-on-8080.md)
- [Claude Code, controlled from your phone](examples/claude-code-remote-control.md)
- [Start Claude from your phone](examples/start-claude-from-your-phone.md)
- [An agent in its own workspace](examples/an-agent-in-its-own-workspace.md)
- [Keep your sessions running](examples/keep-sessions-running.md)
- [A Claude that never sleeps](examples/claude-that-never-sleeps.md)
- [One consultant, three startups](examples/one-consultant-three-startups.md)
- [Hermes Agent](examples/hermes-agent.md)
- [Agno AgentOS with a control plane](examples/agno-agentos-with-control-plane.md)
- [wuzapi as your own package](examples/wuzapi-as-your-own-package.md)

## Concepts

- [Machines and workspaces](concepts/machines-and-workspaces.md) — the two
  ideas everything else builds on.
- [Configuration](concepts/configuration.md) — where your setup lives and how
  devmachine finds it.
- [Packages](concepts/packages.md) — what a server or workspace can get
  installed on it.
- [Credentials](concepts/credentials.md) — signing in to tools and services,
  and what devmachine can do for you.
- [DNS](concepts/dns.md) — pointing a domain at your server.
- [Publishing](concepts/publishing.md) — `expose` and `tunnel`: showing
  something running in a workspace to the outside world, or just to you.
- [Reaching your server](concepts/reaching-your-server.md) — the public address,
  a private network with Tailscale, and private access to your apps.
- [Agent Skills](concepts/agent-skills.md) — teaching a coding agent to use
  devmachine.

## How it works

The reasoning behind decisions that are not obvious from the outside. Read
these when something behaves in a way that surprises you.

- [SSH and authentication](how-it-works/ssh-and-authentication.md) — why the
  CLI shares one key and never your whole agent.
- [SSH host keys](how-it-works/ssh-host-keys.md) — how devmachine checks it is
  talking to the right server, every time.
- [Addresses and fallback](how-it-works/addresses-and-fallback.md) — how a
  server with more than one address is reached.
- [Choosing a target](how-it-works/choosing-a-target.md) — why a command asks
  which server, instead of guessing.
- [Why nothing is embedded](how-it-works/why-nothing-is-embedded.md) — why
  devmachine downloads packages instead of shipping them.
- [Why a login cannot be automated](how-it-works/why-a-login-cannot-be-automated.md)
  — the first question everybody asks.
- [Sharing a login](how-it-works/sharing-a-login.md) — copying one sign-in
  into several workspaces, and keeping one workspace's login separate.
- [DNS providers](how-it-works/dns-providers.md) — what each built-in provider
  does, and its rough edges.
- [Setting up a server for the first time](how-it-works/trust-bootstrap.md) —
  how `setup` makes sure it is talking to your server, and locks it down.
- [Your computer as a machine](how-it-works/your-computer-as-a-machine.md) —
  using your own computer instead of a server.
- [Versioning your configuration](how-it-works/versioning-your-configuration.md)
  — keeping your setup in git safely.
- [Why a published site lives in the configuration](how-it-works/published-sites.md)
  — why `expose` writes to your configuration, not straight to the server.
- [What sync removes](how-it-works/what-sync-removes.md) — what `sync` cleans
  up, and what it leaves alone.

## Reference

- [Commands](reference/commands.md) — every command, its flags and its
  output.
- [The package format](reference/package-format.md) — how to write a
  `package.yml`.
- [The DNS provider contract](reference/dns-provider-contract.md) — how to
  write a DNS provider.
- [Settings](reference/settings.md) — every option the built-in packages
  accept.

## Fixing things

- [Troubleshooting](troubleshooting.md) — errors you may see, and what to do
  about them.

## Contributing

- [Development](development.md) — building devmachine and testing it against
  a real server.
- [Releasing](releasing.md) — how a new version reaches Homebrew.
