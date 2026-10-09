package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestRustAnalyzerDiscoveryUsesOverride(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "rust-analyzer")
	if err := os.WriteFile(binary, []byte("test stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MI_LSP_RUST_ANALYZER_PATH", binary)

	got, err := findRustAnalyzerBinary(t.TempDir())
	if err != nil {
		t.Fatalf("findRustAnalyzerBinary: %v", err)
	}
	if got != binary {
		t.Fatalf("binary = %q, want override %q", got, binary)
	}
	client, err := NewRuntimeClient(t.TempDir(), model.WorkspaceRegistration{Root: t.TempDir()}, model.WorkerRequest{BackendType: "rust-analyzer"})
	if err != nil {
		t.Fatalf("NewRuntimeClient: %v", err)
	}
	lsp, ok := client.(*LSPClient)
	if !ok || lsp.config.ServerCmd != binary {
		t.Fatalf("runtime client = %#v, want LSPClient using %q", client, binary)
	}
}

func TestLanguageIDForRustFile(t *testing.T) {
	if got := languageIDForPath("src/lib.rs"); got != "rust" {
		t.Fatalf("languageIDForPath(.rs) = %q, want rust", got)
	}
}
