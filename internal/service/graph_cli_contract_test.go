package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestGraphCLIContractIntegrationIsDeterministicAndReadOnly(t *testing.T) {
	root := t.TempDir()
	alias := "graph-contract-" + filepath.Base(root)
	if err := workspace.SaveProjectFile(root, model.ProjectFile{Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", Languages: []string{"go"}}, Repos: []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/" + alias, Languages: []string{"go"}}}}); err != nil {
		t.Fatal(err)
	}
	writeGraphContractFile(t, root, "go.mod", "module example.com/graphcontract\n\ngo 1.23\n")
	writeGraphContractFile(t, root, "subject.go", "package graphcontract\n\nfunc Subject() string { return \"ok\" }\n\nfunc Caller() string { return Subject() }\n")
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
	before := sqliteTotalChanges(t, db)
	app := New(root, nil)
	requests := []struct {
		op      string
		payload map[string]any
	}{
		{"nav.graph.stats", map[string]any{}}, {"nav.graph.validate", map[string]any{}},
		{"nav.neighbors", map[string]any{"selector": "Subject", "depth": 2, "limit": 2, "token_budget": 1, "direction": "both"}},
		{"nav.callers", map[string]any{"selector": "Subject"}}, {"nav.callees", map[string]any{"selector": "Caller"}},
		{"nav.path", map[string]any{"from": "Caller", "to": "Subject"}}, {"nav.explain", map[string]any{"selector": "missing-edge"}},
	}
	for _, tc := range requests {
		request := model.CommandRequest{Operation: tc.op, Context: model.QueryOptions{Workspace: alias}, Payload: tc.payload}
		first, err := app.Execute(context.Background(), request)
		if err != nil {
			t.Fatalf("%s: %v", tc.op, err)
		}
		if !first.Ok || first.Graph == nil || first.GenerationID == "" || first.Graph.GenerationID == "" {
			t.Fatalf("%s missing graph contract: %#v", tc.op, first)
		}
		if first.Graph.Schema <= 0 || first.Graph.DeterminismDigest == "" {
			t.Fatalf("%s missing schema/digest: %#v", tc.op, first.Graph)
		}
		firstGraph, _ := json.Marshal(first.Graph)
		firstItems, _ := json.Marshal(first.Items)
		for i := 0; i < 10; i++ {
			next, err := app.Execute(context.Background(), request)
			if err != nil {
				t.Fatalf("%s rerun: %v", tc.op, err)
			}
			nextGraph, _ := json.Marshal(next.Graph)
			nextItems, _ := json.Marshal(next.Items)
			if !bytes.Equal(firstGraph, nextGraph) || !bytes.Equal(firstItems, nextItems) {
				t.Fatalf("%s is not deterministic", tc.op)
			}
		}
	}
	if after := sqliteTotalChanges(t, db); after != before {
		t.Fatalf("graph queries wrote database: before=%d after=%d", before, after)
	}
}

func writeGraphContractFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
func sqliteTotalChanges(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow("SELECT total_changes()").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
