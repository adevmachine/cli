# Choosing a target

## A command never guesses which machine

With one machine configured, commands use it. With several, a command that acts
on a server requires `--machine`:

```
$ devmachine doctor
fail  configuration  several machines are configured (main, sandbox): say which one with --machine
skip  connection     there is no machine to check
```

There is deliberately no `default_machine` setting. The commands coming next
create users, rewrite a proxy's configuration and converge whole servers. A
destructive command that lands on a machine nobody named is how the wrong one
gets wrecked, and a default in a file you edited months ago is exactly how that
happens.

Typing `--machine` is cheap. Explaining what happened to production is not.

## Workspaces name themselves

A command that acts on an environment takes the workspace as its first argument:

```
devmachine ssh bob
devmachine mosh bob
devmachine run --workspace bob "git status"
```

No `--machine` is needed or accepted there — the workspace already says where it
lives. `run` uses a flag only because its first argument is the command.

## Skipping, not failing

When a check cannot run because an earlier one failed, it is reported as `skip`
with the reason, never as a failure.

A machine that is unreachable would otherwise fail every remote check, and the
one line that matters — the connection — would be lost in a wall of red.
