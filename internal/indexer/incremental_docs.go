package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
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
		absPath := docgraph.ResolveDocumentPath(root, path)
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
	return collectCanonicalDocFilesWithBudget(ctx, root, profile, matcher, nil, nil)
}

func collectCanonicalDocFilesBounded(ctx context.Context, root string, profile model.DocsReadProfile, matcher *workspace.IgnoreMatcher, limits canonicalDocScanLimits) ([]canonicalDocFile, []string, error) {
	if ctx == nil || limits.maxEntries < 1 || limits.maxDirectories < 1 || limits.maxFiles < 1 {
		return nil, nil, fmt.Errorf("explicit_incomplete: invalid canonical document scan limits")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, err
	}
	rootReal, setupSource, err := validateCanonicalDocScanInputs(root, profile, matcher)
	if err != nil {
		return nil, nil, err
	}
	budget := &canonicalDocScanBudget{limits: limits, rootReal: rootReal}
	return collectCanonicalDocFilesWithBudget(ctx, root, profile, matcher, budget, &setupSource)
}

func collectCanonicalDocFilesWithBudget(ctx context.Context, root string, profile model.DocsReadProfile, matcher *workspace.IgnoreMatcher, budget *canonicalDocScanBudget, selectedSetup *docgraph.SetupWikiSource) ([]canonicalDocFile, []string, error) {
	seen := make(map[string]canonicalDocFile)
	excluded := make(map[string]struct{})
	visit := func(path string) {
		if err := ctx.Err(); err != nil {
			return
		}
		if matcher != nil && matcher.ShouldIgnore(root, path) {
			rel, relErr := filepath.Rel(root, path)
			if relErr == nil && IsMarkdownPath(path) {
				excluded[filepath.ToSlash(rel)] = struct{}{}
			}
			return
		}
		if !IsMarkdownPath(path) {
			return
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return
		}
		rel = filepath.ToSlash(rel)
		seen[rel] = canonicalDocFile{path: path, relativePath: rel}
	}
	scanPattern := visitDocProfilePattern
	if budget != nil {
		scanPattern = func(ctx context.Context, root, pattern string, matcher *workspace.IgnoreMatcher, visit func(string)) error {
			return visitDocProfilePatternBounded(ctx, root, pattern, matcher, budget, visit)
		}
	}
	for _, family := range profile.Families {
		for _, pattern := range family.Paths {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			if err := scanPattern(ctx, root, pattern, matcher, visit); err != nil {
				return nil, nil, err
			}
		}
	}
	for _, pattern := range profile.GenericDocs.Paths {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if err := scanPattern(ctx, root, pattern, matcher, visit); err != nil {
			return nil, nil, err
		}
	}
	var setupSource docgraph.SetupWikiSource
	if selectedSetup != nil {
		setupSource = *selectedSetup
	} else {
		setupSource = docgraph.ResolveSetupWikiSource(root)
	}
	if setupSource.Configured && setupSource.State == "resolved" {
		if budget != nil {
			setupVisit := func(path string) {
				if err := ctx.Err(); err != nil {
					return
				}
				if IsMarkdownPath(path) {
					rel := docgraph.WorkspacePathForSource(root, path)
					seen[rel] = canonicalDocFile{path: path, relativePath: rel}
				}
			}
			if err := walkCanonicalDocDirBounded(ctx, root, setupSource.Root, nil, budget, true, setupVisit); err != nil {
				return nil, nil, fmt.Errorf("scan Setup wiki source: %w", err)
			}
		} else {
			walkErr := filepath.WalkDir(setupSource.Root, func(path string, entry os.DirEntry, walkErr error) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				if walkErr != nil {
					return walkErr
				}
				if entry.Type()&os.ModeSymlink != 0 {
					if entry.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				if entry.IsDir() {
					if entry.Name() == ".git" {
						return filepath.SkipDir
					}
					return nil
				}
				if IsMarkdownPath(path) {
					rel := docgraph.WorkspacePathForSource(root, path)
					seen[rel] = canonicalDocFile{path: path, relativePath: rel}
				}
				return nil
			})
			if walkErr != nil {
				if ctx.Err() != nil {
					return nil, nil, ctx.Err()
				}
				return nil, nil, fmt.Errorf("scan Setup wiki source: %w", walkErr)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
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

const (
	canonicalDocScanMaxEntries = 16384
	canonicalDocScanMaxDirs    = 4096
	canonicalDocScanMaxFiles   = 4096
)

type canonicalDocScanLimits struct {
	maxEntries, maxDirectories, maxFiles int
}

type canonicalDocScanBudget struct {
	limits                      canonicalDocScanLimits
	rootReal                    string
	entries, directories, files int
}

func defaultCanonicalDocScanLimits() canonicalDocScanLimits {
	return canonicalDocScanLimits{maxEntries: canonicalDocScanMaxEntries, maxDirectories: canonicalDocScanMaxDirs, maxFiles: canonicalDocScanMaxFiles}
}

func validateCanonicalDocScanInputs(root string, profile model.DocsReadProfile, matcher *workspace.IgnoreMatcher) (string, docgraph.SetupWikiSource, error) {
	var noSource docgraph.SetupWikiSource
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", noSource, fmt.Errorf("explicit_incomplete: canonical document scan root is unavailable or unsafe")
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", noSource, fmt.Errorf("explicit_incomplete: canonical document scan root cannot be resolved")
	}
	for _, family := range profile.Families {
		for _, pattern := range family.Paths {
			if err := validateCanonicalDocPattern(root, rootReal, pattern, matcher); err != nil {
				return "", noSource, err
			}
		}
	}
	for _, pattern := range profile.GenericDocs.Paths {
		if err := validateCanonicalDocPattern(root, rootReal, pattern, matcher); err != nil {
			return "", noSource, err
		}
	}
	source := docgraph.ResolveSetupWikiSource(root)
	if source.Configured {
		if source.State != "resolved" || source.Root == "" {
			return "", noSource, fmt.Errorf("explicit_incomplete: configured Setup document source is unresolved")
		}
		if err := validateCanonicalDocRoot(root, rootReal, source.Root); err != nil {
			return "", noSource, fmt.Errorf("explicit_incomplete: configured Setup document root is unsafe")
		}
	}
	return rootReal, source, nil
}

func validateCanonicalDocPattern(root, rootReal, pattern string, matcher *workspace.IgnoreMatcher) error {
	raw := strings.TrimSpace(pattern)
	if raw == "" {
		return nil
	}
	if strings.Contains(raw, "\\") {
		return fmt.Errorf("explicit_incomplete: canonical document pattern is not slash-normalized")
	}
	value := filepath.ToSlash(raw)
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || (len(value) > 1 && value[1] == ':') || filepath.IsAbs(filepath.FromSlash(value)) {
		return fmt.Errorf("explicit_incomplete: canonical document pattern is absolute")
	}
	trailingSlash := strings.HasSuffix(value, "/")
	for strings.HasPrefix(value, "./") {
		value = strings.TrimPrefix(value, "./")
	}
	if value == "" && trailingSlash {
		value = "."
	}
	segments := strings.Split(strings.TrimSuffix(value, "/"), "/")
	if len(segments) == 0 || (len(segments) == 1 && segments[0] == "") {
		return fmt.Errorf("explicit_incomplete: canonical document pattern is empty")
	}
	for _, segment := range segments {
		if segment == "" || segment == ".." {
			return fmt.Errorf("explicit_incomplete: canonical document pattern traverses its root")
		}
		if segment == "." {
			continue
		}
		if _, err := filepath.Match(segment, segment); err != nil {
			return fmt.Errorf("explicit_incomplete: canonical document pattern is malformed")
		}
	}
	current := root
	for i, segment := range segments {
		if segment == "." {
			continue
		}
		if strings.ContainsAny(segment, "*?[") {
			break
		}
		current = filepath.Join(current, filepath.FromSlash(segment))
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("explicit_incomplete: canonical document scan prefix cannot be inspected")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if err := validateCanonicalDocSymlink(rootReal, current); err != nil {
				return err
			}
			// Containment is checked above even for ignored literals. Only a
			// fully specified path may use the same ignore decision as visit;
			// glob prefixes cannot prove their descendants are all excluded.
			if matcher != nil && !strings.ContainsAny(value, "*?[") && matcher.ShouldIgnore(root, filepath.Join(root, filepath.FromSlash(value))) {
				return nil
			}
			return fmt.Errorf("explicit_incomplete: canonical document scan prefix is symlinked")
		}
		if err := validateCanonicalDocContainedPath(rootReal, current); err != nil {
			return err
		}
		if i < len(segments)-1 && !info.IsDir() {
			return nil
		}
	}
	return nil
}

func validateCanonicalDocRoot(root, rootReal, candidate string) error {
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return fmt.Errorf("explicit_incomplete: canonical document scan root cannot be resolved")
	}
	rel, err := filepath.Rel(root, candidateAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("explicit_incomplete: canonical document scan root escapes its workspace")
	}
	info, err := os.Lstat(candidateAbs)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("explicit_incomplete: canonical document scan root is unavailable")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if err := validateCanonicalDocSymlink(rootReal, candidateAbs); err != nil {
			return err
		}
		return fmt.Errorf("explicit_incomplete: canonical document scan root is symlinked")
	}
	return validateCanonicalDocContainedPath(rootReal, candidateAbs)
}

