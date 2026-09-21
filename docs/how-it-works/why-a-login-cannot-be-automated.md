# Why a login cannot be automated

It is the first question everybody asks, so here is the answer before you have
to go looking.

## Because a person is the point

`gh auth login` opens a browser, or prints a device code and waits. `claude
/login` does the same. The step in the middle is a human proving they are
themselves, to a service that deliberately will not accept a script doing it.

A CLI that automated this would either be storing your password — which is what
the whole flow exists to avoid — or holding a token it obtained some other way,
which is the same problem with an extra step.

So `devmachine login` does not try. What it does is remove every part that is
not the person:

- It knows **which command** to run, because the package declared it.
- It knows **where** it has to happen — as that workspace's account, on that
  machine, over a real terminal so a browser prompt or a device code actually
  reaches you.
- It knows **what to do with the result**: a machine-scoped login is copied to
  `/etc/devmachine/<name>/` and spread from there.

You sit through one login. The CLI handles the other nine.

## A real terminal, not a captured one

`login` execs the system `ssh` with a TTY, the same path `devmachine ssh` takes.
A device code you cannot read is a device code that expires.

**One consequence worth knowing:** that path reads `~/.ssh/known_hosts`, and the
Go client the rest of the CLI uses does not. So the very first `devmachine
login` against a machine `ssh` has never seen can fail with `Host key
verification failed`, while `doctor`, `run` and `sync` were working perfectly a
moment earlier. Connect once with `devmachine ssh` and accept the key, or add it
yourself.

## `stored_at` is a claim, not a guarantee

A package says where its tool keeps the result:

```yaml
stored_at: ~/.claude/.credentials.json
```

That is how `doctor` and `credentials list` can look, and how a shared login
knows what to copy. It is a claim by whoever wrote the package, checked against
one version of one tool.

A tool that moves its session file breaks it, and the failure is quiet in the
wrong direction: the CLI reports missing what is in fact present. You log in
again, nothing changes, and the row stays red.

It is worth having anyway — the alternative is no check at all — and it is worth
knowing the shape of the lie before you meet it. A credential with no
`stored_at` is reported as **unknown** rather than missing, because "I cannot
tell" and "it is not there" are different claims.
