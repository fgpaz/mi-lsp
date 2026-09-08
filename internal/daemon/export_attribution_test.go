package daemon

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestComputeAttributionCohortsAndGaps(t *testing.T) {
	now := time.Now()
	events := []model.AccessEvent{
		// Known client + real session: work cohort.
		{ID: 1, OccurredAt: now, ClientName: "claude-code", SessionID: "s1", Operation: "nav.find", Success: true, LatencyMs: 40},
		// manual-cli invocation: unknown attribution even with a session.
		{ID: 2, OccurredAt: now, ClientName: "manual-cli", SessionID: "s2", Operation: "nav.search", Success: true, LatencyMs: 60},
		// Blank client name: unknown attribution even with a session.
		{ID: 3, OccurredAt: now, ClientName: "", SessionID: "s3", Operation: "nav.find", Success: true, LatencyMs: 20},
		// Known client, no session: cannot confirm a real session.
		{ID: 4, OccurredAt: now, ClientName: "claude-code", Operation: "nav.find", Success: true, LatencyMs: 30},
		// Client name indicating a test harness.
		{ID: 5, OccurredAt: now, ClientName: "qa-agent", SessionID: "s3", Operation: "nav.find", Success: true, LatencyMs: 10},
		// Client name indicating system usage.
		{ID: 6, OccurredAt: now, ClientName: "mi-lsp-daemon", SessionID: "s4", Operation: "workspace.status", Success: true, LatencyMs: 10},
		// Navigation failures for repeated-failure candidates.
		{ID: 7, OccurredAt: now, ClientName: "claude-code", SessionID: "s1", Operation: "nav.search", Success: false, LatencyMs: 900, ErrorCode: "no_matches"},
		{ID: 8, OccurredAt: now, ClientName: "claude-code", SessionID: "s1", Operation: "nav.search", Success: false, LatencyMs: 900, ErrorCode: "no_matches"},
		{ID: 9, OccurredAt: now, ClientName: "claude-code", SessionID: "s2", Operation: "nav.search", Success: false, LatencyMs: 900, ErrorCode: "no_matches"},
		// Slow operation candidates (>= 3 events, p95 >= 5000ms).
		{ID: 10, OccurredAt: now, ClientName: "claude-code", SessionID: "s1", Operation: "nav.graph", Success: true, LatencyMs: 9000},
		{ID: 11, OccurredAt: now, ClientName: "claude-code", SessionID: "s1", Operation: "nav.graph", Success: true, LatencyMs: 9100},
		{ID: 12, OccurredAt: now, ClientName: "claude-code", SessionID: "s1", Operation: "nav.graph", Success: true, LatencyMs: 9200},
	}

	summary := ComputeExportSummary(events)
	attr := summary.Attribution
	if attr == nil {
		t.Fatal("expected attribution to be computed by default")
	}

	if attr.Events != len(events) {
		t.Errorf("Attribution.Events = %d, want %d", attr.Events, len(events))
	}

	// Client class: manual-cli and blank are unknown attribution.
	if got := attr.ByClientClass["known"]; got != 10 {
		t.Errorf("ByClientClass[known] = %d, want 10", got)
	}
	if got := attr.ByClientClass["unknown"]; got != 2 {
		t.Errorf("ByClientClass[unknown] = %d, want 2", got)
	}

	// Cohorts sum to total events, with unknown kept as explicit denominator.
	total := 0
	for _, count := range attr.Cohorts {
		total += count
	}
	if total != len(events) {
		t.Errorf("cohort counts sum to %d, want %d", total, len(events))
	}
	if got := attr.Cohorts["work"]; got != 7 {
		t.Errorf("Cohorts[work] = %d, want 7", got)
	}
	if got := attr.Cohorts["test"]; got != 1 {
		t.Errorf("Cohorts[test] = %d, want 1", got)
	}
	if got := attr.Cohorts["system"]; got != 1 {
		t.Errorf("Cohorts[system] = %d, want 1", got)
	}
	if got := attr.Cohorts["unknown"]; got != 3 {
		t.Errorf("Cohorts[unknown] = %d, want 3", got)
	}

	// Real sessions are distinct session ids, not event counts and not a
	// repeat count: s1, s2, s3, s4 are 4 real sessions across 11 events.
	if attr.RealSessions != 4 {
		t.Errorf("RealSessions = %d, want 4", attr.RealSessions)
	}
	if attr.EventsInRealSessions != 11 {
		t.Errorf("EventsInRealSessions = %d, want 11", attr.EventsInRealSessions)
	}
	if attr.EventsUnknownSession != 1 {
		t.Errorf("EventsUnknownSession = %d, want 1", attr.EventsUnknownSession)
	}

	if attr.NavigationFailures != 3 {
		t.Errorf("NavigationFailures = %d, want 3", attr.NavigationFailures)
	}

	found := false
	for _, rf := range attr.RepeatedFailures {
		if rf.Operation == "nav.search" && rf.ErrorCode == "no_matches" {
			found = true
			if rf.Occurrences != 3 {
				t.Errorf("nav.search/no_matches occurrences = %d, want 3", rf.Occurrences)
			}
			if rf.Sessions != 2 {
				t.Errorf("nav.search/no_matches sessions = %d, want 2", rf.Sessions)
			}
		}
	}
	if !found {
		t.Fatalf("expected nav.search/no_matches repeated failure candidate, got %#v", attr.RepeatedFailures)
	}

	found = false
	for _, slow := range attr.SlowOperations {
		if slow.Operation == "nav.graph" {
			found = true
			if slow.P95Ms < 5000 {
				t.Errorf("nav.graph p95_ms = %d, want >= 5000", slow.P95Ms)
			}
			if slow.Events != 3 {
				t.Errorf("nav.graph events = %d, want 3", slow.Events)
			}
		}
	}
	if !found {
		t.Fatalf("expected nav.graph slow operation candidate, got %#v", attr.SlowOperations)
	}

	// Stable ordering: clients sorted by events desc, then client asc.
	for i := 1; i < len(attr.Clients); i++ {
		prev, curr := attr.Clients[i-1], attr.Clients[i]
		if prev.Events == curr.Events && prev.Client > curr.Client {
			t.Errorf("clients not stably ordered: %q before %q at equal counts", prev.Client, curr.Client)
		}
		if prev.Events < curr.Events {
			t.Errorf("clients not sorted by events desc: %d before %d", prev.Events, curr.Events)
		}
	}

	// Bounded output.
	if len(attr.Clients) > 20 {
		t.Errorf("len(Clients) = %d, want <= 20", len(attr.Clients))
	}
	if len(attr.RepeatedFailures) > 10 {
		t.Errorf("len(RepeatedFailures) = %d, want <= 10", len(attr.RepeatedFailures))
	}
	if len(attr.SlowOperations) > 5 {
		t.Errorf("len(SlowOperations) = %d, want <= 5", len(attr.SlowOperations))
	}
}

