package commands

import (
	"context"
	"errors"
	"os"
	"testing"
)

// No test reaches the network by accident: a test that needs a release says
// which one with stubLatestPackagesRelease.
func TestMain(m *testing.M) {
	latestPackagesRelease = func(context.Context) (string, error) {
		return "", errors.New("tests do not reach GitHub")
	}
	os.Exit(m.Run())
}
