package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const embeddingDeleteBatchSize = 500

// LoadWikiChunkEmbeddings returns a map keyed by docPath+"\x00"+chunkID of all stored embeddings.
func LoadWikiChunkEmbeddings(ctx context.Context, db *sql.DB) (map[string]model.WikiChunkEmbedding, error) {
	rows, err := QueryContextWithRetry(ctx, db, `
		SELECT doc_path, chunk_id, start_line, end_line, heading_text, snippet, content_hash, embedding, embedding_model, embedding_dim, indexed_at
		FROM wiki_chunk_embeddings
		ORDER BY doc_path ASC, chunk_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]model.WikiChunkEmbedding)
	for rows.Next() {
		var item model.WikiChunkEmbedding
		if err := rows.Scan(&item.DocPath, &item.ChunkID, &item.StartLine, &item.EndLine, &item.Heading, &item.Snippet, &item.ContentHash, &item.Embedding, &item.EmbeddingModel, &item.EmbeddingDim, &item.IndexedAt); err != nil {
			return nil, err
		}
		key := item.DocPath + "\x00" + item.ChunkID
		result[key] = item
	}
	return result, rows.Err()
}

// ReplaceWikiChunkEmbeddingsForDocs deletes all embeddings for the given docPaths and inserts the new chunks in a transaction.
func ReplaceWikiChunkEmbeddingsForDocs(ctx context.Context, db *sql.DB, docPaths []string, chunks []model.WikiChunkEmbedding) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := deleteInvalidWikiChunkEmbeddingsTx(ctx, tx); err != nil {
		return err
	}

	// Delete existing embeddings for these doc paths
	for start := 0; start < len(docPaths); start += embeddingDeleteBatchSize {
		end := start + embeddingDeleteBatchSize
		if end > len(docPaths) {
			end = len(docPaths)
		}
		batch := docPaths[start:end]
		placeholders := make([]string, len(batch))
		args := make([]interface{}, len(batch))
		for i, p := range batch {
			placeholders[i] = "?"
			args[i] = p
		}
		deleteQuery := "DELETE FROM wiki_chunk_embeddings WHERE doc_path IN (" + joinStrings(placeholders, ",") + ")"
		if _, err := tx.ExecContext(ctx, deleteQuery, args...); err != nil {
			return err
		}
	}

	if err := upsertWikiChunkEmbeddingsTx(ctx, tx, chunks); err != nil {
		return err
	}

	return tx.Commit()
}

// PruneInvalidWikiChunkEmbeddings removes malformed rows without touching valid progress.
func PruneInvalidWikiChunkEmbeddings(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := deleteInvalidWikiChunkEmbeddingsTx(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

// UpsertWikiChunkEmbeddings persists a committed embeddings batch without deleting
// unrelated chunks, allowing interrupted index runs to resume from stored work.
func UpsertWikiChunkEmbeddings(ctx context.Context, db *sql.DB, chunks []model.WikiChunkEmbedding) error {
	if len(chunks) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsertWikiChunkEmbeddingsTx(ctx, tx, chunks); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteStaleWikiChunkEmbeddingsForDocs removes rows for indexed docs that were
// not successfully stored in the current run. It is intentionally called only
// after the embeddings phase finishes, so interrupted runs keep prior progress.
func DeleteStaleWikiChunkEmbeddingsForDocs(ctx context.Context, db *sql.DB, docPaths []string, keepKeys map[string]struct{}) error {
	if len(docPaths) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := deleteInvalidWikiChunkEmbeddingsTx(ctx, tx); err != nil {
		return err
	}
	deleteStmt, err := tx.PrepareContext(ctx, `DELETE FROM wiki_chunk_embeddings WHERE doc_path = ? AND chunk_id = ?`)
	if err != nil {
		return err
	}
	defer deleteStmt.Close()
	for _, docPath := range docPaths {
		rows, err := tx.QueryContext(ctx, `SELECT chunk_id FROM wiki_chunk_embeddings WHERE doc_path = ?`, docPath)
		if err != nil {
			return err
		}
		var stale []string
		for rows.Next() {
			var chunkID string
			if err := rows.Scan(&chunkID); err != nil {
				_ = rows.Close()
				return err
			}
			if _, ok := keepKeys[docPath+"\x00"+chunkID]; !ok {
				stale = append(stale, chunkID)
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, chunkID := range stale {
			if _, err := deleteStmt.ExecContext(ctx, docPath, chunkID); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// AllWikiChunkEmbeddings returns all stored wiki chunk embeddings (for recall ranking).
func AllWikiChunkEmbeddings(ctx context.Context, db *sql.DB) ([]model.WikiChunkEmbedding, error) {
	rows, err := QueryContextWithRetry(ctx, db, `
		SELECT doc_path, chunk_id, start_line, end_line, heading_text, snippet, content_hash, embedding, embedding_model, embedding_dim, indexed_at
		FROM wiki_chunk_embeddings
		ORDER BY doc_path ASC, chunk_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.WikiChunkEmbedding
	for rows.Next() {
		var item model.WikiChunkEmbedding
		if err := rows.Scan(&item.DocPath, &item.ChunkID, &item.StartLine, &item.EndLine, &item.Heading, &item.Snippet, &item.ContentHash, &item.Embedding, &item.EmbeddingModel, &item.EmbeddingDim, &item.IndexedAt); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func deleteInvalidWikiChunkEmbeddingsTx(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		DELETE FROM wiki_chunk_embeddings
		WHERE doc_path = '' OR chunk_id = '' OR embedding IS NULL OR length(embedding) = 0
	`)
	return err
}

func upsertWikiChunkEmbeddingsTx(ctx context.Context, tx *sql.Tx, chunks []model.WikiChunkEmbedding) error {
	if len(chunks) == 0 {
		return nil
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO wiki_chunk_embeddings(doc_path, chunk_id, start_line, end_line, heading_text, snippet, content_hash, embedding, embedding_model, embedding_dim, indexed_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().Unix()
	for _, chunk := range chunks {
		if chunk.DocPath == "" || chunk.ChunkID == "" || len(chunk.Embedding) == 0 {
			continue
		}
		indexedAt := chunk.IndexedAt
		if indexedAt <= 0 {
			indexedAt = now
		}
		if _, err := stmt.ExecContext(ctx, chunk.DocPath, chunk.ChunkID, chunk.StartLine, chunk.EndLine, chunk.Heading, chunk.Snippet, chunk.ContentHash, chunk.Embedding, chunk.EmbeddingModel, chunk.EmbeddingDim, indexedAt); err != nil {
			return err
		}
	}
	return nil
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
