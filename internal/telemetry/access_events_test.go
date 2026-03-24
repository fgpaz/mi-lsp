package telemetry

import (
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestResolveWorkspaceIdentity_PrefersRootAndPreservesInput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := t.TempDir()
	if _, err := workspace.RegisterWorkspace("multi-tedi", model.WorkspaceRegistration{Root: root}); err != nil {
		t.Fatalf("RegisterWorkspace(alias): %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace("multi-tedi") })

	identity := ResolveWorkspaceIdentity(root)
	if identity.Input != root {
		t.Fatalf("Input = %q, want %q", identity.Input, root)
	}
	if identity.Root != root {
		t.Fatalf("Root = %q, want %q", identity.Root, root)
	}
	if identity.Alias != "multi-tedi" {
		t.Fatalf("Alias = %q, want multi-tedi", identity.Alias)
	}
	if identity.Display != "multi-tedi" {
		t.Fatalf("Display = %q, want multi-tedi", identity.Display)
	}
}

func TestClassifyErrorInfo_DetectsGlobalJSONMismatch(t *testing.T) {
	info := ClassifyErrorInfo("roslyn", strings.Join([]string{
		"A compatible .NET SDK was not found.",
		"Requested SDK version: 10.0.201",
		"global.json file: C:\\repos\\mios\\gastos\\backend\\global.json",
	}, "\n"), nil)

	if info.Kind != "sdk" {
		t.Fatalf("Kind = %q, want sdk", info.Kind)
	}
	if info.Code != "dotnet_global_json_mismatch" {
		t.Fatalf("Code = %q, want dotnet_global_json_mismatch", info.Code)
	}
}

func TestClassifyErrorInfo_DetectsDotnetSDKMissing(t *testing.T) {
	info := ClassifyErrorInfo("roslyn", "It was not possible to find any installed .NET SDKs", nil)

	if info.Kind != "sdk" {
		t.Fatalf("Kind = %q, want sdk", info.Kind)
	}
	if info.Code != "dotnet_sdk_missing" {
		t.Fatalf("Code = %q, want dotnet_sdk_missing", info.Code)
	}
}

func TestResolveWindowPresetRecent(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	window, err := ResolveWindow("recent", now)
	if err != nil {
		t.Fatalf("ResolveWindow: %v", err)
	}
	if window.Name != "recent" {
		t.Fatalf("Name = %q, want recent", window.Name)
	}
	if got := now.Sub(window.Since); got != 24*time.Hour {
		t.Fatalf("duration = %s, want 24h", got)
	}
}
