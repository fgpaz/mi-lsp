package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

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

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM wiki_chunk_embeddings
		WHERE doc_path = '' OR chunk_id = '' OR embedding IS NULL OR length(embedding) = 0
	`); err != nil {
		return err
	}

	// Delete existing embeddings for these doc paths
	if len(docPaths) > 0 {
		placeholders := make([]string, len(docPaths))
		args := make([]interface{}, len(docPaths))
		for i, p := range docPaths {
			placeholders[i] = "?"
			args[i] = p
		}
		deleteQuery := "DELETE FROM wiki_chunk_embeddings WHERE doc_path IN (" + joinStrings(placeholders, ",") + ")"
		if _, err := tx.ExecContext(ctx, deleteQuery, args...); err != nil {
			return err
		}
	}

	// Insert new chunks
	if len(chunks) > 0 {
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
			if _, err := stmt.ExecContext(ctx, chunk.DocPath, chunk.ChunkID, chunk.StartLine, chunk.EndLine, chunk.Heading, chunk.Snippet, chunk.ContentHash, chunk.Embedding, chunk.EmbeddingModel, chunk.EmbeddingDim, now); err != nil {
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
