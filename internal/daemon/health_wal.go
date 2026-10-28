package daemon

// telemetryWALStatus reports the admission threshold, not a physical WAL cap.
func telemetryWALStatus(store *TelemetryStore) map[string]any {
	health := store.WALHealth()
	return map[string]any{
		"wal_bytes":                 health.WALBytes,
		"wal_bytes_known":           health.WALBytesKnown,
		"admission_threshold_bytes": health.WALMaxBytes,
		"checkpoint_busy":           health.CheckpointBusy,
		"last_failure":              health.LastFailure,
		"last_write_failure":        health.LastWriteFailure,
		"checkpoint_failure":        health.CheckpointFailure,
		"wal_size_failure":          health.WALSizeFailure,
		"rejected_writes":           health.RejectedWrites,
		"failed_writes":             health.FailedWrites,
		"closed":                    health.Closed,
	}
}
