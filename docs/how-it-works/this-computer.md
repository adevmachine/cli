# This computer

A machine can declare `self: true`: the computer the CLI itself runs on. This
is why it exists, why it is named `self` and not `local`, and what it changes
about `sync` and `setup`.

## Why `self`, not `local`

`machines create-local` already means something: a Lima VM, made on this
computer, that stands in for a bought server while you learn or test the CLI.
That VM has its own address, its own port, its own root login, its own key —
and its own `setup`, exactly like anything else. It is a remote machine in
every way that matters. It only happens to live here.

`self` is a different thing entirely: the computer the command is running on,
right now, with no address to dial and nothing to install a key into. Reusing
"local" for it would make two different ideas share one word, and the first
time somebody typed the wrong one they would find out the hard way. So this
got its own word.

## Why no address fields

`hosts`, `user`, `port` and `key` describe how to reach a machine that is not
this one. A self machine has none of that to describe — there is no dial, no
login, no key to install — so `config.Validate` refuses all four outright
rather than let one sit there unused and eventually mean something to nobody.
The refusal names the field:

```
machine "mac" is this computer (self: true), so it has no hosts: remove it
```

The same reasoning is why a workspace can never live on one. A workspace is a
Linux account on a server, reached by a key the CLI installed for it. A self
machine has no account model like that and no SSH server of its own for a
workspace to be reached through.

## Why no `become`

Two guards carry over from the era before this CLI had a name for a self
machine, when a single unmanaged playbook did this job by hand: the run
escalates nothing, because Homebrew, mise and Claude all live under `$HOME` on
a Mac and none of it needs root; and the run refuses outside macOS, because
nothing about this path has ever been tried anywhere else.

The generated play reflects both. Its header carries `become: false` instead of
`become: true`, and its first task is a guard that fails the run before
anything else happens:

```yaml
- name: refuses anywhere but macOS
  fail:
    msg: "a self machine is converged on macOS only; this is {{ ansible_facts['system'] }}"
  when: ansible_facts['system'] != 'Darwin'
```

A remote machine gets neither: it still escalates, and it carries no such
guard, because none of this is a question there.

## Why `setup` prints the Homebrew command instead of running it

Ansible cannot install itself, so something has to run before the first
`sync`. On a remote machine that is `setup`'s whole bootstrap: a key, a proof,
hardening, then Ansible. None of that applies here — there is no address to
bootstrap — so `setup` on a self machine only makes sure two things are true:
Homebrew is on `PATH`, and Ansible is too.

If Homebrew is missing, `setup` prints the official one-line install command
from [brew.sh](https://brew.sh) and stops. It does not run it. That command
asks for `sudo`, and asking for `sudo` on the operator's own computer, on their
behalf, without them typing it themselves, is not something this CLI does. Once
Homebrew is there, `setup` runs `brew install ansible` itself and streams the
output — that part needs no elevated privilege, so there is nothing stopping
it.

`sync` follows the same rule from the other side: if `ansible-playbook` is not
on `PATH`, it refuses before touching anything and names `setup` as the fix,
the same way it would point at a missing prerequisite on a remote machine.

## Where the bundle lives, and why not `/opt`

A remote machine gets its bundle at `/opt/devmachine` — root's territory,
which is fine, because `setup` already has root on a server it took over
deliberately. `/opt` is not writable without root on a Mac, and asking for
`sudo` here runs into the same objection as installing Homebrew for someone:
it is not this CLI's to ask for.

So a self machine's bundle lives under the operator's own home instead, right
next to everything else unprivileged tooling already puts there:

```
$HOME/.local/share/devmachine/bundle
```

Internally this is `provision.Base(machine)`: `RemoteDir` for anything reached
over SSH, and this path for anything `self: true`. The playbook, the inventory,
`ansible.cfg`, every role and every extension point are generated against
whichever base applies — `provision.GenerateAt` takes it as a plain argument,
never read from the environment inside the pure function that builds the
files, so the same plan and the same base always produce the same bytes.
