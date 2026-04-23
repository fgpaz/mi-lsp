package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func seedTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	root := t.TempDir()
	db, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, root
}

func TestOpen_CreatesSchema(t *testing.T) {
	db, _ := seedTestDB(t)
	// verify tables exist
	tables := []string{"symbols", "files", "workspace_repos", "workspace_entrypoints", "workspace_meta", "index_jobs", "index_generations"}
	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestReplaceWorkspaceIndexPublishesGenerationMetadata(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	job, err := CreateIndexJob(ctx, db, "test", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "test", Kind: "single"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}
	files := []model.FileRecord{{FilePath: "src/Foo.cs", RepoID: "main", RepoName: "main", Language: "csharp"}}
	symbols := []model.SymbolRecord{{FilePath: "src/Foo.cs", RepoID: "main", RepoName: "main", Name: "Foo", Kind: "class", StartLine: 1, EndLine: 2, QualifiedName: "Foo", Language: "csharp"}}
	docs := []model.DocRecord{{Path: ".docs/wiki/04_RF/RF-IDX-001.md", Title: "RF-IDX-001", DocID: "RF-IDX-001", Layer: "04", Family: "functional", SearchText: "indexing"}}
	snapshot := model.ReentryMemorySnapshot{SnapshotBuiltAt: time.Now()}

	if err := ReplaceWorkspaceIndex(ctx, db, job.GenerationID, project, files, symbols, docs, nil, nil, snapshot); err != nil {
		t.Fatalf("ReplaceWorkspaceIndex: %v", err)
	}
	for _, key := range []string{WorkspaceMetaActiveCatalogGeneration, WorkspaceMetaActiveDocsGeneration, WorkspaceMetaActiveMemoryGeneration, WorkspaceMetaLastIndexGeneration} {
		value, ok, err := WorkspaceMetaValue(ctx, db, key)
		if err != nil {
			t.Fatalf("WorkspaceMetaValue(%s): %v", key, err)
		}
		if !ok || value != job.GenerationID {
			t.Fatalf("metadata %s = %q ok=%v, want %q", key, value, ok, job.GenerationID)
		}
	}
	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM index_generations WHERE generation_id = ?", job.GenerationID).Scan(&status); err != nil {
		t.Fatalf("generation status query: %v", err)
	}
	if status != "published" {
		t.Fatalf("generation status = %q, want published", status)
	}
}

func TestUpsertAndQuerySymbols(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "test", Kind: "single"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}
	files := []model.FileRecord{
		{FilePath: "src/Foo.cs", RepoID: "main", RepoName: "main", Language: "csharp"},
	}
	symbols := []model.SymbolRecord{
		{FilePath: "src/Foo.cs", RepoID: "main", RepoName: "main", Name: "FooClass", Kind: "class", StartLine: 5, EndLine: 20, QualifiedName: "Ns.FooClass", Language: "csharp"},
		{FilePath: "src/Foo.cs", RepoID: "main", RepoName: "main", Name: "Bar", Kind: "method", StartLine: 10, EndLine: 15, QualifiedName: "Ns.FooClass.Bar", Language: "csharp"},
	}
	if err := ReplaceCatalog(ctx, db, project, files, symbols); err != nil {
		t.Fatalf("ReplaceCatalog: %v", err)
	}

	// SymbolsByFile
	got, err := SymbolsByFile(ctx, db, "src/Foo.cs", 100, 0)
	if err != nil {
		t.Fatalf("SymbolsByFile: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("SymbolsByFile: want 2 symbols, got %d", len(got))
	}

	// FindSymbols exact
	found, err := FindSymbols(ctx, db, "FooClass", "", true, 10, 0)
	if err != nil {
		t.Fatalf("FindSymbols exact: %v", err)
	}
	if len(found) != 1 || found[0].Name != "FooClass" {
		t.Errorf("FindSymbols exact: want FooClass, got %v", found)
	}

	// FindSymbols fuzzy
	fuzzy, err := FindSymbols(ctx, db, "Foo", "", false, 10, 0)
	if err != nil {
		t.Fatalf("FindSymbols fuzzy: %v", err)
	}
	if len(fuzzy) != 1 {
		t.Errorf("FindSymbols fuzzy: want 1 (FooClass), got %d", len(fuzzy))
	}
}

