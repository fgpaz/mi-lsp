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

// IndexJobGraphPublication describes the graph generation that must be activated
// as part of the same transaction as an owned index publication. A nil value is
// valid for foreground catalog/docs-only work and for explicitly non-graph jobs.
type IndexJobGraphPublication struct {
	GenerationID            *model.GraphDigest
	ExpectedPrior           *model.GraphDigest
	PublishedAt             time.Time
	GraphCurrent            bool
	GraphBundle             *model.GraphBundle
	GenerationSkippedReason string
	CatalogGeneration       string
}

type IncrementalFileChange struct {
	FilePath    string
	RepoID      string
	RepoName    string
	Language    string
	ContentHash string
	Symbols     []model.SymbolRecord
	Deleted     bool
}

// ReplaceWorkspaceIndex is the foreground, no-job publication path. It has no
// ownership capability by design; job workers must use ReplaceWorkspaceIndexForJob.
func ReplaceWorkspaceIndex(ctx context.Context, db *sql.DB, generationID string, project model.ProjectFile, files []model.FileRecord, symbols []model.SymbolRecord, docs []model.DocRecord, edges []model.DocEdge, mentions []model.DocMention, sourceBlocks []model.DocSourceBlock, sourceRecords []model.DocSourceRecord, snapshot model.ReentryMemorySnapshot) error {
	return publishForeground(ctx, db, func(tx *sql.Tx) error {
		if err := replaceCatalogTx(ctx, tx, project, files, symbols); err != nil {
			return err
		}
		if err := replaceDocsWithSourcesTx(ctx, tx, docs, edges, mentions, sourceBlocks, sourceRecords); err != nil {
			return err
		}
		if err := saveReentrySnapshot(ctx, tx, snapshot); err != nil {
			return err
		}
		return publishGenerationTx(ctx, tx, generationID, "full", len(files), len(symbols), len(docs))
	})
}

// ReplaceWorkspaceIndexForJob is the fenced full-index publication path. The
// owner/state/cancellation CAS, index pointers, and optional graph pointer are
// committed or rolled back together.
func ReplaceWorkspaceIndexForJob(ctx context.Context, db *sql.DB, jobID, generationID string, project model.ProjectFile, files []model.FileRecord, symbols []model.SymbolRecord, docs []model.DocRecord, edges []model.DocEdge, mentions []model.DocMention, sourceBlocks []model.DocSourceBlock, sourceRecords []model.DocSourceRecord, snapshot model.ReentryMemorySnapshot, fence IndexJobFence, graph *IndexJobGraphPublication) error {
	return publishOwned(ctx, db, jobID, generationID, "full", len(files), len(symbols), len(docs), fence, graph, func(tx *sql.Tx) error {
		if err := replaceCatalogTx(ctx, tx, project, files, symbols); err != nil {
			return err
		}
		if err := replaceDocsWithSourcesTx(ctx, tx, docs, edges, mentions, sourceBlocks, sourceRecords); err != nil {
			return err
		}
		return saveReentrySnapshot(ctx, tx, snapshot)
	})
}

// ReplaceWorkspaceDocs is the foreground, no-job docs publication path.
func ReplaceWorkspaceDocs(ctx context.Context, db *sql.DB, generationID string, docs []model.DocRecord, edges []model.DocEdge, mentions []model.DocMention, sourceBlocks []model.DocSourceBlock, sourceRecords []model.DocSourceRecord, snapshot model.ReentryMemorySnapshot) error {
	return publishForeground(ctx, db, func(tx *sql.Tx) error {
		if err := replaceDocsWithSourcesTx(ctx, tx, docs, edges, mentions, sourceBlocks, sourceRecords); err != nil {
			return err
		}
		if err := saveReentrySnapshot(ctx, tx, snapshot); err != nil {
			return err
		}
		return publishGenerationTx(ctx, tx, generationID, "docs", 0, 0, len(docs))
	})
}

