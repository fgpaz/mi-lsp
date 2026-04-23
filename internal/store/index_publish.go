package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const (
	WorkspaceMetaActiveCatalogGeneration = "active_catalog_generation_id"
	WorkspaceMetaActiveDocsGeneration    = "active_docs_generation_id"
	WorkspaceMetaActiveMemoryGeneration  = "active_memory_generation_id"
	WorkspaceMetaLastIndexGeneration     = "last_index_generation_id"
)

func ReplaceWorkspaceIndex(ctx context.Context, db *sql.DB, generationID string, project model.ProjectFile, files []model.FileRecord, symbols []model.SymbolRecord, docs []model.DocRecord, edges []model.DocEdge, mentions []model.DocMention, snapshot model.ReentryMemorySnapshot) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := replaceCatalogTx(ctx, tx, project, files, symbols); err != nil {
		return err
	}
	if err := replaceDocsTx(ctx, tx, docs, edges, mentions); err != nil {
		return err
	}
	if err := saveReentrySnapshot(ctx, tx, snapshot); err != nil {
		return err
	}
	if err := publishGenerationTx(ctx, tx, generationID, "full", len(files), len(symbols), len(docs)); err != nil {
		return err
	}
	return tx.Commit()
}

func ReplaceWorkspaceDocs(ctx context.Context, db *sql.DB, generationID string, docs []model.DocRecord, edges []model.DocEdge, mentions []model.DocMention, snapshot model.ReentryMemorySnapshot) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := replaceDocsTx(ctx, tx, docs, edges, mentions); err != nil {
		return err
	}
	if err := saveReentrySnapshot(ctx, tx, snapshot); err != nil {
		return err
	}
	if err := publishGenerationTx(ctx, tx, generationID, "docs", 0, 0, len(docs)); err != nil {
		return err
	}
	return tx.Commit()
}

func ReplaceWorkspaceCatalog(ctx context.Context, db *sql.DB, generationID string, project model.ProjectFile, files []model.FileRecord, symbols []model.SymbolRecord) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := replaceCatalogTx(ctx, tx, project, files, symbols); err != nil {
		return err
	}
	if err := publishGenerationTx(ctx, tx, generationID, "catalog", len(files), len(symbols), 0); err != nil {
		return err
	}
	return tx.Commit()
}

func publishGenerationTx(ctx context.Context, tx *sql.Tx, generationID string, mode string, files int, symbols int, docs int) error {
	if generationID == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		UPDATE index_generations
		SET status = 'published', files = ?, symbols = ?, docs = ?, published_at = ?, error = NULL
		WHERE generation_id = ?
	`, files, symbols, docs, now, generationID); err != nil {
		return err
	}
	metadata := map[string]string{
		WorkspaceMetaLastIndexGeneration: generationID,
	}
	switch mode {
	case "full":
		metadata[WorkspaceMetaActiveCatalogGeneration] = generationID
		metadata[WorkspaceMetaActiveDocsGeneration] = generationID
		metadata[WorkspaceMetaActiveMemoryGeneration] = generationID
	case "docs":
		metadata[WorkspaceMetaActiveDocsGeneration] = generationID
		metadata[WorkspaceMetaActiveMemoryGeneration] = generationID
	case "catalog":
		metadata[WorkspaceMetaActiveCatalogGeneration] = generationID
	default:
		return fmt.Errorf("unknown index generation mode %q", mode)
	}
	metadata["active_generation_mode"] = mode
	metadata["active_generation_published_at"] = now
	metadata["active_generation_files"] = strconv.Itoa(files)
	metadata["active_generation_symbols"] = strconv.Itoa(symbols)
	metadata["active_generation_docs"] = strconv.Itoa(docs)
	return UpsertWorkspaceMetaMap(ctx, tx, metadata)
}
