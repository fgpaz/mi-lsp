package store

import (
	"context"
	"io"
	"os/exec"
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
