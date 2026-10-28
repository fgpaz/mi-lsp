package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/service"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
	"github.com/fgpaz/mi-lsp/internal/worker"
)

type aorphCPUCaller struct {
	entered chan context.Context
	release chan struct{}
}

func (c *aorphCPUCaller) Call(ctx context.Context, _ model.WorkspaceRegistration, _ model.WorkerRequest) (model.WorkerResponse, error) {
	c.entered <- ctx
	select {
	case <-ctx.Done():
		return model.WorkerResponse{}, ctx.Err()
	case <-c.release:
		return model.WorkerResponse{Ok: true, Backend: "roslyn"}, nil
	}
}

func (*aorphCPUCaller) Status() []model.WorkerStatus { return nil }

func aorphCPUPrivateWorkspace(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("USERNAME", "aorph-cpu-private")
	root := filepath.Join(home, "workspace")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := "aorph-cpu-private"
	if err := os.WriteFile(filepath.Join(root, "A.cs"), []byte("class A { void Target() {} }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "A.csproj"), []byte("<Project Sdk=\"Microsoft.NET.Sdk\"></Project>\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: alias, Languages: []string{"csharp"}, Kind: model.WorkspaceKindSingle, DefaultRepo: "main"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: ".", Languages: []string{"csharp"}, DefaultEntrypoint: "main-project"}},
		Entrypoints: []model.WorkspaceEntrypoint{{ID: "main-project", RepoID: "main", Path: "A.csproj", Kind: model.EntrypointKindProject, Default: true}},
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{Name: alias, Root: root, Languages: []string{"csharp"}, Kind: model.WorkspaceKindSingle}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	symbol := model.SymbolRecord{FilePath: "A.cs", Name: "Target", Kind: "method", StartLine: 1, EndLine: 1, Language: "csharp", RepoID: "main", RepoName: "main"}
	file := model.FileRecord{FilePath: "A.cs", RepoID: "main", RepoName: "main", Language: "csharp"}
	if err := store.ReplaceCatalog(context.Background(), db, project, []model.FileRecord{file}, []model.SymbolRecord{symbol}); err != nil {
		t.Fatal(err)
	}
	return root, alias
}

func TestAorphCPUServerContextCancellationReachesSemanticCaller(t *testing.T) {
	aorphCPURunCancellationCase(t, true)
}

func TestAorphCPUPeerEOFCancelsNonQSemanticRequest(t *testing.T) {
	aorphCPURunCancellationCase(t, false)
}

func aorphCPURunCancellationCase(t *testing.T, cancelServer bool) {
	t.Helper()
	root, alias := aorphCPUPrivateWorkspace(t)
	caller := &aorphCPUCaller{entered: make(chan context.Context, 1), release: make(chan struct{})}
	server := &Server{app: service.New(root, caller), inflight: make(chan struct{}, 1)}
	serverCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serverConn, clientConn := net.Pipe()
	done := make(chan struct{})
	readerDone := make(chan struct{})
	type frameResult struct {
		env model.Envelope
		err error
	}
	responses := make(chan frameResult, 1)
	defer func() {
		_ = clientConn.Close()
		close(caller.release)
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("handler did not join during bounded fixture cleanup")
		}
		select {
		case <-readerDone:
		case <-time.After(2 * time.Second):
			t.Error("reader did not join during bounded fixture cleanup")
		}
	}()
	go func() {
		server.handleConnectionContext(serverCtx, serverConn)
		close(done)
	}()
	go func() {
		defer close(readerDone)
		var env model.Envelope
		err := worker.ReadFrame(clientConn, &env)
		responses <- frameResult{env: env, err: err}
	}()
	request := model.CommandRequest{
		ProtocolVersion: model.ProtocolVersion,
		Operation:       "nav.refs",
		Context:         model.QueryOptions{Workspace: alias, MaxItems: 20, BackendHint: "roslyn"},
		Payload:         map[string]any{"symbol": "Target", "file": "A.cs", "line": 1},
	}
	if err := clientConn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set bounded frame deadline: %v", err)
	}
	if err := worker.WriteFrame(clientConn, request); err != nil {
		t.Fatalf("write request: %v", err)
	}
	var requestCtx context.Context
	select {
	case requestCtx = <-caller.entered:
	case result := <-responses:
		if result.err != nil {
			t.Fatal("FIXTURE_INVALID: response frame unavailable before SemanticCaller")
		}
		backend := "other"
		switch result.env.Backend {
		case "daemon", "nav", "router", "roslyn", "text", "catalog":
			backend = result.env.Backend
		}
		code, message := "none", "none"
		if result.env.Error != nil {
			switch result.env.Error.Code {
			case "backpressure_busy", "daemon_request_failed", "daemon_transport_failed", "workspace_resolution_failed":
				code = result.env.Error.Code
			default:
				code = "other"
			}
			switch result.env.Error.Message {
			case "file is required", "symbol is required":
				message = result.env.Error.Message
			default:
				message = "redacted"
			}
		}
		t.Fatalf("FIXTURE_INVALID: response before SemanticCaller: ok=%t backend=%s error-code=%s message=%s", result.env.Ok, backend, code, message)
	case <-time.After(2 * time.Second):
		t.Fatal("FIXTURE_INVALID: request did not reach SemanticCaller")
	}
	if cancelServer {
		cancel()
	} else {
		if err := clientConn.Close(); err != nil {
			t.Fatalf("close client peer: %v", err)
		}
	}
	select {
	case <-requestCtx.Done():
	case <-time.After(400 * time.Millisecond):
		if !cancelServer {
			t.Fatal("peer EOF did not cancel non-q SemanticCaller context")
		}
		t.Fatal("server/root cancellation did not reach SemanticCaller")
	}
	if !cancelServer && serverCtx.Err() != nil {
		t.Fatalf("peer EOF branch unexpectedly cancelled server context: %v", serverCtx.Err())
	}
}
