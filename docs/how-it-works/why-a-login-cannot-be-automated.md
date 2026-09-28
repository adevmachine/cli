# Why a login cannot be automated

It is the first question everybody asks, so here is the answer before you
go looking.

## Because a person is the point

`gh auth login` opens a browser, or prints a device code and waits.
`claude /login` does the same. The step in the middle is a human proving
they are themselves, to a service that will not accept a script doing it
for them. A CLI that automated this would either store your password —
what the whole flow exists to avoid — or hold a token it got some other
way, the same problem with an extra step.

So `devmachine login` does not try. It knows **which command** to run,
because the package declared it; **where** it has to happen — that
workspace's account, that machine, a real terminal so a prompt or code
actually reaches you; and **what to do with the result**: a
machine-scoped login is copied to `/etc/devmachine/<name>/` and spread
from there. You sit through one login; the CLI handles the rest.

## A real terminal, not a captured one

`login` runs the system `ssh` with a TTY, the same path `devmachine ssh`
takes — a device code you cannot read is one that expires.

**One consequence worth knowing:** that path reads `~/.ssh/known_hosts`,
which the Go client the rest of the CLI uses does not. So the first
`devmachine login` against a machine `ssh` has never seen can fail with
`Host key verification failed`, while `doctor` and `sync` worked fine a
moment earlier. Connect once with `devmachine ssh` and accept the key, or
add it yourself.

## `stored_at` is a claim, not a guarantee

A package says where its tool keeps the result:

```yaml
stored_at: ~/.claude/.credentials.json
```

That is how `doctor` and `credentials list` can check, and how a shared
login knows what to copy. It is a claim by whoever wrote the package,
checked against one version of one tool.

A tool that moves its session file breaks the claim in the wrong
direction: the CLI reports missing what is in fact present, and logging
in again changes nothing. Worth having anyway — the alternative is no
check at all — and worth knowing before you meet it. A credential with no
`stored_at` reports **unknown** rather than missing, because "I cannot
tell" and "it is not there" are different claims.
