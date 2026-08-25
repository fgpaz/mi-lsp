package model

import (
	"fmt"
	"path/filepath"
	"strings"
)

var defaultWikiMapRoots = []string{"wiki/", "bibliotecas/"}

// IsWikiMapEnabled defaults to true when the optional section is absent.
func (p *DocsReadProfile) IsWikiMapEnabled() bool {
	if p == nil || p.WikiMap == nil || p.WikiMap.Enabled == nil {
		return true
	}
	return *p.WikiMap.Enabled
}

// WikiMapRoots returns safe configured roots. Invalid roots are excluded by
// LoadProfile; this method remains defensive for callers that build profiles.
func (p *DocsReadProfile) WikiMapRoots() []string {
	if p == nil || !p.IsWikiMapEnabled() || p.WikiMap == nil {
		return nil
	}
	roots, _ := SafeRoots(p.WikiMap.Roots)
	return roots
}

// EffectiveWikiMapRoots returns configured roots or the locked knowledge-wiki
// defaults. Roots are canonical relative directory prefixes ending in slash.
func (p *DocsReadProfile) EffectiveWikiMapRoots() []string {
	if p == nil || !p.IsWikiMapEnabled() {
		return nil
	}
	roots := append([]string(nil), defaultWikiMapRoots...)
	for _, root := range p.WikiMapRoots() {
		if !containsEquivalentRoot(roots, root) {
			roots = append(roots, root)
		}
	}
	return roots
}

func (p *DocsReadProfile) HasCustomHubs() bool {
	return p != nil && p.IsWikiMapEnabled() && p.WikiMap != nil && len(p.WikiMap.Hubs) > 0
}

// ClassifyWikiMapHubClassified returns the first matching declared custom hub.
func (p *DocsReadProfile) ClassifyWikiMapHubClassified(path string) string {
	if !p.HasCustomHubs() {
		return ""
	}
	for _, hub := range p.WikiMap.Hubs {
		for _, pattern := range hub.Patterns {
			if matchWikiMapPattern(path, pattern) {
				return hub.ID
			}
		}
	}
	return ""
}

// SafeRoots validates workspace-relative directory roots without exposing the
// rejected value in warnings.
func SafeRoots(roots []string) (safe []string, warnings []string) {
	for i, raw := range roots {
		value := normalizeWikiMapValue(raw)
		switch {
		case value == "":
			warnings = append(warnings, wikiMapWarning("root_empty", i))
		case wikiMapAbsolute(value):
			warnings = append(warnings, wikiMapWarning("root_absolute", i))
		case strings.ContainsAny(value, "*?["):
			warnings = append(warnings, wikiMapWarning("root_glob", i))
		case wikiMapUnsafeSegments(value):
			warnings = append(warnings, wikiMapWarning("root_unsafe", i))
		default:
			value = strings.TrimSuffix(value, "/") + "/"
			if !containsString(safe, value) {
				safe = append(safe, value)
			}
		}
	}
	return safe, warnings
}

// SafePatterns validates workspace-relative glob patterns. A trailing /**
// means that every descendant of the matched segment prefix is included.
func SafePatterns(patterns []string) (safe []string, warnings []string) {
	for i, raw := range patterns {
		value := normalizeWikiMapValue(raw)
		switch {
		case value == "":
			warnings = append(warnings, wikiMapWarning("pattern_empty", i))
		case wikiMapAbsolute(value):
			warnings = append(warnings, wikiMapWarning("pattern_absolute", i))
		case wikiMapUnsafeSegments(strings.TrimSuffix(value, "/**")):
			warnings = append(warnings, wikiMapWarning("pattern_unsafe", i))
		case !validWikiMapPattern(value):
			warnings = append(warnings, wikiMapWarning("pattern_malformed", i))
		default:
			safe = append(safe, value)
		}
	}
	return safe, warnings
}

func AppendWikiMapRoots(paths []string, profile *DocsReadProfile) []string {
	if profile == nil || !profile.IsWikiMapEnabled() {
		return paths
	}
	for _, root := range profile.EffectiveWikiMapRoots() {
		if !containsEquivalentRoot(paths, root) {
			paths = append(paths, root)
		}
	}
	return paths
}

func matchWikiMapPattern(path, pattern string) bool {
	path = strings.TrimSuffix(normalizeWikiMapValue(path), "/")
	pattern = normalizeWikiMapValue(pattern)
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		pathParts := strings.Split(path, "/")
		patternParts := strings.Split(prefix, "/")
		if len(pathParts) < len(patternParts) {
			return false
		}
		for i, part := range patternParts {
			matched, err := filepath.Match(part, pathParts[i])
			if err != nil || !matched {
				return false
			}
		}
		return true
	}
	matched, err := filepath.Match(filepath.FromSlash(pattern), filepath.FromSlash(path))
	return err == nil && matched
}

func validWikiMapPattern(pattern string) bool {
	base := strings.TrimSuffix(pattern, "/**")
	if base == "" {
		return false
	}
	for _, segment := range strings.Split(base, "/") {
		if _, err := filepath.Match(segment, segment); err != nil {
			return false
		}
	}
	return true
}

func normalizeWikiMapValue(value string) string {
	value = filepath.ToSlash(strings.TrimSpace(value))
	for strings.HasPrefix(value, "./") {
		value = strings.TrimPrefix(value, "./")
	}
	return value
}

func wikiMapAbsolute(value string) bool {
	return filepath.IsAbs(filepath.FromSlash(value)) || strings.HasPrefix(value, "//") || (len(value) > 1 && value[1] == ':')
}

func wikiMapUnsafeSegments(value string) bool {
	value = strings.TrimSuffix(value, "/")
	if value == "" {
		return true
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return true
		}
	}
	return false
}

func wikiMapWarning(kind string, index int) string {
	return fmt.Sprintf("wiki_map_%s_%d", kind, index+1)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsEquivalentRoot(values []string, target string) bool {
	target = strings.TrimSuffix(filepath.ToSlash(target), "/")
	for _, value := range values {
		if strings.TrimSuffix(filepath.ToSlash(value), "/") == target {
			return true
		}
	}
	return false
}
