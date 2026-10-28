package daemon

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestSystemStatusTelemetryWAL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("USERNAME", fmt.Sprintf("aorph-wal-health-%d-%d", os.Getpid(), time.Now().UnixNano()))
	options := DefaultStartOptions()
	options.WatchMode = WatchModeOff
	server, err := NewServerWithOptions(t.TempDir(), 1, time.Minute, options)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Shutdown()
	store := server.telemetry

	status := func() map[string]any {
		t.Helper()
		env, err := server.handleRequest(model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "system.status"})
		if err != nil || !env.Ok {
			t.Fatalf("status: %+v, %v", env, err)
		}
		items, ok := env.Items.([]map[string]any)
		if !ok || len(items) != 1 {
			t.Fatalf("unexpected items: %#v", env.Items)
		}
		for _, key := range []string{"state", "daemon_process", "watchers", "active_runtimes", "recent_accesses", "result_cache", "elastic_pool"} {
			if _, ok := items[0][key]; !ok {
				t.Fatalf("missing existing field %s", key)
			}
		}
		wal, ok := items[0]["telemetry_wal"].(map[string]any)
		if !ok {
			t.Fatalf("missing telemetry_wal: %#v", items[0])
		}
		return wal
	}

	event := model.AccessEvent{Operation: "oversized", Error: strings.Repeat("x", telemetryMaxEventBytes+1)}
	if err := store.RecordAccess(1, event); err == nil {
		t.Fatal("oversized event accepted")
	}
	before := store.db.Stats()
	wal := status()
	after := store.db.Stats()
	if wal["wal_bytes_known"] != true || wal["closed"] != false || wal["admission_threshold_bytes"] != telemetryWALMaxBytes || wal["rejected_writes"] != uint64(1) || wal["failed_writes"] != uint64(0) || wal["last_failure"] != "event_too_large" || wal["last_write_failure"] != "event_too_large" || wal["checkpoint_failure"] != "" || wal["wal_size_failure"] != "" || wal["checkpoint_busy"] != false {
		t.Fatalf("unexpected WAL health: %#v", wal)
	}
	if before.OpenConnections != after.OpenConnections {
		t.Fatalf("status changed open connections: %v -> %v", before, after)
	}

	// Holding the sole private connection forces the real PRAGMA refresh to time out.
	conn, err := store.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	health := store.WALHealth()
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if health.WALBytesKnown || health.LastFailure != "event_too_large" || health.LastWriteFailure != "event_too_large" || health.WALSizeFailure != "wal_size_unavailable" || health.RejectedWrites != 1 || health.FailedWrites != 0 {
		t.Fatalf("failed refresh must report unknown bytes without masking write history: %+v", health)
	}
	recovered := store.WALHealth()
	if !recovered.WALBytesKnown || recovered.WALSizeFailure != "" || recovered.LastFailure != "event_too_large" || recovered.LastWriteFailure != "event_too_large" {
		t.Fatalf("recovered refresh must clear measurement failure, not write history: %+v", recovered)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	wal = status()
	if wal["closed"] != true || wal["wal_bytes_known"] != false {
		t.Fatalf("closed WAL: %#v", wal)
	}
	server.telemetry = nil
	wal = status()
	if wal["closed"] != true || wal["wal_bytes_known"] != false {
		t.Fatalf("nil WAL: %#v", wal)
	}
	server.telemetry = store
}
