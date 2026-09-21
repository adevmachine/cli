package dns

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrNoHolder means no installed provider claims the zone. It is not a
// failure: it is how a command falls through to `manual`.
var ErrNoHolder = errors.New("no installed provider holds this zone")

// Zoner is a provider that can say which zones it holds. Every kind: dns
// package answers this; `manual` does not, and is never asked.
type Zoner interface {
	Provider
	Zones(ctx context.Context) ([]string, error)
}

// Holder is a provider and the zone it holds that matched.
type Holder struct {
	Provider string
	Zone     string
}

// WhoHolds asks every installed provider which zones it can see, and returns
// the one whose longest zone the name falls inside.
//
// Asking costs one call per provider and is always current. A table mapping a
// zone to a provider would cost nothing and be wrong the first time a domain
// moved.
func WhoHolds(ctx context.Context, providers []Zoner, name string) (Holder, error) {
	var (
		best     []Holder
		failures []string
	)

	for _, p := range providers {
		zones, err := p.Zones(ctx)
		if err != nil {
			// One registrar being unreachable must not stop a command headed
			// for a different one. The reason is kept for the error below.
			failures = append(failures, fmt.Sprintf("%s (%v)", p.Name(), err))
			continue
		}
		for _, zone := range zones {
			if name != zone && !strings.HasSuffix(name, "."+zone) {
				continue
			}
			switch {
			case len(best) == 0 || len(zone) > len(best[0].Zone):
				best = []Holder{{Provider: p.Name(), Zone: zone}}
			case len(zone) == len(best[0].Zone) && zone == best[0].Zone:
				best = append(best, Holder{Provider: p.Name(), Zone: zone})
			}
		}
	}

	switch len(best) {
	case 1:
		return best[0], nil
	case 0:
		if len(failures) > 0 {
			return Holder{}, fmt.Errorf("%w: %s could not be asked: %s",
				ErrNoHolder, plural(len(failures), "provider"), strings.Join(failures, "; "))
		}
		return Holder{}, fmt.Errorf("%w: %s", ErrNoHolder, name)
	default:
		var names []string
		for _, h := range best {
			names = append(names, h.Provider)
		}
		return Holder{}, fmt.Errorf("%w: %s is held by %s. Say which with --dns-provider",
			ErrAmbiguous, best[0].Zone, strings.Join(names, " and "))
	}
}

// plural writes a count with the noun pluralised, in the plain -s way every
// noun here happens to take.
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
