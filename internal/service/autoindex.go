package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

// Auto-index self-heal for navigation reads.
//
// When the catalog is missing, unreadable or schema-broken, `nav find` answers
// from text and starts ONE detached full index job in the background. The job
// is deduplicated by the index job ownership table, rate-limited through
// <root>/.mi-lsp/autoindex.last after a failure, and runs with low priority,
// GOMAXPROCS=2 and a wall-clock timeout. It never touches the workspace
// registry.
//
// Manual override: MI_LSP_AUTOINDEX=0 (also "false"/"off") disables the
// background job; reads still fall back to text. MI_LSP_AUTOINDEX_TIMEOUT
// (Go duration, default 10m) bounds the job.
const (
	autoIndexEnvEnable  = "MI_LSP_AUTOINDEX"
	autoIndexEnvTimeout = "MI_LSP_AUTOINDEX_TIMEOUT"
	autoIndexEnvJob     = "MI_LSP_AUTOINDEX_JOB"

	autoIndexMarkerName     = "autoindex.last"
	autoIndexFailureBackoff = 10 * time.Minute
	autoIndexDefaultTimeout = 10 * time.Minute
	autoIndexHardKillGrace  = 30 * time.Second
	maxQuarantinedDBs       = 2

	autoIndexStatusStarted   = "started"
	autoIndexStatusFailed    = "failed"
	autoIndexStatusSucceeded = "succeeded"

	autoIndexOutcomeStarted        = "background reindex started"
	autoIndexOutcomeAlreadyRunning = "background reindex already running"
	autoIndexOutcomeSkipped        = "background reindex skipped"
)

var spawnDetachedAutoIndexJobProcess = startDetachedAutoIndexJobProcess

func autoIndexEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(autoIndexEnvEnable))) {
	case "0", "false", "off", "no":
		return false
	}
	return true
}

func autoIndexTimeout() time.Duration {
	if value := strings.TrimSpace(os.Getenv(autoIndexEnvTimeout)); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
			return parsed
		}
	}
	return autoIndexDefaultTimeout
}

func autoIndexJobProcess() bool {
	return os.Getenv(autoIndexEnvJob) == "1"
}

// classifyCatalogUnavailable maps a catalog read error to a primitives-v2
// reason code. Corruption and schema errors are index_schema_broken; anything
// else (absent, locked, unreadable) is index_not_ready.
//
// When the error carries its workspace root (see withCatalogRoot), a missing,
// empty or table-less index.db is index_not_ready, not schema-broken.
func classifyCatalogUnavailable(err error) string {
	if isIndexSchemaBrokenError(err) {
		if root := catalogErrorRoot(err); root != "" && catalogHasNoTables(root) {
			return model.ReasonIndexNotReady
		}
		return model.ReasonIndexSchemaBroken
	}
	return model.ReasonIndexNotReady
}

type catalogRootError struct {
	root string
	err  error
}

func (e *catalogRootError) Error() string { return e.err.Error() }
func (e *catalogRootError) Unwrap() error { return e.err }

// withCatalogRoot attaches the workspace root to a catalog read error so
// classifyCatalogUnavailable can tell an empty database from a broken one.
func withCatalogRoot(root string, err error) error {
	if err == nil || root == "" {
		return err
	}
	return &catalogRootError{root: root, err: err}
}

func catalogErrorRoot(err error) string {
	var rooted *catalogRootError
	if errors.As(err, &rooted) {
		return rooted.root
	}
	var openErr *workspaceDBOpenError
	if errors.As(err, &openErr) {
		return openErr.root
	}
	return ""
}

func isIndexSchemaBrokenError(err error) bool {
	return errorTextContains(err, "no such table", "no such column", "has no column named") || isIndexCorruptionError(err)
}

// isIndexCorruptionError is the strict subset that justifies quarantining the
// database. A missing table is NOT corruption: EnsureSchema recreates it, and a
// concurrent first-run process may be creating the schema right now.
func isIndexCorruptionError(err error) bool {
	return errorTextContains(err,
		"malformed",
		"not a database",
		"disk image",
		"database corrupt",
		"sqlite_corrupt",
		"sqlite_notadb",
		"file is encrypted",
	)
}

