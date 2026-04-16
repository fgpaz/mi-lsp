package service

import (
	"os"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestParseDepth_Empty(t *testing.T) {
	depth := parseDepth("")

	if !depth.definition {
		t.Error("parseDepth(\"\") definition = false, want true")
	}
	if !depth.implementors {
		t.Error("parseDepth(\"\") implementors = false, want true")
	}
	if !depth.callers {
		t.Error("parseDepth(\"\") callers = false, want true")
	}
	if !depth.tests {
		t.Error("parseDepth(\"\") tests = false, want true")
	}
}

func TestParseDepth_SingleValue(t *testing.T) {
	tests := []struct {
		input string
		check func(relatedDepth) bool
		name  string
	}{
		{"definition", func(d relatedDepth) bool { return d.definition && !d.callers && !d.implementors && !d.tests }, "definition only"},
		{"callers", func(d relatedDepth) bool { return d.callers && !d.definition && !d.implementors && !d.tests }, "callers only"},
		{"implementors", func(d relatedDepth) bool { return d.implementors && !d.definition && !d.callers && !d.tests }, "implementors only"},
		{"tests", func(d relatedDepth) bool { return d.tests && !d.definition && !d.callers && !d.implementors }, "tests only"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			depth := parseDepth(tt.input)
			if !tt.check(depth) {
				t.Errorf("parseDepth(%q) = %+v, check failed", tt.input, depth)
			}
		})
	}
}

func TestParseDepth_MultipleValues(t *testing.T) {
	tests := []struct {
		input         string
		expectDef     bool
		expectCallers bool
		expectImpls   bool
		expectTests   bool
	}{
		{"definition,callers", true, true, false, false},
		{"callers,tests", false, true, false, true},
		{"definition,implementors,tests", true, false, true, true},
		{"callers,implementors", false, true, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			depth := parseDepth(tt.input)

			if depth.definition != tt.expectDef {
				t.Errorf("definition = %v, want %v", depth.definition, tt.expectDef)
			}
			if depth.callers != tt.expectCallers {
				t.Errorf("callers = %v, want %v", depth.callers, tt.expectCallers)
			}
			if depth.implementors != tt.expectImpls {
				t.Errorf("implementors = %v, want %v", depth.implementors, tt.expectImpls)
			}
			if depth.tests != tt.expectTests {
				t.Errorf("tests = %v, want %v", depth.tests, tt.expectTests)
			}
		})
	}
}

func TestParseDepth_WithWhitespace(t *testing.T) {
	depth := parseDepth("  definition , callers , tests  ")

	if !depth.definition {
		t.Error("definition = false, want true (with whitespace)")
	}
	if !depth.callers {
		t.Error("callers = false, want true (with whitespace)")
	}
	if !depth.tests {
		t.Error("tests = false, want true (with whitespace)")
	}
}

func TestParseDepth_InvalidValues(t *testing.T) {
	// Invalid values should be ignored
	depth := parseDepth("definition,invalid,callers,nonexistent")

	if !depth.definition {
		t.Error("definition = false, want true")
	}
	if !depth.callers {
		t.Error("callers = false, want true")
	}
	if depth.implementors {
		t.Error("implementors = true, want false (invalid not recognized)")
	}
	if depth.tests {
		t.Error("tests = true, want false (invalid not recognized)")
	}
}

func TestParseDepth_DuplicateValues(t *testing.T) {
	depth := parseDepth("callers,callers,tests,tests")

	// Should still parse correctly (duplicates don't hurt)
	if !depth.callers {
		t.Error("callers = false, want true")
	}
	if !depth.tests {
		t.Error("tests = false, want true")
	}
	if depth.definition {
		t.Error("definition = true, want false")
	}
}

