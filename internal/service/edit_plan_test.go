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

func TestEditPlanV2GoASTOperationsDryRunDoesNotWrite(t *testing.T) {
	root, name := setupTestWorkspace(t)
	content := `package demo

import "fmt"

func Anchor() string {
	return "anchor"
}

func Label() string {
	return "old"
}

func Greeting() string {
	return fmt.Sprint("hello")
}
`
	path := "src/edit_plan_v2.go"
	writeWorkspaceFile(t, root, path, content)
	packet := testEditPlanV2Packet(t, []model.EditPlanTarget{
		testEditPlanV2Target(path, "target-file", content, nil),
		testEditPlanV2Target(path, "target-anchor", content, &model.EditPlanSymbol{Name: "Anchor", Kind: "function"}),
		testEditPlanV2Target(path, "target-label", content, &model.EditPlanSymbol{Name: "Label", Kind: "function"}),
		testEditPlanV2Target(path, "target-greeting", content, &model.EditPlanSymbol{Name: "Greeting", Kind: "function"}),
	}, []model.EditPlanOperation{
		{ID: "ensure-strings", Kind: "ensure_go_import", TargetID: "target-file", ImportPath: "strings"},
		{ID: "replace-greeting", Kind: "replace_go_function", TargetID: "target-greeting", Content: `func Greeting() string { return strings.ToUpper("hello") }`},
		{ID: "replace-label-body", Kind: "replace_go_function_body", TargetID: "target-label", Content: `return strings.TrimSpace(" new ")`},
		{ID: "insert-added", Kind: "insert_go_function_after", TargetID: "target-anchor", Content: `func Added() string { return strings.ToLower("OK") }`},
		{ID: "remove-fmt", Kind: "remove_go_import", TargetID: "target-file", ImportPath: "fmt"},
	})

	env, err := New(root, &fakeSemanticCaller{}).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.edit-plan",
		Context:   model.QueryOptions{Workspace: name},
		Payload:   map[string]any{"packet": packet, "strict": true},
	})
	if err != nil {
		t.Fatalf("nav.edit-plan v2 dry-run: %v", err)
	}
	if env.Mode != "dry_run" || env.Backend != "edit-plan" {
		t.Fatalf("backend/mode = %s/%s, want edit-plan/dry_run", env.Backend, env.Mode)
	}
	results := env.Items.([]model.EditPlanResult)
	diff := results[0].Diff
	for _, want := range []string{`"strings"`, "func Added() string", "strings.TrimSpace", "strings.ToUpper"} {
		if !strings.Contains(diff, want) {
			t.Fatalf("diff missing %q:\n%s", want, diff)
		}
	}
	got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != content {
		t.Fatalf("dry-run wrote file: got %q want %q", got, content)
	}
}

func TestEditPlanV2GoASTApplyWritesExpectedFile(t *testing.T) {
	root, name := setupTestWorkspace(t)
	writeWorkspaceFile(t, root, "go.mod", "module example.com/editplanv2\n\ngo 1.24\n")
	content := `package demo

func Label() string {
	return "old"
}
`
	path := "src/edit_plan_v2.go"
	writeWorkspaceFile(t, root, path, content)
	initCleanGitWorkspace(t, root)
	packet := testEditPlanV2Packet(t, []model.EditPlanTarget{
		testEditPlanV2Target(path, "target-label", content, &model.EditPlanSymbol{Name: "Label", Kind: "function"}),
	}, []model.EditPlanOperation{
		{ID: "replace-label-body", Kind: "replace_go_function_body", TargetID: "target-label", Content: `return "new"`},
	})

	env, err := New(root, &fakeSemanticCaller{}).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.edit-plan",
		Context:   model.QueryOptions{Workspace: name},
		Payload: map[string]any{
			"packet":             packet,
			"strict":             true,
			"apply":              true,
			"experimental_apply": true,
		},
	})
	if err != nil {
		t.Fatalf("nav.edit-plan v2 apply: %v", err)
	}
	if env.Mode != "applied" {
		t.Fatalf("mode = %q, want applied", env.Mode)
	}
	got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.Contains(string(got), `return "new"`) {
		t.Fatalf("applied file = %q, want new return", got)
	}
	status := runEditPlanGitOutput(t, root, "status", "--short")
	if strings.TrimSpace(status) != "M src/edit_plan_v2.go" {
		t.Fatalf("git status = %q, want only modified Go file", status)
	}
}

