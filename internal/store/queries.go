package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func ReplaceCatalog(ctx context.Context, db *sql.DB, project model.ProjectFile, files []model.FileRecord, symbols []model.SymbolRecord) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, table := range []string{"files", "symbols", "workspace_repos", "workspace_entrypoints"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return err
		}
	}

	repoStmt, err := tx.PrepareContext(ctx, `INSERT INTO workspace_repos(repo_id, name, root, languages, default_entrypoint) VALUES(?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer repoStmt.Close()
	for _, repo := range project.Repos {
		if _, err := repoStmt.ExecContext(ctx, repo.ID, repo.Name, repo.Root, strings.Join(repo.Languages, ","), repo.DefaultEntrypoint); err != nil {
			return err
		}
	}

	entrypointStmt, err := tx.PrepareContext(ctx, `INSERT INTO workspace_entrypoints(entrypoint_id, repo_id, path, kind, is_default) VALUES(?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer entrypointStmt.Close()
	for _, entrypoint := range project.Entrypoints {
		if _, err := entrypointStmt.ExecContext(ctx, entrypoint.ID, entrypoint.RepoID, entrypoint.Path, entrypoint.Kind, boolToInt(entrypoint.Default)); err != nil {
			return err
		}
	}

	fileStmt, err := tx.PrepareContext(ctx, "INSERT INTO files(file_path, repo_id, repo_name, content_hash, indexed_at, language) VALUES(?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer fileStmt.Close()
	for _, file := range files {
		if _, err := fileStmt.ExecContext(ctx, file.FilePath, file.RepoID, file.RepoName, file.ContentHash, file.IndexedAt, file.Language); err != nil {
			return err
		}
	}

	symbolStmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO symbols(
			file_path, repo_id, repo_name, name, kind, start_line, end_line, parent, qualified_name, signature, signature_hash, scope, language, file_hash, implements, search_text
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer symbolStmt.Close()
	for _, symbol := range symbols {
		if _, err := symbolStmt.ExecContext(
			ctx,
			symbol.FilePath,
			symbol.RepoID,
			symbol.RepoName,
			symbol.Name,
			symbol.Kind,
			symbol.StartLine,
			symbol.EndLine,
			symbol.Parent,
			symbol.QualifiedName,
			symbol.Signature,
			symbol.SignatureHash,
			symbol.Scope,
			symbol.Language,
			symbol.FileHash,
			symbol.Implements,
			symbol.SearchText,
		); err != nil {
			return err
		}
	}

	metadata := map[string]string{
		"indexed_at":         fmt.Sprintf("%d", time.Now().Unix()),
		"total_files":        fmt.Sprintf("%d", len(files)),
		"total_symbols":      fmt.Sprintf("%d", len(symbols)),
		"workspace_kind":     project.Project.Kind,
		"default_repo":       project.Project.DefaultRepo,
		"default_entrypoint": project.Project.DefaultEntrypoint,
		"repo_count":         fmt.Sprintf("%d", len(project.Repos)),
	}
	for key, value := range metadata {
		if _, err := tx.ExecContext(ctx, "INSERT OR REPLACE INTO workspace_meta(key, value) VALUES(?, ?)", key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func WorkspaceStats(ctx context.Context, db *sql.DB) (model.Stats, error) {
	stats := model.Stats{}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM symbols").Scan(&stats.Symbols); err != nil {
		return stats, err
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM files").Scan(&stats.Files); err != nil {
		return stats, err
	}
	return stats, nil
}

func SymbolsByFile(ctx context.Context, db *sql.DB, filePath string, limit int, offset int) ([]model.SymbolRecord, error) {
	if limit <= 0 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := db.QueryContext(ctx, `
		SELECT `+symbolColumns+`
		FROM symbols
		WHERE file_path = ?
		ORDER BY start_line ASC
		LIMIT ? OFFSET ?
	`, filePath, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSymbols(rows)
}

func FindSymbols(ctx context.Context, db *sql.DB, pattern string, kind string, exact bool, limit int, offset int) ([]model.SymbolRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	query := `
		SELECT ` + symbolColumns + `
		FROM symbols
		WHERE name LIKE ?
	`
	arg := "%" + pattern + "%"
	if exact {
		query = `
			SELECT ` + symbolColumns + `
			FROM symbols
			WHERE name = ?
		`
		arg = pattern
	}
	args := []any{arg}
	if kind != "" {
		query += " AND kind = ?"
		args = append(args, kind)
	}
	query += " ORDER BY name ASC, file_path ASC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSymbols(rows)
}

func OverviewByPrefix(ctx context.Context, db *sql.DB, prefix string, limit int, offset int) ([]model.SymbolRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := db.QueryContext(ctx, `
		SELECT `+symbolColumns+`
		FROM symbols
		WHERE file_path LIKE ?
		ORDER BY file_path ASC, start_line ASC
		LIMIT ? OFFSET ?
	`, prefix+"%", limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSymbols(rows)
}

func SymbolContainingLine(ctx context.Context, db *sql.DB, filePath string, lineNum int) (model.SymbolRecord, bool, error) {
	row := db.QueryRowContext(ctx, `
		SELECT `+symbolColumns+`
		FROM symbols
		WHERE file_path = ? AND start_line <= ? AND end_line >= ?
		ORDER BY (end_line - start_line) ASC
		LIMIT 1
	`, filePath, lineNum, lineNum)

	var item model.SymbolRecord
	err := row.Scan(
		&item.ID,
		&item.FilePath,
		&item.RepoID,
		&item.RepoName,
		&item.Name,
		&item.Kind,
		&item.StartLine,
		&item.EndLine,
		&item.Parent,
		&item.QualifiedName,
		&item.Signature,
		&item.SignatureHash,
		&item.Scope,
		&item.Language,
		&item.FileHash,
		&item.Implements,
		&item.SearchText,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.SymbolRecord{}, false, nil
		}
		return model.SymbolRecord{}, false, err
	}
	return item, true, nil
}

func CandidateReposForSymbol(ctx context.Context, db *sql.DB, symbol string, exact bool, limit int) ([]model.WorkspaceRepo, error) {
	if limit <= 0 {
		limit = 12
	}
	operator := "LIKE"
	argument := "%" + symbol + "%"
	if exact {
		operator = "="
		argument = symbol
	}
	// Validate operator to prevent SQL injection
	if operator != "LIKE" && operator != "=" {
		return nil, fmt.Errorf("invalid operator: %s", operator)
	}
	rows, err := db.QueryContext(ctx, `
		SELECT s.repo_id, COALESCE(s.repo_name, ''), COALESCE(r.root, '')
		FROM symbols s
		LEFT JOIN workspace_repos r ON r.repo_id = s.repo_id
		WHERE s.name `+operator+` ? AND s.repo_id <> ''
		GROUP BY s.repo_id, s.repo_name, r.root
		ORDER BY COUNT(*) DESC, s.repo_name ASC
		LIMIT ?
	`, argument, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.WorkspaceRepo, 0, limit)
	for rows.Next() {
		var item model.WorkspaceRepo
		if err := rows.Scan(&item.ID, &item.Name, &item.Root); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

const symbolColumns = `id, file_path, repo_id, repo_name, name, kind, start_line, end_line, parent, qualified_name, signature, signature_hash, scope, language, file_hash, implements, search_text`

func scanSymbols(rows *sql.Rows) ([]model.SymbolRecord, error) {
	items := make([]model.SymbolRecord, 0)
	for rows.Next() {
		var item model.SymbolRecord
		if err := rows.Scan(
			&item.ID,
			&item.FilePath,
			&item.RepoID,
			&item.RepoName,
			&item.Name,
			&item.Kind,
			&item.StartLine,
			&item.EndLine,
			&item.Parent,
			&item.QualifiedName,
			&item.Signature,
			&item.SignatureHash,
			&item.Scope,
			&item.Language,
			&item.FileHash,
			&item.Implements,
			&item.SearchText,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// IntentSearch retrieves symbols whose search_text matches any of the given tokens.
func IntentSearch(ctx context.Context, db *sql.DB, tokens []string, limit int, offset int) ([]model.SymbolRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	if len(tokens) == 0 {
		return []model.SymbolRecord{}, nil
	}

	var whereClauses []string
	var args []any
	for _, token := range tokens {
		whereClauses = append(whereClauses, "search_text LIKE ?")
		args = append(args, "%"+token+"%")
	}
	args = append(args, limit, offset)

	query := `
		SELECT ` + symbolColumns + `
		FROM symbols
		WHERE search_text IS NOT NULL AND (` + strings.Join(whereClauses, " OR ") + `)
		LIMIT ? OFFSET ?
	`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSymbols(rows)
}
