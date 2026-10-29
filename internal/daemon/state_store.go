package daemon

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"modernc.org/sqlite"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/runtimeinfo"
	"github.com/fgpaz/mi-lsp/internal/telemetry"
)

const (
	telemetrySQLiteRetryAttempts = 4
	telemetrySQLiteRetryDelay    = 25 * time.Millisecond
	telemetryWALMaxBytes         = int64(16 << 20)
	telemetryMaxEventBytes       = 1 << 20
	telemetryCheckpointInterval  = time.Minute
	telemetryCloseDrainTimeout   = 2 * time.Second
	telemetryCheckpointTimeout   = 250 * time.Millisecond
	telemetryCheckpointBusyWait  = 25 * time.Millisecond
)

func daemonRootDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".mi-lsp", "daemon")
	// SEC-04: Create daemon directory with restricted permissions (0o700) to prevent other users from reading admin_token, PID, etc.
	if err := os.MkdirAll(dir, 0o700); err != nil {
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
	// SEC-04: Save daemon state with restricted permissions (0o600) to protect admin_token and other sensitive data
	return os.WriteFile(path, body, 0o600)
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

// telemetryWriteRequest represents an async write operation to the telemetry store.
type telemetryWriteRequest struct {
	runID    int64
	event    model.AccessEvent
	isDirect bool // true if recording directly without runID
}

var (
	errTelemetryWALLimit       = errors.New("telemetry WAL limit reached")
	errTelemetryCheckpointBusy = errors.New("wal_checkpoint busy")
)

type WALHealth struct {
	WALBytes          int64  `json:"wal_bytes"`
	WALBytesKnown     bool   `json:"wal_bytes_known"`
	WALMaxBytes       int64  `json:"wal_max_bytes"`
	CheckpointBusy    bool   `json:"checkpoint_busy"`
	LastFailure       string `json:"last_failure,omitempty"`
	LastWriteFailure  string `json:"last_write_failure,omitempty"`
	CheckpointFailure string `json:"checkpoint_failure,omitempty"`
	WALSizeFailure    string `json:"wal_size_failure,omitempty"`
	RejectedWrites    uint64 `json:"rejected_writes"`
	FailedWrites      uint64 `json:"failed_writes"`
	Closed            bool   `json:"closed"`
}

type TelemetryStore struct {
	dbPath        string
	db            *sql.DB
	writeQueue    chan telemetryWriteRequest
	done          chan struct{}
	wg            sync.WaitGroup
	admissionMu   sync.Mutex
	ownershipOnce sync.Once
	ownership     chan struct{}
	stateMu       sync.RWMutex
	closed        bool
	health        WALHealth
	walMaxBytes   int64
	ownerOnce     sync.Once
	writeCtx      context.Context
	cancelWrites  context.CancelFunc
	closeMu       sync.Mutex
	closeRunning  bool
	closePhysical bool
	closeAttempt  chan struct{}
	doneOnce      sync.Once
	drainOnce     sync.Once
	drainTimer    *time.Timer
	closeComplete chan struct{}
	closeErr      error
}

func openTelemetryStore() (opened *TelemetryStore, retErr error) {
	path, err := daemonDatabasePath()
	if err != nil {
		return nil, err
	}
	canonicalPath, err := canonicalTelemetryDatabasePath(path)
	if err != nil {
		return nil, err
	}
	guardLease, err := runtimeinfo.AcquireFileLeaseContext(context.Background(), canonicalPath+".writer.guard")
	if err != nil {
		return nil, fmt.Errorf("acquire telemetry schema lease: %w", err)
	}
	guardReleased := false
	defer func() {
		if !guardReleased {
			retErr = errors.Join(retErr, wrapTelemetryLeaseRelease(guardLease.Release()))
		}
	}()
	existingDB, err := inspectTelemetryDatabaseAfterLease(path, canonicalPath)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", canonicalPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &TelemetryStore{
		dbPath:      canonicalPath,
		db:          db,
		writeQueue:  make(chan telemetryWriteRequest, 256),
		done:        make(chan struct{}),
		walMaxBytes: telemetryWALMaxBytes,
	}
	failOpen := func(cause error) (*TelemetryStore, error) {
		return nil, errors.Join(cause, db.Close())
	}
	if existingDB {
		if err := store.admitTelemetryMutationWithLeaseHeldContext(context.Background()); err != nil {
			return failOpen(fmt.Errorf("admit telemetry open/schema: %w", err))
		}
	}
	if err := store.configureConnection(); err != nil {
		return failOpen(err)
	}
	if err := store.initSchema(); err != nil {
		return failOpen(err)
	}
	if err := guardLease.Release(); err != nil {
		guardReleased = true // Release consumes/unlocks its handle even on error.
		wrappedReleaseErr := wrapTelemetryLeaseRelease(err)
		reowned, acquireErr := runtimeinfo.AcquireFileLeaseContext(context.Background(), canonicalPath+".writer.guard")
		if acquireErr != nil {
			// Return the live store with its error so the caller retains a handle
			// and can retry physical cleanup; never close outside proven ownership.
			return store, errors.Join(wrappedReleaseErr, fmt.Errorf("reacquire telemetry lease for failed-open cleanup: %w", acquireErr))
		}
		cleanupPath, pathErr := canonicalTelemetryDatabasePath(path)
		if pathErr == nil && cleanupPath != canonicalPath {
			pathErr = fmt.Errorf("telemetry database identity changed before failed-open cleanup: %q != %q", cleanupPath, canonicalPath)
		}
		if pathErr != nil {
			releaseErr := reowned.Release()
			return store, errors.Join(wrappedReleaseErr, pathErr, wrapTelemetryLeaseRelease(releaseErr))
		}
		closeErr := db.Close()
		reacquireReleaseErr := reowned.Release()
		return nil, errors.Join(wrappedReleaseErr, closeErr, wrapTelemetryLeaseRelease(reacquireReleaseErr))
	}
	guardReleased = true
	// Start async write worker (PERF-06).
	store.wg.Add(1)
	go store.asyncWriteWorker()
	return store, nil
}

