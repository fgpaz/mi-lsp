package daemon

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/telemetry"
)

func daemonRootDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".mi-lsp", "daemon")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func daemonStatePath() (string, error) {
	dir, err := daemonRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

func daemonDatabasePath() (string, error) {
	dir, err := daemonRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "daemon.db"), nil
}

func daemonLockPath() (string, error) {
	dir, err := daemonRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "start.lock"), nil
}

type startLock struct {
	path string
	file *os.File
}

func acquireStartLock(timeout time.Duration) (*startLock, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	path, err := daemonLockPath()
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for {
		file, openErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if openErr == nil {
			return &startLock{path: path, file: file}, nil
		}
		if !errors.Is(openErr, os.ErrExist) {
			return nil, openErr
		}
		if time.Now().After(deadline) {
			return nil, errors.New("timed out waiting for daemon start lock")
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func (l *startLock) Close() error {
	if l == nil {
		return nil
	}
	if l.file != nil {
		_ = l.file.Close()
	}
	if l.path == "" {
		return nil
	}
	if err := os.Remove(l.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func loadDaemonState() (model.DaemonState, error) {
	path, err := daemonStatePath()
	if err != nil {
		return model.DaemonState{}, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return model.DaemonState{}, err
	}
	var state model.DaemonState
	if err := json.Unmarshal(body, &state); err != nil {
		return model.DaemonState{}, err
	}
	return state, nil
}

func saveDaemonState(state model.DaemonState) error {
	path, err := daemonStatePath()
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func clearDaemonState() error {
	path, err := daemonStatePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

type TelemetryStore struct {
	db *sql.DB
}

func openTelemetryStore() (*TelemetryStore, error) {
	path, err := daemonDatabasePath()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &TelemetryStore{db: db}
	if err := store.enableWALMode(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func OpenTelemetryStore() (*TelemetryStore, error) {
	return openTelemetryStore()
}

func (s *TelemetryStore) enableWALMode() error {
	_, err := s.db.Exec("PRAGMA journal_mode=WAL")
	return err
}

func (s *TelemetryStore) PurgeOldEvents(olderThan time.Time) (int64, error) {
	result, err := s.db.Exec(`DELETE FROM access_events WHERE occurred_at < ?`, olderThan.Unix())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *TelemetryStore) PurgeOldRuns(olderThan time.Time) (int64, error) {
	result, err := s.db.Exec(`DELETE FROM daemon_runs WHERE stopped_at IS NOT NULL AND stopped_at < ?`, olderThan.Unix())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *TelemetryStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *TelemetryStore) initSchema() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS daemon_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			pid INTEGER NOT NULL,
			endpoint TEXT NOT NULL,
			admin_url TEXT,
			repo_root TEXT,
			protocol_version TEXT,
			started_at INTEGER NOT NULL,
			stopped_at INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS runtime_snapshots (
			runtime_key TEXT PRIMARY KEY,
			daemon_run_id INTEGER,
			workspace_root TEXT,
			workspace_name TEXT,
			repo_name TEXT,
			repo_root TEXT,
			backend_type TEXT,
			entrypoint_id TEXT,
			entrypoint_path TEXT,
			entrypoint_type TEXT,
			pid INTEGER,
			memory_bytes INTEGER,
			started_at INTEGER,
			last_used_at INTEGER,
			status TEXT,
			FOREIGN KEY(daemon_run_id) REFERENCES daemon_runs(id)
		)`,
		`CREATE TABLE IF NOT EXISTS access_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			daemon_run_id INTEGER,
			occurred_at INTEGER NOT NULL,
			client_name TEXT,
			session_id TEXT,
			workspace TEXT,
			workspace_input TEXT,
			workspace_root TEXT,
			workspace_alias TEXT,
			repo TEXT,
			operation TEXT NOT NULL,
			backend TEXT,
			success INTEGER NOT NULL,
			latency_ms INTEGER,
			warnings_json TEXT,
			runtime_key TEXT,
			entrypoint_id TEXT,
			error_text TEXT,
			error_kind TEXT,
			error_code TEXT,
			truncated INTEGER DEFAULT 0,
			result_count INTEGER DEFAULT 0,
			FOREIGN KEY(daemon_run_id) REFERENCES daemon_runs(id)
		)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
	for _, migration := range []string{
		`ALTER TABLE runtime_snapshots ADD COLUMN repo_name TEXT`,
		`ALTER TABLE runtime_snapshots ADD COLUMN repo_root TEXT`,
		`ALTER TABLE runtime_snapshots ADD COLUMN entrypoint_id TEXT`,
		`ALTER TABLE runtime_snapshots ADD COLUMN entrypoint_path TEXT`,
		`ALTER TABLE runtime_snapshots ADD COLUMN entrypoint_type TEXT`,
		`ALTER TABLE access_events ADD COLUMN repo TEXT`,
		`ALTER TABLE access_events ADD COLUMN workspace_input TEXT`,
		`ALTER TABLE access_events ADD COLUMN workspace_root TEXT`,
		`ALTER TABLE access_events ADD COLUMN workspace_alias TEXT`,
		`ALTER TABLE access_events ADD COLUMN entrypoint_id TEXT`,
		`ALTER TABLE access_events ADD COLUMN error_kind TEXT`,
		`ALTER TABLE access_events ADD COLUMN error_code TEXT`,
		`ALTER TABLE access_events ADD COLUMN truncated INTEGER DEFAULT 0`,
		`ALTER TABLE access_events ADD COLUMN result_count INTEGER DEFAULT 0`,
	} {
		_, _ = s.db.Exec(migration)
	}
	return nil
}

func (s *TelemetryStore) StartRun(state model.DaemonState) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO daemon_runs(pid, endpoint, admin_url, repo_root, protocol_version, started_at) VALUES (?, ?, ?, ?, ?, ?)`,
		state.PID,
		state.Endpoint,
		state.AdminURL,
		state.RepoRoot,
		state.ProtocolVersion,
		state.StartedAt.Unix(),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *TelemetryStore) StopRun(runID int64, stoppedAt time.Time) error {
	if runID == 0 {
		return nil
	}
	_, err := s.db.Exec(`UPDATE daemon_runs SET stopped_at = ? WHERE id = ?`, stoppedAt.Unix(), runID)
	return err
}

func (s *TelemetryStore) ReplaceRuntimeSnapshots(runID int64, statuses []model.WorkerStatus) error {
	if runID == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM runtime_snapshots WHERE daemon_run_id = ?`, runID); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO runtime_snapshots(runtime_key, daemon_run_id, workspace_root, workspace_name, repo_name, repo_root, backend_type, entrypoint_id, entrypoint_path, entrypoint_type, pid, memory_bytes, started_at, last_used_at, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, status := range statuses {
		if _, err := stmt.Exec(
			status.RuntimeKey,
			runID,
			status.WorkspaceRoot,
			status.Workspace,
			status.RepoName,
			status.RepoRoot,
			status.BackendType,
			status.EntrypointID,
			status.EntrypointPath,
			status.EntrypointType,
			status.PID,
			status.MemoryBytes,
			status.StartedAt.Unix(),
			status.LastUsedAt.Unix(),
			"active",
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *TelemetryStore) RecordAccessDirect(event model.AccessEvent) error {
	normalized := telemetry.NormalizeAccessEvent(event)
	warningsJSON := "[]"
	if len(normalized.Warnings) > 0 {
		body, err := json.Marshal(normalized.Warnings)
		if err != nil {
			return err
		}
		warningsJSON = string(body)
	}
	_, err := s.db.Exec(
		`INSERT INTO access_events(daemon_run_id, occurred_at, client_name, session_id, workspace, workspace_input, workspace_root, workspace_alias, repo, operation, backend, success, latency_ms, warnings_json, runtime_key, entrypoint_id, error_text, error_kind, error_code, truncated, result_count) VALUES (NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		normalized.OccurredAt.Unix(),
		normalized.ClientName,
		normalized.SessionID,
		normalized.Workspace,
		normalized.WorkspaceInput,
		normalized.WorkspaceRoot,
		normalized.WorkspaceAlias,
		normalized.Repo,
		normalized.Operation,
		normalized.Backend,
		boolToInt(normalized.Success),
		normalized.LatencyMs,
		warningsJSON,
		normalized.RuntimeKey,
		normalized.EntrypointID,
		normalized.Error,
		normalized.ErrorKind,
		normalized.ErrorCode,
		boolToInt(normalized.Truncated),
		normalized.ResultCount,
	)
	return err
}

func (s *TelemetryStore) RecordAccess(runID int64, event model.AccessEvent) error {
	normalized := telemetry.NormalizeAccessEvent(event)
	warningsJSON := "[]"
	if len(normalized.Warnings) > 0 {
		body, err := json.Marshal(normalized.Warnings)
		if err != nil {
			return err
		}
		warningsJSON = string(body)
	}
	_, err := s.db.Exec(
		`INSERT INTO access_events(daemon_run_id, occurred_at, client_name, session_id, workspace, workspace_input, workspace_root, workspace_alias, repo, operation, backend, success, latency_ms, warnings_json, runtime_key, entrypoint_id, error_text, error_kind, error_code, truncated, result_count) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		runID,
		normalized.OccurredAt.Unix(),
		normalized.ClientName,
		normalized.SessionID,
		normalized.Workspace,
		normalized.WorkspaceInput,
		normalized.WorkspaceRoot,
		normalized.WorkspaceAlias,
		normalized.Repo,
		normalized.Operation,
		normalized.Backend,
		boolToInt(normalized.Success),
		normalized.LatencyMs,
		warningsJSON,
		normalized.RuntimeKey,
		normalized.EntrypointID,
		normalized.Error,
		normalized.ErrorKind,
		normalized.ErrorCode,
		boolToInt(normalized.Truncated),
		normalized.ResultCount,
	)
	return err
}

func (s *TelemetryStore) RecentAccesses(limit int) ([]model.AccessEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, occurred_at, COALESCE(client_name, ''), COALESCE(session_id, ''), COALESCE(workspace, ''), COALESCE(workspace_input, ''), COALESCE(workspace_root, ''), COALESCE(workspace_alias, ''), COALESCE(repo, ''), operation, COALESCE(backend, ''), success, latency_ms, COALESCE(warnings_json, '[]'), COALESCE(runtime_key, ''), COALESCE(entrypoint_id, ''), COALESCE(error_text, ''), COALESCE(error_kind, ''), COALESCE(error_code, ''), COALESCE(truncated, 0), COALESCE(result_count, 0) FROM access_events ORDER BY occurred_at DESC, id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.AccessEvent, 0, limit)
	for rows.Next() {
		item, err := scanAccessEvent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type metricsRow struct {
	operation  string
	workspace  string
	clientName string
	latencyMs  int64
	success    bool
	truncated  bool
}

func (s *TelemetryStore) ComputeMetrics(since time.Time) ([]metricsRow, error) {
	rows, err := s.db.Query(
		`SELECT operation, COALESCE(NULLIF(workspace_root, ''), NULLIF(workspace, ''), ''), COALESCE(client_name, ''), latency_ms, success, COALESCE(truncated, 0)
		 FROM access_events
		 WHERE occurred_at > ?
		   AND operation NOT LIKE 'system.%'
		 ORDER BY operation, latency_ms`,
		since.Unix(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []metricsRow
	for rows.Next() {
		var item metricsRow
		var success, truncated int
		if err := rows.Scan(&item.operation, &item.workspace, &item.clientName, &item.latencyMs, &success, &truncated); err != nil {
			return nil, err
		}
		item.success = success == 1
		item.truncated = truncated == 1
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanAccessEvent(rows *sql.Rows) (model.AccessEvent, error) {
	var (
		item         model.AccessEvent
		occurredAt   int64
		success      int
		warningsJSON string
		truncated    int
		resultCount  int
	)
	if err := rows.Scan(&item.ID, &occurredAt, &item.ClientName, &item.SessionID, &item.Workspace, &item.WorkspaceInput, &item.WorkspaceRoot, &item.WorkspaceAlias, &item.Repo, &item.Operation, &item.Backend, &success, &item.LatencyMs, &warningsJSON, &item.RuntimeKey, &item.EntrypointID, &item.Error, &item.ErrorKind, &item.ErrorCode, &truncated, &resultCount); err != nil {
		return model.AccessEvent{}, err
	}
	item.OccurredAt = time.Unix(occurredAt, 0)
	item.Success = success == 1
	item.Truncated = truncated == 1
	item.ResultCount = resultCount
	if warningsJSON != "" {
		_ = json.Unmarshal([]byte(warningsJSON), &item.Warnings)
	}
	return telemetry.NormalizeAccessEvent(item), nil
}
