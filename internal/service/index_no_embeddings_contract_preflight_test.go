package service

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestIndexNoEmbeddingsContractPreflightBuildsRealDocsGraphWithoutProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	for _, key := range []string{"MI_LSP_EMBEDDINGS_API_KEY", "OPENAI_API_KEY", "MI_LSP_EMBEDDINGS_BASE_URL", "MI_LSP_EMBEDDINGS_MODEL"} {
		t.Setenv(key, "")
	}

	root := t.TempDir()
	alias := "no-embeddings-docs-" + filepath.Base(root)
	project := testProject(alias)
	project.Embeddings = &model.EmbeddingsBlock{BaseURL: "http://127.0.0.1:1/v1", Model: "fixture", Dim: 3, BatchSize: 1, TimeoutMS: 10}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	writeWorkspaceFile(t, root, ".docs/wiki/architecture.md", "# Architecture\n\nThis canonical document remains available without embeddings.\n")
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{Name: alias, Root: root, Languages: []string{"go"}, Kind: model.WorkspaceKindSingle}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })
	seedDB, err := store.Open(root)
	if err != nil {
		t.Fatalf("open seed store: %v", err)
	}
	seed := model.WikiChunkEmbedding{DocPath: ".docs/wiki/architecture.md", ChunkID: "preexisting", ContentHash: "preserve-me", EmbeddingModel: "fixture", EmbeddingDim: 1, Embedding: []byte{0, 0, 128, 63}, IndexedAt: 1}
	if err := store.UpsertWikiChunkEmbeddings(context.Background(), seedDB, []model.WikiChunkEmbedding{seed}); err != nil {
		_ = seedDB.Close()
		t.Fatalf("seed existing wiki vector: %v", err)
	}
	if err := seedDB.Close(); err != nil {
		t.Fatalf("close seed store: %v", err)
	}

	env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "index.run",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"docs_only": true, "no_embeddings": true},
	})
	if err != nil {
		t.Fatalf("index.run docs-only no-embeddings: %v", err)
	}
	if !env.Ok {
		t.Fatalf("index.run envelope not ok: %#v", env)
	}
	if !strings.Contains(strings.Join(env.Warnings, "\n"), "skipped by operation policy") {
		t.Fatalf("explicit skip is not observable in warnings: %#v", env.Warnings)
	}

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	docs, err := store.ListDocRecords(context.Background(), db)
	if err != nil {
		t.Fatalf("ListDocRecords: %v", err)
	}
	if len(docs) == 0 {
		t.Fatal("docs-only no-embeddings did not publish real document records")
	}
	embeddings, err := store.LoadWikiChunkEmbeddings(context.Background(), db)
	if err != nil {
		t.Fatalf("LoadWikiChunkEmbeddings: %v", err)
	}
	if preserved, ok := embeddings[".docs/wiki/architecture.md\x00preexisting"]; !ok || preserved.ContentHash != "preserve-me" {
		t.Fatalf("preexisting wiki vector changed or disappeared: %#v", preserved)
	}
	var graphNodes int
	if err := db.QueryRowContext(context.Background(), "SELECT COALESCE(MAX(node_count), 0) FROM graph_generations WHERE status='active'").Scan(&graphNodes); err != nil {
		t.Fatalf("read active docs graph: %v", err)
	}
	if graphNodes == 0 {
		t.Fatal("docs-only no-embeddings did not publish a non-empty GraphGeneration")
	}
}