// ReplaceWorkspaceDocsForJob is the fenced docs publication path.
func ReplaceWorkspaceDocsForJob(ctx context.Context, db *sql.DB, jobID, generationID string, docs []model.DocRecord, edges []model.DocEdge, mentions []model.DocMention, sourceBlocks []model.DocSourceBlock, sourceRecords []model.DocSourceRecord, snapshot model.ReentryMemorySnapshot, fence IndexJobFence) error {
	return publishOwned(ctx, db, jobID, generationID, "docs", 0, 0, len(docs), fence, nil, func(tx *sql.Tx) error {
		if err := replaceDocsWithSourcesTx(ctx, tx, docs, edges, mentions, sourceBlocks, sourceRecords); err != nil {
			return err
		}
		return saveReentrySnapshot(ctx, tx, snapshot)
	})
}

// ReplaceWorkspaceCatalog is the foreground, no-job catalog publication path.
func ReplaceWorkspaceCatalog(ctx context.Context, db *sql.DB, generationID string, project model.ProjectFile, files []model.FileRecord, symbols []model.SymbolRecord) error {
	return publishForeground(ctx, db, func(tx *sql.Tx) error {
		if err := replaceCatalogTx(ctx, tx, project, files, symbols); err != nil {
			return err
		}
		return publishGenerationTx(ctx, tx, generationID, "catalog", len(files), len(symbols), 0)
	})
}

// ReplaceWorkspaceCatalogForJob is the fenced catalog publication path.
func ReplaceWorkspaceCatalogForJob(ctx context.Context, db *sql.DB, jobID, generationID string, project model.ProjectFile, files []model.FileRecord, symbols []model.SymbolRecord, fence IndexJobFence) error {
	return publishOwned(ctx, db, jobID, generationID, "catalog", len(files), len(symbols), 0, fence, nil, func(tx *sql.Tx) error {
		return replaceCatalogTx(ctx, tx, project, files, symbols)
	})
}

// PublishIncrementalGenerationForJob publishes the generation metadata and,
// when supplied, activates the staged graph in one fenced transaction.
func PublishIncrementalGenerationForJob(ctx context.Context, db *sql.DB, jobID, generationID string, files, symbols, docs int, fence IndexJobFence, graph *IndexJobGraphPublication) error {
	return PublishIncrementalGenerationForJobWithChanges(ctx, db, jobID, generationID, files, symbols, docs, fence, nil, graph)
}

// PublishIncrementalGenerationWithChanges is the foreground, no-job
// incremental publication path. It has no owner capability, but it still
// commits file rows, symbols, generation metadata, and the graph state as one
// SQLite transaction. The workspace write lock serializes callers; it is not
// used as a substitute for this transaction.
func PublishIncrementalGenerationWithChanges(ctx context.Context, db *sql.DB, generationID string, files, symbols, docs int, changes []IncrementalFileChange, graph *IndexJobGraphPublication) error {
	return publishForeground(ctx, db, func(tx *sql.Tx) error {
		if err := applyIncrementalFileChangesTx(ctx, tx, changes); err != nil {
			return err
		}
		return publishIncrementalGenerationTx(ctx, tx, generationID, files, symbols, docs, graph)
	})
}

// PublishIncrementalGenerationForJobWithChanges stages all file-row changes in
// the same owner/fence transaction as generation, graph, and terminal-job
// publication. A stale or canceled worker therefore rolls back every change.
func PublishIncrementalGenerationForJobWithChanges(ctx context.Context, db *sql.DB, jobID, generationID string, files, symbols, docs int, fence IndexJobFence, changes []IncrementalFileChange, graph *IndexJobGraphPublication) error {
	return publishOwned(ctx, db, jobID, generationID, "incremental", files, symbols, docs, fence, graph, func(tx *sql.Tx) error {
		return applyIncrementalFileChangesTx(ctx, tx, changes)
	})
}