func errorTextContains(err error, markers ...string) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	var openErr *workspaceDBOpenError
	if errors.As(err, &openErr) && openErr.cause != nil {
		text += " " + strings.ToLower(openErr.cause.Error())
	}
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// catalogHasNoTables reports whether index.db is absent-in-effect: missing,
// empty file or a database without any user table (fresh, e.g.
// `workspace add --no-index`). Unreadable files are not "empty": they are broken.
func catalogHasNoTables(root string) bool {
	info, err := os.Stat(store.WorkspaceDBPath(root))
	if err != nil {
		return errors.Is(err, os.ErrNotExist)
	}
	if info.Size() == 0 {
		return true
	}
	db, err := store.OpenReadOnly(root)
	if err != nil {
		return false
	}
	defer db.Close()
	var tables int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`).Scan(&tables); err != nil {
		return false
	}
	return tables == 0
}

// workspaceDBReadable reports whether index.db answers a trivial query, i.e. it
// is not (or no longer) corrupt.
func workspaceDBReadable(root string) bool {
	db, err := store.OpenReadOnly(root)
	if err != nil {
		return false
	}
	defer db.Close()
	var tables int
	return db.QueryRow(`SELECT count(*) FROM sqlite_master`).Scan(&tables) == nil
}

type autoIndexMarker struct {
	At     time.Time `json:"at"`
	Status string    `json:"status"`
}

func autoIndexMarkerPath(root string) string {
	return filepath.Join(root, ".mi-lsp", autoIndexMarkerName)
}

func readAutoIndexMarker(root string) (autoIndexMarker, bool) {
	data, err := os.ReadFile(autoIndexMarkerPath(root))
	if err != nil {
		return autoIndexMarker{}, false
	}
	var marker autoIndexMarker
	if err := json.Unmarshal(data, &marker); err != nil || marker.At.IsZero() {
		return autoIndexMarker{}, false
	}
	return marker, true
}

func writeAutoIndexMarker(root string, status string, at time.Time) {
	if err := os.MkdirAll(filepath.Join(root, ".mi-lsp"), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(autoIndexMarker{At: at.UTC(), Status: status})
	if err != nil {
		return
	}
	path := autoIndexMarkerPath(root)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
	}
}

// autoIndexRateLimited reports whether the last auto job failed less than
// autoIndexFailureBackoff ago.
func autoIndexRateLimited(root string, now time.Time) bool {
	marker, ok := readAutoIndexMarker(root)
	if !ok || marker.Status != autoIndexStatusFailed {
		return false
	}
	return now.Sub(marker.At) < autoIndexFailureBackoff
}

// triggerAutoIndex starts at most one background full index job and returns a
// human outcome for warnings. It never returns an error: reads must not fail
// because the self-heal could not start.
func (a *App) triggerAutoIndex(ctx context.Context, registration model.WorkspaceRegistration) string {
	if !autoIndexEnabled() {
		return autoIndexOutcomeSkipped
	}
	now := time.Now()
	if autoIndexRateLimited(registration.Root, now) {
		return autoIndexOutcomeSkipped
	}
	db, err := openWorkspaceDBForIndex(registration, "index.auto")
	if err != nil {
		// Only a confirmed-corrupt database that could not be quarantined is a
		// persistent failure; transient open errors (locks, concurrent first
		// run) must not start a backoff.
		if isIndexCorruptionError(err) {
			writeAutoIndexMarker(registration.Root, autoIndexStatusFailed, now)
		}
		return autoIndexOutcomeSkipped
	}
	defer db.Close()
	job, err := store.CreateIndexJobWithEntrypoint(ctx, db, registration.Name, registration.Root, store.IndexModeFull, false, "")
	if err != nil {
		var activeErr *store.ActiveIndexJobError
		if errors.As(err, &activeErr) {
			return autoIndexOutcomeAlreadyRunning
		}
		// Job creation errors are not tied to the catalog; never back off.
		return autoIndexOutcomeSkipped
	}
	if _, err := a.spawnIndexJobWith(ctx, db, registration, job.JobID, spawnDetachedAutoIndexJobProcess); err != nil {
		_ = store.MarkIndexJobFailed(context.WithoutCancel(ctx), db, job.JobID, err.Error(), store.IndexJobFence{OwnerToken: job.OwnerToken, FencingToken: job.FencingToken})
		writeAutoIndexMarker(registration.Root, autoIndexStatusFailed, now)
		return autoIndexOutcomeSkipped
	}
	writeAutoIndexMarker(registration.Root, autoIndexStatusStarted, now)
	return autoIndexOutcomeStarted
}

// Corruption is confirmed by retrying before the database is moved away, so a
// process that merely raced a concurrent first-run creation never quarantines
// the fresh index.db the other process just created. The guard and the rename
// are serialized by quarantine.lock and re-probe the file inside the lock.
var (
	corruptionConfirmAttempts = 3
	corruptionConfirmDelay    = 150 * time.Millisecond
	quarantineLockStale       = 30 * time.Second
)

const quarantineLockName = "quarantine.lock"

// jitteredDelay spreads concurrent processes so they do not all re-check at the
// same instant.
func jitteredDelay(base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	return base + time.Duration(rand.Int63n(int64(base)))
}

// acquireQuarantineLock takes <root>/.mi-lsp/quarantine.lock with O_EXCL. A
// lock older than quarantineLockStale is considered abandoned. It never blocks:
// ok=false means another process holds it.
func acquireQuarantineLock(root string) (release func(), ok bool) {
	path := filepath.Join(root, ".mi-lsp", quarantineLockName)
	for attempt := 0; attempt < 2; attempt++ {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = file.WriteString(strconv.Itoa(os.Getpid()))
			_ = file.Close()
			return func() { _ = os.Remove(path) }, true
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, false
		}
		info, statErr := os.Stat(path)
		if statErr != nil || time.Since(info.ModTime()) < quarantineLockStale {
			return nil, false
		}
		_ = os.Remove(path)
	}
	return nil, false
}

// openWorkspaceDBForIndex is the write-side open used by index jobs. When the
// database stays unopenable because it is corrupt, it is quarantined (by one
// process at a time) and a fresh one is created so the rebuild can proceed.
func openWorkspaceDBForIndex(registration model.WorkspaceRegistration, operation string) (*sql.DB, error) {
	db, err := openWorkspaceDB(registration, operation, false) // readWrite
	if err == nil {
		return db, nil
	}
	if !isIndexCorruptionError(err) {
		return nil, err
	}
	reopen := func() (*sql.DB, error) { return openWorkspaceDB(registration, operation, false) } // readWrite
	for attempt := 0; attempt < corruptionConfirmAttempts; attempt++ {
		time.Sleep(jitteredDelay(corruptionConfirmDelay))
		retryDB, retryErr := reopen()
		if retryErr == nil {
			return retryDB, nil
		}
		if !isIndexCorruptionError(retryErr) {
			return nil, retryErr
		}
		err = retryErr
	}
	release, locked := acquireQuarantineLock(registration.Root)
	if !locked {
		// Another process is repairing: never rename; wait for its fresh db.
		for attempt := 0; attempt < corruptionConfirmAttempts; attempt++ {
			time.Sleep(jitteredDelay(corruptionConfirmDelay))
			if retryDB, retryErr := reopen(); retryErr == nil {
				return retryDB, nil
			} else {
				err = retryErr
			}
		}
		return nil, err
	}
	defer release()
	// Re-probe inside the lock: a process that held it before us may already
	// have replaced the corrupt file with a healthy one.
	if workspaceDBReadable(registration.Root) {
		return reopen()
	}
	if quarantineErr := quarantineWorkspaceDB(registration.Root, time.Now()); quarantineErr != nil && !errors.Is(quarantineErr, os.ErrNotExist) {
		return nil, err
	}
	return reopen()
}

var quarantineSuffixes = []string{"", "-wal", "-shm"}

// quarantineWorkspaceDB renames index.db (and its -wal/-shm companions) to
// index.db.corrupt-<unixts> and keeps only the maxQuarantinedDBs newest
// quarantines. It only touches files inside <root>/.mi-lsp/.
func quarantineWorkspaceDB(root string, now time.Time) error {
	dbPath := store.WorkspaceDBPath(root)
	if _, err := os.Stat(dbPath); err != nil {
		return err
	}
	stamp := now.Unix()
	for {
		if _, err := os.Stat(dbPath + ".corrupt-" + strconv.FormatInt(stamp, 10)); errors.Is(err, os.ErrNotExist) {
			break
		}
		stamp++
	}
	target := dbPath + ".corrupt-" + strconv.FormatInt(stamp, 10)
	if err := os.Rename(dbPath, target); err != nil {
		return err
	}
	for _, suffix := range quarantineSuffixes[1:] {
		if err := os.Rename(dbPath+suffix, target+suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	pruneQuarantinedDBs(filepath.Dir(dbPath), maxQuarantinedDBs)
	return nil
}

var quarantineNamePattern = regexp.MustCompile(`^index\.db\.corrupt-(\d+)(-wal|-shm)?$`)

func pruneQuarantinedDBs(stateDir string, keep int) {
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return
	}
	byStamp := map[int64][]string{}
	for _, entry := range entries {
		match := quarantineNamePattern.FindStringSubmatch(entry.Name())
		if match == nil || entry.IsDir() {
			continue
		}
		stamp, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			continue
		}
		byStamp[stamp] = append(byStamp[stamp], entry.Name())
	}
	stamps := make([]int64, 0, len(byStamp))
	for stamp := range byStamp {
		stamps = append(stamps, stamp)
	}
	sort.Slice(stamps, func(i, j int) bool { return stamps[i] > stamps[j] })
	for _, stamp := range stamps[min(keep, len(stamps)):] {
		for _, name := range byStamp[stamp] {
			_ = os.Remove(filepath.Join(stateDir, name))
		}
	}
}

// startAutoIndexJobGuards runs inside the detached auto job process. It lowers
// the priority, bounds the run with a wall-clock timeout and arms a hard
// watchdog in case the indexer ignores cancellation. The returned stop
// function must be called when the job finishes.
func startAutoIndexJobGuards(ctx context.Context, root string) (context.Context, func()) {
	lowerProcessPriority()
	timeout := autoIndexTimeout()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	watchdog := time.AfterFunc(timeout+autoIndexHardKillGrace, func() {
		writeAutoIndexMarker(root, autoIndexStatusFailed, time.Now())
		os.Exit(1)
	})
	return ctx, func() {
		watchdog.Stop()
		cancel()
	}
}

func recordAutoIndexResult(root string, job store.IndexJob, err error) {
	switch {
	case err != nil:
		writeAutoIndexMarker(root, autoIndexStatusFailed, time.Now())
	case job.Status == store.IndexJobSucceeded:
		writeAutoIndexMarker(root, autoIndexStatusSucceeded, time.Now())
	}
}

// crossWorkspaceAwareAutoIndex starts the background reindex unless the
// request is a harness read of another workspace: that reindex is a write in
// a workspace the caller did not open, so it needs --allow-cross-workspace and
// each override use is recorded like any other cross-workspace write.
func (a *App) crossWorkspaceAwareAutoIndex(ctx context.Context, registration model.WorkspaceRegistration, request model.CommandRequest) string {
	if request.Context.NoIndexRefresh {
		return autoIndexOutcomeSkipped + " (read-only q query; run mi-lsp index explicitly when needed)"
	}
	if request.Context.CrossWorkspaceRead {
		if !request.Context.AllowCrossWorkspace {
			return autoIndexOutcomeSkipped + " (cross-workspace read; run mi-lsp index from that workspace or pass --allow-cross-workspace)"
		}
		recordCrossWorkspaceOverride(autoIndexOverrideOperation, request.Context.ClientName, request.Context.Workspace, registration.Root, request.Context.CallerCWD)
	}
	return a.triggerAutoIndex(ctx, registration)
}

// autoIndexOverrideOperation names the background reindex in overrides.log.
const autoIndexOverrideOperation = "index.autoindex"

// findTextFallback answers nav.find from text when the catalog cannot. The
// result is ok:true and degraded; it is never an empty failure.
func (a *App) findTextFallback(ctx context.Context, registration model.WorkspaceRegistration, project model.ProjectFile, request model.CommandRequest, pattern string, reason string) model.Envelope {
	if reason == model.ReasonIndexSchemaBroken && catalogHasNoTables(registration.Root) {
		// A fresh or empty index.db is simply not built yet.
		reason = model.ReasonIndexNotReady
	}
	outcome := a.crossWorkspaceAwareAutoIndex(ctx, registration, request)
	limit := request.Context.MaxItems
	items := []map[string]any{}
	warnings := []string{fmt.Sprintf("catalog unavailable (%s); served from text; %s", reason, outcome)}
	if strings.TrimSpace(pattern) != "" {
		exact, _ := request.Payload["exact"].(bool)
		matcher := newTextDeclarationMatcher(pattern, exact)
		// Declarations only; plain occurrences are the last resort.
		hits, err := searchPattern(ctx, registration.Root, project, matcher.searchRe, true, 500)
		if err != nil {
			warnings = append(warnings, "text search failed: "+sanitizeIntentError(err))
		}
		items = matcher.declarationItems(hits)
		if len(items) == 0 {
			hits, err = searchPattern(ctx, registration.Root, project, wordBoundaryPattern(pattern), true, 300)
			if err != nil {
				warnings = append(warnings, "text search failed: "+sanitizeIntentError(err))
			}
			items = matcher.occurrenceItems(hits)
		}
		if offset := intFromAny(request.Payload["offset"], 0); offset > 0 {
			if offset >= len(items) {
				items = []map[string]any{}
			} else {
				items = items[offset:]
			}
		}
		if limit > 0 && len(items) > limit {
			items = items[:limit]
		}
	}
	envelope := model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "text",
		Items:     items,
		Stats:     model.Stats{Symbols: len(items)},
		Warnings:  warnings,
	}
	if len(items) == 0 {
		// The catalog is absent, so the empty result stays marked degraded.
		envelope.MarkDegraded(model.ReasonNoMatches, model.FallbackText)
		return envelope
	}
	envelope.MarkDegraded(reason, model.FallbackText)
	return envelope
}

func wordBoundaryPattern(pattern string) string {
	quoted := regexp.QuoteMeta(pattern)
	if isWordRune(rune(pattern[0])) {
		quoted = `\b` + quoted
	}
	if isWordRune(rune(pattern[len(pattern)-1])) {
		quoted += `\b`
	}
	return quoted
}

func isWordRune(r rune) bool {
	return r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}
