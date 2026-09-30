# How an upload lands

`devmachine upload` exists so an app or an agent can hand a file to a
workspace and get back a path it can use. Each rule below serves that.

## It runs as the account the file is for

With `--workspace`, the CLI logs in as the workspace's own account, the
same way `run --workspace` does. Every folder and file it makes is that
account's from the start. Root never writes into a workspace's home, so
there is nothing to hand over with a `chown`, and nothing root could be
tricked into writing somewhere else.

## A new name every time, never an overwrite

The time goes before the extension (`report-20260930-143012.pdf`), so a
second upload of the same file does not replace the first, and the file
still opens with the right program. When two uploads land in the same
second, the second gets `-2`.

The file arrives under a temporary name and is then linked to its final
name. A link fails when the name is taken, instead of replacing what is
there, so even two uploads at the same moment cannot overwrite each
other. A half-sent file never shows up under its final name.

## It stays inside the home

`--dir` is checked twice. On your computer, a folder that climbs out
with `../` is refused before anything connects. On the machine, the
nearest existing folder on the way is resolved, and if it leads out of
the home through a symbolic link, the upload stops before it writes. A
link that stays inside the home is fine.

## A file name is never a command

The name of a file is whatever its owner typed, and a name like
`'; rm -rf ~; $(id).txt` is legal. The CLI sends names to the machine
encoded in base64, and the machine decodes them into variables. The
name is never part of a command line, so nothing in it can run.

The file's content travels on standard input, over the same SSH
connection `run` keeps open, so many uploads in a row share one login.
