package daemon

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
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
	if events[0].Seq != 0 {
		t.Errorf("Seq = %d, want 0 for event without session_id", events[0].Seq)
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

func TestQueryAccessEvents_ZeroLimitReturnsAllRows(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	for i := 0; i < 505; i++ {
		event := model.AccessEvent{
			OccurredAt:     time.Now().Add(time.Duration(i) * time.Second),
			ClientName:     "bulk",
			Workspace:      "gastos",
			WorkspaceInput: "gastos",
			WorkspaceRoot:  "C:/repos/mios/gastos",
			WorkspaceAlias: "gastos",
			Operation:      "nav.search",
			Backend:        "text",
			Success:        true,
			LatencyMs:      int64(i + 1),
		}
		if err := store.RecordAccessDirect(event); err != nil {
			t.Fatalf("RecordAccessDirect(%d): %v", i, err)
		}
	}

	events, err := QueryAccessEvents(store, ExportQuery{Limit: 0})
	if err != nil {
		t.Fatalf("QueryAccessEvents: %v", err)
	}
	if len(events) != 505 {
		t.Fatalf("len(events) = %d, want 505", len(events))
	}
}

func TestRecordAccessDirect_AssignsSeqPerSession(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	first := model.AccessEvent{
		OccurredAt: time.Now(),
		ClientName: "test-cli",
		SessionID:  "session-1",
		Workspace:  "multi-tedi",
		Operation:  "nav.find",
		Backend:    "catalog",
		Success:    true,
	}
	second := model.AccessEvent{
		OccurredAt: time.Now().Add(time.Second),
		ClientName: "test-cli",
		SessionID:  "session-1",
		Workspace:  "multi-tedi",
		Operation:  "nav.search",
		Backend:    "text",
		Success:    true,
	}
	if err := store.RecordAccessDirect(first); err != nil {
		t.Fatalf("RecordAccessDirect first: %v", err)
	}
	if err := store.RecordAccessDirect(second); err != nil {
		t.Fatalf("RecordAccessDirect second: %v", err)
	}

	recent, err := store.RecentAccesses(10)
	if err != nil {
		t.Fatalf("RecentAccesses: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("len(recent) = %d, want 2", len(recent))
	}
	if recent[0].Seq != 2 || recent[1].Seq != 1 {
		t.Fatalf("recent seqs = [%d %d], want [2 1]", recent[0].Seq, recent[1].Seq)
	}

	events, err := QueryAccessEvents(store, ExportQuery{Limit: 10})
	if err != nil {
		t.Fatalf("QueryAccessEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	if events[0].Seq != 2 || events[1].Seq != 1 {
		t.Fatalf("export seqs = [%d %d], want [2 1]", events[0].Seq, events[1].Seq)
	}

	csv := RenderCSV(events)
	if !strings.Contains(csv, "session_id,seq,workspace") {
		t.Fatalf("csv header missing seq: %q", csv)
	}
	if !strings.Contains(csv, ",session-1,2,") || !strings.Contains(csv, ",session-1,1,") {
		t.Fatalf("csv rows missing seq values: %q", csv)
	}
}

func TestRecordAccessDirect_RoundTripsRouteAndBudgetMetadata(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	event := model.AccessEvent{
		OccurredAt:   time.Now(),
		ClientName:   "test-cli",
		SessionID:    "session-budget",
		Workspace:    "multi-tedi",
		Operation:    "nav.ask",
		Backend:      "ask",
		Route:        "daemon",
		Format:       "toon",
		TokenBudget:  2048,
		MaxItems:     5,
		MaxChars:     800,
		Compress:     true,
		Success:      true,
		LatencyMs:    12,
		ResultCount:  2,
		Truncated:    true,
		EntrypointID: "worker-dotnet::default",
	}
	if err := store.RecordAccessDirect(event); err != nil {
		t.Fatalf("RecordAccessDirect: %v", err)
	}

	events, err := QueryAccessEvents(store, ExportQuery{Limit: 10})
	if err != nil {
		t.Fatalf("QueryAccessEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	got := events[0]
	if got.Route != "daemon" {
		t.Fatalf("route = %q, want daemon", got.Route)
	}
	if got.Format != "toon" {
		t.Fatalf("format = %q, want toon", got.Format)
	}
	if got.TokenBudget != 2048 || got.MaxItems != 5 || got.MaxChars != 800 {
		t.Fatalf("budget metadata = (%d,%d,%d), want (2048,5,800)", got.TokenBudget, got.MaxItems, got.MaxChars)
	}
	if !got.Compress {
		t.Fatal("compress = false, want true")
	}

	csv := RenderCSV(events)
	if !strings.Contains(csv, "route,format,token_budget,max_items,max_chars,compress") {
		t.Fatalf("csv header missing telemetry metadata: %q", csv)
	}
	if !strings.Contains(csv, ",daemon,toon,2048,5,800,true,") {
		t.Fatalf("csv row missing telemetry metadata: %q", csv)
	}
}

func TestRecordAccess_RoundTripsSearchRoutingDiagnostics(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	runID, err := store.StartRun(model.DaemonState{
		PID:       1234,
		Endpoint:  "test-endpoint",
		StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	event := model.AccessEvent{
		OccurredAt:       time.Now(),
		ClientName:       "daemon-cli",
		SessionID:        "session-daemon",
		Workspace:        "multi-tedi",
		Operation:        "nav.search",
		Backend:          "text",
		Route:            "daemon",
		Format:           "toon",
		Success:          true,
		LatencyMs:        12,
		Warnings:         []string{"rerun with --regex"},
		WarningCount:     1,
		PatternMode:      "regex",
		RoutingOutcome:   "direct",
		FailureStage:     "none",
		HintCode:         "search_timeout",
		Truncated:        true,
		ResultCount:      3,
		TruncationReason: "token_budget",
		DecisionJSON:     `{"pattern_len":14,"used_regex":true}`,
		EntrypointID:     "worker-dotnet::default",
	}
	if err := store.RecordAccess(runID, event); err != nil {
		t.Fatalf("RecordAccess: %v", err)
	}

	events, err := QueryAccessEvents(store, ExportQuery{SessionID: "session-daemon", Limit: 10})
	if err != nil {
		t.Fatalf("QueryAccessEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}

	got := events[0]
	if got.WarningCount != 1 {
		t.Fatalf("warning_count = %d, want 1", got.WarningCount)
	}
	if got.PatternMode != "regex" {
		t.Fatalf("pattern_mode = %q, want regex", got.PatternMode)
	}
	if got.RoutingOutcome != "direct" {
		t.Fatalf("routing_outcome = %q, want direct", got.RoutingOutcome)
	}
	if got.FailureStage != "none" {
		t.Fatalf("failure_stage = %q, want none", got.FailureStage)
	}
	if got.HintCode != "search_timeout" {
		t.Fatalf("hint_code = %q, want search_timeout", got.HintCode)
	}
	if got.TruncationReason != "token_budget" {
		t.Fatalf("truncation_reason = %q, want token_budget", got.TruncationReason)
	}
	if got.DecisionJSON != `{"pattern_len":14,"used_regex":true}` {
		t.Fatalf("decision_json = %q, want exact round-trip", got.DecisionJSON)
	}
}

func TestQueryAccessEvents_FiltersBySearchRoutingDiagnostics(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	events := []model.AccessEvent{
		{
			OccurredAt:       time.Now(),
			ClientName:       "codex",
			SessionID:        "session-search",
			Workspace:        "multi-tedi",
			Operation:        "nav.search",
			Backend:          "text",
			Route:            "direct",
			Format:           "toon",
			Success:          true,
			LatencyMs:        30,
			WarningCount:     1,
			PatternMode:      "literal",
			RoutingOutcome:   "narrowed_repo",
			FailureStage:     "none",
			HintCode:         "regex_suspected",
			Truncated:        true,
			ResultCount:      1,
			TruncationReason: "max_items",
		},
		{
			OccurredAt:       time.Now().Add(time.Second),
			ClientName:       "claude",
			SessionID:        "session-router",
			Workspace:        "multi-tedi",
			Operation:        "nav.find",
			Backend:          "router",
			Route:            "direct",
			Format:           "compact",
			Success:          false,
			LatencyMs:        12,
			WarningCount:     1,
			PatternMode:      "none",
			RoutingOutcome:   "router_error",
			FailureStage:     "selector_validation",
			HintCode:         "repo_selector_invalid",
			Truncated:        false,
			ResultCount:      0,
			TruncationReason: "none",
		},
		{
			OccurredAt:       time.Now().Add(2 * time.Second),
			ClientName:       "codex",
			SessionID:        "session-daemon",
			Workspace:        "gastos",
			Operation:        "nav.ask",
			Backend:          "ask",
			Route:            "daemon",
			Format:           "json",
			Success:          true,
			LatencyMs:        80,
			WarningCount:     0,
			PatternMode:      "none",
			RoutingOutcome:   "direct",
			FailureStage:     "none",
			HintCode:         "",
			Truncated:        false,
			ResultCount:      2,
			TruncationReason: "none",
		},
	}
	for _, event := range events {
		if err := store.RecordAccessDirect(event); err != nil {
			t.Fatalf("RecordAccessDirect(%s): %v", event.Operation, err)
		}
	}

	testCases := []struct {
		name  string
		query ExportQuery
		want  string
	}{
		{name: "operation", query: ExportQuery{Operation: "nav.search", Limit: 10}, want: "session-search"},
		{name: "session", query: ExportQuery{SessionID: "session-router", Limit: 10}, want: "session-router"},
		{name: "client", query: ExportQuery{ClientName: "claude", Limit: 10}, want: "session-router"},
		{name: "route", query: ExportQuery{Route: "daemon", Limit: 10}, want: "session-daemon"},
		{name: "format", query: ExportQuery{Format: "compact", Limit: 10}, want: "session-router"},
		{name: "truncated", query: ExportQuery{Truncated: boolPtr(true), Limit: 10}, want: "session-search"},
		{name: "pattern_mode", query: ExportQuery{PatternMode: "literal", Limit: 10}, want: "session-search"},
		{name: "routing_outcome", query: ExportQuery{RoutingOutcome: "router_error", Limit: 10}, want: "session-router"},
		{name: "failure_stage", query: ExportQuery{FailureStage: "selector_validation", Limit: 10}, want: "session-router"},
		{name: "hint_code", query: ExportQuery{HintCode: "repo_selector_invalid", Limit: 10}, want: "session-router"},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			filtered, err := QueryAccessEvents(store, tt.query)
			if err != nil {
				t.Fatalf("QueryAccessEvents: %v", err)
			}
			if len(filtered) != 1 {
				t.Fatalf("len(filtered) = %d, want 1", len(filtered))
			}
			if filtered[0].SessionID != tt.want {
				t.Fatalf("SessionID = %q, want %q", filtered[0].SessionID, tt.want)
			}
		})
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
	if recent[0].WarningCount != 0 {
		t.Fatalf("recent[0].WarningCount = %d, want 0", recent[0].WarningCount)
	}
	if recent[0].PatternMode != "none" {
		t.Fatalf("recent[0].PatternMode = %q, want none", recent[0].PatternMode)
	}
	if recent[0].FailureStage != "none" {
		t.Fatalf("recent[0].FailureStage = %q, want none", recent[0].FailureStage)
	}
}

func boolPtr(value bool) *bool {
	return &value
}
