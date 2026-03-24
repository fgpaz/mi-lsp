package indexer

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

type Result struct {
	Files    []model.FileRecord
	Symbols  []model.SymbolRecord
	Stats    model.Stats
	Warnings []string
}

func IndexWorkspace(ctx context.Context, root string, clean bool) (Result, error) {
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
	if clean {
		if err := store.Reset(root); err != nil {
			return Result{}, err
		}
	}
	candidates, err := WalkWorkspace(root, matcher)
	if err != nil {
		return Result{}, err
	}

	files := make([]model.FileRecord, 0, len(candidates))
	symbols := make([]model.SymbolRecord, 0, len(candidates)*2)
	warnings := make([]string, 0)
	for _, candidate := range candidates {
		content, err := os.ReadFile(candidate)
		if err != nil {
			return Result{}, err
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

	db, err := store.Open(root)
	if err != nil {
		return Result{}, err
	}
	defer db.Close()

	if err := store.ReplaceCatalog(ctx, db, projectFile, files, symbols); err != nil {
		return Result{}, err
	}

	return Result{
		Files:    files,
		Symbols:  symbols,
		Warnings: warnings,
		Stats: model.Stats{
			Files:   len(files),
			Symbols: len(symbols),
			Ms:      time.Since(started).Milliseconds(),
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
