package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestChooseDefaultEntrypointPrefersNonAuxiliarySolution(t *testing.T) {
	items := []model.WorkspaceEntrypoint{
		{ID: "docs-template", Kind: model.EntrypointKindProject, Path: ".docs/features/migracion_dotnet10/templates/csproj-template.csproj"},
		{ID: "backend-sln", Kind: model.EntrypointKindSolution, Path: "backend/Gastos.sln"},
	}

	chosen := chooseDefaultEntrypoint("backend", items)
	if chosen != "backend-sln" {
		t.Fatalf("expected backend solution to be the default entrypoint, got %q", chosen)
	}
}

func TestDetectWorkspaceLayoutKeepsDocsTemplateOutOfDefaultSelection(t *testing.T) {
	root := t.TempDir()
	mustCreateDir(t, filepath.Join(root, ".git"))
	mustCreateDir(t, filepath.Join(root, "backend"))
	mustCreateDir(t, filepath.Join(root, ".docs", "features", "migracion_dotnet10", "templates"))
	mustWriteFile(t, filepath.Join(root, "backend", "Gastos.sln"), "")
	mustWriteFile(t, filepath.Join(root, ".docs", "features", "migracion_dotnet10", "templates", "csproj-template.csproj"), "")

	registration, project, err := DetectWorkspaceLayout(root, "gastos-test")
	if err != nil {
		t.Fatalf("DetectWorkspaceLayout returned error: %v", err)
	}
	if registration.Kind != model.WorkspaceKindSingle {
		t.Fatalf("expected single workspace, got %q", registration.Kind)
	}
	if registration.Solution != "backend/Gastos.sln" {
		t.Fatalf("expected backend/Gastos.sln as default solution, got %q", registration.Solution)
	}
	if project.Project.DefaultEntrypoint == "" {
		t.Fatal("expected a default entrypoint to be selected")
	}
	entrypoint, ok := FindEntrypoint(project, project.Project.DefaultEntrypoint)
	if !ok {
		t.Fatalf("expected to resolve default entrypoint %q", project.Project.DefaultEntrypoint)
	}
	if entrypoint.Path != "backend/Gastos.sln" {
		t.Fatalf("expected backend/Gastos.sln as default entrypoint path, got %q", entrypoint.Path)
	}
	if len(project.Entrypoints) != 2 {
		t.Fatalf("expected both the real solution and docs template to remain visible, got %d entrypoints", len(project.Entrypoints))
	}
}

func mustCreateDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
}

func TestDetectWorkspaceLayoutDetectsPython(t *testing.T) {
	root := t.TempDir()
	mustCreateDir(t, filepath.Join(root, ".git"))
	mustWriteFile(t, filepath.Join(root, "pyproject.toml"), "[project]\nname = \"myapp\"")
	mustWriteFile(t, filepath.Join(root, "main.py"), "def main():\n    pass")

	registration, _, err := DetectWorkspaceLayout(root, "pytest")
	if err != nil {
		t.Fatalf("DetectWorkspaceLayout returned error: %v", err)
	}
	foundPython := false
	for _, lang := range registration.Languages {
		if lang == "python" {
			foundPython = true
		}
	}
	if !foundPython {
		t.Fatalf("expected python in languages, got %v", registration.Languages)
	}
}