func validateCanonicalDocContainedPath(rootReal, candidate string) error {
	real, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return fmt.Errorf("explicit_incomplete: canonical document path cannot be resolved")
	}
	rel, err := filepath.Rel(rootReal, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("explicit_incomplete: canonical document path escapes its workspace")
	}
	return nil
}

func validateCanonicalDocSymlink(rootReal, candidate string) error {
	real, err := filepath.EvalSymlinks(candidate)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("explicit_incomplete: canonical document symlink cannot be resolved")
	}
	rel, err := filepath.Rel(rootReal, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("explicit_incomplete: canonical document symlink escapes its workspace")
	}
	return nil
}

func visitDocProfilePatternBounded(ctx context.Context, root, pattern string, matcher *workspace.IgnoreMatcher, budget *canonicalDocScanBudget, visit func(string)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	trimmed := filepath.ToSlash(strings.TrimSpace(pattern))
	if trimmed == "" {
		return nil
	}
	if strings.HasSuffix(trimmed, "/") {
		dir := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(trimmed, "/")))
		return walkCanonicalDocDirBounded(ctx, root, dir, matcher, budget, false, visit)
	}
	if strings.ContainsAny(trimmed, "*?[") {
		return walkCanonicalDocGlobBounded(ctx, root, trimmed, matcher, budget, visit)
	}
	path := filepath.Join(root, filepath.FromSlash(trimmed))
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("explicit_incomplete: canonical document path cannot be inspected")
	}
	if info.IsDir() {
		return walkCanonicalDocDirBounded(ctx, root, path, matcher, budget, false, visit)
	}
	if err := budget.countFile(); err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return validateBoundedCanonicalDocSymlink(root, budget.rootReal, path, matcher)
	}
	if err := validateCanonicalDocContainedPath(budget.rootReal, path); err != nil {
		return err
	}
	visit(path)
	return ctx.Err()
}

