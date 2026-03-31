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
    indexed_at INTEGER
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

const metaDDL = `
CREATE TABLE IF NOT EXISTS workspace_meta (
    key TEXT PRIMARY KEY,
    value TEXT
);
`

func EnsureSchema(db *sql.DB) error {
	statements := []string{reposDDL, entrypointsDDL, symbolsDDL, filesDDL, docsDDL, docEdgesDDL, docMentionsDDL, metaDDL}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
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
