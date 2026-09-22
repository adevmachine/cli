# Troubleshooting

## "several machines are configured: say which one with --machine"

Working as intended. With more than one machine, a command that acts on a server
will not pick for you. Add `--machine <name>`, or use a workspace name, which
already says where it lives.

## "no address answered"

Nothing is listening, or nothing can reach it. The error lists every address it
tried and what each one said.

- Is the port right? A machine on a non-standard port needs `port:` in its
  configuration.
- Does a second address exist? See
  [Addresses and fallback](how-it-works/addresses-and-fallback.md).

## "answered on … but refused the login"

The machine is there, so the network is not the problem. The account or the key
is.

- Does that account exist on that machine? A workspace configured here does not
  exist on the server until it is created there.
- Is the key authorised for that account?

## "no key in the configuration and no SSH agent"

There is nothing to authenticate with. Either point `key:` at a private key, or
start an agent and load one.

## It used to connect, and now it does not

If you recently added keys to your SSH agent, that is very likely the cause. A
server gives up after a few attempts, and an agent full of keys can use them all
before reaching the one that works.

The CLI avoids this by offering one method at a time — but `ssh` run by hand
does not, and neither does anything else on your machine. Setting `key:` on the
machine makes the CLI's behaviour immune to whatever the agent is holding.

The full explanation:
[SSH and authentication](how-it-works/ssh-and-authentication.md).

## "ansible-playbook is not on the machine"

`sync` runs Ansible on the machine, so the machine needs it. Install it there
once — `apt install ansible`, or whatever that distribution calls it — and
everything after that is `sync`'s job. Everything else in `doctor` still tells
you the truth without it.

## A `tailscale:` address is being ignored

It is dropped when `tailscale` is not installed or does not know that name, and
the next address is tried. That is deliberate.

To see what happened, run `tailscale status` and check the machine is there
under the name you wrote.

## A setting is accepted and the recipe still uses its default

A setting reaches the machine as `devmachine_<package>_<name>`, with a dash or
a dot turned into an underscore. If the recipe reads some other variable, the
value arrives and nothing looks at it.

Read what was sent, which settles it in one command:

```
devmachine run --machine main -- "cat /opt/devmachine/host_vars/devmachine.yml"
```

The variable is there, and the recipe's `defaults/main.yml` names a different
one. The recipe is what has to change: the name in its defaults is the contract
a setting overrides.

## `devmachine ssh` opens a session as the wrong user

`devmachine ssh` with no argument logs in as the machine's **administrative**
login, which is usually `root`. To land in an environment, name it:
`devmachine ssh alice`.

## Changes to config.yml appear to be ignored

Check which file is actually being read:

```
devmachine config path
```

`DEVMACHINE_CONFIG` in your shell beats the default location, and a `--config`
flag beats everything.

## `secrets list` shows nothing after storing one

Check the configuration directory is the same one used when storing it — see
above. The list of names lives in the configuration directory, even when the
values live in the keychain.

## `dns status` says a name does not resolve, but it works in the browser

The check runs from **this computer**, not from the machine, and it does not use
your browser's cache or a proxy. A name that works in the browser and not here
usually means DNS has not propagated everywhere yet, or something local — a VPN,
a `/etc/hosts` entry — is resolving it for you and not for anyone else.

## `dns add`/`dns rm`/`dns list`/`dns check` fail with one of these

Every provider reports one of a fixed set of `kind`s, and the CLI turns each
into the same error whichever provider sent it:

| Error | What it means |
| --- | --- |
| `zone not found, or the token cannot see it` | The zone does not exist under this provider, or the token cannot see it. |
| `the token was rejected` | The credential is wrong or expired. Push a fresh one with `devmachine secrets set` and `devmachine credentials push`. |
| `the token cannot change this zone` | The token can read the zone but not write to it. |
| `the record was rejected` | The registrar refused the value — a bad type, a bad value, or a name it will not accept. |
| `rate limited` | The registrar's API is throttling this token. The CLI never retries a rate limit on its own; wait and run the command again. |
| `this record type is not supported yet` | Only `A`, `AAAA`, `CNAME` and `TXT` carry one value cleanly across every provider. `MX` and `SRV` are refused by name rather than guessed at. |
| `the name holds several values` | Two installed providers both claim the zone. Say which one with `--dns-provider`. |

