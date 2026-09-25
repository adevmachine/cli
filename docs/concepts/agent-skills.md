# Agent Skills

Agent Skills give Claude, Codex, Pi and OpenCode operational knowledge without
turning that knowledge into a Claude-only plugin or duplicating it for every
harness.

## One canonical copy

On each account Devmachine installs the complete skill directories under:

```text
~/.agents/skills/<skill>/
```

Codex, Pi and OpenCode consume that shared location. Claude receives a relative
compatibility link:

```text
~/.claude/skills/<skill> -> ../../.agents/skills/<skill>
```

Scripts, references and assets travel with `SKILL.md`. Ownership records let
Devmachine update its own paths while refusing to overwrite an unmanaged skill
or one owned by another package.

## Skills on this computer

```bash
devmachine skills add
devmachine skills add --package global-skills
devmachine skills list
devmachine skills update
devmachine skills remove <skill>
```

Bare `add` installs the official `devmachine-skills` package from the package
release pinned by the effective configuration. `--package` selects an
operator-owned local package. These commands only read and write this computer;
they never connect to a configured machine.

## Skills in a workspace

A workspace receives a contribution only by selecting its package:

```bash
devmachine packages add devmachine-skills --workspace alice
devmachine sync --check --tags devmachine-skills
devmachine sync --tags devmachine-skills
```

The configuration edit is local. Both `sync` commands contact the selected
machine; obtain approval before running either against a real target.

`sync` installs the canonical tree only in workspaces selecting the package. A
Claude link is added only when that workspace also selects `claude-code`.
Removing a package from `config.yml` is additive and does not erase remote
files.

## Contributing skills from a package

An ordinary workspace package may add:

```yaml
skills:
  path: skills
```

Every direct child of that directory is a complete skill containing a valid
`SKILL.md`. The path must stay inside the package. The role may still contain
tasks, defaults, files and every other normal Ansible role component; the CLI
generates only the repetitive canonical-copy and harness-adapter work.
