package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestHandleAccessesRecentWindowFiltersHistoricalEvents(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	now := time.Now()
	for _, event := range []model.AccessEvent{
		{OccurredAt: now.Add(-2 * time.Hour), Workspace: "multi-tedi", Operation: "nav.context", Backend: "roslyn", Success: true, LatencyMs: 20},
		{OccurredAt: now.Add(-48 * time.Hour), Workspace: "multi-tedi", Operation: "nav.context", Backend: "roslyn", Success: true, LatencyMs: 30},
	} {
		if err := store.RecordAccessDirect(event); err != nil {
			t.Fatalf("RecordAccessDirect: %v", err)
		}
	}

	admin := &AdminServer{store: store}
	req := httptest.NewRequest(http.MethodGet, "/api/accesses?window=recent&limit=10", nil)
	res := httptest.NewRecorder()

	admin.handleAccesses(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	var payload struct {
		Items []model.AccessEvent `json:"items"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(payload.Items))
	}
}

func TestHandleMetricsRecentWindowFiltersHistoricalEvents(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	now := time.Now()
	for _, event := range []model.AccessEvent{
		{OccurredAt: now.Add(-2 * time.Hour), Workspace: "multi-tedi", Operation: "nav.context", Backend: "roslyn", Success: true, LatencyMs: 20},
		{OccurredAt: now.Add(-48 * time.Hour), Workspace: "multi-tedi", Operation: "nav.context", Backend: "roslyn", Success: false, LatencyMs: 30},
	} {
		if err := store.RecordAccessDirect(event); err != nil {
			t.Fatalf("RecordAccessDirect: %v", err)
		}
	}

	admin := &AdminServer{store: store}
	req := httptest.NewRequest(http.MethodGet, "/api/metrics?window=recent", nil)
	res := httptest.NewRecorder()

	admin.handleMetrics(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	var payload MetricsSummary
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload.Total != 1 {
		t.Fatalf("Total = %d, want 1", payload.Total)
	}
}

func TestReadLogTailUsesBoundedTail(t *testing.T) {
	root := t.TempDir()
	logDir := filepath.Join(root, ".mi-lsp")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	logPath := filepath.Join(logDir, "daemon.log")
	if err := os.WriteFile(logPath, []byte("one\ntwo\nthree\nfour\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	admin := &AdminServer{stateFn: func() model.DaemonState { return model.DaemonState{RepoRoot: root} }}
	path, items, warning, err := admin.readLogTail(2)
	if err != nil {
		t.Fatalf("readLogTail: %v", err)
	}
	if path != logPath {
		t.Fatalf("path = %q, want %q", path, logPath)
	}
	if warning != "" {
		t.Fatalf("warning = %q, want empty", warning)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0]["text"] != "three" || items[1]["text"] != "four" {
		t.Fatalf("items = %#v, want last two lines", items)
	}
}
