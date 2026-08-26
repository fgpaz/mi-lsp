package language

import (
	"path/filepath"
	"sort"
	"testing"
)

func TestForPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantLang string
		wantOk   bool
	}{
		// JavaScript
		{"js", "src/app.js", "javascript", true},
		{"jsx", "src/App.jsx", "javascript", true},
		{"mjs", "src/esm.mjs", "javascript", true},
		{"cjs", "src/cjs.cjs", "javascript", true},
		// TypeScript
		{"ts", "src/app.ts", "typescript", true},
		{"tsx", "src/App.tsx", "typescript", true},
		{"mts", "src/esm.mts", "typescript", true},
		{"cts", "src/cjs.cts", "typescript", true},
		// C#
		{"cs", "src/Program.cs", "csharp", true},
		// Go
		{"go", "main.go", "go", true},
		// Python
		{"py", "run.py", "python", true},
		{"pyi", "stub.pyi", "python", true},
		// Case-insensitive extensions
		{"js uppercase", "src/app.JS", "javascript", true},
		{"ts uppercase", "src/app.TS", "typescript", true},
		{"mjs uppercase", "src/app.MJS", "javascript", true},
		{"mts uppercase", "src/app.MTS", "typescript", true},
		// Directories should not match
		{"dir", "src/", "", false},
		{"dir trailing slash", "src/app.js/", "", false},
		// Unknown extension — must NOT default to TypeScript
		{"txt", "doc.txt", "", false},
		{"md", "README.md", "", false},
		{"json", "data.json", "", false},
		{"yml", "config.yml", "", false},
		// Compound filename — extension at end
		{"compound", "src/foo.bar.js", "javascript", true},
		{"compound2", "src/foo.tsx", "typescript", true},
		{"compound3", "src/foo.mjs.config", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLang, gotOk := ForPath(tt.path)
			if gotLang != tt.wantLang || gotOk != tt.wantOk {
				t.Errorf("ForPath(%q) = (%q, %v); want (%q, %v)", tt.path, gotLang, gotOk, tt.wantLang, tt.wantOk)
			}
		})
	}
}

func TestIsSupportedCodePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"js supported", "app.js", true},
		{"mjs supported", "lib.mjs", true},
		{"txt unsupported", "doc.txt", false},
		{"md unsupported", "README.md", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSupportedCodePath(tt.path); got != tt.want {
				t.Errorf("IsSupportedCodePath(%q) = %v; want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestExtensions(t *testing.T) {
	extensions := Extensions()
	// Must be sorted
	if !sort.StringsAreSorted(extensions) {
		t.Errorf("Extensions() not sorted: %v", extensions)
	}
	// Must contain all known modern JS/TS extensions
	expected := []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts", ".cs", ".go", ".py", ".pyi"}
	for _, want := range expected {
		found := false
		for _, got := range extensions {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Extensions() missing %q", want)
		}
	}
	// Must not contain unknown extensions
	if contains(extensions, ".txt") {
		t.Error("Extensions() must not contain .txt")
	}
}

func TestStripKnownExtension(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"js strip", "app.js", "app"},
		{"mjs strip", "lib.mjs", "lib"},
		{"ts strip", "component.tsx", "component"},
		{"mts strip", "esmod.mts", "esmod"},
		{"cs strip", "Program.cs", "Program"},
		{"go strip", "main.go", "main"},
		{"py strip", "run.py", "run"},
		// Unknown extension: must return unchanged
		{"txt strip unknown", "doc.txt", "doc.txt"},
		{"md strip unknown", "README.md", "README.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripKnownExtension(tt.input)
			if got != tt.want {
				t.Errorf("StripKnownExtension(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNoTypeScriptDefaultForUnknown(t *testing.T) {
	// Regression: unknown extensions must NOT be classified as TypeScript.
	for _, path := range []string{"readme.txt", "config.json", "style.scss", "data.csv"} {
		lang, ok := ForPath(path)
		if ok {
			t.Errorf("ForPath(%q) returned ok=true (lang=%q); expected ok=false", path, lang)
		}
		if lang == "typescript" {
			t.Errorf("ForPath(%q) returned TypeScript for unknown extension — regression", path)
		}
	}
}

func TestForPathWithAbsPath(t *testing.T) {
	// Ensure ForPath works with absolute paths (realistic scenario).
	tests := []struct {
		name     string
		path     string
		wantLang string
		wantOk   bool
	}{
		{"abs js", filepath.Join("/tmp", "app.js"), "javascript", true},
		{"abs mts", filepath.Join("src", "lib.mts"), "typescript", true},
		{"abs unknown", filepath.Join("tmp", "data.txt"), "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLang, gotOk := ForPath(tt.path)
			if gotLang != tt.wantLang || gotOk != tt.wantOk {
				t.Errorf("ForPath(%q) = (%q, %v); want (%q, %v)", tt.path, gotLang, gotOk, tt.wantLang, tt.wantOk)
			}
		})
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}