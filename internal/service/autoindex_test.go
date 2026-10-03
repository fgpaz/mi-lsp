package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	oldDelay := corruptionConfirmDelay
	corruptionConfirmDelay = time.Millisecond
	t.Cleanup(func() { corruptionConfirmDelay = oldDelay })
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

func TestTextDeclarationMatcherKindsAndExact(t *testing.T) {
	tests := []struct {
		file, text, symbol string
		exact              bool
		wantKind           string
		wantOK             bool
	}{
		{"a.go", "func NewAllowlist(entries []PeerChat) Allowlist {", "NewAllowlist", true, "func", true},
		{"a.go", "func (a *Allowlist) Allow(id int) bool {", "Allow", true, "method", true},
		{"a.go", "type Widget struct {", "Widget", true, "type", true},
		{"a.go", "type Reader interface {", "Reader", true, "interface", true},
		{"a.go", "const Limit = 3", "Limit", true, "const", true},
		{"a.go", "\treturn NewAllowlist(nil)", "NewAllowlist", true, "", false},
		{"a.go", "func NewAllowlistFrom(x int) {", "NewAllowlist", true, "", false},
		{"a.go", "func NewAllowlistFrom(x int) {", "NewAllowlist", false, "func", true},
		{"a.ts", "export function load(x: number) {", "load", true, "func", true},
		{"a.ts", "export interface Options {", "Options", true, "interface", true},
		{"a.ts", "  async fetch(url: string) {", "fetch", true, "method", true},
		{"a.ts", "  fetch(url);", "fetch", true, "", false},
		{"a.cs", "public sealed record Person(string Name);", "Person", true, "record", true},
		{"a.cs", "    public void Greet() { }", "Greet", true, "method", true},
		{"a.cs", "    public string Title { get; set; }", "Title", true, "property", true},
		{"a.py", "    async def run(self):", "run", true, "func", true},
		{"a.py", "class Job:", "Job", true, "class", true},
		{"a.py", "x = Job()", "Job", true, "", false},
		{"a.md", "func NewAllowlist(", "NewAllowlist", true, "", false},
	}
	for _, tt := range tests {
		matcher := newTextDeclarationMatcher(tt.symbol, tt.exact)
		_, kind, ok := matcher.match(tt.file, tt.text)
		if ok != tt.wantOK || kind != tt.wantKind {
			t.Errorf("match(%q, %q, exact=%v) = %q,%v want %q,%v", tt.file, tt.text, tt.exact, kind, ok, tt.wantKind, tt.wantOK)
		}
	}
}

