package commands

import (
	"encoding/json"
	"fmt"
	"io"
)

// The output formats. JSON is the stable contract; table is a projection of
// the same data for a human, never a different set of facts.
const (
	formatTable = "table"
	formatJSON  = "json"
)

// validateFormat rejects an unknown format before any work is done, so a typo
// never costs a connection.
func validateFormat(format string) error {
	switch format {
	case formatTable, formatJSON:
		return nil
	default:
		return fmt.Errorf("unknown --format %q: use %q or %q", format, formatTable, formatJSON)
	}
}

// writeJSON renders v as indented JSON followed by a newline.
func writeJSON(w io.Writer, v any) error {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("rendering JSON: %w", err)
	}
	if _, err := fmt.Fprintf(w, "%s\n", body); err != nil {
		return fmt.Errorf("writing the output: %w", err)
	}
	return nil
}
