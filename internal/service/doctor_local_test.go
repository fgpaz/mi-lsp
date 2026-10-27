package service

import (
	"context"
	"testing"
)

func TestDoctorOptionalDaemonAndRequiredFailures(t *testing.T) {
	stopped := finalizeDoctorCheck(checkDaemonConnectivity(context.Background(), &RunDoctorOptions{DaemonState: DaemonStateStopped, DaemonErrorKind: DaemonErrorUnavailable}))
	if stopped.Status != "skip" || stopped.Required {
		t.Fatalf("optional daemon: %+v", stopped)
	}
	failed := finalizeDoctorCheck(checkDaemonConnectivity(context.Background(), &RunDoctorOptions{DaemonErrorKind: DaemonErrorCommunication, DaemonError: "broken"}))
	if failed.Status != "fail" || !failed.Required {
		t.Fatalf("communication failure: %+v", failed)
	}
	required := finalizeDoctorCheck(DoctorCheck{ID: "stale-aliases", OK: false})
	if required.Status != "fail" || !required.Required {
		t.Fatalf("required failure: %+v", required)
	}
}
