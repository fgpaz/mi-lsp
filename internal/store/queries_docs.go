package store

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func ReplaceDocs(ctx context.Context, db *sql.DB, docs []model.DocRecord, edges []model.DocEdge, mentions []model.DocMention) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := replaceDocsTx(ctx, tx, docs, edges, mentions); err != nil {
		return err
	}
	return tx.Commit()
}

func replaceDocsTx(ctx context.Context, tx *sql.Tx, docs []model.DocRecord, edges []model.DocEdge, mentions []model.DocMention) error {
	for _, table := range []string{"doc_mentions", "doc_edges", "doc_records"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return err
		}
	}

	if len(docs) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO doc_records(path, title, doc_id, layer, family, snippet, search_text, content_hash, indexed_at, is_snapshot)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, doc := range docs {
			if _, err := stmt.ExecContext(ctx, doc.Path, doc.Title, doc.DocID, doc.Layer, doc.Family, doc.Snippet, doc.SearchText, doc.ContentHash, doc.IndexedAt, doc.IsSnapshot); err != nil {
				return err
			}
		}
	}

	if len(edges) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT OR REPLACE INTO doc_edges(from_path, to_path, to_doc_id, kind, label)
			VALUES(?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, edge := range edges {
			if _, err := stmt.ExecContext(ctx, edge.FromPath, edge.ToPath, edge.ToDocID, edge.Kind, edge.Label); err != nil {
				return err
			}
		}
	}

	if len(mentions) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT OR REPLACE INTO doc_mentions(doc_path, mention_type, mention_value)
			VALUES(?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, mention := range mentions {
			if _, err := stmt.ExecContext(ctx, mention.DocPath, mention.MentionType, mention.MentionValue); err != nil {
				return err
			}
		}
	}

	if err := UpsertWorkspaceMeta(ctx, tx, "doc_count", strconv.Itoa(len(docs))); err != nil {
		return err
	}
	return nil
}

func ListDocRecords(ctx context.Context, db *sql.DB) ([]model.DocRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT path, title, doc_id, layer, family, snippet, search_text, content_hash, indexed_at, is_snapshot
		FROM doc_records
		ORDER BY family ASC, layer ASC, path ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.DocRecord, 0)
	for rows.Next() {
		var item model.DocRecord
		if err := rows.Scan(&item.Path, &item.Title, &item.DocID, &item.Layer, &item.Family, &item.Snippet, &item.SearchText, &item.ContentHash, &item.IndexedAt, &item.IsSnapshot); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func CountDocRecords(ctx context.Context, db *sql.DB) (int, error) {
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM doc_records").Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func FindDocRecordsByMention(ctx context.Context, db *sql.DB, mentionType string, mentionValue string) ([]model.DocRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT dr.path, dr.title, dr.doc_id, dr.layer, dr.family, dr.snippet, dr.search_text, dr.content_hash, dr.indexed_at, dr.is_snapshot
		FROM doc_records dr
		JOIN doc_mentions dm ON dm.doc_path = dr.path
		WHERE dm.mention_type = ? AND UPPER(dm.mention_value) = UPPER(?)
		ORDER BY dr.layer ASC, dr.path ASC
	`, mentionType, mentionValue)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.DocRecord, 0)
	for rows.Next() {
		var item model.DocRecord
		if err := rows.Scan(&item.Path, &item.Title, &item.DocID, &item.Layer, &item.Family, &item.Snippet, &item.SearchText, &item.ContentHash, &item.IndexedAt, &item.IsSnapshot); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func DocEdgesFrom(ctx context.Context, db *sql.DB, docPath string) ([]model.DocEdge, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT from_path, to_path, to_doc_id, kind, label
		FROM doc_edges
		WHERE from_path = ?
		ORDER BY kind ASC, to_path ASC, to_doc_id ASC
	`, docPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.DocEdge, 0)
	for rows.Next() {
		var item model.DocEdge
		if err := rows.Scan(&item.FromPath, &item.ToPath, &item.ToDocID, &item.Kind, &item.Label); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func DocMentionsForPath(ctx context.Context, db *sql.DB, docPath string) ([]model.DocMention, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT doc_path, mention_type, mention_value
		FROM doc_mentions
		WHERE doc_path = ?
		ORDER BY mention_type ASC, mention_value ASC
	`, docPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.DocMention, 0)
	for rows.Next() {
		var item model.DocMention
		if err := rows.Scan(&item.DocPath, &item.MentionType, &item.MentionValue); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func GetRFDocRecords(ctx context.Context, db *sql.DB) ([]model.DocRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT path, title, doc_id, layer, family, snippet, search_text, content_hash, indexed_at
		FROM doc_records
		WHERE layer = '04' OR doc_id LIKE 'RF-%'
		ORDER BY doc_id ASC, path ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.DocRecord, 0)
	for rows.Next() {
		var item model.DocRecord
		if err := rows.Scan(&item.Path, &item.Title, &item.DocID, &item.Layer, &item.Family, &item.Snippet, &item.SearchText, &item.ContentHash, &item.IndexedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func GetMentionsByType(ctx context.Context, db *sql.DB, docPath string, mentionType string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT mention_value
		FROM doc_mentions
		WHERE doc_path = ? AND mention_type = ?
		ORDER BY mention_value ASC
	`, docPath, mentionType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]string, 0)
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		items = append(items, value)
	}
	return items, rows.Err()
}

