package indexer

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestFullIndexPublishesLocalGoGraph(t *testing.T) {
	root := t.TempDir()
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "graph-fixture", Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", Languages: []string{"go"}},
		Repos:   []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/graph-fixture", Languages: []string{"go"}}},
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatal(err)
	}
	writeProgressTestFile(t, root, "go.mod", "module example.com/graph-fixture\n\ngo 1.23\n")
	writeProgressTestFile(t, root, "main.go", "package main\nfunc main() {}\n")

	result, err := IndexWorkspace(context.Background(), root, true)
	if err != nil {
		t.Fatalf("IndexWorkspace: %v", err)
	}
	if result.GraphGenerationID == "" || result.GraphBackendManifest == "" {
		t.Fatalf("graph metadata = %#v, want active graph generation", result)
	}

	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	active, ok, err := store.ActiveGraphGeneration(context.Background(), db)
	if err != nil || !ok || active.String() != result.GraphGenerationID {
		t.Fatalf("active graph = %s, %v, want %s", active, err, result.GraphGenerationID)
	}
}

func TestObserveGraphUsesExactRoslynProjectEntrypoint(t *testing.T) {
	project := model.ProjectFile{
		Project:     model.ProjectBlock{Name: "cs-fixture", Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", DefaultEntrypoint: "project"},
		Repos:       []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://github.com/acme/repo.git", Languages: []string{"csharp"}}},
		Entrypoints: []model.WorkspaceEntrypoint{{ID: "project", RepoID: "repo", Path: "src/Fixture.csproj", Kind: model.EntrypointKindProject, Default: true}},
	}
	var got GraphObservationRequest
	batch := stagingBatch("roslyn", "src/Fixture.csproj", false)
	batch.WorkspaceIdentity = batch.RepositoryIdentity
	if err := model.SealGraphObservationBatch(&batch); err != nil {
		t.Fatal(err)
	}
	if err := batch.Validate(); err != nil {
		t.Fatal(err)
	}
	batches, _, _, err := ObserveGraph(context.Background(), t.TempDir(), project, GraphIndexOptions{RoslynObserver: func(_ context.Context, request GraphObservationRequest) (model.GraphObservationBatch, error) {
		got = request
		return batch, nil
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := GraphObservationRequest{RepositoryIdentity: "github.com/acme/repo", EntrypointID: "project", EntrypointPath: "src/Fixture.csproj", EntrypointKind: model.EntrypointKindProject, Backend: "roslyn"}
	if got.RepositoryIdentity != want.RepositoryIdentity || got.EntrypointID != want.EntrypointID || got.EntrypointPath != want.EntrypointPath || got.EntrypointKind != want.EntrypointKind || got.Backend != want.Backend || len(batches) != 1 {
		t.Fatalf("request=%#v batches=%d", got, len(batches))
	}
}

func TestObserveGraphRejectsMissingOrPartialRoslynBatch(t *testing.T) {
	project := model.ProjectFile{
		Project:     model.ProjectBlock{Name: "cs-fixture", Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", DefaultEntrypoint: "project"},
		Repos:       []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/cs-fixture", Languages: []string{"csharp"}}},
		Entrypoints: []model.WorkspaceEntrypoint{{ID: "project", RepoID: "repo", Path: "src/Fixture.csproj", Kind: model.EntrypointKindProject, Default: true}},
	}
	for _, observer := range []GraphObserver{
		nil,
		func(context.Context, GraphObservationRequest) (model.GraphObservationBatch, error) {
			return model.GraphObservationBatch{}, nil
		},
		func(context.Context, GraphObservationRequest) (model.GraphObservationBatch, error) {
			return model.GraphObservationBatch{}, errors.New("backend failed")
		},
	} {
		_, _, _, err := ObserveGraph(context.Background(), t.TempDir(), project, GraphIndexOptions{RoslynObserver: observer}, nil)
		if err == nil {
			t.Fatal("expected invalid Roslyn observation error")
		}
	}
}

func TestFullIndexObservationFailurePreservesPriorFreshGraph(t *testing.T) {
	root := t.TempDir()
	project := model.ProjectFile{Project: model.ProjectBlock{Name: "go-fixture", Kind: model.WorkspaceKindSingle, DefaultRepo: "repo"}, Repos: []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/go-fixture", Languages: []string{"go"}}}}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatal(err)
	}
	writeProgressTestFile(t, root, "go.mod", "module example.com/go-fixture\n\ngo 1.23\n")
	writeProgressTestFile(t, root, "main.go", "package main\nfunc main() {}\n")
	if _, err := IndexWorkspace(context.Background(), root, true); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	before, ok, err := store.ActiveGraphGeneration(context.Background(), db)
	if err != nil || !ok {
		t.Fatal("missing fresh graph")
	}
	writeProgressTestFile(t, root, "go.mod", "not a module")
	if _, err := IndexWorkspace(context.Background(), root, true); err == nil {
		t.Fatal("expected observation failure")
	}
	after, ok, err := store.ActiveGraphGeneration(context.Background(), db)
	if err != nil || !ok || !reflect.DeepEqual(before, after) {
		t.Fatalf("graph pointer changed after failed observation: before=%v after=%v err=%v", before, after, err)
	}
	if state, err := store.GraphRuntimeState(context.Background(), db); err != nil || state != store.GraphRuntimeFresh {
		t.Fatalf("state=%q err=%v", state, err)
	}
}

func TestObserveGraphGatesUnsupportedLanguageWithoutClaims(t *testing.T) {
	root := t.TempDir()
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "ts-fixture", Kind: model.WorkspaceKindSingle, DefaultRepo: "repo"},
		Repos:   []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/ts-fixture", Languages: []string{"typescript"}}},
	}
	batches, omissions, warnings, err := ObserveGraph(context.Background(), root, project, GraphIndexOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 0 || len(omissions) != 1 || len(warnings) == 0 {
		t.Fatalf("unsupported graph result = batches=%d omissions=%#v warnings=%#v", len(batches), omissions, warnings)
	}
	if omissions[0].Backend != "tsserver" || omissions[0].ReasonCode != "backend_gated" {
		t.Fatalf("omission = %#v", omissions[0])
	}
}
