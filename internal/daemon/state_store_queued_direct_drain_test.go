package daemon

import (
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// The queue's isDirect bit selects NULL daemon_run_id, not whether the request
// bypassed the queue. Closing public admission must not drop an accepted entry.
func TestQueuedDirectEventDrainsAfterAdmissionClosed(t *testing.T) {
	s := newMetadataAdmissionFixture(t)
	s.stateMu.Lock()
	s.closed = true // The admission boundary, before physical DB.Close.
	s.stateMu.Unlock()

	s.persistQueued(telemetryWriteRequest{
		event:    model.AccessEvent{Operation: "accepted-queued-direct"},
		isDirect: true,
	})
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM access_events WHERE operation = ? AND daemon_run_id IS NULL`, "accepted-queued-direct").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("accepted queued direct entries persisted=%d, want 1", count)
	}
	h := s.WALHealth()
	if h.RejectedWrites != 0 || h.FailedWrites != 0 {
		t.Fatalf("accepted queue drain falsely rejected/failed: %+v", h)
	}
	if err := s.RecordAccessDirect(model.AccessEvent{Operation: "new-public-direct"}); err == nil {
		t.Fatal("new public write admitted after close admission")
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM access_events WHERE operation = ?`, "new-public-direct").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("closed public admission persisted %d late entries", count)
	}
}
