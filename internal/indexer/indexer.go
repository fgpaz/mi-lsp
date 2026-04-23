package indexer

import (
	"context"
	"fmt"
	"os"
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

func IndexWorkspace(ctx context.Context, root string, clean bool) (Result, error) {
	return IndexWorkspaceWithGeneration(ctx, root, clean, "")
}

func IndexWorkspaceWithGeneration(ctx context.Context, root string, clean bool, generationID string) (Result, error) {
	started := time.Now()
	projectFile, files, symbols, warnings, matcher, err := buildCatalog(ctx, root)
	if err != nil {
		return Result{}, err
	}
	if clean {
		warnings = appendIfMissing(warnings, "clean=true")
	}

	docs, docEdges, docMentions, docWarnings, err := docgraph.IndexWorkspaceDocs(ctx, root, matcher)
	if err != nil {
		return Result{}, err
	}
	warnings = append(warnings, docWarnings...)
	snapshot := reentry.BuildSnapshot(root, docs, time.Now())

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
	started := time.Now()
	projectFile, files, symbols, warnings, _, err := buildCatalog(ctx, root)
	if err != nil {
		return Result{}, err
	}
	if clean {
		warnings = appendIfMissing(warnings, "clean=true")
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

func buildCatalog(ctx context.Context, root string) (model.ProjectFile, []model.FileRecord, []model.SymbolRecord, []string, *workspace.IgnoreMatcher, error) {
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
	candidates, err := WalkWorkspace(ctx, root, matcher)
	if err != nil {
		return model.ProjectFile{}, nil, nil, nil, nil, err
	}

	files := make([]model.FileRecord, 0, len(candidates))
	symbols := make([]model.SymbolRecord, 0, len(candidates)*2)
	warnings := make([]string, 0)
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
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
	return projectFile, files, symbols, warnings, matcher, nil
}

func IndexWorkspaceDocsOnly(ctx context.Context, root string) (Result, error) {
	return IndexWorkspaceDocsOnlyWithGeneration(ctx, root, "")
}

func IndexWorkspaceDocsOnlyWithGeneration(ctx context.Context, root string, generationID string) (Result, error) {
	started := time.Now()
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

	docs, docEdges, docMentions, warnings, err := docgraph.IndexWorkspaceDocs(ctx, root, matcher)
	if err != nil {
		return Result{}, err
	}
	snapshot := reentry.BuildSnapshot(root, docs, time.Now())

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

func appendIfMissing(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}
