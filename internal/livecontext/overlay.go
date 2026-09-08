package livecontext

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

type fileStamp struct {
	size      int64
	mtimeNsec int64
}

var (
	safeReadStatPath = os.Lstat
	safeReadBytes    = os.ReadFile
)

// SafeReadFile reads a regular file with stat-before/read/stat-after and one
// retry. A second size/mtime mismatch is a typed concurrent-change result.
func SafeReadFile(filePath string) ([]byte, ReadStatus, error) {
	return SafeReadFileContext(context.Background(), filePath)
}

// SafeReadFileContext is the context-aware implementation used by overlays.
func SafeReadFileContext(ctx context.Context, filePath string) ([]byte, ReadStatus, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(filePath) == "" || !filepath.IsAbs(filePath) {
		return nil, ReadStatusStable, ErrUnsafeTarget
	}
	var before fileStamp
	for attempt := 1; attempt <= 2; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, ReadStatusStable, err
		}
		statBefore, err := safeReadStatPath(filePath)
		if err != nil {
			return nil, ReadStatusStable, err
		}
		if statBefore.Mode()&os.ModeSymlink != 0 || !statBefore.Mode().IsRegular() {
			return nil, ReadStatusStable, ErrUnsafeTarget
		}
		before = fileStamp{size: statBefore.Size(), mtimeNsec: statBefore.ModTime().UnixNano()}
		content, err := safeReadBytes(filePath)
		if err != nil {
			return nil, ReadStatusStable, err
		}
		if err := ctx.Err(); err != nil {
			return nil, ReadStatusStable, err
		}
		statAfter, err := safeReadStatPath(filePath)
		if err != nil {
			return nil, ReadStatusStable, err
		}
		if statAfter.Mode()&os.ModeSymlink != 0 || !statAfter.Mode().IsRegular() {
			return nil, ReadStatusConcurrent, ErrUnsafeTarget
		}
		after := fileStamp{size: statAfter.Size(), mtimeNsec: statAfter.ModTime().UnixNano()}
		if before == after {
			if attempt == 1 {
				return content, ReadStatusStable, nil
			}
			return content, ReadStatusRacy, nil
		}
		if attempt == 2 {
			return nil, ReadStatusConcurrent, &model.ErrConcurrentChange{
				Path:             filePath,
				StoredMtimeNsec:  before.mtimeNsec,
				StoredSize:       before.size,
				CurrentMtimeNsec: after.mtimeNsec,
				CurrentSize:      after.size,
				Attempt:          attempt,
			}
		}
	}
	return nil, ReadStatusConcurrent, &model.ErrConcurrentChange{Path: filePath, Attempt: 2}
}

// SafeResolveTarget validates a repository-relative target and resolves every
// existing symlink component. The second return value is an omission code; it
// is empty only for a safe target. Missing final files remain safe and return
// their checked, lexical path so the resolver can report missing_path.
func SafeResolveTarget(workspaceRoot, targetPath string) (string, string, error) {
	root, err := filepath.Abs(strings.TrimSpace(workspaceRoot))
	if err != nil {
		return "", model.OmissionUnsafeTarget, ErrUnsafeTarget
	}
	root = filepath.Clean(root)
	if !validRepoRelativeTarget(targetPath) {
		return "", model.OmissionUnsafeTarget, ErrUnsafeTarget
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() {
		return "", model.OmissionUnsafeTarget, ErrUnsafeTarget
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", model.OmissionUnsafeTarget, ErrUnsafeTarget
	}
	realRoot = filepath.Clean(realRoot)
	// Resolve relative components from the physical workspace root. This keeps
	// a symlinked presentation path from being mistaken for an escape.
	lexical := filepath.Clean(filepath.Join(realRoot, filepath.FromSlash(normalizeSlash(targetPath))))
	if !pathInside(realRoot, lexical) {
		return "", model.OmissionUnsafeTarget, ErrUnsafeTarget
	}

	parts := strings.Split(filepath.ToSlash(normalizeSlash(targetPath)), "/")
	current := realRoot
	resolved := realRoot
	for index, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", model.OmissionUnsafeTarget, ErrUnsafeTarget
		}
		candidate := filepath.Join(current, filepath.FromSlash(part))
		info, statErr := os.Lstat(candidate)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				resolved = filepath.Join(candidate, filepath.FromSlash(strings.Join(parts[index+1:], "/")))
				break
			}
			return "", model.OmissionUnsafeTarget, ErrUnsafeTarget
		}
		if info.Mode()&os.ModeSymlink != 0 {
			physical, evalErr := filepath.EvalSymlinks(candidate)
			if evalErr != nil || !pathInside(realRoot, physical) {
				return "", model.OmissionUnsafeTarget, ErrUnsafeTarget
			}
			current = filepath.Clean(physical)
			resolved = current
			continue
		}
		current = candidate
		resolved = candidate
	}
	if !pathInside(realRoot, resolved) {
		return "", model.OmissionUnsafeTarget, ErrUnsafeTarget
	}
	return filepath.Clean(resolved), "", nil
}

