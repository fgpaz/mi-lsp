package livecontext

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

type manifestEntry struct {
	CanonicalManifestEntry
	absolutePath string
}

// DiscoverCanonicalManifest performs metadata-only discovery using the same
// profile, ignore matcher, declared [[canon]] roots, and symlink boundaries as
// the docs index. It never reads document bodies.
func DiscoverCanonicalManifest(ctx context.Context, root string, profile model.DocsReadProfile, matcher *workspace.IgnoreMatcher) ([]CanonicalManifestEntry, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	entries, err := discoverCanonicalManifest(ctx, root, profile, matcher, nil)
	if err != nil {
		return nil, err
	}
	out := make([]CanonicalManifestEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.CanonicalManifestEntry)
	}
	return out, nil
}

// DiscoverCanonicalDocs is a small explicit-root form useful to callers that
// already resolved canonical roots. Every root is treated as a safe directory
// root and results are sorted by workspace-relative path.
func DiscoverCanonicalDocs(ctx context.Context, canonicalRoots []string) ([]CanonicalManifestEntry, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(canonicalRoots) == 0 {
		return nil, nil
	}
	workspaceRoot, err := filepath.Abs(canonicalRoots[0])
	if err != nil {
		return nil, ErrCanonicalDiscovery
	}
	workspaceRoot = filepath.Clean(workspaceRoot)
	entries := make([]manifestEntry, 0)
	seen := make(map[string]struct{})
	for _, root := range canonicalRoots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		absRoot, err := filepath.Abs(strings.TrimSpace(root))
		if err != nil {
			return nil, ErrCanonicalDiscovery
		}
		absRoot = filepath.Clean(absRoot)
		if err := validateCanonicalRoot(absRoot); err != nil {
			return nil, err
		}
		if err := walkManifestRoot(ctx, workspaceRoot, absRoot, nil, seen, &entries); err != nil {
			return nil, err
		}
	}
	sortManifest(entries)
	out := make([]CanonicalManifestEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.CanonicalManifestEntry)
	}
	return out, nil
}

func discoverCanonicalManifest(ctx context.Context, root string, profile model.DocsReadProfile, matcher *workspace.IgnoreMatcher, extraRoots []string) ([]manifestEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspaceRoot, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return nil, ErrCanonicalDiscovery
	}
	workspaceRoot = filepath.Clean(workspaceRoot)
	if profile.Version == 0 && len(profile.Families) == 0 && len(profile.GenericDocs.Paths) == 0 {
		profile = docgraph.DefaultProfile()
	}
	if matcher == nil {
		matcher, err = workspace.LoadIgnoreMatcher(workspaceRoot, nil)
		if err != nil {
			return nil, ErrCanonicalDiscovery
		}
	}

	entries := make([]manifestEntry, 0)
	seen := make(map[string]struct{})
	addFile := func(absPath string) error {
		return addManifestFile(workspaceRoot, absPath, matcher, seen, &entries)
	}
	for _, family := range profile.Families {
		for _, pattern := range family.Paths {
			if err := expandManifestPattern(ctx, workspaceRoot, pattern, matcher, addFile); err != nil {
				return nil, err
			}
		}
	}
	for _, pattern := range profile.GenericDocs.Paths {
		if err := expandManifestPattern(ctx, workspaceRoot, pattern, matcher, addFile); err != nil {
			return nil, err
		}
	}

	// Declared canon roots use the repository's validated resolver rather than
	// parsing governance/configuration a second time.
	declaredRoots := make(map[string]struct{})
	if project, loadErr := workspace.LoadProjectFile(workspaceRoot); loadErr == nil && len(project.Canons) > 0 {
		// Resolve each declaration independently, matching the indexer's
		// fail-closed behavior: one invalid foreign root must not suppress safe
		// local roots or unrelated declared canons.
		for _, declared := range project.Canons {
			one := model.ProjectFile{Canons: []model.WorkspaceCanon{declared}, CanonPolicy: project.CanonPolicy}
			resolved, resolveErr := workspace.ResolveCanons(workspaceRoot, one)
			if resolveErr != nil {
				continue
			}
			for _, canon := range resolved {
				declaredRoots[filepath.Clean(canon.AbsRoot)] = struct{}{}
				if err := walkManifestRoot(ctx, workspaceRoot, canon.AbsRoot, matcher, seen, &entries); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, extra := range extraRoots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		absRoot, resolveErr := resolveExplicitRoot(workspaceRoot, extra)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if !pathInside(workspaceRoot, absRoot) {
			if _, declared := declaredRoots[filepath.Clean(absRoot)]; !declared {
				return nil, ErrCanonicalDiscovery
			}
		}
		if err := walkManifestRoot(ctx, workspaceRoot, absRoot, matcher, seen, &entries); err != nil {
			return nil, err
		}
	}
	sortManifest(entries)
	return entries, nil
}

func resolveExplicitRoot(workspaceRoot, declared string) (string, error) {
	declared = strings.TrimSpace(declared)
	if declared == "" || strings.ContainsRune(declared, 0) {
		return "", ErrCanonicalDiscovery
	}
	candidate := declared
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(workspaceRoot, filepath.FromSlash(candidate))
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", ErrCanonicalDiscovery
	}
	abs = filepath.Clean(abs)
	return abs, validateCanonicalRoot(abs)
}

