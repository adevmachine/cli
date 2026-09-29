package commands

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// agentsFileName is the file coding agents read when a session is opened in
// the configuration directory: Claude Code (via its skills or an `@AGENTS.md`
// import), Codex, Pi, OpenCode and the others that follow the AGENTS.md
// convention.
const agentsFileName = "AGENTS.md"

//go:embed agents_template.md
var agentsTemplate string

// writeAgentsFile writes AGENTS.md the first time a configuration is
// written, and never after: a file the person has started editing is theirs,
// not the CLI's to overwrite.
func writeAgentsFile(dir string) error {
	path := filepath.Join(dir, agentsFileName)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(agentsTemplate), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