func applyIncrementalFileChangesTx(ctx context.Context, tx *sql.Tx, changes []IncrementalFileChange) error {
	for _, change := range changes {
		if change.Deleted {
			if err := deleteFileSymbolsTx(ctx, tx, change.FilePath); err != nil {
				return err
			}
			continue
		}
		if err := replaceFileSymbolsTx(ctx, tx, change.FilePath, change.RepoID, change.RepoName, change.Language, change.ContentHash, change.Symbols); err != nil {
			return err
		}
	}
	return nil
}

func publishIncrementalGenerationTx(ctx context.Context, tx *sql.Tx, generationID string, files, symbols, docs int, graph *IndexJobGraphPublication) error {
	if err := publishGenerationTx(ctx, tx, generationID, "incremental", files, symbols, docs); err != nil {
		return err
	}
	if graph == nil {
		return setGraphRuntimeStateTx(ctx, tx, GraphRuntimeStale, "")
	}
	if graph.GenerationSkippedReason != "" {
		return setGraphRuntimeStateTx(ctx, tx, GraphRuntimeStale, "")
	}
	if graph.GraphBundle != nil {
		if err := StageGraphGenerationTx(ctx, tx, graph.GraphBundle); err != nil {
			return err
		}
	}
	if graph.GenerationID != nil {
		publishedAt := graph.PublishedAt
		if publishedAt.IsZero() {
			publishedAt = time.Now().UTC()
		}
		if err := activateGraphGenerationTx(ctx, tx, *graph.GenerationID, graph.ExpectedPrior, publishedAt); err != nil {
			return err
		}
	}
	if graph.GraphCurrent {
		catalogGeneration := generationID
		if catalogGeneration == "" {
			catalogGeneration = graph.CatalogGeneration
		}
		return setGraphRuntimeStateTx(ctx, tx, GraphRuntimeFresh, catalogGeneration)
	}
	return setGraphRuntimeStateTx(ctx, tx, GraphRuntimeStale, "")
}

func publishForeground(ctx context.Context, db *sql.DB, body func(*sql.Tx) error) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := body(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_ = PublishIndexPragmaOptimize(db)
	return nil
}

var (
	// These no-op seams make the ownership CAS and final commit boundaries
	// deterministic in package tests without changing the production path.
	indexPublicationBeforeOwnershipCASHook = func() error { return nil }
	indexPublicationBeforeCommitHook       = func() error { return nil }
)

// SetIndexPublicationBeforeCommitHookForTest installs a temporary test seam
// for a real fenced publication transaction. The returned function restores the
// previous hook and must be called by the test that installed it.
func SetIndexPublicationBeforeCommitHookForTest(hook func() error) func() {
	previous := indexPublicationBeforeCommitHook
	if hook == nil {
		indexPublicationBeforeCommitHook = func() error { return nil }
	} else {
		indexPublicationBeforeCommitHook = hook
	}
	return func() { indexPublicationBeforeCommitHook = previous }
}

