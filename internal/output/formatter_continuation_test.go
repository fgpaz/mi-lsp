package output

import (
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestRenderContinuationTargetIncludesWorkspace(t *testing.T) {
	rendered := renderContinuationTarget(model.ContinuationTarget{
		Op:        "nav.pack",
		Query:     "AE-POLICY-PROJECTION",
		DocID:     "AE-POLICY-PROJECTION-V2",
		Workspace: "ae-kernel",
	})
	if !strings.Contains(rendered, "workspace=ae-kernel") {
		t.Fatalf("expected workspace in rendered continuation, got %q", rendered)
	}
	if !strings.Contains(rendered, "doc_id=AE-POLICY-PROJECTION-V2") {
		t.Fatalf("expected doc id preserved, got %q", rendered)
	}
}

func TestRenderContinuationTargetOmitsEmptyWorkspace(t *testing.T) {
	rendered := renderContinuationTarget(model.ContinuationTarget{Op: "nav.pack", Query: "task"})
	if strings.Contains(rendered, "workspace=") {
		t.Fatalf("empty workspace must stay omitted, got %q", rendered)
	}
}
