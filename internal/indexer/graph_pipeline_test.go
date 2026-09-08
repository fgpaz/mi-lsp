package indexer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
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
	writeProgressTestFile(t, root, ".docs/wiki/guide.md", "# Guide\n\n[target](./target.md)\n")
	writeProgressTestFile(t, root, ".docs/wiki/target.md", "# Target\n")

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
	snapshot, err := store.BeginGraphQuerySnapshot(context.Background(), db, "")
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	docNodes, selectorKind, err := snapshot.ResolveGraphSelector(context.Background(), ".docs/wiki/guide.md")
	if err != nil || selectorKind != "doc_path" || len(docNodes) != 1 || docNodes[0].Identity.SymbolKind != "document" {
		t.Fatalf("document selector: nodes=%d kind=%q err=%v", len(docNodes), selectorKind, err)
	}
	docEdges, err := snapshot.Edges(context.Background(), []int{docNodes[0].NodeID}, "out", []string{"doc_markdown_link"}, 10)
	if err != nil || len(docEdges) != 1 || docEdges[0].SourceBackend != "docgraph" {
		t.Fatalf("doc_markdown_link query: edges=%#v err=%v", docEdges, err)
	}
	if refs, err := snapshot.EvidenceRefs(context.Background(), nil, &docEdges[0].EdgeID, 10); err != nil || len(refs) != 1 {
		t.Fatalf("doc_markdown_link evidence: refs=%v err=%v", refs, err)
	}
}

func TestObserveGraphExplicitGoEntrypointOverridesCSharpDefault(t *testing.T) {
	root := t.TempDir()
	writeProgressTestFile(t, root, "go.mod", "module example.com/mixed\n\ngo 1.23\n")
	writeProgressTestFile(t, root, "main.go", "package main\nfunc main() {}\n")
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "mixed", Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", DefaultEntrypoint: "repo::solution"},
		Repos:   []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/mixed", Languages: []string{"csharp", "go"}, DefaultEntrypoint: "repo::solution"}},
		Entrypoints: []model.WorkspaceEntrypoint{
			{ID: "repo::solution", RepoID: "repo", Path: "src/App.sln", Kind: model.EntrypointKindSolution},
			{ID: "repo::go-mod", RepoID: "repo", Path: "go.mod", Kind: model.EntrypointKindProject},
		},
	}
	for _, selector := range []string{"repo::go-mod", "go.mod"} {
		t.Run(selector, func(t *testing.T) {
			batches, omissions, _, err := ObserveGraph(context.Background(), root, project, GraphIndexOptions{EntrypointSelector: selector}, nil)
			if err != nil {
				t.Fatalf("ObserveGraph: %v", err)
			}
			if len(batches) != 1 || batches[0].Backend != "go" || batches[0].ProjectOrModule != "go.mod" {
				t.Fatalf("batches=%#v, want one explicit Go go.mod observation", batches)
			}
			if len(omissions) != 0 {
				t.Fatalf("omissions=%#v, want none", omissions)
			}
		})
	}
}

func TestObserveGraphAbsentEntrypointPreservesCSharpDefaultAndOmitsGo(t *testing.T) {
	root := t.TempDir()
	writeProgressTestFile(t, root, "go.mod", "module example.com/mixed\n\ngo 1.23\n")
	writeProgressTestFile(t, root, "src/App.csproj", "<Project />")
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "mixed", Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", DefaultEntrypoint: "repo::solution"},
		Repos:   []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/mixed", Languages: []string{"csharp", "go"}, DefaultEntrypoint: "repo::solution"}},
		Entrypoints: []model.WorkspaceEntrypoint{
			{ID: "repo::solution", RepoID: "repo", Path: "src/App.sln", Kind: model.EntrypointKindSolution},
			{ID: "repo::project", RepoID: "repo", Path: "src/App.csproj", Kind: model.EntrypointKindProject},
		},
	}
	batches, omissions, _, err := ObserveGraph(context.Background(), root, project, GraphIndexOptions{RoslynObserver: func(_ context.Context, request GraphObservationRequest) (model.GraphObservationBatch, error) {
		return stagingBatch("roslyn", request.ProjectOrModule, false), nil
	}}, nil)
	if err != nil {
		t.Fatalf("ObserveGraph: %v", err)
	}
	if len(batches) != 1 || batches[0].Backend != "roslyn" {
		t.Fatalf("batches=%#v, want one Roslyn default observation", batches)
	}
	foundGoOmission := false
	for _, omission := range omissions {
		if omission.Backend == "go" && omission.ReasonCode == "go_module_missing" {
			foundGoOmission = true
		}
	}
	if !foundGoOmission {
		t.Fatalf("omissions=%#v, want the unchanged Go omission", omissions)
	}
}

