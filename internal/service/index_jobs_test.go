package service

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestRunIndexJobHonorsCooperativeCancelDuringProgress(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	alias := "index-cancel-progress-" + filepath.Base(root)
	if err := workspace.SaveProjectFile(root, testProject(alias)); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, "src/App.cs", "namespace Demo; public class App { }\n")

	registration := model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}
	ctx := context.Background()
	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	job, err := store.CreateIndexJob(ctx, db, alias, root, store.IndexModeCatalog, false)
	if err != nil {
		db.Close()
		t.Fatalf("CreateIndexJob: %v", err)
	}
	_ = db.Close()

	oldHook := indexJobProgressAfterMarkHook
	var canceled atomic.Bool
	indexJobProgressAfterMarkHook = func(ctx context.Context, db *sql.DB, jobID string, progress indexer.Progress) error {
		if progress.Stage != "catalog.detect" || !canceled.CompareAndSwap(false, true) {
			return nil
		}
		_, err := store.RequestIndexJobCancel(ctx, db, jobID)
		return err
	}
	t.Cleanup(func() { indexJobProgressAfterMarkHook = oldHook })

	app := New(root, nil)
	resultJob, _, err := app.runIndexJob(ctx, registration, job.JobID)
	if err != nil {
		t.Fatalf("runIndexJob: %v", err)
	}
	if !canceled.Load() {
		t.Fatal("cooperative cancellation was not requested during catalog progress")
	}
	if resultJob.Status != store.IndexJobCanceled || resultJob.FinishedAt == "" {
		t.Fatalf("job = status=%q finished_at=%q, want canceled with terminal timestamp", resultJob.Status, resultJob.FinishedAt)
	}

	db, err = store.Open(root)
	if err != nil {
		t.Fatalf("store.Open(after): %v", err)
	}
	defer db.Close()
	var released sql.NullString
	var owner string
	if err := db.QueryRow(`SELECT owner_token, released_at FROM index_job_ownership WHERE job_id = ?`, job.JobID).Scan(&owner, &released); err != nil {
		t.Fatalf("read ownership: %v", err)
	}
	if owner != job.OwnerToken || !released.Valid {
		t.Fatalf("ownership = owner=%q released=%v, want original owner released", owner, released.Valid)
	}
	wrongFence := store.IndexJobFence{OwnerToken: "not-the-owner", FencingToken: job.FencingToken}
	if err := store.MarkIndexJobCanceled(ctx, db, job.JobID, wrongFence); !errors.Is(err, store.ErrStaleIndexJobOwner) {
		t.Fatalf("wrong-owner terminal transition = %v, want ErrStaleIndexJobOwner", err)
	}
}

func TestRunIndexJobPropagatesGenuineStaleOwnerWithoutFailing(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	alias := "index-stale-nonterminal-" + filepath.Base(root)
	if err := workspace.SaveProjectFile(root, testProject(alias)); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, "src/App.cs", "namespace Demo; public class App { }\n")
	ctx := context.Background()
	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	job, err := store.CreateIndexJob(ctx, db, alias, root, store.IndexModeCatalog, false)
	if err != nil {
		db.Close()
		t.Fatalf("CreateIndexJob: %v", err)
	}
	_ = db.Close()

	oldHook := indexJobProgressAfterMarkHook
	var released atomic.Bool
	indexJobProgressAfterMarkHook = func(ctx context.Context, db *sql.DB, jobID string, progress indexer.Progress) error {
		if progress.Stage != "catalog.detect" || !released.CompareAndSwap(false, true) {
			return nil
		}
		_, err := db.ExecContext(ctx, `UPDATE index_job_ownership SET released_at=? WHERE job_id=? AND released_at IS NULL`, time.Now().UTC().Format(time.RFC3339Nano), jobID)
		return err
	}
	t.Cleanup(func() { indexJobProgressAfterMarkHook = oldHook })

	app := New(root, nil)
	resultJob, _, runErr := app.runIndexJob(ctx, model.WorkspaceRegistration{Name: alias, Root: root, Languages: []string{"csharp"}}, job.JobID)
	if !errors.Is(runErr, store.ErrStaleIndexJobOwner) {
		t.Fatalf("runIndexJob error = %v, want ErrStaleIndexJobOwner", runErr)
	}
	if !released.Load() {
		t.Fatal("test did not create a genuine stale owner")
	}
	if resultJob.Status != store.IndexJobRunning {
		t.Fatalf("returned job status=%q, want non-terminal running", resultJob.Status)
	}

	finalDB, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open(final): %v", err)
	}
	defer finalDB.Close()
	final, ok, err := store.GetIndexJob(ctx, finalDB, job.JobID)
	if err != nil || !ok {
		t.Fatalf("GetIndexJob(final): ok=%v err=%v", ok, err)
	}
	if final.Status != store.IndexJobRunning {
		t.Fatalf("final status=%q, want running, not failed", final.Status)
	}
	var generationStatus string
	if err := finalDB.QueryRowContext(ctx, "SELECT status FROM index_generations WHERE generation_id=?", job.GenerationID).Scan(&generationStatus); err != nil {
		t.Fatalf("generation status: %v", err)
	}
	if generationStatus != "building" {
		t.Fatalf("generation status=%q, want building", generationStatus)
	}
}