func validateCanonicalRoot(absRoot string) error {
	info, err := os.Lstat(absRoot)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrCanonicalDiscovery
	}
	// Check every existing ancestor as well as the final directory. On Windows,
	// Lstat reports directory junctions as non-directories, so the IsDir check
	// rejects those reparse points without relying on path spelling.
	for ancestor := filepath.Dir(filepath.Clean(absRoot)); ; {
		ancestorInfo, ancestorErr := os.Lstat(ancestor)
		if ancestorErr != nil || !ancestorInfo.IsDir() || ancestorInfo.Mode()&os.ModeSymlink != 0 {
			return ErrCanonicalDiscovery
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			break
		}
		ancestor = parent
	}
	physical, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return ErrCanonicalDiscovery
	}
	// Windows may change case or expand a short path during evaluation; compare
	// directory identity rather than requiring equivalent path spellings.
	physicalInfo, err := os.Stat(physical)
	if err != nil || !physicalInfo.IsDir() || !os.SameFile(info, physicalInfo) {
		return ErrCanonicalDiscovery
	}
	return nil
}

func expandManifestPattern(ctx context.Context, root, pattern string, matcher *workspace.IgnoreMatcher, addFile func(string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	if pattern == "" {
		return nil
	}
	if !safeRelativePattern(pattern) {
		return nil
	}
	cleanPattern := strings.TrimSuffix(pattern, "/")
	if cleanPattern == "" {
		cleanPattern = "."
	}
	candidate := filepath.Join(root, filepath.FromSlash(cleanPattern))
	if strings.HasSuffix(pattern, "/") {
		info, err := os.Lstat(candidate)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		return walkManifestRootWithConfig(ctx, root, candidate, matcher, nil, nil, nilWithAddFile(addFile))
	}
	if strings.ContainsAny(cleanPattern, "*?[") {
		matches, err := filepath.Glob(candidate)
		if err != nil {
			return nil
		}
		for _, match := range matches {
			if err := ctx.Err(); err != nil {
				return err
			}
			info, statErr := os.Lstat(match)
			if statErr != nil || info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if info.IsDir() {
				if err := walkManifestRootWithConfig(ctx, root, match, matcher, nil, nil, nilWithAddFile(addFile)); err != nil {
					return err
				}
				continue
			}
			if err := addFile(match); err != nil {
				return err
			}
		}
		return nil
	}
	return addFile(candidate)
}

// nilWithAddFile adapts the generic root walker to a pattern expansion. The
// walker uses the callback directly and keeps the path/metadata policy shared.
func nilWithAddFile(addFile func(string) error) *manifestWalkConfig {
	return &manifestWalkConfig{addFile: addFile}
}

type manifestWalkConfig struct {
	addFile func(string) error
}

func walkManifestRoot(ctx context.Context, workspaceRoot, root string, matcher *workspace.IgnoreMatcher, seen map[string]struct{}, entries *[]manifestEntry) error {
	return walkManifestRootWithConfig(ctx, workspaceRoot, root, matcher, seen, entries, nil)
}

func walkManifestRootWithConfig(ctx context.Context, workspaceRoot, root string, matcher *workspace.IgnoreMatcher, seen map[string]struct{}, entries *[]manifestEntry, config *manifestWalkConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateCanonicalRoot(root); err != nil {
		return err
	}
	return filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return nil
		}
		if entry == nil {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || isExcludedCanonicalPath(workspaceRoot, current) || (matcher != nil && matcher.ShouldIgnore(workspaceRoot, current)) {
				return filepath.SkipDir
			}
			return nil
		}
		if config != nil && config.addFile != nil {
			return config.addFile(current)
		}
		return addManifestFile(workspaceRoot, current, matcher, seen, entries)
	})
}