func walkCanonicalDocGlobBounded(ctx context.Context, root, pattern string, matcher *workspace.IgnoreMatcher, budget *canonicalDocScanBudget, visit func(string)) error {
	value := filepath.ToSlash(filepath.Clean(filepath.FromSlash(pattern)))
	segments := strings.Split(value, "/")
	current := []string{root}
	for index, segment := range segments {
		if err := ctx.Err(); err != nil {
			return err
		}
		last := index == len(segments)-1
		next := make([]string, 0)
		for _, base := range current {
			if err := ctx.Err(); err != nil {
				return err
			}
			if strings.ContainsAny(segment, "*?[") {
				entries, err := budget.readDirectory(ctx, base)
				if err != nil {
					return err
				}
				for _, entry := range entries {
					if err := ctx.Err(); err != nil {
						return err
					}
					matched, err := filepath.Match(segment, entry.Name())
					if err != nil {
						return fmt.Errorf("explicit_incomplete: canonical document glob is malformed")
					}
					if !matched {
						continue
					}
					candidate := filepath.Join(base, entry.Name())
					if last {
						if err := visitBoundedGlobMatch(ctx, root, candidate, matcher, budget, visit, true); err != nil {
							return err
						}
						continue
					}
					info, err := os.Lstat(candidate)
					if os.IsNotExist(err) {
						return fmt.Errorf("explicit_incomplete: canonical document glob changed during enumeration")
					}
					if err != nil {
						return fmt.Errorf("explicit_incomplete: canonical document glob entry cannot be inspected")
					}
					if info.Mode()&os.ModeSymlink != 0 {
						if err := validateBoundedCanonicalDocGlobPrefixSymlink(budget.rootReal, candidate); err != nil {
							return err
						}
						continue
					}
					if info.IsDir() {
						if err := validateCanonicalDocContainedPath(budget.rootReal, candidate); err != nil {
							return err
						}
						next = append(next, candidate)
					}
				}
				continue
			}
			candidate := filepath.Join(base, filepath.FromSlash(segment))
			info, err := os.Lstat(candidate)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("explicit_incomplete: canonical document glob prefix cannot be inspected")
			}
			if info.Mode()&os.ModeSymlink != 0 {
				if last {
					if err := visitBoundedGlobMatch(ctx, root, candidate, matcher, budget, visit, false); err != nil {
						return err
					}
				} else if err := validateBoundedCanonicalDocGlobPrefixSymlink(budget.rootReal, candidate); err != nil {
					return err
				}
				continue
			}
			if last {
				if err := visitBoundedGlobMatch(ctx, root, candidate, matcher, budget, visit, false); err != nil {
					return err
				}
			} else if info.IsDir() {
				if err := validateCanonicalDocContainedPath(budget.rootReal, candidate); err != nil {
					return err
				}
				next = append(next, candidate)
			}
		}
		if last {
			return nil
		}
		current = next
		if len(current) == 0 {
			return nil
		}
	}
	return nil
}

