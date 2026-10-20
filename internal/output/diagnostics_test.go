package output

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestDiagnosticsHumanAndStructuredFormatsPreserveHonesty(t *testing.T) {
	hint := "request one file"
	env := model.Envelope{ContractVersion: "diagnostics/v1", Ok: true, Partial: true, Truncated: true, Diagnostics: &model.DiagnosticsSummary{Errors: 2, Warnings: 200, FilesRequested: 4, FilesChecked: 1, Returned: 1}, NextHint: &hint, Items: []model.Diagnostic{{Path: "bad.go", Line: 2, Column: 3, Severity: "error", Message: "bad type\ncontinuation", Code: "typecheck", Source: "gopls"}}, Omissions: []model.EnvelopeOmission{{Path: "missing.py", ErrorCode: "file_missing", Reason: "deleted source"}}}
	for _, format := range []string{"text", "agent"} {
		rendered, e := Render(env, format, false)
		if e != nil || !strings.Contains(string(rendered), "2 error(s), 200 warning(s)") || !strings.Contains(string(rendered), "partial/unavailable (not clean)") || !strings.Contains(string(rendered), "bad.go:2:3 error") || !strings.Contains(string(rendered), "file_missing") || !strings.Contains(string(rendered), "truncated=true") {
			t.Fatalf("format=%s rendered=%s err=%v", format, rendered, e)
		}
	}
	rendered, e := Render(env, "json", false)
	if e != nil {
		t.Fatal(e)
	}
	var decoded model.Envelope
	if e := json.Unmarshal(rendered, &decoded); e != nil {
		t.Fatal(e)
	}
	if decoded.ContractVersion != "diagnostics/v1" || decoded.Diagnostics.Errors != 2 || !decoded.Partial {
		t.Fatalf("%s", rendered)
	}
}
func TestDiagnosticsMicroProfileRetainsContinuation(t *testing.T) {
	next := "raise --token-budget"
	env := ApplyProfile(model.Envelope{Profile: model.OutputProfileHarnessMicro, Diagnostics: &model.DiagnosticsSummary{}, NextHint: &next, Truncated: true})
	if env.NextHint == nil || *env.NextHint != next {
		t.Fatal("diagnostic next-hint dropped")
	}
}
func TestDiagnosticsReturnedCountTracksGenericLimit(t *testing.T) {
	env := model.Envelope{Ok: true, Diagnostics: &model.DiagnosticsSummary{Errors: 2, Returned: 2}, Items: []model.Diagnostic{{Path: "a.go", Line: 1, Column: 1, Severity: "error", Message: "one"}, {Path: "b.go", Line: 1, Column: 1, Severity: "error", Message: "two"}}}
	limited := ApplyEnvelopeLimits(env, model.QueryOptions{MaxItems: 1})
	if limited.Diagnostics.Errors != 2 || limited.Diagnostics.Returned != 1 || !limited.Truncated {
		t.Fatalf("%+v", limited)
	}
}
