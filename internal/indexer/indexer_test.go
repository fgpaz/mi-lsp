package indexer

import (
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/language"
)

// TestModernExtensionWalk verifies that all four modern JS/TS module extensions
// are collected by the supported-extension filter (mirrors what WalkWorkspace does).
func TestModernExtensionWalk(t *testing.T) {
	extensions := []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts"}
	for _, ext := range extensions {
		// Simulate the check done in WalkWorkspace.
		if !language.IsSupportedCodePath("file" + ext) {
			t.Errorf("WalkWorkspace should include %s files", ext)
		}
	}
}

// TestModernExtensionWalkCaseInsensitive verifies case-insensitive matching.
func TestModernExtensionWalkCaseInsensitive(t *testing.T) {
	testCases := []string{".JS", ".MJS", ".MTS", ".CTS", ".JSX", ".TSX"}
	for _, ext := range testCases {
		if !language.IsSupportedCodePath("file" + ext) {
			t.Errorf("WalkWorkspace should include %s files (case-insensitive)", ext)
		}
	}
}

// TestWalkerSupportedExtensionsAreSubsetOfRegistry verifies that every extension
// in the walker's internal list is still present in the shared registry.
func TestWalkerSupportedExtensionsAreSubsetOfRegistry(t *testing.T) {
	// This test catches the situation where the old supportedExtensions map
	// was the single source of truth and the registry was added later but
	// walker code still references the old map.  After migration the walker
	// no longer has supportedExtensions; this test verifies the registry
	// covers every extension that any consumer of WalkWorkspace expects.
	registryExtensions := extensionsFromRegistry()

	for _, ext := range []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts", ".cs", ".go", ".py", ".pyi"} {
		found := false
		for _, re := range registryExtensions {
			if re == ext {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("registry missing %q that walkers rely on", ext)
		}
	}
}

// extensionsFromRegistry returns all extensions that language.IsSupportedCodePath recognises.
func extensionsFromRegistry() []string {
	all := []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts", ".cs", ".go", ".py", ".pyi",
		".txt", ".json", ".md", ".scss", ".css"}
	var result []string
	for _, ext := range all {
		if language.IsSupportedCodePath("file" + ext) {
			result = append(result, ext)
		}
	}
	return result
}

// TestLanguageClassificationConsistency verifies that the language returned by
// languageForPath matches what the shared registry would return.
func TestLanguageClassificationConsistency(t *testing.T) {
	files := []struct {
		path   string
		expect string
	}{
		{"app.js", "javascript"},
		{"app.mjs", "javascript"},
		{"app.cjs", "javascript"},
		{"app.ts", "typescript"},
		{"app.mts", "typescript"},
		{"app.cts", "typescript"},
		{"app.cs", "csharp"},
		{"main.go", "go"},
		{"run.py", "python"},
		{"stub.pyi", "python"},
	}
	for _, f := range files {
		got := languageForPath(f.path)
		if got != f.expect {
			t.Errorf("languageForPath(%q) = %q; want %q", f.path, got, f.expect)
		}
	}
}

// TestLanguageFromExtConsistency verifies that languageFromExt matches ForPath.
func TestLanguageFromExtConsistency(t *testing.T) {
	testCases := []struct {
		ext    string
		expect string
	}{
		{".cs", "csharp"},
		{".go", "go"},
		{".ts", "typescript"},
		{".tsx", "typescript"},
		{".mts", "typescript"},
		{".cts", "typescript"},
		{".js", "javascript"},
		{".jsx", "javascript"},
		{".mjs", "javascript"},
		{".cjs", "javascript"},
		{".py", "python"},
		{".pyi", "python"},
		{"", ""},
		{".txt", ""},
	}
	for _, tc := range testCases {
		got := languageFromExt(tc.ext)
		if got != tc.expect {
			t.Errorf("languageFromExt(%q) = %q; want %q", tc.ext, got, tc.expect)
		}
	}
}

// TestIncrementalSkipsUnknownExtensions verifies that the incremental index
// skips files with unknown extensions (languageFromExt returns "").
func TestIncrementalSkipsUnknownExtensions(t *testing.T) {
	// This simulates the check in incrementalIndexWithGraphProgress:
	//   if languageFromExt(...) == "" { skippedFiles++ }
	skipPaths := []string{"doc.txt", "config.json", "style.scss"}
	for _, p := range skipPaths {
		if languageFromExt(filepath.Ext(p)) != "" {
			t.Errorf("languageFromExt(%q) returned %q; expected empty to trigger skip", p, languageFromExt(filepath.Ext(p)))
		}
	}
}

// TestExtractMeaningfulPathSegmentsModernExts verifies that extension stripping
// works for all modern extensions.
func TestExtractMeaningfulPathSegmentsModernExts(t *testing.T) {
	files := []struct {
		path string
		want string // expected segment after stripping
	}{
		{"src/foo.mjs", "foo"},
		{"src/foo.cjs", "foo"},
		{"src/foo.mts", "foo"},
		{"src/foo.cts", "foo"},
		{"src/foo.js", "foo"},
		{"src/foo.ts", "foo"},
		{"src/foo.js.config", "foo.js.config"}, // .config is not a known source ext
	}
	for _, f := range files {
		segments := extractMeaningfulPathSegments(filepath.Join("/tmp", f.path))
		found := false
		for _, seg := range segments {
			if seg == f.want {
				found = true
				break
			}
		}
		if !found && len(segments) > 0 {
			t.Errorf("extractMeaningfulPathSegments(%q) missing segment %q; got %v", f.path, f.want, segments)
		}
	}
	// Verify that .config (unknown ext) is NOT stripped.
	got := extractMeaningfulPathSegments("/tmp/src/foo.js.config")
	found := false
	for _, s := range got {
		if s == "foo.js.config" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("extractMeaningfulPathSegments(%q) lost unknown suffix; got %v", "src/foo.js.config", got)
	}
}