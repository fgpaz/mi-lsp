package workspace

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestDetectCargoPackageKeepsSourceTreeInRootRepo(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Cargo.toml"), []byte("[package]\nname = \"rust-app\"\nversion = \"0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.rs"), []byte("fn main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	registration, project, err := DetectWorkspaceLayout(root, "rust-package")
	if err != nil {
		t.Fatalf("DetectWorkspaceLayout: %v", err)
	}
	if registration.Kind != model.WorkspaceKindSingle || len(project.Repos) != 1 || project.Repos[0].Root != "." {
		t.Fatalf("registration/project = %#v / %#v, want one root Cargo package", registration, project)
	}
	if !slices.Contains(registration.Languages, "rust") {
		t.Fatalf("languages = %v, want rust", registration.Languages)
	}
}

func TestDetectCargoWorkspaceAndMemberManifests(t *testing.T) {
	root := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Cargo.toml", "[workspace]\nmembers = [\"crates/core\"]\nresolver = \"2\"\n")
	write("crates/core/Cargo.toml", "[package]\nname = \"core\"\nversion = \"0.1.0\"\n")
	write("crates/core/src/lib.rs", "pub fn run() {}\n")

	registration, project, err := DetectWorkspaceLayout(root, "rust-fixture")
	if err != nil {
		t.Fatalf("DetectWorkspaceLayout: %v", err)
	}
	if registration.Kind != "single" {
		t.Fatalf("workspace kind = %q, want single Cargo workspace", registration.Kind)
	}
	if !slices.Contains(registration.Languages, "rust") {
		t.Fatalf("languages = %v, want rust", registration.Languages)
	}
	if len(project.Repos) != 1 || project.Repos[0].Root != "." {
		t.Fatalf("repos = %#v, want one root repo", project.Repos)
	}
	manifests := map[string]bool{}
	for _, entrypoint := range project.Entrypoints {
		manifests[entrypoint.Path] = true
	}
	if !manifests["Cargo.toml"] || !manifests["crates/core/Cargo.toml"] {
		t.Fatalf("Cargo entrypoints = %v, want root and workspace member manifests", manifests)
	}
}
