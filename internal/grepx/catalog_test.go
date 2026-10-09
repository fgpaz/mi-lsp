package grepx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestResolveCatalogTargetUsesRegisteredAliasWithoutIndex(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := t.TempDir()
	subdir := filepath.Join(root, "src")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := "grep-alias"
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{Name: alias, Root: root, Kind: model.WorkspaceKindSingle}); err != nil {
		t.Fatal(err)
	}

	target := resolveCatalogTarget(filepath.Join(subdir, "main.go"), subdir)
	if !target.registered || target.alias != alias || target.root != root || target.indexed {
		t.Fatalf("target=%+v, want registered alias %q with no index", target, alias)
	}
	notice := catalogTargetNotice(target)
	if !strings.Contains(notice, "índice ausente") || !strings.Contains(notice, "--workspace '"+alias+"'") {
		t.Fatalf("notice=%q, want actionable alias and index command", notice)
	}
	if _, err := os.Stat(filepath.Join(root, ".mi-lsp", "index.db")); !os.IsNotExist(err) {
		t.Fatalf("lookup created or unexpectedly found index: %v", err)
	}
}

func TestResolveCatalogTargetSuggestsExplicitRegistrationWhenUnscoped(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	target := resolveCatalogTarget(root, root)
	if target.registered || target.root != root {
		t.Fatalf("target=%+v, want unregistered candidate root %q", target, root)
	}
	notice := catalogTargetNotice(target)
	for _, want := range []string{"workspace no registrado", "mi-lsp workspace add", "--no-index", "mi-lsp index --workspace"} {
		if !strings.Contains(notice, want) {
			t.Errorf("notice=%q lacks %q", notice, want)
		}
	}
	registry, err := workspace.LoadRegistryReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Workspaces) != 0 {
		t.Fatalf("diagnostic registered a workspace: %+v", registry.Workspaces)
	}
}
