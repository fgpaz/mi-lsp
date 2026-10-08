package indexer

import (
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestExtractCatalogRustDeclarations(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src", "lib.rs")
	content := []byte(`pub struct Service {
    name: String,
}

impl Service {
    pub async fn execute(&self) {}
    pub(crate) fn reset(&mut self) {}
}

pub enum State { Ready, Failed }
pub trait Runner {
    fn run(&self);
}
pub type UserId = u64;
pub const LIMIT: usize = 10;
pub static GLOBAL: usize = 0;
pub mod api {
    pub fn call() {}
}
macro_rules! make_service { () => {} }
`)
	repo := model.WorkspaceRepo{ID: "main", Name: "fixture"}
	symbols, file := ExtractCatalog(root, repo, path, content)
	if file.Language != "rust" {
		t.Fatalf("file language = %q, want rust", file.Language)
	}
	got := make(map[string]model.SymbolRecord, len(symbols))
	for _, symbol := range symbols {
		if symbol.Language != "rust" || symbol.FilePath != "src/lib.rs" {
			t.Errorf("symbol metadata = %#v, want Rust src/lib.rs", symbol)
		}
		got[symbol.Name] = symbol
	}
	for _, name := range []string{"Service", "execute", "reset", "State", "Runner", "run", "UserId", "LIMIT", "GLOBAL", "api", "call", "make_service"} {
		if _, ok := got[name]; !ok {
			t.Errorf("missing Rust symbol %q (got %v)", name, symbols)
		}
	}
	if got["execute"].Kind != "method" || got["execute"].Parent != "Service" {
		t.Errorf("execute = %#v, want method under Service", got["execute"])
	}
	if got["reset"].Scope != "crate" {
		t.Errorf("reset scope = %q, want crate", got["reset"].Scope)
	}
	if got["Runner"].Kind != "trait" || got["run"].Parent != "Runner" {
		t.Errorf("trait symbols = %#v / %#v", got["Runner"], got["run"])
	}
}