func TestIndexStartNoEmbeddingsContractPreflightWaitPreservesDocsGraph(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	for _, key := range []string{"MI_LSP_EMBEDDINGS_API_KEY", "OPENAI_API_KEY", "MI_LSP_EMBEDDINGS_BASE_URL", "MI_LSP_EMBEDDINGS_MODEL"} {
		t.Setenv(key, "")
	}

	root := t.TempDir()
	alias := "no-embeddings-wait-" + filepath.Base(root)
	project := testProject(alias)
	project.Embeddings = &model.EmbeddingsBlock{BaseURL: "http://127.0.0.1:1/v1", Model: "fixture", Dim: 3, BatchSize: 1, TimeoutMS: 10}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	writeWorkspaceFile(t, root, ".docs/wiki/architecture.md", "# Architecture\n\nThe synchronous durable job retains its documentation graph.\n")
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{Name: alias, Root: root, Languages: []string{"go"}, Kind: model.WorkspaceKindSingle}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })

	env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "index.start",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"mode": "docs", "wait": true, "no_embeddings": true},
	})
	if err != nil || !env.Ok {
		t.Fatalf("index.start wait no-embeddings = (%+v, %v)", env, err)
	}
	if !strings.Contains(strings.Join(env.Warnings, "\n"), "skipped by operation policy") {
		t.Fatalf("synchronous job did not report the embeddings skip: %#v", env.Warnings)
	}
	jobs, ok := env.Items.([]store.IndexJob)
	if !ok || len(jobs) != 1 || !jobs[0].NoEmbeddings || jobs[0].EmbeddingsOutcome != "skipped_by_operation" || jobs[0].Status != store.IndexJobSucceeded {
		t.Fatalf("synchronous job policy/result = %#v, want succeeded explicit skip", env.Items)
	}
	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	var graphNodes int
	if err := db.QueryRowContext(context.Background(), "SELECT COALESCE(MAX(node_count), 0) FROM graph_generations WHERE status='active'").Scan(&graphNodes); err != nil {
		t.Fatalf("read active docs graph: %v", err)
	}
	if graphNodes == 0 {
		t.Fatal("synchronous no-embeddings job did not publish a non-empty GraphGeneration")
	}
}

