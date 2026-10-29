package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLightRegistrationStartedReleaseFailureRetainsReservation(t *testing.T) {
	// Private fixture and per-call injection: no PATH/global factory/registry.
	root := filepath.Join(t.TempDir(), ".light-register-"+strings.Repeat("a", 24))
	receipt := filepath.Join(root, strings.Repeat("b", 32))
	if err := os.MkdirAll(receipt, 0o700); err != nil {
		t.Fatal(err)
	}
	calls := 0
	message := releaseLightRegistrationWorker(func() error { calls++; return errors.New("private handle release failure") })
	if calls != 1 || !strings.HasPrefix(message, "auto_register_pending:") || !strings.Contains(message, "reservation retained") {
		t.Fatalf("untruthful started outcome: %q calls=%d", message, calls)
	}
	if _, err := os.Stat(receipt); err != nil {
		t.Fatalf("live/unknown worker reservation removed: %v", err)
	}
	if _, err := os.Lstat(root + ".failure"); !os.IsNotExist(err) {
		t.Fatalf("release error fabricated a worker failure: %v", err)
	}
}

func TestLightRegistrationSuccessfulReleaseRemainsPending(t *testing.T) {
	calls := 0
	message := releaseLightRegistrationWorker(func() error { calls++; return nil })
	if calls != 1 || message != "auto_register_pending: lightweight registration scheduled; no index started" {
		t.Fatalf("outcome=%q calls=%d", message, calls)
	}
}
