package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestNavGovernanceReportsEffectiveProfileAndSync(t *testing.T) {
	alias := "gov-ok-" + filepath.Base(t.TempDir())
	root := createIndexedWorkspaceFixture(t, alias)
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
		Operation: "nav.governance",
		Context:   model.QueryOptions{Workspace: alias},
	})
	if err != nil {
		t.Fatalf("nav.governance: %v", err)
	}
	if env.Backend != "governance" {
		t.Fatalf("backend = %q, want governance", env.Backend)
	}
	items := env.Items.([]model.GovernanceStatus)
	if len(items) != 1 {
		t.Fatalf("expected one governance status, got %#v", env.Items)
	}
	status := items[0]
	if status.Blocked {
		t.Fatalf("expected governance to pass, got %#v", status)
	}
	if status.Profile != "spec_backend" {
		t.Fatalf("profile = %q, want spec_backend", status.Profile)
	}
	if status.EffectiveBase != "ordered_wiki" {
		t.Fatalf("effective base = %q, want ordered_wiki", status.EffectiveBase)
	}
	if status.Sync != "in_sync" {
		t.Fatalf("sync = %q, want in_sync", status.Sync)
	}
}

func TestNavGovernanceAutoSyncsProjectionWhenMissing(t *testing.T) {
	alias := "gov-sync-" + filepath.Base(t.TempDir())
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# 07. Baseline tecnica\n")
	writeSpecBackendGovernanceFixture(t, root)
	if err := os.Remove(filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml")); err != nil {
		t.Fatalf("remove read-model: %v", err)
	}

	app := New(root, nil)
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "main"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.governance",
		Context:   model.QueryOptions{Workspace: alias},
	})
	if err != nil {
		t.Fatalf("nav.governance: %v", err)
	}
	items := env.Items.([]model.GovernanceStatus)
	if len(items) != 1 {
		t.Fatalf("expected one governance status, got %#v", env.Items)
	}
	if items[0].Sync != "auto_synced" && items[0].Sync != "in_sync" {
		t.Fatalf("expected auto sync or in_sync, got %#v", items[0])
	}
	if _, err := os.Stat(filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml")); err != nil {
		t.Fatalf("expected projected read-model to exist, got %v", err)
	}
}

func TestNavAskBlocksWhenGovernanceDocumentIsMissing(t *testing.T) {
	alias := "gov-block-" + filepath.Base(t.TempDir())
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# 07. Baseline tecnica\n")
	writeWorkspaceFile(t, root, ".docs/wiki/_mi-lsp/read-model.toml", "version = 1\n")

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
		Operation: "nav.ask",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"question": "how does daemon routing work?"},
	})
	if err != nil {
		t.Fatalf("nav.ask: %v", err)
	}
	if env.Backend != "governance" {
		t.Fatalf("backend = %q, want governance", env.Backend)
	}
	items := env.Items.([]model.GovernanceStatus)
	if len(items) != 1 || !items[0].Blocked {
		t.Fatalf("expected blocked governance status, got %#v", env.Items)
	}
}

func TestWorkspaceStatusIncludesGovernanceFields(t *testing.T) {
	alias := "gov-status-" + filepath.Base(t.TempDir())
	root := createIndexedWorkspaceFixture(t, alias)
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
		Operation: "workspace.status",
		Context:   model.QueryOptions{Workspace: alias},
	})
	if err != nil {
		t.Fatalf("workspace.status: %v", err)
	}
	items := env.Items.([]any)
	item := items[0].(map[string]any)
	if item["governance_profile"] != "spec_backend" {
		t.Fatalf("governance_profile = %#v, want spec_backend", item["governance_profile"])
	}
	if item["governance_blocked"] != false {
		t.Fatalf("governance_blocked = %#v, want false", item["governance_blocked"])
	}
	if item["governance_sync"] != "in_sync" {
		t.Fatalf("governance_sync = %#v, want in_sync", item["governance_sync"])
	}
}

func TestWorkspaceStatusCanSkipReadModelAutoSync(t *testing.T) {
	alias := "gov-status-readonly-" + filepath.Base(t.TempDir())
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# 07. Baseline tecnica\n")
	writeSpecBackendGovernanceFixture(t, root)
	projectionPath := filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml")
	if err := os.Remove(projectionPath); err != nil {
		t.Fatalf("remove read-model: %v", err)
	}

	app := New(root, nil)
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "main"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.status",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"auto_sync": false},
	})
	if err != nil {
		t.Fatalf("workspace.status: %v", err)
	}
	items := env.Items.([]any)
	item := items[0].(map[string]any)
	if item["governance_sync"] != "stale" {
		t.Fatalf("governance_sync = %#v, want stale", item["governance_sync"])
	}
	if item["governance_blocked"] != true {
		t.Fatalf("governance_blocked = %#v, want true", item["governance_blocked"])
	}
	if _, err := os.Stat(projectionPath); !os.IsNotExist(err) {
		t.Fatalf("read-model should not be auto-synced, stat err=%v", err)
	}
}

func TestNavPackBlockedWhenGovernanceIsInvalid(t *testing.T) {
	alias := "gov-block-pack-" + filepath.Base(t.TempDir())
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# 07. Baseline tecnica\n")
	writeWorkspaceFile(t, root, ".docs/wiki/_mi-lsp/read-model.toml", "version = 1\n")

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
		Operation: "nav.pack",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"task": "understand how login works"},
	})
	if err != nil {
		t.Fatalf("nav.pack: %v", err)
	}
	if env.Backend != "governance" {
		t.Fatalf("backend = %q, want governance", env.Backend)
	}
	items := env.Items.([]model.GovernanceStatus)
	if len(items) != 1 || !items[0].Blocked {
		t.Fatalf("expected blocked governance status, got %#v", env.Items)
	}
}

func TestNavRouteBlockedWhenGovernanceIsInvalid(t *testing.T) {
	alias := "gov-block-route-" + filepath.Base(t.TempDir())
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# 07. Baseline tecnica\n")
	writeWorkspaceFile(t, root, ".docs/wiki/_mi-lsp/read-model.toml", "version = 1\n")

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
		Operation: "nav.route",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"task": "understand how login works"},
	})
	if err != nil {
		t.Fatalf("nav.route: %v", err)
	}
	if env.Backend != "governance" {
		t.Fatalf("backend = %q, want governance", env.Backend)
	}
	items := env.Items.([]model.GovernanceStatus)
	if len(items) != 1 || !items[0].Blocked {
		t.Fatalf("expected blocked governance status, got %#v", env.Items)
	}
}