func TestObserveGraphRejectsUnknownExplicitEntrypoint(t *testing.T) {
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "go", Kind: model.WorkspaceKindSingle, DefaultRepo: "repo"},
		Repos:   []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", Languages: []string{"go"}}},
	}
	_, _, _, err := ObserveGraph(context.Background(), t.TempDir(), project, GraphIndexOptions{EntrypointSelector: "missing::entrypoint"}, nil)
	var observationErr *model.GraphObservationError
	if !errors.As(err, &observationErr) || observationErr.Code != "GPH_ENTRYPOINT_NOT_FOUND" {
		t.Fatalf("error=%v, want GPH_ENTRYPOINT_NOT_FOUND", err)
	}
}

func TestObserveGraphExplicitCSharpProjectSelectsOneRoslynRequest(t *testing.T) {
	root := t.TempDir()
	writeProgressTestFile(t, root, "backend/src/App.csproj", "<Project />")
	project := model.ProjectFile{
		Project:     model.ProjectBlock{Name: "cs", Kind: model.WorkspaceKindSingle, DefaultRepo: "backend"},
		Repos:       []model.WorkspaceRepo{{ID: "backend", Name: "backend", Root: "backend", RepositoryIdentity: "https://example.com/cs", Languages: []string{"csharp"}}},
		Entrypoints: []model.WorkspaceEntrypoint{{ID: "backend::app", RepoID: "backend", Path: "backend/src/App.csproj", Kind: model.EntrypointKindProject}},
	}
	requests := make([]GraphObservationRequest, 0, 1)
	batches, _, _, err := ObserveGraph(context.Background(), root, project, GraphIndexOptions{EntrypointSelector: "backend::app", RoslynObserver: func(_ context.Context, request GraphObservationRequest) (model.GraphObservationBatch, error) {
		requests = append(requests, request)
		return stagingBatch("roslyn", request.ProjectOrModule, false), nil
	}}, nil)
	if err != nil {
		t.Fatalf("ObserveGraph: %v", err)
	}
	if len(requests) != 1 || len(batches) != 1 || requests[0].EntrypointID != "backend::app" || requests[0].ProjectOrModule != "src/App.csproj" {
		t.Fatalf("requests=%#v batches=%#v, want one selected project request", requests, batches)
	}
}

func TestObserveGraphExplicitSolutionFailsClosedWithoutMembershipHelper(t *testing.T) {
	root := t.TempDir()
	writeProgressTestFile(t, root, "App.sln", "Microsoft Visual Studio Solution File\n")
	project := model.ProjectFile{
		Project:     model.ProjectBlock{Name: "cs", Kind: model.WorkspaceKindSingle, DefaultRepo: "repo"},
		Repos:       []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/cs", Languages: []string{"csharp"}}},
		Entrypoints: []model.WorkspaceEntrypoint{{ID: "repo::solution", RepoID: "repo", Path: "App.sln", Kind: model.EntrypointKindSolution}},
	}
	_, _, _, err := ObserveGraph(context.Background(), root, project, GraphIndexOptions{EntrypointSelector: "repo::solution"}, nil)
	var observationErr *model.GraphObservationError
	if !errors.As(err, &observationErr) || observationErr.Code != "GPH_ENTRYPOINT_SOLUTION_UNAVAILABLE" {
		t.Fatalf("error=%v, want GPH_ENTRYPOINT_SOLUTION_UNAVAILABLE", err)
	}
}

