package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

func TestClassifyCatalogUnavailable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"missing table", errors.New("SQL logic error: no such table: symbols (1)"), model.ReasonIndexSchemaBroken},
		{"missing column", errors.New("no such column: foo"), model.ReasonIndexSchemaBroken},
		{"malformed", errors.New("database disk image is malformed (11)"), model.ReasonIndexSchemaBroken},
		{"not a database", errors.New("file is not a database (26)"), model.ReasonIndexSchemaBroken},
		{"wrapped open error", &workspaceDBOpenError{cause: errors.New("file is not a database")}, model.ReasonIndexSchemaBroken},
		{"generic open error", &workspaceDBOpenError{cause: errors.New("permission denied")}, model.ReasonIndexNotReady},
		{"locked", errors.New("database is locked"), model.ReasonIndexNotReady},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyCatalogUnavailable(tt.err); got != tt.want {
				t.Fatalf("classifyCatalogUnavailable() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAutoIndexEnabledHonorsOverride(t *testing.T) {
	for value, want := range map[string]bool{"": true, "1": true, "0": false, "false": false, "OFF": false} {
		t.Setenv(autoIndexEnvEnable, value)
		if got := autoIndexEnabled(); got != want {
			t.Fatalf("autoIndexEnabled() with %q = %v, want %v", value, got, want)
		}
	}
}

func TestAutoIndexTimeoutEnv(t *testing.T) {
	t.Setenv(autoIndexEnvTimeout, "")
	if got := autoIndexTimeout(); got != autoIndexDefaultTimeout {
		t.Fatalf("default timeout = %v", got)
	}
	t.Setenv(autoIndexEnvTimeout, "90s")
	if got := autoIndexTimeout(); got != 90*time.Second {
		t.Fatalf("timeout = %v", got)
	}
	t.Setenv(autoIndexEnvTimeout, "garbage")
	if got := autoIndexTimeout(); got != autoIndexDefaultTimeout {
		t.Fatalf("invalid timeout = %v, want default", got)
	}
}

func TestAutoIndexRateLimitMarker(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	if autoIndexRateLimited(root, now) {
		t.Fatal("no marker must not rate limit")
	}
	writeAutoIndexMarker(root, autoIndexStatusStarted, now)
	if autoIndexRateLimited(root, now) {
		t.Fatal("a started marker must not rate limit")
	}
	writeAutoIndexMarker(root, autoIndexStatusFailed, now.Add(-5*time.Minute))
	if !autoIndexRateLimited(root, now) {
		t.Fatal("a failure 5m ago must rate limit")
	}
	writeAutoIndexMarker(root, autoIndexStatusFailed, now.Add(-11*time.Minute))
	if autoIndexRateLimited(root, now) {
		t.Fatal("a failure 11m ago must not rate limit")
	}
	writeAutoIndexMarker(root, autoIndexStatusSucceeded, now)
	if autoIndexRateLimited(root, now) {
		t.Fatal("a success must not rate limit")
	}
}

func TestQuarantineWorkspaceDBKeepsTwoNewest(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, ".mi-lsp")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := store.WorkspaceDBPath(root)
	base := time.Unix(1_700_000_000, 0)
	for i := 0; i < 4; i++ {
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if err := os.WriteFile(dbPath+suffix, []byte("broken"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := quarantineWorkspaceDB(root, base.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("quarantine %d: %v", i, err)
		}
	}
	// An unrelated file must survive.
	other := filepath.Join(stateDir, "autoindex.last")
	if err := os.WriteFile(other, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	pruneQuarantinedDBs(stateDir, maxQuarantinedDBs)

	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, entry := range entries {
		names[entry.Name()] = true
	}
	want := []string{
		"index.db.corrupt-1700000002", "index.db.corrupt-1700000002-wal", "index.db.corrupt-1700000002-shm",
		"index.db.corrupt-1700000003", "index.db.corrupt-1700000003-wal", "index.db.corrupt-1700000003-shm",
		"autoindex.last",
	}
	if len(names) != len(want) {
		t.Fatalf("state dir = %v, want only %v", names, want)
	}
	for _, name := range want {
		if !names[name] {
			t.Fatalf("missing %s in %v", name, names)
		}
	}
	if _, err := os.Stat(dbPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("index.db must have been moved away, stat err = %v", err)
	}
}

func TestOpenWorkspaceDBForIndexQuarantinesCorruptDB(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".mi-lsp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.WorkspaceDBPath(root), []byte(strings.Repeat("this is not a sqlite database ", 200)), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := openWorkspaceDBForIndex(model.WorkspaceRegistration{Name: "x", Root: root}, "test")
	if err != nil {
		t.Fatalf("openWorkspaceDBForIndex: %v", err)
	}
	_ = db.Close()
	matches, _ := filepath.Glob(store.WorkspaceDBPath(root) + ".corrupt-*")
	if len(matches) != 1 {
		t.Fatalf("quarantine files = %v, want exactly one", matches)
	}
}

func TestTriggerAutoIndexDedupesAndRateLimits(t *testing.T) {
	root, name := setupTestWorkspace(t)
	t.Setenv(autoIndexEnvEnable, "1")
	registration := model.WorkspaceRegistration{Name: name, Root: root}
	spawned := 0
	old := spawnDetachedAutoIndexJobProcess
	spawnDetachedAutoIndexJobProcess = func(model.WorkspaceRegistration, string) (int, error) {
		spawned++
		return os.Getpid(), nil
	}
	t.Cleanup(func() { spawnDetachedAutoIndexJobProcess = old })
	app := New(root, nil)

	if got := app.triggerAutoIndex(context.Background(), registration); got != autoIndexOutcomeStarted {
		t.Fatalf("first trigger = %q", got)
	}
	if got := app.triggerAutoIndex(context.Background(), registration); got != autoIndexOutcomeAlreadyRunning {
		t.Fatalf("second trigger = %q, want dedupe", got)
	}
	if spawned != 1 {
		t.Fatalf("spawned %d processes, want 1", spawned)
	}

	writeAutoIndexMarker(root, autoIndexStatusFailed, time.Now())
	if got := app.triggerAutoIndex(context.Background(), registration); got != autoIndexOutcomeSkipped {
		t.Fatalf("trigger after failure = %q, want skipped", got)
	}

	t.Setenv(autoIndexEnvEnable, "0")
	writeAutoIndexMarker(root, autoIndexStatusSucceeded, time.Now())
	if got := app.triggerAutoIndex(context.Background(), registration); got != autoIndexOutcomeSkipped {
		t.Fatalf("trigger when disabled = %q, want skipped", got)
	}
}

func TestFind_SchemaBrokenCatalogServesTextAndStartsReindex(t *testing.T) {
	root, name := setupTestWorkspace(t)
	t.Setenv(autoIndexEnvEnable, "1")
	spawned := 0
	old := spawnDetachedAutoIndexJobProcess
	spawnDetachedAutoIndexJobProcess = func(model.WorkspaceRegistration, string) (int, error) {
		spawned++
		return os.Getpid(), nil
	}
	t.Cleanup(func() { spawnDetachedAutoIndexJobProcess = old })

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.UpsertWorkspaceMetaMap(context.Background(), db, map[string]string{
		store.WorkspaceMetaIndexedAt:  "2026-10-01T00:00:00Z",
		store.WorkspaceMetaTotalFiles: "1",
	}); err != nil {
		t.Fatalf("meta: %v", err)
	}
	if _, err := db.Exec("DROP TABLE symbols"); err != nil {
		t.Fatalf("drop symbols: %v", err)
	}
	_ = db.Close()

	env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.find",
		Context:   model.QueryOptions{Workspace: name, MaxItems: 10},
		Payload:   map[string]any{"pattern": "Greet", "exact": true},
	})
	if err != nil {
		t.Fatalf("nav.find: %v", err)
	}
	items, _ := env.Items.([]map[string]any)
	if !env.Ok || !env.Degraded || env.Reason != model.ReasonIndexSchemaBroken || len(items) == 0 || items[0]["origin"] != model.ItemOriginText {
		t.Fatalf("envelope = %+v", env)
	}
	if len(env.Warnings) == 0 || !strings.Contains(env.Warnings[0], "background reindex started") {
		t.Fatalf("warnings = %v", env.Warnings)
	}
	if spawned != 1 {
		t.Fatalf("spawned = %d, want 1", spawned)
	}
}

func TestTextFindItemsRanksDeclarationsFirst(t *testing.T) {
	hits := []map[string]any{
		{"file": "a.go", "line": 3, "text": "\treturn Widget{}"},
		{"file": "b.go", "line": 1, "text": "type Widget struct {"},
		{"file": "c.cs", "line": 9, "text": "    public static void Widget(int x)"},
	}
	items := textFindItems("Widget", hits)
	if len(items) != 3 || items[0]["file"] != "b.go" || items[1]["file"] != "c.cs" || items[2]["file"] != "a.go" {
		t.Fatalf("ranking = %#v", items)
	}
}

func TestWordBoundaryPattern(t *testing.T) {
	if got := wordBoundaryPattern("Foo"); got != `\bFoo\b` {
		t.Fatalf("got %q", got)
	}
	if got := wordBoundaryPattern("operator+"); got != `\boperator\+` {
		t.Fatalf("got %q", got)
	}
}
