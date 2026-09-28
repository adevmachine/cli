# Agent Skills

A skill is a folder of know-how your coding agent can read — Claude,
Codex, Pi, or OpenCode. Add the official set with:

```
devmachine skills add
```

## One shared copy

Devmachine installs skills once, under `~/.agents/skills/<skill>/`.
Codex, Pi, and OpenCode read that folder directly. Claude gets a link
that points to it, so nothing is duplicated:

```text
~/.claude/skills/<skill> -> ../../.agents/skills/<skill>
```

## Skills on your computer

```bash
devmachine skills add                        # the official set
devmachine skills add --package global-skills
devmachine skills list
devmachine skills update
devmachine skills remove <skill>
```

`--package` installs skills from one of your own packages instead. None
of these commands touch a machine.

## Skills in a workspace

A workspace gets skills only when you add the package to it, and run
`sync`:

```bash
devmachine packages add devmachine-skills --workspace alice
devmachine sync --tags devmachine-skills
```

The Claude link is added only for a workspace that also has
`claude-code`. Removing the package from your configuration doesn't
delete files already on the machine.

## Adding skills from your own package

A package can add:

```yaml
skills:
  path: skills
```

Every folder directly inside `skills/` must be a complete skill, with
its own `SKILL.md`. See
[the package format](../reference/package-format.md) for the rest of
what a package can declare.
