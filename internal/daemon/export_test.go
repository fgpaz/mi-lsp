package daemon

import (
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestComputeExportSummary(t *testing.T) {
	now := time.Now()
	events := []model.AccessEvent{
		{ID: 1, OccurredAt: now, ClientName: "codex", Workspace: "multi-tedi", WorkspaceRoot: "C:/repos/mios/multi-tedi", WorkspaceAlias: "multi-tedi", Operation: "nav.find", Backend: "roslyn", Route: "daemon", Success: true, LatencyMs: 80, FailureStage: "none"},
		{ID: 2, OccurredAt: now, ClientName: "codex", Workspace: "C:/repos/mios/multi-tedi", WorkspaceRoot: "C:/repos/mios/multi-tedi", WorkspaceAlias: "multi-tedi", Operation: "nav.refs", Backend: "roslyn", Route: "daemon", Success: true, LatencyMs: 90, Warnings: []string{"slow"}, WarningCount: 1, FailureStage: "none"},
		{ID: 3, OccurredAt: now, ClientName: "claude", Workspace: "gastos", WorkspaceRoot: "C:/repos/mios/gastos", WorkspaceAlias: "gastos", Operation: "nav.find", Backend: "roslyn", Route: "direct", Success: false, LatencyMs: 200, Error: "A compatible .NET SDK was not found", ErrorKind: "sdk", ErrorCode: "dotnet_global_json_mismatch", FailureStage: "backend"},
		{ID: 4, OccurredAt: now, ClientName: "claude", Workspace: "gastos", WorkspaceRoot: "C:/repos/mios/gastos", WorkspaceAlias: "gastos", Operation: "nav.find", Backend: "catalog", Route: "direct", Success: true, LatencyMs: 42, HintCode: "repo_selector_invalid", FailureStage: "selector_validation"},
		{ID: 5, OccurredAt: now, ClientName: "codex", Workspace: "gastos", WorkspaceRoot: "C:/repos/mios/gastos", WorkspaceAlias: "gastos", Operation: "nav.search", Backend: "text", Route: "direct_fallback", Success: true, LatencyMs: 30, HintCode: "regex_suspected", FailureStage: "none"},
	}

	summary := ComputeExportSummary(events)

	if summary.TotalOps != 5 {
		t.Errorf("TotalOps = %d, want 5", summary.TotalOps)
	}
	if len(summary.ByWorkspace) != 2 {
		t.Errorf("len(ByWorkspace) = %d, want 2", len(summary.ByWorkspace))
	}
	if len(summary.ByOperation) != 3 {
		t.Errorf("len(ByOperation) = %d, want 3", len(summary.ByOperation))
	}
	if len(summary.ByRoute) != 3 {
		t.Errorf("len(ByRoute) = %d, want 3", len(summary.ByRoute))
	}
	if len(summary.ByClient) != 2 {
		t.Errorf("len(ByClient) = %d, want 2", len(summary.ByClient))
	}
	if len(summary.ByHintCode) != 2 {
		t.Errorf("len(ByHintCode) = %d, want 2", len(summary.ByHintCode))
	}
	if len(summary.ByFailureStage) != 3 {
		t.Errorf("len(ByFailureStage) = %d, want 3", len(summary.ByFailureStage))
	}

	multiTedi, ok := summary.ByWorkspace["C:/repos/mios/multi-tedi"]
	if !ok {
		t.Fatal("missing workspace root 'C:/repos/mios/multi-tedi'")
	}
	if multiTedi.Ops != 2 {
		t.Errorf("multiTedi.Ops = %d, want 2", multiTedi.Ops)
	}
	if multiTedi.Errors != 0 {
		t.Errorf("multiTedi.Errors = %d, want 0", multiTedi.Errors)
	}
	if multiTedi.Warnings != 1 {
		t.Errorf("multiTedi.Warnings = %d, want 1", multiTedi.Warnings)
	}

	navFind, ok := summary.ByOperation["nav.find"]
	if !ok {
		t.Fatal("missing operation 'nav.find'")
	}
	if navFind.Ops != 3 {
		t.Errorf("nav.find.Ops = %d, want 3", navFind.Ops)
	}
	if navFind.Errors != 1 {
		t.Errorf("nav.find.Errors = %d, want 1", navFind.Errors)
	}

	if len(summary.TopErrors) != 1 {
		t.Fatalf("len(TopErrors) = %d, want 1", len(summary.TopErrors))
	}
	if summary.TopErrors[0].ErrorCode != "dotnet_global_json_mismatch" {
		t.Errorf("TopErrors[0].ErrorCode = %q, want 'dotnet_global_json_mismatch'", summary.TopErrors[0].ErrorCode)
	}
	if summary.TopErrors[0].ErrorKind != "sdk" {
		t.Errorf("TopErrors[0].ErrorKind = %q, want 'sdk'", summary.TopErrors[0].ErrorKind)
	}
	if summary.ByRoute["daemon"].Ops != 2 {
		t.Errorf("ByRoute[daemon].Ops = %d, want 2", summary.ByRoute["daemon"].Ops)
	}
	if summary.ByClient["codex"].Ops != 3 {
		t.Errorf("ByClient[codex].Ops = %d, want 3", summary.ByClient["codex"].Ops)
	}
	if summary.ByHintCode["regex_suspected"].Ops != 1 {
		t.Errorf("ByHintCode[regex_suspected].Ops = %d, want 1", summary.ByHintCode["regex_suspected"].Ops)
	}
	if summary.ByFailureStage["selector_validation"].Ops != 1 {
		t.Errorf("ByFailureStage[selector_validation].Ops = %d, want 1", summary.ByFailureStage["selector_validation"].Ops)
	}
	if summary.ByFailureStage["backend"].Ops != 1 {
		t.Errorf("ByFailureStage[backend].Ops = %d, want 1", summary.ByFailureStage["backend"].Ops)
	}
}

func TestRenderSummaryTable(t *testing.T) {
	summary := ExportSummary{
		TotalOps:    5,
		WindowLabel: "recent (24h)",
		ByWorkspace: map[string]WorkspaceStat{
			"C:/repos/mios/multi-tedi": {Ops: 3, Errors: 1, Warnings: 1, P50Ms: 90},
			"C:/repos/mios/gastos":     {Ops: 2, Errors: 0, Warnings: 0, P50Ms: 36},
		},
		ByOperation: map[string]WorkspaceStat{
			"nav.find":   {Ops: 3, Errors: 1, Warnings: 0, P50Ms: 80},
			"nav.search": {Ops: 1, Errors: 0, Warnings: 0, P50Ms: 30},
		},
		ByRoute: map[string]WorkspaceStat{
			"daemon": {Ops: 2, Errors: 0, Warnings: 1, P50Ms: 90},
			"direct": {Ops: 3, Errors: 1, Warnings: 0, P50Ms: 42},
		},
		ByClient: map[string]WorkspaceStat{
			"codex":  {Ops: 3, Errors: 0, Warnings: 1, P50Ms: 80},
			"claude": {Ops: 2, Errors: 1, Warnings: 0, P50Ms: 42},
		},
		ByHintCode: map[string]WorkspaceStat{
			"regex_suspected":       {Ops: 1, Errors: 0, Warnings: 1, P50Ms: 30},
			"repo_selector_invalid": {Ops: 1, Errors: 1, Warnings: 1, P50Ms: 12},
		},
		ByFailureStage: map[string]WorkspaceStat{
			"none":                {Ops: 4, Errors: 0, Warnings: 1, P50Ms: 42},
			"selector_validation": {Ops: 1, Errors: 1, Warnings: 1, P50Ms: 12},
		},
		TopErrors: []ErrorFrequency{
			{ErrorCode: "dotnet_global_json_mismatch", ErrorKind: "sdk", ErrorText: "A compatible .NET SDK was not found", Count: 1, Workspaces: []string{"C:/repos/mios/gastos"}},
		},
	}

	output := RenderSummaryTable(summary)
	if !strings.Contains(output, "C:/repos/mios/multi-tedi") {
		t.Error("output should contain canonical workspace root")
	}
	if !strings.Contains(output, "recent (24h)") {
		t.Error("output should contain summary window")
	}
	if !strings.Contains(output, "nav.find") {
		t.Error("output should contain per-operation metrics")
	}
	if !strings.Contains(output, "By route") {
		t.Error("output should contain route metrics")
	}
	if !strings.Contains(output, "By client") {
		t.Error("output should contain client metrics")
	}
	if !strings.Contains(output, "By hint code") {
		t.Error("output should contain hint metrics")
	}
	if !strings.Contains(output, "By failure stage") {
		t.Error("output should contain failure-stage metrics")
	}
	if !strings.Contains(output, "dotnet_global_json_mismatch") {
		t.Error("output should contain typed error code")
	}
	if !strings.Contains(output, "Total operations: 5") {
		t.Error("output should contain total ops")
	}
}

func TestRenderCSV(t *testing.T) {
	events := []model.AccessEvent{
		{ID: 1, OccurredAt: time.Date(2026, 3, 18, 10, 0, 0, 0, time.UTC), Workspace: "test", WorkspaceRoot: "C:/repos/mios/test", WorkspaceAlias: "test", WorkspaceInput: "test", Operation: "nav.find", Backend: "catalog", Success: true, LatencyMs: 42, ErrorKind: "sdk", ErrorCode: "dotnet_sdk_missing", WarningCount: 1, PatternMode: "literal", RoutingOutcome: "router_error", FailureStage: "selector_validation", HintCode: "repo_selector_invalid", TruncationReason: "max_items", DecisionJSON: `{"pattern_len":7}`},
	}
	csv := RenderCSV(events)
	lines := strings.Split(strings.TrimSpace(csv), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines (header + 1 row), got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "id,occurred_at") {
		t.Error("CSV header missing")
	}
	if !strings.Contains(lines[0], "workspace_root") {
		t.Error("CSV header should include canonical workspace fields")
	}
	if !strings.Contains(lines[0], "error_code") {
		t.Error("CSV header should include typed error fields")
	}
	if !strings.Contains(lines[0], "warning_count") || !strings.Contains(lines[0], "decision_json") {
		t.Error("CSV header should include search/routing telemetry fields")
	}
}

func TestPercentile(t *testing.T) {
	sorted := []int64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	p50 := percentile(sorted, 0.50)
	if p50 != 50 {
		t.Errorf("P50 = %d, want 50", p50)
	}
	p0 := percentile([]int64{}, 0.50)
	if p0 != 0 {
		t.Errorf("P50 of empty = %d, want 0", p0)
	}
}
