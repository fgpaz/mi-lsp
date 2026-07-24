package service

import (
	"path/filepath"
	"sort"
	"strings"
)

// harnessUsefulnessScore ranks graph/code evidence for harness consumption.
// Higher is better. Anti-noise rules demote fixtures, generated paths, vendored
// code, and ultra-generic hubs unless they are the focus selector/path.
func harnessUsefulnessScore(path, kind, name string, confidence float64, focusPaths []string, focusSymbols []string, isHub bool) float64 {
	path = filepath.ToSlash(strings.TrimSpace(path))
	kind = strings.ToLower(strings.TrimSpace(kind))
	name = strings.TrimSpace(name)
	score := confidence
	if score <= 0 {
		score = 0.5
	}

	lowerPath := strings.ToLower(path)
	lowerName := strings.ToLower(name)

	// Strong boost for focus matches.
	for _, focus := range focusPaths {
		focus = filepath.ToSlash(strings.TrimSpace(focus))
		if focus == "" {
			continue
		}
		if strings.EqualFold(path, focus) || strings.HasPrefix(lowerPath, strings.ToLower(focus)) {
			score += 2.0
			break
		}
	}
	for _, focus := range focusSymbols {
		focus = strings.TrimSpace(focus)
		if focus == "" {
			continue
		}
		if strings.EqualFold(name, focus) || strings.Contains(lowerName, strings.ToLower(focus)) {
			score += 1.5
			break
		}
	}

	// Prefer implementation code over docs/tests/fixtures by default.
	switch {
	case strings.Contains(lowerPath, "/vendor/") || strings.Contains(lowerPath, "/node_modules/") || strings.HasPrefix(lowerPath, "vendor/"):
		score -= 2.5
	case strings.Contains(lowerPath, "/generated/") || strings.Contains(lowerPath, ".gen.") || strings.HasSuffix(lowerPath, "_generated.go"):
		score -= 2.0
	case strings.Contains(lowerPath, "/testdata/") || strings.Contains(lowerPath, "/fixtures/") || strings.Contains(lowerPath, "/mocks/"):
		score -= 1.5
	case strings.HasSuffix(lowerPath, "_test.go") || strings.Contains(lowerPath, ".test.") || strings.Contains(lowerPath, ".spec."):
		score -= 0.8
	case strings.HasPrefix(lowerPath, ".docs/") || strings.Contains(lowerPath, "/docs/"):
		score -= 0.4
	case strings.HasPrefix(lowerPath, "internal/") || strings.HasPrefix(lowerPath, "cmd/") || strings.HasPrefix(lowerPath, "src/"):
		score += 0.6
	}

	switch kind {
	case "function", "method", "class", "type", "interface", "struct":
		score += 0.3
	case "test", "doc", "file":
		score -= 0.2
	}

	// God-nodes / hubs are useful for warnings, not for default reads.
	if isHub {
		score -= 1.2
	}

	// Tiny utility names are noisy unless focused.
	if len(name) > 0 && len(name) <= 3 && !containsFold(focusSymbols, name) {
		score -= 0.5
	}
	return score
}

type rankedAffected struct {
	item  AffectedItem
	score float64
}

func rankAffectedForHarness(items []AffectedItem, focusPaths []string, hubPaths map[string]bool) []AffectedItem {
	if len(items) == 0 {
		return items
	}
	ranked := make([]rankedAffected, 0, len(items))
	for _, item := range items {
		isHub := hubPaths[filepath.ToSlash(item.Path)]
		score := harnessUsefulnessScore(item.Path, item.Kind, filepath.Base(item.Path), item.Confidence, focusPaths, nil, isHub)
		// Keep tests/docs if explicitly requested via kind priority when confidence high.
		if item.Kind == "test" && item.Confidence >= 0.8 {
			score += 0.4
		}
		ranked = append(ranked, rankedAffected{item: item, score: score})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		if ranked[i].item.Confidence != ranked[j].item.Confidence {
			return ranked[i].item.Confidence > ranked[j].item.Confidence
		}
		return ranked[i].item.Path < ranked[j].item.Path
	})
	out := make([]AffectedItem, len(ranked))
	for i, entry := range ranked {
		out[i] = entry.item
	}
	return out
}

func rankPathsForHarness(paths []string, focusPaths []string, hubPaths map[string]bool) []string {
	type rankedPath struct {
		path  string
		score float64
	}
	items := make([]rankedPath, 0, len(paths))
	for _, path := range paths {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path == "" {
			continue
		}
		items = append(items, rankedPath{
			path:  path,
			score: harnessUsefulnessScore(path, "file", filepath.Base(path), 0.5, focusPaths, nil, hubPaths[path]),
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		return items[i].path < items[j].path
	})
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.path)
	}
	return out
}

func isNoisePath(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	switch {
	case strings.HasPrefix(lower, "vendor/") || strings.Contains(lower, "/vendor/"):
		return true
	case strings.HasPrefix(lower, "node_modules/") || strings.Contains(lower, "/node_modules/"):
		return true
	case strings.Contains(lower, "/testdata/") || strings.HasPrefix(lower, "testdata/") || strings.Contains(lower, "/fixtures/") || strings.HasPrefix(lower, "fixtures/"):
		return true
	case strings.Contains(lower, "/generated/") || strings.HasPrefix(lower, "generated/"):
		return true
	default:
		return false
	}
}
