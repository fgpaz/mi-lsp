package indexer

import (
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestExtractCatalogPropagatesSourceLanguageToEveryJavaScriptAndTypeScriptSymbol(t *testing.T) {
	cases := []struct {
		extension string
		language  string
	}{
		{extension: ".js", language: "javascript"},
		{extension: ".jsx", language: "javascript"},
		{extension: ".mjs", language: "javascript"},
		{extension: ".cjs", language: "javascript"},
		{extension: ".ts", language: "typescript"},
		{extension: ".tsx", language: "typescript"},
		{extension: ".mts", language: "typescript"},
		{extension: ".cts", language: "typescript"},
	}
	content := []byte("export class Demo {}\nexport interface Shape {}\nexport type Alias = string\nexport function runDemo() {}\nexport const value = 1\n")
	root := t.TempDir()
	repo := model.WorkspaceRepo{ID: "main", Name: "main", Root: "."}
	for _, test := range cases {
		t.Run(test.extension, func(t *testing.T) {
			path := filepath.Join(root, "src", "demo", "service"+test.extension)
			symbols, file := ExtractCatalog(root, repo, path, content)
			if file.Language != test.language {
				t.Fatalf("file language=%q, want %q", file.Language, test.language)
			}
			if len(symbols) == 0 {
				t.Fatal("ExtractCatalog produced no symbols")
			}
			for _, symbol := range symbols {
				if symbol.Language != test.language {
					t.Fatalf("symbol %s language=%q, want %q", symbol.QualifiedName, symbol.Language, test.language)
				}
			}
		})
	}
}

func TestExtractCatalogPropagatesSourceLanguageToSyntheticRouteSymbols(t *testing.T) {
	cases := []struct {
		extension string
		language  string
	}{
		{extension: ".js", language: "javascript"},
		{extension: ".jsx", language: "javascript"},
		{extension: ".mjs", language: "javascript"},
		{extension: ".cjs", language: "javascript"},
		{extension: ".ts", language: "typescript"},
		{extension: ".tsx", language: "typescript"},
		{extension: ".mts", language: "typescript"},
		{extension: ".cts", language: "typescript"},
	}
	root := t.TempDir()
	repo := model.WorkspaceRepo{ID: "main", Name: "main", Root: "."}
	for _, test := range cases {
		t.Run(test.extension, func(t *testing.T) {
			path := filepath.Join(root, "app", "page"+test.extension)
			symbols, file := ExtractCatalog(root, repo, path, []byte("export function page() {}\n"))
			if file.Language != test.language {
				t.Fatalf("file language=%q, want %q", file.Language, test.language)
			}
			foundRoute := false
			for _, symbol := range symbols {
				if symbol.Language != test.language {
					t.Fatalf("symbol %s language=%q, want %q", symbol.QualifiedName, symbol.Language, test.language)
				}
				if symbol.Kind == "route" {
					foundRoute = true
				}
			}
			if !foundRoute {
				t.Fatalf("symbols=%#v, want synthetic route symbol", symbols)
			}
		})
	}
}
