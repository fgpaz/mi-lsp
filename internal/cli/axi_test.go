package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestEffectiveAXI_RootDefaultsOn(t *testing.T) {
	state := &rootState{}
	cmd := testAXICommand()

	if !state.effectiveAXI(cmd, "root.home", nil) {
		t.Fatalf("expected root.home to default to AXI")
	}
}

func TestEffectiveAXI_ClassicDisablesDefaultSurface(t *testing.T) {
	state := &rootState{classic: true}
	cmd := testAXICommand()
	if err := cmd.Flags().Set("classic", "true"); err != nil {
		t.Fatalf("set classic: %v", err)
	}

	if state.effectiveAXI(cmd, "nav.search", map[string]any{"pattern": "AXI"}) {
		t.Fatalf("expected --classic to disable AXI on default surface")
	}
}

func TestEffectiveAXI_WorkspaceMapStaysClassicWithoutForce(t *testing.T) {
	state := &rootState{}
	cmd := testAXICommand()

	if state.effectiveAXI(cmd, "nav.workspace-map", nil) {
		t.Fatalf("expected nav.workspace-map to stay classic without AXI force")
	}
}

func TestEffectiveAXI_SessionForceEnablesClassicSurface(t *testing.T) {
	state := &rootState{axi: true}
	cmd := testAXICommand()

	if !state.effectiveAXI(cmd, "nav.workspace-map", nil) {
		t.Fatalf("expected session AXI to force nav.workspace-map")
	}
}

func TestEffectiveAXI_AskOrientationDefaultsOn(t *testing.T) {
	state := &rootState{}
	cmd := testAXICommand()

	if !state.effectiveAXI(cmd, "nav.ask", map[string]any{"question": "how is this workspace organized?"}) {
		t.Fatalf("expected orientation ask to default to AXI")
	}
}

func TestEffectiveAXI_PackDefaultsOn(t *testing.T) {
	state := &rootState{}
	cmd := testAXICommand()

	if !state.effectiveAXI(cmd, "nav.pack", map[string]any{"task": "understand login flow"}) {
		t.Fatalf("expected nav.pack to default to AXI")
	}
}

func TestEffectiveAXI_AskImplementationStaysClassic(t *testing.T) {
	state := &rootState{}
	cmd := testAXICommand()

	if state.effectiveAXI(cmd, "nav.ask", map[string]any{"question": "how is AXI mode implemented?"}) {
		t.Fatalf("expected implementation ask to stay classic")
	}
}

func TestEffectiveFormat_DefaultSearchUsesTOON(t *testing.T) {
	state := &rootState{format: "compact"}
	cmd := testAXICommand()
	axiEnabled := state.effectiveAXI(cmd, "nav.search", map[string]any{"pattern": "AXI"})

	if got := state.effectiveFormat(cmd, "nav.search", map[string]any{"pattern": "AXI"}, axiEnabled); got != "toon" {
		t.Fatalf("effectiveFormat() = %q, want toon", got)
	}
}

func TestEffectiveFormat_ExplicitFormatWinsInAXIMode(t *testing.T) {
	state := &rootState{format: "yaml"}
	cmd := testAXICommand()
	if err := cmd.Flags().Set("format", "yaml"); err != nil {
		t.Fatalf("set format: %v", err)
	}
	axiEnabled := state.effectiveAXI(cmd, "nav.search", map[string]any{"pattern": "AXI"})

	if got := state.effectiveFormat(cmd, "nav.search", map[string]any{"pattern": "AXI"}, axiEnabled); got != "yaml" {
		t.Fatalf("effectiveFormat() = %q, want yaml", got)
	}
}

func TestEffectiveFormat_ClassicSearchKeepsCompact(t *testing.T) {
	state := &rootState{format: "compact", classic: true}
	cmd := testAXICommand()
	if err := cmd.Flags().Set("classic", "true"); err != nil {
		t.Fatalf("set classic: %v", err)
	}
	axiEnabled := state.effectiveAXI(cmd, "nav.search", map[string]any{"pattern": "AXI"})

	if got := state.effectiveFormat(cmd, "nav.search", map[string]any{"pattern": "AXI"}, axiEnabled); got != "compact" {
		t.Fatalf("effectiveFormat() = %q, want compact", got)
	}
}

