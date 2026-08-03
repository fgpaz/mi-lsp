package indexer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/reentry"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

type Result struct {
	Files                []model.FileRecord
	Symbols              []model.SymbolRecord
	Docs                 int
	Stats                model.Stats
	Warnings             []string
	GraphGenerationID    string
	GraphBackendManifest string
	GraphOmissions       []model.GraphObservationOmission
	GraphNotApplicable   bool
}

type Progress struct {
	Stage      string
	Path       string
	Files      int
	Symbols    int
	Docs       int
	FilesTotal int
	Force      bool
}

type ProgressFunc func(context.Context, Progress) error

type IndexJobPublication struct {
	JobID string
	Fence store.IndexJobFence
}

func IndexWorkspace(ctx context.Context, root string, clean bool) (Result, error) {
	return IndexWorkspaceWithGeneration(ctx, root, clean, "")
}

func IndexWorkspaceWithGeneration(ctx context.Context, root string, clean bool, generationID string) (Result, error) {
	return IndexWorkspaceWithProgress(ctx, root, clean, generationID, nil)
}

func IndexWorkspaceWithProgress(ctx context.Context, root string, clean bool, generationID string, progress ProgressFunc) (Result, error) {
	return IndexWorkspaceWithGraphProgress(ctx, root, clean, generationID, progress, GraphIndexOptions{})
}

func IndexWorkspaceWithGraphProgress(ctx context.Context, root string, clean bool, generationID string, progress ProgressFunc, graphOptions GraphIndexOptions) (Result, error) {
	return indexWorkspaceWithGraphProgress(ctx, root, clean, generationID, progress, graphOptions, nil)
}

// IndexWorkspaceWithGraphProgressForJob is the explicit owner-bound full index
// path. Its final catalog/docs/generation/graph publication uses one fenced
// store transaction; it never falls through to the foreground publisher.
func IndexWorkspaceWithGraphProgressForJob(ctx context.Context, root string, clean bool, generationID, jobID string, fence store.IndexJobFence, progress ProgressFunc, graphOptions GraphIndexOptions) (Result, error) {
	return indexWorkspaceWithGraphProgress(ctx, root, clean, generationID, progress, graphOptions, &IndexJobPublication{JobID: jobID, Fence: fence})
}