func validRepoRelativeTarget(targetPath string) bool {
	normalized := normalizeSlash(strings.TrimSpace(targetPath))
	if normalized == "" || strings.ContainsRune(normalized, 0) || strings.ContainsAny(normalized, "\r\n") {
		return false
	}
	if strings.HasPrefix(normalized, "/") || strings.HasPrefix(normalized, "//") || strings.HasPrefix(normalized, "~/") || normalized == "~" {
		return false
	}
	if len(normalized) > 1 && normalized[1] == ':' {
		return false
	}
	if path.IsAbs(normalized) {
		return false
	}
	for _, part := range strings.Split(normalized, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func normalizeSlash(value string) string { return strings.ReplaceAll(value, "\\", "/") }

func pathInside(root, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return rel != ".." && !strings.HasPrefix(rel, "../") && !filepath.IsAbs(rel)
}

type overlayState struct {
	baseGeneration string
	dbAvailable    bool
	baseKnown      bool
	manifestReady  bool
	authorityReady bool
	profile        model.DocsReadProfile
	matcher        *workspace.IgnoreMatcher
	states         map[string]model.DocArtifactState
	bindings       []model.DocArtifactBinding
	byOwner        map[string][]model.DocArtifactBinding
}

func buildOverlay(ctx context.Context, req OverlayRequest) (model.WikiCodeOverlay, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	root := strings.TrimSpace(req.WorkspaceRoot)
	if root == "" {
		root = strings.TrimSpace(req.Root)
	}
	if root == "" {
		return model.WikiCodeOverlay{}, ErrInvalidScope
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return model.WikiCodeOverlay{}, ErrInvalidScope
	}
	absRoot = filepath.Clean(absRoot)
	scope, err := normalizeScope(req.Scope)
	if err != nil {
		return model.WikiCodeOverlay{}, err
	}
	if req.MaxDocuments < 0 || req.MaxBytes < 0 {
		return model.WikiCodeOverlay{}, ErrInvalidScope
	}

	overlay := model.WikiCodeOverlay{
		Mode:          model.OverlayModeRAMOnly,
		Status:        model.OverlayStatusUnknown,
		Scope:         scope,
		Freshness:     model.WikiCodeFreshness{DocsManifest: model.FreshnessUnknown, Bindings: model.FreshnessUnknown, Catalog: model.FreshnessUnknown, Graph: model.FreshnessUnknown, Authority: model.FreshnessUnknown},
		ChangedInputs: make([]model.WikiCodeChangedInput, 0),
		Additions:     make([]model.DocArtifactBinding, 0),
		Tombstones:    make([]model.BindingTombstone, 0),
		Omissions:     make([]model.WikiCodeOmission, 0),
	}
	if strings.TrimSpace(req.BaseGeneration) != "" {
		overlay.BaseGeneration = strings.TrimSpace(req.BaseGeneration)
	} else {
		generation, generationErr := store.ReadWorkspaceGenerationSnapshot(ctx, absRoot)
		if generationErr != nil {
			overlay.BaseGeneration = "unavailable"
			appendOmission(&overlay, model.WikiCodeOmission{Code: model.OmissionDatabaseUnavailable, Reason: "published generation is unavailable"})
		} else {
			overlay.BaseGeneration = generation
		}
	}

	profile, profileReady := loadOverlayProfile(absRoot, req.Profile)
	if !profileReady {
		appendOmission(&overlay, model.WikiCodeOmission{Code: model.OmissionAuthorityUnavailable, Reason: "canonical read-model profile is unavailable"})
	}
	matcher := req.Matcher
	if matcher == nil {
		matcher, _ = workspace.LoadIgnoreMatcher(absRoot, nil)
	}
	state := overlayState{baseGeneration: overlay.BaseGeneration, profile: profile, matcher: matcher, authorityReady: profileReady, states: make(map[string]model.DocArtifactState), byOwner: make(map[string][]model.DocArtifactBinding)}

	db, closeDB, dbErr := openOverlayDB(absRoot, req)
	state.baseKnown = dbErr == nil
	if dbErr != nil {
		appendOmission(&overlay, model.WikiCodeOmission{Code: model.OmissionDatabaseUnavailable, Reason: "read-only published snapshot is unavailable"})
	} else if db != nil {
		state.dbAvailable = true
	}
	if closeDB != nil {
		defer closeDB()
	}
	if req.StateReader != nil {
		states, statesErr := req.StateReader.ListDocArtifactStates(ctx)
		if statesErr != nil {
			appendOmission(&overlay, model.WikiCodeOmission{Code: model.OmissionDatabaseUnavailable, Reason: "artifact state snapshot is unavailable"})
		} else {
			for _, item := range states {
				item.Path = normalizePublicPath(item.Path)
				if item.Path != "" {
					state.states[item.Path] = item
				}
			}
		}
	} else if db != nil {
		states, statesErr := readArtifactStates(ctx, db)
		if statesErr != nil {
			state.dbAvailable = false
			appendOmission(&overlay, model.WikiCodeOmission{Code: model.OmissionDatabaseUnavailable, Reason: "artifact state snapshot is unavailable"})
		} else {
			for _, item := range states {
				item.Path = normalizePublicPath(item.Path)
				if item.Path != "" {
					state.states[item.Path] = item
				}
			}
		}
	}
	if db != nil {
		bindings, bindingsErr := readAllBindings(ctx, db)
		if bindingsErr != nil {
			state.dbAvailable = false
			state.baseKnown = false
			appendOmission(&overlay, model.WikiCodeOmission{Code: model.OmissionDatabaseUnavailable, Reason: "binding snapshot is unavailable"})
		} else {
			state.baseKnown = true
			state.bindings = sanitizeBindings(bindings)
			for _, binding := range state.bindings {
				state.byOwner[binding.DocPath] = append(state.byOwner[binding.DocPath], binding)
			}
		}
	}
	validatePersistedBindings(absRoot, scope, &state, &overlay)

	maxDocs, maxBytes := overlayBounds(scope.Kind, req.MaxDocuments, req.MaxBytes)
	if scope.Kind == model.ScopeExactWiki {
		if err := buildExactOverlay(ctx, absRoot, req, &scope, &overlay, &state, db, maxDocs, maxBytes); err != nil {
			if errors.Is(err, ErrInvalidScope) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return model.WikiCodeOverlay{}, err
			}
			appendOmission(&overlay, model.WikiCodeOmission{Code: model.OmissionUnknownDocument, Reason: "exact wiki scope could not be reconciled"})
		}
	} else {
		if err := buildReverseOverlay(ctx, absRoot, req, &scope, &overlay, &state, db, maxDocs, maxBytes); err != nil {
			if errors.Is(err, ErrInvalidScope) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return model.WikiCodeOverlay{}, err
			}
			appendOmission(&overlay, model.WikiCodeOmission{Code: model.OmissionUnknownDocument, Reason: "reverse canonical manifest could not be reconciled"})
		}
	}
	if closeDB != nil {
		// Deferred close remains the owner of the handle; this branch exists only
		// to make the lifetime explicit next to the final canonicalization.
	}
	finalizeOverlay(&overlay, state)
	return overlay, nil
}

func normalizeScope(scope model.WikiCodeScope) (model.WikiCodeScope, error) {
	kind := model.WikiCodeScopeKind(strings.TrimSpace(string(scope.Kind)))
	if kind == "" {
		return model.WikiCodeScope{}, ErrInvalidScope
	}
	if kind != model.ScopeExactWiki && kind != model.ScopeReverseCode {
		return model.WikiCodeScope{}, ErrInvalidScope
	}
	scope.Kind = kind
	scope.DocIDs = normalizeStringSet(scope.DocIDs)
	scope.DocPaths = normalizeDocPathSet(scope.DocPaths)
	for index := range scope.Blocks {
		scope.Blocks[index].DocPath = normalizePublicPath(scope.Blocks[index].DocPath)
		scope.Blocks[index].DocID = strings.TrimSpace(scope.Blocks[index].DocID)
		scope.Blocks[index].BlockID = strings.TrimSpace(scope.Blocks[index].BlockID)
		if scope.Blocks[index].BlockID == "" {
			return model.WikiCodeScope{}, ErrInvalidScope
		}
	}
	sort.Slice(scope.Blocks, func(i, j int) bool {
		a, b := scope.Blocks[i], scope.Blocks[j]
		if a.DocPath != b.DocPath {
			return a.DocPath < b.DocPath
		}
		if a.DocID != b.DocID {
			return a.DocID < b.DocID
		}
		return a.BlockID < b.BlockID
	})
	uniqueBlocks := make([]model.WikiCodeBlockScope, 0, len(scope.Blocks))
	for _, block := range scope.Blocks {
		if len(uniqueBlocks) > 0 {
			last := uniqueBlocks[len(uniqueBlocks)-1]
			if last == block {
				continue
			}
		}
		uniqueBlocks = append(uniqueBlocks, block)
	}
	scope.Blocks = uniqueBlocks
	if kind == model.ScopeReverseCode {
		if strings.TrimSpace(scope.TargetPath) == "" {
			return model.WikiCodeScope{}, ErrInvalidScope
		}
		// Keep only a repo-relative spelling in the public scope. Unsafe input is
		// represented by an empty target and is reported by SafeResolveTarget;
		// the original host path never enters the overlay model.
		target := normalizeSlash(strings.TrimSpace(scope.TargetPath))
		if validRepoRelativeTarget(target) {
			scope.TargetPath = target
		} else {
			scope.TargetPath = ""
		}
		scope.TargetSymbol = strings.TrimSpace(scope.TargetSymbol)
		if len(scope.DocIDs) != 0 || len(scope.DocPaths) != 0 || len(scope.Blocks) != 0 {
			return model.WikiCodeScope{}, ErrInvalidScope
		}
	} else if len(scope.DocIDs) == 0 && len(scope.DocPaths) == 0 && len(scope.Blocks) == 0 {
		return model.WikiCodeScope{}, ErrInvalidScope
	}
	return scope, nil
}

func normalizeStringSet(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
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
	return out
}