func TestIndexStartNoEmbeddingsContractPreflightPersistsPolicyAndRejectsMismatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))

	root := t.TempDir()
	alias := "no-embeddings-job-" + filepath.Base(root)
	project := testProject(alias)
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	writeWorkspaceFile(t, root, ".docs/wiki/architecture.md", "# Architecture\n\nDurable job policy fixture.\n")
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{Name: alias, Root: root, Languages: []string{"go"}, Kind: model.WorkspaceKindSingle}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })

	oldSpawner := spawnDetachedIndexJobProcess
	spawnDetachedIndexJobProcess = func(model.WorkspaceRegistration, string) (int, error) { return os.Getpid(), nil }
	t.Cleanup(func() { spawnDetachedIndexJobProcess = oldSpawner })

	app := New(root, nil)
	first, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "index.start",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"mode": "docs", "no_embeddings": true},
	})
	if err != nil {
		t.Fatalf("index.start no-embeddings: %v", err)
	}
	jobs, ok := first.Items.([]store.IndexJob)
	if !ok || len(jobs) != 1 {
		t.Fatalf("started items = %#v, want one durable job", first.Items)
	}
	jobID := jobs[0].JobID
	if !jobs[0].NoEmbeddings || jobs[0].EmbeddingsOutcome != "skipped_by_operation" {
		t.Fatalf("started job policy/result = (%t, %q), want explicit skip", jobs[0].NoEmbeddings, jobs[0].EmbeddingsOutcome)
	}

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	persisted, found, err := store.GetIndexJob(context.Background(), db, jobID)
	if err != nil || !found {
		t.Fatalf("GetIndexJob = (found %t, err %v)", found, err)
	}
	if !persisted.NoEmbeddings || persisted.EmbeddingsOutcome != "skipped_by_operation" {
		t.Fatalf("persisted job policy/result = (%t, %q), want explicit skip", persisted.NoEmbeddings, persisted.EmbeddingsOutcome)
	}
	status, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "index.status",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"job_id": jobID},
	})
	if err != nil || !status.Ok {
		t.Fatalf("index.status: envelope=%+v err=%v", status, err)
	}
	statusJobs, ok := status.Items.([]store.IndexJob)
	if !ok || len(statusJobs) != 1 || !statusJobs[0].NoEmbeddings || statusJobs[0].EmbeddingsOutcome != "skipped_by_operation" {
		t.Fatalf("index.status policy/result = %#v, want explicit skip", status.Items)
	}

	var jobsBefore, generationsBefore int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM index_jobs").Scan(&jobsBefore); err != nil {
		t.Fatalf("count jobs before mismatch: %v", err)
	}
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM index_generations").Scan(&generationsBefore); err != nil {
		t.Fatalf("count generations before mismatch: %v", err)
	}

	mismatch, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "index.start",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"mode": "docs", "no_embeddings": false},
	})
	if err != nil {
		t.Fatalf("index.start policy mismatch: %v", err)
	}
	if mismatch.Ok || mismatch.Error == nil || mismatch.Error.Code != "index_job_policy_mismatch" {
		t.Fatalf("mismatch envelope = %#v, want index_job_policy_mismatch", mismatch)
	}
	if !strings.Contains(mismatch.Error.Message, "active_no_embeddings=true") || !strings.Contains(mismatch.Error.Message, "requested_no_embeddings=false") {
		t.Fatalf("mismatch message omits both policies: %q", mismatch.Error.Message)
	}
	var jobsAfter, generationsAfter int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM index_jobs").Scan(&jobsAfter); err != nil {
		t.Fatalf("count jobs after mismatch: %v", err)
	}
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM index_generations").Scan(&generationsAfter); err != nil {
		t.Fatalf("count generations after mismatch: %v", err)
	}
	if jobsAfter != jobsBefore || generationsAfter != generationsBefore {
		t.Fatalf("mismatch changed durable rows: jobs %d->%d, generations %d->%d", jobsBefore, jobsAfter, generationsBefore, generationsAfter)
	}
	unchanged, found, err := store.GetIndexJob(context.Background(), db, jobID)
	if err != nil || !found || !unchanged.NoEmbeddings || unchanged.EmbeddingsOutcome != "skipped_by_operation" {
		t.Fatalf("active job changed after mismatch: job=%+v found=%t err=%v", unchanged, found, err)
	}

	defaultRoot := t.TempDir()
	defaultDB, err := store.Open(defaultRoot)
	if err != nil {
		t.Fatalf("open default-policy store: %v", err)
	}
	defer defaultDB.Close()
	defaultJob, err := store.CreateIndexJobWithPolicies(context.Background(), defaultDB, "default", defaultRoot, store.IndexModeDocs, false, "", false, false)
	if err != nil {
		t.Fatalf("create default-policy job: %v", err)
	}
	loadedDefault, found, err := store.GetIndexJob(context.Background(), defaultDB, defaultJob.JobID)
	if err != nil || !found || loadedDefault.NoEmbeddings || loadedDefault.EmbeddingsOutcome != "" {
		t.Fatalf("default job policy/result = (%t, %q), found %t err %v", loadedDefault.NoEmbeddings, loadedDefault.EmbeddingsOutcome, found, err)
	}
}

func TestIndexNoEmbeddingsContractPreflightDetachedRunJobPropagation(t *testing.T) {
	want := []string{"--workspace", "fixture", "--format", "json", "index", "run-job", "--no-embeddings", "job-1"}
	if got := detachedIndexJobCommandArgs("fixture", "job-1", true); !reflect.DeepEqual(got, want) {
		t.Fatalf("detached args = %#v, want %#v", got, want)
	}
	wantDefault := []string{"--workspace", "fixture", "--format", "json", "index", "run-job", "job-1"}
	if got := detachedIndexJobCommandArgs("fixture", "job-1", false); !reflect.DeepEqual(got, wantDefault) {
		t.Fatalf("default detached args = %#v, want %#v", got, wantDefault)
	}
}
