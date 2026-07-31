package cli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/service"
	"github.com/fgpaz/mi-lsp/internal/workspace"
	"github.com/spf13/cobra"
)

func TestRootCommandExposesProbeCommand(t *testing.T) {
	root := NewRootCommand()
	command, _, err := root.Find([]string{"probe"})
	if err != nil {
		t.Fatalf("Find probe: %v", err)
	}
	if command == nil || command.Name() != "probe" {
		t.Fatalf("command = %#v, want probe", command)
	}
	if command.Use != "probe [selector]" {
		t.Fatalf("Use = %q, want probe [selector]", command.Use)
	}
}

func TestWorkspaceStatusCLIExplicitDotAcceptsNestedGitWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	container := t.TempDir()
	parent := filepath.Join(container, "parent")
	runCLIGit(t, container, "init", "parent")
	runCLIGit(t, parent, "config", "user.email", "test@example.com")
	runCLIGit(t, parent, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(parent, "parent.go"), []byte("package parent\\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(parent): %v", err)
	}
	runCLIGit(t, parent, "add", ".")
	runCLIGit(t, parent, "commit", "-m", "parent")
	worktreeRoot := filepath.Join(parent, ".claude", "worktrees", "feature")
	if err := os.MkdirAll(filepath.Dir(worktreeRoot), 0o755); err != nil {
		t.Fatalf("MkdirAll(worktree parent): %v", err)
	}
	runCLIGit(t, parent, "worktree", "add", "--detach", worktreeRoot)
	t.Cleanup(func() {
		_ = exec.Command("git", "-C", parent, "worktree", "remove", "--force", worktreeRoot).Run()
		_ = exec.Command("git", "-C", parent, "worktree", "prune").Run()
	})
	if _, err := workspace.RegisterWorkspace("parent", model.WorkspaceRegistration{Name: "parent", Root: parent}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace("parent") })

	state := &rootState{
		format:       "json",
		workspace:    ".",
		clientName:   "codex",
		noDaemon:     true,
		telemetry:    &CLITelemetry{},
		retentionRun: true,
		appExecute: func(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
			return service.New(parent, nil).Execute(ctx, request)
		},
	}
	t.Chdir(worktreeRoot)
	command := &cobra.Command{}
	command.SetContext(context.Background())
	if err := state.executeOperation(command, "workspace.status", nil, false); err != nil {
		t.Fatalf("CLI workspace status: %v", err)
	}
}

func runCLIGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\\n%s", args, err, output)
	}
}

func TestRenderProbeEnvelopeHasContractFieldsAndProvenance(t *testing.T) {
	envelope := model.ProbeEnvelope{
		Schema:          "mi-lsp/workspace-probe/v1",
		Command:         "mi-lsp probe",
		ProtocolVersion: model.ProtocolVersion,
		OK:              true,
		SideEffects:     false,
		Backend:         "workspace-probe",
		Items: []model.ProbeReport{{
			Status:      model.ProbeStatusPartial,
			SideEffects: false,
			Provenance:  map[string]any{"version": map[string]any{"version": "(devel)"}},
		}},
	}
	raw, err := renderProbeEnvelope(envelope, "json")
	if err != nil {
		t.Fatalf("renderProbeEnvelope: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	for key, want := range map[string]any{
		"schema":           "mi-lsp/workspace-probe/v1",
		"command":          "mi-lsp probe",
		"protocol_version": model.ProtocolVersion,
		"side_effects":     false,
	} {
		if decoded[key] != want {
			t.Fatalf("%s = %#v, want %#v", key, decoded[key], want)
		}
	}
	items, ok := decoded["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %#v, want one item", decoded["items"])
	}
	item := items[0].(map[string]any)
	provenance := item["provenance"].(map[string]any)
	if _, ok := provenance["version"]; !ok {
		t.Fatalf("provenance = %#v, want version", provenance)
	}

	textRaw, err := renderProbeEnvelope(envelope, "text")
	if err != nil {
		t.Fatalf("renderProbeEnvelope(text): %v", err)
	}
	var textDecoded map[string]any
	if err := json.Unmarshal(textRaw, &textDecoded); err != nil {
		t.Fatalf("json.Unmarshal(text): %v", err)
	}
	for key, want := range map[string]any{
		"schema":           "mi-lsp/workspace-probe/v1",
		"command":          "mi-lsp probe",
		"protocol_version": model.ProtocolVersion,
		"side_effects":     false,
	} {
		if textDecoded[key] != want {
			t.Fatalf("text %s = %#v, want %#v", key, textDecoded[key], want)
		}
	}
}