func normalizeDocPathSet(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizePublicPath(value)
		if value == "" || filepath.IsAbs(value) || value == ".." || strings.ContainsRune(value, 0) {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func loadOverlayProfile(root string, supplied *model.DocsReadProfile) (model.DocsReadProfile, bool) {
	if supplied != nil {
		return *supplied, true
	}
	profile, _, warnings := docgraph.LoadProfile(root)
	return profile, len(warnings) == 0
}

func openOverlayDB(root string, req OverlayRequest) (*sql.DB, func(), error) {
	if req.DB != nil {
		return req.DB, nil, nil
	}
	dbPath := strings.TrimSpace(req.DBPath)
	if dbPath == "" {
		dbPath = store.WorkspaceDBPath(root)
	}
	if _, err := os.Stat(dbPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	readOnly, err := store.OpenReadOnlyExisting(root, dbPath)
	if err != nil {
		return nil, nil, err
	}
	return readOnly, func() { _ = readOnly.Close() }, nil
}

func readArtifactStates(ctx context.Context, db *sql.DB) ([]model.DocArtifactState, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT path, size, mtime_nsec, content_sha256, parser_version,
		       authority_config_hash, indexed_at, last_docs_gen_at, lifecycle
		FROM doc_artifact_states ORDER BY path ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.DocArtifactState, 0)
	for rows.Next() {
		var state model.DocArtifactState
		if err := rows.Scan(&state.Path, &state.Size, &state.MtimeNsec, &state.ContentSHA256,
			&state.ParserVersion, &state.AuthorityConfigHash, &state.IndexedAt,
			&state.LastDocsGenAt, &state.Lifecycle); err != nil {
			return nil, err
		}
		result = append(result, state)
	}
	return result, rows.Err()
}

func readAllBindings(ctx context.Context, db *sql.DB) ([]model.DocArtifactBinding, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT doc_path, block_id, doc_id, relation, role, target_path, target_symbol, target_kind,
		       authoring_origin, binding_status, doc_lifecycle, superseded_by, ordinal, start_line, end_line,
		       source_content_hash, binding_ref, indexed_at
		FROM doc_artifact_bindings ORDER BY doc_path ASC, block_id ASC, ordinal ASC, binding_ref ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.DocArtifactBinding, 0)
	for rows.Next() {
		var item model.DocArtifactBinding
		if err := rows.Scan(&item.DocPath, &item.BlockID, &item.DocID, &item.Relation, &item.Role,
			&item.TargetPath, &item.TargetSymbol, &item.TargetKind, &item.AuthoringOrigin,
			&item.BindingStatus, &item.DocLifecycle, &item.SupersededBy, &item.Ordinal,
			&item.StartLine, &item.EndLine, &item.SourceContentHash, &item.BindingRef, &item.IndexedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func readDocPathsForIDs(ctx context.Context, db *sql.DB, ids []string) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	if db == nil || len(ids) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i], args[i] = "?", id
	}
	query := `SELECT path FROM doc_records WHERE doc_id IN (` + strings.Join(placeholders, ",") + `)
		UNION SELECT doc_path FROM doc_artifact_bindings WHERE doc_id IN (` + strings.Join(placeholders, ",") + `)`
	args = append(args, args...)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item string
		if err := rows.Scan(&item); err != nil {
			return nil, err
		}
		if item = normalizePublicPath(item); item != "" {
			result[item] = struct{}{}
		}
	}
	return result, rows.Err()
}

func validatePersistedBindings(root string, scope model.WikiCodeScope, state *overlayState, overlay *model.WikiCodeOverlay) {
	if state == nil || overlay == nil {
		return
	}
	selected := make(map[string]struct{}, len(scope.DocPaths)+len(scope.Blocks))
	for _, docPath := range scope.DocPaths {
		selected[docPath] = struct{}{}
	}
	for _, block := range scope.Blocks {
		if block.DocPath != "" {
			selected[block.DocPath] = struct{}{}
		}
	}
	if scope.Kind == model.ScopeExactWiki && len(selected) == 0 {
		return
	}
	for _, binding := range state.bindings {
		if scope.Kind == model.ScopeExactWiki {
			if _, ok := selected[binding.DocPath]; !ok {
				continue
			}
		} else {
			if binding.TargetPath != scope.TargetPath || (scope.TargetSymbol != "" && binding.TargetSymbol != scope.TargetSymbol) {
				continue
			}
		}
		if _, _, err := SafeResolveTarget(root, binding.TargetPath); err != nil {
			appendTombstone(overlay, model.BindingTombstone{BindingRef: binding.BindingRef, DocPath: binding.DocPath, BlockID: binding.BlockID, Reason: model.OmissionUnsafeTarget})
			appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionUnsafeTarget, DocPath: binding.DocPath, TargetPath: binding.TargetPath, Reason: "persisted target is outside the repository or crosses an unsafe link"})
		}
	}
}

func readDocPathsForBlocks(ctx context.Context, db *sql.DB, blocks []model.WikiCodeBlockScope) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	if db == nil {
		return result, nil
	}
	for _, block := range blocks {
		if strings.TrimSpace(block.DocPath) != "" {
			continue
		}
		if strings.TrimSpace(block.DocID) == "" {
			continue
		}
		rows, err := db.QueryContext(ctx, `
			SELECT doc_path FROM doc_source_blocks WHERE doc_id=? AND block_id=?
			UNION SELECT doc_path FROM doc_artifact_bindings WHERE doc_id=? AND block_id=?
		`, block.DocID, block.BlockID, block.DocID, block.BlockID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var item string
			if err := rows.Scan(&item); err != nil {
				_ = rows.Close()
				return nil, err
			}
			if item = normalizePublicPath(item); item != "" {
				result[item] = struct{}{}
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func buildExactOverlay(ctx context.Context, root string, req OverlayRequest, scope *model.WikiCodeScope, overlay *model.WikiCodeOverlay, state *overlayState, db *sql.DB, maxDocs int, maxBytes int64) error {
	paths := make(map[string]struct{}, len(scope.DocPaths)+len(scope.Blocks))
	for _, item := range scope.DocPaths {
		paths[item] = struct{}{}
	}
	for _, item := range scope.Blocks {
		if item.DocPath != "" {
			paths[item.DocPath] = struct{}{}
		}
	}
	if len(scope.Blocks) > 0 {
		found, err := readDocPathsForBlocks(ctx, db, scope.Blocks)
		if err != nil && db != nil {
			appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionDatabaseUnavailable, Reason: "block selector snapshot is unavailable"})
		} else {
			for item := range found {
				paths[item] = struct{}{}
			}
		}
	}
	if len(scope.DocIDs) > 0 {
		found, err := readDocPathsForIDs(ctx, db, scope.DocIDs)
		if err != nil && db != nil {
			appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionDatabaseUnavailable, Reason: "document selector snapshot is unavailable"})
		} else {
			for item := range found {
				paths[item] = struct{}{}
			}
		}
		for _, id := range scope.DocIDs {
			found := false
			for _, path := range scope.DocPaths {
				if strings.EqualFold(path, id) {
					found = true
				}
			}
			if !found && len(paths) == 0 {
				appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionUnknownDocument, Reason: "document selector is not present in the published snapshot"})
			}
		}
	}
	if len(paths) == 0 {
		for range scope.DocIDs {
			appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionUnknownDocument, Reason: "document selector is not present in the published snapshot"})
		}
		for range scope.Blocks {
			appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionUnknownDocument, Reason: "block selector is not present in the published snapshot"})
		}
		state.manifestReady = true
		return nil
	}
	selected := make([]string, 0, len(paths))
	for item := range paths {
		selected = append(selected, item)
	}
	sort.Strings(selected)
	blockFilter := exactBlockFilter(*scope)
	for _, item := range scope.Blocks {
		if item.DocPath == "" {
			for docPath := range paths {
				if blockFilter[docPath] == nil {
					blockFilter[docPath] = make(map[string]struct{})
				}
				blockFilter[docPath][item.BlockID] = struct{}{}
			}
		}
	}
	state.manifestReady = true
	if len(selected) > maxDocs {
		for _, item := range selected[maxDocs:] {
			appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionUnknownDocument, Path: item, Reason: "exact scope document bound exceeded"})
		}
		selected = selected[:maxDocs]
	}
	for _, docPath := range selected {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry, entryStatus := exactManifestEntry(root, docPath, state.profile, state.matcher, req.CanonicalRoots)
		if entryStatus != "" {
			if entryStatus == model.OmissionUnsafeTarget {
				appendOwnerTombstone(overlay, state, docPath, model.OmissionUnsafeTarget, blockFilter[docPath])
				appendOmission(overlay, model.WikiCodeOmission{Code: entryStatus, Path: docPath, Reason: "document path is outside the canonical workspace"})
			} else {
				appendOmission(overlay, model.WikiCodeOmission{Code: entryStatus, Path: docPath, Reason: "document is excluded from canonical discovery"})
			}
			continue
		}
		if err := processManifestDoc(ctx, root, entry, state, overlay, blockFilter[docPath], maxBytes, false); err != nil {
			return err
		}
	}
	// Explicit selectors can refer to a deleted document that is absent from
	// the current manifest. Positive disk absence, not scan absence, is the
	// only condition that emits a deletion tombstone.
	for _, docPath := range selected {
		if hasChangedInput(overlay, docPath) {
			continue
		}
		if _, ok := state.states[docPath]; !ok {
			continue
		}
		absPath := filepath.Join(root, filepath.FromSlash(docPath))
		if _, statErr := os.Lstat(absPath); errors.Is(statErr, os.ErrNotExist) {
			appendOwnerTombstone(overlay, state, docPath, "disk-absent", blockFilter[docPath])
			appendChangedInput(overlay, model.WikiCodeChangedInput{Path: docPath, Classification: DocChangeDeleted})
		}
	}
	return nil
}

