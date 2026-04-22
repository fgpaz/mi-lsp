package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadContextWindowStreamsAndClampsPastEOF(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.cs")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\nfour\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	focus, start, end, text, err := readContextWindow(path, 99, 1)
	if err != nil {
		t.Fatalf("readContextWindow: %v", err)
	}
	if focus != 4 || start != 2 || end != 4 {
		t.Fatalf("window = focus %d start %d end %d, want 4/2/4", focus, start, end)
	}
	if text != "two\nthree\nfour" {
		t.Fatalf("text = %q, want tail window", text)
	}
}
