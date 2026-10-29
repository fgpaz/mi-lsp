package workspace

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestLightRegistrationAbsentFailureIsNotReadError(t *testing.T) {
	root := privateLightRegistrationRoot(t)
	privateLightRegistrationClaim(t, root)
	failure, err := lightRegistrationFailureResultAt(context.Background(), root)
	if failure != "" || err != nil {
		t.Fatalf("absent receipt failure=%q err=%v", failure, err)
	}
}

func TestLightRegistrationMalformedFailureIsReportedAndRetained(t *testing.T) {
	root := privateLightRegistrationRoot(t)
	receipt := privateLightRegistrationClaim(t, root)
	if err := os.Mkdir(root+".failure", 0o700); err != nil {
		t.Fatal(err)
	}
	failure, err := lightRegistrationFailureResultAt(context.Background(), root)
	if failure != "" || err == nil {
		t.Fatalf("malformed receipt suppressed: failure=%q err=%v", failure, err)
	}
	if _, err := os.Stat(receipt); err != nil {
		t.Fatalf("unconfirmed generation removed: %v", err)
	}
	if _, err := os.Stat(root + ".failure"); err != nil {
		t.Fatalf("malformed receipt removed: %v", err)
	}
}

func TestLightRegistrationCanceledFailureReadReportsUncertainty(t *testing.T) {
	root := privateLightRegistrationRoot(t)
	privateLightRegistrationClaim(t, root)
	if err := os.WriteFile(root+".failure", []byte("private unread failure"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	failure, err := lightRegistrationFailureResultAt(ctx, root)
	if failure != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled receipt suppressed: failure=%q err=%v", failure, err)
	}
	data, err := os.ReadFile(root + ".failure")
	if err != nil || string(data) != "private unread failure" {
		t.Fatalf("unconsumed receipt changed: %v", err)
	}
}