func TestWorkspaceStats(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "test", Kind: "single"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}
	files := []model.FileRecord{
		{FilePath: "a.cs", RepoID: "main", Language: "csharp"},
		{FilePath: "b.cs", RepoID: "main", Language: "csharp"},
	}
	symbols := []model.SymbolRecord{
		{FilePath: "a.cs", RepoID: "main", Name: "A", Kind: "class", StartLine: 1, EndLine: 10, QualifiedName: "A", Language: "csharp"},
		{FilePath: "b.cs", RepoID: "main", Name: "B", Kind: "class", StartLine: 1, EndLine: 10, QualifiedName: "B", Language: "csharp"},
		{FilePath: "b.cs", RepoID: "main", Name: "C", Kind: "method", StartLine: 3, EndLine: 8, QualifiedName: "B.C", Language: "csharp"},
	}
	if err := ReplaceCatalog(ctx, db, project, files, symbols); err != nil {
		t.Fatalf("ReplaceCatalog: %v", err)
	}
	stats, err := WorkspaceStats(ctx, db)
	if err != nil {
		t.Fatalf("WorkspaceStats: %v", err)
	}
	if stats.Files != 2 {
		t.Errorf("want 2 files, got %d", stats.Files)
	}
	if stats.Symbols != 3 {
		t.Errorf("want 3 symbols, got %d", stats.Symbols)
	}
}

func TestCandidateReposForSymbol(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "test", Kind: "container"},
		Repos: []model.WorkspaceRepo{
			{ID: "api", Name: "api", Root: "api"},
			{ID: "web", Name: "web", Root: "web"},
		},
	}
	symbols := []model.SymbolRecord{
		{FilePath: "api/Svc.cs", RepoID: "api", RepoName: "api", Name: "UserService", Kind: "class", StartLine: 1, EndLine: 10, QualifiedName: "Api.UserService", Language: "csharp"},
		{FilePath: "web/Comp.tsx", RepoID: "web", RepoName: "web", Name: "UserService", Kind: "class", StartLine: 1, EndLine: 10, QualifiedName: "Web.UserService", Language: "typescript"},
	}
	if err := ReplaceCatalog(ctx, db, project, nil, symbols); err != nil {
		t.Fatalf("ReplaceCatalog: %v", err)
	}
	repos, err := CandidateReposForSymbol(ctx, db, "UserService", true, 10)
	if err != nil {
		t.Fatalf("CandidateReposForSymbol: %v", err)
	}
	if len(repos) != 2 {
		t.Errorf("want 2 repos, got %d", len(repos))
	}
}

func TestOverviewByPrefix(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "test", Kind: "single"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}
	files := []model.FileRecord{
		{FilePath: "src/foo/Bar.cs", RepoID: "main", Language: "csharp"},
		{FilePath: "src/foo/Baz.cs", RepoID: "main", Language: "csharp"},
		{FilePath: "src/bar/Qux.cs", RepoID: "main", Language: "csharp"},
	}
	symbols := []model.SymbolRecord{
		{FilePath: "src/foo/Bar.cs", RepoID: "main", Name: "BarClass", Kind: "class", StartLine: 1, EndLine: 10, QualifiedName: "BarClass", Language: "csharp"},
		{FilePath: "src/foo/Baz.cs", RepoID: "main", Name: "BazClass", Kind: "class", StartLine: 1, EndLine: 10, QualifiedName: "BazClass", Language: "csharp"},
		{FilePath: "src/bar/Qux.cs", RepoID: "main", Name: "QuxClass", Kind: "class", StartLine: 1, EndLine: 10, QualifiedName: "QuxClass", Language: "csharp"},
	}
	if err := ReplaceCatalog(ctx, db, project, files, symbols); err != nil {
		t.Fatalf("ReplaceCatalog: %v", err)
	}

	// Query by prefix "src/foo/"
	results, err := OverviewByPrefix(ctx, db, "src/foo/", 100, 0)
	if err != nil {
		t.Fatalf("OverviewByPrefix: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("want 2 symbols with src/foo/ prefix, got %d", len(results))
	}
	for _, sym := range results {
		if sym.FilePath != "src/foo/Bar.cs" && sym.FilePath != "src/foo/Baz.cs" {
			t.Errorf("unexpected file path: %s", sym.FilePath)
		}
	}
}

