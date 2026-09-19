// Package history records one line per command that touched a machine.
//
// It exists so that "what did that session do to my machine" has an answer.
// Over ssh it does not: the shell history lives on the machine, under whichever
// account happened to be used, and a command run from the CLI leaves no trace
// at all.
//
// Nothing in the CLI reads this file. Rotation, searching and pruning are what
// tail, grep and logrotate already do better, and a reader here would be the
// beginning of a feature nobody asked for.
package history

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// FileName is the log, inside the configuration directory.
const FileName = "history.log"

// Entry is one command, and how it ended.
type Entry struct {
	At      time.Time
	Target  string
	Command string
	OK      bool
}

// Append writes one line for the entry.
//
// It returns nothing, and every error inside it is dropped on purpose: a log
// that can fail a command is a log somebody removes the first time it costs
// them an afternoon. The record is worth less than the work it records.
func Append(dir string, entry Entry) {
	file, err := os.OpenFile(filepath.Join(dir, FileName),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()

	_, _ = fmt.Fprintf(file, "%s  %-16s  %-6s  %s\n",
		entry.At.UTC().Format(time.RFC3339),
		oneLine(entry.Target),
		entry.status(),
		strconv.Quote(entry.Command))
}

func (e Entry) status() string {
	if e.OK {
		return "ok"
	}
	return "failed"
}

// oneLine keeps a value from forging a second entry. The command is quoted,
// which does this for it; a target is printed as written, so it is done here.
func oneLine(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, s)
}
