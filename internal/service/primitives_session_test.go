package service

import (
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestQSessionRememberSeenAndChangedSnapshot(t *testing.T) {
	state := NewSessionState()
	item := model.QItem{ID: "ws:workspace/symbol/a.go#Run", Revision: "abc123", Text: "func Run() {}"}
	state.Remember("s1", item)
	mark := state.NextMark("s1")
	if !state.Seen("s1", item.ID, item.Rev()) {
		t.Fatal("delivered item not remembered")
	}
	if candidates := state.ChangedCandidates("s1", mark); len(candidates) != 1 || candidates[0].Rev() != item.Rev() {
		t.Fatalf("snapshot: %+v", candidates)
	}
	state.Remember("s1", model.QItem{ID: item.ID, Revision: "def456", Text: "func Run(x int) {}"})
	if state.Seen("s1", item.ID, "abc123") {
		t.Fatal("old revision must not be marked seen after change")
	}
}
func TestQSessionIdleExpiry(t *testing.T) {
	state := NewSessionState()
	state.mu.Lock()
	entry := state.entry("s", time.Now().Add(-sessionIdle-time.Second))
	state.mu.Unlock()
	if entry == nil {
		t.Fatal("entry")
	}
	state.mu.Lock()
	state.entry("other", time.Now())
	_, ok := state.entries["s"]
	state.mu.Unlock()
	if ok {
		t.Fatal("idle session was not expired")
	}
}

func TestQSessionDedupeIsOptInAndRevisionAware(t *testing.T) {
	state := NewSessionState()
	items := []model.QItem{{ID: "one", Revision: "r1"}}
	if got := state.FilterDedupe("s", "key", items); len(got) != 1 {
		t.Fatalf("first results filtered: %+v", got)
	}
	state.RememberDedupe("s", "key", items)
	if got := state.FilterDedupe("s", "key", items); len(got) != 0 {
		t.Fatalf("duplicate results retained: %+v", got)
	}
	changed := []model.QItem{{ID: "one", Revision: "r2"}}
	if got := state.FilterDedupe("s", "key", changed); len(got) != 1 {
		t.Fatalf("changed revision filtered: %+v", got)
	}
}
