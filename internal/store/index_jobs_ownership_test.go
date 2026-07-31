package store

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestCreateIndexJobConcurrentStartsReserveOneWorkspace(t *testing.T) {
	db1, root := seedTestDB(t)
	db2, err := Open(root)
	if err != nil {
		t.Fatalf("Open second connection: %v", err)
	}
	defer db2.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, db := range []*sql.DB{db1, db2} {
		wg.Add(1)
		go func(db *sql.DB) {
			defer wg.Done()
			_, err := CreateIndexJob(ctx, db, "concurrent", root, IndexModeFull, false)
			results <- err
		}(db)
	}
	wg.Wait()
	close(results)

	var created, active int
	for err := range results {
		switch {
		case err == nil:
			created++
		default:
			var activeErr *ActiveIndexJobError
			if errors.As(err, &activeErr) {
				active++
				continue
			}
			t.Fatalf("concurrent reservation error = %T %v", err, err)
		}
	}
	if created != 1 || active != 1 {
		t.Fatalf("concurrent reservation results created=%d active=%d, want 1/1", created, active)
	}

	var reservations int
	if err := db1.QueryRow(`SELECT COUNT(*) FROM index_job_ownership WHERE workspace_root = ? AND released_at IS NULL`, root).Scan(&reservations); err != nil {
		t.Fatalf("count active reservations: %v", err)
	}
	if reservations != 1 {
		t.Fatalf("active reservations = %d, want 1", reservations)
	}
}

func TestCancelIndexJobQueuedImmediate(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	job, err := CreateIndexJob(ctx, db, "queued-cancel", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}

	canceled, err := CancelIndexJob(ctx, db, job.JobID, false)
	if err != nil {
		t.Fatalf("CancelIndexJob queued: %v", err)
	}
	if canceled.Status != IndexJobCanceled || canceled.Phase != "canceled" || !canceled.RequestedCancel {
		t.Fatalf("canceled job = status=%q phase=%q requested_cancel=%v, want canceled/canceled/true", canceled.Status, canceled.Phase, canceled.RequestedCancel)
	}
	var ownershipReleased sql.NullString
	if err := db.QueryRow(`SELECT released_at FROM index_job_ownership WHERE job_id = ?`, job.JobID).Scan(&ownershipReleased); err != nil {
		t.Fatalf("read ownership: %v", err)
	}
	if !ownershipReleased.Valid {
		t.Fatal("queued cancellation did not release ownership")
	}
	var generationStatus string
	if err := db.QueryRow(`SELECT status FROM index_generations WHERE job_id = ?`, job.JobID).Scan(&generationStatus); err != nil {
		t.Fatalf("read generation: %v", err)
	}
	if generationStatus != "canceled" {
		t.Fatalf("generation status = %q, want canceled", generationStatus)
	}
}

func TestIndexJobStaleFenceCannotWinAfterTerminalRelease(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()

	first, err := CreateIndexJob(ctx, db, "fence", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob first: %v", err)
	}
	firstFence := IndexJobFence{OwnerToken: first.OwnerToken, FencingToken: first.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, first.JobID, 101, "indexing", firstFence); err != nil {
		t.Fatalf("MarkIndexJobRunning first: %v", err)
	}
	if err := MarkIndexJobSucceeded(ctx, db, first.JobID, 1, 2, 3, firstFence); err != nil {
		t.Fatalf("MarkIndexJobSucceeded first: %v", err)
	}

	second, err := CreateIndexJob(ctx, db, "fence", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob second: %v", err)
	}
	if second.JobID == first.JobID {
		t.Fatal("replacement job reused stale job id")
	}
	if err := MarkIndexJobSucceeded(ctx, db, first.JobID, 9, 9, 9, firstFence); !errors.Is(err, ErrStaleIndexJobOwner) {
		t.Fatalf("stale terminal update error = %v, want ErrStaleIndexJobOwner", err)
	}

	current, ok, err := GetIndexJob(ctx, db, second.JobID)
	if err != nil || !ok {
		t.Fatalf("GetIndexJob second: ok=%v err=%v", ok, err)
	}
	if current.Status != IndexJobQueued {
		t.Fatalf("replacement status = %q, want queued", current.Status)
	}
}

