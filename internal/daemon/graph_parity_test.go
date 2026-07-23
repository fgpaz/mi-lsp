package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/service"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestGraphQueryDaemonParityUsesCanonicalServiceEnvelope(t *testing.T) {
	root := t.TempDir()
	alias := "graph-daemon-" + filepath.Base(root)
	if err := workspace.SaveProjectFile(root, model.ProjectFile{Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", Languages: []string{"go"}}, Repos: []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/" + alias, Languages: []string{"go"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/graphdaemon\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "subject.go"), []byte("package graphdaemon\nfunc Subject() {}\nfunc Caller() { Subject() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
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
	var explainRID string
	if err := db.QueryRow(`SELECT cross_rid FROM graph_edges ORDER BY edge_key LIMIT 1`).Scan(&explainRID); err != nil || explainRID == "" {
		t.Fatalf("read-only edge selector: rid=%q err=%v", explainRID, err)
	}
	requests := []model.CommandRequest{
		{ProtocolVersion: model.ProtocolVersion, Operation: "nav.graph.stats", Context: model.QueryOptions{Workspace: alias}, Payload: map[string]any{}},
		{ProtocolVersion: model.ProtocolVersion, Operation: "nav.graph.validate", Context: model.QueryOptions{Workspace: alias}, Payload: map[string]any{}},
		{ProtocolVersion: model.ProtocolVersion, Operation: "nav.neighbors", Context: model.QueryOptions{Workspace: alias}, Payload: map[string]any{"selector": "Subject", "depth": 2, "limit": 10, "direction": "both"}},
		{ProtocolVersion: model.ProtocolVersion, Operation: "nav.callers", Context: model.QueryOptions{Workspace: alias}, Payload: map[string]any{"selector": "Subject"}},
		{ProtocolVersion: model.ProtocolVersion, Operation: "nav.callees", Context: model.QueryOptions{Workspace: alias}, Payload: map[string]any{"selector": "Caller"}},
		{ProtocolVersion: model.ProtocolVersion, Operation: "nav.path", Context: model.QueryOptions{Workspace: alias}, Payload: map[string]any{"from": "Caller", "to": "Subject"}},
		{ProtocolVersion: model.ProtocolVersion, Operation: "nav.explain", Context: model.QueryOptions{Workspace: alias}, Payload: map[string]any{"selector": explainRID}},
	}
	directApp := service.New(root, nil)
	daemon := &Server{app: service.New(root, nil)}
	for _, request := range requests {
		direct, err := directApp.Execute(context.Background(), request)
		if err != nil {
			t.Fatalf("%s direct: %v", request.Operation, err)
		}
		routed, err := daemon.handleRequest(request)
		if err != nil {
			t.Fatalf("%s daemon: %v", request.Operation, err)
		}
		direct.Backend, routed.Backend = "", ""
		left, _ := json.Marshal(direct)
		right, _ := json.Marshal(routed)
		if string(left) != string(right) {
			t.Fatalf("%s daemon semantic envelope differs:\ndirect=%s\nrouted=%s", request.Operation, left, right)
		}
	}
}
