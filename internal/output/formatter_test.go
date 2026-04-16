package output

import (
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestRenderCompact_ServiceSurfaceSummary(t *testing.T) {
	env := model.Envelope{
		Ok:      true,
		Backend: "catalog+text",
		Items: []model.ServiceSurfaceSummary{{
			Service: "conversation-fabric",
			Path:    "src/backend/conversation-fabric",
			Profile: "dotnet-microservice",
			Symbols: map[string]int{"class": 3},
		}},
	}

	rendered, err := Render(env, "compact", false)
	if err != nil {
		t.Fatalf("render compact: %v", err)
	}
	text := string(rendered)
	if !strings.Contains(text, `"service":"conversation-fabric"`) {
		t.Fatalf("expected compact json to contain service field, got %s", text)
	}
	if !strings.Contains(text, `"profile":"dotnet-microservice"`) {
		t.Fatalf("expected compact json to contain profile field, got %s", text)
	}
}

func TestRenderText_ServiceSurfaceSummary(t *testing.T) {
	env := model.Envelope{
		Ok:        true,
		Backend:   "catalog+text",
		Workspace: "svc",
		Items: []model.ServiceSurfaceSummary{{
			Service: "conversation-fabric",
			Path:    "src/backend/conversation-fabric",
			Profile: "dotnet-microservice",
		}},
	}

	rendered, err := Render(env, "text", false)
	if err != nil {
		t.Fatalf("render text: %v", err)
	}
	text := string(rendered)
	if !strings.Contains(text, "service conversation-fabric") {
		t.Fatalf("expected readable text output, got %s", text)
	}
}

func TestRenderCompact_SignatureTruncation(t *testing.T) {
	longSig := "public static Task<IEnumerable<IAsyncEnumerable<Dictionary<string, Tuple<int, int, string>>>>> DoSomethingVeryComplicated(string param1, int param2, object param3) async"
	env := model.Envelope{
		Ok:      true,
		Backend: "roslyn",
		Items: []model.SymbolRecord{{
			Name:       "DoSomething",
			Kind:       "method",
			FilePath:   "src/Program.cs",
			StartLine:  42,
			Signature:  longSig,
			Scope:      "public",
			Implements: "",
		}},
	}

	rendered, err := Render(env, "compact", false)
	if err != nil {
		t.Fatalf("render compact: %v", err)
	}
	text := string(rendered)
	// Signature should be truncated to 120 chars and have "..." appended
	if !strings.Contains(text, "...") {
		t.Fatalf("expected truncated signature to contain '...', got %s", text)
	}
	if len(longSig) <= 120 {
		t.Fatalf("test setup error: test signature should be longer than 120 chars")
	}
}

func TestRenderCompact_TokenEstimate(t *testing.T) {
	env := model.Envelope{
		Ok:      true,
		Backend: "roslyn",
		Items: []model.SymbolRecord{{
			Name:      "TestMethod",
			Kind:      "method",
			FilePath:  "src/Test.cs",
			StartLine: 1,
			Signature: "void TestMethod()",
		}},
		Stats: model.Stats{},
	}

	rendered, err := Render(env, "compact", false)
	if err != nil {
		t.Fatalf("render compact: %v", err)
	}

	// After rendering, Stats should have TokensEstimate set
	if !strings.Contains(string(rendered), "tokens_est") {
		t.Fatalf("expected rendered output to contain tokens_est field, got %s", string(rendered))
	}
}

func TestCompactItems_CompressionMode(t *testing.T) {
	items := []model.SymbolRecord{{
		Name:       "CompressTest",
		Kind:       "class",
		FilePath:   "test.cs",
		StartLine:  1,
		Signature:  "public class CompressTest",
		Scope:      "public",
		Implements: "IDisposable",
	}}

	// With compress=false, should include "impl" and "sc"
	uncompressed := compactItems(items, false)
	uncompressedMap := uncompressed.([]map[string]any)[0]
	if _, hasImpl := uncompressedMap["impl"]; !hasImpl {
		t.Fatalf("expected impl field when compress=false")
	}
	if _, hasScope := uncompressedMap["sc"]; !hasScope {
		t.Fatalf("expected sc field when compress=false")
	}

	// With compress=true, should NOT include "impl" and "sc"
	compressed := compactItems(items, true)
	compressedMap := compressed.([]map[string]any)[0]
	if _, hasImpl := compressedMap["impl"]; hasImpl {
		t.Fatalf("expected no impl field when compress=true")
	}
	if _, hasScope := compressedMap["sc"]; hasScope {
		t.Fatalf("expected no sc field when compress=true")
	}
	// But should still have sig (truncated)
	if _, hasSig := compressedMap["sig"]; !hasSig {
		t.Fatalf("expected sig field even when compress=true")
	}
}

func TestRenderStructuredFormats_PreserveSymbolWorkspace(t *testing.T) {
	env := model.Envelope{
		Ok:      true,
		Backend: "catalog",
		Items: []model.SymbolRecord{{
			Name:      "IExpenseRepository",
			Kind:      "interface",
			FilePath:  "backend/Gastos.Domain/Interfaces/IExpenseRepository.cs",
			StartLine: 24,
			Scope:     "public",
			Workspace: "gastos",
		}},
	}

	tests := []struct {
		format   string
		contains []string
	}{
		{format: "compact", contains: []string{`"workspace":"gastos"`}},
		{format: "toon", contains: []string{"workspace", "gastos"}},
		{format: "yaml", contains: []string{"workspace: gastos"}},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			rendered, err := Render(env, tt.format, false)
			if err != nil {
				t.Fatalf("Render(%s): %v", tt.format, err)
			}
			text := string(rendered)
			for _, expected := range tt.contains {
				if !strings.Contains(text, expected) {
					t.Fatalf("Render(%s) should preserve symbol workspace, got %s", tt.format, text)
				}
			}
		})
	}
}

