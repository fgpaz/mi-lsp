package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestNavAffectedFromGitDiffIncludesUntrackedGoFileTestAndDocs(t *testing.T) {
	alias := "affected-ws-" + filepath.Base(t.TempDir())
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

	writeWorkspaceFile(t, root, "internal/service/affected.go", "package service\n\nfunc ChangedImpactSelector() {}\n")

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.affected",
		Context:   model.QueryOptions{Workspace: alias},
		Payload: map[string]any{
			"from_git_diff": true,
			"changed_ref":   "HEAD",
			"include_tests": true,
			"include_docs":  true,
		},
	})
	if err != nil {
		t.Fatalf("nav.affected: %v", err)
	}

	items := affectedItemsFromEnvelope(t, env)
	assertAffectedItem(t, items, "code", "internal/service/affected.go", "")
	assertAffectedItem(t, items, "test", "internal/service", "go test ./internal/service")
	assertAffectedItem(t, items, "doc", ".docs/wiki/04_RF/RF-QRY-017.md", "")
	if !containsWarning(env.Warnings, affectedHeuristicWarning) {
		t.Fatalf("expected heuristic warning, got %#v", env.Warnings)
	}
}

func TestNavAffectedParsesStdinAndUsesOverrideTestCommand(t *testing.T) {
	root, alias := setupTestWorkspace(t)
	app := New(root, nil)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.affected",
		Context:   model.QueryOptions{Workspace: alias},
		Payload: map[string]any{
			"stdin":         `["internal/store/schema.go","internal/store/schema.go","internal/cli/nav.go"]`,
			"include_tests": true,
			"include_docs":  true,
			"test_command":  "go test ./internal/store -run TestStore",
		},
	})
	if err != nil {
		t.Fatalf("nav.affected: %v", err)
	}

	items := affectedItemsFromEnvelope(t, env)
	assertAffectedItem(t, items, "code", "internal/store/schema.go", "")
	assertAffectedItem(t, items, "code", "internal/cli/nav.go", "")
	assertAffectedItem(t, items, "test", "internal/store", "go test ./internal/store -run TestStore")
	assertAffectedItem(t, items, "doc", ".docs/wiki/08_modelo_fisico_datos.md", "")
	assertAffectedItem(t, items, "doc", ".docs/wiki/09_contratos/CT-CLI-DAEMON-ADMIN.md", "")
	if len(items) == 0 {
		t.Fatal("expected affected items")
	}
}

func TestNavAffectedNoChangesQuietHasNoHint(t *testing.T) {
	alias := "affected-clean-ws-" + filepath.Base(t.TempDir())
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

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.affected",
		Context:   model.QueryOptions{Workspace: alias},
		Payload: map[string]any{
			"from_git_diff": true,
			"changed_ref":   "HEAD",
			"quiet":         true,
		},
	})
	if err != nil {
		t.Fatalf("nav.affected: %v", err)
	}
	items := affectedItemsFromEnvelope(t, env)
	if len(items) != 0 {
		t.Fatalf("expected no affected items, got %#v", items)
	}
	if env.Hint != "" {
		t.Fatalf("quiet no-change hint = %q, want empty", env.Hint)
	}
	if !containsWarning(env.Warnings, "no affected paths detected") {
		t.Fatalf("expected no-change warning, got %#v", env.Warnings)
	}
}

