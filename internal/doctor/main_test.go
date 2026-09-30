package doctor

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	aliasCLI = func() string { return "" }
	os.Exit(m.Run())
}
