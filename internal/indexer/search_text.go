package indexer

import (
	"path/filepath"
	"regexp"
	"strings"
)

var camelSplitRe = regexp.MustCompile(`([a-z])([A-Z])|([A-Z])([A-Z][a-z])`)

// BuildSearchText constructs enriched search text from symbol metadata for intent-based search.
func BuildSearchText(name, signature, docComment, parent, filePath, kind string) string {
	parts := make([]string, 0, 8)

	// Symbol name split from PascalCase/camelCase
	parts = append(parts, splitIdentifier(name)...)

	// Kind
	if kind != "" {
		parts = append(parts, strings.ToLower(kind))
	}

	// Parent class/struct name
	if parent != "" {
		parts = append(parts, splitIdentifier(parent)...)
	}

	// Doc comments (stripped of markers, first 500 chars)
	if docComment != "" {
		text := extractDocCommentText(docComment)
		if text != "" {
			parts = append(parts, text)
		}
	}

	// Signature: extract parameter and return type names
	if signature != "" {
		parts = append(parts, extractSignatureNames(signature)...)
	}

	// File path: meaningful segments only
	parts = append(parts, extractMeaningfulPathSegments(filePath)...)

	result := normalizeForSearch(strings.Join(parts, " "))
	if len(result) > 2000 {
		result = result[:2000]
	}
	return result
}

// splitIdentifier converts CamelCase/camelCase into separate lowercase words.
// "HandleDaemonError" -> ["handle", "daemon", "error"]
func splitIdentifier(identifier string) []string {
	if identifier == "" {
		return nil
	}
	spaced := camelSplitRe.ReplaceAllString(identifier, "$1$3 $2$4")
	words := strings.Fields(strings.ToLower(spaced))
	result := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.Trim(w, "_")
		if len(w) > 1 {
			result = append(result, w)
		}
	}
	return result
}

// extractDocCommentText strips comment markers (// or ///) and returns plain text.
func extractDocCommentText(docComment string) string {
	lines := strings.Split(docComment, "\n")
	var result []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "///")
		line = strings.TrimPrefix(line, "//")
		line = strings.TrimSpace(line)
		if line != "" && len(line) > 2 {
			result = append(result, line)
		}
	}
	text := strings.Join(result, " ")
	if len(text) > 500 {
		text = text[:500]
	}
	return text
}

// extractSignatureNames pulls identifier words from a signature string.
func extractSignatureNames(signature string) []string {
	re := regexp.MustCompile(`[a-zA-Z_][a-zA-Z0-9_]*`)
	matches := re.FindAllString(signature, -1)
	keywords := map[string]struct{}{
		"func": {}, "void": {}, "async": {}, "const": {}, "let": {}, "var": {},
		"class": {}, "interface": {}, "type": {}, "extends": {}, "implements": {},
		"public": {}, "private": {}, "protected": {}, "internal": {}, "static": {},
		"virtual": {}, "override": {}, "abstract": {}, "sealed": {}, "partial": {},
		"new": {}, "return": {}, "string": {}, "int": {}, "bool": {}, "error": {},
		"byte": {}, "float": {}, "double": {}, "long": {}, "short": {},
	}
	seen := map[string]struct{}{}
	var result []string
	for _, m := range matches {
		lower := strings.ToLower(m)
		if _, isKw := keywords[lower]; isKw {
			continue
		}
		if _, dup := seen[lower]; dup {
			continue
		}
		seen[lower] = struct{}{}
		result = append(result, lower)
	}
	return result
}

// extractMeaningfulPathSegments returns non-trivial directory/file segments.
func extractMeaningfulPathSegments(filePath string) []string {
	filePath = filepath.ToSlash(filePath)
	segments := strings.Split(filePath, "/")
	trivial := map[string]struct{}{
		"src": {}, "lib": {}, "internal": {}, "pkg": {}, "tests": {}, "test": {},
		"app": {}, "pages": {}, "components": {}, "utils": {}, "helpers": {},
		"common": {}, "shared": {}, "core": {}, "main": {}, "index": {},
	}
	var result []string
	for _, seg := range segments {
		// Remove file extension
		for _, ext := range []string{".go", ".ts", ".tsx", ".js", ".jsx", ".cs", ".py"} {
			seg = strings.TrimSuffix(seg, ext)
		}
		seg = strings.ToLower(seg)
		if seg == "" || len(seg) < 3 {
			continue
		}
		if _, skip := trivial[seg]; skip {
			continue
		}
		result = append(result, seg)
	}
	return result
}

// normalizeForSearch lowercases and normalizes whitespace/separators.
func normalizeForSearch(text string) string {
	text = strings.ToLower(text)
	text = strings.ReplaceAll(text, "_", " ")
	text = strings.ReplaceAll(text, "-", " ")
	text = strings.ReplaceAll(text, ".", " ")
	return strings.Join(strings.Fields(text), " ")
}

// ExtractDocComment looks backwards from a line index to find doc comments above a symbol.
func ExtractDocComment(lines []string, symbolLineIndex int) string {
	if symbolLineIndex <= 0 || symbolLineIndex > len(lines) {
		return ""
	}
	var docLines []string
	for i := symbolLineIndex - 1; i >= 0 && i >= symbolLineIndex-15; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "///") || strings.HasPrefix(trimmed, "//") {
			docLines = append([]string{trimmed}, docLines...)
		} else if trimmed == "" {
			continue // skip blank lines between comment blocks
		} else {
			break // non-comment, non-blank line stops collection
		}
	}
	if len(docLines) == 0 {
		return ""
	}
	return strings.Join(docLines, "\n")
}
