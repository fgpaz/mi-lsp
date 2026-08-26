package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// ReconcileDocsResult holds the classification outcomes for all scanned docs.
type ReconcileDocsResult struct {
	AuthorityConfigHash string
	Warnings            []string
	StoredStates        map[string]model.DocArtifactState
	New                 []string
	Changed             []string
	Unchanged           []string
	Deleted             []string
	ExcludedAlive       []string
	Concurrent          []string
}

type canonicalDocFile struct {
	path         string
	relativePath string
}

// ReconcileDocs compares the canonical document profile against stored
// artifact states. A file missing from a scan is deletion only when disk
// absence is positively proven; an alive excluded file is retained and warned.
func ReconcileDocs(ctx context.Context, root string) (*ReconcileDocsResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	registration, err := workspace.DetectWorkspace(root)
	if err != nil {
		return nil, fmt.Errorf("detect workspace: %w", err)
	}
	projectFile, err := workspace.LoadProjectTopology(root, registration)
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}
	profile, _, profileWarnings := docgraph.LoadProfile(root)
	matcher, err := workspace.LoadIgnoreMatcher(root, projectFile.Ignore.ExtraPatterns)
	if err != nil {
		return nil, fmt.Errorf("load ignore matcher: %w", err)
	}

	db, err := store.Open(root)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	states, err := store.ListDocArtifactStates(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("list artifact states: %w", err)
	}
	storedMap := make(map[string]model.DocArtifactState, len(states))
	for _, state := range states {
		storedMap[state.Path] = state
	}

	result := &ReconcileDocsResult{
		AuthorityConfigHash: store.AuthorityConfigDigestForProfile(profile, nil),
		Warnings:            append([]string(nil), profileWarnings...),
		StoredStates:        storedMap,
	}
	files, excluded, err := collectCanonicalDocFiles(ctx, root, profile, matcher)
	if err != nil {
		return nil, fmt.Errorf("scan canonical docs: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result.ExcludedAlive = append(result.ExcludedAlive, excluded...)
	scanPaths := make(map[string]struct{}, len(files))

	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		relPath := file.relativePath
		scanPaths[relPath] = struct{}{}
		stored, hasStored := storedMap[relPath]
		info, statErr := os.Lstat(file.path)
		if statErr != nil {
			if hasStored && store.IsDiskAbsent(file.path) {
				result.Deleted = append(result.Deleted, relPath)
			} else if hasStored {
				result.Warnings = append(result.Warnings, fmt.Sprintf("doc stat failed for %s: %v", relPath, statErr))
			}
			continue
		}
		if info.IsDir() {
			continue
		}
		if !hasStored {
			result.New = append(result.New, relPath)
			continue
		}

		// Parser/profile changes require a stable body read during build even
		// when filesystem metadata is unchanged.
		if stored.ParserVersion != model.ParserVersion || stored.AuthorityConfigHash != result.AuthorityConfigHash {
			result.Changed = append(result.Changed, relPath)
			continue
		}
		hash, reused, cleanErr := store.RacilyClean(ctx, file.path, func() ([]byte, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return os.ReadFile(file.path)
		}, stored)
		if cleanErr != nil {
			if _, ok := cleanErr.(*model.ErrConcurrentChange); ok {
				result.Concurrent = append(result.Concurrent, relPath)
				result.Warnings = append(result.Warnings, fmt.Sprintf("doc concurrent change: %s", relPath))
			} else {
				result.Warnings = append(result.Warnings, fmt.Sprintf("doc read failed for %s: %v", relPath, cleanErr))
			}
			continue
		}
		if reused {
			result.Unchanged = append(result.Unchanged, relPath)
		} else if hash != "" {
			result.Changed = append(result.Changed, relPath)
		}
	}

	for path := range storedMap {
		if _, seen := scanPaths[path]; seen {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		absPath := filepath.Join(root, filepath.FromSlash(path))
		if store.IsDiskAbsent(absPath) {
			result.Deleted = append(result.Deleted, path)
		} else {
			result.ExcludedAlive = append(result.ExcludedAlive, path)
		}
	}
	sortStringSet(&result.New)
	sortStringSet(&result.Changed)
	sortStringSet(&result.Unchanged)
	sortStringSet(&result.Deleted)
	sortStringSet(&result.ExcludedAlive)
	sortStringSet(&result.Concurrent)
	return result, nil
}

func sortStringSet(values *[]string) {
	seen := make(map[string]struct{}, len(*values))
	out := make([]string, 0, len(*values))
	for _, value := range *values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	*values = out
}

func collectCanonicalDocFiles(ctx context.Context, root string, profile model.DocsReadProfile, matcher *workspace.IgnoreMatcher) ([]canonicalDocFile, []string, error) {
	seen := make(map[string]canonicalDocFile)
	excluded := make(map[string]struct{})
	visit := func(path string) {
		if err := ctx.Err(); err != nil {
			return
		}
		if matcher != nil && matcher.ShouldIgnore(root, path) {
			rel, relErr := filepath.Rel(root, path)
			if relErr == nil && isMarkdownPath(path) {
				excluded[filepath.ToSlash(rel)] = struct{}{}
			}
			return
		}
		if !isMarkdownPath(path) {
			return
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return
		}
		rel = filepath.ToSlash(rel)
		seen[rel] = canonicalDocFile{path: path, relativePath: rel}
	}
	for _, family := range profile.Families {
		for _, pattern := range family.Paths {
			if err := visitDocProfilePattern(ctx, root, pattern, matcher, visit); err != nil {
				return nil, nil, err
			}
		}
	}
	for _, pattern := range profile.GenericDocs.Paths {
		if err := visitDocProfilePattern(ctx, root, pattern, matcher, visit); err != nil {
			return nil, nil, err
		}
	}
	files := make([]canonicalDocFile, 0, len(seen))
	for _, file := range seen {
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].relativePath < files[j].relativePath })
	excludedPaths := make([]string, 0, len(excluded))
	for path := range excluded {
		excludedPaths = append(excludedPaths, path)
	}
	sort.Strings(excludedPaths)
	return files, excludedPaths, nil
}

