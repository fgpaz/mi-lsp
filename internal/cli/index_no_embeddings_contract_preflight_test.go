package cli

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestIndexNoEmbeddingsContractPreflightPublicFlags(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantNoEmb bool
		wantMode  string
		wantWait  bool
	}{
		{name: "default", args: []string{}, wantMode: "full", wantWait: true},
		{name: "index wrapper", args: []string{"--docs-only", "--no-embeddings"}, wantNoEmb: true, wantMode: "docs", wantWait: true},
		{name: "start", args: []string{"start", "--mode", "docs", "--no-embeddings"}, wantNoEmb: true, wantMode: "docs"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotOperation string
			var gotPayload map[string]any
			state := &rootState{executeOperationHook: func(_ *cobra.Command, operation string, payload map[string]any, _ bool) error {
				gotOperation, gotPayload = operation, payload
				return nil
			}}
			command := newIndexCommand(state)
			command.SetArgs(tc.args)
			if err := command.Execute(); err != nil {
				t.Fatalf("execute CLI command: %v", err)
			}
			if gotOperation != "index.start" {
				t.Fatalf("operation = %q, want index.start", gotOperation)
			}
			if got, ok := gotPayload["no_embeddings"].(bool); !ok || got != tc.wantNoEmb {
				t.Fatalf("no_embeddings = %#v, want %t", gotPayload["no_embeddings"], tc.wantNoEmb)
			}
			if got := gotPayload["mode"]; got != tc.wantMode {
				t.Fatalf("mode = %#v, want %q", got, tc.wantMode)
			}
			if got, _ := gotPayload["wait"].(bool); got != tc.wantWait {
				t.Fatalf("wait = %#v, want %t", gotPayload["wait"], tc.wantWait)
			}
		})
	}
}

func TestIndexNoEmbeddingsContractPreflightDetachedRunJobFlag(t *testing.T) {
	var gotOperation string
	var gotPayload map[string]any
	state := &rootState{executeOperationHook: func(_ *cobra.Command, operation string, payload map[string]any, _ bool) error {
		gotOperation, gotPayload = operation, payload
		return nil
	}}
	command := newIndexCommand(state)
	command.SetArgs([]string{"run-job", "--no-embeddings", "job-1"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute hidden detached command: %v", err)
	}
	if gotOperation != "index.run-job" || gotPayload["job_id"] != "job-1" || gotPayload["no_embeddings"] != true {
		t.Fatalf("detached run-job operation/payload = %q/%#v, want persisted no_embeddings hint", gotOperation, gotPayload)
	}
}

func TestIndexNoEmbeddingsContractPreflightSkipsDynamicTimeoutResolution(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("MI_LSP_INDEX_TIMEOUT", "3m")

	root := t.TempDir()
	alias := "no-embeddings-timeout-" + filepath.Base(root)
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: alias, Languages: []string{"go"}, Kind: model.WorkspaceKindSingle, DefaultRepo: "main"},
		Repos: []model.WorkspaceRepo{{ID: "main", Name: "main", Root: ".", RepositoryIdentity: "https://example.invalid/timeout-fixture", Languages: []string{"go"}}},
		Embeddings: &model.EmbeddingsBlock{BaseURL: "http://127.0.0.1:1/v1", Model: "fixture", BatchSize: 1, TimeoutMS: 100, IndexTimeoutMS: int(time.Hour / time.Millisecond)},
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{Name: alias, Root: root, Languages: []string{"go"}, Kind: model.WorkspaceKindSingle}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })

	request := model.CommandRequest{Operation: "index.start", Context: model.QueryOptions{Workspace: alias}, Payload: map[string]any{"no_embeddings": true}}
	if got, ok := embeddingIndexTimeoutForRequest(request); ok || got != 0 {
		t.Fatalf("embedding timeout preflight = (%s, %t), want (0, false) for explicit skip", got, ok)
	}
	if got := timeoutForRequest(request); got != 3*time.Minute {
		t.Fatalf("operation timeout = %s, want unchanged base timeout 3m", got)
	}

	request.Payload["no_embeddings"] = false
	if got, ok := embeddingIndexTimeoutForRequest(request); !ok || got != time.Hour {
		t.Fatalf("default embedding timeout preflight = (%s, %t), want configured 1h", got, ok)
	}
}