func validateBoundedCanonicalDocSymlink(root, rootReal, path string, matcher *workspace.IgnoreMatcher) error {
	if err := validateCanonicalDocSymlink(rootReal, path); err != nil {
		return err
	}
	if matcher != nil && matcher.ShouldIgnore(root, path) {
		return nil
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("explicit_incomplete: canonical document symlink path cannot be assigned to this workspace")
	}
	if isCanonicalGraphDoc(filepath.ToSlash(rel), false) {
		return fmt.Errorf("explicit_incomplete: canonical document symlink cannot be read as a physical owner")
	}
	return nil
}

func validateBoundedCanonicalDocGlobPrefixSymlink(rootReal, path string) error {
	if err := validateCanonicalDocSymlink(rootReal, path); err != nil {
		return err
	}
	return fmt.Errorf("explicit_incomplete: canonical document glob prefix is symlinked")
}

func visitBoundedGlobMatch(ctx context.Context, root, candidate string, matcher *workspace.IgnoreMatcher, budget *canonicalDocScanBudget, visit func(string), entryCounted bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Lstat(candidate)
	if err != nil {
		return fmt.Errorf("explicit_incomplete: canonical document glob match changed during enumeration")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return validateBoundedCanonicalDocSymlink(root, budget.rootReal, candidate, matcher)
	}
	if err := validateCanonicalDocContainedPath(budget.rootReal, candidate); err != nil {
		return err
	}
	if info.IsDir() {
		return walkCanonicalDocDirBounded(ctx, root, candidate, matcher, budget, false, visit)
	}
	if !entryCounted {
		if err := budget.countFile(); err != nil {
			return err
		}
	}
	visit(candidate)
	return ctx.Err()
}

