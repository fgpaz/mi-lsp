package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
