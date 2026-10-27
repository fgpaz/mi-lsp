package repair

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/repair/actions"
	"github.com/fgpaz/mi-lsp/internal/runtimeinfo"
)

func TestRemovedWorkerNeverApplied(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", home)
	t.Setenv("XDG_DATA_HOME", home)
	artifact := filepath.Join(home, "worker")
	if err := os.WriteFile(artifact, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runtimeinfo.Record("worker", artifact); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(artifact); err != nil {
		t.Fatal(err)
	}
	oldInspect, oldApply := inspectAction, performAction
	defer func() { inspectAction = oldInspect; performAction = oldApply }()
	inspectAction = func(kind, target, root, mode string) (actions.Result, error) { return actions.Result{}, nil }
	performAction = func(kind, target, root, mode string) (actions.Result, error) {
		if kind == "worker" {
			t.Fatal("removed worker applied")
		}
		return actions.Result{}, nil
	}
	r := Run(Options{Apply: true, Worker: true})
	for _, a := range r.Actions {
		if a.ID == "worker" {
			if a.Status != "blocked" || a.ReasonCode != "WORKER_REMOVED" || a.Retryable {
				t.Fatalf("worker: %+v", a)
			}
			return
		}
	}
	t.Fatal("missing worker action")
}
