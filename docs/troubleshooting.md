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

Expected on a machine nothing has provisioned yet. Installing it is part of
`sync`, which does not exist in this version. Everything else in `doctor` still
tells you the truth.

## A `tailscale:` address is being ignored

It is dropped when `tailscale` is not installed or does not know that name, and
the next address is tried. That is deliberate.

To see what happened, run `tailscale status` and check the machine is there
under the name you wrote.

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

