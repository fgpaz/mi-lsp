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
	if err := os.WriteFile(filepath.Join(root, "subject.go"), []byte("package graphdaemon\nfunc Subject() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := indexer.IndexWorkspaceWithGraphProgress(context.Background(), root, true, "", nil, indexer.GraphIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{Name: alias, Root: root, Languages: []string{"go"}, Kind: model.WorkspaceKindSingle}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })
	request := model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "nav.graph.stats", Context: model.QueryOptions{Workspace: alias}, Payload: map[string]any{}}
	direct, err := service.New(root, nil).Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	daemon := &Server{app: service.New(root, nil)}
	routed, err := daemon.handleRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	direct.Backend, routed.Backend = "", ""
	left, _ := json.Marshal(direct)
	right, _ := json.Marshal(routed)
	if string(left) != string(right) {
		t.Fatalf("daemon semantic envelope differs:\ndirect=%s\nrouted=%s", left, right)
	}
}
