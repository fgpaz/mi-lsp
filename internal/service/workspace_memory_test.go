package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func createWorkspaceMemoryFixture(t *testing.T, alias string) string {
	t.Helper()
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/01_alcance_funcional.md", "# 01 Alcance\n\nAlcance funcional del flujo.\n")
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# 07 Baseline tecnica\n\nBaseline tecnica del daemon.\n")
	writeWorkspaceFile(t, root, ".docs/raw/plans/2026-04-16-reentry-wave.md", "# Reentry wave\n\nPlan reciente.\n")
	writeSpecBackendGovernanceFixture(t, root)
	return root
}

func TestWorkspaceStatusExposesMemoryPointerAndFullMemory(t *testing.T) {
	alias := "status-memory-" + filepath.Base(t.TempDir())
	root := createWorkspaceMemoryFixture(t, alias)
	app := New(root, nil)

	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	}); err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	previewEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.status",
		Context:   model.QueryOptions{Workspace: alias, AXI: true},
	})
	if err != nil {
		t.Fatalf("workspace.status preview: %v", err)
	}
	if previewEnv.MemoryPointer == nil {
		t.Fatalf("expected memory pointer in preview status, got %#v", previewEnv)
	}
	if previewEnv.Continuation == nil || previewEnv.Continuation.Reason != "expand_preview" {
		t.Fatalf("expected expand_preview continuation, got %#v", previewEnv.Continuation)
	}
	previewItems, ok := previewEnv.Items.([]any)
	if !ok || len(previewItems) != 1 {
		t.Fatalf("preview items = %#v", previewEnv.Items)
	}
	previewItem, ok := previewItems[0].(map[string]any)
	if !ok {
		t.Fatalf("preview item type = %T", previewItems[0])
	}
	if _, exists := previewItem["memory"]; exists {
		t.Fatalf("preview item should not inline full memory payload, got %#v", previewItem["memory"])
	}

	fullEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.status",
		Context:   model.QueryOptions{Workspace: alias, AXI: true, Full: true},
	})
	if err != nil {
		t.Fatalf("workspace.status full: %v", err)
	}
	if fullEnv.MemoryPointer == nil {
		t.Fatalf("expected memory pointer in full status, got %#v", fullEnv)
	}
	fullItems, ok := fullEnv.Items.([]any)
	if !ok || len(fullItems) != 1 {
		t.Fatalf("full items = %#v", fullEnv.Items)
	}
	fullItem, ok := fullItems[0].(map[string]any)
	if !ok {
		t.Fatalf("full item type = %T", fullItems[0])
	}
	memoryValue, ok := fullItem["memory"].(model.WorkspaceStatusMemory)
	if !ok {
		t.Fatalf("memory payload type = %T, value=%#v", fullItem["memory"], fullItem["memory"])
	}
	if len(memoryValue.RecentCanonicalChanges) == 0 {
		t.Fatalf("expected recent canonical changes, got %#v", memoryValue)
	}
	if memoryValue.BestReentry.Op != "nav.search" {
		t.Fatalf("best reentry op = %q, want nav.search", memoryValue.BestReentry.Op)
	}
	if memoryValue.Handoff == "" {
		t.Fatalf("expected handoff breadcrumb, got %#v", memoryValue)
	}
}

func TestWorkspaceStatusWarnsWhenSnapshotAbsentAfterIndex(t *testing.T) {
	alias := "status-missing-snapshot-" + filepath.Base(t.TempDir())
	root := createWorkspaceMemoryFixture(t, alias)
	app := New(root, nil)

	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias, "no_index": true},
	}); err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	if _, err := indexer.IndexWorkspace(context.Background(), root, false); err != nil {
		t.Fatalf("indexer: %v", err)
	}

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.UpsertWorkspaceMetaMap(context.Background(), db, map[string]string{
		"memory_snapshot_json":     "",
		"memory_snapshot_built_at": "",
	}); err != nil {
		db.Close()
		t.Fatalf("clear snapshot: %v", err)
	}
	db.Close()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.status",
		Context:   model.QueryOptions{Workspace: alias, AXI: true, Full: true},
	})
	if err != nil {
		t.Fatalf("workspace.status: %v", err)
	}

	var found bool
	for _, w := range env.Warnings {
		if strings.Contains(w, "reentry memory snapshot absent") && strings.Contains(w, alias) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected warning about missing reentry snapshot, got warnings=%v", env.Warnings)
	}

	items, ok := env.Items.([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 item even without snapshot, got %#v", env.Items)
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("item type = %T", items[0])
	}
	if _, exists := item["memory"]; exists {
		t.Fatalf("memory must be absent when snapshot is missing; got %#v", item["memory"])
	}
	if env.MemoryPointer != nil {
		t.Fatalf("memory_pointer must be nil when snapshot is missing; got %#v", env.MemoryPointer)
	}
}
