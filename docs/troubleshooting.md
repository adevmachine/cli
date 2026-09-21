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

## `credentials list` says `unknown`

The package that declared it never said where its tool keeps the result, so
there is nowhere to look. The credential may well be there. Add `stored_at:` to
the package's declaration and the row starts answering.
