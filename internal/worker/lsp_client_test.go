package worker

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWriteLSPMessage(t *testing.T) {
	var buf bytes.Buffer
	msg := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
	}

	err := writeLSPMessage(&buf, msg)
	if err != nil {
		t.Fatalf("writeLSPMessage failed: %v", err)
	}

	result := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("Content-Length:")) {
		t.Errorf("Expected Content-Length header, got: %s", result)
	}
}

func TestBuildFileURI(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/home/user/file.py", "file:///home/user/file.py"},
		{"/tmp/test.ts", "file:///tmp/test.ts"},
	}

	for _, tt := range tests {
		result := buildFileURI(tt.input)
		// On Windows, paths get converted differently
		if !bytes.Contains([]byte(result), []byte("file://")) {
			t.Errorf("buildFileURI(%s) did not produce file:// URI", tt.input)
		}
	}
}

func TestURIToPath(t *testing.T) {
	tests := []struct {
		input         string
		containsCheck string
	}{
		{"file:///home/user/file.py", "file.py"},
		{"file://home/user/file.py", "file.py"},
	}

	for _, tt := range tests {
		result := uriToPath(tt.input)
		if !bytes.Contains([]byte(result), []byte(tt.containsCheck)) {
			t.Errorf("uriToPath(%s) = %s, expected to contain %s", tt.input, result, tt.containsCheck)
		}
	}
}

func TestLSPPosition(t *testing.T) {
	tests := []struct {
		line     int
		col      int
		expLine  int
		expChar  int
	}{
		{1, 1, 0, 0},
		{10, 5, 9, 4},
		{-1, -1, 0, 0},
	}

	for _, tt := range tests {
		result := lspPositionMap(tt.line, tt.col)
		if result["line"] != tt.expLine || result["character"] != tt.expChar {
			t.Errorf("lspPositionMap(%d, %d) = {line: %d, character: %d}, expected {line: %d, character: %d}",
				tt.line, tt.col, result["line"], result["character"], tt.expLine, tt.expChar)
		}
	}
}

func TestLanguageIDForPath(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"file.py", "python"},
		{"file.pyi", "python"},
		{"file.ts", "typescript"},
		{"file.tsx", "typescript"},
		{"file.js", "javascript"},
		{"file.jsx", "javascript"},
		{"file.cs", "csharp"},
		{"file.go", "go"},
		{"file.rs", "rust"},
		{"file.txt", "plaintext"},
	}

	for _, tt := range tests {
		result := languageIDForPath(tt.path)
		if result != tt.expected {
			t.Errorf("languageIDForPath(%s) = %s, expected %s", tt.path, result, tt.expected)
		}
	}
}

func TestLSPRequestMarshal(t *testing.T) {
	req := lspRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  map[string]interface{}{"key": "value"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal lspRequest: %v", err)
	}

	var unmarshaled lspRequest
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal lspRequest: %v", err)
	}

	if unmarshaled.ID != req.ID || unmarshaled.Method != req.Method {
		t.Errorf("Round-trip marshal/unmarshal failed for lspRequest")
	}
}
