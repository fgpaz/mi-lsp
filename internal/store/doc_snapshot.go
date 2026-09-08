package store

import (
	"context"
	"database/sql"

	"github.com/fgpaz/mi-lsp/internal/docidentity"
	"github.com/fgpaz/mi-lsp/internal/model"
)

// ReadCurrentDocSnapshot reads the trust marker and all document fact families
// in one read transaction. A missing or old marker returns current=false;
// database errors are propagated so they cannot be mistaken for legacy data.
func ReadCurrentDocSnapshot(ctx context.Context, db *sql.DB) (docs []model.DocRecord, edges []model.DocEdge, mentions []model.DocMention, blocks []model.DocSourceBlock, records []model.DocSourceRecord, bindings []model.DocArtifactBinding, current bool, err error) {
	if db == nil {
		return nil, nil, nil, nil, nil, nil, false, sql.ErrConnDone
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, nil, nil, nil, nil, nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	value, ok, err := workspaceMetaValueConn(ctx, tx, WorkspaceMetaDocIdentitySnapshotVersion)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, false, err
	}
	if !ok || value != docidentity.ExtractionVersion {
		return nil, nil, nil, nil, nil, nil, false, nil
	}
	if docs, err = readDocRecordsTx(ctx, tx); err != nil {
		return nil, nil, nil, nil, nil, nil, false, err
	}
	if edges, err = readDocEdgesTx(ctx, tx); err != nil {
		return nil, nil, nil, nil, nil, nil, false, err
	}
	if mentions, err = readDocMentionsTx(ctx, tx); err != nil {
		return nil, nil, nil, nil, nil, nil, false, err
	}
	if blocks, err = readDocSourceBlocksTx(ctx, tx); err != nil {
		return nil, nil, nil, nil, nil, nil, false, err
	}
	if records, err = readDocSourceRecordsTx(ctx, tx); err != nil {
		return nil, nil, nil, nil, nil, nil, false, err
	}
	if bindings, err = readDocArtifactBindingsTx(ctx, tx); err != nil {
		return nil, nil, nil, nil, nil, nil, false, err
	}
	return docs, edges, mentions, blocks, records, bindings, true, nil
}

func readDocRecordsTx(ctx context.Context, tx *sql.Tx) ([]model.DocRecord, error) {
	rows, err := tx.QueryContext(ctx, `SELECT path, title, doc_id, layer, family, snippet, search_text, content_hash, indexed_at, is_snapshot FROM doc_records ORDER BY family ASC, layer ASC, path ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.DocRecord
	for rows.Next() {
		var item model.DocRecord
		if err := rows.Scan(&item.Path, &item.Title, &item.DocID, &item.Layer, &item.Family, &item.Snippet, &item.SearchText, &item.ContentHash, &item.IndexedAt, &item.IsSnapshot); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func readDocEdgesTx(ctx context.Context, tx *sql.Tx) ([]model.DocEdge, error) {
	rows, err := tx.QueryContext(ctx, `SELECT from_path, to_path, to_doc_id, kind, label FROM doc_edges ORDER BY from_path ASC, kind ASC, to_path ASC, to_doc_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.DocEdge
	for rows.Next() {
		var item model.DocEdge
		if err := rows.Scan(&item.FromPath, &item.ToPath, &item.ToDocID, &item.Kind, &item.Label); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func readDocMentionsTx(ctx context.Context, tx *sql.Tx) ([]model.DocMention, error) {
	rows, err := tx.QueryContext(ctx, `SELECT doc_path, mention_type, mention_value, source_block FROM doc_mentions ORDER BY doc_path ASC, mention_type ASC, mention_value ASC`)
	legacy := false
	if err != nil && isSQLiteMissingColumnError(err, "source_block") {
		rows, err = tx.QueryContext(ctx, `SELECT doc_path, mention_type, mention_value FROM doc_mentions ORDER BY doc_path ASC, mention_type ASC, mention_value ASC`)
		legacy = true
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.DocMention
	for rows.Next() {
		item, err := scanDocMention(rows, legacy)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func readDocSourceBlocksTx(ctx context.Context, tx *sql.Tx) ([]model.DocSourceBlock, error) {
	rows, err := tx.QueryContext(ctx, `SELECT doc_path, block_id, doc_id, kind, source_format, ordinal, start_line, end_line, content_hash, indexed_at FROM doc_source_blocks ORDER BY doc_path ASC, ordinal ASC, block_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.DocSourceBlock
	for rows.Next() {
		var item model.DocSourceBlock
		if err := rows.Scan(&item.DocPath, &item.BlockID, &item.DocID, &item.Kind, &item.SourceFormat, &item.Ordinal, &item.StartLine, &item.EndLine, &item.ContentHash, &item.IndexedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func readDocSourceRecordsTx(ctx context.Context, tx *sql.Tx) ([]model.DocSourceRecord, error) {
	rows, err := tx.QueryContext(ctx, `SELECT doc_path, block_id, record_id, record_type, ordinal, start_line, end_line, content_hash, indexed_at FROM doc_source_records ORDER BY doc_path ASC, ordinal ASC, record_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.DocSourceRecord
	for rows.Next() {
		var item model.DocSourceRecord
		if err := rows.Scan(&item.DocPath, &item.BlockID, &item.RecordID, &item.RecordType, &item.Ordinal, &item.StartLine, &item.EndLine, &item.ContentHash, &item.IndexedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func readDocArtifactBindingsTx(ctx context.Context, tx *sql.Tx) ([]model.DocArtifactBinding, error) {
	rows, err := tx.QueryContext(ctx, `SELECT doc_path, block_id, doc_id, relation, role, target_path, target_symbol, target_kind, authoring_origin, binding_status, doc_lifecycle, superseded_by, ordinal, start_line, end_line, source_content_hash, binding_ref, indexed_at FROM doc_artifact_bindings ORDER BY doc_path ASC, ordinal ASC, block_id ASC, binding_ref ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.DocArtifactBinding
	for rows.Next() {
		var item model.DocArtifactBinding
		if err := rows.Scan(&item.DocPath, &item.BlockID, &item.DocID, &item.Relation, &item.Role, &item.TargetPath, &item.TargetSymbol, &item.TargetKind, &item.AuthoringOrigin, &item.BindingStatus, &item.DocLifecycle, &item.SupersededBy, &item.Ordinal, &item.StartLine, &item.EndLine, &item.SourceContentHash, &item.BindingRef, &item.IndexedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