func TestAttributionNoContentLeaks(t *testing.T) {
	now := time.Now()
	events := []model.AccessEvent{
		{ID: 1, OccurredAt: now, ClientName: "manual-cli", SessionID: "", Operation: "nav.search", Success: false, LatencyMs: 100, Error: "raw error text with C:/Users/felipe secret path", ErrorCode: "search_failed"},
		{ID: 2, OccurredAt: now, ClientName: "claude", SessionID: "s1", Operation: "nav.search", Success: false, LatencyMs: 120, Error: "another raw error"},
		{ID: 3, OccurredAt: now, ClientName: "claude", SessionID: "s1", Operation: "nav.search", Success: false, LatencyMs: 120},
	}

	attr := ComputeExportSummary(events).Attribution
	if attr == nil {
		t.Fatal("expected attribution to be computed")
	}
	body, err := json.Marshal(attr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rendered := string(body)
	if strings.Contains(rendered, "secret") || strings.Contains(rendered, "C:/Users") {
		t.Errorf("attribution leaks raw error text: %s", rendered)
	}
	if strings.Contains(rendered, "raw error") {
		t.Errorf("attribution leaks raw error text: %s", rendered)
	}
	// Repeated failures carry stable error codes, never free-text errors.
	for _, rf := range attr.RepeatedFailures {
		if strings.Contains(rf.ErrorCode, " ") {
			t.Errorf("repeated failure carries free-text error: %q", rf.ErrorCode)
		}
	}
}