func walkCanonicalDocDirBounded(ctx context.Context, root, dir string, matcher *workspace.IgnoreMatcher, budget *canonicalDocScanBudget, skipGit bool, visit func(string)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("explicit_incomplete: canonical document directory cannot be inspected")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return validateCanonicalDocSymlink(budget.rootReal, dir)
	}
	if !info.IsDir() {
		return nil
	}
	if err := validateCanonicalDocContainedPath(budget.rootReal, dir); err != nil {
		return err
	}
	if (skipGit && filepath.Base(filepath.Clean(dir)) == ".git") || (matcher != nil && matcher.ShouldIgnore(root, dir)) {
		return nil
	}
	stack := []string{dir}
	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		entries, err := budget.readDirectory(ctx, current)
		if err != nil {
			return err
		}
		children := make([]string, 0)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			path := filepath.Join(current, entry.Name())
			info, err := os.Lstat(path)
			if err != nil {
				return fmt.Errorf("explicit_incomplete: canonical document entry changed during enumeration")
			}
			if info.Mode()&os.ModeSymlink != 0 {
				if err := validateBoundedCanonicalDocSymlink(root, budget.rootReal, path, matcher); err != nil {
					return err
				}
				continue
			}
			if err := validateCanonicalDocContainedPath(budget.rootReal, path); err != nil {
				return err
			}
			if info.IsDir() {
				if (skipGit && entry.Name() == ".git") || (matcher != nil && matcher.ShouldIgnore(root, path)) {
					continue
				}
				children = append(children, path)
				continue
			}
			visit(path)
		}
		for index := len(children) - 1; index >= 0; index-- {
			stack = append(stack, children[index])
		}
	}
	return ctx.Err()
}

func (budget *canonicalDocScanBudget) countFile() error {
	budget.files++
	if budget.files > budget.limits.maxFiles {
		return fmt.Errorf("explicit_incomplete: canonical document scan file budget exceeded")
	}
	return nil
}

func (budget *canonicalDocScanBudget) readDirectory(ctx context.Context, dir string) ([]os.DirEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateCanonicalDocContainedPath(budget.rootReal, dir); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("explicit_incomplete: canonical document scan directory is unsafe")
	}
	budget.directories++
	if budget.directories > budget.limits.maxDirectories {
		return nil, fmt.Errorf("explicit_incomplete: canonical document scan directory budget exceeded")
	}
	file, err := os.Open(dir)
	if err != nil {
		return nil, fmt.Errorf("explicit_incomplete: canonical document scan directory cannot be opened")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.IsDir() || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("explicit_incomplete: canonical document scan directory changed before read")
	}
	entries := make([]os.DirEntry, 0)
	const chunkSize = 256
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		remaining := budget.limits.maxEntries - budget.entries
		n := chunkSize
		if remaining < n {
			n = remaining + 1
		}
		if n < 1 {
			n = 1
		}
		batch, readErr := file.ReadDir(n)
		for _, entry := range batch {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			budget.entries++
			if budget.entries > budget.limits.maxEntries {
				return nil, fmt.Errorf("explicit_incomplete: canonical document scan entry budget exceeded")
			}
			if !entry.IsDir() {
				if err := budget.countFile(); err != nil {
					return nil, err
				}
			}
			entries = append(entries, entry)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("explicit_incomplete: canonical document directory read failed")
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

// IsMarkdownPath reports whether a path has an extension supported by the docs indexer.
func IsMarkdownPath(path string) bool {
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

func lifecycleFromSource(doc model.DocRecord, bindings []model.DocArtifactBinding, fallback string) string {
	if doc.BindingSources != nil && doc.BindingSources.Availability == model.DocBindingSourceAvailable {
		switch doc.BindingSources.DocumentLifecycle {
		case model.DocLifecycleActive, model.DocLifecycleDeprecated, model.DocLifecycleRetired:
			return doc.BindingSources.DocumentLifecycle
		}
	}
	return lifecycleFromBindings(bindings, fallback)
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
		absPath := docgraph.ResolveDocumentPath(root, path)
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