func TestRenderFormats_IncludeCoachBlock(t *testing.T) {
	env := model.Envelope{
		Ok:        true,
		Backend:   "ask",
		Workspace: "mi-lsp",
		Items: []model.AskResult{{
			Summary:    "Fallback answer",
			PrimaryDoc: model.AskDocEvidence{Path: ".docs/wiki/07_baseline_tecnica.md"},
		}},
		Coach: &model.Coach{
			Trigger:    "text_fallback",
			Message:    "This answer relied on textual fallback.",
			Confidence: "low",
			Actions: []model.CoachAction{{
				Kind:    "refine",
				Label:   "Inspect supporting code",
				Command: `mi-lsp nav search "Program" --workspace mi-lsp --include-content`,
			}},
		},
		Continuation: &model.Continuation{
			Reason: "low_evidence",
			Next: model.ContinuationTarget{
				Op:    "nav.search",
				Query: "Program",
				DocID: "RF-QRY-010",
			},
		},
		MemoryPointer: &model.MemoryPointer{
			DocID:     "RF-QRY-010",
			Why:       "RF-QRY-010 changed recently.",
			ReentryOp: "nav.search",
			Handoff:   "plans/2026-04-16-reentry-wave",
			Stale:     true,
		},
	}

	compactRendered, err := Render(env, "compact", false)
	if err != nil {
		t.Fatalf("render compact: %v", err)
	}
	if !strings.Contains(string(compactRendered), `"coach":{"trigger":"text_fallback"`) {
		t.Fatalf("expected compact render to include coach block, got %s", string(compactRendered))
	}
	if !strings.Contains(string(compactRendered), `"continuation":{"reason":"low_evidence"`) {
		t.Fatalf("expected compact render to include continuation block, got %s", string(compactRendered))
	}
	if !strings.Contains(string(compactRendered), `"memory_pointer":{"doc_id":"RF-QRY-010"`) {
		t.Fatalf("expected compact render to include memory pointer, got %s", string(compactRendered))
	}

	textRendered, err := Render(env, "text", false)
	if err != nil {
		t.Fatalf("render text: %v", err)
	}
	text := string(textRendered)
	if !strings.Contains(text, "coach: trigger=text_fallback confidence=low") {
		t.Fatalf("expected text render to include coach header, got %s", text)
	}
	if !strings.Contains(text, "action refine Inspect supporting code") {
		t.Fatalf("expected text render to include coach action, got %s", text)
	}
	if !strings.Contains(text, "continuation: reason=low_evidence") {
		t.Fatalf("expected text render to include continuation, got %s", text)
	}
	if !strings.Contains(text, "memory_pointer: doc_id=RF-QRY-010 reentry_op=nav.search stale=true") {
		t.Fatalf("expected text render to include memory pointer, got %s", text)
	}
}