// OpenTelemetryStore joins any partially opened store before returning an open error.
func OpenTelemetryStore() (*TelemetryStore, error) {
	store, err := openTelemetryStore()
	return cleanupPartialTelemetryStoreResult(store, err)
}

func (s *TelemetryStore) enableWALMode() error {
	var mode string
	return s.db.QueryRow("PRAGMA journal_mode=WAL").Scan(&mode)
}

func (s *TelemetryStore) configureConnection() error {
	for _, pragma := range []string{
		"PRAGMA busy_timeout=250",
		"PRAGMA synchronous=NORMAL",
	} {
		if _, err := s.db.Exec(pragma); err != nil {
			return err
		}
	}
	return s.enableWALMode()
}

func retryTelemetrySQLite(fn func() error) error {
	return retryTelemetrySQLiteContext(context.Background(), fn)
}

func retryTelemetrySQLiteContext(ctx context.Context, fn func() error) error {
	var err error
	delay := telemetrySQLiteRetryDelay
	for attempt := 0; attempt < telemetrySQLiteRetryAttempts; attempt++ {
		if err = ctx.Err(); err != nil {
			return err
		}
		err = fn()
		if !isTelemetrySQLiteLocked(err) {
			return err
		}
		if attempt == telemetrySQLiteRetryAttempts-1 {
			break
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		delay *= 2
	}
	return err
}

func isTelemetrySQLiteLocked(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "database is busy") ||
		strings.Contains(message, "sqlite_busy") ||
		strings.Contains(message, "sqlite_locked")
}

func (s *TelemetryStore) execWithRetry(query string, args ...any) (sql.Result, error) {
	return s.execWithRetryContext(s.ownedWriteContext(), query, args...)
}

func (s *TelemetryStore) execWithRetryContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	var result sql.Result
	err := retryTelemetrySQLiteContext(ctx, func() error {
		var execErr error
		result, execErr = s.db.ExecContext(ctx, query, args...)
		return execErr
	})
	return result, err
}

// PurgeAndVacuum enforces telemetry retention and reclaims space (PERF-05).
//
// If the database exceeds maxBytes, aggressively purges to 7 days.
// Otherwise, respects retentionDays. Always runs VACUUM to reclaim space.
//
// Returns (purged_count, vacuum_success, error).
func (s *TelemetryStore) PurgeAndVacuum(retentionDays int, maxBytes int64) (int, bool, error) {
	return s.purgeAndVacuumContext(s.ownedWriteContext(), retentionDays, maxBytes)
}

// dbSize returns the current database file size in bytes.
func (s *TelemetryStore) dbSize() (int64, error) {
	path, err := daemonDatabasePath()
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (s *TelemetryStore) PurgeOldEvents(olderThan time.Time) (int64, error) {
	return s.purgeOldEventsContext(s.ownedWriteContext(), olderThan)
}

func (s *TelemetryStore) PurgeOldRuns(olderThan time.Time) (int64, error) {
	return s.purgeOldRunsContext(s.ownedWriteContext(), olderThan)
}

// asyncWriteWorker is the single consumer goroutine that processes telemetry writes.
// This eliminates "database is locked" errors under concurrent write load (PERF-06).
func (s *TelemetryStore) asyncWriteWorker() {
	defer s.wg.Done()
	ticker := time.NewTicker(telemetryCheckpointInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			for {
				select {
				case req := <-s.writeQueue:
					s.persistQueued(req)
				case <-s.ownedWriteContext().Done():
					for {
						select {
						case <-s.writeQueue:
							s.failWrite()
						default:
							return
						}
					}
				default:
					return
				}
			}
		case req := <-s.writeQueue:
			s.persistQueued(req)
		case <-ticker.C:
			if len(s.writeQueue) == 0 {
				_ = s.Checkpoint()
			}
		}
	}
}

