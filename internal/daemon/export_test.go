package daemon

import (
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestClientCohortTreatsBenchAsTestAndManualCLIAsUnknown(t *testing.T) {
	tests := []struct {
		name      string
		client    string
		session   string
		operation string
		want      string
	}{
		{name: "bench operation", client: "cursor", session: "bench-session", operation: "bench", want: "test"},
		{name: "manual cli", client: "manual-cli", session: "manual-session", operation: "nav.refs", want: "unknown"},
		{name: "blank client", session: "session", operation: "nav.refs", want: "unknown"},
		{name: "ordinary session", client: "cursor", session: "agent-session", operation: "nav.refs", want: "work"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := clientCohort(test.client, test.session, test.operation); got != test.want {
				t.Fatalf("clientCohort() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSummaryAttributionDoesNotCountBenchOrManualCLIAsWork(t *testing.T) {
	accumulator := newSummaryAccumulator()
	accumulator.add(model.AccessEvent{Operation: "bench", ClientName: "cursor", SessionID: "bench-session", Success: true})
	accumulator.add(model.AccessEvent{Operation: "nav.refs", ClientName: "manual-cli", SessionID: "manual-session", Success: true})

	if got := accumulator.attr.cohortCounts["test"]; got != 1 {
		t.Fatalf("test cohort count = %d, want 1", got)
	}
	if got := accumulator.attr.cohortCounts["unknown"]; got != 1 {
		t.Fatalf("unknown cohort count = %d, want 1", got)
	}
	if got := accumulator.attr.cohortCounts["work"]; got != 0 {
		t.Fatalf("work cohort count = %d, want 0", got)
	}
	if got := accumulator.attr.classCounts["unknown"]; got != 1 {
		t.Fatalf("unknown client class count = %d, want 1", got)
	}
}