func TestFindSymbols_WithKindFilter(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "test", Kind: "single"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}
	symbols := []model.SymbolRecord{
		{FilePath: "test.cs", RepoID: "main", Name: "MyClass", Kind: "class", StartLine: 1, EndLine: 10, QualifiedName: "MyClass", Language: "csharp"},
		{FilePath: "test.cs", RepoID: "main", Name: "MyMethod", Kind: "method", StartLine: 5, EndLine: 8, QualifiedName: "MyClass.MyMethod", Language: "csharp"},
		{FilePath: "test.cs", RepoID: "main", Name: "MyField", Kind: "field", StartLine: 2, EndLine: 2, QualifiedName: "MyClass.MyField", Language: "csharp"},
	}
	if err := ReplaceCatalog(ctx, db, project, nil, symbols); err != nil {
		t.Fatalf("ReplaceCatalog: %v", err)
	}

	// Find all My*
	allMy, err := FindSymbols(ctx, db, "My", "", false, 10, 0)
	if err != nil {
		t.Fatalf("FindSymbols all: %v", err)
	}
	if len(allMy) != 3 {
		t.Errorf("want 3 symbols matching My, got %d", len(allMy))
	}

	// Find only methods
	onlyMethods, err := FindSymbols(ctx, db, "My", "method", false, 10, 0)
	if err != nil {
		t.Fatalf("FindSymbols method: %v", err)
	}
	if len(onlyMethods) != 1 {
		t.Errorf("want 1 method, got %d", len(onlyMethods))
	}
	if onlyMethods[0].Kind != "method" {
		t.Errorf("expected kind method, got %s", onlyMethods[0].Kind)
	}
}

func TestReplaceCatalog_ClearsOldData(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()

	// First insert
	project1 := model.ProjectFile{
		Project: model.ProjectBlock{Name: "test", Kind: "single"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}
	symbols1 := []model.SymbolRecord{
		{FilePath: "a.cs", RepoID: "main", Name: "A", Kind: "class", StartLine: 1, EndLine: 10, QualifiedName: "A", Language: "csharp"},
	}
	if err := ReplaceCatalog(ctx, db, project1, nil, symbols1); err != nil {
		t.Fatalf("First ReplaceCatalog: %v", err)
	}

	stats1, _ := WorkspaceStats(ctx, db)
	if stats1.Symbols != 1 {
		t.Errorf("after first insert: want 1 symbol, got %d", stats1.Symbols)
	}

	// Second insert (should clear old data)
	symbols2 := []model.SymbolRecord{
		{FilePath: "b.cs", RepoID: "main", Name: "B", Kind: "class", StartLine: 1, EndLine: 10, QualifiedName: "B", Language: "csharp"},
		{FilePath: "b.cs", RepoID: "main", Name: "C", Kind: "method", StartLine: 5, EndLine: 8, QualifiedName: "B.C", Language: "csharp"},
	}
	if err := ReplaceCatalog(ctx, db, project1, nil, symbols2); err != nil {
		t.Fatalf("Second ReplaceCatalog: %v", err)
	}

	stats2, _ := WorkspaceStats(ctx, db)
	if stats2.Symbols != 2 {
		t.Errorf("after second insert: want 2 symbols, got %d", stats2.Symbols)
	}

	// Verify old symbol is gone
	oldResults, _ := FindSymbols(ctx, db, "A", "", true, 10, 0)
	if len(oldResults) != 0 {
		t.Errorf("old symbol A should be gone, but found %d results", len(oldResults))
	}
}

func TestSymbolsByFile_Limit(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "test", Kind: "single"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}

	// Insert many symbols in one file
	symbols := make([]model.SymbolRecord, 10)
	for i := 0; i < 10; i++ {
		symbols[i] = model.SymbolRecord{
			FilePath:      "test.cs",
			RepoID:        "main",
			Name:          "Symbol" + string(rune('A'+i)),
			Kind:          "class",
			StartLine:     i*10 + 1,
			EndLine:       i*10 + 5,
			QualifiedName: "Symbol" + string(rune('A'+i)),
			Language:      "csharp",
		}
	}
	if err := ReplaceCatalog(ctx, db, project, nil, symbols); err != nil {
		t.Fatalf("ReplaceCatalog: %v", err)
	}

	// Query with limit
	limited, err := SymbolsByFile(ctx, db, "test.cs", 3, 0)
	if err != nil {
		t.Fatalf("SymbolsByFile: %v", err)
	}
	if len(limited) != 3 {
		t.Errorf("want 3 symbols (limited), got %d", len(limited))
	}

	// Query with no limit (should get all)
	all, err := SymbolsByFile(ctx, db, "test.cs", 0, 0)
	if err != nil {
		t.Fatalf("SymbolsByFile unlimited: %v", err)
	}
	if len(all) != 10 {
		t.Errorf("want 10 symbols (unlimited), got %d", len(all))
	}
}

