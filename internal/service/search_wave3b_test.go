package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestSearchPatternRg_StartAccessDeniedFallsBackToGoScanner(t *testing.T) {
	root, name := setupTestWorkspace(t)
	project := testProject(name)
	originalCommand := rgCommand
	rgCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Err = errors.New("access is denied")
		return cmd
	}
	t.Cleanup(func() { rgCommand = originalCommand })

	items, err := searchPatternRg(context.Background(), root, root, project, "HelloWorld", false, 10, "rg")
	if err != nil {
		t.Fatalf("searchPatternRg should fall back on start access denied: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected fallback result, got %#v", items)
	}
	if got, _ := items[0]["file"].(string); got != "src/Hello.cs" {
		t.Fatalf("file = %q, want src/Hello.cs", got)
	}
}

func TestSearchPatternRg_WaitPermissionDeniedFallsBackToGoScanner(t *testing.T) {
	root, name := setupTestWorkspace(t)
	project := testProject(name)

	scriptPath := filepath.Join(root, "fake-rg-permission")
	scriptBody := "#!/bin/sh\n" +
		"printf 'rg: .mi-lsp/index.db-wal: Permission denied\\n' >&2\n" +
		"exit 2\n"
	if runtime.GOOS == "windows" {
		scriptPath += ".cmd"
		scriptBody = "@echo off\r\n" +
			"echo rg: .mi-lsp\\index.db-wal: Access is denied. 1>&2\r\n" +
			"exit /b 2\r\n"
	}
	if err := os.WriteFile(scriptPath, []byte(scriptBody), 0o755); err != nil {
		t.Fatalf("write fake rg: %v", err)
	}

	items, err := searchPatternRg(context.Background(), root, root, project, "HelloWorld", false, 10, scriptPath)
	if err != nil {
		t.Fatalf("searchPatternRg should fall back on wait permission error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected fallback result, got %#v", items)
	}
}

func TestSearchPatternRg_IgnoresMiLspIndexSidecars(t *testing.T) {
	args := strings.Join(buildRipgrepArgs("needle", false, "."), "\x00")
	for _, ignored := range []string{"!.mi-lsp/**", "!**/.mi-lsp/**", "!.mi-lsp/index.db", "!.mi-lsp/index.db-wal", "!.mi-lsp/index.db-shm"} {
		if !strings.Contains(args, ignored) {
			t.Fatalf("buildRipgrepArgs missing ignore glob %q in %#v", ignored, args)
		}
	}
}

func TestSearchPatternFallbackIgnoresNestedMiLspState(t *testing.T) {
	root, name := setupTestWorkspace(t)
	project := testProject(name)
	writeWorkspaceFile(t, root, "repo/.mi-lsp/index.db", "NestedNeedle should never be returned\n")
	writeWorkspaceFile(t, root, "repo/src/Visible.cs", "namespace Demo; public class Visible { const string Value = \"NestedNeedle\"; }\n")

	items, err := searchPatternFallback(context.Background(), root, root, project, "NestedNeedle", false, 10)
	if err != nil {
		t.Fatalf("searchPatternFallback: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected only visible source match, got %#v", items)
	}
	file, _ := items[0]["file"].(string)
	if strings.Contains(filepath.ToSlash(file), ".mi-lsp") {
		t.Fatalf("fallback search returned operational state: %#v", items)
	}
	if file != "repo/src/Visible.cs" {
		t.Fatalf("file = %q, want repo/src/Visible.cs", file)
	}
}

func TestEnrichSearchResultsWithContent_BatchesLineContentByFile(t *testing.T) {
	root, name := setupTestWorkspace(t)
	writeWorkspaceFile(t, root, "src/Batched.ts", strings.Join([]string{
		"line 1",
		"line 2 target",
		"line 3",
		"line 4 target",
		"line 5",
	}, "\n"))

	items := []map[string]any{
		{"file": "src/Batched.ts", "line": 2, "text": "line 2 target"},
		{"file": "src/Batched.ts", "line": 4, "text": "line 4 target"},
	}
	warnings := enrichSearchResultsWithContent(context.Background(), model.WorkspaceRegistration{Name: name, Root: root}, items, 1, "lines")
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if got, _ := items[0]["content"].(string); !strings.Contains(got, "line 2 target") || strings.Contains(got, "line 5") {
		t.Fatalf("first content = %q", got)
	}
	if got, _ := items[1]["content"].(string); !strings.Contains(got, "line 4 target") || strings.Contains(got, "line 1") {
		t.Fatalf("second content = %q", got)
	}
	if items[0]["content_mode"] != "lines" || items[1]["content_mode"] != "lines" {
		t.Fatalf("content modes = %#v %#v", items[0]["content_mode"], items[1]["content_mode"])
	}
}
