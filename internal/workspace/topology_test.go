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

func TestDetectWorkspaceLayoutDetectsGo(t *testing.T) {
	root := t.TempDir()
	mustCreateDir(t, filepath.Join(root, ".git"))
	mustWriteFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	mustWriteFile(t, filepath.Join(root, "main.go"), "package main\n\nfunc main() {}\n")

	registration, project, err := DetectWorkspaceLayout(root, "gotest")
	if err != nil {
		t.Fatalf("DetectWorkspaceLayout returned error: %v", err)
	}
	if !hasLanguage(registration.Languages, "go") {
		t.Fatalf("expected go in registration languages, got %v", registration.Languages)
	}
	if !hasLanguage(project.Project.Languages, "go") {
		t.Fatalf("expected go in project languages, got %v", project.Project.Languages)
	}
}

func TestDetectWorkspaceLayoutAcceptsMarkdownProjectConfigWithoutCodeMarkers(t *testing.T) {
	root := t.TempDir()
	if err := SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{
			Name:      "knowledge-base",
			Languages: []string{"markdown"},
			Kind:      model.WorkspaceKindSingle,
		},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	registration, project, err := DetectWorkspaceLayout(root, "memoria-karen")
	if err != nil {
		t.Fatalf("DetectWorkspaceLayout returned error: %v", err)
	}
	if registration.Name != "memoria-karen" {
		t.Fatalf("expected explicit workspace name, got %q", registration.Name)
	}
	if registration.Kind != model.WorkspaceKindSingle {
		t.Fatalf("expected single workspace, got %q", registration.Kind)
	}
	if !hasLanguage(registration.Languages, "markdown") {
		t.Fatalf("expected markdown in registration languages, got %v", registration.Languages)
	}
	if project.Project.Name != "memoria-karen" {
		t.Fatalf("expected explicit project name, got %q", project.Project.Name)
	}
	if len(project.Repos) != 1 {
		t.Fatalf("expected a default repo for configured workspace, got %#v", project.Repos)
	}
	if project.Repos[0].Root != "." {
		t.Fatalf("expected default repo root '.', got %q", project.Repos[0].Root)
	}
	if !hasLanguage(project.Repos[0].Languages, "markdown") {
		t.Fatalf("expected markdown in default repo languages, got %v", project.Repos[0].Languages)
	}
}

func TestLoadProjectTopologyMergesDetectedGoIntoExistingProjectFile(t *testing.T) {
	root := t.TempDir()
	mustCreateDir(t, filepath.Join(root, ".git"))
	mustWriteFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	mustWriteFile(t, filepath.Join(root, "main.go"), "package main\n\nfunc main() {}\n")
	registration := model.WorkspaceRegistration{
		Name:      "mi-lsp",
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}
	if err := SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{
			Name:        "mi-lsp",
			Languages:   []string{"csharp"},
			Kind:        model.WorkspaceKindSingle,
			DefaultRepo: "mi-lsp",
		},
		Repos: []model.WorkspaceRepo{{
			ID:        "mi-lsp",
			Name:      "mi-lsp",
			Root:      ".",
			Languages: []string{"csharp"},
		}},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	project, err := LoadProjectTopology(root, registration)
	if err != nil {
		t.Fatalf("LoadProjectTopology: %v", err)
	}
	if !hasLanguage(project.Project.Languages, "go") {
		t.Fatalf("expected merged go project language, got %v", project.Project.Languages)
	}
	if len(project.Repos) != 1 || !hasLanguage(project.Repos[0].Languages, "go") {
		t.Fatalf("expected merged go repo language, got %#v", project.Repos)
	}
}

func TestLoadProjectTopologyUsesCompleteProjectFileWithoutRedetecting(t *testing.T) {
	root := t.TempDir()
	mustCreateDir(t, filepath.Join(root, ".git"))
	mustWriteFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	mustWriteFile(t, filepath.Join(root, "main.go"), "package main\n\nfunc main() {}\n")
	registration := model.WorkspaceRegistration{
		Name:      "cached",
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}
	if err := SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{
			Name:              "cached",
			Languages:         []string{"csharp"},
			Kind:              model.WorkspaceKindSingle,
			DefaultRepo:       "cached",
			DefaultEntrypoint: "cached::app-sln",
		},
		Repos: []model.WorkspaceRepo{{
			ID:                "cached",
			Name:              "cached",
			Root:              ".",
			Languages:         []string{"csharp"},
			DefaultEntrypoint: "cached::app-sln",
		}},
		Entrypoints: []model.WorkspaceEntrypoint{{
			ID:      "cached::app-sln",
			RepoID:  "cached",
			Path:    "App.sln",
			Kind:    model.EntrypointKindSolution,
			Default: true,
		}},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	project, err := LoadProjectTopology(root, registration)
	if err != nil {
		t.Fatalf("LoadProjectTopology: %v", err)
	}
	if hasLanguage(project.Project.Languages, "go") {
		t.Fatalf("complete project file should be authoritative and avoid redetecting go, got %v", project.Project.Languages)
	}
	if len(project.Repos) != 1 || hasLanguage(project.Repos[0].Languages, "go") {
		t.Fatalf("complete project file should preserve cached repo languages, got %#v", project.Repos)
	}
	if len(project.Entrypoints) != 1 || project.Entrypoints[0].ID != "cached::app-sln" {
		t.Fatalf("complete project file should preserve cached entrypoints, got %#v", project.Entrypoints)
	}
}

func hasLanguage(languages []string, expected string) bool {
	for _, language := range languages {
		if language == expected {
			return true
		}
	}
	return false
}
