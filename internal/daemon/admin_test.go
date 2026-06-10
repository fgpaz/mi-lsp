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

func TestHandleIndexAddsSecurityHeaders(t *testing.T) {
	admin := &AdminServer{adminToken: "test-token"}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()

	admin.handleIndex(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	csp := res.Header().Get("Content-Security-Policy")
	if csp != "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'" {
		t.Fatalf("CSP = %q, want expected policy", csp)
	}
	xframe := res.Header().Get("X-Frame-Options")
	if xframe != "DENY" {
		t.Fatalf("X-Frame-Options = %q, want DENY", xframe)
	}
}

// TestHandleWorkspaceWarmRequiresToken verifies that warm endpoint requires admin token.
func TestHandleWorkspaceWarmRequiresToken(t *testing.T) {
	store := testStore(t)
	defer store.Close()
	admin := &AdminServer{store: store, adminToken: "correct-token"}

	// Test missing token
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces/test/warm", nil)
	req.Host = "127.0.0.1:9999"
	res := httptest.NewRecorder()
	admin.handleWorkspaceWarm(res, req, "test")
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: status = %d, want 401", res.Code)
	}

	// Test wrong token
	req = httptest.NewRequest(http.MethodPost, "/api/workspaces/test/warm", nil)
	req.Host = "127.0.0.1:9999"
	req.Header.Set("X-Mi-Lsp-Token", "wrong-token")
	res = httptest.NewRecorder()
	admin.handleWorkspaceWarm(res, req, "test")
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: status = %d, want 401", res.Code)
	}

	// Test correct token but non-loopback host
	req = httptest.NewRequest(http.MethodPost, "/api/workspaces/test/warm", nil)
	req.Host = "example.com:9999"
	req.Header.Set("X-Mi-Lsp-Token", "correct-token")
	res = httptest.NewRecorder()
	admin.handleWorkspaceWarm(res, req, "test")
	if res.Code != http.StatusForbidden {
		t.Fatalf("non-loopback: status = %d, want 403", res.Code)
	}
}

// TestIsLoopbackHostValidatesLoopback checks loopback detection.
func TestIsLoopbackHostValidatesLoopback(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"127.0.0.1:9999", true},
		{"127.1.1.1:8000", true},
		{"localhost:9999", true},
		{"[::1]:9999", true},
		{"example.com:9999", false},
		{"192.168.1.1:9999", false},
	}
	admin := &AdminServer{}
	for _, test := range tests {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = test.host
		got := admin.isLoopbackHost(req)
		if got != test.want {
			t.Fatalf("isLoopbackHost(%q) = %v, want %v", test.host, got, test.want)
		}
	}
}