func indexWorkspaceWithGraphProgress(ctx context.Context, root string, clean bool, generationID string, progress ProgressFunc, graphOptions GraphIndexOptions, publication *IndexJobPublication) (Result, error) {
	started := time.Now()
	projectFile, files, symbols, warnings, matcher, err := buildCatalog(ctx, root, progress)
	if err != nil {
		return Result{}, err
	}
	if clean {
		warnings = appendIfMissing(warnings, "clean=true")
	}

	// Observe before publishing catalog data so an observer failure preserves the prior graph state.
	graphBatches, graphOmissions, graphWarnings, err := ObserveGraph(ctx, root, projectFile, graphOptions, progress)
	if err != nil {
		return Result{}, err
	}
	// Complete-only staging may omit every Roslyn/Go batch (e.g. all projects partial due to
	// compiler diagnostics). That is not a catalog failure: publish files/symbols/docs and mark
	// graph stale. ObserveGraph already hard-fails on invalid seals / missing entrypoints.
	if len(graphBatches) == 0 && !explicitlyNonGraphProject(projectFile) {
		warnings = append(warnings, "graph observation produced no stageable complete batch; publishing catalog with graph stale")
	}
	warnings = append(warnings, graphWarnings...)

	priorDocs := loadPriorDocSnapshot(ctx, root)
	docs, docEdges, docMentions, sourceBlocks, sourceRecords, docWarnings, err := docgraph.IndexWorkspaceDocsWithSourcesWithProgressPrior(ctx, root, matcher, func(ctx context.Context, progressValue docgraph.Progress) error {
		return reportProgress(ctx, progress, Progress{
			Stage:      progressValue.Stage,
			Path:       progressValue.Path,
			Files:      len(files),
			Symbols:    len(symbols),
			Docs:       progressValue.Docs,
			FilesTotal: progressValue.FilesTotal,
			Force:      progressValue.Force,
		})
	}, priorDocs)
	if err != nil {
		return Result{}, err
	}
	warnings = append(warnings, docWarnings...)
	snapshot := reentry.BuildSnapshot(root, docs, time.Now())
	if err := reportProgress(ctx, progress, Progress{Stage: "publishing", Files: len(files), Symbols: len(symbols), Docs: len(docs), FilesTotal: len(files), Force: true}); err != nil {
		return Result{}, err
	}

	var graphGeneration model.GraphGeneration
	if err := store.WithWorkspaceWriteLock(root, func() error {
		db, err := store.Open(root)
		if err != nil {
			return err
		}
		defer db.Close()

		// Capture the pointer before catalog publication; activation must reject a concurrent replacement.
		var prior *model.GraphDigest
		if active, ok, activeErr := store.ActiveGraphGeneration(ctx, db); activeErr != nil {
			return activeErr
		} else if ok {
			prior = &active
		}
		request := GraphAssemblyRequest{Batches: graphBatches, Docs: docs, DocEdges: docEdges, DocMentions: docMentions, CreatedAt: time.Now().UTC()}
		var jobGraph *store.IndexJobGraphPublication
		if len(graphBatches) != 0 {
			if err := reportProgress(ctx, progress, Progress{Stage: "graph.activate", Files: len(files), Symbols: len(symbols), Docs: len(docs), Force: true}); err != nil {
				return err
			}
			if publication != nil {
				bundle, assembleErr := AssembleGraphObservationBatches(request)
				if assembleErr != nil {
					return fmt.Errorf("graph staging failed: %w", assembleErr)
				}
				graphGeneration = bundle.Generation
				jobGraph = &store.IndexJobGraphPublication{GenerationID: &bundle.Generation.GenerationID, ExpectedPrior: prior, PublishedAt: request.CreatedAt, GraphCurrent: true, GraphBundle: &bundle}
			} else {
				graphGeneration, err = PublishGraphObservationBatches(ctx, db, request, prior)
				if err != nil {
					return fmt.Errorf("graph publication failed: %w", err)
				}
			}
		}
		if publication != nil {
			return store.ReplaceWorkspaceIndexForJob(ctx, db, publication.JobID, generationID, projectFile, files, symbols, docs, docEdges, docMentions, sourceBlocks, sourceRecords, snapshot, publication.Fence, jobGraph)
		}
		if err := store.ReplaceWorkspaceIndex(ctx, db, generationID, projectFile, files, symbols, docs, docEdges, docMentions, sourceBlocks, sourceRecords, snapshot); err != nil {
			return err
		}
		if len(graphBatches) == 0 {
			return store.SetGraphRuntimeState(ctx, db, store.GraphRuntimeStale, "")
		}
		return store.SetGraphRuntimeState(ctx, db, store.GraphRuntimeFresh, generationID)
	}); err != nil {
		return Result{}, err
	}

	result := Result{Files: files, Symbols: symbols, Docs: len(docs), Warnings: warnings, GraphOmissions: graphOmissions,
		Stats: model.Stats{Files: len(files), Symbols: len(symbols), Ms: time.Since(started).Milliseconds()}}
	if graphGeneration.GenerationID != (model.GraphDigest{}) {
		result.GraphGenerationID = graphGeneration.GenerationID.String()
		result.GraphBackendManifest = graphGeneration.BackendManifestDigest.String()
	}
	return result, nil
}

func IndexWorkspaceCatalogOnly(ctx context.Context, root string, clean bool) (Result, error) {
	return IndexWorkspaceCatalogOnlyWithGeneration(ctx, root, clean, "")
}

func IndexWorkspaceCatalogOnlyWithGeneration(ctx context.Context, root string, clean bool, generationID string) (Result, error) {
	return IndexWorkspaceCatalogOnlyWithProgress(ctx, root, clean, generationID, nil)
}

func IndexWorkspaceCatalogOnlyWithProgress(ctx context.Context, root string, clean bool, generationID string, progress ProgressFunc) (Result, error) {
	return indexWorkspaceCatalogOnlyWithProgress(ctx, root, clean, generationID, progress, nil)
}

// IndexWorkspaceCatalogOnlyWithProgressForJob is the owner-bound catalog path.
func IndexWorkspaceCatalogOnlyWithProgressForJob(ctx context.Context, root string, clean bool, generationID, jobID string, fence store.IndexJobFence, progress ProgressFunc) (Result, error) {
	return indexWorkspaceCatalogOnlyWithProgress(ctx, root, clean, generationID, progress, &IndexJobPublication{JobID: jobID, Fence: fence})
}

