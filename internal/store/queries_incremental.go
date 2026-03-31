package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// ReplaceFileSymbols deletes and re-inserts symbols and file record for a single file.
// This is used by the file watcher for incremental indexing.
func ReplaceFileSymbols(ctx context.Context, db *sql.DB, filePath string, repoID string, repoName string, language string, contentHash string, symbols []model.SymbolRecord) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Delete existing symbols and file record for this path
	if _, err := tx.ExecContext(ctx, "DELETE FROM symbols WHERE file_path = ?", filePath); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM files WHERE file_path = ?", filePath); err != nil {
		return err
	}

	// Insert updated file record
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO files(file_path, repo_id, repo_name, content_hash, indexed_at, language) VALUES(?, ?, ?, ?, ?, ?)",
		filePath, repoID, repoName, contentHash, time.Now().Unix(), language,
	); err != nil {
		return err
	}

	// Insert updated symbols
	if len(symbols) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT OR REPLACE INTO symbols(
				file_path, repo_id, repo_name, name, kind, start_line, end_line, parent, qualified_name, signature, signature_hash, scope, language, file_hash, implements, search_text
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, sym := range symbols {
			if _, err := stmt.ExecContext(ctx,
				sym.FilePath, sym.RepoID, sym.RepoName, sym.Name, sym.Kind,
				sym.StartLine, sym.EndLine, sym.Parent, sym.QualifiedName,
				sym.Signature, sym.SignatureHash, sym.Scope, sym.Language,
				sym.FileHash, sym.Implements, sym.SearchText,
			); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// DeleteFileSymbols removes all symbols and file record for a deleted file.
// Used by incremental indexing when files are deleted.
func DeleteFileSymbols(ctx context.Context, db *sql.DB, filePath string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Delete symbols and file record
	if _, err := tx.ExecContext(ctx, "DELETE FROM symbols WHERE file_path = ?", filePath); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM files WHERE file_path = ?", filePath); err != nil {
		return err
	}

	return tx.Commit()
}
