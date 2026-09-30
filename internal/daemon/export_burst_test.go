package daemon

import (
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestKnownBurstMarksRepeatsAndKeepsTheFirst(t *testing.T) {
	start := time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC)
	acc := newSummaryAccumulator()
	for i := 0; i < 6; i++ {
		acc.add(model.AccessEvent{
			OccurredAt:     start.Add(time.Duration(i) * time.Second),
			Operation:      "workspace.status",
			Success:        false,
			HintCode:       "workspace_resolution_failed",
			WorkspaceInput: "demo-local",
			ErrorCode:      "workspace_resolution_failed",
			Error:          "workspace not found",
		})
	}
	summary := acc.summary()
	if len(summary.KnownBursts) != 1 {
		t.Fatalf("bursts = %d, want 1", len(summary.KnownBursts))
	}
	if summary.KnownBursts[0].Count != 6 || summary.KnownBursts[0].Noise != 5 {
		t.Fatalf("burst = %+v, want count 6 noise 5", summary.KnownBursts[0])
	}
	if summary.ByHintCode["workspace_resolution_failed"].Ops != 6 {
		t.Fatalf("hint ops = %d, want all 6 still counted", summary.ByHintCode["workspace_resolution_failed"].Ops)
	}
	found := false
	for _, rec := range summary.Recommendations {
		if rec.ID == "known_hint_burst" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing known_hint_burst recommendation")
	}
}