func TestIsTestFile_GoTestFiles(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"MyService_test.go", true},
		{"service_test.go", true},
		{"handlers_test.go", true},
		{"app_test.go", true},
		{"main.go", false},
		{"helpers.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isTestFile(tt.path)
			if got != tt.want {
				t.Errorf("isTestFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsTestFile_CSharpTestFiles(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"MyServiceTests.cs", true},
		{"ServiceTests.cs", true},
		{"UserService.cs", false},
		{"MyServiceTest.cs", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isTestFile(tt.path)
			if got != tt.want {
				t.Errorf("isTestFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsTestFile_TypeScriptTestFiles(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"component.spec.ts", true},
		{"service.spec.ts", true},
		{"utils.test.ts", true},
		{"index.test.tsx", true},
		{"page.tsx", false},
		{"component.ts", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isTestFile(tt.path)
			if got != tt.want {
				t.Errorf("isTestFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsTestFile_PathsWithDirs(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"src/tests/MyTest.go", true},
		{"test/service.go", true},
		{"src/service.go", false},
		{"__tests__/component.tsx", true},
		{"src/__test__/helper.ts", true},
		{"tests/__snapshots__/file.snap", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isTestFile(tt.path)
			if got != tt.want {
				t.Errorf("isTestFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsTestFile_CaseInsensitive(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"SERVICE_TEST.GO", true},
		{"Component.SPEC.TS", true},
		{"UserTest.CS", true},
		{"Main.GO", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isTestFile(tt.path)
			if got != tt.want {
				t.Errorf("isTestFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsTestFile_EdgeCases(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"", false},
		{".", false},
		{"test", true}, // contains "test"
		{".test", true},
		{"test.js", true},
		{"jest.config.js", false}, // doesn't match test patterns exactly
		{"mocha.opts", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isTestFile(tt.path)
			if got != tt.want {
				t.Errorf("isTestFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestSymbolToContent_WithLines(t *testing.T) {
	root := t.TempDir()

	// Create a test file
	testFilePath := "src/test.go"
	fullPath := root + "/src/test.go"
	if err := os.MkdirAll(root+"/src", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte("package main\n\nfunc Hello() {\n  println(\"hello\")\n}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Create a SymbolRecord
	sym := model.SymbolRecord{
		FilePath:  testFilePath,
		Name:      "Hello",
		Kind:      "function",
		StartLine: 3,
		EndLine:   5,
	}

	got := symbolToContent(root, sym, true)

	if got.Name != "Hello" {
		t.Errorf("symbolToContent Name = %q, want Hello", got.Name)
	}

	if got.Kind != "function" {
		t.Errorf("symbolToContent Kind = %q, want function", got.Kind)
	}

	if got.Line != 3 {
		t.Errorf("symbolToContent Line = %d, want 3", got.Line)
	}

	if got.ContentMode != "symbol" {
		t.Errorf("symbolToContent ContentMode = %q, want symbol", got.ContentMode)
	}

	if got.Content == "" {
		t.Error("symbolToContent Content is empty, should have file content")
	}
}

func TestSymbolToContent_WithoutLines(t *testing.T) {
	root := t.TempDir()
	testFilePath := "src/test.go"

	// Create a symbol with no line info
	sym := model.SymbolRecord{
		FilePath:  testFilePath,
		Name:      "Hello",
		Kind:      "function",
		StartLine: 0,
		EndLine:   0,
	}

	got := symbolToContent(root, sym, true)

	if got.Name != "Hello" {
		t.Errorf("symbolToContent Name = %q, want Hello", got.Name)
	}

	if got.Content != "" {
		t.Errorf("symbolToContent Content should be empty without line info, got %q", got.Content)
	}

	if got.ContentMode != "" {
		t.Errorf("symbolToContent ContentMode should be empty without line info, got %q", got.ContentMode)
	}
}

func TestSymbolToContent_FileNotFound(t *testing.T) {
	root := t.TempDir()
	testFilePath := "src/nonexistent.go"

	sym := model.SymbolRecord{
		FilePath:  testFilePath,
		Name:      "Hello",
		Kind:      "function",
		StartLine: 1,
		EndLine:   5,
	}

	got := symbolToContent(root, sym, true)

	if got.Name != "Hello" {
		t.Errorf("symbolToContent Name = %q, want Hello", got.Name)
	}

	if got.Content != "" {
		t.Errorf("symbolToContent Content should be empty for nonexistent file, got %q", got.Content)
	}

	if got.ContentMode != "" {
		t.Errorf("symbolToContent ContentMode should be empty for nonexistent file, got %q", got.ContentMode)
	}
}
