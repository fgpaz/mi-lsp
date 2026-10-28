package daemon

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestWALAdmissionMeasurementDeadlineIsNotLimitOrBusy(t *testing.T) {
	s, db, _ := orphanTelemetryStore(t)
	conn, err := db.Conn(context.Background())
	if err != nil { t.Fatal(err) }
	defer conn.Close()
	err = s.RecordAccess(0, model.AccessEvent{Operation: "measurement"})
	if !errors.Is(err, context.DeadlineExceeded) { t.Fatalf("expected actual metadata acquisition deadline, got %v", err) }
	if err := conn.Close(); err != nil { t.Fatal(err) }
	h := s.WALHealth()
	if h.RejectedWrites != 1 || h.LastFailure == "" || h.LastFailure == "wal_limit" || h.LastFailure == "busy" || h.CheckpointBusy {
		t.Fatalf("metadata deadline must retain a non-size, non-busy failure: %+v", h)
	}
}

func TestCheckpointSuccessPreservesAdmissionFailureHistory(t *testing.T) {
	s, _, _ := orphanTelemetryStore(t)
	err := s.RecordAccess(0, model.AccessEvent{Operation: "oversized", Error: strings.Repeat("x", telemetryMaxEventBytes+1)})
	if err == nil { t.Fatal("oversized admission unexpectedly succeeded") }
	before := s.WALHealth()
	if before.RejectedWrites != 1 || before.LastFailure == "" { t.Fatalf("missing admission failure: %+v", before) }
	if err := s.Checkpoint(); err != nil { t.Fatalf("checkpoint should succeed: %v", err) }
	after := s.WALHealth()
	if after.LastFailure != before.LastFailure || after.RejectedWrites != before.RejectedWrites || after.FailedWrites != before.FailedWrites || after.CheckpointBusy {
		t.Fatalf("successful checkpoint erased write/admission history: before=%+v after=%+v", before, after)
	}
}
