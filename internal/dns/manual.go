package dns

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// Manual is the one Provider compiled into the binary.
//
// It makes no request: a registrar with no package still gets a useful
// answer, the exact record to create by hand, and `dns status` still says
// whether that record took effect.
type Manual struct {
	out io.Writer
}

// NewManual returns a Manual that prints its instructions to out.
func NewManual(out io.Writer) *Manual {
	return &Manual{out: out}
}

// Name is how manual is written in a --dns-provider flag.
func (m *Manual) Name() string { return "manual" }

// List cannot work: manual never speaks to a registrar, so it has nothing to
// read a zone from.
func (m *Manual) List(context.Context, string) ([]Record, error) {
	return nil, errors.New(
		"manual cannot list a zone it does not speak to. Check a name with `devmachine dns status`")
}

// Upsert prints the record to create by hand, how to check it took effect,
// and how to stop being manual for this registrar.
func (m *Manual) Upsert(_ context.Context, zone string, r Record) error {
	fmt.Fprintf(m.out, "No installed provider holds %s. Create this record by hand:\n\n", zone)
	fmt.Fprintf(m.out, "  %s\t%s\t%s", r.Name, r.Type, r.Value)
	if r.TTL > 0 {
		fmt.Fprintf(m.out, "\t%d", r.TTL)
	}
	fmt.Fprintln(m.out)
	fmt.Fprintf(m.out, "\nCheck it with `devmachine dns status %s`.\n", fullName(r.Name, zone))
	fmt.Fprintln(m.out,
		"Run `devmachine packages list` to see whether a provider package exists for this registrar,",
		"and `devmachine packages add <name>` to let the CLI do this instead.")
	return nil
}

// Delete prints the record to remove by hand.
func (m *Manual) Delete(_ context.Context, zone string, r Record) error {
	fmt.Fprintf(m.out, "No installed provider holds %s. Remove this record by hand:\n\n", zone)
	fmt.Fprintf(m.out, "  %s\t%s\t%s\n", r.Name, r.Type, r.Value)
	return nil
}

// fullName joins a record's label onto its zone, the way somebody would type
// it into `dns status`.
func fullName(label, zone string) string {
	if label == "" || label == "@" {
		return zone
	}
	return label + "." + zone
}
