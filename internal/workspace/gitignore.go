package workspace

import (
	"bufio"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"strings"
)

type IgnoreMatcher struct {
	rules []ignoreRule
}

type ignoreRule struct {
	pattern string
	negated bool
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
	rawPatterns := append([]string{}, DefaultIgnorePatterns()...)
	rawPatterns = append(rawPatterns, loadPatternsFromFile(filepath.Join(root, ".gitignore"))...)
	rawPatterns = append(rawPatterns, loadPatternsFromFile(filepath.Join(root, ".milspignore"))...)
	rawPatterns = append(rawPatterns, extraPatterns...)

	rules := make([]ignoreRule, 0, len(rawPatterns))
	for _, raw := range rawPatterns {
		rule, ok := normalizeIgnoreRule(raw)
		if !ok {
			continue
		}
		rules = append(rules, rule)
	}
	return &IgnoreMatcher{rules: rules}, nil
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
	ignored := false
	for _, rule := range m.rules {
		if ignorePatternMatches(normalized, rule.pattern) {
			ignored = !rule.negated
		}
	}
	return ignored
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

func normalizeIgnoreRule(pattern string) (ignoreRule, bool) {
	normalized := strings.TrimSpace(filepath.ToSlash(pattern))
	if normalized == "" {
		return ignoreRule{}, false
	}
	rule := ignoreRule{negated: strings.HasPrefix(normalized, "!")}
	if rule.negated {
		normalized = strings.TrimSpace(strings.TrimPrefix(normalized, "!"))
	}
	normalized = strings.TrimPrefix(normalized, "./")
	normalized = strings.TrimPrefix(normalized, "/")
	normalized = strings.TrimSpace(normalized)
	if normalized == "" {
		return ignoreRule{}, false
	}
	rule.pattern = normalized
	return rule, true
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

	for _, candidate := range pathWithAncestors(path) {
		if globPatternMatches(candidate, dirPattern) {
			return true
		}
	}
	if !strings.Contains(dirPattern, "/") {
		return hasSegmentGlob(path, dirPattern)
	}
	return false
}

func hasPathSegment(path string, segment string) bool {
	needle := "/" + strings.Trim(segment, "/") + "/"
	haystack := "/" + strings.Trim(path, "/") + "/"
	return strings.Contains(haystack, needle)
}

func pathWithAncestors(path string) []string {
	normalized := strings.Trim(path, "/")
	if normalized == "" {
		return nil
	}
	items := []string{normalized}
	current := normalized
	for {
		idx := strings.LastIndex(current, "/")
		if idx < 0 {
			break
		}
		current = current[:idx]
		if current == "" {
			break
		}
		items = append(items, current)
	}
	return items
}

func hasSegmentGlob(path string, pattern string) bool {
	for _, segment := range strings.Split(strings.Trim(path, "/"), "/") {
		if ok, _ := pathpkg.Match(pattern, segment); ok {
			return true
		}
	}
	return false
}

func globPatternMatches(path string, pattern string) bool {
	if strings.Contains(pattern, "**") {
		return doubleStarPatternMatches(path, pattern)
	}
	ok, err := pathpkg.Match(pattern, path)
	return err == nil && ok
}

func doubleStarPatternMatches(path string, pattern string) bool {
	regexPattern := doubleStarPatternToRegexp(pattern)
	matched, err := regexp.MatchString(regexPattern, path)
	return err == nil && matched
}

func doubleStarPatternToRegexp(pattern string) string {
	var builder strings.Builder
	builder.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				builder.WriteString(".*")
				i++
				continue
			}
			builder.WriteString("[^/]*")
		case '?':
			builder.WriteString("[^/]")
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '\\', '[', ']':
			builder.WriteByte('\\')
			builder.WriteByte(ch)
		default:
			builder.WriteByte(ch)
		}
	}
	builder.WriteString("$")
	return builder.String()
}
