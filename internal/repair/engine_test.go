package repair

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/repair/actions"
	"github.com/fgpaz/mi-lsp/internal/runtimeinfo"
)

func TestReportJSONArraysAndNullTimes(t *testing.T) {
	r := Report{Schema: Schema, OK: true, Mode: "plan", Runtime: runtimeinfo.Location{}, Actions: []Action{makeAction("workspace", "skip", "WORKSPACE_REQUIRED", "", "")}, ReasonCodes: []string{}}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema":"mi-lsp-repair/v1","ok":true,"mode":"plan","runtime":{"executablePath":"","managedRoot":"","managedExecutablePath":"","receiptPath":""},"actions":[{"id":"workspace","status":"skip","reasonCode":"WORKSPACE_REQUIRED","detail":"","retryable":false,"nextAction":"","lastAttemptAt":null,"retryAfter":null}],"reasonCodes":[],"changed":false}`
	if string(data) != want {
		t.Fatalf("json = %s", data)
	}
}

func TestStateRejectsMalformedWithoutOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	original := []byte(`{"schema":"bad","attempts":[]}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadState(path); err == nil {
		t.Fatal("invalid state accepted")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(original) {
		t.Fatal("state modified")
	}
}

func TestApplyCooldownAndPersistBeforeEffect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	target := filepath.Join(t.TempDir(), "target")
	old := performAction
	defer func() { performAction = old }()
	calls := 0
	performAction = func(kind, target, root, mode string) (actions.Result, error) {
		calls++
		s, err := loadState(path)
		if err != nil || len(s.Attempts) != 1 || s.Attempts[0].Result != "pending" {
			t.Fatalf("pending not persisted: %+v %v", s, err)
		}
		return actions.Result{ReasonCode: "WORKER_INSTALL_FAILED"}, errors.New("failure")
	}
	a, _ := applyAction(path, "worker", target, Options{}, makeAction("worker", "planned", "WORKER_MISSING", "", ""))
	if a.Status != "failed" || calls != 1 {
		t.Fatalf("first attempt: %+v, calls %d", a, calls)
	}
	a, _ = applyAction(path, "worker", target, Options{}, makeAction("worker", "planned", "WORKER_MISSING", "", ""))
	if a.ReasonCode != "REPAIR_COOLDOWN" || calls != 1 || a.RetryAfter == nil || a.RetryAfter.Before(time.Now()) {
		t.Fatalf("cooldown: %+v, calls %d", a, calls)
	}
}

func TestPlanDoesNotApplyAndNoWorkspaceSkips(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	oldInspect, oldApply := inspectAction, performAction
	defer func() { inspectAction = oldInspect; performAction = oldApply }()
	inspectAction = func(kind, target, root, mode string) (actions.Result, error) {
		return actions.Result{Needed: true, ReasonCode: "CONFIG_MISSING"}, nil
	}
	performAction = func(kind, target, root, mode string) (actions.Result, error) {
		t.Fatal("plan applied")
		return actions.Result{}, nil
	}
	r := Run(Options{})
	if r.Mode != "plan" || r.Changed {
		t.Fatalf("plan: %+v", r)
	}
	for _, a := range r.Actions {
		if (a.ID == "workspace" || a.ID == "index") && a.Status != "skip" {
			t.Fatalf("workspace not skipped: %+v", a)
		}
	}
	p, _ := statePath()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("state created: %v", err)
	}
}

func TestMalformedStateBlocksActions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path, _ := statePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("not json"), 0600)
	r := Run(Options{Apply: true})
	if r.OK || !strings.Contains(strings.Join(r.ReasonCodes, ","), "REPAIR_STATE_INVALID") {
		t.Fatalf("report: %+v", r)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "not json" {
		t.Fatal("state overwritten")
	}
}