func TestFindSymbols_NoResults(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "test", Kind: "single"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}
	symbols := []model.SymbolRecord{
		{FilePath: "test.cs", RepoID: "main", Name: "Foo", Kind: "class", StartLine: 1, EndLine: 10, QualifiedName: "Foo", Language: "csharp"},
	}
	if err := ReplaceCatalog(ctx, db, project, nil, symbols); err != nil {
		t.Fatalf("ReplaceCatalog: %v", err)
	}

	// Search for non-existent symbol
	results, err := FindSymbols(ctx, db, "NonExistent", "", true, 10, 0)
	if err != nil {
		t.Fatalf("FindSymbols: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("want 0 results, got %d", len(results))
	}
}

func TestCatalogQueries_WithOffset(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "test", Kind: "single"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}
	symbols := []model.SymbolRecord{
		{FilePath: "src/a.cs", RepoID: "main", Name: "Alpha", Kind: "class", StartLine: 1, EndLine: 2, QualifiedName: "Alpha", Language: "csharp", SearchText: "alpha first"},
		{FilePath: "src/b.cs", RepoID: "main", Name: "Beta", Kind: "class", StartLine: 3, EndLine: 4, QualifiedName: "Beta", Language: "csharp", SearchText: "beta second"},
		{FilePath: "src/c.cs", RepoID: "main", Name: "Gamma", Kind: "class", StartLine: 5, EndLine: 6, QualifiedName: "Gamma", Language: "csharp", SearchText: "gamma third"},
	}
	if err := ReplaceCatalog(ctx, db, project, nil, symbols); err != nil {
		t.Fatalf("ReplaceCatalog: %v", err)
	}

	byFile, err := SymbolsByFile(ctx, db, "src/c.cs", 10, 0)
	if err != nil {
		t.Fatalf("SymbolsByFile seed: %v", err)
	}
	if len(byFile) != 1 {
		t.Fatalf("SymbolsByFile seed len = %d, want 1", len(byFile))
	}

	found, err := FindSymbols(ctx, db, "", "", false, 1, 1)
	if err != nil {
		t.Fatalf("FindSymbols offset: %v", err)
	}
	if len(found) != 1 || found[0].Name != "Beta" {
		t.Fatalf("FindSymbols offset got %#v, want Beta", found)
	}

	overview, err := OverviewByPrefix(ctx, db, "src/", 1, 1)
	if err != nil {
		t.Fatalf("OverviewByPrefix offset: %v", err)
	}
	if len(overview) != 1 || overview[0].FilePath != "src/b.cs" {
		t.Fatalf("OverviewByPrefix offset got %#v, want src/b.cs", overview)
	}

	intent, err := IntentSearch(ctx, db, []string{"second", "third"}, 1, 1)
	if err != nil {
		t.Fatalf("IntentSearch offset: %v", err)
	}
	if len(intent) != 1 || intent[0].Name != "Gamma" {
		t.Fatalf("IntentSearch offset got %#v, want Gamma", intent)
	}
}

func TestSymbolQueries_HandleNullSearchText(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `
		INSERT INTO symbols(
			id, file_path, repo_id, repo_name, name, kind, start_line, end_line,
			parent, qualified_name, signature, signature_hash, scope, language,
			file_hash, implements, search_text
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
	`,
		1,
		".claude/scripts/ux_ui_exploration.py",
		"main",
		"main",
		"log_errors",
		"function",
		33,
		33,
		"",
		".claude/scripts/ux_ui_exploration.py::log_errors",
		"",
		"sig-hash",
		"module",
		"python",
		"file-hash",
		"",
	)
	if err != nil {
		t.Fatalf("insert symbol: %v", err)
	}

	found, err := FindSymbols(ctx, db, "log_errors", "", true, 10, 0)
	if err != nil {
		t.Fatalf("FindSymbols exact with NULL search_text: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("FindSymbols exact with NULL search_text: want 1 result, got %d", len(found))
	}
	if found[0].SearchText != "" {
		t.Fatalf("FindSymbols exact with NULL search_text: SearchText = %q, want empty string", found[0].SearchText)
	}

	byFile, err := SymbolsByFile(ctx, db, ".claude/scripts/ux_ui_exploration.py", 10, 0)
	if err != nil {
		t.Fatalf("SymbolsByFile with NULL search_text: %v", err)
	}
	if len(byFile) != 1 {
		t.Fatalf("SymbolsByFile with NULL search_text: want 1 result, got %d", len(byFile))
	}
	if byFile[0].SearchText != "" {
		t.Fatalf("SymbolsByFile with NULL search_text: SearchText = %q, want empty string", byFile[0].SearchText)
	}
}

