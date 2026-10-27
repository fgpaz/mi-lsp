package service

import (
	"testing"
	"time"
)

func TestUpdateHasLiveSessionsReadOnly(t *testing.T) {
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	var nilState *SessionState
	if nilState.HasLiveSessions(now) {
		t.Fatal("nil state must be idle")
	}
	for _, tc := range []struct {
		name    string
		touched time.Time
		live    bool
	}{
		{"live", now.Add(-time.Minute), true},
		{"expired", now.Add(-sessionIdle - time.Nanosecond), false},
		{"boundary", now.Add(-sessionIdle), true},
		{"future", now.Add(time.Minute), true},
		{"unknown", time.Time{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &SessionState{entries: map[string]*sessionEntry{"fixture": {touched: tc.touched}}}
			if got := s.HasLiveSessions(now); got != tc.live {
				t.Fatalf("live=%v want=%v", got, tc.live)
			}
			if len(s.entries) != 1 || s.entries["fixture"].touched != tc.touched {
				t.Fatal("snapshot mutated session")
			}
		})
	}
}
