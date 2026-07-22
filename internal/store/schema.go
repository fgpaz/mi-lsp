package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const reposDDL = `
CREATE TABLE IF NOT EXISTS workspace_repos (
    repo_id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    root TEXT NOT NULL,
    languages TEXT,
    default_entrypoint TEXT
);
`

const entrypointsDDL = `
CREATE TABLE IF NOT EXISTS workspace_entrypoints (
    entrypoint_id TEXT PRIMARY KEY,
    repo_id TEXT NOT NULL,
    path TEXT NOT NULL,
    kind TEXT NOT NULL,
    is_default INTEGER NOT NULL DEFAULT 0,
    UNIQUE(repo_id, path)
);
`

const symbolsDDL = `
CREATE TABLE IF NOT EXISTS symbols (
    id INTEGER PRIMARY KEY,
    file_path TEXT NOT NULL,
    repo_id TEXT NOT NULL DEFAULT '',
    repo_name TEXT,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    parent TEXT,
    qualified_name TEXT NOT NULL DEFAULT '',
    signature TEXT,
    signature_hash TEXT,
    scope TEXT,
    language TEXT NOT NULL,
    file_hash TEXT,
    implements TEXT,
    UNIQUE(file_path, qualified_name, signature_hash, start_line)
);
`

const filesDDL = `
CREATE TABLE IF NOT EXISTS files (
    file_path TEXT PRIMARY KEY,
    repo_id TEXT NOT NULL DEFAULT '',
    repo_name TEXT,
    content_hash TEXT,
    indexed_at INTEGER,
    language TEXT
);
`

const docsDDL = `
CREATE TABLE IF NOT EXISTS doc_records (
    path TEXT PRIMARY KEY,
    title TEXT,
    doc_id TEXT,
    layer TEXT,
    family TEXT,
    snippet TEXT,
    search_text TEXT,
    content_hash TEXT,
    indexed_at INTEGER,
    is_snapshot INTEGER NOT NULL DEFAULT 0
);
`

const docsFtsDDL = `
CREATE VIRTUAL TABLE IF NOT EXISTS doc_records_fts USING fts5(
    title,
    doc_id,
    search_text,
    content='doc_records',
    content_rowid='rowid',
    tokenize='porter unicode61'
);
`

const docEdgesDDL = `
CREATE TABLE IF NOT EXISTS doc_edges (
    from_path TEXT NOT NULL,
    to_path TEXT NOT NULL DEFAULT '',
    to_doc_id TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL,
    label TEXT,
    UNIQUE(from_path, to_path, to_doc_id, kind, label)
);
`

const docMentionsDDL = `
CREATE TABLE IF NOT EXISTS doc_mentions (
    doc_path TEXT NOT NULL,
    mention_type TEXT NOT NULL,
    mention_value TEXT NOT NULL,
    UNIQUE(doc_path, mention_type, mention_value)
);
`

const docSourceBlocksDDL = `
CREATE TABLE IF NOT EXISTS doc_source_blocks (
    doc_path TEXT NOT NULL,
    block_id TEXT NOT NULL,
    doc_id TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL DEFAULT '',
    source_format TEXT NOT NULL,
    ordinal INTEGER NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    content_hash TEXT,
    indexed_at INTEGER,
    UNIQUE(doc_path, block_id)
);
`

const docSourceRecordsDDL = `
CREATE TABLE IF NOT EXISTS doc_source_records (
    doc_path TEXT NOT NULL,
    block_id TEXT NOT NULL,
    record_id TEXT NOT NULL,
    record_type TEXT NOT NULL DEFAULT '',
    ordinal INTEGER NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    content_hash TEXT,
    indexed_at INTEGER,
    UNIQUE(doc_path, block_id, record_id)
);
`

const metaDDL = `
CREATE TABLE IF NOT EXISTS workspace_meta (
    key TEXT PRIMARY KEY,
    value TEXT
);
`

