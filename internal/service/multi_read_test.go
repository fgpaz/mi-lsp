package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFileRangeString_WholeFile(t *testing.T) {
	tests := []struct {
		input string
		want  fileRange
	}{
		{"file.go", fileRange{File: "file.go", StartLine: 1, EndLine: 0}},
		{"src/main.cs", fileRange{File: "src/main.cs", StartLine: 1, EndLine: 0}},
		{"path/to/module.ts", fileRange{File: "path/to/module.ts", StartLine: 1, EndLine: 0}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseFileRangeString(tt.input)
			if err != nil {
				t.Fatalf("parseFileRangeString(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseFileRangeString(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseFileRangeString_SingleLine(t *testing.T) {
	tests := []struct {
		input string
		want  fileRange
	}{
		{"file.go:10", fileRange{File: "file.go", StartLine: 10, EndLine: 10}},
		{"src/main.cs:5", fileRange{File: "src/main.cs", StartLine: 5, EndLine: 5}},
		{"path/module.ts:100", fileRange{File: "path/module.ts", StartLine: 100, EndLine: 100}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseFileRangeString(tt.input)
			if err != nil {
				t.Fatalf("parseFileRangeString(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseFileRangeString(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseFileRangeString_LineRange(t *testing.T) {
	tests := []struct {
		input string
		want  fileRange
	}{
		{"file.go:10-20", fileRange{File: "file.go", StartLine: 10, EndLine: 20}},
		{"src/main.cs:1-50", fileRange{File: "src/main.cs", StartLine: 1, EndLine: 50}},
		{"path/module.ts:5-15", fileRange{File: "path/module.ts", StartLine: 5, EndLine: 15}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseFileRangeString(tt.input)
			if err != nil {
				t.Fatalf("parseFileRangeString(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseFileRangeString(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseFileRangeString_WindowsPaths(t *testing.T) {
	tests := []struct {
		input string
		want  fileRange
	}{
		{"C:\\path\\file.go:10-20", fileRange{File: "C:\\path\\file.go", StartLine: 10, EndLine: 20}},
		{"D:\\src\\main.cs:5", fileRange{File: "D:\\src\\main.cs", StartLine: 5, EndLine: 5}},
		{"E:\\project\\module.ts", fileRange{File: "E:\\project\\module.ts", StartLine: 1, EndLine: 0}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseFileRangeString(tt.input)
			if err != nil {
				t.Fatalf("parseFileRangeString(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseFileRangeString(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseFileRangeString_InvalidLineNumbers(t *testing.T) {
	tests := []struct {
		input string
		name  string
	}{
		{"file.go:abc", "invalid start line"},
		{"file.go:10-xyz", "invalid end line"},
		{"file.go:notanumber", "single invalid line"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseFileRangeString(tt.input)
			if err == nil {
				t.Errorf("parseFileRangeString(%q) expected error, got nil", tt.input)
			}
		})
	}
}

func TestReadFileRange_CorrectContent(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.go")

	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	gotContent, lineCount, truncated, err := readFileRange(filePath, 2, 4, 1000)
	if err != nil {
		t.Fatalf("readFileRange: %v", err)
	}

	wantContent := "line2\nline3\nline4"
	if gotContent != wantContent {
		t.Errorf("readFileRange content = %q, want %q", gotContent, wantContent)
	}

	if lineCount != 3 {
		t.Errorf("readFileRange lineCount = %d, want 3", lineCount)
	}

	if truncated {
		t.Error("readFileRange truncated = true, want false")
	}
}

func TestReadFileRange_SingleLine(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.go")

	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	gotContent, lineCount, truncated, err := readFileRange(filePath, 2, 2, 1000)
	if err != nil {
		t.Fatalf("readFileRange: %v", err)
	}

	if gotContent != "line2" {
		t.Errorf("readFileRange content = %q, want 'line2'", gotContent)
	}

	if lineCount != 1 {
		t.Errorf("readFileRange lineCount = %d, want 1", lineCount)
	}

	if truncated {
		t.Error("readFileRange truncated = true, want false")
	}
}

func TestReadFileRange_WholeFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.go")

	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	gotContent, lineCount, truncated, err := readFileRange(filePath, 1, 0, 1000)
	if err != nil {
		t.Fatalf("readFileRange: %v", err)
	}

	wantContent := "line1\nline2\nline3"
	if gotContent != wantContent {
		t.Errorf("readFileRange content = %q, want %q", gotContent, wantContent)
	}

	if lineCount != 3 {
		t.Errorf("readFileRange lineCount = %d, want 3", lineCount)
	}

	if truncated {
		t.Error("readFileRange truncated = true, want false")
	}
}

func TestReadFileRange_CharBudgetTruncation(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.go")

	content := "short\nmedium line\nlong line with lots of text\n"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	gotContent, _, truncated, err := readFileRange(filePath, 1, 0, 15)
	if err != nil {
		t.Fatalf("readFileRange: %v", err)
	}

	if !truncated {
		t.Error("readFileRange truncated = false, want true (exceeded budget)")
	}

	if len(gotContent) > 15 {
		t.Errorf("readFileRange content exceeds budget: %d > 15", len(gotContent))
	}
}

func TestReadFileRange_ZeroBudget(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.go")

	if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	gotContent, lineCount, truncated, err := readFileRange(filePath, 1, 0, 0)
	if err != nil {
		t.Fatalf("readFileRange: %v", err)
	}

	if gotContent != "" {
		t.Errorf("readFileRange with zero budget should return empty string, got %q", gotContent)
	}

	if lineCount != 0 {
		t.Errorf("readFileRange lineCount = %d, want 0", lineCount)
	}

	if !truncated {
		t.Error("readFileRange truncated = false, want true (zero budget)")
	}
}

func TestReadFileRange_NonexistentFile(t *testing.T) {
	_, _, _, err := readFileRange("/nonexistent/path/file.go", 1, 10, 1000)
	if err == nil {
		t.Fatal("readFileRange expected error for nonexistent file, got nil")
	}
}

func TestReadFileRange_OutOfBounds(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.go")

	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	gotContent, lineCount, truncated, err := readFileRange(filePath, 10, 20, 1000)
	if err != nil {
		t.Fatalf("readFileRange: %v", err)
	}

	if gotContent != "" {
		t.Errorf("readFileRange out of bounds should return empty string, got %q", gotContent)
	}

	if lineCount != 0 {
		t.Errorf("readFileRange lineCount = %d, want 0", lineCount)
	}

	if truncated {
		t.Error("readFileRange truncated = true, want false (no content found)")
	}
}

func TestReadFileRange_LargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "large.go")

	// Create file with 1000 lines
	var buf strings.Builder
	for i := 1; i <= 1000; i++ {
		buf.WriteString("line " + string(rune('0' + (i % 10))) + "\n")
	}

	if err := os.WriteFile(filePath, []byte(buf.String()), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	gotContent, lineCount, truncated, err := readFileRange(filePath, 500, 510, 100000)
	if err != nil {
		t.Fatalf("readFileRange: %v", err)
	}

	if lineCount != 11 {
		t.Errorf("readFileRange lineCount = %d, want 11", lineCount)
	}

	if truncated {
		t.Error("readFileRange truncated = true, want false (sufficient budget)")
	}

	if !strings.Contains(gotContent, "line 0") {
		t.Errorf("readFileRange content does not match expected pattern")
	}
}
