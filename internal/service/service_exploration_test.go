package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestNavService_ReturnsEvidenceFirstSummary(t *testing.T) {
	root, name := setupServiceExplorationWorkspace(t)
	app := New(root, nil)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.service",
		Context:   model.QueryOptions{Workspace: name, MaxItems: 10},
		Payload:   map[string]any{"path": "src/backend/conversation-fabric"},
	})
	if err != nil {
		t.Fatalf("nav.service: %v", err)
	}
	if !env.Ok {
		t.Fatalf("expected ok=true, warnings=%v", env.Warnings)
	}
	if env.Backend != "catalog+text" {
		t.Fatalf("expected backend catalog+text, got %q", env.Backend)
	}
	items, ok := env.Items.([]model.ServiceSurfaceSummary)
	if !ok {
		t.Fatalf("expected service summaries, got %T", env.Items)
	}
	if len(items) != 1 {
		t.Fatalf("expected one summary, got %d", len(items))
	}

	summary := items[0]
	if summary.Service != "conversation-fabric" {
		t.Fatalf("expected service name conversation-fabric, got %q", summary.Service)
	}
	if summary.Profile != "dotnet-microservice" {
		t.Fatalf("expected dotnet-microservice profile, got %q", summary.Profile)
	}
	if len(summary.HTTPEndpoints) != 2 {
		t.Fatalf("expected two endpoints, got %d", len(summary.HTTPEndpoints))
	}
	if len(summary.EventConsumers) != 1 {
		t.Fatalf("expected one consumer, got %d", len(summary.EventConsumers))
	}
	if len(summary.EventPublishers) != 1 {
		t.Fatalf("expected one publisher, got %d", len(summary.EventPublishers))
	}
	if len(summary.Entities) != 1 {
		t.Fatalf("expected one non-archetype entity, got %d", len(summary.Entities))
	}
	if summary.Entities[0]["name"] != "ConversationThread" {
		t.Fatalf("expected ConversationThread entity, got %#v", summary.Entities[0])
	}
	if len(summary.ArchetypeMatches) == 0 {
		t.Fatalf("expected archetype match to be reported")
	}
	if summary.Infrastructure["event_bus"] != true {
		t.Fatalf("expected event_bus wiring to be detected, got %#v", summary.Infrastructure)
	}
	if len(summary.NextQueries) == 0 {
		t.Fatalf("expected next queries to be suggested")
	}
}

func TestNavServiceGoPackageUsesCatalogProfile(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	name := "go-service-" + filepath.Base(root)
	writeWorkspaceFile(t, root, "internal/service/app.go", "package service\n\n// fixture mentions app.MapPost(\"/fake\", nil) but is still Go\nfunc New() {}\ntype App struct{}\n")

	project := model.ProjectFile{
		Project: model.ProjectBlock{
			Name:        name,
			Kind:        model.WorkspaceKindSingle,
			DefaultRepo: "main",
			Languages:   []string{"go"},
		},
		Repos: []model.WorkspaceRepo{{
			ID:        "main",
			Name:      "main",
			Root:      ".",
			Languages: []string{"go"},
		}},
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	_, err := workspace.RegisterWorkspace(name, model.WorkspaceRegistration{
		Name:      name,
		Root:      root,
		Languages: []string{"go"},
		Kind:      model.WorkspaceKindSingle,
	})
	if err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(name) })

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	files := []model.FileRecord{{FilePath: "internal/service/app.go", RepoID: "main", RepoName: "main", Language: "go"}}
	symbols := []model.SymbolRecord{
		{FilePath: "internal/service/app.go", RepoID: "main", RepoName: "main", Name: "New", Kind: "function", StartLine: 4, Language: "go"},
		{FilePath: "internal/service/app.go", RepoID: "main", RepoName: "main", Name: "App", Kind: "struct", StartLine: 5, Language: "go"},
	}
	if err := store.ReplaceCatalog(context.Background(), db, project, files, symbols); err != nil {
		t.Fatalf("ReplaceCatalog: %v", err)
	}

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.service",
		Context:   model.QueryOptions{Workspace: name},
		Payload:   map[string]any{"path": "internal/service"},
	})
	if err != nil {
		t.Fatalf("nav.service: %v", err)
	}
	items, ok := env.Items.([]model.ServiceSurfaceSummary)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one service summary, got %#v", env.Items)
	}
	summary := items[0]
	if summary.Profile != "go-package" {
		t.Fatalf("Profile = %q, want go-package", summary.Profile)
	}
	if len(summary.HTTPEndpoints) != 0 {
		t.Fatalf("expected no .NET endpoint false positives, got %#v", summary.HTTPEndpoints)
	}
	if summary.Symbols["function"] != 1 || summary.Symbols["struct"] != 1 {
		t.Fatalf("unexpected symbols: %#v", summary.Symbols)
	}
	if len(summary.NextQueries) == 0 || !strings.Contains(summary.NextQueries[1], "func ") {
		t.Fatalf("expected Go-oriented next queries, got %#v", summary.NextQueries)
	}
}