func TestIndexJobTerminalRaceHasSingleImmutableWinner(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	oldProcessExists := indexJobProcessExists
	indexJobProcessExists = func(pid int) bool { return pid == 202 }
	t.Cleanup(func() { indexJobProcessExists = oldProcessExists })
	job, err := CreateIndexJob(ctx, db, "race", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	jobFence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, job.JobID, 202, "indexing", jobFence); err != nil {
		t.Fatalf("MarkIndexJobRunning: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		requested, requestErr := RequestIndexJobCancel(ctx, db, job.JobID)
		if requestErr == nil && requested.Status == IndexJobCancelRequested {
			_ = MarkIndexJobCanceled(ctx, db, job.JobID, jobFence)
		}
	}()
	go func() {
		defer wg.Done()
		_ = MarkIndexJobSucceeded(ctx, db, job.JobID, 3, 4, 5, jobFence)
	}()
	wg.Wait()

	final, ok, err := GetIndexJob(ctx, db, job.JobID)
	if err != nil || !ok {
		t.Fatalf("GetIndexJob final: ok=%v err=%v", ok, err)
	}
	if final.Status != IndexJobSucceeded && final.Status != IndexJobCanceled {
		t.Fatalf("terminal race status = %q, want succeeded or canceled", final.Status)
	}
	if err := MarkIndexJobSucceeded(ctx, db, job.JobID, 7, 8, 9, jobFence); !errors.Is(err, ErrStaleIndexJobOwner) {
		t.Fatalf("post-race success error = %v, want ErrStaleIndexJobOwner", err)
	}
}

func TestControlPlaneIndexJobRedactsOwnerFence(t *testing.T) {
	private := IndexJob{JobID: "job", OwnerToken: "owner-secret", FencingToken: 7}
	public := ControlPlaneIndexJob(private)
	if public.OwnerToken != "" || public.FencingToken != 0 {
		t.Fatalf("control-plane projection leaked capability: owner=%q fencing=%d", public.OwnerToken, public.FencingToken)
	}
	if private.OwnerToken != "owner-secret" || private.FencingToken != 7 {
		t.Fatal("control-plane projection mutated the owner capability in memory")
	}
}

func TestControlPlaneProjectionCannotExerciseOwnerRights(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	job, err := CreateIndexJob(ctx, db, "control-plane-capability", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	fence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, job.JobID, 0, "indexing", fence); err != nil {
		t.Fatalf("MarkIndexJobRunning: %v", err)
	}
	public := ControlPlaneIndexJob(job)
	if public.OwnerToken != "" || public.FencingToken != 0 {
		t.Fatalf("public capability = owner=%q fencing=%d, want empty/zero", public.OwnerToken, public.FencingToken)
	}
	publicFence := IndexJobFence{OwnerToken: public.OwnerToken, FencingToken: public.FencingToken}
	if err := PublishIncrementalGenerationForJob(ctx, db, job.JobID, job.GenerationID, 1, 1, 1, publicFence, nil); !errors.Is(err, ErrStaleIndexJobOwner) {
		t.Fatalf("control-plane publish error = %v, want ErrStaleIndexJobOwner", err)
	}
	if err := MarkIndexJobProgress(ctx, db, job.JobID, IndexJobProgress{CurrentStage: "progress"}, publicFence); !errors.Is(err, ErrStaleIndexJobOwner) {
		t.Fatalf("control-plane progress error = %v, want ErrStaleIndexJobOwner", err)
	}
	if _, err := RequestIndexJobCancelByID(ctx, db, job.JobID); err != nil {
		t.Fatalf("RequestIndexJobCancelByID: %v", err)
	}
	if err := MarkIndexJobCanceled(ctx, db, job.JobID, publicFence); !errors.Is(err, ErrStaleIndexJobOwner) {
		t.Fatalf("control-plane terminal error = %v, want ErrStaleIndexJobOwner", err)
	}
	final, ok, err := GetIndexJob(ctx, db, job.JobID)
	if err != nil || !ok {
		t.Fatalf("GetIndexJob: ok=%v err=%v", ok, err)
	}
	if final.Status != IndexJobCancelRequested {
		t.Fatalf("final status = %q, want cancel_requested until owner confirms", final.Status)
	}
	var released sql.NullString
	if err := db.QueryRow(`SELECT released_at FROM index_job_ownership WHERE job_id = ?`, job.JobID).Scan(&released); err != nil {
		t.Fatalf("read ownership: %v", err)
	}
	if released.Valid {
		t.Fatal("control-plane projection released owner capability")
	}
}

func TestOwnerBoundIndexJobMutationsRequireExplicitFence(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	oldProcessExists := indexJobProcessExists
	indexJobProcessExists = func(pid int) bool { return pid == 303 }
	t.Cleanup(func() { indexJobProcessExists = oldProcessExists })
	job, err := CreateIndexJob(ctx, db, "missing-fence", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	if err := MarkIndexJobRunning(ctx, db, job.JobID, 303, "indexing", IndexJobFence{}); !errors.Is(err, ErrStaleIndexJobOwner) {
		t.Fatalf("missing running fence error = %v, want ErrStaleIndexJobOwner", err)
	}

	fence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, job.JobID, 303, "indexing", fence); err != nil {
		t.Fatalf("MarkIndexJobRunning: %v", err)
	}
	if err := MarkIndexJobProgress(ctx, db, job.JobID, IndexJobProgress{CurrentStage: "indexing"}, IndexJobFence{}); !errors.Is(err, ErrStaleIndexJobOwner) {
		t.Fatalf("missing progress fence error = %v, want ErrStaleIndexJobOwner", err)
	}
	if err := MarkIndexJobSucceeded(ctx, db, job.JobID, 1, 1, 1, IndexJobFence{}); !errors.Is(err, ErrStaleIndexJobOwner) {
		t.Fatalf("missing success fence error = %v, want ErrStaleIndexJobOwner", err)
	}
	if _, err := RequestIndexJobCancelByID(ctx, db, job.JobID); err != nil {
		t.Fatalf("RequestIndexJobCancelByID: %v", err)
	}
	if err := MarkIndexJobCanceled(ctx, db, job.JobID, IndexJobFence{}); !errors.Is(err, ErrStaleIndexJobOwner) {
		t.Fatalf("missing canceled fence error = %v, want ErrStaleIndexJobOwner", err)
	}
	if err := MarkIndexJobCanceled(ctx, db, job.JobID, fence); err != nil {
		t.Fatalf("MarkIndexJobCanceled with owner fence: %v", err)
	}
}

func TestIndexJobWrongOwnerFenceCannotTransition(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	job, err := CreateIndexJob(ctx, db, "wrong-fence", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	wrong := IndexJobFence{OwnerToken: "wrong-owner", FencingToken: job.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, job.JobID, 303, "indexing", wrong); !errors.Is(err, ErrStaleIndexJobOwner) {
		t.Fatalf("wrong owner transition error = %v, want ErrStaleIndexJobOwner", err)
	}
	correct := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, job.JobID, 303, "indexing", correct); err != nil {
		t.Fatalf("correct owner transition: %v", err)
	}
}

func TestLegacyPID0ModernSemantics(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	job, err := CreateIndexJob(ctx, db, "modern-pid0", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	stale, err := staleIndexJobForRead(ctx, db, job)
	if err != nil {
		t.Fatalf("staleIndexJobForRead modern PID0: %v", err)
	}
	if stale {
		t.Fatal("modern ownership row with PID 0 was classified stale")
	}
}

func TestLegacyPID0Compatibility(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	job, err := CreateIndexJob(ctx, db, "legacy-pid0", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM index_job_ownership WHERE job_id = ?`, job.JobID); err != nil {
		t.Fatalf("remove ownership for legacy fixture: %v", err)
	}
	stale, err := staleIndexJobForRead(ctx, db, job)
	if err != nil {
		t.Fatalf("staleIndexJobForRead legacy PID0: %v", err)
	}
	if !stale {
		t.Fatal("legacy active row with PID 0 was not classified stale")
	}
}

func TestConcurrentLegacyOwnershipMigration(t *testing.T) {
	baseDB, root := seedTestDB(t)
	if err := baseDB.Close(); err != nil {
		t.Fatalf("close initial database: %v", err)
	}
	legacyDB, err := Open(root)
	if err != nil {
		t.Fatalf("Open legacy fixture: %v", err)
	}
	if err := ensureIndexJobOwnershipSchema(legacyDB); err != nil {
		legacyDB.Close()
		t.Fatalf("prepare ownership schema: %v", err)
	}
	if _, err := legacyDB.Exec(`ALTER TABLE index_jobs DROP COLUMN requested_cancel`); err != nil {
		legacyDB.Close()
		t.Fatalf("drop requested_cancel: %v", err)
	}
	if _, err := legacyDB.Exec(`ALTER TABLE index_jobs DROP COLUMN publication_started`); err != nil {
		legacyDB.Close()
		t.Fatalf("drop publication_started: %v", err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("close legacy fixture: %v", err)
	}

	const workers = 4
	connections := make([]*sql.DB, workers)
	for i := range connections {
		connections[i], err = sql.Open(driverName, WorkspaceDBPath(root))
		if err != nil {
			t.Fatalf("open migration connection %d: %v", i, err)
		}
		connections[i].SetMaxOpenConns(1)
		if err := configureWorkspaceDB(connections[i]); err != nil {
			connections[i].Close()
			t.Fatalf("configure migration connection %d: %v", i, err)
		}
	}
	start := make(chan struct{})
	errs := make(chan error, workers)
	for _, connection := range connections {
		go func(db *sql.DB) {
			<-start
			errs <- ensureIndexJobOwnershipSchema(db)
		}(connection)
	}
	close(start)
	for i := 0; i < workers; i++ {
		if err := <-errs; err != nil {
			for _, connection := range connections {
				_ = connection.Close()
			}
			t.Fatalf("concurrent legacy migration: %v", err)
		}
	}
	for _, connection := range connections {
		if err := connection.Close(); err != nil {
			t.Fatalf("close migration connection: %v", err)
		}
	}

	migrated, err := Open(root)
	if err != nil {
		t.Fatalf("Open migrated database: %v", err)
	}
	defer migrated.Close()
	for _, column := range []string{"requested_cancel", "publication_started"} {
		hasColumn, err := tableHasColumn(migrated, "index_jobs", column)
		if err != nil {
			t.Fatalf("check migrated column %s: %v", column, err)
		}
		if !hasColumn {
			t.Fatalf("migrated index_jobs missing %s", column)
		}
	}
}

func TestIndexJobReadCompatibilityWithoutRequestedCancelColumn(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	job, err := CreateIndexJob(ctx, db, "legacy-read", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE index_jobs DROP COLUMN requested_cancel`); err != nil {
		t.Fatalf("drop requested_cancel for legacy fixture: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close writable db: %v", err)
	}
	readOnly, err := OpenReadOnly(root)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer readOnly.Close()
	read, ok, err := GetIndexJob(ctx, readOnly, job.JobID)
	if err != nil || !ok {
		t.Fatalf("GetIndexJob legacy: ok=%v err=%v", ok, err)
	}
	if read.RequestedCancel {
		t.Fatal("legacy read synthesized requested_cancel=true")
	}
}

func TestFencedPublicationModeMatrix(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	snapshot := model.ReentryMemorySnapshot{SnapshotBuiltAt: time.Now().UTC()}

	publishers := []struct {
		name    string
		mode    string
		publish func(IndexJob, IndexJobFence) error
	}{
		{
			name: "full",
			mode: IndexModeFull,
			publish: func(job IndexJob, fence IndexJobFence) error {
				return ReplaceWorkspaceIndexForJob(ctx, db, job.JobID, job.GenerationID, model.ProjectFile{}, nil, nil, nil, nil, nil, nil, nil, snapshot, fence, nil)
			},
		},
		{
			name: "docs",
			mode: IndexModeDocs,
			publish: func(job IndexJob, fence IndexJobFence) error {
				return ReplaceWorkspaceDocsForJob(ctx, db, job.JobID, job.GenerationID, nil, nil, nil, nil, nil, snapshot, fence)
			},
		},
		{
			name: "catalog",
			mode: IndexModeCatalog,
			publish: func(job IndexJob, fence IndexJobFence) error {
				return ReplaceWorkspaceCatalogForJob(ctx, db, job.JobID, job.GenerationID, model.ProjectFile{}, nil, nil, fence)
			},
		},
		{
			name: "incremental",
			mode: IndexModeFull,
			publish: func(job IndexJob, fence IndexJobFence) error {
				return PublishIncrementalGenerationForJob(ctx, db, job.JobID, job.GenerationID, 0, 0, 0, fence, nil)
			},
		},
	}

	for _, publisher := range publishers {
		t.Run(publisher.name, func(t *testing.T) {
			job, err := CreateIndexJob(ctx, db, "fenced-"+publisher.name, root, publisher.mode, false)
			if err != nil {
				t.Fatalf("CreateIndexJob: %v", err)
			}
			fence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
			wrongFence := IndexJobFence{OwnerToken: "wrong-owner", FencingToken: fence.FencingToken}
			if err := publisher.publish(job, wrongFence); !errors.Is(err, ErrStaleIndexJobOwner) {
				t.Fatalf("wrong fence publication error = %v, want ErrStaleIndexJobOwner", err)
			}
			if err := MarkIndexJobRunning(ctx, db, job.JobID, 0, "indexing", fence); err != nil {
				t.Fatalf("MarkIndexJobRunning: %v", err)
			}
			if err := publisher.publish(job, fence); err != nil {
				t.Fatalf("owner-bound %s publication: %v", publisher.name, err)
			}
			final, ok, err := GetIndexJob(ctx, db, job.JobID)
			if err != nil || !ok {
				t.Fatalf("GetIndexJob: ok=%v err=%v", ok, err)
			}
			if final.Status != IndexJobSucceeded {
				t.Fatalf("job status = %q, want succeeded", final.Status)
			}
			var generationStatus string
			if err := db.QueryRow(`SELECT status FROM index_generations WHERE generation_id = ?`, job.GenerationID).Scan(&generationStatus); err != nil {
				t.Fatalf("read generation status: %v", err)
			}
			if generationStatus != "published" {
				t.Fatalf("generation status = %q, want published", generationStatus)
			}
		})
	}
}

func TestStaleOrCanceledFencePreservesPointersAndRuntime(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	baseline, err := CreateIndexJob(ctx, db, "baseline", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob baseline: %v", err)
	}
	baselineFence := IndexJobFence{OwnerToken: baseline.OwnerToken, FencingToken: baseline.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, baseline.JobID, 0, "indexing", baselineFence); err != nil {
		t.Fatalf("MarkIndexJobRunning baseline: %v", err)
	}
	if err := ReplaceWorkspaceIndexForJob(ctx, db, baseline.JobID, baseline.GenerationID, model.ProjectFile{}, nil, nil, nil, nil, nil, nil, nil, model.ReentryMemorySnapshot{SnapshotBuiltAt: time.Now().UTC()}, baselineFence, nil); err != nil {
		t.Fatalf("baseline publication: %v", err)
	}

	keys := []string{WorkspaceMetaActiveCatalogGeneration, WorkspaceMetaActiveDocsGeneration, WorkspaceMetaActiveMemoryGeneration, WorkspaceMetaLastIndexGeneration}
	before := make(map[string]string, len(keys))
	for _, key := range keys {
		value, ok, err := WorkspaceMetaValue(ctx, db, key)
		if err != nil || !ok {
			t.Fatalf("read baseline metadata %s: value=%q ok=%v err=%v", key, value, ok, err)
		}
		before[key] = value
	}
	beforeRuntime, err := GraphRuntimeState(ctx, db)
	if err != nil {
		t.Fatalf("read baseline graph runtime: %v", err)
	}

	if err := PublishIncrementalGenerationForJob(ctx, db, baseline.JobID, baseline.GenerationID, 1, 1, 1, baselineFence, nil); !errors.Is(err, ErrStaleIndexJobOwner) {
		t.Fatalf("stale publication error = %v, want ErrStaleIndexJobOwner", err)
	}
	canceled, err := CreateIndexJob(ctx, db, "canceled", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob canceled: %v", err)
	}
	canceledFence := IndexJobFence{OwnerToken: canceled.OwnerToken, FencingToken: canceled.FencingToken}
	if _, err := RequestIndexJobCancelByID(ctx, db, canceled.JobID); err != nil {
		t.Fatalf("RequestIndexJobCancelByID: %v", err)
	}
	if err := PublishIncrementalGenerationForJob(ctx, db, canceled.JobID, canceled.GenerationID, 1, 1, 1, canceledFence, nil); !errors.Is(err, ErrStaleIndexJobOwner) {
		t.Fatalf("canceled publication error = %v, want ErrStaleIndexJobOwner", err)
	}

	for _, key := range keys {
		value, ok, err := WorkspaceMetaValue(ctx, db, key)
		if err != nil || !ok || value != before[key] {
			t.Fatalf("metadata %s drifted: value=%q ok=%v err=%v, want %q", key, value, ok, err, before[key])
		}
	}
	afterRuntime, err := GraphRuntimeState(ctx, db)
	if err != nil {
		t.Fatalf("read final graph runtime: %v", err)
	}
	if afterRuntime != beforeRuntime {
		t.Fatalf("graph runtime drifted from %q to %q", beforeRuntime, afterRuntime)
	}
}

func TestCancelIndexJobForceTimeoutRetainsReservation(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	job, err := CreateIndexJob(ctx, db, "terminating", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	if err := MarkIndexJobRunning(ctx, db, job.JobID, 4242, "indexing", IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}); err != nil {
		t.Fatalf("MarkIndexJobRunning: %v", err)
	}

	oldExists := indexJobProcessExists
	oldTerminate := indexJobTerminateProcess
	oldWait := indexJobWaitForExit
	defer func() {
		indexJobProcessExists = oldExists
		indexJobTerminateProcess = oldTerminate
		indexJobWaitForExit = oldWait
	}()
	indexJobProcessExists = func(pid int) bool { return pid == 4242 }
	indexJobTerminateProcess = func(int) error { return nil }
	indexJobWaitForExit = func(int, time.Duration) bool { return false }

	cancelled, err := CancelIndexJob(ctx, db, job.JobID, true)
	if err != nil {
		t.Fatalf("CancelIndexJob: %v", err)
	}
	if cancelled.Status != IndexJobCancelRequested || cancelled.Phase != "terminating" {
		t.Fatalf("timeout state = status=%q phase=%q, want cancel_requested/terminating", cancelled.Status, cancelled.Phase)
	}
	var released sql.NullString
	if err := db.QueryRow(`SELECT released_at FROM index_job_ownership WHERE job_id = ?`, job.JobID).Scan(&released); err != nil {
		t.Fatalf("read reservation: %v", err)
	}
	if released.Valid {
		t.Fatalf("reservation released at %q while process remained alive", released.String)
	}
}

func TestMarkIndexJobFailedRejectsRequestedCancellationAndKeepsOwnership(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	job, err := CreateIndexJob(ctx, db, "cancel-failure-boundary", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	fence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, job.JobID, 0, "indexing", fence); err != nil {
		t.Fatalf("MarkIndexJobRunning: %v", err)
	}
	if _, err := RequestIndexJobCancel(ctx, db, job.JobID); err != nil {
		t.Fatalf("RequestIndexJobCancel: %v", err)
	}

	if err := MarkIndexJobFailed(ctx, db, job.JobID, "stale publication failure", fence); !errors.Is(err, ErrStaleIndexJobOwner) {
		t.Fatalf("MarkIndexJobFailed error = %v, want ErrStaleIndexJobOwner", err)
	}
	current, ok, err := GetIndexJob(ctx, db, job.JobID)
	if err != nil || !ok {
		t.Fatalf("GetIndexJob: ok=%v err=%v", ok, err)
	}
	if current.Status != IndexJobCancelRequested || !current.RequestedCancel {
		t.Fatalf("job = status=%q requested_cancel=%v, want cancel_requested/true", current.Status, current.RequestedCancel)
	}
	var released sql.NullString
	if err := db.QueryRow(`SELECT released_at FROM index_job_ownership WHERE job_id = ?`, job.JobID).Scan(&released); err != nil {
		t.Fatalf("read reservation: %v", err)
	}
	if released.Valid {
		t.Fatalf("reservation released at %q after rejected failure", released.String)
	}
}

func TestIndexJobReadLoadersPreserveCancelRequestedAfterStaleFailureCAS(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	job, err := CreateIndexJob(ctx, db, "stale-cancel-read", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	fence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	oldExists := indexJobProcessExists
	indexJobProcessExists = func(pid int) bool { return pid == 5151 }
	t.Cleanup(func() { indexJobProcessExists = oldExists })
	if err := MarkIndexJobRunning(ctx, db, job.JobID, 5151, "indexing", fence); err != nil {
		t.Fatalf("MarkIndexJobRunning: %v", err)
	}
	if requested, err := RequestIndexJobCancel(ctx, db, job.JobID); err != nil || requested.Status != IndexJobCancelRequested {
		t.Fatalf("RequestIndexJobCancel: status=%q err=%v, want cancel_requested", requested.Status, err)
	}
	indexJobProcessExists = func(int) bool { return false }

	readers := []struct {
		name string
		read func() (IndexJob, bool, error)
	}{
		{name: "get", read: func() (IndexJob, bool, error) { return GetIndexJob(ctx, db, job.JobID) }},
		{name: "latest", read: func() (IndexJob, bool, error) { return LatestIndexJob(ctx, db) }},
		{name: "active", read: func() (IndexJob, bool, error) { return ActiveIndexJob(ctx, db, root) }},
	}
	for _, reader := range readers {
		t.Run(reader.name, func(t *testing.T) {
			read, ok, err := reader.read()
			if err != nil || !ok {
				t.Fatalf("read: ok=%v err=%v", ok, err)
			}
			if read.Status != IndexJobCancelRequested || !read.RequestedCancel {
				t.Fatalf("read job = status=%q requested_cancel=%v, want cancel_requested/true", read.Status, read.RequestedCancel)
			}
		})
	}

	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM index_jobs WHERE job_id = ?`, job.JobID).Scan(&status); err != nil {
		t.Fatalf("read terminal truth: %v", err)
	}
	if status != IndexJobCancelRequested {
		t.Fatalf("database status = %q, want cancel_requested", status)
	}
}

func TestIndexJobReadPropagatesMarkFailedDatabaseError(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	job, err := CreateIndexJob(ctx, db, "stale-mark-error", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	fence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	oldExists := indexJobProcessExists
	oldMarkFailed := indexJobMarkFailed
	indexJobProcessExists = func(int) bool { return false }
	markErr := errors.New("injected mark failed database error")
	indexJobMarkFailed = func(context.Context, *sql.DB, string, string, IndexJobFence) error { return markErr }
	t.Cleanup(func() {
		indexJobProcessExists = oldExists
		indexJobMarkFailed = oldMarkFailed
	})
	if err := MarkIndexJobRunning(ctx, db, job.JobID, 5252, "indexing", fence); err != nil {
		t.Fatalf("MarkIndexJobRunning: %v", err)
	}

	_, _, err = GetIndexJob(ctx, db, job.JobID)
	if !errors.Is(err, markErr) {
		t.Fatalf("GetIndexJob error = %v, want wrapped mark error", err)
	}
	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM index_jobs WHERE job_id = ?`, job.JobID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != IndexJobRunning {
		t.Fatalf("database status = %q, want running after mark error", status)
	}
}

func TestIndexJobReadPropagatesReloadErrorAfterStaleCASRace(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	job, err := CreateIndexJob(ctx, db, "stale-reload-error", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	fence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	oldExists := indexJobProcessExists
	oldMarkFailed := indexJobMarkFailed
	oldReload := indexJobReload
	indexJobProcessExists = func(int) bool { return false }
	reloadErr := errors.New("injected reload error")
	indexJobMarkFailed = func(context.Context, *sql.DB, string, string, IndexJobFence) error { return ErrStaleIndexJobOwner }
	indexJobReload = func(context.Context, *sql.DB, string) (IndexJob, bool, error) { return IndexJob{}, false, reloadErr }
	t.Cleanup(func() {
		indexJobProcessExists = oldExists
		indexJobMarkFailed = oldMarkFailed
		indexJobReload = oldReload
	})
	if err := MarkIndexJobRunning(ctx, db, job.JobID, 5353, "indexing", fence); err != nil {
		t.Fatalf("MarkIndexJobRunning: %v", err)
	}

	_, _, err = GetIndexJob(ctx, db, job.JobID)
	if !errors.Is(err, reloadErr) {
		t.Fatalf("GetIndexJob error = %v, want reload error", err)
	}
}

func TestIndexJobCancellationWinsOverStaleFailureWriter(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	job, err := CreateIndexJob(ctx, db, "cancel-publication-race", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	fence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, job.JobID, 0, "indexing", fence); err != nil {
		t.Fatalf("MarkIndexJobRunning: %v", err)
	}

	cancelDone := make(chan error, 1)
	failureDone := make(chan error, 1)
	go func() {
		if _, err := RequestIndexJobCancel(ctx, db, job.JobID); err != nil {
			cancelDone <- err
			return
		}
		cancelDone <- MarkIndexJobCanceled(ctx, db, job.JobID, fence)
	}()
	go func() {
		if err := <-cancelDone; err != nil {
			failureDone <- err
			return
		}
		failureDone <- MarkIndexJobFailed(ctx, db, job.JobID, "stale publication failure", fence)
	}()
	if err := <-failureDone; !errors.Is(err, ErrStaleIndexJobOwner) {
		t.Fatalf("stale failure writer error = %v, want ErrStaleIndexJobOwner", err)
	}

	current, ok, err := GetIndexJob(ctx, db, job.JobID)
	if err != nil || !ok {
		t.Fatalf("GetIndexJob: ok=%v err=%v", ok, err)
	}
	if current.Status == IndexJobFailed {
		t.Fatal("stale failure writer converted canceled job to failed")
	}
	if current.Status != IndexJobCanceled {
		t.Fatalf("job status = %q, want canceled", current.Status)
	}
}

func TestMarkIndexJobFailedInvalidatesGraphForGenuineFailure(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	job, err := CreateIndexJob(ctx, db, "genuine-failure", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	fence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, job.JobID, 0, "indexing", fence); err != nil {
		t.Fatalf("MarkIndexJobRunning: %v", err)
	}

	if err := MarkIndexJobFailed(ctx, db, job.JobID, "genuine indexing failure", fence); err != nil {
		t.Fatalf("MarkIndexJobFailed: %v", err)
	}
	current, ok, err := GetIndexJob(ctx, db, job.JobID)
	if err != nil || !ok {
		t.Fatalf("GetIndexJob: ok=%v err=%v", ok, err)
	}
	if current.Status != IndexJobFailed || current.Error != "genuine indexing failure" {
		t.Fatalf("job = status=%q error=%q, want failed/genuine indexing failure", current.Status, current.Error)
	}
	var generationStatus string
	if err := db.QueryRow(`SELECT status FROM index_generations WHERE job_id = ?`, job.JobID).Scan(&generationStatus); err != nil {
		t.Fatalf("read generation status: %v", err)
	}
	if generationStatus != "failed" {
		t.Fatalf("generation status = %q, want failed", generationStatus)
	}
	state, err := GraphRuntimeState(ctx, db)
	if err != nil {
		t.Fatalf("GraphRuntimeState: %v", err)
	}
	if state != GraphRuntimeStale {
		t.Fatalf("graph runtime state = %q, want stale", state)
	}
	var released sql.NullString
	if err := db.QueryRow(`SELECT released_at FROM index_job_ownership WHERE job_id = ?`, job.JobID).Scan(&released); err != nil {
		t.Fatalf("read reservation: %v", err)
	}
	if !released.Valid {
		t.Fatal("genuine failure did not release ownership")
	}
}

func TestFencedGraphPointerActivation(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	job, err := CreateIndexJob(ctx, db, "fenced-graph", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	fence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, job.JobID, 0, "indexing", fence); err != nil {
		t.Fatalf("MarkIndexJobRunning: %v", err)
	}

	bundle := testGraphBundle(t)
	graphID := bundle.Generation.GenerationID
	if err := ReplaceWorkspaceIndexForJob(ctx, db, job.JobID, job.GenerationID, model.ProjectFile{}, nil, nil, nil, nil, nil, nil, nil, model.ReentryMemorySnapshot{SnapshotBuiltAt: time.Now().UTC()}, fence, &IndexJobGraphPublication{
		GenerationID:  &graphID,
		ExpectedPrior: nil,
		PublishedAt:   time.Now().UTC(),
		GraphCurrent:  true,
		GraphBundle:   &bundle,
	}); err != nil {
		t.Fatalf("ReplaceWorkspaceIndexForJob with graph: %v", err)
	}

	active, ok, err := ActiveGraphGeneration(ctx, db)
	if err != nil || !ok || active != graphID {
		t.Fatalf("active graph = %v ok=%v err=%v, want %v", active, ok, err, graphID)
	}
	state, err := GraphRuntimeState(ctx, db)
	if err != nil {
		t.Fatalf("GraphRuntimeState: %v", err)
	}
	if state != GraphRuntimeFresh {
		t.Fatalf("graph runtime state = %q, want %q", state, GraphRuntimeFresh)
	}
	catalogGeneration, ok, err := WorkspaceMetaValue(ctx, db, GraphCatalogGenerationMeta)
	if err != nil || !ok || catalogGeneration != job.GenerationID {
		t.Fatalf("graph catalog generation = %q ok=%v err=%v, want %q", catalogGeneration, ok, err, job.GenerationID)
	}
	final, ok, err := GetIndexJob(ctx, db, job.JobID)
	if err != nil || !ok {
		t.Fatalf("GetIndexJob: ok=%v err=%v", ok, err)
	}
	if final.Status != IndexJobSucceeded {
		t.Fatalf("job status = %q, want succeeded", final.Status)
	}
}

func TestFencedGraphExpectedPriorActivation(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	first, err := CreateIndexJob(ctx, db, "graph-prior-first", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob first: %v", err)
	}
	firstFence := IndexJobFence{OwnerToken: first.OwnerToken, FencingToken: first.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, first.JobID, 0, "indexing", firstFence); err != nil {
		t.Fatalf("MarkIndexJobRunning first: %v", err)
	}
	priorBundle := testGraphBundle(t)
	priorID := priorBundle.Generation.GenerationID
	if err := ReplaceWorkspaceIndexForJob(ctx, db, first.JobID, first.GenerationID, model.ProjectFile{}, nil, nil, nil, nil, nil, nil, nil, model.ReentryMemorySnapshot{SnapshotBuiltAt: time.Now().UTC()}, firstFence, &IndexJobGraphPublication{
		GenerationID: &priorID,
		PublishedAt:  time.Now().UTC(),
		GraphCurrent: true,
		GraphBundle:  &priorBundle,
	}); err != nil {
		t.Fatalf("first graph publication: %v", err)
	}

	second, err := CreateIndexJob(ctx, db, "graph-prior-second", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob second: %v", err)
	}
	secondFence := IndexJobFence{OwnerToken: second.OwnerToken, FencingToken: second.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, second.JobID, 0, "indexing", secondFence); err != nil {
		t.Fatalf("MarkIndexJobRunning second: %v", err)
	}
	candidateBundle := testGraphBundle(t)
	candidateBundle.Nodes[0].DisplayName = "candidate"
	if err := candidateBundle.SealIDs(); err != nil {
		t.Fatalf("SealIDs candidate: %v", err)
	}
	candidateID := candidateBundle.Generation.GenerationID
	if candidateID == priorID {
		t.Fatal("candidate graph reused prior generation id")
	}
	if err := ReplaceWorkspaceIndexForJob(ctx, db, second.JobID, second.GenerationID, model.ProjectFile{}, nil, nil, nil, nil, nil, nil, nil, model.ReentryMemorySnapshot{SnapshotBuiltAt: time.Now().UTC()}, secondFence, &IndexJobGraphPublication{
		GenerationID:  &candidateID,
		ExpectedPrior: &priorID,
		PublishedAt:   time.Now().UTC(),
		GraphCurrent:  true,
		GraphBundle:   &candidateBundle,
	}); err != nil {
		t.Fatalf("expected-prior graph publication: %v", err)
	}
	active, ok, err := ActiveGraphGeneration(ctx, db)
	if err != nil || !ok || active != candidateID {
		t.Fatalf("active graph = %v ok=%v err=%v, want candidate %v", active, ok, err, candidateID)
	}
	previous, ok, err := WorkspaceMetaValue(ctx, db, "previous_graph_generation_id")
	if err != nil || !ok || previous == "" {
		t.Fatalf("previous graph metadata = %q ok=%v err=%v, want prior pointer", previous, ok, err)
	}
	state, err := GraphRuntimeState(ctx, db)
	if err != nil || state != GraphRuntimeFresh {
		t.Fatalf("graph runtime state = %q err=%v, want fresh", state, err)
	}
	catalogGeneration, ok, err := WorkspaceMetaValue(ctx, db, GraphCatalogGenerationMeta)
	if err != nil || !ok || catalogGeneration != second.GenerationID {
		t.Fatalf("graph catalog generation = %q ok=%v err=%v, want %q", catalogGeneration, ok, err, second.GenerationID)
	}
}

func TestStaleOrCanceledGraphFencePreservesPointerAndRuntime(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	baseline, err := CreateIndexJob(ctx, db, "graph-baseline", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob baseline: %v", err)
	}
	baselineFence := IndexJobFence{OwnerToken: baseline.OwnerToken, FencingToken: baseline.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, baseline.JobID, 0, "indexing", baselineFence); err != nil {
		t.Fatalf("MarkIndexJobRunning baseline: %v", err)
	}
	baselineBundle := testGraphBundle(t)
	baselineGraphID := baselineBundle.Generation.GenerationID
	if err := ReplaceWorkspaceIndexForJob(ctx, db, baseline.JobID, baseline.GenerationID, model.ProjectFile{}, nil, nil, nil, nil, nil, nil, nil, model.ReentryMemorySnapshot{SnapshotBuiltAt: time.Now().UTC()}, baselineFence, &IndexJobGraphPublication{
		GenerationID: &baselineGraphID,
		PublishedAt:  time.Now().UTC(),
		GraphCurrent: true,
		GraphBundle:  &baselineBundle,
	}); err != nil {
		t.Fatalf("baseline graph publication: %v", err)
	}
	beforeGraph, ok, err := ActiveGraphGeneration(ctx, db)
	if err != nil || !ok {
		t.Fatalf("baseline active graph = %v ok=%v err=%v", beforeGraph, ok, err)
	}
	beforeState, err := GraphRuntimeState(ctx, db)
	if err != nil {
		t.Fatalf("baseline runtime state: %v", err)
	}
	beforeCatalog, ok, err := WorkspaceMetaValue(ctx, db, GraphCatalogGenerationMeta)
	if err != nil || !ok {
		t.Fatalf("baseline graph catalog generation = %q ok=%v err=%v", beforeCatalog, ok, err)
	}

	stale, err := CreateIndexJob(ctx, db, "graph-stale", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob stale: %v", err)
	}
	staleBundle := testGraphBundle(t)
	staleGraphID := staleBundle.Generation.GenerationID
	wrongFence := IndexJobFence{OwnerToken: "stale-owner", FencingToken: stale.FencingToken}
	if err := ReplaceWorkspaceIndexForJob(ctx, db, stale.JobID, stale.GenerationID, model.ProjectFile{}, nil, nil, nil, nil, nil, nil, nil, model.ReentryMemorySnapshot{SnapshotBuiltAt: time.Now().UTC()}, wrongFence, &IndexJobGraphPublication{
		GenerationID: &staleGraphID,
		PublishedAt:  time.Now().UTC(),
		GraphCurrent: true,
		GraphBundle:  &staleBundle,
	}); !errors.Is(err, ErrStaleIndexJobOwner) {
		t.Fatalf("stale graph publication error = %v, want ErrStaleIndexJobOwner", err)
	}
	if _, err := RequestIndexJobCancelByID(ctx, db, stale.JobID); err != nil {
		t.Fatalf("cancel stale fixture: %v", err)
	}

	canceled, err := CreateIndexJob(ctx, db, "graph-canceled", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob canceled: %v", err)
	}
	canceledFence := IndexJobFence{OwnerToken: canceled.OwnerToken, FencingToken: canceled.FencingToken}
	canceledBundle := testGraphBundle(t)
	canceledGraphID := canceledBundle.Generation.GenerationID
	if _, err := RequestIndexJobCancelByID(ctx, db, canceled.JobID); err != nil {
		t.Fatalf("RequestIndexJobCancelByID canceled: %v", err)
	}
	if err := ReplaceWorkspaceIndexForJob(ctx, db, canceled.JobID, canceled.GenerationID, model.ProjectFile{}, nil, nil, nil, nil, nil, nil, nil, model.ReentryMemorySnapshot{SnapshotBuiltAt: time.Now().UTC()}, canceledFence, &IndexJobGraphPublication{
		GenerationID: &canceledGraphID,
		PublishedAt:  time.Now().UTC(),
		GraphCurrent: true,
		GraphBundle:  &canceledBundle,
	}); !errors.Is(err, ErrStaleIndexJobOwner) {
		t.Fatalf("canceled graph publication error = %v, want ErrStaleIndexJobOwner", err)
	}

	afterGraph, ok, err := ActiveGraphGeneration(ctx, db)
	if err != nil || !ok || afterGraph != beforeGraph {
		t.Fatalf("active graph changed from %v to %v ok=%v err=%v", beforeGraph, afterGraph, ok, err)
	}
	afterState, err := GraphRuntimeState(ctx, db)
	if err != nil || afterState != beforeState {
		t.Fatalf("runtime state changed from %q to %q err=%v", beforeState, afterState, err)
	}
	afterCatalog, ok, err := WorkspaceMetaValue(ctx, db, GraphCatalogGenerationMeta)
	if err != nil || !ok || afterCatalog != beforeCatalog {
		t.Fatalf("graph catalog generation changed from %q to %q ok=%v err=%v", beforeCatalog, afterCatalog, ok, err)
	}
}

func TestPublicationAndTerminalCommitAtomically(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	baselineSymbols := []model.SymbolRecord{{FilePath: "src/main.go", Name: "Before", Kind: "function", Language: "go"}}
	if err := replaceFileSymbols(ctx, db, "src/main.go", "repo", "repo", "go", "before", baselineSymbols); err != nil {
		t.Fatalf("replaceFileSymbols baseline: %v", err)
	}
	baselineBundle := testGraphBundle(t)
	if err := StageGraphGeneration(ctx, db, &baselineBundle); err != nil {
		t.Fatalf("StageGraphGeneration baseline: %v", err)
	}
	if err := ActivateGraphGenerationAt(ctx, db, baselineBundle.Generation.GenerationID, nil, time.Now().UTC()); err != nil {
		t.Fatalf("ActivateGraphGenerationAt baseline: %v", err)
	}
	if err := SetGraphRuntimeState(ctx, db, GraphRuntimeFresh, "catalog-baseline"); err != nil {
		t.Fatalf("SetGraphRuntimeState baseline: %v", err)
	}
	beforeCatalog := captureIncrementalCatalogSnapshot(t, ctx, db)
	beforeGraph, ok, err := ActiveGraphGeneration(ctx, db)
	if err != nil || !ok {
		t.Fatalf("baseline active graph=%v ok=%v err=%v", beforeGraph, ok, err)
	}
	beforeRuntime, err := GraphRuntimeState(ctx, db)
	if err != nil {
		t.Fatalf("baseline graph runtime: %v", err)
	}
	job, err := CreateIndexJob(ctx, db, "publication-atomicity", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	fence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, job.JobID, 0, "indexing", fence); err != nil {
		t.Fatalf("MarkIndexJobRunning: %v", err)
	}
	bundle := testGraphBundle(t)
	graphID := bundle.Generation.GenerationID
	commitErr := errors.New("injected publication commit boundary failure")
	oldHook := indexPublicationBeforeCommitHook
	indexPublicationBeforeCommitHook = func() error { return commitErr }
	t.Cleanup(func() { indexPublicationBeforeCommitHook = oldHook })

	changes := []IncrementalFileChange{{FilePath: "src/main.go", RepoID: "repo", RepoName: "repo", Language: "go", ContentHash: "after", Symbols: []model.SymbolRecord{{FilePath: "src/main.go", Name: "After", Kind: "function", Language: "go"}}}}
	if err := PublishIncrementalGenerationForJobWithChanges(ctx, db, job.JobID, job.GenerationID, 1, 1, 0, fence, changes, &IndexJobGraphPublication{
		GenerationID:  &graphID,
		ExpectedPrior: nil,
		PublishedAt:   time.Now().UTC(),
		GraphCurrent:  true,
		GraphBundle:   nil,
	}); !errors.Is(err, commitErr) {
		t.Fatalf("publication error = %v, want injected commit failure", err)
	}
	final, ok, err := GetIndexJob(ctx, db, job.JobID)
	if err != nil || !ok {
		t.Fatalf("GetIndexJob after rollback: ok=%v err=%v", ok, err)
	}
	if final.Status != IndexJobRunning || final.FinishedAt != "" {
		t.Fatalf("job after rollback = status=%q finished_at=%q, want running/non-terminal", final.Status, final.FinishedAt)
	}
	var generationStatus string
	if err := db.QueryRow(`SELECT status FROM index_generations WHERE generation_id = ?`, job.GenerationID).Scan(&generationStatus); err != nil {
		t.Fatalf("read index generation status: %v", err)
	}
	if generationStatus != "building" {
		t.Fatalf("index generation status = %q, want building after rollback", generationStatus)
	}
	afterCatalog := captureIncrementalCatalogSnapshot(t, ctx, db)
	if !reflect.DeepEqual(afterCatalog, beforeCatalog) {
		t.Fatalf("catalog changed after rollback: before=%#v after=%#v", beforeCatalog, afterCatalog)
	}
	afterGraph, ok, err := ActiveGraphGeneration(ctx, db)
	if err != nil || !ok || afterGraph != beforeGraph {
		t.Fatalf("active graph after rollback=%v ok=%v err=%v, want %v", afterGraph, ok, err, beforeGraph)
	}
	afterRuntime, err := GraphRuntimeState(ctx, db)
	if err != nil || afterRuntime != beforeRuntime {
		t.Fatalf("graph runtime after rollback=%q err=%v, want %q", afterRuntime, err, beforeRuntime)
	}
	if _, ok, err := WorkspaceMetaValue(ctx, db, WorkspaceMetaLastIndexGeneration); err != nil || ok {
		t.Fatalf("last index metadata after rollback ok=%v err=%v, want absent", ok, err)
	}
	var graphCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM graph_generations WHERE generation_id = ?`, graphID[:]).Scan(&graphCount); err != nil {
		t.Fatalf("count graph generation after rollback: %v", err)
	}
	if graphCount != 1 {
		t.Fatalf("graph generation count after rollback = %d, want baseline 1", graphCount)
	}
	var released sql.NullString
	if err := db.QueryRow(`SELECT released_at FROM index_job_ownership WHERE job_id = ?`, job.JobID).Scan(&released); err != nil {
		t.Fatalf("read ownership after rollback: %v", err)
	}
	if released.Valid {
		t.Fatal("ownership released after rolled-back publication")
	}

	indexPublicationBeforeCommitHook = func() error { return nil }
	if err := PublishIncrementalGenerationForJobWithChanges(ctx, db, job.JobID, job.GenerationID, 1, 1, 0, fence, changes, &IndexJobGraphPublication{
		GenerationID:  &graphID,
		ExpectedPrior: nil,
		PublishedAt:   time.Now().UTC(),
		GraphCurrent:  true,
		GraphBundle:   nil,
	}); err != nil {
		t.Fatalf("publication retry: %v", err)
	}
	final, ok, err = GetIndexJob(ctx, db, job.JobID)
	if err != nil || !ok || final.Status != IndexJobSucceeded || final.FinishedAt == "" {
		t.Fatalf("job after commit = status=%q finished_at=%q ok=%v err=%v, want succeeded/terminal", final.Status, final.FinishedAt, ok, err)
	}
	var publishedStatus string
	if err := db.QueryRow(`SELECT status FROM index_generations WHERE generation_id = ?`, job.GenerationID).Scan(&publishedStatus); err != nil {
		t.Fatalf("read published generation: %v", err)
	}
	if publishedStatus != "published" {
		t.Fatalf("generation status = %q, want published", publishedStatus)
	}
	active, ok, err := ActiveGraphGeneration(ctx, db)
	if err != nil || !ok || active != graphID {
		t.Fatalf("active graph after commit = %v ok=%v err=%v, want %v", active, ok, err, graphID)
	}
	catalogGeneration, ok, err := WorkspaceMetaValue(ctx, db, GraphCatalogGenerationMeta)
	if err != nil || !ok || catalogGeneration != job.GenerationID {
		t.Fatalf("graph catalog metadata = %q ok=%v err=%v, want %q", catalogGeneration, ok, err, job.GenerationID)
	}
	if err := db.QueryRow(`SELECT released_at FROM index_job_ownership WHERE job_id = ?`, job.JobID).Scan(&released); err != nil {
		t.Fatalf("read released ownership: %v", err)
	}
	if !released.Valid {
		t.Fatal("successful publication did not release ownership in the commit")
	}
}

func TestCancelPublicationRaceAtCommitBoundary(t *testing.T) {
	ctx := context.Background()
	oldPublicationCAS := indexPublicationBeforeOwnershipCASHook
	oldPublicationCommit := indexPublicationBeforeCommitHook
	oldCancelCAS := indexJobCancelBeforeCASHook
	t.Cleanup(func() {
		indexPublicationBeforeOwnershipCASHook = oldPublicationCAS
		indexPublicationBeforeCommitHook = oldPublicationCommit
		indexJobCancelBeforeCASHook = oldCancelCAS
	})

	t.Run("cancel-wins-before-publication-cas", func(t *testing.T) {
		db, root := seedTestDB(t)
		job, err := CreateIndexJob(ctx, db, "cancel-wins", root, IndexModeFull, false)
		if err != nil {
			t.Fatalf("CreateIndexJob: %v", err)
		}
		fence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
		if err := MarkIndexJobRunning(ctx, db, job.JobID, 0, "indexing", fence); err != nil {
			t.Fatalf("MarkIndexJobRunning: %v", err)
		}
		entered := make(chan struct{})
		release := make(chan struct{})
		indexPublicationBeforeOwnershipCASHook = func() error { close(entered); <-release; return nil }
		indexPublicationBeforeCommitHook = func() error { return nil }
		indexJobCancelBeforeCASHook = func() error { return nil }
		publicationDone := make(chan error, 1)
		go func() {
			publicationDone <- ReplaceWorkspaceIndexForJob(ctx, db, job.JobID, job.GenerationID, model.ProjectFile{}, nil, nil, nil, nil, nil, nil, nil, model.ReentryMemorySnapshot{SnapshotBuiltAt: time.Now().UTC()}, fence, nil)
		}()
		<-entered
		canceled, err := RequestIndexJobCancel(ctx, db, job.JobID)
		if err == nil && canceled.Status == IndexJobCancelRequested {
			err = MarkIndexJobCanceled(ctx, db, job.JobID, fence)
			canceled, _, _ = GetIndexJob(ctx, db, job.JobID)
		}
		if err != nil || canceled.Status != IndexJobCanceled {
			close(release)
			t.Fatalf("cancel before publication = status=%q err=%v, want canceled", canceled.Status, err)
		}
		close(release)
		if err := <-publicationDone; !errors.Is(err, ErrStaleIndexJobOwner) {
			t.Fatalf("losing publication error = %v, want ErrStaleIndexJobOwner", err)
		}
		final, ok, err := GetIndexJob(ctx, db, job.JobID)
		if err != nil || !ok || final.Status != IndexJobCanceled {
			t.Fatalf("final cancel winner = status=%q ok=%v err=%v, want canceled", final.Status, ok, err)
		}
	})

	t.Run("publication-wins-before-commit", func(t *testing.T) {
		db, root := seedTestDB(t)
		job, err := CreateIndexJob(ctx, db, "publication-wins", root, IndexModeFull, false)
		if err != nil {
			t.Fatalf("CreateIndexJob: %v", err)
		}
		fence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
		if err := MarkIndexJobRunning(ctx, db, job.JobID, 0, "indexing", fence); err != nil {
			t.Fatalf("MarkIndexJobRunning: %v", err)
		}
		publicationEntered := make(chan struct{})
		releasePublication := make(chan struct{})
		cancelEntered := make(chan struct{})
		releaseCancel := make(chan struct{})
		indexPublicationBeforeOwnershipCASHook = func() error { return nil }
		indexPublicationBeforeCommitHook = func() error { close(publicationEntered); <-releasePublication; return nil }
		indexJobCancelBeforeCASHook = func() error { close(cancelEntered); <-releaseCancel; return nil }
		publicationDone := make(chan error, 1)
		cancelDone := make(chan IndexJob, 1)
		cancelErr := make(chan error, 1)
		go func() {
			publicationDone <- ReplaceWorkspaceIndexForJob(ctx, db, job.JobID, job.GenerationID, model.ProjectFile{}, nil, nil, nil, nil, nil, nil, nil, model.ReentryMemorySnapshot{SnapshotBuiltAt: time.Now().UTC()}, fence, nil)
		}()
		<-publicationEntered
		go func() {
			requested, requestErr := RequestIndexJobCancel(ctx, db, job.JobID)
			cancelDone <- requested
			cancelErr <- requestErr
		}()
		<-cancelEntered
		close(releasePublication)
		if err := <-publicationDone; err != nil {
			t.Fatalf("publication winner error = %v", err)
		}
		close(releaseCancel)
		requested := <-cancelDone
		if err := <-cancelErr; err != nil {
			t.Fatalf("cancel loser error = %v", err)
		}
		if requested.Status != IndexJobSucceeded {
			t.Fatalf("cancel loser observed status=%q, want succeeded", requested.Status)
		}
		final, ok, err := GetIndexJob(ctx, db, job.JobID)
		if err != nil || !ok || final.Status != IndexJobSucceeded {
			t.Fatalf("final publication winner = status=%q ok=%v err=%v, want succeeded", final.Status, ok, err)
		}
		if err := MarkIndexJobFailed(ctx, db, job.JobID, "late stale failure", fence); !errors.Is(err, ErrStaleIndexJobOwner) {
			t.Fatalf("late failure writer error = %v, want ErrStaleIndexJobOwner", err)
		}
	})
}

func TestCancelPublicationRaceNeverEndsFailed(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	job, err := CreateIndexJob(ctx, db, "cancel-publication-boundary", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	fence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, job.JobID, 0, "indexing", fence); err != nil {
		t.Fatalf("MarkIndexJobRunning: %v", err)
	}

	start := make(chan struct{})
	publicationDone := make(chan error, 1)
	cancelDone := make(chan error, 1)
	go func() {
		<-start
		publicationDone <- ReplaceWorkspaceIndexForJob(ctx, db, job.JobID, job.GenerationID, model.ProjectFile{}, nil, nil, nil, nil, nil, nil, nil, model.ReentryMemorySnapshot{SnapshotBuiltAt: time.Now().UTC()}, fence, nil)
	}()
	go func() {
		<-start
		requested, requestErr := RequestIndexJobCancel(ctx, db, job.JobID)
		if requestErr != nil {
			cancelDone <- requestErr
			return
		}
		if requested.Status == IndexJobCancelRequested {
			requestErr = MarkIndexJobCanceled(ctx, db, job.JobID, fence)
		}
		cancelDone <- requestErr
	}()
	close(start)

	publicationErr := <-publicationDone
	cancelErr := <-cancelDone
	if publicationErr != nil && !errors.Is(publicationErr, ErrStaleIndexJobOwner) {
		t.Fatalf("publication race error = %v", publicationErr)
	}
	if cancelErr != nil && !errors.Is(cancelErr, ErrStaleIndexJobOwner) {
		t.Fatalf("cancel race error = %v", cancelErr)
	}
	final, ok, err := GetIndexJob(ctx, db, job.JobID)
	if err != nil || !ok {
		t.Fatalf("GetIndexJob: ok=%v err=%v", ok, err)
	}
	if final.Status == IndexJobFailed {
		t.Fatal("cancel/publication race converted the job to failed")
	}
	if final.Status != IndexJobCanceled && final.Status != IndexJobSucceeded {
		t.Fatalf("final status = %q, want canceled or succeeded", final.Status)
	}
}

type incrementalCatalogSnapshot struct {
	fileCount int
	hash      string
	symbols   []model.SymbolRecord
}

func captureIncrementalCatalogSnapshot(t *testing.T, ctx context.Context, db *sql.DB) incrementalCatalogSnapshot {
	t.Helper()
	stats, err := WorkspaceStats(ctx, db)
	if err != nil {
		t.Fatalf("WorkspaceStats: %v", err)
	}
	var hash string
	if err := db.QueryRowContext(ctx, "SELECT content_hash FROM files WHERE file_path = ?", "src/main.go").Scan(&hash); err != nil {
		t.Fatalf("file hash: %v", err)
	}
	symbols, err := SymbolsByFile(ctx, db, "src/main.go", 100, 0)
	if err != nil {
		t.Fatalf("SymbolsByFile: %v", err)
	}
	return incrementalCatalogSnapshot{fileCount: stats.Files, hash: hash, symbols: symbols}
}

func TestIncrementalPublicationCanceledFencePreservesFilesAndSymbols(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	baselineSymbols := []model.SymbolRecord{{FilePath: "src/main.go", Name: "Before", Kind: "function", Language: "go"}}
	if err := replaceFileSymbols(ctx, db, "src/main.go", "repo", "repo", "go", "before", baselineSymbols); err != nil {
		t.Fatalf("replaceFileSymbols: %v", err)
	}
	job, err := CreateIndexJob(ctx, db, "incremental-cancel", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	fence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, job.JobID, 0, "indexing", fence); err != nil {
		t.Fatalf("MarkIndexJobRunning: %v", err)
	}
	before := captureIncrementalCatalogSnapshot(t, ctx, db)
	if _, err := RequestIndexJobCancelByID(ctx, db, job.JobID); err != nil {
		t.Fatalf("RequestIndexJobCancelByID: %v", err)
	}
	changes := []IncrementalFileChange{{FilePath: "src/main.go", RepoID: "repo", RepoName: "repo", Language: "go", ContentHash: "after", Symbols: []model.SymbolRecord{{FilePath: "src/main.go", Name: "After", Kind: "function", Language: "go"}}}}
	if err := PublishIncrementalGenerationForJobWithChanges(ctx, db, job.JobID, job.GenerationID, 1, 1, 0, fence, changes, nil); !errors.Is(err, ErrStaleIndexJobOwner) {
		t.Fatalf("canceled incremental publication error = %v, want ErrStaleIndexJobOwner", err)
	}
	after := captureIncrementalCatalogSnapshot(t, ctx, db)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("catalog drift after canceled incremental publication: before=%#v after=%#v", before, after)
	}
}

func TestIncrementalPublicationStaleFencePreservesFilesAndSymbols(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()
	baselineSymbols := []model.SymbolRecord{{FilePath: "src/main.go", Name: "Before", Kind: "function", Language: "go"}}
	if err := replaceFileSymbols(ctx, db, "src/main.go", "repo", "repo", "go", "before", baselineSymbols); err != nil {
		t.Fatalf("replaceFileSymbols: %v", err)
	}
	job, err := CreateIndexJob(ctx, db, "incremental-stale", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	fence := IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken}
	if err := MarkIndexJobRunning(ctx, db, job.JobID, 0, "indexing", fence); err != nil {
		t.Fatalf("MarkIndexJobRunning: %v", err)
	}
	before := captureIncrementalCatalogSnapshot(t, ctx, db)
	if err := MarkIndexJobSucceeded(ctx, db, job.JobID, 0, 0, 0, fence); err != nil {
		t.Fatalf("MarkIndexJobSucceeded: %v", err)
	}
	changes := []IncrementalFileChange{{FilePath: "src/main.go", RepoID: "repo", RepoName: "repo", Language: "go", ContentHash: "after", Symbols: []model.SymbolRecord{{FilePath: "src/main.go", Name: "After", Kind: "function", Language: "go"}}}}
	if err := PublishIncrementalGenerationForJobWithChanges(ctx, db, job.JobID, job.GenerationID, 1, 1, 0, fence, changes, nil); !errors.Is(err, ErrStaleIndexJobOwner) {
		t.Fatalf("stale incremental publication error = %v, want ErrStaleIndexJobOwner", err)
	}
	after := captureIncrementalCatalogSnapshot(t, ctx, db)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("catalog drift after stale incremental publication: before=%#v after=%#v", before, after)
	}
}

func TestForegroundIncrementalPublicationUpdatesFilesAndSymbolsTogether(t *testing.T) {
	db, _ := seedTestDB(t)
	ctx := context.Background()
	baseline := []model.SymbolRecord{{FilePath: "src/main.go", Name: "Before", Kind: "function", Language: "go"}}
	if err := replaceFileSymbols(ctx, db, "src/main.go", "repo", "repo", "go", "before", baseline); err != nil {
		t.Fatalf("replaceFileSymbols baseline: %v", err)
	}

	changes := []IncrementalFileChange{{
		FilePath: "src/main.go", RepoID: "repo", RepoName: "repo", Language: "go", ContentHash: "after",
		Symbols: []model.SymbolRecord{{FilePath: "src/main.go", Name: "After", Kind: "function", Language: "go"}},
	}}
	if err := PublishIncrementalGenerationWithChanges(ctx, db, "", 1, 1, 0, changes, nil); err != nil {
		t.Fatalf("PublishIncrementalGenerationWithChanges: %v", err)
	}

	var hash string
	if err := db.QueryRowContext(ctx, "SELECT content_hash FROM files WHERE file_path = ?", "src/main.go").Scan(&hash); err != nil {
		t.Fatalf("read file hash: %v", err)
	}
	if hash != "after" {
		t.Fatalf("content hash = %q, want after", hash)
	}
	symbols, err := SymbolsByFile(ctx, db, "src/main.go", 100, 0)
	if err != nil {
		t.Fatalf("SymbolsByFile: %v", err)
	}
	if len(symbols) != 1 || symbols[0].Name != "After" {
		t.Fatalf("symbols = %#v, want one After symbol", symbols)
	}
	state, err := GraphRuntimeState(ctx, db)
	if err != nil {
		t.Fatalf("GraphRuntimeState: %v", err)
	}
	if state != GraphRuntimeStale {
		t.Fatalf("graph runtime state = %q, want stale for no graph publication", state)
	}
}
