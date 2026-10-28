package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// searchWorktreeOverlay searches the base tree plus only the physical delta.
// Changed paths mask base hits even if the base checkout itself has local edits.
func searchWorktreeOverlay(ctx context.Context, root, searchRoot string, project model.ProjectFile, pattern string, regex bool, limit int, diagnostics *searchPatternDiagnostics) ([]map[string]any, bool, error) {
	overlay, linked, err := workspace.TryResolveWorktreeOverlay(root)
	if err != nil || !linked {
		return nil, linked, err
	}
	baseSearchRoot := overlay.BaseRoot
	if searchRoot != root {
		rel, relErr := filepath.Rel(root, searchRoot)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return nil, true, fmt.Errorf("worktree search scope escapes physical root")
		}
		baseSearchRoot = filepath.Join(overlay.BaseRoot, rel)
	}
	changed := make(map[string]struct{}, len(overlay.Changed)+len(overlay.Deleted))
	ignorePolicyChanged := false
	for _, path := range overlay.Changed {
		changed[filepath.ToSlash(path)] = struct{}{}
		base := filepath.Base(filepath.FromSlash(path))
		if base == ".gitignore" || base == ".ignore" || base == ".rgignore" {
			ignorePolicyChanged = true
		}
	}
	for _, path := range overlay.Deleted {
		changed[filepath.ToSlash(path)] = struct{}{}
		base := filepath.Base(filepath.FromSlash(path))
		if base == ".gitignore" || base == ".ignore" || base == ".rgignore" {
			ignorePolicyChanged = true
		}
	}
	if ignorePolicyChanged {
		items, err := searchPatternScopedWithDiagnostics(ctx, overlay.Root, searchRoot, project, pattern, regex, limit, diagnostics)
		return items, true, err
	}
	baseItems, err := searchPatternScopedWithDiagnostics(ctx, overlay.BaseRoot, baseSearchRoot, project, pattern, regex, limit+len(changed)*8, diagnostics)
	if err != nil {
		return nil, true, err
	}
	items := make([]map[string]any, 0, min(limit, len(baseItems)+len(overlay.Changed)))
	for _, item := range baseItems {
		path, _ := item["file"].(string)
		if _, masked := changed[filepath.ToSlash(path)]; !masked {
			items = append(items, item)
		}
	}
	rootReal, err := filepath.EvalSymlinks(overlay.Root)
	if err != nil {
		return nil, true, fmt.Errorf("resolve physical worktree root: %w", err)
	}
	deltaPaths := make([]string, 0, len(overlay.Changed))
	for _, rel := range overlay.Changed {
		if _, deleted := containsPath(overlay.Deleted, rel); deleted {
			continue
		}
		physical := filepath.Join(overlay.Root, filepath.FromSlash(rel))
		physicalReal, evalErr := filepath.EvalSymlinks(physical)
		if evalErr != nil {
			continue
		} // removed/renamed-away or untracked only in base
		relReal, relErr := filepath.Rel(rootReal, physicalReal)
		if relErr != nil || relReal == ".." || strings.HasPrefix(relReal, ".."+string(filepath.Separator)) || filepath.IsAbs(relReal) {
			continue
		}
		info, statErr := os.Stat(physicalReal)
		if statErr != nil || !info.Mode().IsRegular() {
			continue
		}
		if searchRoot != root {
			pathRel, pathErr := filepath.Rel(searchRoot, physical)
			if pathErr != nil || pathRel == ".." || strings.HasPrefix(pathRel, ".."+string(filepath.Separator)) {
				continue
			}
		}
		deltaPaths = append(deltaPaths, physicalReal)
	}
	deltaItems, searchErr := searchPatternPathsWithDiagnostics(ctx, overlay.Root, deltaPaths, project, pattern, regex, limit, diagnostics)
	if searchErr != nil {
		return nil, true, searchErr
	}
	items = append(items, deltaItems...)
	return mergeOverlaySearchItems(items, limit), true, nil
}

func mergeOverlaySearchItems(items []map[string]any, limit int) []map[string]any {
	if limit <= 0 {
		limit = DefaultConfig().DefaultSearchLimit
	}
	type located struct {
		item map[string]any
		file string
		line int
	}
	locatedItems := make([]located, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		file, _ := item["file"].(string)
		line, _ := item["line"].(int)
		key := fmt.Sprintf("%s\x00%d\x00%v", file, line, item["text"])
		if seen[key] {
			continue
		}
		seen[key] = true
		locatedItems = append(locatedItems, located{item: item, file: file, line: line})
	}
	sort.Slice(locatedItems, func(i, j int) bool {
		if locatedItems[i].file != locatedItems[j].file {
			return locatedItems[i].file < locatedItems[j].file
		}
		return locatedItems[i].line < locatedItems[j].line
	})
	if len(locatedItems) > limit {
		locatedItems = locatedItems[:limit]
	}
	out := make([]map[string]any, 0, len(locatedItems))
	for _, value := range locatedItems {
		out = append(out, value.item)
	}
	return out
}

func containsPath(paths []string, target string) (int, bool) {
	target = filepath.ToSlash(filepath.Clean(filepath.FromSlash(target)))
	for i, path := range paths {
		if filepath.ToSlash(filepath.Clean(filepath.FromSlash(path))) == target {
			return i, true
		}
	}
	return 0, false
}