Two more that are not errors, but change what a command does:

- **"no installed provider holds this zone"** — none of the providers this
  machine has installed listed the zone when asked. This is the ordinary
  state for a registrar with no package yet: the command falls back to
  `manual` and prints the record to create by hand.
- **"the provider failed"**, with what looks like a traceback — this means a
  bug in the provider package itself, not in the CLI. The package answered
  something that is not the JSON the [DNS provider
  contract](reference/dns-provider-contract.md) requires.

## A certificate never arrives after `expose add`

Caddy only gets a certificate for a name that already resolves to the
machine. Two ordinary causes:

- **The name does not resolve yet.** DNS can take a few minutes to
  propagate, even when `expose add` wrote the record (or printed it for you
  to create by hand) a moment ago. Caddy retries on its own — there is
  nothing to do but wait, and `devmachine dns status <host>` says when it
  has caught up.
- **Caddy has not noticed the new site yet.** It watches `sites.d`, but a
  reload can be missed on a busy machine. Force one:

  ```
  devmachine run --machine <name> -- 'systemctl reload caddy'
  ```

## `expose add` served, and the response is "Blocked request"

This is the application's own host check, not Caddy and not `expose`. Many
frameworks refuse a `Host` header they do not recognise, by default, as a
guard against a different kind of attack — and a name that was just
published is exactly the kind of header the app has never seen. Add the
published hostname to the application's own list of allowed hosts; `expose`
has nothing to do with that list.

## A sync cannot fetch the release

```
fetching https://github.com/.../packages-v1.tar.gz: the server answered 404
```

The pin in `config.yml` names a release that is not published. Check `packages:`
there against the tags the packages repository actually has. A pin is a release
tag, never a branch.

If the URL is right, the network is the problem: the fetch is an ordinary
anonymous download, so a proxy or a firewall that blocks GitHub blocks this.

Once a release is fetched it is cached, and a later sync at the same pin needs
no network at all.

## "the asset changed, which a pin exists to prevent"

The tarball downloaded does not have the checksum the release published. The
CLI refuses it and stops.

That means the asset was replaced after it was released, or something rewrote
it on the way. Neither is worth guessing about: get the release fixed, or pin a
different one. Nothing was sent to the machine.

## "package X needs a CLI >= 0.3.0, and this one is 0.2.1"

The recipe says which CLI can read it, and this binary is older. Upgrade it:

```
brew upgrade devmachine
```

Pinning an older release of the packages is the other way out, and it is the
right one when the upgrade is not yours to make.

## "package X is a workspace package, and main is a machine"

Scope is a property of the software, not a preference. Docker is installed once
and serves everyone; Claude Code has a login per person. The error names the
command that puts it in the right place:

```
devmachine packages add claude-code --workspace alice
```

## "package X extends caddy.sites.d, but caddy is not installed on machine main"

An extension writes into a place another package opened, so that other package
has to be on the same machine. Add it:

```
devmachine packages add caddy --machine main
```

The same error appears when the provider is installed but does not declare that
place. Check its `provides:` against the `extends:` that names it — the point is
written `<package>.<place>`, and both halves have to match.

## `sync --check` fails on a machine nothing has been applied to yet

A dry run against a machine that has no packages on it reports failures like:

```
No package matching 'docker-ce' is available
Could not find the requested service caddy: host
```

**Nothing is wrong.** This is what `--check` cannot do rather than something it
found. A dry run changes nothing, so a package's repository is never really
added, the package index is never really refreshed, and the package that
repository would have provided is genuinely not available to look at. The same
goes for a service belonging to software that was never installed.

Ansible has this limit with any third-party repository; it is not particular to
this CLI.

What to do: run `devmachine sync` for real once. From then on `--check` is
meaningful, because the packages and their repositories exist and it is
comparing against something. A dry run is a tool for seeing what a change would
do to a machine you already built, not for previewing the build itself.

