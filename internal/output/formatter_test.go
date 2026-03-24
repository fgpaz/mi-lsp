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
			Name:      "DoSomething",
			Kind:      "method",
			FilePath:  "src/Program.cs",
			StartLine: 42,
			Signature: longSig,
			Scope:     "public",
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