func TestRunIndexJobReconcilesPublicationWinsDuringCancel(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	alias := "index-publication-wins-" + filepath.Base(root)
	ctx := context.Background()
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", Languages: []string{"go"}},
		Repos:   []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/" + alias, Languages: []string{"go"}}},
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	writeWorkspaceFile(t, root, "go.mod", "module example.com/publication-wins\n\ngo 1.23\n")
	writeWorkspaceFile(t, root, "main.go", "package main\nfunc main() {}\n")

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	job, err := store.CreateIndexJob(ctx, db, alias, root, store.IndexModeFull, false)
	if err != nil {
		db.Close()
		t.Fatalf("CreateIndexJob: %v", err)
	}
	_ = db.Close()

	publicationEntered := make(chan struct{})
	releasePublication := make(chan struct{})
	restoreHook := store.SetIndexPublicationBeforeCommitHookForTest(func() error {
		close(publicationEntered)
		<-releasePublication
		return nil
	})
	t.Cleanup(restoreHook)

	cancelDB, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open(cancel): %v", err)
	}
	defer cancelDB.Close()

	runDone := make(chan struct {
		job    store.IndexJob
		result indexer.Result
		err    error
	}, 1)
	app := New(root, nil)
	go func() {
		resultJob, result, runErr := app.runIndexJob(ctx, model.WorkspaceRegistration{Name: alias, Root: root, Languages: []string{"go"}}, job.JobID)
		runDone <- struct {
			job    store.IndexJob
			result indexer.Result
			err    error
		}{resultJob, result, runErr}
	}()
	select {
	case <-publicationEntered:
	case <-time.After(30 * time.Second):
		t.Fatal("runIndexJob did not reach the real fenced publication commit boundary")
	}

	cancelDone := make(chan struct {
		job store.IndexJob
		err error
	}, 1)
	go func() {
		cancelJob, cancelErr := store.RequestIndexJobCancel(ctx, cancelDB, job.JobID)
		cancelDone <- struct {
			job store.IndexJob
			err error
		}{cancelJob, cancelErr}
	}()
	close(releasePublication)

	run := <-runDone
	if run.err != nil {
		t.Fatalf("runIndexJob publication-wins cancel: %v", run.err)
	}
	if run.job.Status != store.IndexJobSucceeded || run.job.FinishedAt == "" {
		t.Fatalf("job status = %q finished_at=%q, want succeeded terminal", run.job.Status, run.job.FinishedAt)
	}
	cancel := <-cancelDone
	if cancel.err != nil {
		t.Fatalf("cancel loser: %v", cancel.err)
	}
	if cancel.job.Status != store.IndexJobSucceeded {
		t.Fatalf("cancel loser observed status=%q, want succeeded", cancel.job.Status)
	}

	finalDB, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open(final): %v", err)
	}
	defer finalDB.Close()
	final, ok, err := store.GetIndexJob(ctx, finalDB, job.JobID)
	if err != nil || !ok || final.Status != store.IndexJobSucceeded {
		t.Fatalf("final job=%#v ok=%v err=%v, want succeeded", final, ok, err)
	}
	var released sql.NullString
	if err := finalDB.QueryRowContext(ctx, "SELECT released_at FROM index_job_ownership WHERE job_id=?", job.JobID).Scan(&released); err != nil {
		t.Fatalf("ownership release: %v", err)
	}
	if !released.Valid {
		t.Fatal("publication winner did not release ownership")
	}
}

