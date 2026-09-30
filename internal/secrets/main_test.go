package secrets

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	os.Setenv(KeychainEnv, "off")
	os.Exit(m.Run())
}
