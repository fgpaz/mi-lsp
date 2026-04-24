package store

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCancelIndexJobForceTerminatesProcessAndMarksCanceled(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()

	job, err := CreateIndexJob(ctx, db, "test", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}

	cmd := startLongRunningProcess(t)
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})

	if err := MarkIndexJobRunning(ctx, db, job.JobID, cmd.Process.Pid, "indexing"); err != nil {
		t.Fatalf("MarkIndexJobRunning: %v", err)
	}

	canceled, err := CancelIndexJob(ctx, db, job.JobID, true)
	if err != nil {
		t.Fatalf("CancelIndexJob(force): %v", err)
	}
	if canceled.Status != IndexJobCanceled {
		t.Fatalf("job status = %q, want %q", canceled.Status, IndexJobCanceled)
	}

	_, _ = cmd.Process.Wait()
	deadline := time.Now().Add(2 * time.Second)
	for processExists(cmd.Process.Pid) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if processExists(cmd.Process.Pid) {
		t.Fatalf("expected pid %d to be terminated", cmd.Process.Pid)
	}
}

func TestMarkIndexJobProgressUpdatesCountersStageAndTimestamp(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()

	job, err := CreateIndexJob(ctx, db, "test", root, IndexModeCatalog, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}
	if err := MarkIndexJobRunning(ctx, db, job.JobID, 12345, "indexing"); err != nil {
		t.Fatalf("MarkIndexJobRunning: %v", err)
	}
	before, _, err := GetIndexJob(ctx, db, job.JobID)
	if err != nil {
		t.Fatalf("GetIndexJob(before): %v", err)
	}
	time.Sleep(time.Millisecond)

	if err := MarkIndexJobProgress(ctx, db, job.JobID, IndexJobProgress{
		CurrentStage: "catalog.read",
		CurrentPath:  "src/App.cs",
		Files:        7,
		Symbols:      11,
		Docs:         0,
		FilesTotal:   20,
	}); err != nil {
		t.Fatalf("MarkIndexJobProgress: %v", err)
	}

	after, _, err := GetIndexJob(ctx, db, job.JobID)
	if err != nil {
		t.Fatalf("GetIndexJob(after): %v", err)
	}
	if after.CurrentStage != "catalog.read" || after.CurrentPath != "src/App.cs" {
		t.Fatalf("progress location = %q/%q, want catalog.read/src/App.cs", after.CurrentStage, after.CurrentPath)
	}
	if after.Files != 7 || after.Symbols != 11 || after.FilesTotal != 20 {
		t.Fatalf("progress counters files=%d symbols=%d total=%d, want 7/11/20", after.Files, after.Symbols, after.FilesTotal)
	}
	if after.UpdatedAt == before.UpdatedAt {
		t.Fatalf("updated_at did not change: %s", after.UpdatedAt)
	}
}

func TestCancelIndexJobForceRemovesMatchingDeadPIDLock(t *testing.T) {
	db, root := seedTestDB(t)
	ctx := context.Background()

	job, err := CreateIndexJob(ctx, db, "test", root, IndexModeFull, false)
	if err != nil {
		t.Fatalf("CreateIndexJob: %v", err)
	}

	cmd := startLongRunningProcess(t)
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})

	if err := MarkIndexJobRunning(ctx, db, job.JobID, cmd.Process.Pid, "indexing"); err != nil {
		t.Fatalf("MarkIndexJobRunning: %v", err)
	}
	lockDir := filepath.Join(root, ".mi-lsp")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lockPath := filepath.Join(lockDir, "index.lock")
	body, err := json.Marshal(IndexLockInfo{PID: cmd.Process.Pid, Operation: "index.full", StartedAt: time.Now().UTC().Format(time.RFC3339)})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(lockPath, append(body, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := CancelIndexJob(ctx, db, job.JobID, true); err != nil {
		t.Fatalf("CancelIndexJob(force): %v", err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock path still exists or stat failed with non-not-exist error: %v", err)
	}
}

func startLongRunningProcess(t *testing.T) *exec.Cmd {
	t.Helper()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "ping", "-n", "30", "127.0.0.1")
	} else {
		cmd = exec.Command("sleep", "30")
	}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("start long running process: %v", err)
	}
	return cmd
}
