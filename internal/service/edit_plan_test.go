package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestEditPlanDryRunDoesNotWrite(t *testing.T) {
	root, name := setupTestWorkspace(t)
	content := "package demo\n\nfunc Label() string { return \"old\" }\n"
	writeWorkspaceFile(t, root, "src/edit_plan.go", content)

	packet := testEditPlanPacket(t, "src/edit_plan.go", content, []model.EditPlanOperation{{
		ID:       "op-replace",
		Kind:     "replace_literal",
		TargetID: "target-main",
		Find:     "old",
		Replace:  "new",
	}})
	app := New(root, &fakeSemanticCaller{})
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.edit-plan",
		Context:   model.QueryOptions{Workspace: name},
		Payload:   map[string]any{"packet": packet},
	})
	if err != nil {
		t.Fatalf("nav.edit-plan dry-run: %v", err)
	}
	if env.Backend != "edit-plan" || env.Mode != "dry_run" {
		t.Fatalf("backend/mode = %s/%s, want edit-plan/dry_run", env.Backend, env.Mode)
	}
	results := env.Items.([]model.EditPlanResult)
	if len(results) != 1 {
		t.Fatalf("items = %#v, want one result", env.Items)
	}
	if !strings.Contains(results[0].Diff, `return "new"`) {
		t.Fatalf("diff = %q, want replacement", results[0].Diff)
	}
	if results[0].ApplyStatus.Applied {
		t.Fatalf("dry-run apply status = %#v, want not applied", results[0].ApplyStatus)
	}
	got, err := os.ReadFile(filepath.Join(root, "src", "edit_plan.go"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != content {
		t.Fatalf("dry-run wrote file: got %q want %q", got, content)
	}
}

func TestEditPlanApplyWritesOnlyWithExperimentalGate(t *testing.T) {
	root, name := setupTestWorkspace(t)
	content := "package demo\n\nfunc Label() string { return \"old\" }\n"
	writeWorkspaceFile(t, root, "src/edit_plan.go", content)
	initCleanGitWorkspace(t, root)

	packet := testEditPlanPacket(t, "src/edit_plan.go", content, []model.EditPlanOperation{{
		ID:       "op-replace",
		Kind:     "replace_literal",
		TargetID: "target-main",
		Find:     "old",
		Replace:  "new",
	}})
	app := New(root, &fakeSemanticCaller{})
	_, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.edit-plan",
		Context:   model.QueryOptions{Workspace: name},
		Payload: map[string]any{
			"packet": packet,
			"apply":  true,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "--experimental-apply") {
		t.Fatalf("apply without experimental err = %v, want --experimental-apply rejection", err)
	}

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.edit-plan",
		Context:   model.QueryOptions{Workspace: name},
		Payload: map[string]any{
			"packet":             packet,
			"apply":              true,
			"experimental_apply": true,
		},
	})
	if err != nil {
		t.Fatalf("nav.edit-plan apply: %v", err)
	}
	if env.Mode != "applied" {
		t.Fatalf("mode = %q, want applied", env.Mode)
	}
	got, err := os.ReadFile(filepath.Join(root, "src", "edit_plan.go"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.Contains(string(got), `return "new"`) {
		t.Fatalf("applied file = %q, want replacement", got)
	}
}

func TestEditPlanApplyRequiresCleanGit(t *testing.T) {
	root, name := setupTestWorkspace(t)
	content := "package demo\n\nfunc Label() string { return \"old\" }\n"
	writeWorkspaceFile(t, root, "src/edit_plan.go", content)
	initCleanGitWorkspace(t, root)
	writeWorkspaceFile(t, root, "src/dirty.txt", "dirty\n")

	packet := testEditPlanPacket(t, "src/edit_plan.go", content, []model.EditPlanOperation{{
		ID:       "op-replace",
		Kind:     "replace_literal",
		TargetID: "target-main",
		Find:     "old",
		Replace:  "new",
	}})
	_, err := New(root, &fakeSemanticCaller{}).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.edit-plan",
		Context:   model.QueryOptions{Workspace: name},
		Payload: map[string]any{
			"packet":             packet,
			"apply":              true,
			"experimental_apply": true,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "clean git workspace") {
		t.Fatalf("dirty apply err = %v, want clean git rejection", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "src", "edit_plan.go"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != content {
		t.Fatalf("dirty apply wrote file: got %q want %q", got, content)
	}
}

func TestEditPlanRejectsUnsafePackets(t *testing.T) {
	root, name := setupTestWorkspace(t)
	content := "alpha\nbeta\n"
	writeWorkspaceFile(t, root, "src/edit_plan.txt", content)
	writeWorkspaceFile(t, root, ".docs/wiki/_mi-lsp/read-model.toml", "version = 1\n")
	writeWorkspaceFile(t, root, "src/blob.bin", "\x00binary")

	baseOp := model.EditPlanOperation{
		ID:       "op",
		Kind:     "replace_literal",
		TargetID: "target-main",
		Find:     "alpha",
		Replace:  "omega",
	}
	cases := []struct {
		name       string
		path       string
		hash       string
		operations []model.EditPlanOperation
	}{
		{name: "hash mismatch", path: "src/edit_plan.txt", hash: "sha256:bad", operations: []model.EditPlanOperation{baseOp}},
		{name: "path traversal", path: "../escape.txt", hash: testHash(""), operations: []model.EditPlanOperation{baseOp}},
		{name: "read model denied", path: ".docs/wiki/_mi-lsp/read-model.toml", hash: testHash("version = 1\n"), operations: []model.EditPlanOperation{baseOp}},
		{name: "binary denied", path: "src/blob.bin", hash: testHash("\x00binary"), operations: []model.EditPlanOperation{baseOp}},
		{name: "regex needs limit", path: "src/edit_plan.txt", hash: testHash(content), operations: []model.EditPlanOperation{{
			ID:       "op",
			Kind:     "replace_regex_limited",
			TargetID: "target-main",
			Find:     "alpha",
			Replace:  "omega",
		}}},
		{name: "overlapping range ops", path: "src/edit_plan.txt", hash: testHash(content), operations: []model.EditPlanOperation{
			{ID: "op-a", Kind: "insert_before", TargetID: "target-main", Content: "before\n"},
			{ID: "op-b", Kind: "replace_range", TargetID: "target-main", Content: "after\n"},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			packet := testEditPlanPacketWithHash(t, tc.path, tc.hash, tc.operations)
			_, err := New(root, &fakeSemanticCaller{}).Execute(context.Background(), model.CommandRequest{
				Operation: "nav.edit-plan",
				Context:   model.QueryOptions{Workspace: name},
				Payload:   map[string]any{"packet": packet},
			})
			if err == nil {
				t.Fatalf("expected rejection")
			}
		})
	}
}

func testEditPlanPacket(t *testing.T, path string, content string, operations []model.EditPlanOperation) string {
	t.Helper()
	return testEditPlanPacketWithHash(t, path, testHash(content), operations)
}

func testEditPlanPacketWithHash(t *testing.T, path string, hash string, operations []model.EditPlanOperation) string {
	t.Helper()
	packet := model.EditPlanRequest{
		Version: model.EditPlanVersion,
		Intent:  "test edit-plan packet",
		Targets: []model.EditPlanTarget{{
			ID:           "target-main",
			Path:         path,
			Range:        model.EditPlanRange{StartLine: 1, EndLine: 0},
			ExpectedHash: hash,
		}},
		Operations: operations,
		Constraints: model.EditPlanConstraints{
			RequireCleanMatch: true,
			RequireEvidence:   true,
		},
	}
	data, err := json.Marshal(packet)
	if err != nil {
		t.Fatalf("marshal packet: %v", err)
	}
	return string(data)
}

func testHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func initCleanGitWorkspace(t *testing.T, root string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for apply tests")
	}
	runEditPlanGit(t, root, "init")
	runEditPlanGit(t, root, "config", "user.email", "mi-lsp-test@example.invalid")
	runEditPlanGit(t, root, "config", "user.name", "mi-lsp Test")
	runEditPlanGit(t, root, "add", ".")
	runEditPlanGit(t, root, "commit", "-m", "initial")
}

func runEditPlanGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}