func buildReverseOverlay(ctx context.Context, root string, req OverlayRequest, scope *model.WikiCodeScope, overlay *model.WikiCodeOverlay, state *overlayState, db *sql.DB, maxDocs int, maxBytes int64) error {
	if _, _, err := SafeResolveTarget(root, scope.TargetPath); err != nil {
		appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionUnsafeTarget, TargetPath: scope.TargetPath, Reason: "reverse target is unsafe"})
		return nil
	}
	entries, err := discoverCanonicalManifest(ctx, root, state.profile, state.matcher, req.CanonicalRoots)
	if err != nil {
		return err
	}
	state.manifestReady = true
	overlay.Cost.MetadataChecked += len(entries)
	// Prefer exact active canonical owners and bounded cold candidates before
	// applying the reverse document bound. Candidate ranking only controls reads;
	// the resolver still requires an exact live binding before returning an owner.
	entries = prioritizeReverseManifestEntries(ctx, db, entries, *scope, state)
	if len(entries) > maxDocs && maxDocs > 0 {
		// Metadata reconciliation is intentionally complete; only changed body
		// reads are bounded. The deterministic suffix is represented as a typed
		// omission so callers never infer completeness.
		for _, entry := range entries[maxDocs:] {
			if reverseEntryNeedsRead(entry, state) {
				appendOwnerTombstone(overlay, state, entry.Path, "reverse-scope-bound", nil)
				appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionUnknownDocument, Path: entry.Path, Reason: "reverse scope document bound exceeded"})
			}
		}
	}
	for index, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if maxDocs > 0 && index >= maxDocs {
			if reverseEntryNeedsRead(entry, state) {
				appendOwnerTombstone(overlay, state, entry.Path, "reverse-scope-bound", nil)
				appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionUnknownDocument, Path: entry.Path, Reason: "reverse scope document bound exceeded"})
			}
			continue
		}
		if err := processManifestDoc(ctx, root, entry, state, overlay, nil, maxBytes, true); err != nil {
			return err
		}
	}
	// Include positive deletions for persisted owners no longer in the
	// discovered manifest. Alive-but-excluded owners remain omissions and are
	// deliberately not tombstoned.
	for docPath := range state.byOwner {
		if hasChangedInput(overlay, docPath) || manifestContains(entries, docPath) {
			continue
		}
		absPath := filepath.Join(root, filepath.FromSlash(docPath))
		info, statErr := os.Lstat(absPath)
		if errors.Is(statErr, os.ErrNotExist) {
			appendOwnerTombstone(overlay, state, docPath, "disk-absent", nil)
			appendChangedInput(overlay, model.WikiCodeChangedInput{Path: docPath, Classification: DocChangeDeleted})
		} else if statErr == nil && (info.Mode()&os.ModeSymlink != 0 || (pathInside(root, absPath) && pathContainsSymlink(root, absPath))) {
			appendOwnerTombstone(overlay, state, docPath, model.OmissionUnsafeTarget, nil)
			appendChangedInput(overlay, model.WikiCodeChangedInput{Path: docPath, Classification: DocChangeReadFailed})
			appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionUnsafeTarget, Path: docPath, Reason: "persisted document is now a symlink"})
		} else {
			appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionExcludedAlive, Path: docPath, Reason: "persisted document is alive but outside the current canonical manifest"})
		}
	}
	return nil
}

func prioritizeReverseManifestEntries(ctx context.Context, db *sql.DB, entries []manifestEntry, scope model.WikiCodeScope, state *overlayState) []manifestEntry {
	if len(entries) < 2 || state == nil {
		return entries
	}
	exactOwners := make(map[string]struct{})
	for _, binding := range state.bindings {
		if reverseBindingOwnerMatches(binding, scope) {
			exactOwners[normalizePublicPath(binding.DocPath)] = struct{}{}
		}
	}
	ftsOwners := make(map[string]float64)
	for _, candidate := range reverseColdCandidateOwners(ctx, db, entries, scope, state) {
		ftsOwners[candidate.path] = candidate.score
	}
	metadataOwners := make(map[string]struct{})
	for _, path := range reverseMetadataCandidateOwners(entries, state) {
		metadataOwners[path] = struct{}{}
	}

	// Assign every manifest path to exactly one ranked class. A lower class
	// cannot union with a higher class for the same path, and unknown paths stay
	// in the bounded remainder rather than being treated as new owners.
	type rankedEntry struct {
		entry manifestEntry
		class int
		score float64
	}
	ranked := make(map[string]rankedEntry, len(entries))
	for _, entry := range entries {
		path := normalizePublicPath(entry.Path)
		class := 4 // unknown/unclassified/rest
		if _, ok := exactOwners[path]; ok {
			class = 1
		} else if score, ok := ftsOwners[path]; ok {
			class = 2
			candidate, exists := ranked[path]
			if !exists || class < candidate.class {
				ranked[path] = rankedEntry{entry: entry, class: class, score: score}
			}
			continue
		} else if _, ok := metadataOwners[path]; ok {
			class = 3
		}
		candidate, exists := ranked[path]
		if !exists || class < candidate.class {
			ranked[path] = rankedEntry{entry: entry, class: class}
		}
	}
	ordered := make([]rankedEntry, 0, len(ranked))
	for _, candidate := range ranked {
		ordered = append(ordered, candidate)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].class != ordered[j].class {
			return ordered[i].class < ordered[j].class
		}
		if ordered[i].class == 2 && ordered[i].score != ordered[j].score {
			return ordered[i].score > ordered[j].score
		}
		return normalizePublicPath(ordered[i].entry.Path) < normalizePublicPath(ordered[j].entry.Path)
	})
	result := make([]manifestEntry, 0, len(ordered))
	for _, candidate := range ordered {
		result = append(result, candidate.entry)
	}
	return result
}

func reverseMetadataCandidateOwners(entries []manifestEntry, state *overlayState) []string {
	if state == nil {
		return nil
	}
	owners := make([]string, 0)
	for _, entry := range entries {
		if reverseEntryIsNewOrChanged(entry, state) {
			owners = append(owners, normalizePublicPath(entry.Path))
		}
	}
	return owners
}

func reverseEntryIsNewOrChanged(entry manifestEntry, state *overlayState) bool {
	prior, known := state.states[entry.Path]
	if !known {
		return false
	}
	return prior.Size != entry.Size || prior.MtimeNsec != entry.MtimeNsec ||
		prior.ParserVersion != model.ParserVersion ||
		prior.AuthorityConfigHash != store.AuthorityConfigDigestForProfile(state.profile, nil)
}

type reverseFTSCandidate struct {
	path  string
	score float64
}

