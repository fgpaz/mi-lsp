package indexer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestWikiCodeIncrementalFixtureCatalogsModernExtensionsAndRefreshesChangedCode(t *testing.T) {
	root := newWikiCodeIncrementalFixture(t)
	for _, item := range []struct {
		path     string
		language string
		symbol   string
	}{
		{path: "src/demo/service.mjs", language: "javascript", symbol: "runDemo"},
		{path: "src/demo/compat.cjs", language: "javascript", symbol: "compatCJS"},
		{path: "src/demo/compat.mts", language: "typescript", symbol: "compatMTS"},
		{path: "src/demo/compat.cts", language: "typescript", symbol: "compatCTS"},
	} {
		t.Run(item.path, func(t *testing.T) {
			db, err := store.OpenReadOnlyExisting(root, store.WorkspaceDBPath(root))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var language string
			if err := db.QueryRowContext(context.Background(), "SELECT language FROM files WHERE file_path = ?", item.path).Scan(&language); err != nil {
				t.Fatal(err)
			}
			if language != item.language {
				t.Fatalf("file language=%q, want %q", language, item.language)
			}
			symbols, err := store.SymbolsByFile(context.Background(), db, item.path, 20, 0)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, symbol := range symbols {
				if symbol.Name == item.symbol {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("symbols=%#v, want %s", symbols, item.symbol)
			}
		})
	}

	changedPath := filepath.Join(root, filepath.FromSlash("src/demo/unmapped.mjs"))
	if err := os.WriteFile(changedPath, []byte("export function standaloneFeature() { return 'incremental'; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := IncrementalIndexWithPaths(context.Background(), root, []string{"src/demo/unmapped.mjs"}, nil)
	if err != nil {
		t.Fatalf("IncrementalIndexWithPaths: %v", err)
	}
	if result.Stats.Files != 1 || result.Stats.Ms < 0 {
		t.Fatalf("incremental result=%#v, want one processed code file", result)
	}

	db, err := store.OpenReadOnlyExisting(root, store.WorkspaceDBPath(root))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var hash string
	if err := db.QueryRowContext(context.Background(), "SELECT content_hash FROM files WHERE file_path = ?", "src/demo/unmapped.mjs").Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if hash == "" {
		t.Fatal("incremental catalog published an empty content hash")
	}
	symbols, err := store.SymbolsByFile(context.Background(), db, "src/demo/unmapped.mjs", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(symbols) != 1 || symbols[0].Name != "standaloneFeature" {
		t.Fatalf("incremental symbols=%#v, want standaloneFeature only", symbols)
	}
}

func TestWikiCodeIncrementalFixturePathNormalizationAndLanguageContract(t *testing.T) {
	for _, item := range []struct {
		path string
		want string
	}{
		{path: "src/demo/service.mjs", want: "javascript"},
		{path: "src/demo/compat.cjs", want: "javascript"},
		{path: "src/demo/compat.mts", want: "typescript"},
		{path: "src/demo/compat.cts", want: "typescript"},
	} {
		if got := languageFromExt(filepath.Ext(item.path)); got != item.want {
			t.Fatalf("languageFromExt(%q)=%q, want %q", item.path, got, item.want)
		}
	}
	root := t.TempDir()
	paths := normalizeIncrementalPaths(root, []string{filepath.Join(root, "src", "demo", "service.mjs"), "src/demo/service.mjs", "src/demo/service.mjs"})
	if len(paths) != 1 || paths[0] != "src/demo/service.mjs" {
		t.Fatalf("normalized paths=%#v", paths)
	}
	for _, excluded := range []string{".docs/raw/decoy.md", ".docs/auditoria/decoy.md", ".mi-lsp/index.db"} {
		if !isExcludedIncrementalPath(excluded) {
			t.Fatalf("path %q was not excluded", excluded)
		}
	}
}

func newWikiCodeIncrementalFixture(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	source := filepath.Join(repoRoot, "testdata", "wiki-code-bidirectional")
	root := t.TempDir()
	if err := copyWikiCodeIncrementalTree(source, root); err != nil {
		t.Fatalf("copy T3 fixture: %v", err)
	}
	for _, item := range []struct {
		path    string
		content string
	}{
		{path: "src/demo/compat.cjs", content: "export function compatCJS() { return 'cjs'; }\n"},
		{path: "src/demo/compat.mts", content: "export function compatMTS() { return 'mts'; }\n"},
		{path: "src/demo/compat.cts", content: "export function compatCTS() { return 'cts'; }\n"},
	} {
		path := filepath.Join(root, filepath.FromSlash(item.path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(item.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "wiki-code-incremental", Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", Languages: []string{"javascript", "typescript"}},
		Repos: []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/wiki-code-incremental", Languages: []string{"javascript", "typescript"}}},
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	if _, err := IndexWorkspaceWithGeneration(context.Background(), root, true, "wiki-code-incremental-baseline"); err != nil {
		t.Fatalf("IndexWorkspace: %v", err)
	}
	return root
}

func copyWikiCodeIncrementalTree(source, destination string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(source, entry.Name())
		dstPath := filepath.Join(destination, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("fixture contains unsupported symlink %s", entry.Name())
		}
		if entry.IsDir() {
			if err := copyWikiCodeIncrementalTree(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		content, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dstPath, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func TestWikiCodeIncrementalFixtureCatalogPathsAreSorted(t *testing.T) {
	root := newWikiCodeIncrementalFixture(t)
	db, err := store.OpenReadOnlyExisting(root, store.WorkspaceDBPath(root))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	files, err := store.FilesCountByLanguage(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if files["javascript"] == 0 || files["typescript"] == 0 {
		t.Fatalf("file language counts=%#v, want JavaScript and TypeScript entries", files)
	}
	symbols, err := store.SymbolsByFile(context.Background(), db, "src/demo/service.mjs", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{}
	for _, symbol := range symbols {
		paths = append(paths, symbol.FilePath+"::"+symbol.Name)
	}
	sort.Strings(paths)
	if len(paths) == 0 || paths[0] != "src/demo/service.mjs::runDemo" {
		t.Fatalf("service catalog paths=%#v", paths)
	}
}
