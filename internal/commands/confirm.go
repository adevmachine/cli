package commands

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// errDeclined is what a command returns when the person said no.
//
// It is an error rather than a quiet success because the exit code is what a
// script reads, and "I did not do it" must not look like "I did it".
var errDeclined = errors.New("nothing was changed")

// confirm asks a yes-or-no question and reads one line.
//
// Anything but yes is no, including an empty answer and a closed input: a
// command that changes a machine defaults to changing nothing.
func confirm(in io.Reader, out io.Writer, question string) (bool, error) {
	if _, err := fmt.Fprintf(out, "%s [y/N]: ", question); err != nil {
		return false, err
	}

	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		return false, fmt.Errorf("reading the answer: %w", err)
	}

	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
