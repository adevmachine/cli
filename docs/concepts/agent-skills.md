# Agent Skills

Write your agent skills once, in a package in your configuration folder.
Every workspace that uses the package gets the same skills on the next
`devmachine sync` — Claude Code, Codex, Pi, and OpenCode all read them.
Your own computer gets them with `devmachine skills add --package
<name>`. Edit one skill, sync again, and the change reaches every
workspace and your computer alike, from one place.

A skill is a folder of know-how your coding agent can read. Add the
official set — devmachine's own skills for operating the CLI — with:

```
devmachine skills add
```

See [shared skills for every workspace](../examples/shared-skills-for-every-workspace.md)
for a full walkthrough with a package of your own.

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
devmachine skills add --package team-skills   # skills from your own package
devmachine skills list
devmachine skills update
devmachine skills remove <skill>
```

`--package` installs skills from one of your own packages instead of the
official set. None of these commands touch a machine.

## Skills in a workspace

A workspace gets skills only when you add the package to it, and run
`sync`:

```bash
devmachine packages add team-skills --workspace acme
devmachine sync --tags team-skills
```

The Claude link is added only for a workspace that also has
`claude-code`. Removing the package from your configuration doesn't
delete files already on the machine.

## Writing your own package of skills

A package can add:

```yaml
skills:
  path: skills
```

Every folder directly inside `skills/` must be a complete skill, with
its own `SKILL.md`. See
[the package format](../reference/package-format.md) for the rest of
what a package can declare.