func visitDocProfilePattern(ctx context.Context, root, pattern string, matcher *workspace.IgnoreMatcher, visit func(string)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	trimmed := filepath.ToSlash(strings.TrimSpace(pattern))
	if trimmed == "" {
		return nil
	}
	if strings.HasSuffix(trimmed, "/") {
		dir := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(trimmed, "/")))
		return walkCanonicalDocDir(ctx, root, dir, matcher, visit)
	}
	if strings.ContainsAny(trimmed, "*?[") {
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(trimmed)))
		if err != nil {
			return err
		}
		for _, match := range matches {
			if err := ctx.Err(); err != nil {
				return err
			}
			info, statErr := os.Lstat(match)
			if statErr != nil {
				continue
			}
			if info.IsDir() {
				if err := walkCanonicalDocDir(ctx, root, match, matcher, visit); err != nil {
					return err
				}
			} else {
				visit(match)
			}
		}
		return nil
	}
	path := filepath.Join(root, filepath.FromSlash(trimmed))
	info, err := os.Lstat(path)
	if err != nil {
		return nil
	}
	if info.IsDir() {
		return walkCanonicalDocDir(ctx, root, path, matcher, visit)
	}
	visit(path)
	return nil
}

func walkCanonicalDocDir(ctx context.Context, root, dir string, matcher *workspace.IgnoreMatcher, visit func(string)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if matcher != nil && matcher.ShouldIgnore(root, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		visit(path)
		return nil
	})
}

func isMarkdownPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".mdown", ".markdown":
		return true
	default:
		return false
	}
}

// collectCanonicalRoots returns deterministic literal directory roots useful
// for callers that need a bounded profile walk. Globs are reduced to their
// literal parent directory, while exact files contribute their parent.
func collectCanonicalRoots(root string, profile model.DocsReadProfile) []string {
	seen := make(map[string]struct{})
	var roots []string
	add := func(pattern string) {
		pattern = filepath.ToSlash(strings.TrimSpace(pattern))
		if pattern == "" {
			return
		}
		pattern = strings.TrimSuffix(pattern, "/")
		wild := strings.IndexAny(pattern, "*?[")
		if wild >= 0 {
			pattern = pattern[:wild]
		}
		if pattern == "" {
			pattern = "."
		}
		if wild >= 0 || strings.HasSuffix(pattern, ".md") || strings.HasSuffix(pattern, ".mdown") || strings.HasSuffix(pattern, ".markdown") {
			pattern = filepath.ToSlash(filepath.Dir(filepath.FromSlash(pattern)))
		}
		if pattern == "." || pattern == "" {
			pattern = "."
		}
		abs := filepath.Clean(filepath.Join(root, filepath.FromSlash(pattern)))
		if _, ok := seen[abs]; ok {
			return
		}
		seen[abs] = struct{}{}
		roots = append(roots, abs)
	}
	for _, family := range profile.Families {
		for _, pattern := range family.Paths {
			add(pattern)
		}
	}
	for _, pattern := range profile.GenericDocs.Paths {
		add(pattern)
	}
	sort.Strings(roots)
	return roots
}

