package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestSearchPatternRgReturnsPartialResultsOnTimeout(t *testing.T) {
	root, name := setupTestWorkspace(t)
	project := testProject(name)
	writeWorkspaceFile(t, root, "src/Partial.cs", "namespace Demo; public class PartialNeedle { }\n")

	ctx := newManualDeadlineContext(context.Background())
	originalCommand := rgCommand
	rgCommand = fakeSlowRgCommand(t, root, func() {
		time.Sleep(200 * time.Millisecond)
		ctx.expire()
	})
	t.Cleanup(func() { rgCommand = originalCommand })
	t.Cleanup(ctx.cancel)

	diagnostics := &searchPatternDiagnostics{}
	items, err := searchPatternRgWithDiagnostics(ctx, root, root, project, "PartialNeedle", false, 10, "rg", diagnostics)
	if err != nil {
		t.Fatalf("searchPatternRgWithDiagnostics: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected partial result, got %#v", items)
	}
	if !diagnostics.TimedOut || diagnostics.PartialCount != 1 {
		t.Fatalf("timeout diagnostics = %#v, want timed out with one partial", diagnostics)
	}
}

func TestNavSearchTimeoutReturnsUsefulEnvelope(t *testing.T) {
	root, name := setupTestWorkspace(t)
	writeWorkspaceFile(t, root, "src/Partial.cs", "namespace Demo; public class PartialNeedle { }\n")
	forceTestRipgrepPath(t, root)

	ctx := newManualDeadlineContext(context.Background())
	originalCommand := rgCommand
	rgCommand = fakeSlowRgCommand(t, root, func() {
		time.Sleep(200 * time.Millisecond)
		ctx.expire()
	})
	t.Cleanup(func() { rgCommand = originalCommand })
	t.Cleanup(ctx.cancel)

	app := New(root, nil)
	env, err := app.Execute(ctx, model.CommandRequest{
		Operation: "nav.search",
		Context:   model.QueryOptions{Workspace: name, MaxItems: 10},
		Payload:   map[string]any{"pattern": "PartialNeedle"},
	})
	if err != nil {
		t.Fatalf("nav.search should return partial timeout envelope, got error: %v", err)
	}
	items, ok := env.Items.([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one partial item, got %#v", env.Items)
	}
	warnings := strings.Join(env.Warnings, " ")
	if !strings.Contains(warnings, "search timed out") || !strings.Contains(env.Hint, "partial result") {
		t.Fatalf("expected timeout warning and hint, warnings=%v hint=%q", env.Warnings, env.Hint)
	}
	if env.NextHint == nil || !strings.Contains(*env.NextHint, "--repo") {
		t.Fatalf("expected next_hint to suggest narrowing, got %#v", env.NextHint)
	}
	if env.Coach == nil || env.Coach.Trigger != coachTriggerSearchTimeout {
		t.Fatalf("expected search_timeout coach, got %#v", env.Coach)
	}
}

func TestSearchPatternFallbackReturnsErrorOnExternalCancellation(t *testing.T) {
	root, name := setupTestWorkspace(t)
	project := testProject(name)
	writeWorkspaceFile(t, root, "src/Partial.cs", "namespace Demo; public class PartialNeedle { }\n")

	ctx, cancel := context.WithCancel(context.Background())
	originalHook := searchPatternGoAfterMatch
	searchPatternGoAfterMatch = cancel
	t.Cleanup(func() {
		searchPatternGoAfterMatch = originalHook
		cancel()
	})

	diagnostics := &searchPatternDiagnostics{}
	items, err := searchPatternFallbackWithDiagnostics(ctx, root, root, project, "PartialNeedle", false, 10, diagnostics)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected external cancellation error, got items=%#v err=%v", items, err)
	}
	if diagnostics.TimedOut || diagnostics.PartialCount != 0 {
		t.Fatalf("external cancellation should not be reported as timeout diagnostics: %#v", diagnostics)
	}
}