func (s *TelemetryStore) persistQueued(req telemetryWriteRequest) {
	if err := s.ownedWriteContext().Err(); err != nil {
		s.failWrite()
		return
	}
	// isDirect describes the event's run association, not queue admission.
	err := s.withTelemetryMutationDrainContext(s.ownedWriteContext(), true, func(ctx context.Context) error {
		return s.recordAccessDirectUnderLease(ctx, req.runID, req.event, req.isDirect)
	})
	if err != nil {
		if isTelemetryAdmissionError(err) {
			s.failWriteReason(walAdmissionReason(err))
		} else {
			s.failWrite()
		}
	}
}

// recordAccessDirectInternal admits a synchronous public write.
func (s *TelemetryStore) recordAccessDirectInternal(runID int64, event model.AccessEvent, isDirect bool) error {
	return s.withTelemetryMutationContext(s.ownedWriteContext(), func(ctx context.Context) error {
		return s.recordAccessDirectUnderLease(ctx, runID, event, isDirect)
	})
}

func (s *TelemetryStore) recordAccessDirectUnderLease(ctx context.Context, runID int64, event model.AccessEvent, isDirect bool) error {
	if event.Seq == 0 {
		var err error
		event.Seq, err = s.nextSeqWithOwnershipContext(ctx, event.SessionID)
		if err != nil {
			return err
		}
	}
	normalized := telemetry.NormalizeAccessEvent(event)
	warningsJSON := "[]"
	if len(normalized.Warnings) > 0 {
		body, err := json.Marshal(normalized.Warnings)
		if err != nil {
			return err
		}
		warningsJSON = string(body)
	}
	args := []any{
		normalized.OccurredAt.Unix(),
		normalized.ClientName,
		normalized.SessionID,
		normalized.Seq,
		normalized.Workspace,
		normalized.WorkspaceInput,
		normalized.WorkspaceRoot,
		normalized.WorkspaceAlias,
		normalized.Repo,
		normalized.Operation,
		normalized.Backend,
		normalized.Route,
		normalized.Format,
		normalized.TokenBudget,
		normalized.MaxItems,
		normalized.MaxChars,
		boolToInt(normalized.Compress),
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
		normalized.WarningCount,
		normalized.PatternMode,
		normalized.RoutingOutcome,
		normalized.FailureStage,
		normalized.HintCode,
		normalized.TruncationReason,
		normalized.DecisionJSON,
		normalized.BytesOut,
		normalized.Harness,
		normalized.QStages,
	}
	if isDirect {
		_, err := s.execWithRetryContext(ctx,
			`INSERT INTO access_events(daemon_run_id, occurred_at, client_name, session_id, seq, workspace, workspace_input, workspace_root, workspace_alias, repo, operation, backend, route, format, token_budget, max_items, max_chars, compress, success, latency_ms, warnings_json, runtime_key, entrypoint_id, error_text, error_kind, error_code, truncated, result_count, warning_count, pattern_mode, routing_outcome, failure_stage, hint_code, truncation_reason, decision_json, bytes_out, harness, q_stages) VALUES (NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			args...,
		)
		return err
	} else {
		_, err := s.execWithRetryContext(ctx,
			`INSERT INTO access_events(daemon_run_id, occurred_at, client_name, session_id, seq, workspace, workspace_input, workspace_root, workspace_alias, repo, operation, backend, route, format, token_budget, max_items, max_chars, compress, success, latency_ms, warnings_json, runtime_key, entrypoint_id, error_text, error_kind, error_code, truncated, result_count, warning_count, pattern_mode, routing_outcome, failure_stage, hint_code, truncation_reason, decision_json, bytes_out, harness, q_stages) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			append([]any{runID}, args...)...,
		)
		return err
	}
}

func (s *TelemetryStore) Checkpoint() error {
	if s == nil || s.db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(s.ownedWriteContext(), telemetryCheckpointTimeout)
	defer cancel()
	return s.withTelemetryOwnershipOnlyContext(ctx, func(ownedCtx context.Context) error {
		return s.checkpointWithLeaseHeldContext(ownedCtx)
	})
}

