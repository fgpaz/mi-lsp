package telemetry

import (
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestBuildUsageReportFollowThroughAndFallback(t *testing.T) {
	start := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	events := []model.AccessEvent{
		{
			OccurredAt: start, SessionID: "s", Seq: 1, ClientName: "claude-code", Operation: "nav.intent",
			Success: true, LatencyMs: 10, ResultCount: 1,
			DecisionJSON: `{"continuation_op":"nav.search","fallback_reason_code":"unsupported_operation"}`,
		},
		{
			OccurredAt: start.Add(time.Second), SessionID: "s", Seq: 2, ClientName: "claude-code", Operation: "nav.search",
			Success: true, LatencyMs: 40, ResultCount: 0,
		},
		{
			OccurredAt: start.Add(2 * time.Second), SessionID: "s", Seq: 3, ClientName: "pi-chief", Operation: "nav.find",
			Success: false, LatencyMs: 90, ResultCount: 0, Truncated: true,
			DecisionJSON: `{"continuation_op":"nav.refs","stale_graph":true,"fallback_reason_code":"invalid_workspace"}`,
		},
		{
			OccurredAt: start.Add(3 * time.Second), SessionID: "s", Seq: 4, ClientName: "pi-chief", Operation: "nav.overview",
			Success: true, LatencyMs: 20, ResultCount: 2,
		},
	}
	report := BuildUsageReport(events)
	if report.FollowThrough.Followed != 1 || report.FollowThrough.Ignored != 1 || report.FollowThrough.NoneEmitted != 2 {
		t.Fatalf("follow = %+v", report.FollowThrough)
	}
	if report.FallbackByReason["unsupported_operation"] != 1 || report.FallbackByReason["invalid_workspace"] != 1 {
		t.Fatalf("fallback = %+v", report.FallbackByReason)
	}
	if report.ByHarness["claude-code"] != 2 || report.ByHarness["pi-chief"] != 2 {
		t.Fatalf("harness = %+v", report.ByHarness)
	}
	if report.EmptyResults < 1 || report.PartialResults < 1 || report.StaleGraphHits < 1 {
		t.Fatalf("signals empty=%d partial=%d stale=%d", report.EmptyResults, report.PartialResults, report.StaleGraphHits)
	}
	if report.LatencyP50Ms != 20 && report.LatencyP50Ms != 40 {
		t.Fatalf("p50 = %d", report.LatencyP50Ms)
	}
}

func TestUsageSince(t *testing.T) {
	now := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	got, err := UsageSince("7d", now)
	if err != nil || !got.Equal(now.Add(-7*24*time.Hour)) {
		t.Fatalf("since = %v %v", got, err)
	}
}