func reverseColdCandidateOwners(ctx context.Context, db *sql.DB, entries []manifestEntry, scope model.WikiCodeScope, state *overlayState) []reverseFTSCandidate {
	if db == nil || strings.TrimSpace(scope.TargetPath) == "" || state == nil {
		return nil
	}
	query := strings.ReplaceAll(normalizePublicPath(scope.TargetPath), "/", " ")
	if query == "" {
		return nil
	}
	matches, scores, err := store.FTSSearchDocs(ctx, db, query, 64)
	if err != nil {
		return nil
	}
	manifest := make(map[string]manifestEntry, len(entries))
	for _, entry := range entries {
		manifest[normalizePublicPath(entry.Path)] = entry
	}
	owners := make([]reverseFTSCandidate, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, doc := range matches {
		path := normalizePublicPath(doc.Path)
		_, ok := manifest[path]
		if !ok || !model.CanonicalWikiAuthority(path) {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		owners = append(owners, reverseFTSCandidate{path: path, score: scores[path]})
	}
	sort.SliceStable(owners, func(i, j int) bool {
		if owners[i].score != owners[j].score {
			return owners[i].score > owners[j].score
		}
		return owners[i].path < owners[j].path
	})
	return owners
}

// reverseBindingOwnerMatches proves only a safe target-path owner candidate;
// symbol identity remains the service resolver's responsibility.
func reverseBindingOwnerMatches(binding model.DocArtifactBinding, scope model.WikiCodeScope) bool {
	if !model.CanonicalWikiAuthority(binding.DocPath) || isRetiredBindingPath(binding.DocPath) {
		return false
	}
	if binding.TargetPath != normalizePublicPath(scope.TargetPath) {
		return false
	}
	if binding.BindingStatus != "" && !strings.EqualFold(strings.TrimSpace(binding.BindingStatus), model.BindingStatusExact) {
		return false
	}
	if binding.DocLifecycle != "" && !strings.EqualFold(strings.TrimSpace(binding.DocLifecycle), model.DocLifecycleActive) {
		return false
	}
	origin := strings.TrimSpace(binding.AuthoringOrigin)
	if origin != "" && !strings.EqualFold(origin, model.AuthoringOriginCanonical) && !strings.EqualFold(origin, model.AuthoringOriginLegacy) {
		return false
	}
	return reverseBindingRelationAllowed(binding.Relation) && reverseBindingTargetKindAllowed(binding.TargetKind)
}

func reverseBindingRelationAllowed(relation string) bool {
	switch strings.ToLower(strings.TrimSpace(relation)) {
	case model.RelationImplements, model.RelationTests, model.RelationConfigures, model.RelationOperates:
		return true
	default:
		return false
	}
}

func reverseBindingTargetKindAllowed(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case model.TargetKindFile, model.TargetKindSymbol, model.TargetKindTest, model.TargetKindConfig:
		return true
	default:
		return false
	}
}

func reverseEntryNeedsRead(entry manifestEntry, state *overlayState) bool {
	prior, known := state.states[entry.Path]
	if !known || prior.Size != entry.Size || prior.MtimeNsec != entry.MtimeNsec || prior.ParserVersion != model.ParserVersion || prior.AuthorityConfigHash != store.AuthorityConfigDigestForProfile(state.profile, nil) {
		return true
	}
	return !manifestStampOld(entry.MtimeNsec)
}

func exactBlockFilter(scope model.WikiCodeScope) map[string]map[string]struct{} {
	result := make(map[string]map[string]struct{})
	for _, item := range scope.Blocks {
		if item.DocPath == "" {
			continue
		}
		if result[item.DocPath] == nil {
			result[item.DocPath] = make(map[string]struct{})
		}
		result[item.DocPath][item.BlockID] = struct{}{}
	}
	return result
}

func exactManifestEntry(root, docPath string, profile model.DocsReadProfile, matcher *workspace.IgnoreMatcher, extraRoots []string) (manifestEntry, string) {
	if docPath == "" || filepath.IsAbs(docPath) || docPath == ".." || !isMarkdownPath(docPath) {
		return manifestEntry{}, model.OmissionUnsafeTarget
	}
	absPath := filepath.Join(root, filepath.FromSlash(docPath))
	if !pathInside(root, absPath) && !isAllowedDeclaredCanonPath(root, absPath, extraRoots) {
		return manifestEntry{}, model.OmissionUnsafeTarget
	}
	info, err := os.Lstat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return manifestEntry{CanonicalManifestEntry: CanonicalManifestEntry{Path: docPath}, absolutePath: absPath}, ""
		}
		return manifestEntry{}, model.OmissionReadFailed
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() || (pathInside(root, absPath) && pathContainsSymlink(root, absPath)) {
		return manifestEntry{}, model.OmissionUnsafeTarget
	}
	if matcher != nil && matcher.ShouldIgnore(root, absPath) {
		return manifestEntry{}, model.OmissionExcludedAlive
	}
	if isExcludedCanonicalPath(root, absPath) {
		return manifestEntry{}, model.OmissionExcludedAlive
	}
	if !matchesProfileOrRoot(docPath, profile, root, absPath, extraRoots) {
		return manifestEntry{}, model.OmissionExcludedAlive
	}
	return manifestEntry{CanonicalManifestEntry: CanonicalManifestEntry{Path: docPath, Size: info.Size(), MtimeNsec: info.ModTime().UnixNano()}, absolutePath: absPath}, ""
}

func isAllowedDeclaredCanonPath(workspaceRoot, absPath string, extraRoots []string) bool {
	for _, root := range extraRoots {
		if declared, err := resolveExplicitRoot(workspaceRoot, root); err == nil && pathInside(declared, absPath) {
			return true
		}
	}
	project, err := workspace.LoadProjectFile(workspaceRoot)
	if err != nil || len(project.Canons) == 0 {
		return false
	}
	for _, declared := range project.Canons {
		one := model.ProjectFile{Canons: []model.WorkspaceCanon{declared}, CanonPolicy: project.CanonPolicy}
		canons, resolveErr := workspace.ResolveCanons(workspaceRoot, one)
		if resolveErr != nil {
			continue
		}
		for _, canon := range canons {
			if pathInside(canon.AbsRoot, absPath) {
				return true
			}
		}
	}
	return false
}

func matchesProfileOrRoot(docPath string, profile model.DocsReadProfile, workspaceRoot, absPath string, extraRoots []string) bool {
	patterns := make([]string, 0)
	for _, family := range profile.Families {
		patterns = append(patterns, family.Paths...)
	}
	patterns = append(patterns, profile.GenericDocs.Paths...)
	for _, pattern := range patterns {
		pattern = filepath.ToSlash(strings.TrimSpace(pattern))
		if pattern == "" || !safeRelativePattern(pattern) {
			continue
		}
		trimmed := strings.TrimSuffix(pattern, "/")
		if strings.HasSuffix(pattern, "/") && (docPath == trimmed || strings.HasPrefix(docPath, trimmed+"/")) {
			return true
		}
		if strings.ContainsAny(trimmed, "*?[") {
			if ok, _ := path.Match(trimmed, docPath); ok {
				return true
			}
			if ok, _ := filepath.Match(filepath.FromSlash(trimmed), filepath.FromSlash(docPath)); ok {
				return true
			}
		} else if docPath == trimmed {
			return true
		}
	}
	for _, root := range extraRoots {
		if absRoot, err := resolveExplicitRoot(workspaceRoot, root); err == nil && pathInside(absRoot, absPath) {
			return true
		}
	}
	if !pathInside(workspaceRoot, absPath) && isAllowedDeclaredCanonPath(workspaceRoot, absPath, extraRoots) {
		return true
	}
	return false
}