// FTSSearchDocs uses the FTS5 virtual table to find doc_records matching the question.
// Returns nil, nil, nil if the FTS5 table is unavailable (graceful degradation for old databases).
func FTSSearchDocs(ctx context.Context, db *sql.DB, question string, limit int) ([]model.DocRecord, map[string]float64, error) {
	if limit <= 0 {
		limit = 20
	}

	// Build FTS5-safe query: split into words >= 2 chars, join with OR
	words := strings.Fields(strings.ToLower(question))
	terms := make([]string, 0, len(words))
	for _, w := range words {
		// Strip non-alphanumeric characters that break FTS5 syntax
		clean := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r > 127 {
				return r
			}
			return -1
		}, w)
		if len(clean) >= 2 {
			terms = append(terms, clean)
		}
	}
	if len(terms) == 0 {
		return nil, nil, nil
	}
	matchQuery := strings.Join(terms, " OR ")

	rows, err := db.QueryContext(ctx, `
		SELECT dr.path, dr.title, dr.doc_id, dr.layer, dr.family, dr.snippet, dr.search_text,
		       -rank * 10 as fts_score
		FROM doc_records_fts
		JOIN doc_records dr ON dr.rowid = doc_records_fts.rowid
		WHERE doc_records_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, matchQuery, limit)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "no such table") || strings.Contains(errStr, "no such module") {
			return nil, nil, nil
		}
		// FTS5 syntax error or other query error - fall back gracefully
		return nil, nil, nil
	}
	defer rows.Close()

	docs := make([]model.DocRecord, 0)
	scores := make(map[string]float64)
	for rows.Next() {
		var item model.DocRecord
		var ftsScore float64
		if err := rows.Scan(&item.Path, &item.Title, &item.DocID, &item.Layer, &item.Family, &item.Snippet, &item.SearchText, &ftsScore); err != nil {
			return nil, nil, err
		}
		docs = append(docs, item)
		scores[item.Path] = ftsScore
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return docs, scores, nil
}

func VerifySymbolExists(ctx context.Context, db *sql.DB, filePath string, symbolName string) (model.SymbolRecord, bool, error) {
	row := db.QueryRowContext(ctx, `
		SELECT `+symbolColumns+`
		FROM symbols
		WHERE file_path = ? AND name = ?
		LIMIT 1
	`, filePath, symbolName)

	var item model.SymbolRecord
	err := row.Scan(
		&item.ID, &item.FilePath, &item.RepoID, &item.RepoName,
		&item.Name, &item.Kind, &item.StartLine, &item.EndLine,
		&item.Parent, &item.QualifiedName, &item.Signature,
		&item.SignatureHash, &item.Scope, &item.Language,
		&item.FileHash, &item.Implements, &item.SearchText,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.SymbolRecord{}, false, nil
		}
		return model.SymbolRecord{}, false, err
	}
	return item, true, nil
}