func indexWorkspaceCatalogOnlyWithProgress(ctx context.Context, root string, clean bool, generationID string, progress ProgressFunc, publication *IndexJobPublication) (Result, error) {
	started := time.Now()
	projectFile, files, symbols, warnings, _, err := buildCatalog(ctx, root, progress)
	if err != nil {
		return Result{}, err
	}
	if clean {
		warnings = appendIfMissing(warnings, "clean=true")
	}

	if err := reportProgress(ctx, progress, Progress{Stage: "publishing", Files: len(files), Symbols: len(symbols), FilesTotal: len(files), Force: true}); err != nil {
		return Result{}, err
	}

	if err := store.WithWorkspaceWriteLock(root, func() error {
		db, err := store.Open(root)
		if err != nil {
			return err
		}
		defer db.Close()

		if publication != nil {
			return store.ReplaceWorkspaceCatalogForJob(ctx, db, publication.JobID, generationID, projectFile, files, symbols, publication.Fence)
		}
		return store.ReplaceWorkspaceCatalog(ctx, db, generationID, projectFile, files, symbols)
	}); err != nil {
		return Result{}, err
	}

	return Result{
		Files:    files,
		Symbols:  symbols,
		Docs:     0,
		Warnings: warnings,
		Stats: model.Stats{
			Files:   len(files),
			Symbols: len(symbols),
			Ms:      time.Since(started).Milliseconds(),
		},
	}, nil
}

func buildCatalog(ctx context.Context, root string, progress ProgressFunc) (model.ProjectFile, []model.FileRecord, []model.SymbolRecord, []string, *workspace.IgnoreMatcher, error) {
	if err := reportProgress(ctx, progress, Progress{Stage: "catalog.detect", Force: true}); err != nil {
		return model.ProjectFile{}, nil, nil, nil, nil, err
	}
	registration, err := workspace.DetectWorkspace(root)
	if err != nil {
		return model.ProjectFile{}, nil, nil, nil, nil, err
	}
	projectFile, err := workspace.LoadProjectTopology(root, registration)
	if err != nil {
		return model.ProjectFile{}, nil, nil, nil, nil, err
	}
	matcher, err := workspace.LoadIgnoreMatcher(root, projectFile.Ignore.ExtraPatterns)
	if err != nil {
		return model.ProjectFile{}, nil, nil, nil, nil, err
	}
	if err := reportProgress(ctx, progress, Progress{Stage: "catalog.walk", Force: true}); err != nil {
		return model.ProjectFile{}, nil, nil, nil, nil, err
	}
	candidates, err := WalkWorkspace(ctx, root, matcher)
	if err != nil {
		return model.ProjectFile{}, nil, nil, nil, nil, err
	}
	if err := reportProgress(ctx, progress, Progress{Stage: "catalog.read", FilesTotal: len(candidates), Force: true}); err != nil {
		return model.ProjectFile{}, nil, nil, nil, nil, err
	}

	files := make([]model.FileRecord, 0, len(candidates))
	symbols := make([]model.SymbolRecord, 0, len(candidates)*2)
	warnings := make([]string, 0)
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return model.ProjectFile{}, nil, nil, nil, nil, err
		}
		relativePath := catalogProgressPath(root, candidate)
		if err := reportProgress(ctx, progress, Progress{Stage: "catalog.read", Path: relativePath, Files: len(files), Symbols: len(symbols), FilesTotal: len(candidates)}); err != nil {
			return model.ProjectFile{}, nil, nil, nil, nil, err
		}
		content, err := os.ReadFile(candidate)
		if err != nil {
			return model.ProjectFile{}, nil, nil, nil, nil, err
		}
		repo, ok := workspace.FindRepoByFile(projectFile, root, candidate)
		if !ok {
			warnings = appendIfMissing(warnings, fmt.Sprintf("catalog file '%s' did not match any configured repo; treating it as workspace root content", candidate))
			repo, _ = workspace.FindRepo(projectFile, projectFile.Project.DefaultRepo)
		}
		extractedSymbols, fileRecord := ExtractCatalog(root, repo, candidate, content)
		files = append(files, fileRecord)
		symbols = append(symbols, extractedSymbols...)
	}
	if err := reportProgress(ctx, progress, Progress{Stage: "catalog.read", Files: len(files), Symbols: len(symbols), FilesTotal: len(candidates), Force: true}); err != nil {
		return model.ProjectFile{}, nil, nil, nil, nil, err
	}
	return projectFile, files, symbols, warnings, matcher, nil
}

func IndexWorkspaceDocsOnly(ctx context.Context, root string) (Result, error) {
	return IndexWorkspaceDocsOnlyWithGeneration(ctx, root, "")
}

func IndexWorkspaceDocsOnlyWithGeneration(ctx context.Context, root string, generationID string) (Result, error) {
	return IndexWorkspaceDocsOnlyWithProgress(ctx, root, generationID, nil)
}