func publishOwned(ctx context.Context, db *sql.DB, jobID, generationID, mode string, files, symbols, docs int, fence IndexJobFence, graph *IndexJobGraphPublication, body func(*sql.Tx) error) error {
	ownerToken, fencingToken, err := resolveIndexJobFence(fence)
	if err != nil {
		return err
	}
	if db == nil || jobID == "" {
		return ErrStaleIndexJobOwner
	}
	if err := ensureIndexJobOwnershipSchema(db); err != nil {
		return err
	}
	if err := indexPublicationBeforeOwnershipCASHook(); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `
		UPDATE index_jobs
		SET status='publishing', phase='publishing', current_stage='publishing', publication_started=1, updated_at=?
		WHERE job_id=? AND requested_cancel=0
		  AND ((status='running' AND publication_started=0) OR (status='publishing' AND publication_started=1))
		  AND EXISTS (SELECT 1 FROM index_job_ownership o
		             WHERE o.job_id=index_jobs.job_id AND o.owner_token=? AND o.fencing_token=? AND o.released_at IS NULL)
	`, now, jobID, ownerToken, fencingToken)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrStaleIndexJobOwner
	}

	if err := body(tx); err != nil {
		return err
	}
	if graph != nil && graph.GenerationSkippedReason != "" {
		result, err := tx.ExecContext(ctx, `
			UPDATE index_generations
			SET status='skipped', error=?
			WHERE generation_id=? AND job_id=? AND status='building'
		`, graph.GenerationSkippedReason, generationID, jobID)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return ErrStaleIndexJobOwner
		}
	} else if err := publishGenerationForJobTx(ctx, tx, jobID, generationID, mode, files, symbols, docs, now); err != nil {
		return err
	}
	if graph != nil {
		if graph.GenerationSkippedReason == "" {
			if graph.GraphBundle != nil {
				if err := StageGraphGenerationTx(ctx, tx, graph.GraphBundle); err != nil {
					return err
				}
			}
			if graph.GenerationID != nil {
				publishedAt := graph.PublishedAt
				if publishedAt.IsZero() {
					publishedAt = time.Now().UTC()
				}
				if err := activateGraphGenerationTx(ctx, tx, *graph.GenerationID, graph.ExpectedPrior, publishedAt); err != nil {
					return err
				}
			}
			if graph.GraphCurrent {
				if err := setGraphRuntimeStateTx(ctx, tx, GraphRuntimeFresh, generationID); err != nil {
					return err
				}
			} else if mode == "full" || mode == "incremental" {
				if err := setGraphRuntimeStateTx(ctx, tx, GraphRuntimeStale, ""); err != nil {
					return err
				}
			}
		}
	} else if mode == "full" || mode == "incremental" {
		if err := setGraphRuntimeStateTx(ctx, tx, GraphRuntimeStale, ""); err != nil {
			return err
		}
	}
	if err := completeIndexJobTx(ctx, tx, jobID, files, symbols, docs, ownerToken, fencingToken, true); err != nil {
		return err
	}
	if err := indexPublicationBeforeCommitHook(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_ = PublishIndexPragmaOptimize(db)
	return nil
}