func TestEditPlanV2ApplyRequiresCleanGit(t *testing.T) {
	root, name := setupTestWorkspace(t)
	content := "package demo\n\nfunc Label() string { return \"old\" }\n"
	path := "src/edit_plan_v2.go"
	writeWorkspaceFile(t, root, path, content)
	initCleanGitWorkspace(t, root)
	writeWorkspaceFile(t, root, "src/dirty.txt", "dirty\n")

	packet := testEditPlanV2Packet(t, []model.EditPlanTarget{
		testEditPlanV2Target(path, "target-label", content, &model.EditPlanSymbol{Name: "Label", Kind: "function"}),
	}, []model.EditPlanOperation{{
		ID:       "replace-label-body",
		Kind:     "replace_go_function_body",
		TargetID: "target-label",
		Content:  `return "new"`,
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
		t.Fatalf("dirty v2 apply err = %v, want clean git rejection", err)
	}
	got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != content {
		t.Fatalf("dirty v2 apply wrote file: got %q want %q", got, content)
	}
}

func TestEditPlanV2RejectsUnsupportedLanguages(t *testing.T) {
	root, name := setupTestWorkspace(t)
	cases := []struct {
		name     string
		path     string
		language string
		content  string
	}{
		{name: "csharp", path: "src/Demo.cs", language: "csharp", content: "class Demo { string Label() => \"old\"; }\n"},
		{name: "typescript", path: "src/demo.ts", language: "typescript", content: "export function label() { return 'old'; }\n"},
		{name: "python", path: "src/demo.py", language: "python", content: "def label():\n    return 'old'\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writeWorkspaceFile(t, root, tc.path, tc.content)
			target := testEditPlanV2Target(tc.path, "target-main", tc.content, &model.EditPlanSymbol{Name: "Label", Kind: "function"})
			target.Language = tc.language
			packet := testEditPlanV2Packet(t, []model.EditPlanTarget{target}, []model.EditPlanOperation{{
				ID:       "op",
				Kind:     "replace_go_function",
				TargetID: "target-main",
				Content:  `func Label() string { return "new" }`,
			}})
			_, err := New(root, &fakeSemanticCaller{}).Execute(context.Background(), model.CommandRequest{
				Operation: "nav.edit-plan",
				Context:   model.QueryOptions{Workspace: name},
				Payload:   map[string]any{"packet": packet},
			})
			if err == nil || !strings.Contains(err.Error(), "language_not_supported") {
				t.Fatalf("err = %v, want language_not_supported", err)
			}
		})
	}
}

func TestEditPlanV2RejectsMissingAmbiguousAndInvalidGo(t *testing.T) {
	root, name := setupTestWorkspace(t)
	content := `package demo

type A struct{}
type B struct{}

func (A) Reset() {}
func (B) Reset() {}
`
	path := "src/ambiguous.go"
	writeWorkspaceFile(t, root, path, content)
	cases := []struct {
		name   string
		symbol model.EditPlanSymbol
		op     model.EditPlanOperation
		want   string
	}{
		{
			name:   "missing symbol",
			symbol: model.EditPlanSymbol{Name: "Missing", Kind: "function"},
			op:     model.EditPlanOperation{ID: "op", Kind: "replace_go_function_body", TargetID: "target-main", Content: `return`},
			want:   "not found",
		},
		{
			name:   "ambiguous method",
			symbol: model.EditPlanSymbol{Name: "Reset", Kind: "method"},
			op:     model.EditPlanOperation{ID: "op", Kind: "replace_go_function_body", TargetID: "target-main", Content: `_ = 1`},
			want:   "ambiguous",
		},
		{
			name:   "invalid replacement",
			symbol: model.EditPlanSymbol{Name: "Reset", Kind: "method", Receiver: "A"},
			op:     model.EditPlanOperation{ID: "op", Kind: "replace_go_function", TargetID: "target-main", Content: `func Broken(`},
			want:   "invalid Go",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := testEditPlanV2Target(path, "target-main", content, &tc.symbol)
			packet := testEditPlanV2Packet(t, []model.EditPlanTarget{target}, []model.EditPlanOperation{tc.op})
			_, err := New(root, &fakeSemanticCaller{}).Execute(context.Background(), model.CommandRequest{
				Operation: "nav.edit-plan",
				Context:   model.QueryOptions{Workspace: name},
				Payload:   map[string]any{"packet": packet},
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestEditPlanV1TextualCompatibilityForNonGoFixtures(t *testing.T) {
	root, name := setupTestWorkspace(t)
	cases := []struct {
		name    string
		path    string
		content string
		find    string
		replace string
	}{
		{name: "csharp", path: "src/Demo.cs", content: "class Demo { string Label() => \"old\"; }\n", find: "old", replace: "new"},
		{name: "typescript", path: "src/demo.ts", content: "export function label() { return 'old'; }\n", find: "old", replace: "new"},
		{name: "python", path: "src/demo.py", content: "def label():\n    return 'old'\n", find: "old", replace: "new"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writeWorkspaceFile(t, root, tc.path, tc.content)
			packet := testEditPlanPacket(t, tc.path, tc.content, []model.EditPlanOperation{{
				ID:              "op",
				Kind:            "replace_literal",
				TargetID:        "target-main",
				Find:            tc.find,
				Replace:         tc.replace,
				MaxReplacements: 1,
			}})
			env, err := New(root, &fakeSemanticCaller{}).Execute(context.Background(), model.CommandRequest{
				Operation: "nav.edit-plan",
				Context:   model.QueryOptions{Workspace: name},
				Payload:   map[string]any{"packet": packet},
			})
			if err != nil {
				t.Fatalf("nav.edit-plan v1 dry-run: %v", err)
			}
			results := env.Items.([]model.EditPlanResult)
			if !strings.Contains(results[0].Diff, tc.replace) {
				t.Fatalf("diff = %q, want %q", results[0].Diff, tc.replace)
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

func testEditPlanV2Target(path string, id string, content string, symbol *model.EditPlanSymbol) model.EditPlanTarget {
	return model.EditPlanTarget{
		ID:           id,
		Path:         path,
		Language:     "go",
		ExpectedHash: testHash(content),
		Symbol:       symbol,
	}
}

func testEditPlanV2Packet(t *testing.T, targets []model.EditPlanTarget, operations []model.EditPlanOperation) string {
	t.Helper()
	packet := model.EditPlanRequest{
		Version:    model.EditPlanVersionV2,
		Intent:     "test edit-plan-v2 packet",
		Targets:    targets,
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

func runEditPlanGitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}