func TestNavAffectedUsesPublishedGoGraphForCallerImpact(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	alias := "affected-graph-" + filepath.Base(root)
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", Languages: []string{"go"}},
		Repos:   []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/" + alias, Languages: []string{"go"}}},
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	writeWorkspaceFile(t, root, "go.mod", "module example.com/affectedgraph\n\ngo 1.23\n")
	writeWorkspaceFile(t, root, "subject.go", "package affectedgraph\n\nfunc Subject() string { return \"ok\" }\n")
	writeWorkspaceFile(t, root, "caller.go", "package affectedgraph\n\nfunc Caller() string { return Subject() }\n")
	writeWorkspaceFile(t, root, "caller_test.go", "package affectedgraph\n\nimport \"testing\"\n\nfunc TestCaller(t *testing.T) { if Caller() != \"ok\" { t.Fatal(\"unexpected\") } }\n")
	if _, err := indexer.IndexWorkspaceWithGraphProgress(context.Background(), root, true, "", nil, indexer.GraphIndexOptions{}); err != nil {
		t.Fatalf("IndexWorkspaceWithGraphProgress: %v", err)
	}
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{Name: alias, Root: root, Languages: []string{"go"}, Kind: model.WorkspaceKindSingle}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })

	env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.affected",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"paths": []string{"subject.go"}, "include_tests": true},
	})
	if err != nil {
		t.Fatalf("nav.affected: %v", err)
	}
	if env.Backend != "graph-native" && env.Backend != "graph-native+heuristic" {
		t.Fatalf("backend = %q, want graph-native", env.Backend)
	}
	items := affectedItemsFromEnvelope(t, env)
	var caller *AffectedItem
	for i := range items {
		if items[i].Path == "caller.go" {
			caller = &items[i]
			break
		}
	}
	if caller == nil {
		t.Fatalf("missing graph caller impact in %#v", items)
	}
	if caller.CrossRID == "" || len(caller.EvidencePath) == 0 {
		t.Fatalf("caller lacks graph proof: %#v", caller)
	}
	if caller.ConfidenceClass != "exact" && caller.ConfidenceClass != "extracted" {
		t.Fatalf("caller confidence = %q, want exact or extracted", caller.ConfidenceClass)
	}
	for _, item := range items {
		if item.Path == "unrelated.go" {
			t.Fatalf("unrelated graph positive: %#v", item)
		}
	}

	massivePaths := make([]string, model.MaxImpactSeeds+1)
	massivePaths[0] = "subject.go"
	for i := 1; i < len(massivePaths); i++ {
		massivePaths[i] = fmt.Sprintf("missing/%04d.go", i)
	}
	limited, err := New(root, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.affected",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"paths": massivePaths},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !limited.Truncated || limited.GenerationID == "" || !containsWarning(limited.Warnings, "impact seed input was bounded") {
		t.Fatalf("outer impact truncation was not propagated: %#v", limited)
	}
}

func TestNavAffectedBlocksTypedStaleGraph(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	alias := "affected-stale-" + filepath.Base(root)
	project := model.ProjectFile{Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", Languages: []string{"go"}}, Repos: []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/" + alias, Languages: []string{"go"}}}}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatal(err)
	}
	writeWorkspaceFile(t, root, "go.mod", "module example.com/affectedstale\n\ngo 1.23\n")
	writeWorkspaceFile(t, root, "subject.go", "package affectedstale\nfunc Subject() {}\n")
	if _, err := indexer.IndexWorkspaceWithGraphProgress(context.Background(), root, true, "", nil, indexer.GraphIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{Name: alias, Root: root, Languages: []string{"go"}, Kind: model.WorkspaceKindSingle}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.SetGraphRuntimeState(context.Background(), db, store.GraphRuntimeStale, ""); err != nil {
		t.Fatal(err)
	}

	_, err = New(root, nil).Execute(context.Background(), model.CommandRequest{Operation: "nav.affected", Context: model.QueryOptions{Workspace: alias}, Payload: map[string]any{"paths": []string{"subject.go"}}})
	var graphErr *model.GraphQueryError
	if !errors.As(err, &graphErr) || graphErr.Code != "GPH_IMPACT_GRAPH_STALE" {
		t.Fatalf("nav.affected error = %v, want typed GPH_IMPACT_GRAPH_STALE", err)
	}
}

func affectedItemsFromEnvelope(t *testing.T, env model.Envelope) []AffectedItem {
	t.Helper()
	items, ok := env.Items.([]AffectedItem)
	if !ok {
		t.Fatalf("expected []AffectedItem, got %#v", env.Items)
	}
	return items
}

func assertAffectedItem(t *testing.T, items []AffectedItem, kind string, path string, suggestedCommand string) {
	t.Helper()
	for _, item := range items {
		if item.Kind != kind || item.Path != path {
			continue
		}
		if suggestedCommand != "" && item.SuggestedCommand != suggestedCommand {
			t.Fatalf("item %s/%s command = %q, want %q", kind, path, item.SuggestedCommand, suggestedCommand)
		}
		if item.Reason == "" || item.Confidence <= 0 {
			t.Fatalf("item %s/%s missing stable reason/confidence: %#v", kind, path, item)
		}
		return
	}
	t.Fatalf("missing affected item kind=%s path=%s command=%s in %#v", kind, path, suggestedCommand, items)
}