func (s *TelemetryStore) checkpointWithLeaseHeldContext(ctx context.Context) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			s.setFailure("checkpoint_timeout", false)
		} else {
			s.setFailure("checkpoint_error", false)
		}
		return fmt.Errorf("wal_checkpoint connection: %w", err)
	}
	defer conn.Close()
	var originalBusyTimeout int
	if err := conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&originalBusyTimeout); err != nil {
		s.setFailure("checkpoint_error", false)
		return fmt.Errorf("wal_checkpoint read busy_timeout: %w", err)
	}
	// Keep the busy-handler wait well below the overall context budget. Otherwise
	// SQLite's configured wait can consume the operation deadline and hide BUSY.
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout=%d", telemetryCheckpointBusyWait.Milliseconds())); err != nil {
		// The driver may fail after applying the PRAGMA. Discard uncertain state.
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		s.setFailure("checkpoint_error", false)
		return fmt.Errorf("wal_checkpoint set busy_timeout: %w", err)
	}
	var checkpointErr error
	busyResult := false
	var busy, logFrames, checkpointed int
	if err := conn.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &checkpointed); err != nil {
		checkpointErr = fmt.Errorf("wal_checkpoint: %w", err)
		if isTelemetrySQLiteLocked(err) {
			busyResult = true
			s.setFailure("busy", true)
		} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			s.setFailure("checkpoint_timeout", false)
		} else {
			s.setFailure("checkpoint_error", false)
		}
	} else if busy != 0 {
		busyResult = true
		checkpointErr = fmt.Errorf("%w (log=%d checkpointed=%d)", errTelemetryCheckpointBusy, logFrames, checkpointed)
		s.setFailure("busy", true)
	} else {
		s.setFailure("", false)
	}

	restoreCtx, restoreCancel := context.WithTimeout(context.Background(), telemetryCheckpointBusyWait)
	_, restoreErr := conn.ExecContext(restoreCtx, fmt.Sprintf("PRAGMA busy_timeout=%d", originalBusyTimeout))
	restoreCancel()
	if restoreErr != nil {
		// database/sql discards this physical connection when Raw returns ErrBadConn;
		// never let a connection with unknown PRAGMA state back into the pool.
		discardErr := conn.Raw(func(any) error { return driver.ErrBadConn })
		s.setFailure("checkpoint_restore_error", busyResult)
		return errors.Join(checkpointErr, fmt.Errorf("restore busy_timeout=%d: %w", originalBusyTimeout, restoreErr), discardErr)
	}
	return checkpointErr
}

func (s *TelemetryStore) setFailure(code string, busy bool) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.health.CheckpointFailure = code
	s.health.CheckpointBusy = busy
	s.health.LastFailure = s.health.LastWriteFailure
	if s.health.LastFailure == "" {
		s.health.LastFailure = code
	}
}

func (s *TelemetryStore) walSize() (int64, error) {
	ctx, cancel := context.WithTimeout(s.ownedWriteContext(), telemetryOwnershipBudget)
	defer cancel()
	if s.dbPath != "" {
		return s.walSizeWithContext(ctx)
	}
	var size int64
	err := s.withTelemetryReadOwnershipContext(ctx, func(ownedCtx context.Context) error {
		var measureErr error
		size, measureErr = s.walSizeWithContext(ownedCtx)
		return measureErr
	})
	return size, err
}

