package indexer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
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