func addManifestFile(workspaceRoot, absPath string, matcher *workspace.IgnoreMatcher, seen map[string]struct{}, entries *[]manifestEntry) error {
	if !isMarkdownPath(absPath) {
		return nil
	}
	if pathContainsSymlink(workspaceRoot, absPath) {
		return nil
	}
	info, err := os.Lstat(absPath)
	if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	if matcher != nil && matcher.ShouldIgnore(workspaceRoot, absPath) {
		return nil
	}
	if isExcludedCanonicalPath(workspaceRoot, absPath) {
		return nil
	}
	rel, err := filepath.Rel(workspaceRoot, absPath)
	if err != nil || filepath.IsAbs(rel) {
		return nil
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." || strings.ContainsRune(rel, 0) {
		return nil
	}
	if seen != nil {
		if _, ok := seen[rel]; ok {
			return nil
		}
		seen[rel] = struct{}{}
	}
	if entries == nil {
		return nil
	}
	*entries = append(*entries, manifestEntry{
		CanonicalManifestEntry: CanonicalManifestEntry{Path: rel, Size: info.Size(), MtimeNsec: info.ModTime().UnixNano()},
		absolutePath:           filepath.Clean(absPath),
	})
	return nil
}

func pathContainsSymlink(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil || filepath.IsAbs(rel) || rel == "." {
		return false
	}
	current := filepath.Clean(root)
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == "" || part == "." || part == ".." {
			return true
		}
		current = filepath.Join(current, filepath.FromSlash(part))
		info, statErr := os.Lstat(current)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				break
			}
			return true
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return true
		}
	}
	return false
}

func sortManifest(entries []manifestEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Path != entries[j].Path {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].absolutePath < entries[j].absolutePath
	})
}

func safeRelativePattern(pattern string) bool {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	if pattern == "" || strings.ContainsRune(pattern, 0) || strings.HasPrefix(pattern, "/") || strings.HasPrefix(pattern, "//") {
		return false
	}
	if len(pattern) > 1 && pattern[1] == ':' {
		return false
	}
	for _, segment := range strings.Split(pattern, "/") {
		if segment == ".." {
			return false
		}
	}
	return true
}

func isMarkdownPath(filePath string) bool {
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".md", ".mdown", ".markdown":
		return true
	default:
		return false
	}
}

func isExcludedCanonicalPath(workspaceRoot, absPath string) bool {
	rel, err := filepath.Rel(workspaceRoot, absPath)
	if err != nil {
		return true
	}
	normalized := strings.ToLower(filepath.ToSlash(rel))
	// Declared sibling canons may legitimately be outside the workspace; only
	// exclude the explicit raw/audit/state segments below.
	parts := strings.Split(strings.Trim(normalized, "/"), "/")
	for _, part := range parts {
		switch part {
		case ".mi-lsp", ".git", ".worktrees":
			return true
		}
	}
	for index := 0; index+1 < len(parts); index++ {
		if parts[index] == ".docs" && (parts[index+1] == "raw" || parts[index+1] == "auditoria" || parts[index+1] == "temp") {
			return true
		}
	}
	return false
}

func manifestEntryPathForState(workspaceRoot string, entry manifestEntry, statePath string) bool {
	statePath = filepath.ToSlash(strings.TrimSpace(statePath))
	if statePath == "" {
		return false
	}
	if statePath == entry.Path {
		return true
	}
	// The explicit-root skeleton permits state keys relative to a canonical
	// root. Keep this compatibility path bounded to a suffix comparison.
	rel, err := filepath.Rel(workspaceRoot, entry.absolutePath)
	if err == nil && filepath.ToSlash(rel) == statePath {
		return true
	}
	trimmedState := strings.TrimPrefix(statePath, "./")
	return strings.HasSuffix(trimmedState, "/"+entry.Path) || filepath.Base(trimmedState) == filepath.Base(entry.Path)
}

