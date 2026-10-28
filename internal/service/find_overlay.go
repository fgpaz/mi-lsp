package service

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// findSymbolsWithWorktreeOverlay queries the registered common-dir base index,
// masks changed/deleted paths, and extracts symbols only from changed physical
// worktree files. It never creates or refreshes an index for the worktree.
func findSymbolsWithWorktreeOverlay(ctx context.Context, db *sql.DB, registration model.WorkspaceRegistration, project model.ProjectFile, overlay workspace.WorktreeOverlay, pattern, kind string, exact bool, limit, offset int, repo *model.WorkspaceRepo) ([]model.SymbolRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	excludedSet := make(map[string]struct{}, len(overlay.Changed)+len(overlay.Deleted))
	for _, p := range overlay.Changed {
		excludedSet[filepath.ToSlash(p)] = struct{}{}
	}
	for _, p := range overlay.Deleted {
		excludedSet[filepath.ToSlash(p)] = struct{}{}
	}
	excluded := make([]string, 0, len(excludedSet))
	for p := range excludedSet {
		excluded = append(excluded, p)
	}
	sort.Strings(excluded)

	rootReal, err := filepath.EvalSymlinks(overlay.Root)
	if err != nil {
		return nil, err
	}
	delta := make([]model.SymbolRecord, 0)
	for _, rel := range overlay.Changed {
		if _, deleted := containsPath(overlay.Deleted, rel); deleted {
			continue
		}
		abs := filepath.Join(overlay.Root, filepath.FromSlash(rel))
		real, evalErr := filepath.EvalSymlinks(abs)
		if evalErr != nil {
			continue
		}
		relReal, relErr := filepath.Rel(rootReal, real)
		if relErr != nil || relReal == ".." || strings.HasPrefix(relReal, ".."+string(filepath.Separator)) || filepath.IsAbs(relReal) {
			continue
		}
		info, statErr := os.Stat(real)
		if statErr != nil || !info.Mode().IsRegular() || info.Size() > 4*1024*1024 {
			continue
		}
		language := filepath.Ext(real)
		if language == "" {
			continue
		}
		content, readErr := os.ReadFile(real)
		if readErr != nil {
			continue
		}
		fileRepo := model.WorkspaceRepo{ID: registration.Name, Name: registration.Name}
		if found, ok := workspace.FindRepoByFile(project, overlay.Root, abs); ok {
			fileRepo = found
		}
		symbols, _ := indexer.ExtractCatalog(overlay.Root, fileRepo, abs, content)
		for _, symbol := range symbols {
			if exact && symbol.Name != pattern {
				continue
			}
			if !exact && !sqlLikeContains(symbol.Name, pattern) {
				continue
			}
			if kind != "" && symbol.Kind != kind {
				continue
			}
			symbol.Workspace = registration.Name
			delta = append(delta, symbol)
		}
	}
	baseLimit := limit + offset + len(delta)
	if repo != nil {
		baseLimit = max(baseLimit*10, 100)
	}
	base, err := store.FindSymbolsExcludingPaths(ctx, db, pattern, kind, exact, baseLimit, 0, excluded)
	if err != nil {
		return nil, err
	}
	for i := range base {
		base[i].Workspace = registration.Name
	}
	all := append(base, delta...)
	if repo != nil {
		all = filterSymbolsByRepo(all, repo)
	}
	sort.Slice(all, func(i, j int) bool { return symbolOverlayLess(all[i], all[j]) })
	if offset >= len(all) {
		return []model.SymbolRecord{}, nil
	}
	all = all[offset:]
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func worktreeOverlayContains(paths []string, rel string) bool {
	rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	for _, candidate := range paths {
		if filepath.ToSlash(filepath.Clean(filepath.FromSlash(candidate))) == rel {
			return true
		}
	}
	return false
}

func sqlLikeContains(value, pattern string) bool {
	value = strings.ToLower(value)
	pattern = strings.ToLower(pattern)
	if !strings.ContainsAny(pattern, "%_") {
		return strings.Contains(value, pattern)
	}
	var b strings.Builder
	b.WriteString("(?s).*")
	for _, r := range pattern {
		switch r {
		case '%':
			b.WriteString(".*")
		case '_':
			b.WriteByte('.')
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString(".*")
	matched, _ := regexp.MatchString(b.String(), value)
	return matched
}

func symbolOverlayLess(a, b model.SymbolRecord) bool {
	for _, pair := range [][2]string{{a.Name, b.Name}, {a.FilePath, b.FilePath}, {a.RepoID, b.RepoID}, {a.RepoName, b.RepoName}, {a.Kind, b.Kind}} {
		if pair[0] != pair[1] {
			return pair[0] < pair[1]
		}
	}
	for _, pair := range [][2]int{{a.StartLine, b.StartLine}, {a.EndLine, b.EndLine}} {
		if pair[0] != pair[1] {
			return pair[0] < pair[1]
		}
	}
	for _, pair := range [][2]string{{a.Parent, b.Parent}, {a.QualifiedName, b.QualifiedName}, {a.Signature, b.Signature}, {a.SignatureHash, b.SignatureHash}, {a.Scope, b.Scope}, {a.Language, b.Language}, {a.FileHash, b.FileHash}, {a.Implements, b.Implements}} {
		if pair[0] != pair[1] {
			return pair[0] < pair[1]
		}
	}
	return a.ID < b.ID
}
