package dns

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/adevmachine/cli/internal/packages"
	"github.com/adevmachine/cli/internal/provision"
	"github.com/adevmachine/cli/internal/remote"
)

// KindDNS is what a package.yml writes in `kind` to be a DNS provider.
const KindDNS = "dns"

// ProviderManual is Manual's name, and what `--dns-provider manual` asks for.
const ProviderManual = "manual"

// buildExternal wraps a found DNS package as a Provider, sourced from the
// credential it declares.
func buildExternal(found packages.Found, client remote.Client) (*External, error) {
	m := found.Manifest

	var credential string
	for _, c := range m.Credentials {
		if c.Kind == packages.KindSecret {
			credential = c.Name
			break
		}
	}
	if credential == "" {
		return nil, fmt.Errorf(
			"the %s package declares no credential, so there is nothing to source before it runs", m.Name)
	}

	entrypoint := provision.RolePath(found.Source, m.Name, m.Entrypoint)
	return NewExternal(m.Name, client, entrypoint, credential, m.Commands), nil
}

// Installed builds every package of kind: dns that the lock says is on this
// machine.
//
// The lock is what says a package is installed, not the cache: a recipe in
// the cache and not in the lock is the ordinary state for every package this
// machine never asked for, and asking one anyway fails in a way nobody can
// act on.
func Installed(dir, machine string, client remote.Client) ([]*External, error) {
	lock, err := packages.LoadLock(dir)
	if err != nil {
		return nil, err
	}
	store, err := packages.Open(context.Background(), dir, lock.Release)
	if err != nil {
		return nil, err
	}

	var out []*External
	for _, entry := range lock.Machines[machine] {
		found, err := store.Get(entry.Name)
		if err != nil {
			return nil, err
		}
		if found.Manifest.Kind != KindDNS {
			continue
		}
		ext, err := buildExternal(found, client)
		if err != nil {
			return nil, err
		}
		out = append(out, ext)
	}
	return out, nil
}

// lookupInstalled finds a single package this machine's lock says is
// installed, whatever its kind.
//
// Unlike Installed, it reports rather than skips: somebody who named a
// package wants to know why it did not work, not silence.
func lookupInstalled(dir, machine, name string) (packages.Found, error) {
	lock, err := packages.LoadLock(dir)
	if err != nil {
		return packages.Found{}, err
	}

	installed := false
	for _, entry := range lock.Machines[machine] {
		if entry.Name == name {
			installed = true
			break
		}
	}
	if !installed {
		return packages.Found{}, fmt.Errorf(
			"%s is not installed on %s. Add it with `devmachine packages add %s` and apply with `devmachine sync`",
			name, machine, name)
	}

	store, err := packages.Open(context.Background(), dir, lock.Release)
	if err != nil {
		return packages.Found{}, err
	}
	return store.Get(name)
}

// One builds a single installed DNS provider by name.
func One(dir, machine, name string, client remote.Client) (*External, error) {
	found, err := lookupInstalled(dir, machine, name)
	if err != nil {
		return nil, err
	}
	if found.Manifest.Kind != KindDNS {
		return nil, fmt.Errorf("%s is not a DNS provider (kind %q)", name, found.Manifest.Kind)
	}
	return buildExternal(found, client)
}

// Any builds a single installed package's entrypoint, whatever its kind.
//
// One refuses anything that is not a DNS provider, which is right for a
// command that only makes sense against one. `run --package` reaches any
// package that declares an entrypoint, and this is the door for that.
func Any(dir, machine, name string, client remote.Client) (*External, error) {
	found, err := lookupInstalled(dir, machine, name)
	if err != nil {
		return nil, err
	}
	if found.Manifest.Entrypoint == "" {
		return nil, fmt.Errorf("%s declares no entrypoint, so there is nothing to call", name)
	}
	return buildExternal(found, client)
}

// Choice is what a command decided to act on, and why.
type Choice struct {
	Provider Provider
	Name     string
	Zone     string
	Why      string
}

// Choose resolves a name to a provider and a zone.
//
// It returns an error only when --dns-provider names something unusable or
// when two installed providers claim the same zone: every other outcome has
// an answer, and the answer is manual. A nil client means nothing can be
// asked, and the answer is manual too.
func Choose(ctx context.Context, dir, machine, name, providerFlag string,
	client remote.Client, out io.Writer) (Choice, error) {

	manual := Choice{Provider: NewManual(out), Name: ProviderManual, Zone: name}

	if providerFlag == ProviderManual {
		manual.Why = "the --dns-provider flag"
		return manual, nil
	}
	if client == nil {
		// A provider runs on the machine. With no machine there is nothing
		// to ask and nothing to write, but there is still a record to print.
		manual.Why = "the machine could not be reached"
		return manual, nil
	}
	if providerFlag != "" {
		p, err := One(dir, machine, providerFlag, client)
		if err != nil {
			return Choice{}, err
		}
		return Choice{Provider: p, Name: p.Name(), Zone: name, Why: "the --dns-provider flag"}, nil
	}

	providers, err := Installed(dir, machine, client)
	if err != nil {
		return Choice{}, err
	}
	zoners := make([]Zoner, 0, len(providers))
	for _, p := range providers {
		zoners = append(zoners, p)
	}

	holder, err := WhoHolds(ctx, zoners, name)
	switch {
	case err == nil:
		for _, p := range providers {
			if p.Name() == holder.Provider {
				return Choice{Provider: p, Name: p.Name(), Zone: holder.Zone,
					Why: fmt.Sprintf("%s holds %s", holder.Provider, holder.Zone)}, nil
			}
		}
		return Choice{}, fmt.Errorf("provider %q answered and then vanished", holder.Provider)
	case errors.Is(err, ErrAmbiguous):
		return Choice{}, err
	default:
		// Nobody holds it. That is ordinary — but saying nothing about a
		// provider that could not be asked is not.
		if len(providers) > 0 {
			fmt.Fprintf(out, "No installed provider holds %s: %v\n", name, err)
		}
		manual.Why = "no installed provider holds this zone"
		return manual, nil
	}
}
