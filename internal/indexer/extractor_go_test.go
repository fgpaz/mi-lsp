package indexer

import (
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestExtractGoSymbols(t *testing.T) {
	repo := model.WorkspaceRepo{ID: "main", Name: "main"}
	content := []byte(`package workspace

// Registry keeps workspace aliases.
type Registry struct{}

// Loader defines registry loading.
type Loader interface {
	Load() error
}

const DefaultName = "mi-lsp"

var currentName = DefaultName

// Add registers a workspace.
func Add(name string) error {
	return nil
}

// Remove unregisters a workspace.
func (r *Registry) Remove(name string) error {
	return nil
}
`)

	items := extractGo(repo, "internal/workspace/registry.go", "hash", content)
	assertGoSymbol(t, items, "Registry", "struct", "", "public")
	assertGoSymbol(t, items, "Loader", "interface", "", "public")
	assertGoSymbol(t, items, "DefaultName", "const", "", "public")
	assertGoSymbol(t, items, "currentName", "var", "", "package")
	assertGoSymbol(t, items, "Add", "function", "", "public")
	assertGoSymbol(t, items, "Remove", "method", "Registry", "public")
}

func assertGoSymbol(t *testing.T, items []model.SymbolRecord, name string, kind string, parent string, scope string) {
	t.Helper()
	for _, item := range items {
		if item.Name == name && item.Kind == kind {
			if item.Parent != parent {
				t.Fatalf("%s parent = %q, want %q", name, item.Parent, parent)
			}
			if item.Scope != scope {
				t.Fatalf("%s scope = %q, want %q", name, item.Scope, scope)
			}
			if item.Language != "go" {
				t.Fatalf("%s language = %q, want go", name, item.Language)
			}
			if item.SearchText == "" {
				t.Fatalf("%s search text is empty", name)
			}
			return
		}
	}
	t.Fatalf("symbol %s/%s not found in %#v", name, kind, items)
}
