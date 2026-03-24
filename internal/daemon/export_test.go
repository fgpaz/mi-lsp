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
		{ID: 1, OccurredAt: now, Workspace: "multi-tedi", WorkspaceRoot: "C:/repos/mios/multi-tedi", WorkspaceAlias: "multi-tedi", Operation: "nav.find", Backend: "roslyn", Success: true, LatencyMs: 80},
		{ID: 2, OccurredAt: now, Workspace: "C:/repos/mios/multi-tedi", WorkspaceRoot: "C:/repos/mios/multi-tedi", WorkspaceAlias: "multi-tedi", Operation: "nav.refs", Backend: "roslyn", Success: true, LatencyMs: 90, Warnings: []string{"slow"}},
		{ID: 3, OccurredAt: now, Workspace: "gastos", WorkspaceRoot: "C:/repos/mios/gastos", WorkspaceAlias: "gastos", Operation: "nav.find", Backend: "roslyn", Success: false, LatencyMs: 200, Error: "A compatible .NET SDK was not found", ErrorKind: "sdk", ErrorCode: "dotnet_global_json_mismatch"},
		{ID: 4, OccurredAt: now, Workspace: "gastos", WorkspaceRoot: "C:/repos/mios/gastos", WorkspaceAlias: "gastos", Operation: "nav.find", Backend: "catalog", Success: true, LatencyMs: 42},
		{ID: 5, OccurredAt: now, Workspace: "gastos", WorkspaceRoot: "C:/repos/mios/gastos", WorkspaceAlias: "gastos", Operation: "nav.search", Backend: "text", Success: true, LatencyMs: 30},
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
	if !strings.Contains(output, "dotnet_global_json_mismatch") {
		t.Error("output should contain typed error code")
	}
	if !strings.Contains(output, "Total operations: 5") {
		t.Error("output should contain total ops")
	}
}

func TestRenderCSV(t *testing.T) {
	events := []model.AccessEvent{
		{ID: 1, OccurredAt: time.Date(2026, 3, 18, 10, 0, 0, 0, time.UTC), Workspace: "test", WorkspaceRoot: "C:/repos/mios/test", WorkspaceAlias: "test", WorkspaceInput: "test", Operation: "nav.find", Backend: "catalog", Success: true, LatencyMs: 42, ErrorKind: "sdk", ErrorCode: "dotnet_sdk_missing"},
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