func TestEmbeddingFailureAfterPublicationLeavesSucceededGeneration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		http.Error(w, "provider secret EMBEDDING_PROVIDER_SECRET", http.StatusInternalServerError)
	}))
	defer server.Close()

	ensureWritableTestHome(t)
	root := t.TempDir()
	alias := "index-embedding-post-publication-" + filepath.Base(root)
	writeWorkspaceFile(t, root, "go.mod", "module embedding-post-publication\n\ngo 1.24\n")
	writeWorkspaceFile(t, root, ".docs/wiki/post-publication.md", "# Post publication\n\nThis chunk is published before embeddings are attempted.\n")
	app := New(root, nil)
	initEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Payload:   map[string]any{"path": root, "alias": alias, "no_index": true},
	})
	if err != nil || !initEnv.Ok {
		t.Fatalf("workspace.init: ok=%v err=%v", initEnv.Ok, err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })

	project, err := workspace.LoadProjectFile(root)
	if err != nil {
		t.Fatalf("LoadProjectFile: %v", err)
	}
	project.Embeddings = &model.EmbeddingsBlock{
		Enabled:   boolPtr(true),
		Provider:  "openai",
		BaseURL:   server.URL,
		Model:     "fake",
		Dim:       8,
		BatchSize: 1,
		TimeoutMS: 5000,
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "index.start",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"docs_only": true, "wait": true},
	})
	if err != nil {
		t.Fatalf("index.start: %v", err)
	}
	if !env.Ok {
		t.Fatalf("index.start not ok: %#v", env)
	}
	jobs, ok := env.Items.([]store.IndexJob)
	if !ok || len(jobs) != 1 {
		t.Fatalf("items = %#v, want one control-plane job", env.Items)
	}
	published := jobs[0]
	if published.Status != store.IndexJobSucceeded || published.FinishedAt == "" {
		t.Fatalf("published job = status=%q finished_at=%q, want succeeded with terminal timestamp", published.Status, published.FinishedAt)
	}
	warningText := strings.Join(env.Warnings, " ")
	if !strings.Contains(warningText, "status 500") {
		t.Fatalf("warnings = %q, want sanitized provider status warning", warningText)
	}
	for _, forbidden := range []string{"EMBEDDING_PROVIDER_SECRET", "provider secret"} {
		if strings.Contains(warningText, forbidden) {
			t.Fatalf("warning leaked %q: %q", forbidden, warningText)
		}
	}

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	var generationStatus string
	if err := db.QueryRow(`SELECT status FROM index_generations WHERE generation_id = ?`, published.GenerationID).Scan(&generationStatus); err != nil {
		t.Fatalf("generation status: %v", err)
	}
	if generationStatus != "published" {
		t.Fatalf("generation status = %q, want published", generationStatus)
	}
	activeDocs, ok, err := store.WorkspaceMetaValue(context.Background(), db, store.WorkspaceMetaActiveDocsGeneration)
	if err != nil || !ok || activeDocs != published.GenerationID {
		t.Fatalf("active docs generation = %q ok=%v err=%v, want %q", activeDocs, ok, err, published.GenerationID)
	}
	var released sql.NullString
	if err := db.QueryRow(`SELECT released_at FROM index_job_ownership WHERE job_id = ?`, published.JobID).Scan(&released); err != nil {
		t.Fatalf("ownership release: %v", err)
	}
	if !released.Valid {
		t.Fatal("embedding failure after publication retained ownership")
	}

}
