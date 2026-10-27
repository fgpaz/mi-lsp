package daemon

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/service"
)

func TestUpdateManagerSnapshotConservative(t *testing.T) {
	var nilManager *Manager
	if nilManager.UpdateIdleSnapshot() {
		t.Fatal("unknown manager cannot prove idle")
	}
	m := &Manager{runtimes: map[string]*managedRuntime{}, watchers: map[string]*FileWatcher{}}
	if !m.UpdateIdleSnapshot() {
		t.Fatal("empty manager should be idle")
	}
	m.runtimes["fixture"] = &managedRuntime{}
	if m.UpdateIdleSnapshot() {
		t.Fatal("live runtime permits idle")
	}
	delete(m.runtimes, "fixture")
	m.watchers["fixture"] = &FileWatcher{}
	if m.UpdateIdleSnapshot() {
		t.Fatal("watcher permits idle")
	}
}

func TestUpdateAdminMiddlewareRetainsLeaseThroughResponse(t *testing.T) {
	gate := newUpdateAdmissionGate()
	entered, unblock, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	h := updateAdminHandler(gate, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-unblock
		_, _ = w.Write([]byte("intact"))
	}))
	rec := httptest.NewRecorder()
	go func() { h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil)); close(done) }()
	<-entered
	if release, idle := gate.TryIdleBarrier(); idle {
		release()
		t.Fatal("active HTTP counted idle")
	}
	close(unblock)
	<-done
	if rec.Body.String() != "intact" {
		t.Fatal("response interrupted")
	}
	release, ok := gate.TryIdleBarrier()
	if !ok {
		t.Fatal("idle barrier denied")
	}
	denied := httptest.NewRecorder()
	h.ServeHTTP(denied, httptest.NewRequest("GET", "/", nil))
	if denied.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d", denied.Code)
	}
	release()
}

func TestUpdateIdleSnapshotDefersLiveSession(t *testing.T) {
	s := &Server{manager: &Manager{runtimes: map[string]*managedRuntime{}, watchers: map[string]*FileWatcher{}}, qSessions: service.NewSessionState()}
	if !s.updateResourcesIdle(time.Now()) {
		t.Fatal("empty resources should be idle")
	}
	s.qSessions.NextMark("fixture")
	if s.updateResourcesIdle(time.Now()) {
		t.Fatal("session must veto restart")
	}
}
