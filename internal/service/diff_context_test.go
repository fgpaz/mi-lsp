package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func createDiffWorkspaceFixture(t *testing.T, alias string) string {
	t.Helper()
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, "src/S1.cs", strings.Join([]string{
		"namespace Demo;",
		"public class SvcOne",
		"{",
		"    public void Alpha() { }",
		"}",
	}, "\n"))
	runGit(t, root, "init")
	runGit(t, root, "add", ".")
	runGit(t, root, "-c", "user.name=smoke", "-c", "user.email=smoke@example.com", "commit", "-m", "init")
	return root
}

func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out)
}

func TestNavDiffContextIncludesStagedAddedAndDeletedFiles(t *testing.T) {
	alias := "diff-ws-" + filepath.Base(t.TempDir())
	root := createDiffWorkspaceFixture(t, alias)
	app := New(root, nil)

	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	}); err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	writeWorkspaceFile(t, root, "src/Added.cs", strings.Join([]string{
		"namespace Demo;",
		"public class AddedOne",
		"{",
		"}",
	}, "\n"))
	if err := os.Remove(filepath.Join(root, "src", "S1.cs")); err != nil {
		t.Fatalf("Remove S1.cs: %v", err)
	}
	runGit(t, root, "add", "-A", "src")

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.diff-context",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{},
	})
	if err != nil {
		t.Fatalf("nav.diff-context: %v", err)
	}

	items, ok := env.Items.([]DiffContextResult)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one diff result, got %#v", env.Items)
	}
	if items[0].ChangedFiles == 0 {
		t.Fatalf("changed_files = 0, want > 0")
	}
	if len(items[0].ChangedSymbols) == 0 {
		t.Fatalf("changed_symbols empty, want at least one entry")
	}

	var sawAdded bool
	for _, sym := range items[0].ChangedSymbols {
		if sym.ChangeType == "added" && sym.File == "src/Added.cs" {
			sawAdded = true
			break
		}
	}
	if !sawAdded {
		t.Fatalf("expected added symbol for src/Added.cs, got %#v", items[0].ChangedSymbols)
	}
}