const indexJobsDDL = `
CREATE TABLE IF NOT EXISTS index_jobs (
    job_id TEXT PRIMARY KEY,
    generation_id TEXT NOT NULL,
    workspace_name TEXT NOT NULL,
    workspace_root TEXT NOT NULL,
    mode TEXT NOT NULL,
    clean INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    phase TEXT NOT NULL DEFAULT '',
    current_stage TEXT NOT NULL DEFAULT '',
    current_path TEXT NOT NULL DEFAULT '',
    files_total INTEGER NOT NULL DEFAULT 0,
    pid INTEGER NOT NULL DEFAULT 0,
    requested_cancel INTEGER NOT NULL DEFAULT 0,
    error TEXT,
    files INTEGER NOT NULL DEFAULT 0,
    symbols INTEGER NOT NULL DEFAULT 0,
    docs INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT,
    updated_at TEXT NOT NULL
);
`

const indexGenerationsDDL = `
CREATE TABLE IF NOT EXISTS index_generations (
    generation_id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL,
    workspace_name TEXT NOT NULL,
    workspace_root TEXT NOT NULL,
    mode TEXT NOT NULL,
    status TEXT NOT NULL,
    files INTEGER NOT NULL DEFAULT 0,
    symbols INTEGER NOT NULL DEFAULT 0,
    docs INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    published_at TEXT,
    error TEXT
);
`

const wikiChunkEmbeddingsDDL = `
CREATE TABLE IF NOT EXISTS wiki_chunk_embeddings (
    doc_path        TEXT NOT NULL,
    chunk_id        TEXT NOT NULL,
    start_line      INTEGER NOT NULL,
    end_line        INTEGER NOT NULL,
    heading_text    TEXT,
    snippet         TEXT,
    content_hash    TEXT NOT NULL,
    embedding       BLOB,
    embedding_model TEXT,
    embedding_dim   INTEGER,
    indexed_at      INTEGER,
    UNIQUE(doc_path, chunk_id)
);
`