func (s *TelemetryStore) walSizeWithContext(ctx context.Context) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	path := s.dbPath
	if path == "" {
		// Compatibility for private legacy fixtures only. Production stores set
		// their canonical path before first SQL, so measurement never opens SQLite.
		var name string
		if err := s.db.QueryRowContext(ctx, "PRAGMA database_list").Scan(new(int), &name, &path); err != nil {
			return 0, err
		}
		if name != "main" || path == "" {
			return 0, errors.New("telemetry database main path unavailable")
		}
	}
	canonicalPath, err := canonicalTelemetryDatabasePath(path)
	if err != nil {
		return 0, err
	}
	dbInfo, err := os.Stat(canonicalPath)
	if err != nil {
		return 0, fmt.Errorf("stat telemetry database for WAL measurement: %w", err)
	}
	if !dbInfo.Mode().IsRegular() {
		return 0, errors.New("telemetry database is not a regular file for WAL measurement")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	info, err := os.Stat(canonicalPath + "-wal")
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (s *TelemetryStore) admitWAL() error {
	admissionEntered := false
	err := s.withTelemetryOwnershipOnlyContext(s.ownedWriteContext(), func(ctx context.Context) error {
		admissionEntered = true
		return s.admitTelemetryMutationWithLeaseHeldContext(ctx)
	})
	if err != nil && !admissionEntered {
		s.reject(walAdmissionReason(err))
	}
	return err
}

func (s *TelemetryStore) WALHealth() WALHealth {
	if s == nil {
		return WALHealth{Closed: true}
	}
	s.stateMu.RLock()
	h := s.health
	h.Closed = s.closed
	s.stateMu.RUnlock()
	h.WALBytes = 0
	h.WALBytesKnown = false
	if !h.Closed {
		size, err := s.walSize()
		s.stateMu.Lock()
		h = s.health
		h.Closed = s.closed
		h.WALBytes = 0
		h.WALBytesKnown = false
		if err == nil && !h.Closed {
			h.WALBytes = size
			h.WALBytesKnown = true
			s.health.WALBytes = size
			s.health.WALBytesKnown = true
		} else if !h.Closed {
			h.WALSizeFailure = "wal_size_unavailable"
			if h.LastFailure == "" {
				h.LastFailure = h.WALSizeFailure
			}
		}
		s.stateMu.Unlock()
	}
	h.WALMaxBytes = s.walMaxBytes
	if h.WALMaxBytes <= 0 {
		h.WALMaxBytes = telemetryWALMaxBytes
	}
	return h
}

func (s *TelemetryStore) ownedWriteContext() context.Context {
	s.ownerOnce.Do(func() {
		s.writeCtx, s.cancelWrites = context.WithCancel(context.Background())
		s.closeComplete = make(chan struct{})
	})
	return s.writeCtx
}

// startDrainDeadline is shared by shutdown and Close; it does not close the DB.
func (s *TelemetryStore) startDrainDeadline() {
	if s == nil || s.db == nil {
		return
	}
	s.ownedWriteContext()
	s.drainOnce.Do(func() {
		s.drainTimer = time.AfterFunc(telemetryCloseDrainTimeout, s.cancelWrites)
	})
}

// Close bounds the caller, not physical ownership. A failed gate acquisition
// leaves closeComplete open and can be retried by a later Close call.
func (s *TelemetryStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	s.startDrainDeadline()
	s.stateMu.Lock()
	s.closed = true
	s.stateMu.Unlock()
	attempt := s.startTelemetryCloseAttempt()
	ctx, cancel := context.WithTimeout(context.Background(), telemetryCloseDrainTimeout)
	defer cancel()
	select {
	case <-s.closeComplete:
		return s.closeResult()
	case <-attempt:
		if s.isTelemetryClosePhysical() {
			return s.closeResult()
		}
		return fmt.Errorf("telemetry close incomplete: %w", s.closeResult())
	case <-ctx.Done():
		return fmt.Errorf("telemetry close incomplete: %w", ctx.Err())
	}
}

func (s *TelemetryStore) startTelemetryCloseAttempt() <-chan struct{} {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closePhysical {
		return s.closeComplete
	}
	if s.closeRunning {
		return s.closeAttempt
	}
	attempt := make(chan struct{})
	s.closeAttempt = attempt
	s.closeRunning = true
	go func() {
		// Stop new enqueues, then join the worker and all writes it admitted.
		s.admissionMu.Lock()
		if s.done != nil {
			s.doneOnce.Do(func() { close(s.done) })
		}
		s.admissionMu.Unlock()
		if s.done != nil {
			s.wg.Wait()
		}
		if s.drainTimer != nil {
			s.drainTimer.Stop()
		}
		s.cancelWrites()
		physicalCloseReturned := false
		err := s.withTelemetryCloseOwnership(func() error {
			physicalCloseReturned = true
			return s.db.Close()
		})
		s.closeMu.Lock()
		s.closeErr = err
		s.closePhysical = physicalCloseReturned
		s.closeRunning = false
		if physicalCloseReturned {
			close(s.closeComplete)
		}
		close(attempt)
		s.closeMu.Unlock()
	}()
	return attempt
}

func (s *TelemetryStore) closeResult() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	return s.closeErr
}

func (s *TelemetryStore) isTelemetryClosePhysical() bool {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	return s.closePhysical
}

// WaitClosed joins the physical owner only after DB.Close has actually returned.
func (s *TelemetryStore) WaitClosed(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.ownedWriteContext()
	select {
	case <-s.closeComplete:
		return s.closeResult()
	case <-ctx.Done():
		return fmt.Errorf("telemetry close incomplete: %w", ctx.Err())
	}
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
			route TEXT,
			format TEXT,
			token_budget INTEGER DEFAULT 0,
			max_items INTEGER DEFAULT 0,
			max_chars INTEGER DEFAULT 0,
			compress INTEGER DEFAULT 0,
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
			warning_count INTEGER DEFAULT 0,
			pattern_mode TEXT,
			routing_outcome TEXT,
			failure_stage TEXT,
			hint_code TEXT,
			truncation_reason TEXT,
			decision_json TEXT,
			bytes_out INTEGER,
			harness TEXT,
			q_stages INTEGER,
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
		`ALTER TABLE access_events ADD COLUMN seq INTEGER DEFAULT 0`,
		`ALTER TABLE access_events ADD COLUMN route TEXT`,
		`ALTER TABLE access_events ADD COLUMN format TEXT`,
		`ALTER TABLE access_events ADD COLUMN token_budget INTEGER DEFAULT 0`,
		`ALTER TABLE access_events ADD COLUMN max_items INTEGER DEFAULT 0`,
		`ALTER TABLE access_events ADD COLUMN max_chars INTEGER DEFAULT 0`,
		`ALTER TABLE access_events ADD COLUMN compress INTEGER DEFAULT 0`,
		`ALTER TABLE access_events ADD COLUMN warning_count INTEGER DEFAULT 0`,
		`ALTER TABLE access_events ADD COLUMN pattern_mode TEXT`,
		`ALTER TABLE access_events ADD COLUMN routing_outcome TEXT`,
		`ALTER TABLE access_events ADD COLUMN failure_stage TEXT`,
		`ALTER TABLE access_events ADD COLUMN hint_code TEXT`,
		`ALTER TABLE access_events ADD COLUMN truncation_reason TEXT`,
		`ALTER TABLE access_events ADD COLUMN decision_json TEXT`,
		`ALTER TABLE access_events ADD COLUMN bytes_out INTEGER`,
		`ALTER TABLE access_events ADD COLUMN harness TEXT`,
		`ALTER TABLE access_events ADD COLUMN q_stages INTEGER`,
	} {
		if _, err := s.db.Exec(migration); err != nil && !isTelemetryDuplicateColumnError(err) {
			return fmt.Errorf("telemetry schema migration %q: %w", migration, err)
		}
	}
	for _, index := range []string{
		`CREATE INDEX IF NOT EXISTS idx_access_events_occurred_at ON access_events(occurred_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_access_events_workspace_time ON access_events(workspace_root, workspace_alias, workspace, occurred_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_access_events_operation_time ON access_events(operation, occurred_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_access_events_backend_time ON access_events(backend, occurred_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_access_events_session_time ON access_events(session_id, occurred_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_access_events_route_time ON access_events(route, occurred_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_access_events_failure_stage_time ON access_events(failure_stage, occurred_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_access_events_hint_time ON access_events(hint_code, occurred_at DESC)`,
	} {
		if _, err := s.db.Exec(index); err != nil {
			return err
		}
	}
	return nil
}

func isTelemetryDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "duplicate column name")
}

func (s *TelemetryStore) StartRun(state model.DaemonState) (int64, error) {
	var runID int64
	err := s.withTelemetryMutationContext(s.ownedWriteContext(), func(ctx context.Context) error {
		result, err := s.db.ExecContext(ctx,
			`INSERT INTO daemon_runs(pid, endpoint, admin_url, repo_root, protocol_version, started_at) VALUES (?, ?, ?, ?, ?, ?)`,
			state.PID,
			state.Endpoint,
			state.AdminURL,
			state.RepoRoot,
			state.ProtocolVersion,
			state.StartedAt.Unix(),
		)
		if err != nil {
			return err
		}
		runID, err = result.LastInsertId()
		return err
	})
	if err != nil {
		return 0, err
	}
	return runID, nil
}

func (s *TelemetryStore) StopRun(runID int64, stoppedAt time.Time) error {
	return s.stopRunContext(s.ownedWriteContext(), runID, stoppedAt)
}

func (s *TelemetryStore) stopRunContext(ctx context.Context, runID int64, stoppedAt time.Time) error {
	if runID == 0 {
		return nil
	}
	return s.withTelemetryMutationContext(ctx, func(ownedCtx context.Context) error {
		_, err := s.db.ExecContext(ownedCtx, `UPDATE daemon_runs SET stopped_at = ? WHERE id = ?`, stoppedAt.Unix(), runID)
		return err
	})
}

func (s *TelemetryStore) ReplaceRuntimeSnapshots(runID int64, statuses []model.WorkerStatus) error {
	return s.replaceRuntimeSnapshotsContext(s.ownedWriteContext(), runID, statuses)
}