func IndexWorkspaceDocsOnlyWithProgress(ctx context.Context, root string, generationID string, progress ProgressFunc) (Result, error) {
	return indexWorkspaceDocsOnlyWithProgress(ctx, root, generationID, progress, nil)
}

// IndexWorkspaceDocsOnlyWithProgressForJob is the owner-bound docs publisher.
func IndexWorkspaceDocsOnlyWithProgressForJob(ctx context.Context, root string, generationID, jobID string, fence store.IndexJobFence, progress ProgressFunc) (Result, error) {
	return indexWorkspaceDocsOnlyWithProgress(ctx, root, generationID, progress, &IndexJobPublication{JobID: jobID, Fence: fence})
}

func indexWorkspaceDocsOnlyWithProgress(ctx context.Context, root string, generationID string, progress ProgressFunc, publication *IndexJobPublication) (Result, error) {
	started := time.Now()
	if err := reportProgress(ctx, progress, Progress{Stage: "docs.detect", Force: true}); err != nil {
		return Result{}, err
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Result{}, err
	}
	projectFile, err := docsOnlyProjectFile(root)
	if err != nil {
		return Result{}, err
	}
	matcher, err := workspace.LoadIgnoreMatcher(root, projectFile.Ignore.ExtraPatterns)
	if err != nil {
		return Result{}, err
	}

	priorDocs := loadPriorDocSnapshot(ctx, root)
	docs, docEdges, docMentions, sourceBlocks, sourceRecords, warnings, err := docgraph.IndexWorkspaceDocsWithSourcesWithProgressPrior(ctx, root, matcher, func(ctx context.Context, progressValue docgraph.Progress) error {
		return reportProgress(ctx, progress, Progress{
			Stage:      progressValue.Stage,
			Path:       progressValue.Path,
			Docs:       progressValue.Docs,
			FilesTotal: progressValue.FilesTotal,
			Force:      progressValue.Force,
		})
	}, priorDocs)
	if err != nil {
		return Result{}, err
	}
	snapshot := reentry.BuildSnapshot(root, docs, time.Now())
	if err := reportProgress(ctx, progress, Progress{Stage: "publishing", Docs: len(docs), FilesTotal: len(docs), Force: true}); err != nil {
		return Result{}, err
	}

	if err := store.WithWorkspaceWriteLock(root, func() error {
		db, err := store.Open(root)
		if err != nil {
			return err
		}
		defer db.Close()

		if publication != nil {
			return store.ReplaceWorkspaceDocsForJob(ctx, db, publication.JobID, generationID, docs, docEdges, docMentions, sourceBlocks, sourceRecords, snapshot, publication.Fence)
		}
		return store.ReplaceWorkspaceDocs(ctx, db, generationID, docs, docEdges, docMentions, sourceBlocks, sourceRecords, snapshot)
	}); err != nil {
		return Result{}, err
	}

	warnings = appendIfMissing(warnings, "docs_only=true")
	return Result{
		Warnings: warnings,
		Docs:     len(docs),
		Stats: model.Stats{
			Files: len(docs),
			Ms:    time.Since(started).Milliseconds(),
		},
	}, nil
}

func docsOnlyProjectFile(root string) (model.ProjectFile, error) {
	registration := model.WorkspaceRegistration{
		Name: filepath.Base(root),
		Root: root,
		Kind: model.WorkspaceKindSingle,
	}
	return workspace.LoadProjectSummary(root, registration)
}

// loadPriorDocSnapshot best-effort loads the last published docs index for path-level skip-reparse.
// Missing/empty indexes return nil (full parse path).
func loadPriorDocSnapshot(ctx context.Context, root string) *docgraph.PriorDocSnapshot {
	db, err := store.Open(root)
	if err != nil {
		return nil
	}
	defer db.Close()

	docs, err := store.ListDocRecords(ctx, db)
	if err != nil || len(docs) == 0 {
		return nil
	}
	edges, err := store.ListDocEdges(ctx, db)
	if err != nil {
		return nil
	}
	mentions, err := store.ListDocMentions(ctx, db)
	if err != nil {
		return nil
	}
	blocks, err := store.ListDocSourceBlocks(ctx, db)
	if err != nil {
		return nil
	}
	records, err := store.ListDocSourceRecords(ctx, db)
	if err != nil {
		return nil
	}
	return docgraph.BuildPriorDocSnapshot(docs, edges, mentions, blocks, records)
}

func reportProgress(ctx context.Context, progress ProgressFunc, value Progress) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if progress == nil {
		return nil
	}
	return progress(ctx, value)
}

func catalogProgressPath(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func appendIfMissing(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}
