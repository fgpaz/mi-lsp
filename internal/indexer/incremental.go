package indexer

import (
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// ExtractFileSymbols reads a file and extracts symbols from it.
// Used by the file watcher for incremental indexing.
func ExtractFileSymbols(workspaceRoot string, filePath string, repoID string, repoName string) ([]model.SymbolRecord, string, error) {
	absPath := filePath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(workspaceRoot, filepath.FromSlash(filePath))
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, "", err
	}

	// Use relative path for storage
	relPath := filePath
	if filepath.IsAbs(relPath) {
		if rel, err := filepath.Rel(workspaceRoot, relPath); err == nil {
			relPath = filepath.ToSlash(rel)
		}
	}

	// Determine language from extension
	ext := strings.ToLower(filepath.Ext(absPath))
	language := languageFromExt(ext)

	// Create a temporary repo object for ExtractCatalog
	repo := model.WorkspaceRepo{
		ID:   repoID,
		Name: repoName,
	}

	// Use the existing ExtractCatalog function
	symbols, _ := ExtractCatalog(workspaceRoot, repo, absPath, content)

	return symbols, language, nil
}

func languageFromExt(ext string) string {
	switch ext {
	case ".cs":
		return "csharp"
	case ".ts", ".tsx", ".mts", ".cts":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".py", ".pyi":
		return "python"
	default:
		return ""
	}
}

// ResolveRepoFromProjectFile finds the repo that owns a file.
// Returns empty strings if not found, to be handled gracefully by the caller.
func ResolveRepoFromProjectFile(workspaceRoot string, projectFile model.ProjectFile, filePath string) (string, string) {
	if repo, ok := workspace.FindRepoByFile(projectFile, workspaceRoot, filePath); ok {
		return repo.ID, repo.Name
	}
	// Fallback to default repo
	if repo, ok := workspace.FindRepo(projectFile, projectFile.Project.DefaultRepo); ok {
		return repo.ID, repo.Name
	}
	return "", ""
}

// gitChangedFiles returns lists of changed and deleted files from git status.
// Returns relative paths normalized to forward slashes.
// If git is unavailable or returns an error, both slices will be empty (not an error).
func gitChangedFiles(ctx context.Context, workspaceRoot string) (changed []string, deleted []string) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = workspaceRoot
	output, err := cmd.Output()
	if err != nil {
		// Git not available or error — return empty lists
		return []string{}, []string{}
	}

	lines := strings.Split(string(output), "\n")
	changed = make([]string, 0)
	deleted = make([]string, 0)

	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		status := line[:2]
		filePath := strings.TrimSpace(line[2:])
		if filePath == "" {
			continue
		}
		filePath = filepath.ToSlash(filePath)

		// Parse git status codes
		// M = modified, A = added, ? = untracked, D = deleted
		switch status {
		case " M", "M ", "MM":
			// Modified (staged or unstaged)
			changed = append(changed, filePath)
		case "A ", " A", "AA":
			// Added (staged or unstaged)
			changed = append(changed, filePath)
		case "??":
			// Untracked
			changed = append(changed, filePath)
		case " D", "D ", "DD":
			// Deleted
			deleted = append(deleted, filePath)
		}
	}

	return changed, deleted
}

// IncrementalIndex performs incremental indexing using git status.
// If git is unavailable or index.db doesn't exist, it returns empty stats (fallback expected).
func IncrementalIndex(ctx context.Context, workspaceRoot string) (Result, error) {
	started := time.Now()

	// Check if index.db exists
	indexPath := filepath.Join(workspaceRoot, ".mi-lsp", "index.db")
	if _, err := os.Stat(indexPath); err != nil {
		return Result{}, fmt.Errorf("index.db not found; fallback to full index")
	}

	// Get git changed files
	changedFiles, deletedFiles := gitChangedFiles(ctx, workspaceRoot)
	if len(changedFiles) == 0 && len(deletedFiles) == 0 {
		// No changes detected
		return Result{
			Files:    []model.FileRecord{},
			Symbols:  []model.SymbolRecord{},
			Warnings: []string{},
			Stats: model.Stats{
				Files:   0,
				Symbols: 0,
				Ms:      time.Since(started).Milliseconds(),
			},
		}, nil
	}

	// Load workspace and project
	registration, err := workspace.DetectWorkspace(workspaceRoot)
	if err != nil {
		return Result{}, fmt.Errorf("detect workspace: %w", err)
	}
	projectFile, err := workspace.LoadProjectTopology(workspaceRoot, registration)
	if err != nil {
		return Result{}, fmt.Errorf("load project: %w", err)
	}
	matcher, err := workspace.LoadIgnoreMatcher(workspaceRoot, projectFile.Ignore.ExtraPatterns)
	if err != nil {
		return Result{}, fmt.Errorf("load ignore matcher: %w", err)
	}

	// Open database
	db, err := store.Open(workspaceRoot)
	if err != nil {
		return Result{}, fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Process changed files
	processedFiles := 0
	skippedFiles := 0
	var allSymbols []model.SymbolRecord

	for _, relPath := range changedFiles {
		absPath := filepath.Join(workspaceRoot, filepath.FromSlash(relPath))

		// Check if file should be ignored
		if matcher.ShouldIgnore(workspaceRoot, absPath) {
			skippedFiles++
			continue
		}

		// Check if file still exists (might have been deleted externally)
		content, err := os.ReadFile(absPath)
		if err != nil {
			// File no longer exists; treat as deleted
			if err := store.DeleteFileSymbols(ctx, db, relPath); err != nil {
				return Result{}, fmt.Errorf("delete symbols for %s: %w", relPath, err)
			}
			processedFiles++
			continue
		}

		// Extract symbols
		symbols, language, err := ExtractFileSymbols(workspaceRoot, relPath, "", "")
		if err != nil {
			return Result{}, fmt.Errorf("extract symbols for %s: %w", relPath, err)
		}

		// Resolve repo
		repoID, repoName := ResolveRepoFromProjectFile(workspaceRoot, projectFile, relPath)

		// Compute content hash
		contentHash := fmt.Sprintf("%x", md5.Sum(content))

		// Replace file symbols
		if err := store.ReplaceFileSymbols(ctx, db, relPath, repoID, repoName, language, contentHash, symbols); err != nil {
			return Result{}, fmt.Errorf("replace symbols for %s: %w", relPath, err)
		}

		allSymbols = append(allSymbols, symbols...)
		processedFiles++
	}

	// Process deleted files
	for _, relPath := range deletedFiles {
		if err := store.DeleteFileSymbols(ctx, db, relPath); err != nil {
			return Result{}, fmt.Errorf("delete symbols for %s: %w", relPath, err)
		}
		processedFiles++
	}

	return Result{
		Files:   []model.FileRecord{},
		Symbols: allSymbols,
		Warnings: []string{
			fmt.Sprintf("incremental: processed %d files, skipped %d", processedFiles, skippedFiles),
		},
		Stats: model.Stats{
			Files:   processedFiles,
			Symbols: len(allSymbols),
			Ms:      time.Since(started).Milliseconds(),
		},
	}, nil
}
