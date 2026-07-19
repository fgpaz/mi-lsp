package store

import (
	"database/sql"
	"fmt"
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

const graphGenerationsDDL = `
CREATE TABLE IF NOT EXISTS graph_generations (
    generation_id TEXT PRIMARY KEY, workspace_root TEXT NOT NULL, schema_version INTEGER NOT NULL,
    source_fingerprint TEXT, backend_version TEXT, compiler_version TEXT, created_at TEXT NOT NULL,
    sealed_at TEXT, activated_at TEXT, retired_at TEXT, status TEXT NOT NULL CHECK(status IN ('staged', 'active', 'retired', 'invalid')),
    owner_id TEXT, owner_pid INTEGER NOT NULL DEFAULT 0, prior_generation_id TEXT, migration_id TEXT,
    expected_nodes INTEGER NOT NULL DEFAULT 0, expected_edges INTEGER NOT NULL DEFAULT 0,
    expected_evidence INTEGER NOT NULL DEFAULT 0, expected_unresolved INTEGER NOT NULL DEFAULT 0,
    digest TEXT, error TEXT
);
`
const graphNodesDDL = `
CREATE TABLE IF NOT EXISTS graph_nodes (
    generation_id TEXT NOT NULL, node_key BLOB NOT NULL, canonical_tuple BLOB NOT NULL,
    kind TEXT NOT NULL, display_name TEXT NOT NULL, declaration_path TEXT NOT NULL,
    declaration_start INTEGER NOT NULL DEFAULT 0, declaration_end INTEGER NOT NULL DEFAULT 0,
    source_fingerprint TEXT, backend TEXT NOT NULL, compiler_version TEXT, confidence TEXT,
    status TEXT NOT NULL, cross_rid TEXT NOT NULL, provenance TEXT,
    PRIMARY KEY(generation_id, node_key), FOREIGN KEY(generation_id) REFERENCES graph_generations(generation_id) ON DELETE CASCADE
);
`
const graphEvidenceDDL = `
CREATE TABLE IF NOT EXISTS graph_evidence (
    generation_id TEXT NOT NULL, evidence_id TEXT NOT NULL, source_uri TEXT NOT NULL,
    source_range TEXT, backend TEXT NOT NULL, extractor_version TEXT NOT NULL, digest TEXT NOT NULL,
    observed_claim TEXT NOT NULL, cross_rid TEXT NOT NULL, status TEXT,
    PRIMARY KEY(generation_id, evidence_id), FOREIGN KEY(generation_id) REFERENCES graph_generations(generation_id) ON DELETE CASCADE
);
`
const graphEdgesDDL = `
CREATE TABLE IF NOT EXISTS graph_edges (
    generation_id TEXT NOT NULL, from_node_key BLOB NOT NULL, to_node_key BLOB NOT NULL,
    relation TEXT NOT NULL, claim_scope TEXT NOT NULL, evidence_id TEXT, source_path TEXT, source_start INTEGER NOT NULL DEFAULT 0,
    source_end INTEGER NOT NULL DEFAULT 0, provenance TEXT NOT NULL, confidence TEXT,
    status TEXT NOT NULL, cross_rid TEXT NOT NULL,
    PRIMARY KEY(generation_id, from_node_key, to_node_key, relation, claim_scope),
    FOREIGN KEY(generation_id, from_node_key) REFERENCES graph_nodes(generation_id, node_key) ON DELETE CASCADE,
    FOREIGN KEY(generation_id, to_node_key) REFERENCES graph_nodes(generation_id, node_key) ON DELETE CASCADE,
    FOREIGN KEY(generation_id, evidence_id) REFERENCES graph_evidence(generation_id, evidence_id) ON DELETE RESTRICT
);
`
const graphUnresolvedDDL = `
CREATE TABLE IF NOT EXISTS graph_unresolved (
    generation_id TEXT NOT NULL, unresolved_id TEXT NOT NULL, reason_code TEXT NOT NULL,
    selector TEXT, cross_rid TEXT NOT NULL, recovery_hint TEXT,
    PRIMARY KEY(generation_id, unresolved_id), FOREIGN KEY(generation_id) REFERENCES graph_generations(generation_id) ON DELETE CASCADE
);
`
const graphOwnerStateDDL = `
CREATE TABLE IF NOT EXISTS graph_owner_state (
    workspace_root TEXT PRIMARY KEY, owner_id TEXT NOT NULL, owner_pid INTEGER NOT NULL DEFAULT 0,
    heartbeat_at TEXT NOT NULL, state TEXT NOT NULL, lease_token TEXT NOT NULL
);
`
const graphMigrationsDDL = `
CREATE TABLE IF NOT EXISTS graph_migrations (
    migration_id TEXT PRIMARY KEY, workspace_root TEXT NOT NULL, from_schema INTEGER NOT NULL,
    to_schema INTEGER NOT NULL, status TEXT NOT NULL, checksum TEXT NOT NULL, started_at TEXT NOT NULL,
    completed_at TEXT, rollback_generation_id TEXT, error TEXT
);
`

func EnsureSchema(db *sql.DB) error {
	statements := []string{reposDDL, entrypointsDDL, symbolsDDL, filesDDL, docsDDL, docsFtsDDL, docEdgesDDL, docMentionsDDL, docSourceBlocksDDL, docSourceRecordsDDL, metaDDL, indexJobsDDL, indexGenerationsDDL, wikiChunkEmbeddingsDDL, graphGenerationsDDL, graphNodesDDL, graphEvidenceDDL, graphEdgesDDL, graphUnresolvedDDL, graphOwnerStateDDL, graphMigrationsDDL}
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
		{table: "graph_generations", column: "workspace_root", statement: `CREATE INDEX IF NOT EXISTS idx_graph_generations_workspace_status ON graph_generations(workspace_root, status, created_at);`, required: true},
		{table: "graph_generations", column: "status", statement: `CREATE INDEX IF NOT EXISTS idx_graph_generations_status ON graph_generations(status, created_at);`, required: true},
		{table: "graph_nodes", column: "generation_id", statement: `CREATE INDEX IF NOT EXISTS idx_graph_nodes_generation ON graph_nodes(generation_id);`, required: true},
		{table: "graph_nodes", column: "cross_rid", statement: `CREATE INDEX IF NOT EXISTS idx_graph_nodes_cross_rid ON graph_nodes(cross_rid);`, required: true},
		{table: "graph_edges", column: "generation_id", statement: `CREATE INDEX IF NOT EXISTS idx_graph_edges_generation_from ON graph_edges(generation_id, from_node_key);`, required: true},
		{table: "graph_edges", column: "to_node_key", statement: `CREATE INDEX IF NOT EXISTS idx_graph_edges_generation_to ON graph_edges(generation_id, to_node_key);`, required: true},
		{table: "graph_edges", column: "relation", statement: `CREATE INDEX IF NOT EXISTS idx_graph_edges_relation ON graph_edges(generation_id, relation, claim_scope);`, required: true},
		{table: "graph_evidence", column: "cross_rid", statement: `CREATE INDEX IF NOT EXISTS idx_graph_evidence_cross_rid ON graph_evidence(cross_rid);`, required: true},
		{table: "graph_unresolved", column: "generation_id", statement: `CREATE INDEX IF NOT EXISTS idx_graph_unresolved_generation ON graph_unresolved(generation_id);`, required: true},
		{table: "graph_migrations", column: "workspace_root", statement: `CREATE INDEX IF NOT EXISTS idx_graph_migrations_workspace ON graph_migrations(workspace_root, status);`, required: true},
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
