package daemon

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func testStore(t *testing.T) *TelemetryStore {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	store := &TelemetryStore{db: db}
	if err := store.enableWALMode(); err != nil {
		t.Fatal(err)
	}
	if err := store.initSchema(); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestWALMode(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	var mode string
	if err := store.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want 'wal'", mode)
	}
}

func TestRecordAccessDirect(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	event := model.AccessEvent{
		OccurredAt:     time.Now(),
		ClientName:     "test-cli",
		Workspace:      "multi-tedi",
		WorkspaceInput: "C:/repos/mios/multi-tedi",
		WorkspaceRoot:  "C:/repos/mios/multi-tedi",
		WorkspaceAlias: "multi-tedi",
		Operation:      "nav.find",
		Backend:        "catalog",
		Success:        true,
		LatencyMs:      42,
		ErrorKind:      "sdk",
		ErrorCode:      "dotnet_sdk_missing",
	}
	if err := store.RecordAccessDirect(event); err != nil {
		t.Fatal(err)
	}

	events, err := store.RecentAccesses(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ClientName != "test-cli" {
		t.Errorf("ClientName = %q, want 'test-cli'", events[0].ClientName)
	}
	if events[0].WorkspaceRoot != "C:/repos/mios/multi-tedi" {
		t.Errorf("WorkspaceRoot = %q, want canonical root", events[0].WorkspaceRoot)
	}
	if events[0].WorkspaceAlias != "multi-tedi" {
		t.Errorf("WorkspaceAlias = %q, want multi-tedi", events[0].WorkspaceAlias)
	}
	if events[0].ErrorCode != "dotnet_sdk_missing" {
		t.Errorf("ErrorCode = %q, want dotnet_sdk_missing", events[0].ErrorCode)
	}
}

func TestPurgeOldEvents(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	old := model.AccessEvent{
		OccurredAt: time.Now().Add(-60 * 24 * time.Hour),
		ClientName: "old",
		Operation:  "nav.find",
		Backend:    "catalog",
		Success:    true,
		LatencyMs:  10,
	}
	recent := model.AccessEvent{
		OccurredAt: time.Now(),
		ClientName: "recent",
		Operation:  "nav.find",
		Backend:    "catalog",
		Success:    true,
		LatencyMs:  10,
	}
	if err := store.RecordAccessDirect(old); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordAccessDirect(recent); err != nil {
		t.Fatal(err)
	}

	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	deleted, err := store.PurgeOldEvents(cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	events, err := store.RecentAccesses(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Errorf("remaining events = %d, want 1", len(events))
	}
	if events[0].ClientName != "recent" {
		t.Errorf("remaining event client = %q, want 'recent'", events[0].ClientName)
	}
}

func TestPurgeOldRuns(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	state := model.DaemonState{PID: 1, Endpoint: "test", StartedAt: time.Now().Add(-60 * 24 * time.Hour)}
	runID, err := store.StartRun(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StopRun(runID, time.Now().Add(-60*24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	deleted, err := store.PurgeOldRuns(cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}
}

func TestQueryAccessEvents(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	events := []model.AccessEvent{
		{OccurredAt: time.Now(), ClientName: "a", Workspace: "multi-tedi", WorkspaceRoot: "C:/repos/mios/multi-tedi", WorkspaceAlias: "multi-tedi", Operation: "nav.find", Backend: "roslyn", Success: true, LatencyMs: 80},
		{OccurredAt: time.Now(), ClientName: "b", Workspace: "gastos", WorkspaceRoot: "C:/repos/mios/gastos", WorkspaceAlias: "gastos", Operation: "nav.search", Backend: "text", Success: true, LatencyMs: 30},
		{OccurredAt: time.Now(), ClientName: "c", Workspace: "gastos", WorkspaceRoot: "C:/repos/mios/gastos", WorkspaceAlias: "gastos", Operation: "nav.refs", Backend: "roslyn", Success: false, LatencyMs: 200, Error: "Requested SDK version: 10.0.201", ErrorKind: "sdk", ErrorCode: "dotnet_global_json_mismatch"},
	}
	for _, e := range events {
		if err := store.RecordAccessDirect(e); err != nil {
			t.Fatal(err)
		}
	}

	all, err := QueryAccessEvents(store, ExportQuery{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Errorf("all = %d, want 3", len(all))
	}

	gastos, err := QueryAccessEvents(store, ExportQuery{Workspace: "C:/repos/mios/gastos", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(gastos) != 2 {
		t.Errorf("gastos = %d, want 2", len(gastos))
	}

	errs, err := QueryAccessEvents(store, ExportQuery{ErrorsOnly: true, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 {
		t.Errorf("errors = %d, want 1", len(errs))
	}
	if errs[0].ErrorCode != "dotnet_global_json_mismatch" {
		t.Errorf("ErrorCode = %q, want dotnet_global_json_mismatch", errs[0].ErrorCode)
	}

	roslyn, err := QueryAccessEvents(store, ExportQuery{Backend: "roslyn", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(roslyn) != 2 {
		t.Errorf("roslyn = %d, want 2", len(roslyn))
	}
}

func init() {
	os.Setenv("HOME", os.TempDir())
	os.Setenv("USERPROFILE", os.TempDir())
}

func TestRecentAccesses_And_QueryAccessEvents_HandleLegacyNullFields(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	_, err := store.db.Exec(
		`INSERT INTO access_events(daemon_run_id, occurred_at, client_name, session_id, workspace, workspace_input, workspace_root, workspace_alias, repo, operation, backend, success, latency_ms, warnings_json, runtime_key, entrypoint_id, error_text, error_kind, error_code, truncated, result_count)
		 VALUES (NULL, ?, ?, ?, ?, NULL, NULL, NULL, NULL, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?)`,
		time.Now().Unix(),
		"legacy-cli",
		"session-null",
		"multi-tedi",
		"nav.context",
		"roslyn",
		1,
		12,
		"[]",
		"",
		"",
		"",
		0,
		1,
	)
	if err != nil {
		t.Fatalf("insert legacy access event: %v", err)
	}

	recent, err := store.RecentAccesses(10)
	if err != nil {
		t.Fatalf("RecentAccesses: %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("len(recent) = %d, want 1", len(recent))
	}
	if recent[0].Repo != "" {
		t.Fatalf("recent[0].Repo = %q, want empty string", recent[0].Repo)
	}
	if recent[0].WorkspaceRoot != "multi-tedi" {
		t.Fatalf("recent[0].WorkspaceRoot = %q, want fallback workspace key", recent[0].WorkspaceRoot)
	}

	events, err := QueryAccessEvents(store, ExportQuery{Workspace: "multi-tedi", Limit: 10})
	if err != nil {
		t.Fatalf("QueryAccessEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if events[0].Repo != "" {
		t.Fatalf("events[0].Repo = %q, want empty string", events[0].Repo)
	}
}
