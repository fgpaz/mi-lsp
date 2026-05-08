package daemon

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func BenchmarkRecordAccessDirectTelemetry(b *testing.B) {
	store := benchmarkTelemetryStore(b)
	defer store.Close()
	event := model.AccessEvent{
		OccurredAt:     time.Now(),
		ClientName:     "bench-cli",
		Workspace:      "mi-lsp",
		WorkspaceInput: ".",
		WorkspaceRoot:  "C:/repos/mios/mi-lsp",
		WorkspaceAlias: "mi-lsp",
		Operation:      "nav.search",
		Backend:        "text",
		Route:          "direct_text",
		Success:        true,
		LatencyMs:      7,
		TokenBudget:    4000,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		event.OccurredAt = time.Now()
		if err := store.RecordAccessDirect(event); err != nil {
			b.Fatalf("RecordAccessDirect: %v", err)
		}
	}
}

func benchmarkTelemetryStore(b *testing.B) *TelemetryStore {
	b.Helper()
	db, err := sql.Open("sqlite", filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	store := &TelemetryStore{db: db}
	if err := store.enableWALMode(); err != nil {
		b.Fatal(err)
	}
	if err := store.initSchema(); err != nil {
		b.Fatal(err)
	}
	return store
}
