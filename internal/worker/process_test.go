package worker

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestClientStartRetriesNextLaunchSpecWhenPrimaryLaunchFails(t *testing.T) {
	workDir := t.TempDir()
	fallbackCommand, fallbackArgs := writeLongRunningCommand(t, workDir)

	client := &Client{
		workspace: model.WorkspaceRegistration{Name: "test", Root: workDir},
		specs: []LaunchSpec{
			{
				Source:        "installed",
				CandidatePath: filepath.Join(workDir, "missing-worker.exe"),
				Command:       filepath.Join(workDir, "missing-worker.exe"),
				WorkDir:       workDir,
			},
			{
				Source:        "bundle",
				CandidatePath: fallbackCommand,
				Command:       fallbackCommand,
				Args:          fallbackArgs,
				WorkDir:       workDir,
			},
		},
	}

	if err := client.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer client.Close()

	if client.cmd == nil || client.cmd.Process == nil {
		t.Fatal("expected fallback process to be started")
	}
	if client.spec.Command != fallbackCommand {
		t.Fatalf("selected command = %q, want %q", client.spec.Command, fallbackCommand)
	}
	if client.spec.Source != "bundle" {
		t.Fatalf("selected source = %q, want bundle", client.spec.Source)
	}
}

func writeLongRunningCommand(t *testing.T, dir string) (string, []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		scriptPath := filepath.Join(dir, "sleepy.cmd")
		body := "@echo off\r\nping -n 30 127.0.0.1 > nul\r\n"
		if err := os.WriteFile(scriptPath, []byte(body), 0o644); err != nil {
			t.Fatalf("write sleepy.cmd: %v", err)
		}
		return "cmd", []string{"/c", scriptPath}
	}

	scriptPath := filepath.Join(dir, "sleepy.sh")
	body := "#!/bin/sh\nsleep 30\n"
	if err := os.WriteFile(scriptPath, []byte(body), 0o755); err != nil {
		t.Fatalf("write sleepy.sh: %v", err)
	}
	return scriptPath, nil
}
