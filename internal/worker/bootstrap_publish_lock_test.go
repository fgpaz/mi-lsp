package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/runtimeinfo"
)

func TestPublishWorkerRejectsConcurrentWriterWithoutMutation(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "installed")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	previous := filepath.Join(target, workerBinaryName())
	if err := os.WriteFile(previous, []byte("previous"), 0600); err != nil {
		t.Fatal(err)
	}
	err := runtimeinfo.WithFileLock(target, func() error {
		path, err := publishWorkerDistribution(filepath.Join(parent, "source"), target, workerBinaryName())
		if err == nil || path != "" {
			t.Fatalf("concurrent writer: path=%q err=%v", path, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(previous)
	if err != nil || string(body) != "previous" {
		t.Fatalf("previous worker changed: %q %v", body, err)
	}
}