func TestObserveGraphRejectsExplicitSelectorSafetyAndTopologyFailures(t *testing.T) {
	tests := []struct {
		name     string
		selector string
		setup    func(*testing.T, string)
		project  model.ProjectFile
		wantCode string
	}{
		{
			name: "unknown", selector: "missing::entrypoint",
			project:  model.ProjectFile{Project: model.ProjectBlock{Kind: model.WorkspaceKindSingle}, Repos: []model.WorkspaceRepo{{ID: "repo", Root: ".", Languages: []string{"go"}}}},
			wantCode: "GPH_ENTRYPOINT_NOT_FOUND",
		},
		{
			name: "unsupported kind", selector: "README.md",
			setup:    func(t *testing.T, root string) { writeProgressTestFile(t, root, "README.md", "readme") },
			project:  model.ProjectFile{Project: model.ProjectBlock{Kind: model.WorkspaceKindSingle}, Repos: []model.WorkspaceRepo{{ID: "repo", Root: ".", Languages: []string{"go"}}}},
			wantCode: "GPH_ENTRYPOINT_NOT_FOUND",
		},
		{
			name: "traversal", selector: "../go.mod",
			project:  model.ProjectFile{Project: model.ProjectBlock{Kind: model.WorkspaceKindSingle}, Repos: []model.WorkspaceRepo{{ID: "repo", Root: ".", Languages: []string{"go"}}}},
			wantCode: "GPH_ENTRYPOINT_NOT_FOUND",
		},
		{
			name: "absolute", selector: filepath.Join(string(filepath.Separator), "outside", "go.mod"),
			project:  model.ProjectFile{Project: model.ProjectBlock{Kind: model.WorkspaceKindSingle}, Repos: []model.WorkspaceRepo{{ID: "repo", Root: ".", Languages: []string{"go"}}}},
			wantCode: "GPH_ENTRYPOINT_NOT_FOUND",
		},
		{
			name: "go.work", selector: "go.work",
			setup:    func(t *testing.T, root string) { writeProgressTestFile(t, root, "go.work", "go 1.23\n") },
			project:  model.ProjectFile{Project: model.ProjectBlock{Kind: model.WorkspaceKindSingle}, Repos: []model.WorkspaceRepo{{ID: "repo", Root: ".", Languages: []string{"go"}}}},
			wantCode: "GPH_ENTRYPOINT_NOT_FOUND",
		},
		{
			name: "unknown repo id", selector: "missing::go",
			setup:    func(t *testing.T, root string) { writeProgressTestFile(t, root, "go.mod", "module example.com/go\n") },
			project:  model.ProjectFile{Project: model.ProjectBlock{Kind: model.WorkspaceKindSingle}, Repos: []model.WorkspaceRepo{{ID: "repo", Root: ".", Languages: []string{"go"}}}, Entrypoints: []model.WorkspaceEntrypoint{{ID: "missing::go", RepoID: "missing", Path: "go.mod", Kind: model.EntrypointKindProject}}},
			wantCode: "GPH_ENTRYPOINT_REPO_NOT_FOUND",
		},
		{
			name: "language mismatch", selector: "go.mod",
			setup:    func(t *testing.T, root string) { writeProgressTestFile(t, root, "go.mod", "module example.com/go\n") },
			project:  model.ProjectFile{Project: model.ProjectBlock{Kind: model.WorkspaceKindSingle}, Repos: []model.WorkspaceRepo{{ID: "repo", Root: ".", Languages: []string{"csharp"}}}},
			wantCode: "GPH_ENTRYPOINT_LANGUAGE_MISMATCH",
		},
		{
			name: "declared path outside repo", selector: "repo::foreign",
			setup: func(t *testing.T, root string) {
				writeProgressTestFile(t, root, "foreign/go.mod", "module example.com/foreign\n")
			},
			project:  model.ProjectFile{Project: model.ProjectBlock{Kind: model.WorkspaceKindSingle}, Repos: []model.WorkspaceRepo{{ID: "repo", Root: "backend", Languages: []string{"go"}}}, Entrypoints: []model.WorkspaceEntrypoint{{ID: "repo::foreign", RepoID: "repo", Path: "foreign/go.mod", Kind: model.EntrypointKindProject}}},
			wantCode: "GPH_ENTRYPOINT_REPO_MISMATCH",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if test.setup != nil {
				test.setup(t, root)
			}
			_, _, _, err := ObserveGraph(context.Background(), root, test.project, GraphIndexOptions{EntrypointSelector: test.selector}, nil)
			var observationErr *model.GraphObservationError
			if !errors.As(err, &observationErr) || observationErr.Code != test.wantCode {
				t.Fatalf("error=%v, want %s", err, test.wantCode)
			}
		})
	}
}

