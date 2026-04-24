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
	Files    []model.FileRecord
	Symbols  []model.SymbolRecord
	Docs     int
	Stats    model.Stats
	Warnings []string
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

func IndexWorkspace(ctx context.Context, root string, clean bool) (Result, error) {
	return IndexWorkspaceWithGeneration(ctx, root, clean, "")
}

func IndexWorkspaceWithGeneration(ctx context.Context, root string, clean bool, generationID string) (Result, error) {
	return IndexWorkspaceWithProgress(ctx, root, clean, generationID, nil)
}

func IndexWorkspaceWithProgress(ctx context.Context, root string, clean bool, generationID string, progress ProgressFunc) (Result, error) {
	started := time.Now()
	projectFile, files, symbols, warnings, matcher, err := buildCatalog(ctx, root, progress)
	if err != nil {
		return Result{}, err
	}
	if clean {
		warnings = appendIfMissing(warnings, "clean=true")
	}

	docs, docEdges, docMentions, docWarnings, err := docgraph.IndexWorkspaceDocsWithProgress(ctx, root, matcher, func(ctx context.Context, progressValue docgraph.Progress) error {
		return reportProgress(ctx, progress, Progress{
			Stage:      progressValue.Stage,
			Path:       progressValue.Path,
			Files:      len(files),
			Symbols:    len(symbols),
			Docs:       progressValue.Docs,
			FilesTotal: progressValue.FilesTotal,
			Force:      progressValue.Force,
		})
	})
	if err != nil {
		return Result{}, err
	}
	warnings = append(warnings, docWarnings...)
	snapshot := reentry.BuildSnapshot(root, docs, time.Now())
	if err := reportProgress(ctx, progress, Progress{Stage: "publishing", Files: len(files), Symbols: len(symbols), Docs: len(docs), FilesTotal: len(files), Force: true}); err != nil {
		return Result{}, err
	}

	if err := store.WithWorkspaceWriteLock(root, func() error {
		db, err := store.Open(root)
		if err != nil {
			return err
		}
		defer db.Close()

		return store.ReplaceWorkspaceIndex(ctx, db, generationID, projectFile, files, symbols, docs, docEdges, docMentions, snapshot)
	}); err != nil {
		return Result{}, err
	}

	return Result{
		Files:    files,
		Symbols:  symbols,
		Docs:     len(docs),
		Warnings: warnings,
		Stats: model.Stats{
			Files:   len(files),
			Symbols: len(symbols),
			Ms:      time.Since(started).Milliseconds(),
		},
	}, nil
}

func IndexWorkspaceCatalogOnly(ctx context.Context, root string, clean bool) (Result, error) {
	return IndexWorkspaceCatalogOnlyWithGeneration(ctx, root, clean, "")
}

func IndexWorkspaceCatalogOnlyWithGeneration(ctx context.Context, root string, clean bool, generationID string) (Result, error) {
	return IndexWorkspaceCatalogOnlyWithProgress(ctx, root, clean, generationID, nil)
}

func IndexWorkspaceCatalogOnlyWithProgress(ctx context.Context, root string, clean bool, generationID string, progress ProgressFunc) (Result, error) {
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
	started := time.Now()
	if err := reportProgress(ctx, progress, Progress{Stage: "docs.detect", Force: true}); err != nil {
		return Result{}, err
	}
	registration, err := workspace.DetectWorkspace(root)
	if err != nil {
		return Result{}, err
	}
	projectFile, err := workspace.LoadProjectTopology(root, registration)
	if err != nil {
		return Result{}, err
	}
	matcher, err := workspace.LoadIgnoreMatcher(root, projectFile.Ignore.ExtraPatterns)
	if err != nil {
		return Result{}, err
	}

	docs, docEdges, docMentions, warnings, err := docgraph.IndexWorkspaceDocsWithProgress(ctx, root, matcher, func(ctx context.Context, progressValue docgraph.Progress) error {
		return reportProgress(ctx, progress, Progress{
			Stage:      progressValue.Stage,
			Path:       progressValue.Path,
			Docs:       progressValue.Docs,
			FilesTotal: progressValue.FilesTotal,
			Force:      progressValue.Force,
		})
	})
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

		return store.ReplaceWorkspaceDocs(ctx, db, generationID, docs, docEdges, docMentions, snapshot)
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
