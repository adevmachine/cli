package dns

import "context"

// Record is one DNS record.
type Record struct {
	// Name is the label, not the full name: "www", or "@" for the apex.
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
	TTL   int    `json:"ttl,omitempty"`
}

// Provider is a place records live.
//
// The interface exists from the first version even though no provider is
// implemented yet, because it is what keeps a vendor out of the core: the rest
// of the CLI only ever talks to this.
type Provider interface {
	// Name is how the provider is written in the configuration.
	Name() string
	List(ctx context.Context, zone string) ([]Record, error)
	Upsert(ctx context.Context, zone string, r Record) error
	Delete(ctx context.Context, zone string, r Record) error
}