func TestFind_TextFallbackReturnsOnlyDeclarations(t *testing.T) {
	root, name := setupTestWorkspace(t)
	writeWorkspaceFile(t, root, "pkg/allow.go", "package pkg\n\nfunc NewAllowlist(n int) int {\n\treturn n\n}\n")
	writeWorkspaceFile(t, root, "pkg/use.go", "package pkg\n\nfunc use() int {\n\treturn NewAllowlist(1)\n}\n")
	writeWorkspaceFile(t, root, ".docs/wiki/note.go", "func NewAllowlist(x int) {}\n")
	writeWorkspaceFile(t, root, "README.md", "call NewAllowlist here\n")
	app := New(root, nil)

	find := func(pattern string, exact bool) []map[string]any {
		env, err := app.Execute(context.Background(), model.CommandRequest{
			Operation: "nav.find",
			Context:   model.QueryOptions{Workspace: name, MaxItems: 10},
			Payload:   map[string]any{"pattern": pattern, "exact": exact},
		})
		if err != nil || !env.Ok {
			t.Fatalf("nav.find: env=%+v err=%v", env, err)
		}
		items, _ := env.Items.([]map[string]any)
		return items
	}

	items := find("NewAllowlist", true)
	if len(items) != 1 || items[0]["file"] != "pkg/allow.go" || items[0]["kind"] != "func" || items[0]["name"] != "NewAllowlist" || items[0]["origin"] != model.ItemOriginText {
		t.Fatalf("exact declaration items = %#v, want only pkg/allow.go func", items)
	}

	// No declaration: plain occurrences, code files only (no README, no .docs).
	items = find("use", true)
	if len(items) != 1 || items[0]["file"] != "pkg/use.go" {
		t.Fatalf("declaration of use = %#v", items)
	}
	items = find("n", true)
	for _, item := range items {
		if item["kind"] != "text_match" && item["kind"] != "func" {
			t.Fatalf("unexpected item %#v", item)
		}
		if f := item["file"].(string); strings.HasSuffix(f, ".md") || strings.HasPrefix(f, ".docs/") {
			t.Fatalf("non-code file leaked: %#v", item)
		}
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

func TestMissingTableIsNotCorruption(t *testing.T) {
	missing := errors.New("SQL logic error: no such table: symbols (1)")
	if isIndexCorruptionError(missing) {
		t.Fatal("a missing table must not justify quarantine")
	}
	if !isIndexSchemaBrokenError(missing) {
		t.Fatal("a missing table is still a broken schema for reads")
	}
	if !isIndexCorruptionError(errors.New("database disk image is malformed")) {
		t.Fatal("malformed must count as corruption")
	}
}

func TestCatalogHasNoTables(t *testing.T) {
	root := t.TempDir()
	if !catalogHasNoTables(root) {
		t.Fatal("a missing db counts as having no tables")
	}
	if err := os.MkdirAll(filepath.Join(root, ".mi-lsp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.WorkspaceDBPath(root), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if !catalogHasNoTables(root) {
		t.Fatal("zero-byte db must count as having no tables")
	}
	if !workspaceDBReadable(root) {
		t.Fatal("zero-byte db is a valid empty database")
	}

	if err := os.WriteFile(store.WorkspaceDBPath(root), []byte(strings.Repeat("garbage ", 500)), 0o644); err != nil {
		t.Fatal(err)
	}
	if catalogHasNoTables(root) || workspaceDBReadable(root) {
		t.Fatal("a corrupt file is neither empty nor readable")
	}

	healthy := t.TempDir()
	db, err := store.Open(healthy)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if catalogHasNoTables(healthy) {
		t.Fatal("a database with a schema has tables")
	}
	if !workspaceDBReadable(healthy) {
		t.Fatal("a fresh schema database is readable")
	}
}

func TestFind_EmptyIndexDBIsIndexNotReady(t *testing.T) {
	root, name := setupTestWorkspace(t)
	if err := os.MkdirAll(filepath.Join(root, ".mi-lsp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.WorkspaceDBPath(root), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.find",
		Context:   model.QueryOptions{Workspace: name, MaxItems: 10},
		Payload:   map[string]any{"pattern": "HelloWorld", "exact": true},
	})
	if err != nil {
		t.Fatalf("nav.find: %v", err)
	}
	if !env.Ok || env.Reason != model.ReasonIndexNotReady {
		t.Fatalf("envelope = %+v, want ok with reason index_not_ready", env)
	}
}

func TestTriggerAutoIndexTransientOpenErrorDoesNotBackOff(t *testing.T) {
	root := t.TempDir()
	t.Setenv(autoIndexEnvEnable, "1")
	// A directory in place of index.db makes the open fail without being corruption.
	if err := os.MkdirAll(store.WorkspaceDBPath(root), 0o755); err != nil {
		t.Fatal(err)
	}
	app := New(root, nil)
	got := app.triggerAutoIndex(context.Background(), model.WorkspaceRegistration{Name: "x", Root: root})
	if got != autoIndexOutcomeSkipped {
		t.Fatalf("trigger = %q, want skipped", got)
	}
	if _, ok := readAutoIndexMarker(root); ok {
		t.Fatal("a non-corruption open error must not write a failed marker")
	}
}

func TestQuarantinePruneGroupsSetsByTimestamp(t *testing.T) {
	stateDir := t.TempDir()
	for _, stamp := range []string{"10", "20", "30"} {
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if err := os.WriteFile(filepath.Join(stateDir, "index.db.corrupt-"+stamp+suffix), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	pruneQuarantinedDBs(stateDir, 2)
	entries, _ := os.ReadDir(stateDir)
	if len(entries) != 6 {
		t.Fatalf("kept %d files, want 2 full sets (6 files)", len(entries))
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(filepath.Join(stateDir, "index.db.corrupt-10"+suffix)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("oldest set file with suffix %q must be removed", suffix)
		}
	}
}

func TestTextDeclarationNameIsMatchedIdentifier(t *testing.T) {
	tests := []struct {
		file, text, symbol string
		exact              bool
		want               string
	}{
		{"a.go", "func New(config Config) *Server {", "New(config", true, "New"},
		{"a.go", "type apiResponse[T any] struct {", "apiResponse[T", false, "apiResponse"},
		{"a.go", "func NewAllowlistFrom(x int) {", "NewAllowlist", false, "NewAllowlistFrom"},
		{"a.go", "func NewAllowlist(x int) {", "NewAllowlist", true, "NewAllowlist"},
	}
	for _, tt := range tests {
		name, _, ok := newTextDeclarationMatcher(tt.symbol, tt.exact).match(tt.file, tt.text)
		if !ok || name != tt.want {
			t.Errorf("match(%q, %q) name = %q ok=%v, want %q", tt.text, tt.symbol, name, ok, tt.want)
		}
	}
}

func TestClassifyCatalogUnavailableUsesRootForEmptyDB(t *testing.T) {
	missingTable := errors.New("no such table: symbols")
	empty := t.TempDir()
	if err := os.MkdirAll(filepath.Join(empty, ".mi-lsp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.WorkspaceDBPath(empty), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := classifyCatalogUnavailable(withCatalogRoot(empty, missingTable)); got != model.ReasonIndexNotReady {
		t.Fatalf("empty db = %q, want index_not_ready", got)
	}
	if got := classifyCatalogUnavailable(withCatalogRoot(t.TempDir(), missingTable)); got != model.ReasonIndexNotReady {
		t.Fatalf("missing db = %q, want index_not_ready", got)
	}
	if got := classifyCatalogUnavailable(&workspaceDBOpenError{cause: missingTable, root: empty}); got != model.ReasonIndexNotReady {
		t.Fatalf("open error on empty db = %q, want index_not_ready", got)
	}

	partial := t.TempDir()
	db, err := store.Open(partial)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DROP TABLE symbols"); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if got := classifyCatalogUnavailable(withCatalogRoot(partial, missingTable)); got != model.ReasonIndexSchemaBroken {
		t.Fatalf("partial schema = %q, want index_schema_broken", got)
	}

	corrupt := t.TempDir()
	if err := os.MkdirAll(filepath.Join(corrupt, ".mi-lsp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.WorkspaceDBPath(corrupt), []byte(strings.Repeat("garbage ", 500)), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := classifyCatalogUnavailable(withCatalogRoot(corrupt, errors.New("file is not a database"))); got != model.ReasonIndexSchemaBroken {
		t.Fatalf("corrupt db = %q, want index_schema_broken", got)
	}
	// Without a root the classification keeps its previous behavior.
	if got := classifyCatalogUnavailable(missingTable); got != model.ReasonIndexSchemaBroken {
		t.Fatalf("rootless = %q, want index_schema_broken", got)
	}
}

func TestQuarantineLockIsExclusiveAndExpires(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".mi-lsp"), 0o755); err != nil {
		t.Fatal(err)
	}
	release, ok := acquireQuarantineLock(root)
	if !ok {
		t.Fatal("first acquire must succeed")
	}
	if _, second := acquireQuarantineLock(root); second {
		t.Fatal("second acquire must fail while held")
	}
	release()
	releaseAgain, ok := acquireQuarantineLock(root)
	if !ok {
		t.Fatal("acquire after release must succeed")
	}
	releaseAgain()

	// A stale lock is taken over.
	lockPath := filepath.Join(root, ".mi-lsp", quarantineLockName)
	if err := os.WriteFile(lockPath, []byte("999999"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * quarantineLockStale)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}
	releaseStale, ok := acquireQuarantineLock(root)
	if !ok {
		t.Fatal("a stale lock must be taken over")
	}
	releaseStale()
}

func TestOpenWorkspaceDBForIndexDoesNotRenameWhenLockHeld(t *testing.T) {
	oldDelay := corruptionConfirmDelay
	corruptionConfirmDelay = time.Millisecond
	t.Cleanup(func() { corruptionConfirmDelay = oldDelay })
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".mi-lsp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.WorkspaceDBPath(root), []byte(strings.Repeat("garbage ", 500)), 0o644); err != nil {
		t.Fatal(err)
	}
	release, ok := acquireQuarantineLock(root)
	if !ok {
		t.Fatal("lock")
	}
	defer release()
	if _, err := openWorkspaceDBForIndex(model.WorkspaceRegistration{Name: "x", Root: root}, "test"); err == nil {
		t.Fatal("a process without the lock must not repair the database")
	}
	if matches, _ := filepath.Glob(store.WorkspaceDBPath(root) + ".corrupt-*"); len(matches) != 0 {
		t.Fatalf("quarantined without the lock: %v", matches)
	}
}

func TestConcurrentColdStartNeverQuarantinesFreshDB(t *testing.T) {
	oldDelay := corruptionConfirmDelay
	corruptionConfirmDelay = 2 * time.Millisecond
	t.Cleanup(func() { corruptionConfirmDelay = oldDelay })
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".mi-lsp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.WorkspaceDBPath(root), []byte(strings.Repeat("garbage ", 500)), 0o644); err != nil {
		t.Fatal(err)
	}
	registration := model.WorkspaceRegistration{Name: "x", Root: root}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if db, err := openWorkspaceDBForIndex(registration, "test"); err == nil {
				_ = db.Close()
			}
		}()
	}
	wg.Wait()
	matches, _ := filepath.Glob(store.WorkspaceDBPath(root) + ".corrupt-*")
	if len(matches) != 1 {
		t.Fatalf("quarantine files = %v, want exactly the one corrupt original", matches)
	}
	if !workspaceDBReadable(root) {
		t.Fatal("the surviving index.db must be the healthy fresh one")
	}
}

