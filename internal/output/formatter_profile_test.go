package output

import (
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestApplyProfileHarnessMicroStripsNoise(t *testing.T) {
	hint := "next page"
	env := model.Envelope{
		Ok:      true,
		Backend: "test",
		Profile: model.OutputProfileHarnessMicro,
		Hint:    "human hint",
		NextHint: &hint,
		Coach: &model.Coach{
			Trigger: "x",
			Message: "coach",
		},
		Metrics: &model.EnvelopeMetrics{ResponseBytes: 99},
		MemoryPointer: &model.MemoryPointer{
			DocID: "DOC",
			Why:   "memory",
		},
		Warnings: []string{"w1", "w2", "w3", "w4"},
		Items: []map[string]any{{
			"path":    "internal/service/app.go",
			"verbose": strings.Repeat("x", 200),
		}},
		Continuation: &model.Continuation{
			Reason: "keep me",
			Next:   model.ContinuationTarget{Op: "nav.batch"},
		},
	}

	out := ApplyProfile(env)
	if out.Hint != "" || out.NextHint != nil || out.Coach != nil || out.Metrics != nil || out.MemoryPointer != nil {
		t.Fatalf("harness-micro should strip human noise, got hint=%q coach=%v metrics=%v memory=%v next=%v", out.Hint, out.Coach, out.Metrics, out.MemoryPointer, out.NextHint)
	}
	if len(out.Warnings) != 3 {
		t.Fatalf("expected warnings capped to 3, got %d", len(out.Warnings))
	}
	if out.Continuation == nil || out.Continuation.Next.Op != "nav.batch" {
		t.Fatalf("continuation must be preserved, got %#v", out.Continuation)
	}
}
