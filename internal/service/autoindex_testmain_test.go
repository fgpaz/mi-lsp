package service

import (
	"os"
	"testing"
)

// TestMain disables the background auto-index so tests that read an
// unindexed workspace never spawn a detached process. Auto-index tests opt in
// with t.Setenv(autoIndexEnvEnable, "1") and a fake spawner.
func TestMain(m *testing.M) {
	_ = os.Setenv(autoIndexEnvEnable, "0")
	os.Exit(m.Run())
}