func TestTextDeclarationNameFallsBackToStrippedIdentifier(t *testing.T) {
	name, _, ok := newTextDeclarationMatcher("(a", true).match("a.go", "func (a *App) find() {")
	if ok && name != "a" {
		t.Fatalf("name = %q, want a valid identifier", name)
	}
	if got := identifierAt("func (a *App) find()", 5, "(a", "(a"); got != "a" {
		t.Fatalf("identifierAt = %q, want %q", got, "a")
	}
	if got := identifierAt("x (", 2, "(", "("); got != "" {
		t.Fatalf("identifierAt without identifier = %q, want empty", got)
	}
}

func TestCrossWorkspaceReadDoesNotStartReindexWithoutOverride(t *testing.T) {
	root, name := setupTestWorkspace(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
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
	request := model.CommandRequest{Operation: "nav.find", Context: model.QueryOptions{
		Workspace: name, CallerCWD: t.TempDir(), ClientName: "codex", CrossWorkspaceRead: true,
	}}
	logPath := filepath.Join(home, ".mi-lsp", crossWorkspaceOverrideLog)

	got := app.crossWorkspaceAwareAutoIndex(context.Background(), registration, request)
	if !strings.HasPrefix(got, autoIndexOutcomeSkipped) || !strings.Contains(got, "cross-workspace read") {
		t.Fatalf("outcome = %q, want skipped for cross-workspace read", got)
	}
	if spawned != 0 {
		t.Fatalf("spawned %d reindex processes, want 0", spawned)
	}
	if _, err := os.Stat(logPath); err == nil {
		t.Fatal("a skipped reindex must not record an override")
	}

	request.Context.AllowCrossWorkspace = true
	if got := app.crossWorkspaceAwareAutoIndex(context.Background(), registration, request); got != autoIndexOutcomeStarted {
		t.Fatalf("outcome with override = %q, want started", got)
	}
	if spawned != 1 {
		t.Fatalf("spawned %d reindex processes with override, want 1", spawned)
	}
	data, err := os.ReadFile(logPath)
	if err != nil || !strings.Contains(string(data), `"operation":"`+autoIndexOverrideOperation+`"`) {
		t.Fatalf("override log = %s (err %v), want %s entry", data, err, autoIndexOverrideOperation)
	}
}