// isDocPath returns true when the file path looks like a document. It is used
// to separate document and catalog domains before any graph observation.
func isDocPath(path string) bool {
	normalized := strings.ToLower(filepath.ToSlash(path))
	if strings.HasPrefix(normalized, "wiki/") || strings.HasPrefix(normalized, ".docs/wiki/") {
		return true
	}
	base := strings.ToLower(filepath.Base(path))
	return strings.HasSuffix(normalized, ".md") || strings.HasSuffix(normalized, ".mdown") || strings.HasSuffix(normalized, ".markdown") || (strings.HasPrefix(base, "readme") && strings.HasSuffix(base, ".md"))
}

type docFileStamp struct {
	size      int64
	mtimeNsec int64
}

func statDocument(ctx context.Context, path string) (docFileStamp, error) {
	if err := ctx.Err(); err != nil {
		return docFileStamp{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return docFileStamp{}, err
	}
	return docFileStamp{size: info.Size(), mtimeNsec: info.ModTime().UnixNano()}, nil
}

func readStableDocument(ctx context.Context, path string) ([]byte, docFileStamp, error) {
	for attempt := 1; attempt <= 2; attempt++ {
		before, err := statDocument(ctx, path)
		if err != nil {
			return nil, docFileStamp{}, err
		}
		if err := ctx.Err(); err != nil {
			return nil, docFileStamp{}, err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, docFileStamp{}, err
		}
		if err := ctx.Err(); err != nil {
			return nil, docFileStamp{}, err
		}
		after, err := statDocument(ctx, path)
		if err != nil {
			return nil, docFileStamp{}, err
		}
		if before == after {
			return content, after, nil
		}
		if attempt == 2 {
			return nil, docFileStamp{}, &model.ErrConcurrentChange{Path: path, CurrentMtimeNsec: after.mtimeNsec, CurrentSize: after.size, Attempt: attempt}
		}
		if err := ctx.Err(); err != nil {
			return nil, docFileStamp{}, err
		}
	}
	return nil, docFileStamp{}, fmt.Errorf("stable document read failed for %s", path)
}

// buildDocChanges is retained as a small compatibility wrapper for package
// tests. Production callers use the context-aware variant below.
func buildDocChanges(root string, paths []string, parserVersion, configHash string) []store.IncrementalDocChange {
	changes, _ := buildDocChangesContext(context.Background(), root, paths, parserVersion, configHash, nil)
	return changes
}

func lifecycleFromBindings(bindings []model.DocArtifactBinding, fallback string) string {
	lifecycle := fallback
	if lifecycle == "" {
		lifecycle = model.DocLifecycleActive
	}
	for _, binding := range bindings {
		switch binding.DocLifecycle {
		case model.DocLifecycleRetired:
			return model.DocLifecycleRetired
		case model.DocLifecycleDeprecated:
			if lifecycle == model.DocLifecycleActive {
				lifecycle = model.DocLifecycleDeprecated
			}
		}
	}
	return lifecycle
}

func stableContentSHA256(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

func buildDocChangesContext(ctx context.Context, root string, paths []string, parserVersion, configHash string, storedStates map[string]model.DocArtifactState) ([]store.IncrementalDocChange, []string) {
	if parserVersion == "" {
		parserVersion = model.ParserVersion
	}
	sortedPaths := append([]string(nil), paths...)
	sortStringSet(&sortedPaths)
	changes := make([]store.IncrementalDocChange, 0, len(sortedPaths))
	warnings := make([]string, 0)
	for _, path := range sortedPaths {
		if err := ctx.Err(); err != nil {
			return changes, append(warnings, err.Error())
		}
		absPath := filepath.Join(root, filepath.FromSlash(path))
		content, stamp, err := readStableDocument(ctx, absPath)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("doc read failed for %s: %v", path, err))
			continue
		}
		contentHash := stableContentSHA256(content)
		indexedAt := time.Now().Unix()
		lifecycle := model.DocLifecycleActive
		if prior, ok := storedStates[path]; ok && prior.Lifecycle != "" {
			lifecycle = prior.Lifecycle
		}
		changes = append(changes, store.IncrementalDocChange{
			Action:  "replace",
			Path:    path,
			Content: append([]byte(nil), content...),
			State: &model.DocArtifactState{
				Path:                path,
				Size:                stamp.size,
				MtimeNsec:           stamp.mtimeNsec,
				ContentSHA256:       contentHash,
				ParserVersion:       parserVersion,
				AuthorityConfigHash: configHash,
				IndexedAt:           indexedAt,
				LastDocsGenAt:       indexedAt,
				Lifecycle:           lifecycle,
			},
		})
	}
	return changes, warnings
}