func TestEffectiveMaxItems_DefaultSearchNarrows(t *testing.T) {
	state := &rootState{maxItems: 50}
	cmd := testAXICommand()
	axiEnabled := state.effectiveAXI(cmd, "nav.search", map[string]any{"pattern": "AXI"})

	if got := state.effectiveMaxItems(cmd, "nav.search", axiEnabled, false); got != 5 {
		t.Fatalf("effectiveMaxItems(nav.search) = %d, want 5", got)
	}
}

func TestEffectiveMaxItems_ClassicSearchKeepsConfiguredValue(t *testing.T) {
	state := &rootState{maxItems: 50, classic: true}
	cmd := testAXICommand()
	if err := cmd.Flags().Set("classic", "true"); err != nil {
		t.Fatalf("set classic: %v", err)
	}
	axiEnabled := state.effectiveAXI(cmd, "nav.search", map[string]any{"pattern": "AXI"})

	if got := state.effectiveMaxItems(cmd, "nav.search", axiEnabled, false); got != 50 {
		t.Fatalf("effectiveMaxItems(nav.search) = %d, want 50", got)
	}
}

func TestBuildAXIHomeEnvelopeWithoutWorkspaceShowsBootstrapGuidance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cwd := t.TempDir()
	t.Chdir(cwd)

	state := &rootState{repoRoot: cwd}
	cmd := testAXICommand()
	cmd.SetContext(context.Background())

	env, err := state.buildAXIHomeEnvelope(cmd)
	if err != nil {
		t.Fatalf("buildAXIHomeEnvelope: %v", err)
	}
	items := env.Items.([]map[string]any)
	nextSteps := items[0]["next_steps"].([]string)
	if len(nextSteps) == 0 || nextSteps[0] != "mi-lsp init ." {
		t.Fatalf("expected bootstrap next steps without forced --axi, got %#v", nextSteps)
	}
}

func TestBuildAXIHomeEnvelopeUsesRegisteredWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "App.csproj"), []byte("<Project Sdk=\"Microsoft.NET.Sdk\"></Project>"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	registration, project, err := workspace.DetectWorkspaceLayout(root, "axi-home")
	if err != nil {
		t.Fatalf("DetectWorkspaceLayout: %v", err)
	}
	registration.Name = "axi-home"
	if _, err := workspace.RegisterWorkspace("axi-home", registration); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace("axi-home") })
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	t.Chdir(root)

	state := &rootState{repoRoot: root}
	cmd := testAXICommand()
	cmd.SetContext(context.Background())

	env, err := state.buildAXIHomeEnvelope(cmd)
	if err != nil {
		t.Fatalf("buildAXIHomeEnvelope: %v", err)
	}
	items := env.Items.([]map[string]any)
	if items[0]["workspace"] != "axi-home" {
		t.Fatalf("workspace = %#v, want axi-home", items[0]["workspace"])
	}
	nextSteps := items[0]["next_steps"].([]string)
	if len(nextSteps) < 4 {
		t.Fatalf("expected four next steps, got %#v", nextSteps)
	}
	if nextSteps[0] != "mi-lsp workspace status axi-home" {
		t.Fatalf("expected default AXI workspace status step, got %q", nextSteps[0])
	}
	if nextSteps[1] != "mi-lsp nav governance --workspace axi-home --format toon" {
		t.Fatalf("expected governance diagnostic step, got %q", nextSteps[1])
	}
	if nextSteps[2] != "mi-lsp nav ask \"how is this workspace organized?\" --workspace axi-home" {
		t.Fatalf("expected default AXI ask step, got %q", nextSteps[2])
	}
	if nextSteps[3] != "mi-lsp nav workspace-map --workspace axi-home --axi --full" {
		t.Fatalf("expected explicit AXI workspace-map step, got %q", nextSteps[3])
	}
}

func TestRootRejectsAxiAndClassicTogether(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"--axi", "--classic"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--axi and --classic cannot be used together") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func testAXICommand() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("format", "compact", "")
	cmd.Flags().Int("max-items", 50, "")
	cmd.Flags().Bool("axi", false, "")
	cmd.Flags().Bool("classic", false, "")
	return cmd
}
