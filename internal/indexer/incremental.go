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
	"github.com/fgpaz/mi-lsp/internal/processutil"
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

	relPath := filePath
	if filepath.IsAbs(relPath) {
		if rel, err := filepath.Rel(workspaceRoot, relPath); err == nil {
			relPath = filepath.ToSlash(rel)
		}
	}

	ext := strings.ToLower(filepath.Ext(absPath))
	language := languageFromExt(ext)
	repo := model.WorkspaceRepo{ID: repoID, Name: repoName}
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

func ResolveRepoFromProjectFile(workspaceRoot string, projectFile model.ProjectFile, filePath string) (string, string) {
	if repo, ok := workspace.FindRepoByFile(projectFile, workspaceRoot, filePath); ok {
		return repo.ID, repo.Name
	}
	if repo, ok := workspace.FindRepo(projectFile, projectFile.Project.DefaultRepo); ok {
		return repo.ID, repo.Name
	}
	return "", ""
}

func gitChangedFiles(ctx context.Context, workspaceRoot string) (changed []string, deleted []string) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	processutil.ConfigureNonInteractiveCommand(cmd)
	cmd.Dir = workspaceRoot
	output, err := cmd.Output()
	if err != nil {
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

		switch status {
		case " M", "M ", "MM", "A ", " A", "AA", "??":
			changed = append(changed, filePath)
		case " D", "D ", "DD":
			deleted = append(deleted, filePath)
		}
	}

	return changed, deleted
}

func IncrementalIndex(ctx context.Context, workspaceRoot string) (Result, error) {
	started := time.Now()
	indexPath := filepath.Join(workspaceRoot, ".mi-lsp", "index.db")
	if _, err := os.Stat(indexPath); err != nil {
		return Result{}, fmt.Errorf("index.db not found; fallback to full index")
	}
	needsRecovery, err := docIndexNeedsRecovery(ctx, workspaceRoot)
	if err != nil {
		return Result{}, err
	}
	if needsRecovery {
		return Result{}, fmt.Errorf("canonical docs missing from index; fallback to full index")
	}

	changedFiles, deletedFiles := gitChangedFiles(ctx, workspaceRoot)
	if len(changedFiles) == 0 && len(deletedFiles) == 0 {
		return Result{Files: []model.FileRecord{}, Symbols: []model.SymbolRecord{}, Warnings: []string{}, Stats: model.Stats{Files: 0, Symbols: 0, Ms: time.Since(started).Milliseconds()}}, nil
	}
	if requiresFullReindex(changedFiles) || requiresFullReindex(deletedFiles) {
		return Result{}, fmt.Errorf("documentation or read-model changed; fallback to full index")
	}

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

	processedFiles := 0
	skippedFiles := 0
	var allSymbols []model.SymbolRecord
	if err := store.WithWorkspaceWriteLock(workspaceRoot, func() error {
		db, err := store.Open(workspaceRoot)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		for _, relPath := range changedFiles {
			absPath := filepath.Join(workspaceRoot, filepath.FromSlash(relPath))
			if matcher.ShouldIgnore(workspaceRoot, absPath) {
				skippedFiles++
				continue
			}
			if languageFromExt(strings.ToLower(filepath.Ext(relPath))) == "" {
				skippedFiles++
				continue
			}
			content, err := os.ReadFile(absPath)
			if err != nil {
				if err := store.DeleteFileSymbols(ctx, db, relPath); err != nil {
					return fmt.Errorf("delete symbols for %s: %w", relPath, err)
				}
				processedFiles++
				continue
			}
			symbols, language, err := ExtractFileSymbols(workspaceRoot, relPath, "", "")
			if err != nil {
				return fmt.Errorf("extract symbols for %s: %w", relPath, err)
			}
			repoID, repoName := ResolveRepoFromProjectFile(workspaceRoot, projectFile, relPath)
			contentHash := fmt.Sprintf("%x", md5.Sum(content))
			if err := store.ReplaceFileSymbols(ctx, db, relPath, repoID, repoName, language, contentHash, symbols); err != nil {
				return fmt.Errorf("replace symbols for %s: %w", relPath, err)
			}
			allSymbols = append(allSymbols, symbols...)
			processedFiles++
		}

		for _, relPath := range deletedFiles {
			if languageFromExt(strings.ToLower(filepath.Ext(relPath))) == "" {
				continue
			}
			if err := store.DeleteFileSymbols(ctx, db, relPath); err != nil {
				return fmt.Errorf("delete symbols for %s: %w", relPath, err)
			}
			processedFiles++
		}
		return nil
	}); err != nil {
		return Result{}, err
	}

	return Result{
		Files:    []model.FileRecord{},
		Symbols:  allSymbols,
		Warnings: []string{fmt.Sprintf("incremental: processed %d files, skipped %d", processedFiles, skippedFiles)},
		Stats:    model.Stats{Files: processedFiles, Symbols: len(allSymbols), Ms: time.Since(started).Milliseconds()},
	}, nil
}

func requiresFullReindex(paths []string) bool {
	for _, path := range paths {
		normalized := filepath.ToSlash(strings.ToLower(path))
		base := filepath.Base(normalized)
		if strings.HasPrefix(normalized, ".docs/") || strings.HasPrefix(normalized, "docs/") {
			return true
		}
		if strings.HasPrefix(normalized, "readme") && strings.HasSuffix(normalized, ".md") {
			return true
		}
		if base == "read-model.toml" && strings.Contains(normalized, ".docs/wiki/_mi-lsp/") {
			return true
		}
	}
	return false
}

func docIndexNeedsRecovery(ctx context.Context, workspaceRoot string) (bool, error) {
	if !canonicalDocsExistOnDisk(workspaceRoot) {
		return false, nil
	}

	db, err := store.Open(workspaceRoot)
	if err != nil {
		return false, err
	}
	defer db.Close()

	docs, err := store.ListDocRecords(ctx, db)
	if err != nil {
		return false, err
	}
	if len(docs) == 0 {
		return true, nil
	}
	for _, doc := range docs {
		if doc.IsSnapshot {
			continue
		}
		if doc.Family != "" && doc.Family != "generic" {
			return false, nil
		}
	}
	return true, nil
}

func canonicalDocsExistOnDisk(workspaceRoot string) bool {
	for _, relativePath := range []string{
		".docs/wiki/00_gobierno_documental.md",
		".docs/wiki/_mi-lsp/read-model.toml",
		".docs/wiki/03_FL.md",
		".docs/wiki/04_RF.md",
		".docs/wiki/07_baseline_tecnica.md",
		".docs/wiki/09_contratos_tecnicos.md",
	} {
		if _, err := os.Stat(filepath.Join(workspaceRoot, filepath.FromSlash(relativePath))); err == nil {
			return true
		}
	}
	return false
}
