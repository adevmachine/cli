# Choosing a target

## A command never guesses which machine

With one machine set up, a command uses it. With several, a command that
acts on a server needs `--machine`:

```
$ devmachine doctor
fail  configuration  several machines are configured (main, sandbox): say which one with --machine
skip  connection     there is no machine to check
```

There is no `default_machine` setting, on purpose. These commands create
accounts, rewrite a proxy's config, and update whole servers. A command
like that landing on a machine nobody named is how the wrong server gets
wrecked — and a default set in a file you edited months ago is exactly how
that happens.

Typing `--machine` is cheap. Explaining what happened to production is
not.

## Workspaces name themselves

A command that acts on a workspace takes it as its first argument:

```
devmachine ssh bob
devmachine mosh bob
devmachine run --workspace bob "git status"
```

No `--machine` is needed or accepted there — the workspace already says
where it lives. `run` uses a flag only because its first argument is the
command to run.

## Skipping, not failing

When a check cannot run because an earlier one failed, devmachine reports
it as `skip` with the reason, never as a failure.

A machine that is unreachable would otherwise fail every remote check, and
the one line that matters — the connection — would get lost in a wall of
red.