## `sync` asks and I answered nothing

An empty answer is no, and so is a closed input. A command that changes a
machine defaults to changing nothing. Pass `--yes` to skip the question, or
`--check` to see what would happen without being asked at all.

## `ERROR! Invalid options for include_role: devmachine_<package>_<name>`

A setting for a workspace package was written among `include_role`'s own
options instead of in the task's `vars:`. `include_role` takes a fixed set of
options and refuses the whole play when it meets one it does not know, so the
run stops before anything happens.

This was a defect in the generated playbook and it is fixed. If you see it,
your binary predates the fix: build or install a newer one. Nothing on the
machine is wrong, and nothing was half applied — the play never started.

## `credential "X" cannot be shared`

You asked for `X: machine`, and the package that declares it does not say
`shareable: true`. That is the package saying a copy of its session file does
not work on another account — a token bound to a device or a browser, usually.
Sharing it would put a file where the tool looks, the tool would reject it, and
nothing on the machine would say why.

What to do: ask for `own` instead, and log in once in each workspace. If you
know a copy does work for that tool, the fix belongs in the package, not in
your configuration: add `shareable: true` there.

## The shared login did not reach a workspace

Two ordinary reasons, in this order:

- **Nobody has logged in yet.** The copy comes from
  `/etc/devmachine/<name>/`, and `sync` skips rather than fails when nothing is
  there. Run `devmachine login <name>`, then `devmachine sync --tags
  credentials`.
- **That workspace asked to keep its own.** A workspace with `<name>: own` in
  its `credentials:` is left out of the copying on purpose, so it never loses
  the account it logged in with. Remove that line if you meant to share.

The other direction — a workspace whose own login was overwritten — is the same
setting read the other way: it had no `own` and the shared login reached it.
## `credentials list` says `unknown`

The package that declared it never said where its tool keeps the result, so
there is nowhere to look. The credential may well be there. Add `stored_at:` to
the package's declaration and the row starts answering.

## "credential X belongs to a workspace: name it with --workspace"

Two workspaces log into the same tool with different accounts, so there is no
one answer to "log in where". Name the workspace. The list in the error is
every workspace that asks for it.

## `devmachine login` says "Host key verification failed"

The login runs through the system `ssh`, and that `ssh` has never seen this
machine before. `doctor`, `run` and `sync` do not hit this: they use the CLI's
own SSH client, which does not read `~/.ssh/known_hosts`.

Open a session once and accept the key — `devmachine ssh`, and answer `yes` —
then run the login again. In a script, with nothing able to answer, `ssh` exits
255 and nothing was run.

## "the login left nothing at …"

The login command ran and the file the package promised is not there. Either it
was cancelled or declined, or the tool keeps its session somewhere else than
the package's `stored_at` says. `stored_at` is a claim by the package, not a
guarantee — check where the tool really writes, and correct the package.

## `credentials push` says "nothing to deliver" and the value is out of date

`push` writes only what is missing, so a machine that already has the file
keeps the old value. To replace one, remove the file on the machine — the path
is in `credentials list` — and push again.

## I already committed a key

Untracking it is not enough. `git rm --cached` takes a file out of the next
commit, but it stays in every commit that already has it — anyone with a clone,
or the history itself if the remote is ever made public, still has it.

1. **Rotate the key first.** Generate a new one and get it authorised wherever
   the old one was, so the copy sitting in your history stops being able to get
   in anywhere.
2. Then untrack it: `git rm --cached <path>`, commit that, and add it to
   `.gitignore` if `devmachine setup git` had not already.
3. If the repository was ever pushed anywhere, rewriting history
   (`git filter-repo`, or deleting and recreating the remote) removes the key
   from the copy other people can see — but the key is still compromised the
   moment it was committed, whatever you do to the history afterwards. Step 1
   is the one that actually fixes anything.

`devmachine setup git` refuses to run against a directory that already tracks
one of these paths, and its error says exactly this — see
[versioning your configuration](how-it-works/versioning-your-configuration.md).