func TestCandidateReposForSymbol_Fuzzy(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "test", Kind: "container"},
		Repos: []model.WorkspaceRepo{
			{ID: "repo1", Name: "repo1", Root: "repo1"},
			{ID: "repo2", Name: "repo2", Root: "repo2"},
		},
	}
	symbols := []model.SymbolRecord{
		{FilePath: "repo1/A.cs", RepoID: "repo1", RepoName: "repo1", Name: "FooService", Kind: "class", StartLine: 1, EndLine: 10, QualifiedName: "FooService", Language: "csharp"},
		{FilePath: "repo2/B.cs", RepoID: "repo2", RepoName: "repo2", Name: "FooBar", Kind: "class", StartLine: 1, EndLine: 10, QualifiedName: "FooBar", Language: "csharp"},
	}
	if err := ReplaceCatalog(ctx, db, project, nil, symbols); err != nil {
		t.Fatalf("ReplaceCatalog: %v", err)
	}

	// Fuzzy search for "Foo" should match both
	results, err := CandidateReposForSymbol(ctx, db, "Foo", false, 10)
	if err != nil {
		t.Fatalf("CandidateReposForSymbol: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("fuzzy Foo: want 2 repos, got %d", len(results))
	}

	// Exact search for "Foo" should match neither
	exact, err := CandidateReposForSymbol(ctx, db, "Foo", true, 10)
	if err != nil {
		t.Fatalf("CandidateReposForSymbol exact: %v", err)
	}
	if len(exact) != 0 {
		t.Errorf("exact Foo: want 0 repos, got %d", len(exact))
	}
}
func TestOpen_MigratesLegacyRepoColumns(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, ".mi-lsp")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	legacyDB, err := sql.Open(driverName, WorkspaceDBPath(root))
	if err != nil {
		t.Fatalf("sql.Open legacy: %v", err)
	}
	defer legacyDB.Close()
	legacyDDL := []string{
		`CREATE TABLE symbols (id INTEGER PRIMARY KEY, file_path TEXT NOT NULL, name TEXT NOT NULL, kind TEXT NOT NULL, start_line INTEGER NOT NULL, end_line INTEGER NOT NULL, parent TEXT, qualified_name TEXT NOT NULL DEFAULT '', signature TEXT, signature_hash TEXT, scope TEXT, language TEXT NOT NULL, file_hash TEXT, implements TEXT, UNIQUE(file_path, qualified_name, signature_hash, start_line))`,
		`CREATE TABLE files (file_path TEXT PRIMARY KEY, content_hash TEXT, indexed_at INTEGER, language TEXT)`,
		`CREATE TABLE workspace_meta (key TEXT PRIMARY KEY, value TEXT)`,
	}
	for _, stmt := range legacyDDL {
		if _, err := legacyDB.Exec(stmt); err != nil {
			t.Fatalf("legacy exec: %v", err)
		}
	}
	if _, err := legacyDB.Exec(`INSERT INTO files(file_path, language) VALUES ('legacy.cs', 'csharp')`); err != nil {
		t.Fatalf("insert legacy file: %v", err)
	}
	if _, err := legacyDB.Exec(`INSERT INTO symbols(file_path, name, kind, start_line, end_line, qualified_name, language) VALUES ('legacy.cs', 'LegacyType', 'class', 1, 4, 'LegacyType', 'csharp')`); err != nil {
		t.Fatalf("insert legacy symbol: %v", err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	migrated, err := Open(root)
	if err != nil {
		t.Fatalf("Open migrated: %v", err)
	}
	defer migrated.Close()

	stats, err := WorkspaceStats(context.Background(), migrated)
	if err != nil {
		t.Fatalf("WorkspaceStats after migration: %v", err)
	}
	if stats.Files != 1 || stats.Symbols != 1 {
		t.Fatalf("stats after migration = %#v, want files=1 symbols=1", stats)
	}

	for _, columnCheck := range []struct {
		table  string
		column string
	}{
		{table: "symbols", column: "repo_id"},
		{table: "symbols", column: "repo_name"},
		{table: "files", column: "repo_id"},
		{table: "files", column: "repo_name"},
	} {
		hasColumn, err := tableHasColumn(migrated, columnCheck.table, columnCheck.column)
		if err != nil {
			t.Fatalf("tableHasColumn(%s.%s): %v", columnCheck.table, columnCheck.column, err)
		}
		if !hasColumn {
			t.Fatalf("expected migrated column %s.%s", columnCheck.table, columnCheck.column)
		}
	}
}