func TestObserveGraphRejectsAmbiguousRepositoryRelativeEntrypoint(t *testing.T) {
	root := t.TempDir()
	writeProgressTestFile(t, root, "a/go.mod", "module example.com/a\n")
	writeProgressTestFile(t, root, "b/go.mod", "module example.com/b\n")
	project := model.ProjectFile{
		Project: model.ProjectBlock{Kind: model.WorkspaceKindContainer},
		Repos: []model.WorkspaceRepo{
			{ID: "a", Root: "a", Languages: []string{"go"}},
			{ID: "b", Root: "b", Languages: []string{"go"}},
		},
	}
	_, _, _, err := ObserveGraph(context.Background(), root, project, GraphIndexOptions{EntrypointSelector: "go.mod"}, nil)
	var observationErr *model.GraphObservationError
	if !errors.As(err, &observationErr) || observationErr.Code != "GPH_ENTRYPOINT_AMBIGUOUS" {
		t.Fatalf("error=%v, want GPH_ENTRYPOINT_AMBIGUOUS", err)
	}
}

func TestObserveGraphRejectsExplicitSymlinkEntrypoint(t *testing.T) {
	root := t.TempDir()
	writeProgressTestFile(t, root, "target/go.mod", "module example.com/target\n")
	if err := os.Symlink(filepath.Join("..", "target", "go.mod"), filepath.Join(root, "link-go.mod")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	project := model.ProjectFile{Project: model.ProjectBlock{Kind: model.WorkspaceKindSingle}, Repos: []model.WorkspaceRepo{{ID: "repo", Root: ".", Languages: []string{"go"}}}}
	_, _, _, err := ObserveGraph(context.Background(), root, project, GraphIndexOptions{EntrypointSelector: "link-go.mod"}, nil)
	var observationErr *model.GraphObservationError
	if !errors.As(err, &observationErr) || observationErr.Code != "GPH_ENTRYPOINT_NOT_FOUND" {
		t.Fatalf("error=%v, want GPH_ENTRYPOINT_NOT_FOUND", err)
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
	root := t.TempDir()
	writeProgressTestFile(t, root, "src/Fixture.csproj", "<Project />")
	batches, _, _, err := ObserveGraph(context.Background(), root, project, GraphIndexOptions{RoslynObserver: func(_ context.Context, request GraphObservationRequest) (model.GraphObservationBatch, error) {
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
func TestObserveGraphResolvesAutoDetectedGoEntrypointThroughTopology(t *testing.T) {
	root := t.TempDir()
	writeProgressTestFile(t, root, "runtime/go.mod", "module example.test/runtime\n\ngo 1.24\n")
	writeProgressTestFile(t, root, "runtime/main.go", "package runtime\nfunc Run() {}\n")
	project := model.ProjectFile{
		Project: model.ProjectBlock{
			Name:              "nested-go",
			Kind:              model.WorkspaceKindSingle,
			DefaultRepo:       "runtime",
			DefaultEntrypoint: "runtime::runtime-go-mod",
		},
		Repos: []model.WorkspaceRepo{{
			ID:                 "runtime",
			Name:               "runtime",
			Root:               "runtime",
			RepositoryIdentity: "https://example.com/nested-go",
			Languages:          []string{"go"},
			DefaultEntrypoint:  "runtime::runtime-go-mod",
		}},
		Entrypoints: []model.WorkspaceEntrypoint{{
			ID:      "runtime::runtime-go-mod",
			RepoID:  "runtime",
			Path:    "runtime/go.mod",
			Kind:    model.EntrypointKindProject,
			Default: true,
		}},
	}

	batches, omissions, _, err := ObserveGraph(context.Background(), root, project, GraphIndexOptions{}, nil)
	if err != nil {
		t.Fatalf("ObserveGraph: %v", err)
	}
	if len(batches) != 1 {
		t.Fatalf("batches=%d, want one auto-detected Go observation", len(batches))
	}
	if batches[0].ProjectOrModule != "go.mod" {
		t.Fatalf("Go project/module=%q, want repo-local go.mod", batches[0].ProjectOrModule)
	}
	for _, omission := range omissions {
		if omission.ReasonCode == "go_module_missing" {
			t.Fatalf("auto-detected Go module was omitted: %#v", omissions)
		}
	}
}

func TestObserveGraphRejectsUnknownOrUnsafeGoTopologyEntrypoint(t *testing.T) {
	tests := []struct {
		name           string
		defaultID      string
		entrypointID   string
		entrypointPath string
	}{
		{name: "unknown ID", defaultID: "runtime::missing-go-mod"},
		{name: "malformed ID", defaultID: "runtime::"},
		{name: "missing generated ID", defaultID: "runtime::runtime-go-mod"},
		{name: "unsafe declared path", defaultID: "runtime::runtime-go-mod", entrypointID: "runtime::runtime-go-mod", entrypointPath: "../go.mod"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeProgressTestFile(t, root, "runtime/go.mod", "module example.test/runtime\n\ngo 1.24\n")
			writeProgressTestFile(t, root, "runtime/main.go", "package runtime\nfunc Run() {}\n")
			project := model.ProjectFile{
				Project: model.ProjectBlock{
					Name:              "nested-go",
					Kind:              model.WorkspaceKindSingle,
					DefaultRepo:       "runtime",
					DefaultEntrypoint: test.defaultID,
				},
				Repos: []model.WorkspaceRepo{{
					ID:                 "runtime",
					Name:               "runtime",
					Root:               "runtime",
					RepositoryIdentity: "https://example.com/nested-go",
					Languages:          []string{"go"},
					DefaultEntrypoint:  test.defaultID,
				}},
			}
			if test.entrypointID != "" {
				project.Entrypoints = []model.WorkspaceEntrypoint{{
					ID:     test.entrypointID,
					RepoID: "runtime",
					Path:   test.entrypointPath,
					Kind:   model.EntrypointKindProject,
				}}
			}

			batches, omissions, _, err := ObserveGraph(context.Background(), root, project, GraphIndexOptions{}, nil)
			if err != nil {
				t.Fatalf("ObserveGraph: %v", err)
			}
			if len(batches) != 0 {
				t.Fatalf("batches=%d, want no observation for unresolvable topology entrypoint", len(batches))
			}
			foundMissing := false
			for _, omission := range omissions {
				if omission.ReasonCode == "go_module_missing" {
					foundMissing = true
				}
			}
			if !foundMissing {
				t.Fatalf("omissions=%#v, want go_module_missing", omissions)
			}
		})
	}
}

func TestObserveGraphFansOutContainerBackendsAndRebasesRoslynPaths(t *testing.T) {
	root := t.TempDir()
	writeProgressTestFile(t, root, ".mi-lsp/project.toml", "[project]\nkind = \"container\"\n")
	writeProgressTestFile(t, root, "go.mod", "module example.com/container\n\ngo 1.23\n")
	writeProgressTestFile(t, root, "cmd/main.go", "package main\nfunc main() {}\n")
	writeProgressTestFile(t, root, "worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj", "<Project />")
	writeProgressTestFile(t, root, "worker-dotnet/MiLsp.Worker.ContractTests/MiLsp.Worker.ContractTests.csproj", "<Project />")
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "container", Kind: model.WorkspaceKindContainer, DefaultRepo: "benchmarks"},
		Repos: []model.WorkspaceRepo{
			{ID: "benchmarks", Name: "benchmarks", Root: "benchmarks", RepositoryIdentity: "https://example.com/container.git", Languages: []string{"go", "python", "typescript"}},
			{ID: "cmd", Name: "cmd", Root: "cmd", Languages: []string{"go"}},
			{ID: "internal", Name: "internal", Root: "internal", Languages: []string{"go"}},
			{ID: "scripts", Name: "scripts", Root: "scripts", Languages: []string{"python"}},
			{ID: "worker-dotnet", Name: "worker-dotnet", Root: "worker-dotnet", Languages: []string{"csharp"}},
		},
		Entrypoints: []model.WorkspaceEntrypoint{
			{ID: "solution", RepoID: "worker-dotnet", Path: "worker-dotnet/MiLsp.Worker.sln", Kind: model.EntrypointKindSolution},
			{ID: "contract", RepoID: "worker-dotnet", Path: "worker-dotnet/MiLsp.Worker.ContractTests/MiLsp.Worker.ContractTests.csproj", Kind: model.EntrypointKindProject},
			{ID: "worker", RepoID: "worker-dotnet", Path: "worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj", Kind: model.EntrypointKindProject},
		},
	}
	var requests []GraphObservationRequest
	batches, omissions, _, err := ObserveGraph(context.Background(), root, project, GraphIndexOptions{RoslynObserver: func(_ context.Context, request GraphObservationRequest) (model.GraphObservationBatch, error) {
		requests = append(requests, request)
		return stagingBatch("roslyn", request.ProjectOrModule, false), nil
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || len(batches) != 3 {
		t.Fatalf("requests=%d batches=%d, want one go and two Roslyn batches", len(requests), len(batches))
	}
	for _, request := range requests {
		if request.EntrypointKind == model.EntrypointKindSolution {
			t.Fatalf("solution was sent to Roslyn: %#v", request)
		}
		if strings.Contains(request.ProjectOrModule, "worker-dotnet/") || !strings.HasPrefix(request.EntrypointPath, "worker-dotnet/") || !strings.HasSuffix(strings.ToLower(request.ProjectOrModule), ".csproj") {
			t.Fatalf("Roslyn path split is not repo/workspace relative: %#v", request)
		}
		if !strings.HasPrefix(request.RepoRoot, root) {
			t.Fatalf("RepoRoot=%q is not logical root under workspace %q", request.RepoRoot, root)
		}
	}
	for _, batch := range batches {
		if batch.WorkspaceIdentity != "example.com/container" || batch.RepositoryIdentity != "example.com/container" {
			t.Fatalf("batch identity=%q/%q", batch.WorkspaceIdentity, batch.RepositoryIdentity)
		}
		if batch.Backend == "roslyn" && !strings.HasPrefix(batch.ProjectOrModule, "worker-dotnet/") {
			t.Fatalf("Roslyn batch was not rebased: %q", batch.ProjectOrModule)
		}
	}
	foundGated := map[string]bool{}
	for _, omission := range omissions {
		if omission.ReasonCode == "backend_gated" {
			foundGated[omission.Backend+":"+omission.OwnerPath] = true
		}
	}
	if !foundGated["tsserver:benchmarks"] || !foundGated["pyright:benchmarks"] || !foundGated["pyright:scripts"] {
		t.Fatalf("gated omissions=%#v", omissions)
	}
}

func TestObserveGraphOmitsPartialRoslynProjectWithoutBlockingCompleteBatches(t *testing.T) {
	root := t.TempDir()
	writeProgressTestFile(t, root, ".mi-lsp/project.toml", "[project]\nkind = \"container\"\n")
	writeProgressTestFile(t, root, "go.mod", "module example.com/container\n\ngo 1.23\n")
	writeProgressTestFile(t, root, "main.go", "package main\nfunc main() {}\n")
	writeProgressTestFile(t, root, "src/Complete.csproj", "<Project />")
	writeProgressTestFile(t, root, "src/Partial.csproj", "<Project />")
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "container", Kind: model.WorkspaceKindContainer},
		Repos: []model.WorkspaceRepo{
			{ID: "go", Name: "go", Root: ".", RepositoryIdentity: "https://example.com/container", Languages: []string{"go"}},
			{ID: "cs", Name: "cs", Root: ".", RepositoryIdentity: "https://example.com/container", Languages: []string{"csharp"}},
		},
		Entrypoints: []model.WorkspaceEntrypoint{
			{ID: "complete", RepoID: "cs", Path: "src/Complete.csproj", Kind: model.EntrypointKindProject},
			{ID: "partial", RepoID: "cs", Path: "src/Partial.csproj", Kind: model.EntrypointKindProject},
		},
	}
	requests := 0
	batches, omissions, warnings, err := ObserveGraph(context.Background(), root, project, GraphIndexOptions{RoslynObserver: func(_ context.Context, request GraphObservationRequest) (model.GraphObservationBatch, error) {
		requests++
		batch := stagingBatch("roslyn", request.ProjectOrModule, false)
		if strings.HasSuffix(request.ProjectOrModule, "Partial.csproj") {
			batch.Completeness = model.GraphCompletenessPartial
			batch.Omissions = append(batch.Omissions, model.GraphObservationOmission{Ref: "omission:partial", OwnerPath: "Partial.csproj", SubjectKind: "project", Backend: "roslyn", Capability: "declarations", ReasonCode: "compiler_errors", RecoveryHintCode: "repair_project_or_retry"})
			batch.Coverage[0].Eligible++
			batch.Coverage[0].Omitted++
		}
		return batch, nil
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(batches) != 2 {
		t.Fatalf("requests=%d batches=%d, want Go plus complete Roslyn batch", requests, len(batches))
	}
	foundPartial := false
	for _, omission := range omissions {
		if omission.Backend == "roslyn" && omission.ReasonCode == "backend_partial" {
			foundPartial = true
		}
	}
	if !foundPartial || len(warnings) == 0 {
		t.Fatalf("omissions=%#v warnings=%#v", omissions, warnings)
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
func TestGraphGoModuleAcceptsNestedSelectorAndPreservesRootFallback(t *testing.T) {
	root := t.TempDir()
	writeProgressTestFile(t, root, "runtime/go.mod", "module example.test/runtime\n\ngo 1.24\n")
	if got := graphGoModule(root, "runtime/go.mod"); got != "runtime/go.mod" {
		t.Fatalf("nested module selector = %q, want runtime/go.mod", got)
	}
	writeProgressTestFile(t, root, "go.mod", "module example.test/root\n\ngo 1.24\n")
	if got := graphGoModule(root, ""); got != "go.mod" {
		t.Fatalf("root fallback = %q, want go.mod", got)
	}
	if err := os.MkdirAll(filepath.Join(root, "directory", "go.mod"), 0o755); err != nil {
		t.Fatalf("mkdir non-regular selector: %v", err)
	}
	writeProgressTestFile(t, root, "target/go.mod", "module example.test/target\n\ngo 1.24\n")
	if err := os.MkdirAll(filepath.Join(root, "link"), 0o755); err != nil {
		t.Fatalf("mkdir symlink parent: %v", err)
	}
	selectors := []string{"missing/go.mod", "directory/go.mod", "../go.mod", "/tmp/go.mod", "runtime/../outside/go.mod", "runtime\\go.mod"}
	if err := os.Symlink(filepath.Join("..", "target", "go.mod"), filepath.Join(root, "link", "go.mod")); err == nil {
		selectors = append(selectors, "link/go.mod")
	} else {
		t.Logf("symlink selector unavailable: %v", err)
	}
	for _, selector := range selectors {
		if got := graphGoModule(root, selector); got != "" {
			t.Fatalf("invalid explicit selector %q accepted as %q", selector, got)
		}
	}
}

func TestGraphSafeRelativeModuleRejectsDriveRelativeAndAcceptsNormalRelative(t *testing.T) {
	root := t.TempDir()
	writeProgressTestFile(t, root, "runtime/go.mod", "module example.test/runtime\n\ngo 1.24\n")
	if got, safe := graphSafeRelativeModule(root, "runtime/go.mod"); !safe || got != "runtime/go.mod" {
		t.Fatalf("normal relative module = %q, safe=%v; want runtime/go.mod,true", got, safe)
	}
	if runtime.GOOS != "windows" {
		// A colon is a valid POSIX filename, so create the path to ensure the
		// rejection is exercised before filesystem resolution rather than only
		// because the path is absent.
		writeProgressTestFile(t, root, "C:foo/go.mod", "module example.test/drive-relative\n\ngo 1.24\n")
	}
	if got, safe := graphSafeRelativeModule(root, "C:foo/go.mod"); safe {
		t.Fatalf("drive-relative module accepted as %q", got)
	}
}

func TestGraphGoModuleRejectsGoWorkSelectorAndFallback(t *testing.T) {
	root := t.TempDir()
	writeProgressTestFile(t, root, "go.work", "go 1.24\nuse ./runtime\n")
	if got := graphGoModule(root, ""); got != "" {
		t.Fatalf("go.work-only fallback = %q, want no module", got)
	}
	if got := graphGoModule(root, "go.work"); got != "" {
		t.Fatalf("go.work selector = %q, want no module", got)
	}
}
