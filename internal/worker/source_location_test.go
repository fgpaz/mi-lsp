package worker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInferLineAndOffsetFindsSymbolAcrossFileWhenLineIsMissing(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "contracts.py")
	content := "" +
		"from dataclasses import dataclass\n" +
		"\n" +
		"@dataclass(frozen=True)\n" +
		"class TenantActorContext:\n" +
		"    tenant_id: str\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	line, offset := inferLineAndOffset(file, 0, "TenantActorContext")
	if line != 4 {
		t.Fatalf("line = %d, want 4", line)
	}
	if offset != 7 {
		t.Fatalf("offset = %d, want 7", offset)
	}
}

func TestInferLineAndOffsetTargetsDeclaredIdentifierWhenSymbolIsEmpty(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "contracts.py")
	content := "" +
		"class TenantActorContext:\n" +
		"    tenant_id: str\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	line, offset := inferLineAndOffset(file, 1, "")
	if line != 1 {
		t.Fatalf("line = %d, want 1", line)
	}
	if offset != 7 {
		t.Fatalf("offset = %d, want 7", offset)
	}
}

func TestInferLineAndOffsetClampsPastEOFWithoutWholeFileBuffer(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "contracts.py")
	content := "first\n    value = 1\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	line, offset := inferLineAndOffset(file, 999, "")
	if line != 2 {
		t.Fatalf("line = %d, want 2", line)
	}
	if offset != 5 {
		t.Fatalf("offset = %d, want 5", offset)
	}
}
