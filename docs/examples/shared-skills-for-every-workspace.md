# One set of skills for every workspace

Write a skill once and every workspace that uses your team's package gets
it. Change the skill, sync, and the change lands everywhere — no copying
files into each workspace by hand, and no drift between one workspace's
copy and another's.

This example builds a package called `team-skills` with two skills,
`write-tests` and `deploy-preview`, and puts it on two workspaces, `acme`
and `globex`. It also installs the same skills on your own computer, so
your coding agent there knows them too.

**You need:** a machine already set up, and a [getting
started](../getting-started.md) run through once.

## Before you start: machine, skills, workspace

```
brew install mydevmachine/tap/devmachine
devmachine setup
devmachine skills add
devmachine workspaces new acme
devmachine sync
```

Already have a machine? Skip `setup`. Already have the workspace? Skip
the last two. See [getting started](../getting-started.md) for what each
command does.

## By hand

### 1. Create the package

```
devmachine packages new team-skills --scope workspace --into ~/.config/devmachine/packages
```

`~/.config/devmachine` is your configuration folder — run `devmachine
config path` if you are not sure yours is there. `--scope workspace`
because skills are something each workspace gets, not the machine
itself. This writes a starting `package.yml` at
`~/.config/devmachine/packages/team-skills/package.yml`.

### 2. Tell the package to ship skills

Open `package.yml` and add the `skills` field:

```yaml
format: 1
name: team-skills
scope: workspace
category: Coding agents
summary: Our team's shared agent skills.
skills:
  path: skills
```

`path: skills` says every folder directly inside `skills/` is a complete
Agent Skill.

### 3. Write the skills

```
mkdir -p ~/.config/devmachine/packages/team-skills/skills/write-tests
mkdir -p ~/.config/devmachine/packages/team-skills/skills/deploy-preview
```

`~/.config/devmachine/packages/team-skills/skills/write-tests/SKILL.md`:

```markdown
---
name: write-tests
description: Use when adding a new feature or fixing a bug in this project, before opening a PR, to write the tests that cover it. Follow the project's existing test style and put the file next to the code it tests.
---

# Write tests

1. Find the existing tests closest to the code you changed. Match their
   file name pattern and test framework.
2. Write one test for the normal case and one for the edge case that
   would break if the fix regressed.
3. Run the test suite and confirm the new tests fail without your change
   and pass with it.
4. Do not commit until the whole suite is green.
```

`~/.config/devmachine/packages/team-skills/skills/deploy-preview/SKILL.md`:

```markdown
---
name: deploy-preview
description: Use when a change is ready for someone else to look at, to publish a preview build they can open in a browser, instead of asking them to check out the branch.
---

# Deploy a preview

1. Build the project in preview mode.
2. Publish the build to the team's preview host.
3. Post the preview URL where the reviewer will see it.
4. Say what changed since the last preview, in one or two lines.
```

Each `SKILL.md` needs frontmatter with `name` (matching the folder) and
a `description` that says when to use it — that is what your agent reads
to decide to pick the skill up.

### 4. Validate it

```
devmachine packages validate ~/.config/devmachine/packages/team-skills
```

Fix anything it reports before moving on — it checks every problem at
once, not just the first.

### 5. Add the package to both workspaces

```
devmachine packages add team-skills --workspace acme
devmachine workspaces new globex
devmachine packages add team-skills --workspace globex
```

### 6. Sync

```
devmachine sync
```

This sends `team-skills` to both workspaces. A workspace that also has
`claude-code` gets a Claude link to the same files; Codex, Pi, and
OpenCode read them directly.

### 7. Install it on your own computer too

```
devmachine skills add --package team-skills
```

Now your own coding agent knows `write-tests` and `deploy-preview` as
well, without a machine involved.

### 8. Change a skill, then sync again

Edit
`~/.config/devmachine/packages/team-skills/skills/write-tests/SKILL.md` —
say, add a step about coverage. Then:

```
devmachine sync
devmachine skills update
```

`sync` pushes the new version to `acme` and `globex`. `skills update`
refreshes your own computer's copy. One edit, three places updated.

## With your agent

In a Claude Code, Codex, or Pi session on your own computer
(`devmachine skills add` already done there):

```text
Create a devmachine package called team-skills, scope workspace, in my
configuration folder. Give it two skills: write-tests (use it before
opening a PR to write tests for the change) and deploy-preview (use it
to publish a preview build and share the URL). Validate the package,
then add it to the acme and globex workspaces and sync.
```

The agent runs the same commands as above and writes the two `SKILL.md`
files for you — check the wording of `description` matches what you
actually want each skill used for. It asks before `devmachine sync`
applies anything to a real machine; approve it once you've seen the
plan.

**Check it:** `devmachine skills list` on your computer shows
`write-tests` and `deploy-preview` from `team-skills`. On the server,
`devmachine ssh acme` then `ls ~/.agents/skills/` shows the same two
folders, and so does `devmachine ssh globex`.