const graphGenerationsDDL = `CREATE TABLE IF NOT EXISTS graph_generations (generation_id BLOB NOT NULL CHECK(length(generation_id)=32), schema_version INTEGER NOT NULL CHECK(schema_version=1), workspace_identity TEXT NOT NULL, source_fingerprint BLOB NOT NULL CHECK(length(source_fingerprint)=32), config_fingerprint BLOB NOT NULL CHECK(length(config_fingerprint)=32), backend_manifest_digest BLOB NOT NULL CHECK(length(backend_manifest_digest)=32), content_digest BLOB NOT NULL CHECK(length(content_digest)=32), status TEXT NOT NULL CHECK(status IN ('staged','active','retired','invalid')), node_count INTEGER NOT NULL DEFAULT 0 CHECK(node_count>=0), edge_count INTEGER NOT NULL DEFAULT 0 CHECK(edge_count>=0), evidence_count INTEGER NOT NULL DEFAULT 0 CHECK(evidence_count>=0), unresolved_count INTEGER NOT NULL DEFAULT 0 CHECK(unresolved_count>=0), previous_generation_id BLOB CHECK(previous_generation_id IS NULL OR length(previous_generation_id)=32), created_at TEXT NOT NULL, published_at TEXT, error_code TEXT, PRIMARY KEY(generation_id), FOREIGN KEY(previous_generation_id) REFERENCES graph_generations(generation_id) ON DELETE RESTRICT);`
const graphNodesDDL = `CREATE TABLE IF NOT EXISTS graph_nodes (generation_id BLOB NOT NULL CHECK(length(generation_id)=32), node_id INTEGER NOT NULL CHECK(node_id>=0), node_key BLOB NOT NULL CHECK(length(node_key)=32), identity_schema TEXT NOT NULL, repository_identity TEXT NOT NULL, backend_type TEXT NOT NULL, language TEXT NOT NULL, project_or_module TEXT NOT NULL, owner_path TEXT NOT NULL, symbol_kind TEXT NOT NULL, semantic_identity TEXT NOT NULL, display_name TEXT NOT NULL, source_digest BLOB NOT NULL CHECK(length(source_digest)=32), claim_status TEXT NOT NULL, cross_rid TEXT NOT NULL, sort_key TEXT NOT NULL, PRIMARY KEY(generation_id,node_id), UNIQUE(generation_id,node_key), FOREIGN KEY(generation_id) REFERENCES graph_generations(generation_id) ON DELETE CASCADE);`
const graphEdgesDDL = `CREATE TABLE IF NOT EXISTS graph_edges (generation_id BLOB NOT NULL CHECK(length(generation_id)=32), edge_id INTEGER NOT NULL CHECK(edge_id>=0), edge_key BLOB NOT NULL CHECK(length(edge_key)=32), from_node_id INTEGER NOT NULL CHECK(from_node_id>=0), to_node_id INTEGER NOT NULL CHECK(to_node_id>=0), relation TEXT NOT NULL, claim_scope TEXT NOT NULL, claim_status TEXT NOT NULL, owner_path TEXT NOT NULL, source_backend TEXT NOT NULL, cross_rid TEXT NOT NULL, PRIMARY KEY(generation_id,edge_id), UNIQUE(generation_id,edge_key), FOREIGN KEY(generation_id) REFERENCES graph_generations(generation_id) ON DELETE CASCADE, FOREIGN KEY(generation_id,from_node_id) REFERENCES graph_nodes(generation_id,node_id) ON DELETE CASCADE, FOREIGN KEY(generation_id,to_node_id) REFERENCES graph_nodes(generation_id,node_id) ON DELETE CASCADE);`
const graphEvidenceDDL = `CREATE TABLE IF NOT EXISTS graph_evidence (generation_id BLOB NOT NULL CHECK(length(generation_id)=32), evidence_id INTEGER NOT NULL CHECK(evidence_id>=0), evidence_key BLOB NOT NULL CHECK(length(evidence_key)=32), subject_kind TEXT NOT NULL CHECK(subject_kind IN ('node','edge')), node_id INTEGER, edge_id INTEGER, source_uri TEXT NOT NULL, start_line INTEGER CHECK(start_line IS NULL OR start_line>=0), start_column INTEGER CHECK(start_column IS NULL OR start_column>=0), end_line INTEGER CHECK(end_line IS NULL OR end_line>=0), end_column INTEGER CHECK(end_column IS NULL OR end_column>=0), backend TEXT NOT NULL, extractor_version TEXT NOT NULL, source_digest BLOB NOT NULL CHECK(length(source_digest)=32), claim_kind TEXT NOT NULL, observed_claim_digest BLOB NOT NULL CHECK(length(observed_claim_digest)=32), claim_status TEXT NOT NULL, cross_rid TEXT NOT NULL, CHECK((subject_kind='node' AND node_id IS NOT NULL AND edge_id IS NULL) OR (subject_kind='edge' AND node_id IS NULL AND edge_id IS NOT NULL)), PRIMARY KEY(generation_id,evidence_id), UNIQUE(generation_id,evidence_key), FOREIGN KEY(generation_id) REFERENCES graph_generations(generation_id) ON DELETE CASCADE, FOREIGN KEY(generation_id,node_id) REFERENCES graph_nodes(generation_id,node_id) ON DELETE CASCADE, FOREIGN KEY(generation_id,edge_id) REFERENCES graph_edges(generation_id,edge_id) ON DELETE CASCADE);`
const graphUnresolvedDDL = `CREATE TABLE IF NOT EXISTS graph_unresolved (generation_id BLOB NOT NULL CHECK(length(generation_id)=32), unresolved_id INTEGER NOT NULL CHECK(unresolved_id>=0), unresolved_key BLOB NOT NULL CHECK(length(unresolved_key)=32), owner_path TEXT NOT NULL, subject_kind TEXT NOT NULL, selector_digest BLOB NOT NULL CHECK(length(selector_digest)=32), reason_code TEXT NOT NULL, candidates_json TEXT NOT NULL, backend TEXT NOT NULL, source_digest BLOB CHECK(source_digest IS NULL OR length(source_digest)=32), cross_rid TEXT NOT NULL, recovery_hint_code TEXT, PRIMARY KEY(generation_id,unresolved_id), UNIQUE(generation_id,unresolved_key), FOREIGN KEY(generation_id) REFERENCES graph_generations(generation_id) ON DELETE CASCADE);`
const graphMigrationsDDL = `CREATE TABLE IF NOT EXISTS graph_migrations (migration_id TEXT PRIMARY KEY, from_version INTEGER NOT NULL, to_version INTEGER NOT NULL, status TEXT NOT NULL CHECK(status IN ('prepared','applying','validated','committed','rolled_back','failed')), preflight_digest BLOB NOT NULL CHECK(length(preflight_digest)=32), backup_digest BLOB NOT NULL CHECK(length(backup_digest)=32), prior_active_generation_id BLOB CHECK(prior_active_generation_id IS NULL OR length(prior_active_generation_id)=32), started_at TEXT NOT NULL, completed_at TEXT, error_code TEXT, FOREIGN KEY(prior_active_generation_id) REFERENCES graph_generations(generation_id) ON DELETE RESTRICT);`
const graphAnalysisDDL = `CREATE TABLE IF NOT EXISTS graph_analysis (analysis_key BLOB NOT NULL CHECK(length(analysis_key)=32) PRIMARY KEY, generation_id BLOB NOT NULL CHECK(length(generation_id)=32), extension_id TEXT NOT NULL, extension_version TEXT NOT NULL, executable_digest BLOB NOT NULL CHECK(length(executable_digest)=32), operation TEXT NOT NULL, parameters_digest BLOB NOT NULL CHECK(length(parameters_digest)=32), authority_profile_digest BLOB NOT NULL CHECK(length(authority_profile_digest)=32), output_schema TEXT NOT NULL, result_json_bounded TEXT NOT NULL, result_digest BLOB NOT NULL CHECK(length(result_digest)=32), provenance_json_sanitized TEXT NOT NULL, omissions_json_sanitized TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL, FOREIGN KEY(generation_id) REFERENCES graph_generations(generation_id) ON DELETE CASCADE);`

