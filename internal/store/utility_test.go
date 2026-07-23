package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestRecordUtilityEventRejectsRawOrUnscopedCandidate(t *testing.T) {
	db, _ := seedTestDB(t)
	defer db.Close()
	event := model.UtilityEvent{WorkspaceScope: "demo", Intent: "callers", Operation: "rank", CandidateNodeKey: "prompt text", Signal: model.UtilitySignalResultSelected, Value: 1}
	if err := RecordUtilityEvent(context.Background(), db, event); err == nil {
		t.Fatal("raw candidate must be rejected")
	}
	event.CandidateNodeKey = strings.Repeat("a", 64)
	if err := RecordUtilityEvent(context.Background(), db, event); err != nil {
		t.Fatal(err)
	}
	if err := RecordUtilityEvent(context.Background(), db, model.UtilityEvent{WorkspaceScope: "demo", Intent: "callers", Operation: "rank", CandidateNodeKey: "", Signal: model.UtilitySignalResultSelected, Value: 1}); err == nil {
		t.Fatal("empty candidate must be rejected")
	}
}

func TestRecordUtilityEventRejectsInvalidGenerationAndOversizedScope(t *testing.T) {
	db, _ := seedTestDB(t)
	defer db.Close()
	candidate := strings.Repeat("c", 64)
	if err := RecordUtilityEvent(context.Background(), db, model.UtilityEvent{WorkspaceScope: "demo", Intent: "callers", Operation: "rank", CandidateNodeKey: candidate, Signal: model.UtilitySignalResultSelected, Value: 1, GenerationID: "not-a-digest"}); err == nil {
		t.Fatal("invalid generation must be rejected")
	}
	if err := RecordUtilityEvent(context.Background(), db, model.UtilityEvent{WorkspaceScope: strings.Repeat("x", model.UtilityMaxWorkspaceScopeLength+1), Intent: "callers", Operation: "rank", CandidateNodeKey: candidate, Signal: model.UtilitySignalResultSelected, Value: 1}); err == nil {
		t.Fatal("oversized workspace scope must be rejected")
	}
}

func TestUtilityGlobalScopeCapBoundsManyCandidateSignals(t *testing.T) {
	db, _ := seedTestDB(t)
	defer db.Close()
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	for i := 0; i < model.UtilityMaxEventsPerScopeIntentOperation+17; i++ {
		event := model.UtilityEvent{
			OccurredAt:       now.Add(time.Duration(i) * time.Nanosecond),
			WorkspaceScope:   "bounded-workspace",
			Intent:           "callers",
			Operation:        "rank",
			CandidateNodeKey: fmt.Sprintf("%064x", i+1),
			Signal:           model.UtilitySignalResultSelected,
			Value:            1,
		}
		if err := RecordUtilityEvent(context.Background(), db, event); err != nil {
			t.Fatalf("RecordUtilityEvent(%d): %v", i, err)
		}
	}
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM utility_events WHERE workspace_scope=? AND intent=? AND operation=?`, "bounded-workspace", "callers", "rank").Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != model.UtilityMaxEventsPerScopeIntentOperation {
		t.Fatalf("stored utility events=%d, want global cap %d", total, model.UtilityMaxEventsPerScopeIntentOperation)
	}
	signals, err := UtilitySignals(context.Background(), db, "bounded-workspace", "callers", "rank", now.Add(time.Duration(model.UtilityMaxEventsPerScopeIntentOperation+17)*time.Nanosecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) > model.UtilityMaxEventsPerScopeIntentOperation {
		t.Fatalf("signals=%d, exceeded global cap %d", len(signals), model.UtilityMaxEventsPerScopeIntentOperation)
	}
	if _, ok := signals[fmt.Sprintf("%064x", 1)]; ok {
		t.Fatal("oldest candidate survived global eviction")
	}
}

func TestUtilityEventsApplyScopeRetentionAndDecay(t *testing.T) {
	db, _ := seedTestDB(t)
	defer db.Close()
	candidate := strings.Repeat("b", 64)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	for _, event := range []model.UtilityEvent{
		{OccurredAt: now.Add(-8 * 24 * time.Hour), WorkspaceScope: "demo", Intent: "callers", Operation: "rank", CandidateNodeKey: candidate, Signal: model.UtilitySignalResultSelected, Value: 1},
		{OccurredAt: now.Add(-40 * 24 * time.Hour), WorkspaceScope: "demo", Intent: "callers", Operation: "rank", CandidateNodeKey: candidate, Signal: model.UtilitySignalFeedbackNegative, Value: -1},
		{OccurredAt: now.Add(-time.Hour), WorkspaceScope: "other", Intent: "callers", Operation: "rank", CandidateNodeKey: candidate, Signal: model.UtilitySignalFeedbackNegative, Value: -1},
	} {
		if event.OccurredAt.Before(now.Add(-30 * 24 * time.Hour)) {
			// Exercise retention through the same persistence path; the event is
			// removed on the next scoped insert.
			if err := RecordUtilityEvent(context.Background(), db, event); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := RecordUtilityEvent(context.Background(), db, event); err != nil {
			t.Fatal(err)
		}
	}
	if err := RecordUtilityEvent(context.Background(), db, model.UtilityEvent{OccurredAt: now, WorkspaceScope: "demo", Intent: "callers", Operation: "rank", CandidateNodeKey: candidate, Signal: model.UtilitySignalResultSelected, Value: 1}); err != nil {
		t.Fatal(err)
	}
	signal, err := UtilityScore(context.Background(), db, "demo", "callers", "rank", candidate, now)
	if err != nil {
		t.Fatal(err)
	}
	if signal.Samples != 2 || signal.Score <= 0 {
		t.Fatalf("signal=%+v, want scoped retained decayed samples", signal)
	}
}
