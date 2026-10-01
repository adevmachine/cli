# Updating

`devmachine update` does five things in a row. Most of what it does is plain;
these are the parts that are not.

## It stops before your machines

Everything before the last step changes only your computer: the CLI, one line
in `config.yml`, the skills in your home folder. The last step changes
servers, and servers run things people are using. So `update` runs `sync
--check` first, shows what would change, and asks one question.

No is the default. An empty answer, closed input, or no terminal at all
(a cron job, a pipe) is also no, and `update` prints the exact command to
run when you are ready. A machine never changes on an answer nobody gave.
`--yes` exists for automation you already trust to change servers.

## The new CLI runs the rest

When the CLI itself is out of date, `update` replaces it and then starts the
new binary to finish the job. The old process is still running the old code
in memory; if it went on, steps 2 to 5 would run with whatever bugs the new
release fixed. The new binary gets the same command line plus one hidden flag
naming the version it replaced, so it skips step 1 and reports `0.7.17 →
0.7.18`. If the "new" binary turns out to be the same version, that is
reported as a failure, not as success.

## Only the CLI, when that is all you asked for

`update --cli-only` is step 1 and nothing more. It is what the Devmachine
app runs behind its "Update CLI" button: the app replaces a program on your
computer for you, but moving the packages pin or syncing a server stays
something you start yourself. So `--cli-only` reads no configuration and
refuses `--yes`, `--skip-…` and `--machine` instead of ignoring them. The
new binary is still started after the swap, with `--cli-only`, only to
confirm the version changed.

## Homebrew's binary goes through Homebrew

A binary Homebrew installed (the real file lives in Homebrew's `Cellar`,
under `/opt/homebrew`, `/usr/local` or whatever `brew --prefix` says) is
upgraded with `brew upgrade`. Writing a new file over it by hand would
work once, and then Homebrew would believe in a version that is not there.
`brew update` runs first so the tap knows about the new release, and
`brew trust --formula` runs when your Homebrew has it — newer Homebrew
refuses a third-party tap without it. The same steps `install.sh` takes.

Any other binary is replaced from the release archive. The archive is checked
against the release's `checksums.txt` before anything is written; a mismatch
stops the update and leaves the old binary alone. The new file is written
next to the old one and renamed over it, so there is never a moment with a
half-written `devmachine` on your `PATH`.

## Packages before skills

The skills your coding agent reads come from the packages release
`config.yml` pins. Pinning first, then refreshing the skills, means the skills
match the release your machines are about to get, not the one they are
leaving.

## A pin moves only forward

`update` pins the newest packages release only when yours is older. A pin
that is already the newest, or newer (a release you are testing), stays
where it is. A configuration with no pin stays without one: that is a choice
to use only your own packages, and `update` does not undo it.

## An unreachable machine is not a reason to stop

`doctor` runs on every machine. Warnings and failed checks are printed and
counted, and the run goes on — one broken credential should not stop the
other machines from being checked. A machine `doctor` could not reach
(configuration, host key or connection failed) skips its sync check, since
there is nothing to compare against, and the summary lists it as `skipped
(unreachable)`.

What `doctor` finds never changes `update`'s exit code. `update`'s job is
its own steps: the CLI, the pin, the skills and the sync. If a missing login
on one machine made `update` exit non-zero, a script could not tell "the
update failed" from "the update worked and a login is missing". The summary
line says what doctor found (`doctor  2 warning(s)`, `doctor  machine far
unreachable`), and `devmachine doctor` keeps its own exit code for scripts
that care.

## Warn or fail

`doctor` fails a check only when the machine is unusable: the configuration
is invalid, the host key is unknown or changed, the machine cannot be
reached, or an essential is missing (a supported operating system, Ansible,
and on a self machine a bundle folder `sync` can write). Each of those stops
`sync` itself.

A missing login or secret, and a DNS provider whose token stopped working, is
a `warn`. The machine still works; only the tool that needs the login does
not, and the detail says the command that fixes it.

## Old is a warning, and offline is a skip

`doctor` also says when the CLI or the packages pin is behind. That is a
`warn`, never a `fail`: an old CLI still works, and a script that runs
`doctor` should not start failing the day a release comes out. When GitHub
cannot be asked, both checks `skip` — being offline says nothing about your
versions.

GitHub allows 60 unauthenticated API requests an hour, and `doctor` runs
often, so its answers are kept for 6 hours in your cache folder. `update`
always asks GitHub again, and stores what it hears, so `doctor` is current
right after an update.
