# Day to day

Once your server is set up, you rarely type devmachine commands yourself.
You ask your coding agent, from whatever you are working on.

## Teach your agent once

```
devmachine skills add
```

This installs the devmachine skills for Claude Code, Codex and the other
agents on your computer. From then on, any new session knows the CLI, in any
folder. See [agent skills](concepts/agent-skills.md).

## Ask from where you work

The CLI always finds your configuration, whatever folder a session was
opened in. So from a session in your project you can ask:

- "Publish port 3000 of the alice workspace at app.example.com."
- "Add Docker to alice and sync."
- "Why did the last sync fail?"
- "Open a tunnel to port 5432 on alice."

There is no need to switch folders or open another session.

## Open a session in the configuration folder to change devmachine itself

Your configuration is a folder on your computer. Find it with:

```
$ devmachine config path
/Users/alice/.config/devmachine (from default)
```

Open a coding agent there when the configuration itself is the work:

- **Writing or changing a package of your own.** Its files live in
  `packages/`, and the agent can read, test and commit them.
- **A change across many machines or workspaces**, or a review of the whole
  setup.
- **Reading the history.** With [git set up](how-it-works/versioning-your-configuration.md),
  every change the CLI makes is a commit.
- **Your own rules.** A `CLAUDE.md` or `AGENTS.md` in this folder loads only
  in a session opened here. Put in it what your agent must know about your
  setup: which machine is production, what to ask before touching it, notes
  about your network.

In short: to use devmachine, ask from where you are. To change devmachine,
open a session in its folder.

## What a session on the server can do

Your configuration and the CLI live on your computer, not on the server. A
coding agent running inside a workspace can build, test and run your app, but
it cannot publish a port or add a package. Ask for those from a session on
your computer.