func TestNavService_RejectsPathsOutsideWorkspace(t *testing.T) {
	root, name := setupServiceExplorationWorkspace(t)
	app := New(root, nil)

	outside := filepath.Join(filepath.Dir(root), "outside-service")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside path: %v", err)
	}

	_, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.service",
		Context:   model.QueryOptions{Workspace: name, MaxItems: 10},
		Payload:   map[string]any{"path": outside},
	})
	if err == nil {
		t.Fatal("expected path outside workspace to fail")
	}
}
func TestNavService_IncludeArchetypeOptIn(t *testing.T) {
	root, name := setupServiceExplorationWorkspace(t)
	app := New(root, nil)

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.service",
		Context:   model.QueryOptions{Workspace: name, MaxItems: 10},
		Payload: map[string]any{
			"path":              "src/backend/conversation-fabric",
			"include_archetype": true,
		},
	})
	if err != nil {
		t.Fatalf("nav.service: %v", err)
	}
	items, ok := env.Items.([]model.ServiceSurfaceSummary)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one summary, got %T (%d)", env.Items, len(items))
	}
	if len(items[0].Entities) != 2 {
		t.Fatalf("expected archetype entity to be included, got %d entities", len(items[0].Entities))
	}
}

func setupServiceExplorationWorkspace(t *testing.T) (string, string) {
	t.Helper()
	ensureWritableTestHome(t)

	root := t.TempDir()
	name := "svc-ws-" + filepath.Base(root)
	serviceRoot := filepath.Join(root, "src", "backend", "conversation-fabric")
	if err := os.MkdirAll(filepath.Join(serviceRoot, "Domain", "Entities"), 0o755); err != nil {
		t.Fatalf("mkdir entities: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(serviceRoot, "Consumers"), 0o755); err != nil {
		t.Fatalf("mkdir consumers: %v", err)
	}

	files := map[string]string{
		filepath.Join(serviceRoot, "Program.cs"): `var builder = WebApplication.CreateBuilder(args);
builder.Services.AddSetupEventBus();
builder.Services.AddStackExchangeRedisCache(options => { });
builder.Services.AddDbContext<AppDbContext>(options => options.UseNpgsql("Host=localhost"));
var app = builder.Build();
app.MapPost("/api/v1/conversations/messages", () => "ok");
app.MapGet("/health", () => "ok");
await publisher.PublishAsync<ConversationMessageReceivedEvent>(new ConversationMessageReceivedEvent());`,
		filepath.Join(serviceRoot, "Domain", "Entities", "ConversationThread.cs"): `namespace Demo.Domain.Entities;
public class ConversationThread
{
    public string Id { get; set; } = string.Empty;
}`,
		filepath.Join(serviceRoot, "Domain", "Entities", "Usuario.cs"): `namespace Demo.Domain.Entities;
public class Usuario
{
    public string Id { get; set; } = string.Empty;
}`,
		filepath.Join(serviceRoot, "Consumers", "ConversationMessageReceivedConsumer.cs"): `public class ConversationMessageReceivedConsumer : IConsumer<ConversationMessageReceivedEvent>
{
    public Task Consume(ConsumeContext<ConversationMessageReceivedEvent> context) => Task.CompletedTask;
}`,
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	registration := model.WorkspaceRegistration{
		Name:      name,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}
	project := model.ProjectFile{
		Project: model.ProjectBlock{
			Name:        name,
			Languages:   []string{"csharp"},
			Kind:        model.WorkspaceKindSingle,
			DefaultRepo: "main",
		},
		Repos: []model.WorkspaceRepo{{
			ID:        "main",
			Name:      "main",
			Root:      ".",
			Languages: []string{"csharp"},
		}},
	}
	if _, err := workspace.RegisterWorkspace(name, registration); err != nil {
		t.Fatalf("register workspace: %v", err)
	}
	t.Cleanup(func() {
		_ = workspace.RemoveWorkspace(name)
	})
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("save project file: %v", err)
	}

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store open: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	catalogFiles := []model.FileRecord{
		{FilePath: "src/backend/conversation-fabric/Program.cs", RepoID: "main", RepoName: "main", Language: "csharp"},
		{FilePath: "src/backend/conversation-fabric/Domain/Entities/ConversationThread.cs", RepoID: "main", RepoName: "main", Language: "csharp"},
		{FilePath: "src/backend/conversation-fabric/Domain/Entities/Usuario.cs", RepoID: "main", RepoName: "main", Language: "csharp"},
		{FilePath: "src/backend/conversation-fabric/Consumers/ConversationMessageReceivedConsumer.cs", RepoID: "main", RepoName: "main", Language: "csharp"},
	}
	catalogSymbols := []model.SymbolRecord{
		{FilePath: "src/backend/conversation-fabric/Program.cs", RepoID: "main", RepoName: "main", Name: "Program", Kind: "class", StartLine: 1, Language: "csharp"},
		{FilePath: "src/backend/conversation-fabric/Program.cs", RepoID: "main", RepoName: "main", Name: "MapConversationMessage", Kind: "method", StartLine: 6, Language: "csharp"},
		{FilePath: "src/backend/conversation-fabric/Program.cs", RepoID: "main", RepoName: "main", Name: "Health", Kind: "method", StartLine: 7, Language: "csharp"},
		{FilePath: "src/backend/conversation-fabric/Domain/Entities/ConversationThread.cs", RepoID: "main", RepoName: "main", Name: "ConversationThread", Kind: "class", StartLine: 2, Language: "csharp"},
		{FilePath: "src/backend/conversation-fabric/Domain/Entities/Usuario.cs", RepoID: "main", RepoName: "main", Name: "Usuario", Kind: "class", StartLine: 2, Language: "csharp"},
		{FilePath: "src/backend/conversation-fabric/Consumers/ConversationMessageReceivedConsumer.cs", RepoID: "main", RepoName: "main", Name: "ConversationMessageReceivedConsumer", Kind: "class", StartLine: 1, Language: "csharp"},
	}
	if err := store.ReplaceCatalog(context.Background(), db, project, catalogFiles, catalogSymbols); err != nil {
		t.Fatalf("replace catalog: %v", err)
	}

	return root, name
}