func TestSearchPatternFallbackReturnsPartialResultsOnDeadline(t *testing.T) {
	root, name := setupTestWorkspace(t)
	project := testProject(name)
	writeWorkspaceFile(t, root, "src/Partial.cs", "namespace Demo; public class PartialNeedle { }\n")

	ctx := newManualDeadlineContext(context.Background())
	originalHook := searchPatternGoAfterMatch
	searchPatternGoAfterMatch = ctx.expire
	t.Cleanup(func() {
		searchPatternGoAfterMatch = originalHook
		ctx.cancel()
	})

	diagnostics := &searchPatternDiagnostics{}
	items, err := searchPatternFallbackWithDiagnostics(ctx, root, root, project, "PartialNeedle", false, 10, diagnostics)
	if err != nil {
		t.Fatalf("deadline timeout should return partial fallback results, got err=%v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one partial fallback item, got %#v", items)
	}
	if !diagnostics.TimedOut || diagnostics.PartialCount != 1 {
		t.Fatalf("timeout diagnostics = %#v, want timed out with one partial", diagnostics)
	}
}

func forceTestRipgrepPath(t *testing.T, root string) {
	t.Helper()

	rgPath = ""
	rgResolved = false
	rgOnce = sync.Once{}
	t.Cleanup(func() {
		rgPath = ""
		rgResolved = false
		rgOnce = sync.Once{}
	})

	placeholder := filepath.Join(root, "fake-rg")
	if runtime.GOOS == "windows" {
		placeholder += ".cmd"
	}
	if err := os.WriteFile(placeholder, []byte("placeholder\n"), 0o755); err != nil {
		t.Fatalf("write fake rg placeholder: %v", err)
	}
	t.Setenv("MI_LSP_RG", placeholder)
}

func fakeSlowRgCommand(t *testing.T, root string, afterPartial func()) func(context.Context, string, ...string) *exec.Cmd {
	t.Helper()
	return func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestSearchPatternRgPartialTimeoutHelper")
		partialSignal := filepath.Join(t.TempDir(), "partial-ready")
		cmd.Env = append(os.Environ(),
			"MI_LSP_HELPER_RG_PARTIAL=1",
			"MI_LSP_HELPER_ROOT="+root,
			"MI_LSP_HELPER_PARTIAL_SIGNAL="+partialSignal,
		)
		go func() {
			for {
				if _, err := os.Stat(partialSignal); err == nil {
					afterPartial()
					return
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Millisecond):
				}
			}
		}()
		return cmd
	}
}

func TestSearchPatternRgPartialTimeoutHelper(t *testing.T) {
	if os.Getenv("MI_LSP_HELPER_RG_PARTIAL") != "1" {
		return
	}
	root := os.Getenv("MI_LSP_HELPER_ROOT")
	fmt.Printf("%s:1:PartialNeedle\n", filepath.Join(root, "src", "Partial.cs"))
	if signal := os.Getenv("MI_LSP_HELPER_PARTIAL_SIGNAL"); signal != "" {
		if err := os.WriteFile(signal, []byte("ready"), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write partial signal: %v\n", err)
			os.Exit(2)
		}
	}
	time.Sleep(5 * time.Second)
	os.Exit(0)
}

type manualDeadlineContext struct {
	parent context.Context
	done   chan struct{}
	once   sync.Once
	mu     sync.Mutex
	err    error
}

func newManualDeadlineContext(parent context.Context) *manualDeadlineContext {
	return &manualDeadlineContext{
		parent: parent,
		done:   make(chan struct{}),
	}
}

func (c *manualDeadlineContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (c *manualDeadlineContext) Done() <-chan struct{} {
	return c.done
}

func (c *manualDeadlineContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	return c.parent.Err()
}

func (c *manualDeadlineContext) Value(key any) any {
	return c.parent.Value(key)
}

func (c *manualDeadlineContext) expire() {
	c.finish(context.DeadlineExceeded)
}

func (c *manualDeadlineContext) cancel() {
	c.finish(context.Canceled)
}

func (c *manualDeadlineContext) finish(err error) {
	c.once.Do(func() {
		c.mu.Lock()
		c.err = err
		c.mu.Unlock()
		close(c.done)
	})
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