func publishGenerationForJobTx(ctx context.Context, tx *sql.Tx, jobID, generationID, mode string, files, symbols, docs int, now string) error {
	if generationID == "" {
		return nil
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE index_generations
		SET status='published', files=?, symbols=?, docs=?, published_at=?, error=NULL
		WHERE generation_id=? AND job_id=? AND status IN ('building', 'published')
	`, files, symbols, docs, now, generationID, jobID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrStaleIndexJobOwner
	}
	metadata := map[string]string{
		WorkspaceMetaLastIndexGeneration: generationID,
		"active_generation_mode":         mode,
		"active_generation_published_at": now,
		"active_generation_files":        strconv.Itoa(files),
		"active_generation_symbols":      strconv.Itoa(symbols),
		"active_generation_docs":         strconv.Itoa(docs),
	}
	switch mode {
	case "full":
		metadata[WorkspaceMetaActiveCatalogGeneration] = generationID
		metadata[WorkspaceMetaActiveDocsGeneration] = generationID
		metadata[WorkspaceMetaActiveMemoryGeneration] = generationID
	case "docs":
		metadata[WorkspaceMetaActiveDocsGeneration] = generationID
		metadata[WorkspaceMetaActiveMemoryGeneration] = generationID
	case "catalog", "incremental":
		metadata[WorkspaceMetaActiveCatalogGeneration] = generationID
	default:
		return fmt.Errorf("unknown index generation mode %q", mode)
	}
	return UpsertWorkspaceMetaMap(ctx, tx, metadata)
}

func setGraphRuntimeStateTx(ctx context.Context, tx *sql.Tx, state, catalogGeneration string) error {
	if state != GraphRuntimeFresh && state != GraphRuntimeStale {
		return model.ErrGraphGenerationInvalid
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO workspace_meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", GraphRuntimeStateMeta, state); err != nil {
		return err
	}
	if catalogGeneration == "" {
		_, err := tx.ExecContext(ctx, "DELETE FROM workspace_meta WHERE key=?", GraphCatalogGenerationMeta)
		return err
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO workspace_meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", GraphCatalogGenerationMeta, catalogGeneration)
	return err
}

// activateGraphGenerationTx is the transaction-local equivalent of
// ActivateGraphGenerationAt. Keeping the CAS on this same transaction is what
// prevents a cancel/replacement from racing a job's pointer activation.
func activateGraphGenerationTx(ctx context.Context, tx *sql.Tx, id model.GraphDigest, expectedPrior *model.GraphDigest, publishedAt time.Time) error {
	if publishedAt.IsZero() {
		return model.ErrGraphGenerationInvalid
	}
	g, err := validateGraphGenerationConn(ctx, tx, id)
	if err != nil {
		return err
	}
	var old []byte
	metaErr := tx.QueryRowContext(ctx, "SELECT value FROM workspace_meta WHERE key=?", graphActiveMeta).Scan(&old)
	if metaErr != nil && metaErr != sql.ErrNoRows {
		return metaErr
	}
	if g.Status == model.GraphGenerationActive && len(old) > 0 && string(old) == string(digestArg(id)) {
		if expectedPrior != nil && g.PreviousGenerationID != nil && *g.PreviousGenerationID != *expectedPrior {
			return model.ErrGraphPointerConflict
		}
		return nil
	}
	var activeRows int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM graph_generations WHERE status=?", model.GraphGenerationActive).Scan(&activeRows); err != nil {
		return err
	}
	// A missing pointer with exactly one active generation is a repairable
	// dangling-pointer state. Resolve that prior generation inside this same
	// transaction; never repair workspace_meta on a separate connection before
	// the catalog/graph publication can commit.
	if len(old) == 0 && activeRows == 1 && expectedPrior == nil {
		if err := tx.QueryRowContext(ctx, "SELECT generation_id FROM graph_generations WHERE status=? LIMIT 2", model.GraphGenerationActive).Scan(&old); err != nil {
			return err
		}
	}
	if (expectedPrior != nil && string(digestArg(*expectedPrior)) != string(old)) || (expectedPrior == nil && len(old) == 0 && activeRows != 0) {
		return model.ErrGraphPointerConflict
	}
	if (len(old) == 0 && activeRows != 0) || (len(old) != 0 && activeRows != 1) {
		return model.ErrGraphPointerConflict
	}
	if len(old) > 0 {
		oldID, err := scanDigest(old)
		if err != nil {
			return model.ErrGraphPointerConflict
		}
		oldGeneration, err := validateGraphGenerationConn(ctx, tx, oldID)
		if err != nil || oldGeneration.Status != model.GraphGenerationActive {
			return model.ErrGraphPointerConflict
		}
		result, err := tx.ExecContext(ctx, "UPDATE graph_generations SET status=? WHERE generation_id=? AND status=?", model.GraphGenerationRetired, old, model.GraphGenerationActive)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return model.ErrGraphPointerConflict
		}
	}
	if g.Status != model.GraphGenerationStaged {
		return model.ErrGraphGenerationInvalid
	}
	result, err := tx.ExecContext(ctx, "UPDATE graph_generations SET status=?,published_at=?,previous_generation_id=? WHERE generation_id=? AND status=?", model.GraphGenerationActive, publishedAt.UTC().Format(time.RFC3339Nano), old, digestArg(id), model.GraphGenerationStaged)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return model.ErrGraphPointerConflict
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO workspace_meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", graphPreviousMeta, old); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO workspace_meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", graphActiveMeta, digestArg(id))
	return err
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
	case "catalog", "incremental":
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
