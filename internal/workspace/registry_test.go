package workspace

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestResolveWorkspaceSelectionPrefersCallerCWDOverLastWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	otherRoot := t.TempDir()
	callerRoot := t.TempDir()
	mustCreateDir(t, filepath.Join(callerRoot, "src", "backend"))

	if err := SaveProjectFile(callerRoot, model.ProjectFile{
		Project: model.ProjectBlock{
			Name:      "interbancarizacion_coelsa",
			Languages: []string{"csharp"},
			Kind:      model.WorkspaceKindSingle,
		},
	}); err != nil {
		t.Fatalf("SaveProjectFile(callerRoot): %v", err)
	}

	registerTestWorkspace(t, "interbancarizacion_coelsa", callerRoot)
	registerTestWorkspace(t, "mis-cals", otherRoot)

	resolution, err := ResolveWorkspaceSelection("", filepath.Join(callerRoot, "src", "backend"))
	if err != nil {
		t.Fatalf("ResolveWorkspaceSelection: %v", err)
	}
	if resolution.Registration.Name != "interbancarizacion_coelsa" {
		t.Fatalf("Registration.Name = %q, want interbancarizacion_coelsa", resolution.Registration.Name)
	}
	if resolution.Source != ResolutionSourceCallerCWD {
		t.Fatalf("Source = %q, want %q", resolution.Source, ResolutionSourceCallerCWD)
	}
	if len(resolution.Warnings) != 0 {
		t.Fatalf("Warnings = %v, want none", resolution.Warnings)
	}
}

func TestResolveWorkspaceSelectionUsesProjectNameForSameRootAliases(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := t.TempDir()
	mustCreateDir(t, filepath.Join(root, "src"))

	if err := SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{
			Name:      "interbancarizacion_coelsa",
			Languages: []string{"csharp"},
			Kind:      model.WorkspaceKindSingle,
		},
	}); err != nil {
		t.Fatalf("SaveProjectFile(root): %v", err)
	}

	registerTestWorkspace(t, "coelsa", root)
	registerTestWorkspace(t, "interbanc-parent", root)
	registerTestWorkspace(t, "interbancarizacion_coelsa", root)

	resolution, err := ResolveWorkspaceSelection("", filepath.Join(root, "src"))
	if err != nil {
		t.Fatalf("ResolveWorkspaceSelection: %v", err)
	}
	if resolution.Registration.Name != "interbancarizacion_coelsa" {
		t.Fatalf("Registration.Name = %q, want interbancarizacion_coelsa", resolution.Registration.Name)
	}
	if resolution.Source != ResolutionSourceCallerCWD {
		t.Fatalf("Source = %q, want %q", resolution.Source, ResolutionSourceCallerCWD)
	}
	if len(resolution.Warnings) == 0 {
		t.Fatal("expected ambiguity warning for same-root aliases")
	}
	if !strings.Contains(strings.Join(resolution.Warnings, " "), "multiple registry aliases") {
		t.Fatalf("Warnings = %v, want multiple registry aliases message", resolution.Warnings)
	}
}

func TestResolveWorkspaceSelectionFallsBackToLastWorkspaceWhenCWDDoesNotMatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := t.TempDir()
	unmatched := t.TempDir()
	registerTestWorkspace(t, "mis-cals", root)

	resolution, err := ResolveWorkspaceSelection("", unmatched)
	if err != nil {
		t.Fatalf("ResolveWorkspaceSelection: %v", err)
	}
	if resolution.Registration.Name != "mis-cals" {
		t.Fatalf("Registration.Name = %q, want mis-cals", resolution.Registration.Name)
	}
	if resolution.Source != ResolutionSourceLastWorkspace {
		t.Fatalf("Source = %q, want %q", resolution.Source, ResolutionSourceLastWorkspace)
	}
	if len(resolution.Warnings) == 0 {
		t.Fatal("expected last_workspace fallback warning")
	}
	if !strings.Contains(strings.Join(resolution.Warnings, " "), "last_workspace") {
		t.Fatalf("Warnings = %v, want last_workspace message", resolution.Warnings)
	}
}

func registerTestWorkspace(t *testing.T, alias string, root string) {
	t.Helper()
	if _, err := RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace(%s): %v", alias, err)
	}
	t.Cleanup(func() {
		_ = RemoveWorkspace(alias)
	})
}