func processManifestDoc(ctx context.Context, root string, entry manifestEntry, state *overlayState, overlay *model.WikiCodeOverlay, blockFilter map[string]struct{}, maxBytes int64, metadataAlreadyChecked bool) error {
	if entry.Path == "" {
		return nil
	}
	prior, hasPrior := state.states[entry.Path]
	if !metadataAlreadyChecked {
		overlay.Cost.MetadataChecked++
	}
	info, statErr := os.Lstat(entry.absolutePath)
	if errors.Is(statErr, os.ErrNotExist) {
		if hasPrior || len(state.byOwner[entry.Path]) > 0 {
			appendOwnerTombstone(overlay, state, entry.Path, "disk-absent", blockFilter)
			appendChangedInput(overlay, model.WikiCodeChangedInput{Path: entry.Path, Classification: DocChangeDeleted})
		} else {
			appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionUnknownDocument, Path: entry.Path, Reason: "selected document is absent"})
		}
		return nil
	}
	if statErr != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		if shouldMaskOwner(state, entry.Path) {
			appendOwnerTombstone(overlay, state, entry.Path, model.OmissionReadFailed, blockFilter)
		}
		appendChangedInput(overlay, model.WikiCodeChangedInput{Path: entry.Path, Classification: DocChangeReadFailed})
		appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionReadFailed, Path: entry.Path, Reason: "document metadata could not be checked"})
		return nil
	}
	entry.Size = info.Size()
	entry.MtimeNsec = info.ModTime().UnixNano()
	metadataEqual := hasPrior && prior.Size == entry.Size && prior.MtimeNsec == entry.MtimeNsec
	configEqual := hasPrior && prior.ParserVersion == model.ParserVersion && prior.AuthorityConfigHash == store.AuthorityConfigDigestForProfile(state.profile, nil)
	if hasPrior && metadataEqual && configEqual && prior.ContentSHA256 != "" && manifestStampOld(entry.MtimeNsec) {
		overlay.Cost.UnchangedReused++
		return nil
	}
	if maxBytes > 0 && (overlay.Cost.BytesRead >= maxBytes || entry.Size > maxBytes-overlay.Cost.BytesRead) {
		appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionUnknownDocument, Path: entry.Path, Reason: "overlay byte bound exceeded"})
		return nil
	}
	content, readStatus, readErr := SafeReadFileContext(ctx, entry.absolutePath)
	if readErr != nil {
		var concurrentErr *model.ErrConcurrentChange
		if readStatus == ReadStatusConcurrent || errors.As(readErr, &concurrentErr) {
			if shouldMaskOwner(state, entry.Path) {
				appendOwnerTombstone(overlay, state, entry.Path, model.OmissionConcurrentChange, blockFilter)
			}
			appendChangedInput(overlay, model.WikiCodeChangedInput{Path: entry.Path, Classification: DocChangeConcurrent, Status: string(ReadStatusConcurrent)})
			appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionConcurrentChange, Path: entry.Path, Reason: "document changed during bounded read"})
		} else if errors.Is(readErr, ErrUnsafeTarget) {
			if shouldMaskOwner(state, entry.Path) {
				appendOwnerTombstone(overlay, state, entry.Path, model.OmissionUnsafeTarget, blockFilter)
			}
			appendChangedInput(overlay, model.WikiCodeChangedInput{Path: entry.Path, Classification: DocChangeReadFailed, Status: string(readStatus)})
			appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionUnsafeTarget, Path: entry.Path, Reason: "document became an unsafe link during bounded read"})
		} else {
			if shouldMaskOwner(state, entry.Path) {
				appendOwnerTombstone(overlay, state, entry.Path, model.OmissionReadFailed, blockFilter)
			}
			appendChangedInput(overlay, model.WikiCodeChangedInput{Path: entry.Path, Classification: DocChangeReadFailed, Status: string(readStatus)})
			appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionReadFailed, Path: entry.Path, Reason: "document could not be read safely"})
		}
		return nil
	}
	if maxBytes > 0 && overlay.Cost.BytesRead+int64(len(content)) > maxBytes {
		overlay.Cost.BytesRead += int64(len(content))
		if shouldMaskOwner(state, entry.Path) {
			appendOwnerTombstone(overlay, state, entry.Path, "overlay-byte-bound", blockFilter)
		}
		appendChangedInput(overlay, model.WikiCodeChangedInput{Path: entry.Path, Classification: DocChangeReadFailed, Status: string(readStatus)})
		appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionUnknownDocument, Path: entry.Path, Reason: "overlay byte bound exceeded during read"})
		return nil
	}
	overlay.Cost.BytesRead += int64(len(content))
	overlay.Cost.FilesHashed++
	contentHash := sha256Hex(content)
	if hasPrior && metadataEqual && configEqual && contentHash == prior.ContentSHA256 {
		overlay.Cost.UnchangedReused++
		return nil
	}
	if shouldMaskOwner(state, entry.Path) {
		appendOwnerTombstone(overlay, state, entry.Path, "document-replaced", blockFilter)
	}
	appendChangedInput(overlay, model.WikiCodeChangedInput{Path: entry.Path, Classification: DocChangeChanged, Status: string(readStatus), ContentHash: contentHash})
	if !hasPrior {
		overlay.ChangedInputs[len(overlay.ChangedInputs)-1].Classification = DocChangeNew
	}
	profile := state.profile
	_, _, _, _, _, bindings, warning, parseErr := docgraph.ParseSingleDoc(root, entry.Path, content, profile)
	if parseErr != nil || warning != "" {
		overlay.Cost.FilesParsed++
		appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionParseFailed, Path: entry.Path, Reason: "document parser rejected the current body"})
		return nil
	}
	overlay.Cost.FilesParsed++
	for _, binding := range bindings {
		if blockFilter != nil {
			if _, ok := blockFilter[binding.BlockID]; !ok {
				continue
			}
		}
		binding = sanitizeBinding(binding)
		if binding.DocPath != entry.Path || binding.BindingRef == "" {
			continue
		}
		if _, _, resolveErr := SafeResolveTarget(root, binding.TargetPath); resolveErr != nil {
			appendOmission(overlay, model.WikiCodeOmission{Code: model.OmissionUnsafeTarget, DocPath: entry.Path, TargetPath: binding.TargetPath, Reason: "declared target is outside the repository or crosses an unsafe link"})
			continue
		}
		// T4 lifecycle state is authoritative for an existing document. A
		// changed retired/deprecated file must not be promoted to an active
		// claim merely because a binding entry omitted its lifecycle field.
		if hasPrior && (strings.EqualFold(prior.Lifecycle, model.DocLifecycleRetired) || strings.EqualFold(prior.Lifecycle, model.DocLifecycleDeprecated)) {
			binding.DocLifecycle = prior.Lifecycle
		}
		// IndexedAt is publication telemetry and is not meaningful in an
		// ephemeral view. Zeroing it also prevents wall-clock drift in callers
		// that compare complete overlay values.
		binding.IndexedAt = 0
		overlay.Additions = append(overlay.Additions, binding)
	}
	return nil
}

func manifestStampOld(mtimeNsec int64) bool {
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

func shouldMaskOwner(state *overlayState, docPath string) bool {
	if state == nil {
		return false
	}
	_, hasPrior := state.states[docPath]
	return hasPrior || len(state.byOwner[docPath]) > 0 || !state.baseKnown
}

func sanitizeBinding(binding model.DocArtifactBinding) model.DocArtifactBinding {
	binding.DocPath = normalizePublicPath(binding.DocPath)
	binding.TargetPath = normalizePublicPath(binding.TargetPath)
	binding.BindingRef = strings.TrimSpace(binding.BindingRef)
	if binding.BindingRef == "" {
		binding.BindingRef = model.WikiCodeBindingRef(binding.DocPath, binding.BlockID, binding.DocID, binding.Relation, binding.TargetPath, binding.TargetSymbol, binding.TargetKind)
	}
	return binding
}

func sanitizeBindings(bindings []model.DocArtifactBinding) []model.DocArtifactBinding {
	result := make([]model.DocArtifactBinding, 0, len(bindings))
	for _, binding := range bindings {
		binding = sanitizeBinding(binding)
		if binding.DocPath == "" || binding.TargetPath == "" {
			continue
		}
		result = append(result, binding)
	}
	return result
}

func normalizePublicPath(value string) string {
	value = normalizeSlash(strings.TrimSpace(value))
	if value == "" || strings.ContainsRune(value, 0) || filepath.IsAbs(value) || strings.HasPrefix(value, "//") || (len(value) > 1 && value[1] == ':') || strings.HasPrefix(value, "~/") || value == "~" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(value))
}

func appendOwnerTombstone(overlay *model.WikiCodeOverlay, state *overlayState, docPath, reason string, blocks map[string]struct{}) {
	docPath = normalizePublicPath(docPath)
	if docPath == "" {
		return
	}
	if blocks != nil {
		ids := make([]string, 0, len(blocks))
		for blockID := range blocks {
			ids = append(ids, blockID)
		}
		sort.Strings(ids)
		for _, blockID := range ids {
			tombstone := model.BindingTombstone{DocPath: docPath, BlockID: blockID, Reason: reason}
			if reason == "disk-absent" {
				tombstone.Proof = reason
			}
			appendTombstone(overlay, tombstone)
		}
		return
	}
	tombstone := model.BindingTombstone{DocPath: docPath, Reason: reason}
	if reason == "disk-absent" {
		tombstone.Proof = reason
	}
	appendTombstone(overlay, tombstone)
	_ = state
}

