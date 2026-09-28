# Agent Skills

Agent Skills give coding agents — Claude, Codex, Pi, OpenCode — reusable
know-how, like a shared playbook. Devmachine installs one copy and lets
every agent read it, instead of duplicating it per tool.

## One shared copy

On each account, Devmachine installs the full skill folders under:

```text
~/.agents/skills/<skill>/
```

Codex, Pi, and OpenCode read that shared location directly. Claude gets a
link that points to it:

```text
~/.claude/skills/<skill> -> ../../.agents/skills/<skill>
```

Scripts, references, and assets travel with each skill's `SKILL.md`.
Devmachine tracks what it installed, so an update never overwrites a
skill it doesn't own.

## Skills on your computer

```bash
devmachine skills add
devmachine skills add --package global-skills
devmachine skills list
devmachine skills update
devmachine skills remove <skill>
```

Bare `add` installs the official `devmachine-skills` package, from the
package release your configuration is pinned to — or, before you've run
`devmachine setup`, the latest one. `--package` installs one of your own
local packages instead. None of these commands touch a machine.

## Skills in a workspace

A workspace gets a skill only by selecting its package:

```bash
devmachine packages add devmachine-skills --workspace alice
devmachine sync --check --tags devmachine-skills
devmachine sync --tags devmachine-skills
```

The configuration edit is local, but both `sync` commands reach the
machine — get approval before running either for real. `sync` installs
the shared skill tree only in workspaces that selected the package, and
adds the Claude link only when that workspace also has `claude-code`.
Removing the package from `config.yml` doesn't delete files already on
the machine.

## Adding skills from a package

An ordinary workspace package can add:

```yaml
skills:
  path: skills
```

Every direct subfolder of that path must be a complete skill with a
valid `SKILL.md`, and must stay inside the package. The package can still
hold its own tasks, files, and everything else a normal Ansible role has
— the CLI only handles the repeated work of copying the skill and adding
the Claude link.