func EnsureSchema(db *sql.DB) error {
	if err := graphSchemaPreflight(db); err != nil {
		return err
	}
	statements := []string{reposDDL, entrypointsDDL, symbolsDDL, filesDDL, docsDDL, docsFtsDDL, docEdgesDDL, docMentionsDDL, docSourceBlocksDDL, docSourceRecordsDDL, metaDDL, indexJobsDDL, indexGenerationsDDL, wikiChunkEmbeddingsDDL}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	for _, triggerDDL := range []string{
		`CREATE TRIGGER IF NOT EXISTS doc_records_ai AFTER INSERT ON doc_records BEGIN
    INSERT INTO doc_records_fts(rowid, title, doc_id, search_text)
    VALUES (new.rowid, new.title, new.doc_id, new.search_text);
END`,
		`CREATE TRIGGER IF NOT EXISTS doc_records_ad AFTER DELETE ON doc_records BEGIN
    INSERT INTO doc_records_fts(doc_records_fts, rowid, title, doc_id, search_text)
    VALUES ('delete', old.rowid, old.title, old.doc_id, old.search_text);
END`,
		`CREATE TRIGGER IF NOT EXISTS doc_records_au AFTER UPDATE ON doc_records BEGIN
    INSERT INTO doc_records_fts(doc_records_fts, rowid, title, doc_id, search_text)
    VALUES ('delete', old.rowid, old.title, old.doc_id, old.search_text);
    INSERT INTO doc_records_fts(rowid, title, doc_id, search_text)
    VALUES (new.rowid, new.title, new.doc_id, new.search_text);
END`,
	} {
		if _, err := db.Exec(triggerDDL); err != nil {
			return fmt.Errorf("fts trigger: %w", err)
		}
	}

	if err := ensureColumn(db, "symbols", "repo_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "symbols", "repo_name", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "files", "repo_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "files", "repo_name", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "symbols", "search_text", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "doc_records", "is_snapshot", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumn(db, "index_jobs", "clean", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumn(db, "index_jobs", "current_stage", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "index_jobs", "current_path", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "index_jobs", "files_total", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	indexes := []struct {
		table     string
		column    string
		statement string
		required  bool
	}{
		{table: "workspace_entrypoints", column: "repo_id", statement: `CREATE INDEX IF NOT EXISTS idx_workspace_entrypoints_repo ON workspace_entrypoints(repo_id);`, required: true},
		{table: "symbols", column: "name", statement: `CREATE INDEX IF NOT EXISTS idx_symbols_name ON symbols(name);`, required: true},
		{table: "symbols", column: "file_path", statement: `CREATE INDEX IF NOT EXISTS idx_symbols_file ON symbols(file_path);`, required: true},
		{table: "symbols", column: "kind", statement: `CREATE INDEX IF NOT EXISTS idx_symbols_kind ON symbols(kind);`, required: true},
		{table: "symbols", column: "qualified_name", statement: `CREATE INDEX IF NOT EXISTS idx_symbols_qualified_name ON symbols(qualified_name);`, required: true},
		{table: "symbols", column: "repo_id", statement: `CREATE INDEX IF NOT EXISTS idx_symbols_repo_id ON symbols(repo_id);`, required: false},
		{table: "files", column: "repo_id", statement: `CREATE INDEX IF NOT EXISTS idx_files_repo_id ON files(repo_id);`, required: false},
		{table: "symbols", column: "file_path", statement: `CREATE INDEX IF NOT EXISTS idx_symbols_file_lines ON symbols(file_path, start_line, end_line);`, required: false},
		{table: "doc_records", column: "family", statement: `CREATE INDEX IF NOT EXISTS idx_doc_records_family ON doc_records(family, layer);`, required: true},
		{table: "doc_records", column: "doc_id", statement: `CREATE INDEX IF NOT EXISTS idx_doc_records_doc_id ON doc_records(doc_id);`, required: true},
		{table: "doc_mentions", column: "mention_type", statement: `CREATE INDEX IF NOT EXISTS idx_doc_mentions_type ON doc_mentions(mention_type, mention_value);`, required: true},
		{table: "doc_edges", column: "from_path", statement: `CREATE INDEX IF NOT EXISTS idx_doc_edges_from ON doc_edges(from_path);`, required: true},
		{table: "doc_source_blocks", column: "block_id", statement: `CREATE INDEX IF NOT EXISTS idx_doc_source_blocks_block_id ON doc_source_blocks(block_id);`, required: true},
		{table: "doc_source_blocks", column: "doc_id", statement: `CREATE INDEX IF NOT EXISTS idx_doc_source_blocks_doc_id ON doc_source_blocks(doc_id);`, required: true},
		{table: "doc_source_records", column: "record_id", statement: `CREATE INDEX IF NOT EXISTS idx_doc_source_records_record_id ON doc_source_records(record_id);`, required: true},
		{table: "index_jobs", column: "workspace_root", statement: `CREATE INDEX IF NOT EXISTS idx_index_jobs_workspace_status ON index_jobs(workspace_root, status, updated_at);`, required: true},
		{table: "index_generations", column: "workspace_root", statement: `CREATE INDEX IF NOT EXISTS idx_index_generations_workspace ON index_generations(workspace_root, status, published_at);`, required: true},
		{table: "wiki_chunk_embeddings", column: "doc_path", statement: `CREATE INDEX IF NOT EXISTS idx_wiki_chunk_embeddings_doc ON wiki_chunk_embeddings(doc_path);`, required: false},
	}

	for _, index := range indexes {
		hasColumn, err := tableHasColumn(db, index.table, index.column)
		if err != nil {
			return err
		}
		if !hasColumn {
			if index.required {
				return fmt.Errorf("schema missing required column %s.%s", index.table, index.column)
			}
			continue
		}
		if _, err := db.Exec(index.statement); err != nil {
			return err
		}
	}
	return ensureGraphSchema(db)
}

type graphSchemaCompatibilityError struct{ reason string }

func (e *graphSchemaCompatibilityError) Error() string {
	return "graph schema incompatible: " + e.reason
}

var graphTables = map[string]string{
	"graph_generations": graphGenerationsDDL, "graph_nodes": graphNodesDDL,
	"graph_edges": graphEdgesDDL, "graph_evidence": graphEvidenceDDL,
	"graph_unresolved": graphUnresolvedDDL, "graph_migrations": graphMigrationsDDL,
	"graph_analysis": graphAnalysisDDL,
}
var graphIndexes = map[string]string{
	"idx_graph_generations_one_active":                  `CREATE UNIQUE INDEX IF NOT EXISTS idx_graph_generations_one_active ON graph_generations(workspace_identity) WHERE status = 'active';`,
	"idx_graph_generations_workspace_status":            `CREATE INDEX IF NOT EXISTS idx_graph_generations_workspace_status ON graph_generations(workspace_identity, status, created_at);`,
	"idx_graph_nodes_generation_owner_path":             `CREATE INDEX IF NOT EXISTS idx_graph_nodes_generation_owner_path ON graph_nodes(generation_id, owner_path);`,
	"idx_graph_nodes_generation_kind_sort_key":          `CREATE INDEX IF NOT EXISTS idx_graph_nodes_generation_kind_sort_key ON graph_nodes(generation_id, symbol_kind, sort_key);`,
	"idx_graph_nodes_generation_cross_rid":              `CREATE INDEX IF NOT EXISTS idx_graph_nodes_generation_cross_rid ON graph_nodes(generation_id, cross_rid);`,
	"idx_graph_edges_generation_from_relation_to":       `CREATE INDEX IF NOT EXISTS idx_graph_edges_generation_from_relation_to ON graph_edges(generation_id, from_node_id, relation, to_node_id);`,
	"idx_graph_edges_generation_to_relation_from":       `CREATE INDEX IF NOT EXISTS idx_graph_edges_generation_to_relation_from ON graph_edges(generation_id, to_node_id, relation, from_node_id);`,
	"idx_graph_edges_generation_owner_path":             `CREATE INDEX IF NOT EXISTS idx_graph_edges_generation_owner_path ON graph_edges(generation_id, owner_path);`,
	"idx_graph_evidence_generation_edge":                `CREATE INDEX IF NOT EXISTS idx_graph_evidence_generation_edge ON graph_evidence(generation_id, edge_id);`,
	"idx_graph_evidence_generation_node":                `CREATE INDEX IF NOT EXISTS idx_graph_evidence_generation_node ON graph_evidence(generation_id, node_id);`,
	"idx_graph_unresolved_generation_owner_reason":      `CREATE INDEX IF NOT EXISTS idx_graph_unresolved_generation_owner_reason ON graph_unresolved(generation_id, owner_path, reason_code);`,
	"idx_graph_analysis_generation_extension_operation": `CREATE INDEX IF NOT EXISTS idx_graph_analysis_generation_extension_operation ON graph_analysis(generation_id, extension_id, operation);`,
	"idx_graph_migrations_status":                       `CREATE INDEX IF NOT EXISTS idx_graph_migrations_status ON graph_migrations(status, started_at);`,
}

var graphAdditiveIndexes = map[string]string{
	"idx_graph_nodes_generation_semantic_identity": `CREATE INDEX IF NOT EXISTS idx_graph_nodes_generation_semantic_identity ON graph_nodes(generation_id, semantic_identity);`,
	"idx_graph_nodes_generation_display_name":      `CREATE INDEX IF NOT EXISTS idx_graph_nodes_generation_display_name ON graph_nodes(generation_id, display_name);`,
	"idx_graph_analysis_generation_key":            `CREATE INDEX IF NOT EXISTS idx_graph_analysis_generation_key ON graph_analysis(generation_id, analysis_key);`,
	"idx_graph_analysis_generation_profile":        `CREATE INDEX IF NOT EXISTS idx_graph_analysis_generation_profile ON graph_analysis(generation_id, algorithm, algorithm_version, profile);`,
}

func normalizeSchemaSQL(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " if not exists", "")
	s = strings.TrimSuffix(s, ";")
	return strings.Join(strings.Fields(s), " ")
}

func graphTableDefinitionCompatible(name, actual, expected string) bool {
	actual = normalizeSchemaSQL(actual)
	expected = normalizeSchemaSQL(expected)
	if actual == expected {
		return true
	}
	if name != "graph_analysis" {
		return false
	}
	// graph_analysis received additive ranking metadata columns. SQLite keeps
	// ALTER TABLE additions in sqlite_master, so compare the original table
	// contract after removing only those known additive columns.
	for _, column := range []string{
		", algorithm text not null default 'bounded-deterministic-v1'",
		", algorithm_version text not null default '1'",
		", profile text not null default 'exact-extracted-only'",
		", determinism_digest text not null default ''",
	} {
		actual = strings.ReplaceAll(actual, column, "")
	}
	return actual == expected
}

func graphSchemaPreflight(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("graph schema database is required")
	}
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'graph_%'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	present := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return err
		}
		present[n] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(present) == 0 {
		return nil
	}
	for name, ddl := range graphTables {
		if !present[name] {
			return &graphSchemaCompatibilityError{"partial schema missing table " + name}
		}
		var actual string
		if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&actual); err != nil || !graphTableDefinitionCompatible(name, actual, ddl) {
			return &graphSchemaCompatibilityError{"incompatible table definition " + name}
		}
	}
	for name, ddl := range graphIndexes {
		var actual sql.NullString
		if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='index' AND name=?`, name).Scan(&actual); err != nil {
			return &graphSchemaCompatibilityError{"missing required index " + name}
		}
		old := `CREATE UNIQUE INDEX IF NOT EXISTS idx_graph_generations_one_active ON graph_generations(status) WHERE status = 'active';`
		actualSQL := normalizeSchemaSQL(actual.String)
		if name == "idx_graph_generations_one_active" && actualSQL == normalizeSchemaSQL(old) {
			continue
		}
		if actualSQL != normalizeSchemaSQL(ddl) {
			return &graphSchemaCompatibilityError{"incompatible index " + name}
		}
	}
	return nil
}
func ensureGraphSchema(db *sql.DB) error {
	if err := graphSchemaPreflight(db); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin graph schema migration: %w", err)
	}
	if err := ensureGraphSchemaTx(context.Background(), tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit graph schema migration: %w", err)
	}
	return nil
}

// ensureGraphSchemaTx creates the graph schema inside the caller's transaction.
// It is intentionally separate from ensureGraphSchema so a 0->1 bootstrap can
// be atomic with its migration intent.
func ensureGraphSchemaTx(ctx context.Context, tx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}) error {
	if _, err := tx.ExecContext(ctx, metaDDL); err != nil {
		return fmt.Errorf("create workspace_meta: %w", err)
	}
	for name, ddl := range graphTables {
		if _, err := tx.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("create %s: %w", name, err)
		}
	}
	// These columns are additive and intentionally migrated inside the same
	// transaction as graph bootstrap. This handles both a fresh database and
	// an existing v1 graph_analysis table without changing its sqlite_master
	// definition used by the compatibility preflight.
	for _, column := range []struct{ name, definition string }{
		{"algorithm", "TEXT NOT NULL DEFAULT 'bounded-deterministic-v1'"},
		{"algorithm_version", "TEXT NOT NULL DEFAULT '1'"},
		{"profile", "TEXT NOT NULL DEFAULT 'exact-extracted-only'"},
		{"determinism_digest", "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := ensureColumnTx(ctx, tx, "graph_analysis", column.name, column.definition); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS idx_graph_generations_one_active`); err != nil {
		return err
	}
	for name, ddl := range graphIndexes {
		if _, err := tx.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("create %s: %w", name, err)
		}
	}
	for name, ddl := range graphAdditiveIndexes {
		if _, err := tx.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("create %s: %w", name, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO workspace_meta(key,value) VALUES ('graph_schema_version','1'), ('graph_compatibility_state','legacy-preserved-no-dual-write'), ('active_graph_generation_id',NULL), ('previous_graph_generation_id',NULL)`); err != nil {
		return err
	}
	return nil
}

func ensureColumnTx(ctx context.Context, tx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, table, column, definition string) error {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var name, typeName string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &typeName, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition)); err != nil {
		return err
	}
	return nil
}

func ensureColumn(db *sql.DB, table string, column string, definition string) error {
	hasColumn, err := tableHasColumn(db, table, column)
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}
	_, err = db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	return err
}

func tableHasColumn(db *sql.DB, table string, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid          int
			name         string
			typeName     string
			notNull      int
			defaultValue sql.NullString
			pk           int
		)
		if err := rows.Scan(&cid, &name, &typeName, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}
