package store

import (
	"context"
	"database/sql"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// ReadCatalog pins files and symbols to the same committed SQLite snapshot.
// It never copies graph generations, jobs, locks or workspace identity.
func ReadCatalog(ctx context.Context, db *sql.DB) (map[string]model.FileRecord, map[string][]model.SymbolRecord, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT file_path, repo_id, repo_name, content_hash, indexed_at, language FROM files")
	if err != nil {
		return nil, nil, err
	}
	files := map[string]model.FileRecord{}
	for rows.Next() {
		var f model.FileRecord
		if err := rows.Scan(&f.FilePath, &f.RepoID, &f.RepoName, &f.ContentHash, &f.IndexedAt, &f.Language); err != nil {
			rows.Close()
			return nil, nil, err
		}
		files[f.FilePath] = f
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT "+symbolColumns+" FROM symbols ORDER BY id")
	if err != nil {
		return nil, nil, err
	}
	symbols, err := scanSymbols(rows)
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	byFile := map[string][]model.SymbolRecord{}
	for _, symbol := range symbols {
		byFile[symbol.FilePath] = append(byFile[symbol.FilePath], symbol)
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return files, byFile, nil
}
