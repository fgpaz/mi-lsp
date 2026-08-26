package language

import (
	"path/filepath"
	"sort"
	"strings"
)

// registry pairs extension -> language for all supported source-code files.
var registry = map[string]string{
	// JavaScript
	".js":   "javascript",
	".jsx":  "javascript",
	".mjs":  "javascript",
	".cjs":  "javascript",
	// TypeScript
	".ts":   "typescript",
	".tsx":  "typescript",
	".mts":  "typescript",
	".cts":  "typescript",
	// C#
	".cs": "csharp",
	// Go
	".go": "go",
	// Python
	".py":  "python",
	".pyi": "python",
}

// ForPath returns the language for a file path based on its extension and whether
// the extension is known. Extension comparison is case-insensitive.
func ForPath(path string) (language string, ok bool) {
	ext := strings.ToLower(filepath.Ext(path))
	l, found := registry[ext]
	if !found {
		return "", false
	}
	return l, true
}

// IsSupportedCodePath returns true when path has a known source-code extension.
func IsSupportedCodePath(path string) bool {
	_, ok := ForPath(path)
	return ok
}

// Extensions returns a sorted copy of all supported file extensions.
func Extensions() []string {
	result := make([]string, 0, len(registry))
	for ext := range registry {
		result = append(result, ext)
	}
	sort.Strings(result)
	return result
}

// StripKnownExtension returns name without the trailing extension, when that
// extension is a known source-code extension.  Otherwise it returns name
// unchanged (caller retains existing fallback semantics).
func StripKnownExtension(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if _, found := registry[ext]; found {
		return strings.TrimSuffix(name, name[len(name)-len(ext):])
	}
	return name
}