func (s *TelemetryStore) replaceRuntimeSnapshotsContext(ctx context.Context, runID int64, statuses []model.WorkerStatus) error {
	if runID == 0 {
		return nil
	}
	return s.withTelemetryMutationContext(ctx, func(ownedCtx context.Context) (retErr error) {
		// The transaction lifetime is deliberately detached from cancellation so
		// database/sql's awaitDone goroutine cannot race physical rollback against
		// release of the cross-process writer lease. Every SQL operation still
		// uses ownedCtx; rollback/commit and connection return finish synchronously.
		conn, err := s.db.Conn(ownedCtx)
		if err != nil {
			return err
		}
		defer func() { retErr = errors.Join(retErr, conn.Close()) }()
		tx, err := conn.BeginTx(context.WithoutCancel(ownedCtx), nil)
		if err != nil {
			return err
		}
		txFinished := false
		defer func() {
			if !txFinished {
				retErr = errors.Join(retErr, tx.Rollback())
			}
		}()

		if _, err := tx.ExecContext(ownedCtx, `DELETE FROM runtime_snapshots WHERE daemon_run_id = ?`, runID); err != nil {
			return err
		}
		stmt, err := tx.PrepareContext(ownedCtx, `INSERT INTO runtime_snapshots(runtime_key, daemon_run_id, workspace_root, workspace_name, repo_name, repo_root, backend_type, entrypoint_id, entrypoint_path, entrypoint_type, pid, memory_bytes, started_at, last_used_at, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		for _, status := range statuses {
			if _, err := stmt.ExecContext(ownedCtx,
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
				return errors.Join(err, stmt.Close())
			}
		}
		if err := stmt.Close(); err != nil {
			return err
		}
		if err := ownedCtx.Err(); err != nil {
			return err
		}
		commitErr := tx.Commit()
		txFinished = true // Commit returns only after its driver operation has returned.
		return commitErr
	})
}

// NextSeq returns the next sequence number for the given session_id.
// Returns 0 if session_id is empty, 1 for the first event in a session.
func (s *TelemetryStore) NextSeq(sessionID string) int {
	seq, _ := s.nextSeqContext(s.ownedWriteContext(), sessionID)
	return seq
}

func (s *TelemetryStore) nextSeqContext(ctx context.Context, sessionID string) (int, error) {
	var seq int
	err := s.withTelemetryReadOwnershipContext(ctx, func(ownedCtx context.Context) error {
		var queryErr error
		seq, queryErr = s.nextSeqWithOwnershipContext(ownedCtx, sessionID)
		return queryErr
	})
	return seq, err
}

// nextSeqWithOwnershipContext is the YA-HELD form for the event writer.
func (s *TelemetryStore) nextSeqWithOwnershipContext(ctx context.Context, sessionID string) (int, error) {
	if sessionID == "" {
		return 0, ctx.Err()
	}
	var seq int
	err := retryTelemetrySQLiteContext(ctx, func() error {
		row := s.db.QueryRowContext(ctx,
			`SELECT COALESCE(MAX(seq), 0) + 1 FROM access_events WHERE session_id = ?`,
			sessionID,
		)
		return row.Scan(&seq)
	})
	return seq, err
}

func (s *TelemetryStore) RecordAccessDirect(event model.AccessEvent) error {
	if s == nil {
		return errors.New("telemetry store unavailable")
	}
	s.stateMu.RLock()
	closed := s.closed
	s.stateMu.RUnlock()
	if closed {
		s.reject("closed")
		return errors.New("telemetry store closed")
	}
	if err := checkTelemetryEventSize(event); err != nil {
		s.reject("event_too_large")
		return err
	}
	if err := s.recordAccessDirectInternal(0, event, true); err != nil {
		if !isTelemetryAdmissionError(err) {
			s.failWrite()
		}
		return err
	}
	return nil
}

func (s *TelemetryStore) RecordAccess(runID int64, event model.AccessEvent) error {
	if s == nil {
		return errors.New("telemetry store unavailable")
	}
	s.stateMu.RLock()
	closed := s.closed
	s.stateMu.RUnlock()
	if closed {
		s.reject("closed")
		return errors.New("telemetry store closed")
	}
	if err := checkTelemetryEventSize(event); err != nil {
		s.reject("event_too_large")
		return err
	}
	// Enqueue preflight is advisory; persistQueued rechecks under the same
	// ownership/OS lease immediately before its real SQL write.
	if err := s.admitWAL(); err != nil {
		return err
	}
	s.admissionMu.Lock()
	defer s.admissionMu.Unlock()
	s.stateMu.RLock()
	closed = s.closed
	s.stateMu.RUnlock()
	if closed {
		s.reject("closed")
		return errors.New("telemetry store closed")
	}
	if err := checkTelemetryEventSize(event); err != nil {
		s.reject("event_too_large")
		return err
	}
	select {
	case s.writeQueue <- telemetryWriteRequest{runID: runID, event: event, isDirect: false}:
		return nil
	default:
		s.reject("queue_full")
		return errors.New("telemetry write queue full")
	}
}

// Metadata PathError text may contain sqlite_busy in the filename; only an
// actual SQLite driver code (or the checkpoint sentinel) establishes BUSY.
func isTelemetrySQLiteBusyCode(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	code := sqliteErr.Code() & 0xff
	return code == 5 || code == 6 // SQLITE_BUSY / SQLITE_LOCKED, including extended codes.
}

func walAdmissionReason(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, errTelemetryWALLimit):
		return "wal_limit"
	case errors.Is(err, errTelemetryCheckpointBusy), isTelemetrySQLiteBusyCode(err):
		return "busy"
	default:
		return "wal_admission_error"
	}
}

func checkTelemetryEventSize(event model.AccessEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode telemetry event: %w", err)
	}
	if len(body) > telemetryMaxEventBytes {
		return errors.New("telemetry event exceeds 1 MiB admission limit")
	}
	return nil
}

func (s *TelemetryStore) reject(code string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.health.RejectedWrites++
	s.health.LastWriteFailure = code
	s.health.LastFailure = code
}
func (s *TelemetryStore) failWrite() { s.failWriteReason("write_error") }
func (s *TelemetryStore) failWriteReason(code string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.health.FailedWrites++
	s.health.LastWriteFailure = code
	s.health.LastFailure = code
}

func (s *TelemetryStore) RecentAccesses(limit int) ([]model.AccessEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	items := make([]model.AccessEvent, 0, limit)
	err := s.withTelemetryReadOwnershipContext(s.ownedWriteContext(), func(ctx context.Context) error {
		var rows *sql.Rows
		err := retryTelemetrySQLiteContext(ctx, func() error {
			var queryErr error
			rows, queryErr = s.db.QueryContext(ctx,
				`SELECT id, occurred_at, COALESCE(client_name, ''), COALESCE(session_id, ''), COALESCE(seq, 0), COALESCE(workspace, ''), COALESCE(workspace_input, ''), COALESCE(workspace_root, ''), COALESCE(workspace_alias, ''), COALESCE(repo, ''), operation, COALESCE(backend, ''), COALESCE(route, ''), COALESCE(format, ''), COALESCE(token_budget, 0), COALESCE(max_items, 0), COALESCE(max_chars, 0), COALESCE(compress, 0), success, latency_ms, COALESCE(warnings_json, '[]'), COALESCE(runtime_key, ''), COALESCE(entrypoint_id, ''), COALESCE(error_text, ''), COALESCE(error_kind, ''), COALESCE(error_code, ''), COALESCE(truncated, 0), COALESCE(result_count, 0), COALESCE(warning_count, 0), COALESCE(pattern_mode, ''), COALESCE(routing_outcome, ''), COALESCE(failure_stage, ''), COALESCE(hint_code, ''), COALESCE(truncation_reason, ''), COALESCE(decision_json, ''), COALESCE(bytes_out, 0), COALESCE(harness, ''), COALESCE(q_stages, 0) FROM access_events ORDER BY occurred_at DESC, id DESC LIMIT ?`,
				limit,
			)
			return queryErr
		})
		if err != nil { return err }
		defer rows.Close()
		for rows.Next() {
			item, err := scanAccessEvent(rows)
			if err != nil { return err }
			if strings.TrimSpace(item.DecisionJSON) != "" {
				hash := sha256.Sum256([]byte(item.DecisionJSON))
				item.DecisionHash = hex.EncodeToString(hash[:])[:12]
			}
			item.DecisionJSON = ""
			items = append(items, item)
		}
		return rows.Err()
	})
	if err != nil { return nil, err }
	return items, nil
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
	var items []metricsRow
	err := s.withTelemetryReadOwnershipContext(s.ownedWriteContext(), func(ctx context.Context) error {
		var rows *sql.Rows
		err := retryTelemetrySQLiteContext(ctx, func() error {
			var queryErr error
			rows, queryErr = s.db.QueryContext(ctx,
				`SELECT operation, COALESCE(NULLIF(workspace_root, ''), NULLIF(workspace, ''), ''), COALESCE(client_name, ''), latency_ms, success, COALESCE(truncated, 0)
				 FROM access_events
				 WHERE occurred_at > ?
				   AND operation NOT LIKE 'system.%'
				 ORDER BY operation, latency_ms`,
				since.Unix(),
			)
			return queryErr
		})
		if err != nil { return err }
		defer rows.Close()
		for rows.Next() {
			var item metricsRow
			var success, truncated int
			if err := rows.Scan(&item.operation, &item.workspace, &item.clientName, &item.latencyMs, &success, &truncated); err != nil { return err }
			item.success = success == 1
			item.truncated = truncated == 1
			items = append(items, item)
		}
		return rows.Err()
	})
	if err != nil { return nil, err }
	return items, nil
}

func scanAccessEvent(rows *sql.Rows) (model.AccessEvent, error) {
	var (
		item             model.AccessEvent
		occurredAt       int64
		compress         int
		success          int
		warningsJSON     string
		truncated        int
		resultCount      int
		warningCount     int
		patternMode      string
		routingOutcome   string
		failureStage     string
		hintCode         string
		truncationReason string
		decisionJSON     string
	)
	if err := rows.Scan(&item.ID, &occurredAt, &item.ClientName, &item.SessionID, &item.Seq, &item.Workspace, &item.WorkspaceInput, &item.WorkspaceRoot, &item.WorkspaceAlias, &item.Repo, &item.Operation, &item.Backend, &item.Route, &item.Format, &item.TokenBudget, &item.MaxItems, &item.MaxChars, &compress, &success, &item.LatencyMs, &warningsJSON, &item.RuntimeKey, &item.EntrypointID, &item.Error, &item.ErrorKind, &item.ErrorCode, &truncated, &resultCount, &warningCount, &patternMode, &routingOutcome, &failureStage, &hintCode, &truncationReason, &decisionJSON, &item.BytesOut, &item.Harness, &item.QStages); err != nil {
		return model.AccessEvent{}, err
	}
	item.OccurredAt = time.Unix(occurredAt, 0)
	item.Compress = compress == 1
	item.Success = success == 1
	item.Truncated = truncated == 1
	item.ResultCount = resultCount
	item.WarningCount = warningCount
	item.PatternMode = patternMode
	item.RoutingOutcome = routingOutcome
	item.FailureStage = failureStage
	item.HintCode = hintCode
	item.TruncationReason = truncationReason
	item.DecisionJSON = decisionJSON
	if warningsJSON != "" {
		_ = json.Unmarshal([]byte(warningsJSON), &item.Warnings)
	}
	return telemetry.NormalizeAccessEvent(item), nil
}
