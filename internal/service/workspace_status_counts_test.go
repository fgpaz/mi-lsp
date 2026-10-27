package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/output"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestWorkspaceStatusCountsWithManyEntrypoints(t *testing.T) {
	root, name := setupTestWorkspace(t)
	project := testProject(name)
	project.Entrypoints = nil
	project.Project.DefaultEntrypoint = ""
	project.Repos[0].DefaultEntrypoint = ""
	for i := 0; i < 87; i++ {
		project.Entrypoints = append(project.Entrypoints, model.WorkspaceEntrypoint{ID: fmt.Sprintf("main::%d", i), RepoID: "main", Path: fmt.Sprintf("src/%d.csproj", i), Kind: model.EntrypointKindSolution})
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	err = store.ReplaceCatalog(ctx, db, project, []model.FileRecord{{FilePath: "src/A.cs", RepoID: "main", Language: "csharp"}}, []model.SymbolRecord{{FilePath: "src/A.cs", RepoID: "main", Name: "A", Kind: "class", Language: "csharp"}})
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	app := New(root, nil)
	for _, omit := range []bool{false, true} {
		env, err := app.Execute(ctx, model.CommandRequest{Operation: "workspace.status", Context: model.QueryOptions{Workspace: name}, Payload: map[string]any{"auto_sync": false, "no_entrypoints": omit}})
		if err != nil {
			t.Fatal(err)
		}
		item := env.Items.([]any)[0].(map[string]any)
		if item["entrypoint_count"] != 87 || env.Stats.Files != 1 || env.Stats.Symbols != 1 {
			t.Fatalf("counts: item=%v stats=%+v", item["entrypoint_count"], env.Stats)
		}
		_, present := item["entrypoints"]
		if present == omit {
			t.Fatalf("entrypoints present=%v omit=%v", present, omit)
		}
		limited := output.ApplyEnvelopeLimits(env, model.QueryOptions{MaxChars: 1})
		if !limited.Truncated || limited.Stats.Files != 1 || limited.Stats.Symbols != 1 || limited.Items.([]any)[0].(map[string]any)["entrypoint_count"] != 87 {
			t.Fatalf("limited counts: %+v", limited)
		}
	}
}