// ReconcileCanonicalDocs returns a metadata/content classification for every
// document under the supplied roots. It performs no parse and no write; the
// overlay builder parses only changes returned by this pass.
func ReconcileCanonicalDocs(ctx context.Context, canonicalRoots []string, artifactStateStore ArtifactStateReader) ([]IncrementalDocChange, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if artifactStateStore == nil {
		return nil, errors.New("artifact state reader is required")
	}
	if len(canonicalRoots) == 0 {
		return nil, nil
	}
	states, err := artifactStateStore.ListDocArtifactStates(ctx)
	if err != nil {
		return nil, err
	}
	workspaceRoot, err := filepath.Abs(strings.TrimSpace(canonicalRoots[0]))
	if err != nil {
		return nil, ErrCanonicalDiscovery
	}
	workspaceRoot = filepath.Clean(workspaceRoot)
	entries := make([]manifestEntry, 0)
	seen := make(map[string]struct{})
	matcher, _ := workspace.LoadIgnoreMatcher(workspaceRoot, nil)
	for _, root := range canonicalRoots {
		absRoot, absErr := filepath.Abs(strings.TrimSpace(root))
		if absErr != nil {
			return nil, ErrCanonicalDiscovery
		}
		if walkErr := walkManifestRoot(ctx, workspaceRoot, filepath.Clean(absRoot), matcher, seen, &entries); walkErr != nil {
			return nil, walkErr
		}
	}
	sortManifest(entries)
	stored := make(map[string]model.DocArtifactState, len(states))
	for _, state := range states {
		key := normalizePublicPath(state.Path)
		if key == "" {
			continue
		}
		state.Path = key
		stored[key] = state
	}
	matched := make(map[string]struct{})
	changes := make([]IncrementalDocChange, 0, len(entries)+len(states))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var state model.DocArtifactState
		var ok bool
		for key, candidate := range stored {
			if _, seen := matched[key]; seen {
				continue
			}
			if manifestEntryPathForState(workspaceRoot, entry, key) {
				state, ok = candidate, true
				matched[key] = struct{}{}
				break
			}
		}
		change := IncrementalDocChange{Path: entry.Path, State: state}
		if !ok {
			content, readStatus, readErr := SafeReadFileContext(ctx, entry.absolutePath)
			if readErr != nil {
				change.Classification = DocChangeReadFailed
				if readStatus == ReadStatusConcurrent {
					change.Classification = DocChangeConcurrent
				}
				change.Action = "omit"
				change.ReadStatus = readStatus
				changes = append(changes, change)
				continue
			}
			change.Classification = DocChangeNew
			change.Action = "replace"
			change.Content = content
			change.ContentHash = sha256Hex(content)
			change.ReadStatus = readStatus
			changes = append(changes, change)
			continue
		}
		classification, content, contentHash, readStatus := classifyManifestEntry(ctx, entry, state)
		change.Classification = classification
		change.ReadStatus = readStatus
		change.Content = content
		change.ContentHash = contentHash
		switch classification {
		case DocChangeChanged, DocChangeNew:
			change.Action = "replace"
		case DocChangeUnchanged:
			change.Action = "reuse"
		case DocChangeDeleted:
			change.Action = "delete"
		default:
			change.Action = "omit"
		}
		changes = append(changes, change)
	}
	for key, state := range stored {
		if _, ok := matched[key]; ok {
			continue
		}
		absPath := filepath.Join(workspaceRoot, filepath.FromSlash(key))
		info, statErr := os.Lstat(absPath)
		if statErr == nil && !info.IsDir() {
			changes = append(changes, IncrementalDocChange{Path: key, Classification: DocChangeExcludedAlive, Action: "omit", State: state})
			continue
		}
		if errors.Is(statErr, os.ErrNotExist) {
			changes = append(changes, IncrementalDocChange{Path: key, Classification: DocChangeDeleted, Action: "delete", Proof: "disk-absent", State: state})
			continue
		}
		changes = append(changes, IncrementalDocChange{Path: key, Classification: DocChangeExcludedAlive, Action: "omit", State: state})
	}
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Path != changes[j].Path {
			return changes[i].Path < changes[j].Path
		}
		return changes[i].Classification < changes[j].Classification
	})
	return changes, nil
}

func classifyManifestEntry(ctx context.Context, entry manifestEntry, stored model.DocArtifactState) (string, []byte, string, ReadStatus) {
	metadataEqual := entry.Size == stored.Size && entry.MtimeNsec == stored.MtimeNsec
	if metadataEqual && stored.ContentSHA256 != "" && safelyOlderThanRacyWindow(entry.MtimeNsec) {
		return DocChangeUnchanged, nil, "", ""
	}
	content, status, err := SafeReadFileContext(ctx, entry.absolutePath)
	if err != nil {
		if status == ReadStatusConcurrent {
			return DocChangeConcurrent, nil, "", status
		}
		return DocChangeReadFailed, nil, "", status
	}
	hash := sha256Hex(content)
	if metadataEqual && hash == stored.ContentSHA256 {
		return DocChangeUnchanged, nil, hash, status
	}
	return DocChangeChanged, content, hash, status
}

func safelyOlderThanRacyWindow(mtimeNsec int64) bool {
	if mtimeNsec <= 0 {
		return false
	}
	now := time.Now().UnixNano()
	window := int64(2)
	if runtime.GOOS == "windows" {
		window = 20_000_000
	}
	return now > mtimeNsec && now-mtimeNsec > window
}

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
