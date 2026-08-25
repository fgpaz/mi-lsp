package indexer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestCatalogIndexProgressReportsAndCanCancel(t *testing.T) {
	root := t.TempDir()
	project := model.ProjectFile{
		Project: model.ProjectBlock{
			Name:              "progress-test",
			Kind:              model.WorkspaceKindSingle,
			DefaultRepo:       "main",
			DefaultEntrypoint: "main::src-app-csproj",
			Languages:         []string{"csharp"},
		},
		Repos: []model.WorkspaceRepo{{
			ID:                "main",
			Name:              "main",
			Root:              ".",
			DefaultEntrypoint: "main::src-app-csproj",
			Languages:         []string{"csharp"},
		}},
		Entrypoints: []model.WorkspaceEntrypoint{{
			ID:      "main::src-app-csproj",
			RepoID:  "main",
			Path:    "src/App.csproj",
			Kind:    model.EntrypointKindProject,
			Default: true,
		}},
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	writeProgressTestFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeProgressTestFile(t, root, "src/App.cs", "namespace Demo; public class App { }\n")
	writeProgressTestFile(t, root, "src/Other.cs", "namespace Demo; public class Other { }\n")

	stop := errors.New("stop indexing")
	events := make([]Progress, 0)
	_, err := IndexWorkspaceCatalogOnlyWithProgress(context.Background(), root, false, "", func(ctx context.Context, progress Progress) error {
		events = append(events, progress)
		if progress.Stage == "catalog.read" && progress.Files >= 1 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) {
		t.Fatalf("IndexWorkspaceCatalogOnlyWithProgress error = %v, want %v", err, stop)
	}
	if len(events) == 0 {
		t.Fatal("expected progress events")
	}
	foundTotal := false
	for _, event := range events {
		if event.Stage == "catalog.read" && event.FilesTotal >= 2 {
			foundTotal = true
			break
		}
	}
	if !foundTotal {
		t.Fatalf("events did not include catalog total: %#v", events)
	}
}

func TestDocsOnlyIndexDoesNotRequireCodeProjectMarkers(t *testing.T) {
	root := t.TempDir()
	writeProgressTestFile(t, root, ".docs/wiki/00_gobierno_documental.md", "# Gobierno documental\n")

	result, err := IndexWorkspaceDocsOnlyWithProgress(context.Background(), root, "", nil)
	if err != nil {
		t.Fatalf("IndexWorkspaceDocsOnlyWithProgress returned error: %v", err)
	}
	if result.Docs == 0 {
		t.Fatal("expected docs-only index to publish documentation records")
	}
}

func TestDocsOnlyIndexIncludesDeclaredCanonRelativePaths(t *testing.T) {
	parent := t.TempDir()
	code := filepath.Join(parent, "code")
	canon := filepath.Join(parent, "wiki-repo", "Ingenieria")
	if err := os.MkdirAll(code, 0o755); err != nil {
		t.Fatalf("mkdir code: %v", err)
	}
	writeProgressTestFile(t, code, "README.md", "# code\n")
	writeProgressTestFile(t, canon, "foo.md", "# canon foo\n")
	if err := workspace.SaveProjectFile(code, model.ProjectFile{
		Project: model.ProjectBlock{Name: "code", Kind: model.WorkspaceKindSingle},
		Canons:  []model.WorkspaceCanon{{ID: "wiki", Root: "../wiki-repo/Ingenieria", Role: "producto"}},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	result, err := IndexWorkspaceDocsOnly(context.Background(), code)
	if err != nil {
		t.Fatalf("IndexWorkspaceDocsOnly: %v", err)
	}
	if result.Docs == 0 {
		t.Fatal("expected docs-only index to publish records")
	}

	db, err := store.Open(code)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	docs, err := store.ListDocRecords(context.Background(), db)
	if err != nil {
		t.Fatalf("ListDocRecords: %v", err)
	}
	found := false
	for _, doc := range docs {
		if doc.Path == "../wiki-repo/Ingenieria/foo.md" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ListDocRecords missing canon relative path, got %#v", docs)
	}
}

func TestCanonicalGraphDocAcceptsKnowledgeWikiPaths(t *testing.T) {
	cases := map[string]bool{
		"wiki/10-chiamo.md":                    true,
		"bibliotecas/memorias/ficha.md":        true,
		".docs/wiki/00_gobierno_documental.md": true,
		".docs/raw/plans/x.md":                 false,
		"src/main.go":                          false,
	}
	for path, want := range cases {
		if got := isCanonicalGraphDoc(path, false); got != want {
			t.Fatalf("isCanonicalGraphDoc(%q)=%v want %v", path, got, want)
		}
	}
}

func TestWalkWorkspaceIgnoresNestedMiLspState(t *testing.T) {
	root := t.TempDir()
	writeProgressTestFile(t, root, "src/App.cs", "namespace Demo; public class App { }\n")
	writeProgressTestFile(t, root, "repo/.mi-lsp/generated.cs", "namespace Operational; public class Generated { }\n")

	matcher, err := workspace.LoadIgnoreMatcher(root, nil)
	if err != nil {
		t.Fatalf("LoadIgnoreMatcher: %v", err)
	}
	files, err := WalkWorkspace(context.Background(), root, matcher)
	if err != nil {
		t.Fatalf("WalkWorkspace: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one indexed source file, got %#v", files)
	}
	if strings.Contains(filepath.ToSlash(files[0]), "/.mi-lsp/") {
		t.Fatalf("WalkWorkspace returned operational state: %#v", files)
	}
}

func writeProgressTestFile(t *testing.T, root string, relativePath string, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", relativePath, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relativePath, err)
	}
}

func TestDocsOnlyIndexWarnsWhenRepositoryIdentityIsUnavailable(t *testing.T) {
	root := t.TempDir()
	writeProgressTestFile(t, root, ".docs/wiki/00_gobierno_documental.md", "# Gobierno documental\n")
	result, err := IndexWorkspaceDocsOnlyWithProgress(context.Background(), root, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	const expected = "documentation graph omitted: repository identity unavailable; configure repository_identity or exactly one origin remote"
	found := false
	for _, warning := range result.Warnings {
		if warning == expected {
			found = true
		}
		if strings.Contains(warning, filepath.ToSlash(root)) || strings.Contains(warning, root) {
			t.Fatalf("warning leaked workspace path: %q", warning)
		}
	}
	if !found {
		t.Fatalf("identity warning missing from %v", result.Warnings)
	}
}

func TestDocumentationGraphRequestAcceptsExplicitRepositoryIdentity(t *testing.T) {
	project := model.ProjectFile{Repos: []model.WorkspaceRepo{{ID: "main", RepositoryIdentity: "HTTPS://Example.com/acme/repo.git"}}}
	docs := []model.DocRecord{{Path: "wiki/00-person.md"}}
	request, publish, warning := documentationGraphRequest(context.Background(), t.TempDir(), project, nil, docs, nil, nil, time.Unix(1, 0))
	if !publish || warning != "" {
		t.Fatalf("publish=%v warning=%q", publish, warning)
	}
	if request.RepositoryIdentity != "example.com/acme/repo" {
		t.Fatalf("repository identity=%q", request.RepositoryIdentity)
	}
}
