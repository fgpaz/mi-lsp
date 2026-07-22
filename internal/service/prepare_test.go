package service

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestNavPrepareEvidenceIsReadOnlyAndBoundsAllowedPaths(t *testing.T) {
	root, alias := setupTestWorkspace(t)
	writeWorkspaceFile(t, root, filepath.Join(".docs", "wiki", "00_gobierno_documental.md"), "governance")
	app := New(root, nil)
	before, err := os.ReadFile(filepath.Join(root, "src/Hello.cs"))
	if err != nil {
		t.Fatal(err)
	}
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.prepare",
		Context:   model.QueryOptions{Workspace: alias},
		Payload: map[string]any{
			"task":           "prepare a bounded query",
			"affected_paths": []string{"src/Hello.cs", "src/Hello.cs"},
		},
	})
	if err != nil {
		t.Fatalf("nav.prepare: %v", err)
	}
	if len(env.Items.([]model.SemanticPreparationEvidence)) != 1 {
		t.Fatalf("unexpected envelope: %#v", env)
	}
	item := env.Items.([]model.SemanticPreparationEvidence)[0]
	if item.Schema != semanticPreparationSchema || len(item.AllowedPaths) != 1 || item.AllowedPaths[0] != "src/Hello.cs" {
		t.Fatalf("unexpected evidence: %#v", item)
	}
	if item.TaskDigest == "" || item.GovernanceDigest == "" || item.IndexGeneration == "" || item.TotalMS < 0 {
		t.Fatalf("missing identity/timing evidence: %#v", item)
	}
	after, err := os.ReadFile(filepath.Join(root, "src/Hello.cs"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("nav.prepare mutated workspace bytes")
	}
}

func TestNavPrepareRejectsUnsafeAffectedPaths(t *testing.T) {
	root, alias := setupTestWorkspace(t)
	writeWorkspaceFile(t, root, filepath.Join(".docs", "wiki", "00_gobierno_documental.md"), "governance")
	outside := filepath.Join(filepath.Dir(root), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outside)
	cases := []string{"../outside.txt", root, "bad\nname.go"}
	for _, path := range cases {
		env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{Operation: "nav.prepare", Context: model.QueryOptions{Workspace: alias}, Payload: map[string]any{"task": "validate path", "affected_paths": []string{path}}})
		if err != nil {
			t.Fatalf("path %q: %v", path, err)
		}
		if env.Ok || env.Error == nil || env.Error.Code != "affected_path_invalid" {
			t.Fatalf("path %q accepted: %#v", path, env)
		}
		if len(env.Items.([]model.SemanticPreparationEvidence)) != 1 || env.Stats.Ms < 0 {
			t.Fatalf("path %q missing failure timing: %#v", path, env)
		}
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(root, "src", "escape-link")
		if err := os.Symlink(filepath.Dir(root), link); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		defer os.Remove(link)
		env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{Operation: "nav.prepare", Context: model.QueryOptions{Workspace: alias}, Payload: map[string]any{"task": "validate symlink", "affected_paths": []string{"src/escape-link/outside.txt"}}})
		if err != nil {
			t.Fatal(err)
		}
		if env.Ok || env.Error == nil || env.Error.Code != "affected_path_invalid" {
			t.Fatalf("symlink escape accepted: %#v", env)
		}
	}
}

func TestPreparationGovernanceDigestIsEmptyWhenFilesAreUnreadable(t *testing.T) {
	root := t.TempDir()
	if got := PreparationGovernanceDigest(root); got != "" {
		t.Fatalf("digest = %q, want empty when no governance files are readable", got)
	}
}

func TestNavPrepareValidPlanReportsExactAllowedPathsAndTimings(t *testing.T) {
	root, alias := setupTestWorkspace(t)
	writeWorkspaceFile(t, root, filepath.Join(".docs", "wiki", "00_gobierno_documental.md"), "governance")
	plan := `{"version":"edit-plan-v1","intent":"bounded","targets":[{"id":"hello","path":"src/Hello.cs","range":{"start_line":1,"end_line":1}}],"operations":[{"id":"op","kind":"replace_literal","target_id":"hello","find":"namespace","replace":"namespace"}]}`
	env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{Operation: "nav.prepare", Context: model.QueryOptions{Workspace: alias}, Payload: map[string]any{"task": "validate plan", "plan": plan}})
	if err != nil {
		t.Fatalf("nav.prepare: %v", err)
	}
	if env.Error != nil {
		t.Fatalf("valid plan failed: %#v", env)
	}
	item := env.Items.([]model.SemanticPreparationEvidence)[0]
	if len(item.AllowedPaths) != 1 || item.AllowedPaths[0] != "src/Hello.cs" || item.PlanDigest == "" {
		t.Fatalf("evidence = %#v", item)
	}
	for _, key := range []string{"governance", "index_generation", "route", "pack", "allowed_paths"} {
		if _, ok := item.Timings[key]; !ok {
			t.Fatalf("timings missing %q: %#v", key, item.Timings)
		}
	}
}

func TestNavPrepareStableFailureCodes(t *testing.T) {
	root, alias := setupTestWorkspace(t)
	writeWorkspaceFile(t, root, filepath.Join(".docs", "wiki", "00_gobierno_documental.md"), "governance")
	cases := []struct{ name, task, plan, want string }{
		{name: "task required", plan: "", want: "task_required"},
		{name: "invalid plan", task: "validate", plan: "{}", want: "plan_invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{}
			if tc.task != "" {
				payload["task"] = tc.task
			}
			if tc.plan != "" {
				payload["plan"] = tc.plan
			}
			env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{Operation: "nav.prepare", Context: model.QueryOptions{Workspace: alias}, Payload: payload})
			if err != nil {
				t.Fatal(err)
			}
			if env.Error == nil || env.Error.Code != tc.want || env.Stats.Ms < 0 {
				t.Fatalf("env = %#v", env)
			}
		})
	}
}

func TestPreparationCacheIdentityPropagatesGenerationError(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".mi-lsp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".mi-lsp", "index.db"), []byte("not sqlite"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := PreparationCacheIdentity(root)
	if err == nil {
		t.Fatal("expected generation read error")
	}
}

func TestNavPrepareWithoutTaskSpecificPathsFailsClosed(t *testing.T) {
	root, alias := setupTestWorkspace(t)
	writeWorkspaceFile(t, root, filepath.Join(".docs", "wiki", "00_gobierno_documental.md"), "governance")
	env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.prepare",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"task": "prepare without paths"},
	})
	if err != nil {
		t.Fatalf("nav.prepare: %v", err)
	}
	items := env.Items.([]model.SemanticPreparationEvidence)
	if len(items) != 1 || len(items[0].AllowedPaths) != 0 {
		t.Fatalf("expected empty allowed_paths: %#v", items)
	}
	if !containsWarning(env.Warnings, "no task-specific affected paths") {
		t.Fatalf("expected fail-closed warning: %#v", env.Warnings)
	}
}
