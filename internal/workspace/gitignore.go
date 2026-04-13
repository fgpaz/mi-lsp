package workspace

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type IgnoreMatcher struct {
	patterns []string
}

func DefaultIgnorePatterns() []string {
	return []string{
		".claude/",
		".git/",
		".idea/",
		".mi-lsp/",
		".next/",
		".worktrees/",
		"bin/",
		"dist/",
		"node_modules/",
		"obj/",
	}
}

func LoadIgnoreMatcher(root string, extraPatterns []string) (*IgnoreMatcher, error) {
	patterns := append([]string{}, DefaultIgnorePatterns()...)
	patterns = append(patterns, loadPatternsFromFile(filepath.Join(root, ".gitignore"))...)
	patterns = append(patterns, loadPatternsFromFile(filepath.Join(root, ".milspignore"))...)
	patterns = append(patterns, extraPatterns...)
	return &IgnoreMatcher{patterns: dedupePatterns(patterns)}, nil
}

func (m *IgnoreMatcher) ShouldIgnore(root, path string) bool {
	if m == nil {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	normalized := filepath.ToSlash(rel)
	if normalized == "." {
		return false
	}
	for _, rawPattern := range m.patterns {
		pattern := normalizeIgnorePattern(rawPattern)
		if pattern == "" || strings.HasPrefix(pattern, "!") {
			continue
		}
		if ignorePatternMatches(normalized, pattern) {
			return true
		}
	}
	return false
}

func loadPatternsFromFile(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	patterns := make([]string, 0, 16)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

func dedupePatterns(items []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(items))
	for _, item := range items {
		normalized := normalizeIgnorePattern(item)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func normalizeIgnorePattern(pattern string) string {
	normalized := strings.TrimSpace(filepath.ToSlash(pattern))
	normalized = strings.TrimPrefix(normalized, "./")
	normalized = strings.TrimPrefix(normalized, "/")
	return normalized
}

func ignorePatternMatches(path string, pattern string) bool {
	dirPattern := strings.TrimSuffix(pattern, "/")
	if dirPattern == "" {
		return false
	}

	if !strings.ContainsAny(dirPattern, "*?[]") {
		if path == dirPattern || strings.HasPrefix(path, dirPattern+"/") {
			return true
		}
		return hasPathSegment(path, dirPattern)
	}

	if strings.HasPrefix(dirPattern, "**/") {
		needle := strings.Trim(strings.TrimPrefix(dirPattern, "**/"), "/")
		return hasPathSegment(path, needle)
	}

	if ok, _ := filepath.Match(dirPattern, path); ok {
		return true
	}
	if ok, _ := filepath.Match(pattern, path); ok {
		return true
	}
	return false
}

func hasPathSegment(path string, segment string) bool {
	needle := "/" + strings.Trim(segment, "/") + "/"
	haystack := "/" + strings.Trim(path, "/") + "/"
	return strings.Contains(haystack, needle)
}