func appendTombstone(overlay *model.WikiCodeOverlay, tombstone model.BindingTombstone) {
	if tombstone.DocPath != "" {
		tombstone.DocPath = normalizePublicPath(tombstone.DocPath)
	}
	if tombstone.BindingRef != "" {
		tombstone.BindingRef = strings.TrimSpace(tombstone.BindingRef)
	}
	for _, existing := range overlay.Tombstones {
		if existing == tombstone {
			return
		}
	}
	overlay.Tombstones = append(overlay.Tombstones, tombstone)
}

func appendChangedInput(overlay *model.WikiCodeOverlay, item model.WikiCodeChangedInput) {
	item.Path = normalizePublicPath(item.Path)
	if item.Path == "" {
		return
	}
	for index := range overlay.ChangedInputs {
		if overlay.ChangedInputs[index].Path == item.Path {
			if overlay.ChangedInputs[index].Classification == DocChangeChanged && item.Classification == DocChangeNew {
				overlay.ChangedInputs[index] = item
			}
			return
		}
	}
	overlay.ChangedInputs = append(overlay.ChangedInputs, item)
}

func normalizeOmissionCandidates(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizePublicPath(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func appendOmission(overlay *model.WikiCodeOverlay, omission model.WikiCodeOmission) {
	omission.Path = normalizePublicPath(omission.Path)
	omission.DocPath = normalizePublicPath(omission.DocPath)
	omission.TargetPath = normalizePublicPath(omission.TargetPath)
	omission.Candidates = normalizeOmissionCandidates(omission.Candidates)
	for _, existing := range overlay.Omissions {
		if existing.Code == omission.Code && existing.Path == omission.Path && existing.DocPath == omission.DocPath && existing.TargetPath == omission.TargetPath && existing.Reason == omission.Reason && strings.Join(existing.Candidates, "\x00") == strings.Join(omission.Candidates, "\x00") {
			return
		}
	}
	overlay.Omissions = append(overlay.Omissions, omission)
}

func hasChangedInput(overlay *model.WikiCodeOverlay, path string) bool {
	for _, item := range overlay.ChangedInputs {
		if item.Path == path {
			return true
		}
	}
	return false
}

func manifestContains(entries []manifestEntry, docPath string) bool {
	for _, entry := range entries {
		if entry.Path == docPath {
			return true
		}
	}
	return false
}

func overlayBounds(kind model.WikiCodeScopeKind, requestedDocs int, requestedBytes int64) (int, int64) {
	maxDocs := requestedDocs
	if maxDocs == 0 {
		if kind == model.ScopeReverseCode {
			maxDocs = DefaultReverseMaxDocuments
		} else {
			maxDocs = DefaultExactMaxDocuments
		}
	}
	maxBytes := requestedBytes
	if maxBytes == 0 {
		maxBytes = DefaultOverlayMaxBytes
	}
	return maxDocs, maxBytes
}

func finalizeOverlay(overlay *model.WikiCodeOverlay, state overlayState) {
	if overlay == nil {
		return
	}
	sort.Slice(overlay.ChangedInputs, func(i, j int) bool { return overlay.ChangedInputs[i].Path < overlay.ChangedInputs[j].Path })
	sort.Slice(overlay.Additions, func(i, j int) bool {
		a, b := overlay.Additions[i], overlay.Additions[j]
		if a.DocPath != b.DocPath {
			return a.DocPath < b.DocPath
		}
		if a.BlockID != b.BlockID {
			return a.BlockID < b.BlockID
		}
		if a.Ordinal != b.Ordinal {
			return a.Ordinal < b.Ordinal
		}
		return a.BindingRef < b.BindingRef
	})
	sort.Slice(overlay.Tombstones, func(i, j int) bool {
		a, b := overlay.Tombstones[i], overlay.Tombstones[j]
		if a.DocPath != b.DocPath {
			return a.DocPath < b.DocPath
		}
		if a.BlockID != b.BlockID {
			return a.BlockID < b.BlockID
		}
		if a.BindingRef != b.BindingRef {
			return a.BindingRef < b.BindingRef
		}
		if a.Reason != b.Reason {
			return a.Reason < b.Reason
		}
		return a.Proof < b.Proof
	})
	for index := range overlay.Omissions {
		overlay.Omissions[index].Candidates = normalizeOmissionCandidates(overlay.Omissions[index].Candidates)
	}
	sort.Slice(overlay.Omissions, func(i, j int) bool {
		a, b := overlay.Omissions[i], overlay.Omissions[j]
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.DocPath != b.DocPath {
			return a.DocPath < b.DocPath
		}
		if a.TargetPath != b.TargetPath {
			return a.TargetPath < b.TargetPath
		}
		return a.Reason < b.Reason
	})
	concurrent := false
	for _, omission := range overlay.Omissions {
		if omission.Code == model.OmissionConcurrentChange {
			concurrent = true
			break
		}
	}
	hasOverlay := len(overlay.Additions) > 0 || len(overlay.Tombstones) > 0
	switch {
	case concurrent:
		overlay.Status = model.OverlayStatusConcurrentChange
	case hasOverlay:
		overlay.Status = model.OverlayStatusOverlay
	case len(overlay.Omissions) > 0:
		overlay.Status = model.OverlayStatusStale
	case !state.dbAvailable:
		overlay.Status = model.OverlayStatusUnknown
	default:
		overlay.Status = model.OverlayStatusCurrent
	}
	if state.authorityReady {
		overlay.Freshness.Authority = model.FreshnessCurrent
	}
	if state.manifestReady || len(overlay.ChangedInputs) > 0 {
		overlay.Freshness.DocsManifest = model.FreshnessCurrent
	}
	for _, omission := range overlay.Omissions {
		if omission.Path == "" {
			continue
		}
		switch omission.Code {
		case model.OmissionUnknownDocument, model.OmissionReadFailed, model.OmissionExcludedAlive, model.OmissionParseFailed:
			overlay.Freshness.DocsManifest = model.FreshnessStale
		}
	}
	if hasOverlay {
		overlay.Freshness.Bindings = model.FreshnessOverlay
	} else if state.dbAvailable {
		overlay.Freshness.Bindings = model.FreshnessCurrent
	}
	if state.dbAvailable {
		overlay.Freshness.Catalog = model.FreshnessCurrent
	}
	if hasOverlay {
		overlay.Freshness.Graph = model.FreshnessStale
	}
	if concurrent {
		overlay.Freshness.DocsManifest = model.FreshnessConcurrentChange
		overlay.Freshness.Bindings = model.FreshnessConcurrentChange
	}
	overlay.Freshness.Domains = map[string]string{
		model.FreshnessDomainDocsManifest: overlay.Freshness.DocsManifest,
		model.FreshnessDomainBindings:     overlay.Freshness.Bindings,
		model.FreshnessDomainCatalog:      overlay.Freshness.Catalog,
		model.FreshnessDomainGraph:        overlay.Freshness.Graph,
		model.FreshnessDomainAuthority:    overlay.Freshness.Authority,
	}
	overlay.Digest = overlayDigest(*overlay)
}

func overlayDigest(overlay model.WikiCodeOverlay) string {
	type digestInput struct {
		Version        string
		ParserVersion  string
		Resolver       string
		BaseGeneration string
		Mode           string
		Scope          model.WikiCodeScope
		Freshness      model.WikiCodeFreshness
		ChangedInputs  []model.WikiCodeChangedInput
		Additions      []model.DocArtifactBinding
		Tombstones     []model.BindingTombstone
		Omissions      []model.WikiCodeOmission
	}
	scope := overlay.Scope
	scope.DocIDs = normalizeStringSet(scope.DocIDs)
	scope.DocPaths = normalizeDocPathSet(scope.DocPaths)
	scope.Blocks = append([]model.WikiCodeBlockScope(nil), scope.Blocks...)
	for index := range scope.Blocks {
		scope.Blocks[index].DocPath = normalizePublicPath(scope.Blocks[index].DocPath)
		scope.Blocks[index].DocID = strings.TrimSpace(scope.Blocks[index].DocID)
		scope.Blocks[index].BlockID = strings.TrimSpace(scope.Blocks[index].BlockID)
	}
	sort.Slice(scope.Blocks, func(i, j int) bool {
		a, b := scope.Blocks[i], scope.Blocks[j]
		if a.DocPath != b.DocPath {
			return a.DocPath < b.DocPath
		}
		if a.DocID != b.DocID {
			return a.DocID < b.DocID
		}
		return a.BlockID < b.BlockID
	})
	inputs := append([]model.WikiCodeChangedInput(nil), overlay.ChangedInputs...)
	sort.Slice(inputs, func(i, j int) bool {
		if inputs[i].Path != inputs[j].Path {
			return inputs[i].Path < inputs[j].Path
		}
		return inputs[i].Classification < inputs[j].Classification
	})
	for index := range inputs {
		inputs[index].Path = normalizePublicPath(inputs[index].Path)
		inputs[index].Status = ""
	}
	sort.Slice(inputs, func(i, j int) bool {
		if inputs[i].Path != inputs[j].Path {
			return inputs[i].Path < inputs[j].Path
		}
		return inputs[i].Classification < inputs[j].Classification
	})
	additions := append([]model.DocArtifactBinding(nil), overlay.Additions...)
	sort.Slice(additions, func(i, j int) bool {
		a, b := additions[i], additions[j]
		if a.DocPath != b.DocPath {
			return a.DocPath < b.DocPath
		}
		if a.BlockID != b.BlockID {
			return a.BlockID < b.BlockID
		}
		if a.Ordinal != b.Ordinal {
			return a.Ordinal < b.Ordinal
		}
		return a.BindingRef < b.BindingRef
	})
	for index := range additions {
		additions[index] = sanitizeBinding(additions[index])
		additions[index].IndexedAt = 0
	}
	sort.Slice(additions, func(i, j int) bool {
		a, b := additions[i], additions[j]
		if a.DocPath != b.DocPath {
			return a.DocPath < b.DocPath
		}
		if a.BlockID != b.BlockID {
			return a.BlockID < b.BlockID
		}
		if a.Ordinal != b.Ordinal {
			return a.Ordinal < b.Ordinal
		}
		return a.BindingRef < b.BindingRef
	})
	tombstones := append([]model.BindingTombstone(nil), overlay.Tombstones...)
	for index := range tombstones {
		tombstones[index].DocPath = normalizePublicPath(tombstones[index].DocPath)
		tombstones[index].BindingRef = strings.TrimSpace(tombstones[index].BindingRef)
	}
	sort.Slice(tombstones, func(i, j int) bool {
		a, b := tombstones[i], tombstones[j]
		if a.DocPath != b.DocPath {
			return a.DocPath < b.DocPath
		}
		if a.BlockID != b.BlockID {
			return a.BlockID < b.BlockID
		}
		if a.BindingRef != b.BindingRef {
			return a.BindingRef < b.BindingRef
		}
		if a.Reason != b.Reason {
			return a.Reason < b.Reason
		}
		return a.Proof < b.Proof
	})
	omissions := append([]model.WikiCodeOmission(nil), overlay.Omissions...)
	for index := range omissions {
		omissions[index].Path = normalizePublicPath(omissions[index].Path)
		omissions[index].DocPath = normalizePublicPath(omissions[index].DocPath)
		omissions[index].TargetPath = normalizePublicPath(omissions[index].TargetPath)
		omissions[index].Candidates = normalizeOmissionCandidates(omissions[index].Candidates)
	}
	sort.Slice(omissions, func(i, j int) bool {
		a, b := omissions[i], omissions[j]
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.DocPath != b.DocPath {
			return a.DocPath < b.DocPath
		}
		if a.TargetPath != b.TargetPath {
			return a.TargetPath < b.TargetPath
		}
		return a.Reason < b.Reason
	})
	freshness := overlay.Freshness
	if freshness.Domains != nil {
		freshness.Domains = map[string]string{}
		for key, value := range overlay.Freshness.Domains {
			freshness.Domains[key] = value
		}
	}
	payload := digestInput{
		Version:        model.WikiCodeOverlayVersion,
		ParserVersion:  model.ParserVersion,
		Resolver:       "wiki-code-overlay-resolver-v1",
		BaseGeneration: overlay.BaseGeneration,
		Mode:           overlay.Mode,
		Scope:          scope,
		Freshness:      freshness,
		ChangedInputs:  inputs,
		Additions:      additions,
		Tombstones:     tombstones,
		Omissions:      omissions,
	}
	encoded, _ := json.Marshal(payload)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// OverlayDigest exposes the stable digest helper for tests and consumers that
// persist only a sanitized closure packet.
func OverlayDigest(overlay model.WikiCodeOverlay) string { return overlayDigest(overlay) }

// EffectiveBindings merges a persisted binding snapshot with an overlay. By
// default planned and retired declarations are excluded from navigable claims.
func EffectiveBindings(base []model.DocArtifactBinding, overlay model.WikiCodeOverlay) []model.DocArtifactBinding {
	return EffectiveBindingsWithOptions(base, overlay, false, false)
}

// MergeBindings is a descriptive alias for the default active binding view.
func MergeBindings(base []model.DocArtifactBinding, overlay model.WikiCodeOverlay) []model.DocArtifactBinding {
	return EffectiveBindings(base, overlay)
}

func EffectiveBindingsWithOptions(base []model.DocArtifactBinding, overlay model.WikiCodeOverlay, includeRetired, includePlanned bool) []model.DocArtifactBinding {
	merged := make(map[string]model.DocArtifactBinding)
	for _, binding := range base {
		binding = sanitizeBinding(binding)
		if binding.BindingRef == "" || isExcludedBindingDoc(binding.DocPath) || !validRepoRelativeTarget(binding.TargetPath) {
			continue
		}
		merged[binding.BindingRef] = binding
	}
	for _, tombstone := range overlay.Tombstones {
		for ref, binding := range merged {
			if tombstone.BindingRef != "" && ref == tombstone.BindingRef {
				delete(merged, ref)
				continue
			}
			if tombstone.DocPath != "" && binding.DocPath == tombstone.DocPath && (tombstone.BlockID == "" || binding.BlockID == tombstone.BlockID) {
				delete(merged, ref)
			}
		}
	}
	for _, binding := range overlay.Additions {
		binding = sanitizeBinding(binding)
		if binding.BindingRef != "" && !isExcludedBindingDoc(binding.DocPath) && validRepoRelativeTarget(binding.TargetPath) {
			merged[binding.BindingRef] = binding
		}
	}
	result := make([]model.DocArtifactBinding, 0, len(merged))
	for _, binding := range merged {
		if !includePlanned && strings.EqualFold(binding.BindingStatus, model.BindingStatusPlanned) {
			continue
		}
		if !includeRetired && (strings.EqualFold(binding.DocLifecycle, model.DocLifecycleRetired) || isRetiredBindingPath(binding.DocPath)) {
			continue
		}
		result = append(result, binding)
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.DocPath != b.DocPath {
			return a.DocPath < b.DocPath
		}
		if a.BlockID != b.BlockID {
			return a.BlockID < b.BlockID
		}
		if a.Ordinal != b.Ordinal {
			return a.Ordinal < b.Ordinal
		}
		return a.BindingRef < b.BindingRef
	})
	return result
}

func isRetiredBindingPath(docPath string) bool {
	normalized := strings.ToLower(normalizePublicPath(docPath))
	return normalized == "_retired" || strings.HasPrefix(normalized, "_retired/") || strings.Contains(normalized, "/_retired/")
}

func isExcludedBindingDoc(docPath string) bool {
	normalized := strings.ToLower(normalizePublicPath(docPath))
	return normalized == ".docs/raw" || normalized == ".docs/auditoria" || normalized == ".docs/temp" || strings.HasPrefix(normalized, ".docs/raw/") || strings.HasPrefix(normalized, ".docs/auditoria/") || strings.HasPrefix(normalized, ".docs/temp/")
}

// BaseBindings returns the sanitized persisted rows captured by an overlay.
// It is intentionally a copy so callers cannot mutate the request snapshot.
func BaseBindings(overlay model.WikiCodeOverlay, db *sql.DB) ([]model.DocArtifactBinding, error) {
	if db == nil {
		return nil, fmt.Errorf("read-only binding snapshot is required")
	}
	bindings, err := readAllBindings(context.Background(), db)
	if err != nil {
		return nil, err
	}
	return sanitizeBindings(bindings), nil
}
