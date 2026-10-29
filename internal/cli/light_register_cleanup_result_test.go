package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLightRegistrationCleanupFailureIsNotSuccessfulSilence(t *testing.T) {
	calls := 0
	message := finishLightRegistrationResult(nil, func(workErr error) error {
		calls++
		if workErr != nil {
			t.Fatal("unexpected registration error")
		}
		return context.DeadlineExceeded
	})
	if calls != 1 || !strings.HasPrefix(message, "auto_register_pending:") || !strings.Contains(message, "cleanup unconfirmed") {
		t.Fatalf("hidden cleanup: %q calls=%d", message, calls)
	}
}

func TestLightRegistrationCleanupPreservesBothErrors(t *testing.T) {
	workErr := errors.New("private registration error")
	message := finishLightRegistrationResult(workErr, func(got error) error {
		if got != workErr {
			t.Fatal("registration error not forwarded")
		}
		return errors.New("private cleanup error")
	})
	if !strings.Contains(message, workErr.Error()) || !strings.Contains(message, "private cleanup error") {
		t.Fatalf("error lost: %q", message)
	}
}

func TestLightRegistrationUnneededConfirmedCleanupHasNoWarning(t *testing.T) {
	calls := 0
	message := finishLightRegistrationResult(nil, func(err error) error { calls++; return nil })
	if calls != 1 || message != "" {
		t.Fatalf("confirmed cleanup: %q calls=%d", message, calls)
	}
}

func TestLightRegistrationPriorFailureNeverRepublished(t *testing.T) {
	calls := 0
	message := finishPriorLightRegistrationFailure("private prior failure", func(err error) error {
		calls++
		if err != nil {
			t.Fatal("consumed old failure would be republished")
		}
		return nil
	})
	if calls != 1 || message != "auto_register_failed: private prior failure" {
		t.Fatalf("prior failure: %q calls=%d", message, calls)
	}
}
