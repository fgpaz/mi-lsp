package repair

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/worker"
)

func TestWorkerTargetReadOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	got, err := targetPath("worker", "")
	if err != nil {
		t.Fatal(err)
	}
	want, err := canonicalTarget(filepath.Join(home, ".mi-lsp", "workers", worker.ResolveRID()))
	if err != nil || got != want {
		t.Fatalf("target %q want %q: %v", got, want, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".mi-lsp")); !os.IsNotExist(err) {
		t.Fatalf("plan created directories: %v", err)
	}
}

func TestTargetSymlinkAndCaseComparison(t *testing.T) {
	root := t.TempDir()
	actual := filepath.Join(root, "real")
	if err := os.Mkdir(actual, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(actual, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	s := stateFile{Schema: stateSchema, Attempts: []attempt{{ID: "index", Target: alias, LastAttemptAt: time.Now().UTC(), Result: "failed"}}}
	if findAttempt(s, "index", actual) == nil {
		t.Fatal("alias bypassed cooldown")
	}
	if runtime.GOOS == "windows" && findAttempt(s, "index", strings.ToUpper(actual)) == nil {
		t.Fatal("case bypassed cooldown")
	}
}

func TestInvalidStateCannotBeOverwritten(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	cases := []attempt{
		{ID: "other", Target: target, LastAttemptAt: time.Now().UTC(), Result: "failed"},
		{ID: "worker", Target: "relative", LastAttemptAt: time.Now().UTC(), Result: "failed"},
		{ID: "worker", Target: target, LastAttemptAt: time.Now().UTC().Add(10 * time.Minute), Result: "failed"},
		{ID: "worker", Target: target, LastAttemptAt: time.Now().In(time.FixedZone("offset", 3600)), Result: "failed"},
	}
	for _, entry := range cases {
		t.Run(entry.ID+entry.Target+entry.LastAttemptAt.String(), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			data, err := json.Marshal(stateFile{Schema: stateSchema, Attempts: []attempt{entry}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			a, _ := applyAction(path, "worker", target, Options{}, makeAction("worker", "planned", "WORKER_MISSING", "", ""))
			if a.Status != "blocked" || a.ReasonCode != "REPAIR_STATE_INVALID" {
				t.Fatalf("action: %+v", a)
			}
			after, _ := os.ReadFile(path)
			if string(after) != string(data) {
				t.Fatal("invalid state overwritten")
			}
		})
	}
}

func TestPlanTargetsDoNotCreateStateOrRegistry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, kind := range []string{"config", "workspace", "index", "worker"} {
		selector := filepath.Join(home, "project")
		if _, err := targetPath(kind, selector); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".mi-lsp")); !os.IsNotExist(err) {
		t.Fatalf("targets created directories: %v", err)
	}
}
