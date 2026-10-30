package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/worker"
)

const memCacheTTL = 10 * time.Second

// failureCache holds cached deterministic solution-load failures keyed by runtime_key
type failureCache struct {
	mu      sync.Mutex
	entries map[string]failureCacheEntry
}

type failureCacheEntry struct {
	err       error
	expiresAt time.Time
}

func newFailureCache() *failureCache {
	return &failureCache{
		entries: make(map[string]failureCacheEntry),
	}
}

func (fc *failureCache) get(key string) (error, bool) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	entry, ok := fc.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(fc.entries, key)
		return nil, false
	}
	return entry.err, true
}

func (fc *failureCache) set(key string, err error, ttl time.Duration) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.entries[key] = failureCacheEntry{
		err:       err,
		expiresAt: time.Now().Add(ttl),
	}
}

func (fc *failureCache) getContext(ctx context.Context, key string) (error, bool, error) {
	if err := lockManagerContext(ctx, &fc.mu); err != nil {
		return nil, false, err
	}
	defer fc.mu.Unlock()
	entry, ok := fc.entries[key]
	if !ok {
		return nil, false, nil
	}
	if time.Now().After(entry.expiresAt) {
		delete(fc.entries, key)
		return nil, false, nil
	}
	return entry.err, true, nil
}

func (fc *failureCache) setContext(ctx context.Context, key string, err error, ttl time.Duration) error {
	if err := lockManagerContext(ctx, &fc.mu); err != nil {
		return err
	}
	fc.entries[key] = failureCacheEntry{err: err, expiresAt: time.Now().Add(ttl)}
	fc.mu.Unlock()
	return nil
}

// isDeterministicSolutionFailure returns true if the error is a deterministic
// solution/project configuration failure (not transient like network/timeout).
func isDeterministicSolutionFailure(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Cache failures that indicate config/duplicate issues, not transient errors
	return strings.Contains(errStr, "already exists") ||
		strings.Contains(errStr, "duplicate") ||
		strings.Contains(errStr, "not found") && strings.Contains(errStr, "project") ||
		strings.Contains(errStr, "invalid") && strings.Contains(errStr, "solution")
}

type managedRuntime struct {
	workspace    model.WorkspaceRegistration
	request      model.WorkerRequest
	client       worker.RuntimeClient
	status       model.WorkerStatus
	memCachedAt  time.Time
	activeCalls  atomic.Int64
	lastUsedUnix atomic.Int64
}

type Manager struct {
	repoRoot           string
	maxWorkers         int
	idleTimeout        time.Duration
	callTimeout        time.Duration
	softMemoryThresh   uint64
	options            StartOptions
	mu                 sync.Mutex
	runtimes           map[string]*managedRuntime
	stopCh             chan struct{}
	watchers           map[string]*FileWatcher
	watcherRoots       []string
	skippedWatchers    int
	watcherCtx         context.Context
	watcherCancel      context.CancelFunc
	failureCache       *failureCache
	telemetryStore     *TelemetryStore
	reapCount          int
	reapDone           chan struct{}
	reapRequest        chan struct{}
	reapHook           func()
	watcherTransitions atomic.Int64
}

// UpdateIdleSnapshot is conservative: existing runtimes or watchers can own
// activity outside request accounting. An uninitialized manager is unknown.
func (m *Manager) UpdateIdleSnapshot() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runtimes != nil && m.watchers != nil && len(m.runtimes) == 0 && len(m.watchers) == 0 && m.watcherTransitions.Load() == 0
}

func NewManager(repoRoot string, maxWorkers int, idleTimeout time.Duration) *Manager {
	return NewManagerWithOptions(repoRoot, maxWorkers, idleTimeout, DefaultStartOptions())
}

func NewManagerWithOptions(repoRoot string, maxWorkers int, idleTimeout time.Duration, options StartOptions) *Manager {
	maxWorkers = normalizeMaxWorkers(maxWorkers)
	if idleTimeout <= 0 {
		idleTimeout = 30 * time.Minute
	}
	callTimeout := parseDurationEnv("MI_LSP_WORKER_CALL_TIMEOUT_SECONDS", 30*time.Second)
	softMemoryThresh := runtimeMemoryLimitBytes("MI_LSP_DAEMON_SOFT_MEMORY_MB")
	if softMemoryThresh == 0 {
		softMemoryThresh = 500 * 1024 * 1024 // 500MB default
	}
	options = NormalizeStartOptions(options)
	watcherCtx, watcherCancel := context.WithCancel(context.Background())
	manager := &Manager{
		repoRoot:         repoRoot,
		maxWorkers:       maxWorkers,
		idleTimeout:      idleTimeout,
		callTimeout:      callTimeout,
		softMemoryThresh: softMemoryThresh,
		options:          options,
		runtimes:         map[string]*managedRuntime{},
		stopCh:           make(chan struct{}),
		reapDone:         make(chan struct{}),
		watchers:         map[string]*FileWatcher{},
		watcherCtx:       watcherCtx,
		watcherCancel:    watcherCancel,
		failureCache:     newFailureCache(),
	}
	go manager.reapLoop()
	return manager
}

func (m *Manager) Call(ctx context.Context, workspace model.WorkspaceRegistration, request model.WorkerRequest) (model.WorkerResponse, error) {
	request.BackendType = normalizeBackendType(request.BackendType)
	if ctx == nil {
		ctx = context.Background()
	}
	callCtx, cancel := context.WithTimeout(ctx, m.callTimeout)
	defer cancel()
	if err := m.EnsureFileWatcherContext(callCtx, workspace); err != nil && callCtx.Err() != nil {
		return model.WorkerResponse{}, callCtx.Err()
	}
	if err := callCtx.Err(); err != nil {
		return model.WorkerResponse{}, err
	}

	// Check failure cache for this runtime
	rk := runtimeKey(workspace, request)
	cachedErr, cached, cacheErr := m.failureCache.getContext(callCtx, rk)
	if cacheErr != nil {
		return model.WorkerResponse{}, cacheErr
	}
	if cached {
		return model.WorkerResponse{}, fmt.Errorf("cached backend failure: %w", cachedErr)
	}

	managed, err := m.getOrCreateContext(callCtx, workspace, request, true)
	if err != nil {
		return model.WorkerResponse{}, err
	}
	defer m.releaseRuntime(managed)

	response, err := managed.client.Call(callCtx, request)
	m.updateStatusContext(callCtx, managed)
	if err != nil && request.BackendType == worker.KotlinBackend {
		m.mu.Lock()
		if m.runtimes[rk] == managed {
			delete(m.runtimes, rk)
		}
		m.mu.Unlock()
		_ = managed.client.Close()
	}

	// Cache deterministic solution-load failures to avoid repeated long timeouts
	if err != nil && callCtx.Err() == nil && isDeterministicSolutionFailure(err) {
		_ = m.failureCache.setContext(callCtx, rk, err, 5*time.Minute)
	}

	return response, err
}

func (m *Manager) Warm(workspace model.WorkspaceRegistration) []string {
	warnings := make([]string, 0)
	m.EnsureFileWatcher(workspace)
	for _, backendType := range backendsForWorkspace(workspace) {
		request := defaultWarmRequest(workspace, backendType)
		if _, err := m.getOrCreate(workspace, request, false); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s warm skipped: %v", backendType, err))
		}
	}
	return warnings
}

func (m *Manager) Status() []model.WorkerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	items := make([]model.WorkerStatus, 0, len(m.runtimes))
	for _, managed := range m.runtimes {
		if now.Sub(managed.memCachedAt) > memCacheTTL {
			managed.status.MemoryBytes = processMemoryBytes(managed.status.PID)
			managed.memCachedAt = now
		}
		if lastUsed := managed.lastUsedUnix.Load(); lastUsed != 0 {
			managed.status.LastUsedAt = time.Unix(0, lastUsed)
		}
		items = append(items, managed.status)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Workspace == items[j].Workspace {
			if items[i].RepoName == items[j].RepoName {
				return items[i].BackendType < items[j].BackendType
			}
			return items[i].RepoName < items[j].RepoName
		}
		return items[i].Workspace < items[j].Workspace
	})
	return items
}

func (m *Manager) SetTelemetryStore(ts *TelemetryStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.telemetryStore = ts
}

func (m *Manager) Shutdown() {
	close(m.stopCh)
	if m.reapDone != nil {
		<-m.reapDone
	}
	m.watcherCancel()
	m.StopWatchers()
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, managed := range m.runtimes {
		_ = managed.client.Close()
		delete(m.runtimes, key)
	}
}

func (m *Manager) beginUpdateWatcherTransition() func() {
	m.watcherTransitions.Add(1)
	var once sync.Once
	return func() { once.Do(m.endUpdateWatcherTransition) }
}
func (m *Manager) endUpdateWatcherTransition() {
	m.watcherTransitions.Add(-1)
}

// StartFileWatchers initializes and starts file watchers for registered workspaces.
func (m *Manager) StartFileWatchers(registrations []model.WorkspaceRegistration) {
	if m.options.WatchMode == WatchModeOff {
		return
	}
	for _, reg := range registrations {
		m.EnsureFileWatcher(reg)
	}
}

func (m *Manager) EnsureFileWatcher(registration model.WorkspaceRegistration) {
	_ = m.EnsureFileWatcherContext(m.watcherCtx, registration)
}

func (m *Manager) EnsureFileWatcherContext(ctx context.Context, registration model.WorkspaceRegistration) error {
	if m.options.WatchMode == WatchModeOff {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	key := canonicalWatcherRootKey(registration.Root)
	if key == "" {
		return nil
	}
	if err := lockManagerContext(ctx, &m.mu); err != nil {
		return err
	}
	if _, ok := m.watchers[key]; ok {
		m.touchWatcherLocked(key)
		m.mu.Unlock()
		return nil
	}
	if len(m.watchers) >= m.options.MaxWatchedRoots {
		m.evictWatcherLocked()
	}
	m.watcherTransitions.Add(1)
	m.mu.Unlock()
	defer m.endUpdateWatcherTransition()

	fw, err := NewFileWatcher(registration, 500*time.Millisecond)
	if err != nil {
		return err
	}
	if err := fw.StartWithContexts(ctx, m.watcherCtx); err != nil {
		fw.Stop()
		return err
	}
	if err := lockManagerContext(ctx, &m.mu); err != nil {
		fw.Stop()
		return err
	}
	if existing, ok := m.watchers[key]; ok {
		m.touchWatcherLocked(key)
		m.mu.Unlock()
		fw.Stop()
		_ = existing
		return nil
	}
	if len(m.watchers) >= m.options.MaxWatchedRoots {
		m.evictWatcherLocked()
	}
	m.watchers[key] = fw
	m.watcherRoots = append(m.watcherRoots, key)
	m.mu.Unlock()
	return nil
}

func lockManagerContext(ctx context.Context, mu *sync.Mutex) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for !mu.TryLock() {
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

// StopWatchers stops all running file watchers.
func (m *Manager) StopWatchers() {
	m.mu.Lock()
	watchers := make([]*FileWatcher, 0, len(m.watchers))
	for _, fw := range m.watchers {
		watchers = append(watchers, fw)
	}
	m.watchers = map[string]*FileWatcher{}
	m.watcherRoots = nil
	m.watcherTransitions.Add(1)
	m.mu.Unlock()
	defer m.endUpdateWatcherTransition()

	for _, fw := range watchers {
		fw.Stop()
	}
}

func (m *Manager) WatcherStats() model.DaemonWatcherStats {
	m.mu.Lock()
	defer m.mu.Unlock()
	stats := model.DaemonWatcherStats{
		Mode:             m.options.WatchMode,
		MaxWatchedRoots:  m.options.MaxWatchedRoots,
		WatchedRoots:     len(m.watchers),
		ActiveRootKeys:   append([]string(nil), m.watcherRoots...),
		SkippedRootCount: m.skippedWatchers,
	}
	for _, fw := range m.watchers {
		stats.WatchedDirs += fw.WatchedDirCount()
		stats.PendingEvents += fw.PendingEvents()
	}
	return stats
}

func (m *Manager) touchWatcherLocked(key string) {
	for i, candidate := range m.watcherRoots {
		if candidate == key {
			copy(m.watcherRoots[i:], m.watcherRoots[i+1:])
			m.watcherRoots[len(m.watcherRoots)-1] = key
			return
		}
	}
	m.watcherRoots = append(m.watcherRoots, key)
}

func (m *Manager) evictWatcherLocked() {
	if len(m.watcherRoots) == 0 {
		return
	}
	victimKey := m.watcherRoots[0]
	m.watcherRoots = append([]string(nil), m.watcherRoots[1:]...)
	if victim := m.watchers[victimKey]; victim != nil {
		delete(m.watchers, victimKey)
		m.skippedWatchers++
		m.watcherTransitions.Add(1)
		go func() { defer m.endUpdateWatcherTransition(); victim.Stop() }()
	}
}

func (m *Manager) reapLoop() {
	defer close(m.reapDone)
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-m.reapRequest:
			if m.reapHook != nil {
				m.reapHook()
			}
		case <-ticker.C:
			m.reapIdle()
			m.checkpointWALIfNeeded()
		}
	}
}

func (m *Manager) checkpointWALIfNeeded() {
	m.mu.Lock()
	m.reapCount++
	count := m.reapCount
	store := m.telemetryStore
	m.mu.Unlock()

	if count%30 == 0 && store != nil {
		if err := store.Checkpoint(); err != nil {
			// Error is non-fatal; log to debug and continue.
			log.Printf("[mi-lsp:daemon] WAL checkpoint failed (non-fatal): %v", err)
		}
	}
}

func (m *Manager) reapIdle() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()

	// Check memory pressure and apply soft limit
	processStats := getProcessStats()
	if processStats.PrivateBytes > m.softMemoryThresh {
		log.Printf("[mi-lsp:daemon] memory pressure: %dMB exceeds soft limit %dMB, reducing idle timeout and triggering GC",
			processStats.PrivateBytes/(1024*1024), m.softMemoryThresh/(1024*1024))

		// Reduce idle timeout under memory pressure
		effectiveIdleTimeout := m.idleTimeout / 2
		for key, managed := range m.runtimes {
			lastUsed := managed.status.LastUsedAt
			if stored := managed.lastUsedUnix.Load(); stored != 0 {
				lastUsed = time.Unix(0, stored)
			}
			if managed.activeCalls.Load() == 0 && now.Sub(lastUsed) > effectiveIdleTimeout {
				_ = managed.client.Close()
				delete(m.runtimes, key)
			}
		}

		// Trigger garbage collection
		runtime.GC()
	} else {
		// Normal reap at full idle timeout
		for key, managed := range m.runtimes {
			lastUsed := managed.status.LastUsedAt
			if stored := managed.lastUsedUnix.Load(); stored != 0 {
				lastUsed = time.Unix(0, stored)
			}
			if managed.activeCalls.Load() == 0 && now.Sub(lastUsed) > m.idleTimeout {
				_ = managed.client.Close()
				delete(m.runtimes, key)
			}
		}
	}

	m.enforceIdleMemoryBoundsLocked(now)
}

func (m *Manager) WorkerCapacity() (inUse int, slots int) {
	if m == nil {
		return 0, 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.runtimes), m.maxWorkers
}

func (m *Manager) getOrCreate(workspace model.WorkspaceRegistration, request model.WorkerRequest, markActive bool) (*managedRuntime, error) {
	m.mu.Lock()
	return m.getOrCreateLocked(context.Background(), workspace, request, markActive)
}

func (m *Manager) getOrCreateContext(ctx context.Context, workspace model.WorkspaceRegistration, request model.WorkerRequest, markActive bool) (*managedRuntime, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for !m.mu.TryLock() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
	return m.getOrCreateLocked(ctx, workspace, request, markActive)
}

func (m *Manager) getOrCreateLocked(ctx context.Context, workspace model.WorkspaceRegistration, request model.WorkerRequest, markActive bool) (*managedRuntime, error) {
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key := runtimeKey(workspace, request)
	if managed, ok := m.runtimes[key]; ok {
		lastUsed := time.Now()
		managed.status.LastUsedAt = lastUsed
		managed.lastUsedUnix.Store(lastUsed.UnixNano())
		if markActive {
			managed.activeCalls.Add(1)
		}
		return managed, nil
	}

	if len(m.runtimes) >= m.maxWorkers {
		m.evictLeastRecentlyUsed()
	}

	client, err := worker.NewRuntimeClient(m.repoRoot, workspace, request)
	if err != nil {
		return nil, err
	}

	managed := &managedRuntime{
		workspace: workspace,
		request:   request,
		client:    client,
		status: model.WorkerStatus{
			Workspace:      workspace.Name,
			WorkspaceRoot:  workspace.Root,
			BackendType:    request.BackendType,
			RuntimeKey:     key,
			RepoID:         request.RepoID,
			RepoName:       request.RepoName,
			RepoRoot:       request.RepoRoot,
			EntrypointID:   request.EntrypointID,
			EntrypointPath: request.EntrypointPath,
			EntrypointType: request.EntrypointType,
			StartedAt:      time.Now(),
			LastUsedAt:     time.Now(),
		},
	}
	managed.lastUsedUnix.Store(managed.status.LastUsedAt.UnixNano())

	// LSP initialize belongs to Call(ctx), not pool allocation under a lock.
	_, genericLSP := client.(*worker.LSPClient)
	_, kotlinLSP := client.(*worker.KotlinClient)
	if starter, ok := client.(interface{ Start() error }); ok && !genericLSP && !kotlinLSP {
		if err := starter.Start(); err != nil {
			return nil, err
		}
	}
	managed.status.PID = client.PID()
	managed.status.MemoryBytes = processMemoryBytes(managed.status.PID)
	managed.memCachedAt = time.Now()
	if markActive {
		managed.activeCalls.Add(1)
	}
	m.runtimes[key] = managed
	m.enforceIdleMemoryBoundsLocked(time.Now())
	return managed, nil
}

func (m *Manager) evictLeastRecentlyUsed() {
	victimKey, victim := leastRecentlyUsedIdleRuntime(m.runtimes)
	if victim != nil {
		_ = victim.client.Close()
		delete(m.runtimes, victimKey)
	}
}

func (m *Manager) updateStatusContext(ctx context.Context, managed *managedRuntime) {
	if managed == nil {
		return
	}
	now := time.Now()
	managed.lastUsedUnix.Store(now.UnixNano())
	// PID may acquire the worker transport lock; sample it before taking the manager lock.
	pid := managed.client.PID()
	if lockManagerContext(ctx, &m.mu) != nil {
		return
	}
	if m.runtimes[managed.status.RuntimeKey] == managed {
		if managed.status.PID != pid {
			managed.status.PID = pid
			managed.status.MemoryBytes = 0
			if pid == 0 {
				managed.memCachedAt = now
			} else {
				managed.memCachedAt = time.Time{}
			}
		}
		managed.status.LastUsedAt = now
	}
	m.mu.Unlock()
}

func (m *Manager) releaseRuntime(managed *managedRuntime) {
	if managed != nil {
		managed.activeCalls.Add(-1)
	}
}

func (m *Manager) enforceIdleMemoryBoundsLocked(now time.Time) {
	maxRuntimeBytes := runtimeMemoryLimitBytes("MI_LSP_DAEMON_MAX_RUNTIME_MEMORY_MB")
	if maxRuntimeBytes == 0 {
		maxRuntimeBytes = 1024 * 1024 * 1024 // 1024MB default
	}
	totalRuntimeBytes := runtimeMemoryLimitBytes("MI_LSP_DAEMON_TOTAL_RUNTIME_MEMORY_MB")

	// Enforce per-runtime memory limit
	for key, managed := range m.runtimes {
		refreshManagedMemory(managed, now)
		if managed.activeCalls.Load() == 0 && managed.status.MemoryBytes > maxRuntimeBytes {
			_ = managed.client.Close()
			delete(m.runtimes, key)
		}
	}

	// Enforce total runtime memory limit if set
	if totalRuntimeBytes == 0 {
		return
	}
	for runtimeMemoryTotalLocked(m.runtimes, now) > totalRuntimeBytes {
		victimKey, victim := leastRecentlyUsedIdleRuntime(m.runtimes)
		if victim == nil {
			return
		}
		_ = victim.client.Close()
		delete(m.runtimes, victimKey)
	}
}

func refreshManagedMemory(managed *managedRuntime, now time.Time) {
	if now.Sub(managed.memCachedAt) > memCacheTTL {
		managed.status.MemoryBytes = processMemoryBytes(managed.status.PID)
		managed.memCachedAt = now
	}
}

func runtimeMemoryTotalLocked(runtimes map[string]*managedRuntime, now time.Time) uint64 {
	var total uint64
	for _, managed := range runtimes {
		refreshManagedMemory(managed, now)
		total += managed.status.MemoryBytes
	}
	return total
}

func leastRecentlyUsedIdleRuntime(runtimes map[string]*managedRuntime) (string, *managedRuntime) {
	var victimKey string
	var victim *managedRuntime
	var victimLastUsed time.Time
	for key, candidate := range runtimes {
		if candidate.activeCalls.Load() > 0 {
			continue
		}
		lastUsed := candidate.status.LastUsedAt
		if stored := candidate.lastUsedUnix.Load(); stored != 0 {
			lastUsed = time.Unix(0, stored)
		}
		if victim == nil || lastUsed.Before(victimLastUsed) {
			victim = candidate
			victimKey = key
			victimLastUsed = lastUsed
		}
	}
	return victimKey, victim
}

func runtimeMemoryLimitBytes(envName string) uint64 {
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "" {
		return 0
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 {
		return 0
	}
	return value * 1024 * 1024
}

func runtimeKey(workspace model.WorkspaceRegistration, request model.WorkerRequest) string {
	entrypoint := strings.TrimSpace(request.EntrypointID)
	if entrypoint == "" {
		entrypoint = strings.TrimSpace(request.RepoID)
	}
	if entrypoint == "" {
		entrypoint = filepath.Base(strings.TrimSpace(workspace.Root))
	}
	workspaceScope := canonicalRuntimeRootKey(workspace.Root)
	if request.BackendType == worker.KotlinBackend {
		config, err := worker.KotlinRuntimeConfiguration(workspace, request)
		if err != nil {
			return worker.KotlinBackend + "::" + workspaceScope + "::unavailable::" + request.EntrypointPath
		}
		return worker.KotlinBackend + "::" + canonicalRuntimeRootKey(config.ModuleRoot) + "::" + config.Entrypoint + "::" + config.Fingerprint
	}
	return normalizeBackendType(request.BackendType) + "::" + workspaceScope + "::" + entrypoint
}

func canonicalRuntimeRootKey(root string) string {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return "-"
	}
	abs, err := filepath.Abs(trimmed)
	if err == nil {
		trimmed = abs
	}
	cleaned := filepath.Clean(trimmed)
	if runtime.GOOS == "windows" {
		return strings.ToLower(cleaned)
	}
	return cleaned
}

func canonicalWatcherRootKey(root string) string {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return ""
	}
	abs, err := filepath.Abs(trimmed)
	if err == nil {
		trimmed = abs
	}
	if evaluated, err := filepath.EvalSymlinks(trimmed); err == nil {
		trimmed = evaluated
	}
	cleaned := filepath.Clean(trimmed)
	if cleaned == "." || cleaned == string(filepath.Separator) {
		return cleaned
	}
	if runtime.GOOS == "windows" {
		return strings.ToLower(cleaned)
	}
	return cleaned
}

func normalizeBackendType(backendType string) string {
	if strings.TrimSpace(backendType) == "" {
		return "roslyn"
	}
	return strings.ToLower(strings.TrimSpace(backendType))
}

func backendsForWorkspace(workspace model.WorkspaceRegistration) []string {
	items := make([]string, 0, 2)
	for _, language := range workspace.Languages {
		switch strings.ToLower(language) {
		case "csharp":
			items = appendIfMissing(items, "roslyn")
		case "typescript":
			if worker.CanUseTsserver(workspace.Root) {
				items = appendIfMissing(items, "tsserver")
			}
		case "python":
			if worker.CanUsePyright(workspace.Root) {
				items = appendIfMissing(items, "pyright")
			}
		case "go":
			if worker.CanUseGopls(workspace.Root) {
				items = appendIfMissing(items, "gopls")
			}
		case "rust":
			if worker.CanUseRustAnalyzer(workspace.Root) {
				items = appendIfMissing(items, "rust-analyzer")
			}
		}
	}
	// Roslyn is warmed only for C# workspaces; a Go/TS/Python repo never pays
	// for the .NET worker.
	return items
}

func defaultWarmRequest(workspace model.WorkspaceRegistration, backendType string) model.WorkerRequest {
	request := model.WorkerRequest{
		ProtocolVersion: model.ProtocolVersion,
		Method:          "status",
		Workspace:       workspace.Root,
		WorkspaceName:   workspace.Name,
		BackendType:     backendType,
		RepoName:        filepath.Base(workspace.Root),
		RepoRoot:        workspace.Root,
	}
	if strings.EqualFold(backendType, "roslyn") && workspace.Solution != "" {
		request.EntrypointID = "default"
		request.EntrypointPath = workspace.Solution
		request.EntrypointType = model.EntrypointKindSolution
	}
	if strings.EqualFold(backendType, "tsserver") {
		request.EntrypointID = "default::tsserver"
		request.EntrypointType = "repo"
	}
	if strings.EqualFold(backendType, "pyright") {
		request.EntrypointID = "default::pyright"
		request.EntrypointType = "repo"
	}
	return request
}

func appendIfMissing(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func statusSummary(statuses []model.WorkerStatus) string {
	parts := make([]string, 0, len(statuses))
	for _, status := range statuses {
		label := status.Workspace + ":" + status.BackendType
		if status.RepoName != "" {
			label += ":" + status.RepoName
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, ",")
}

func parseDurationEnv(envName string, defaultVal time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "" {
		return defaultVal
	}
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seconds <= 0 {
		return defaultVal
	}
	return time.Duration(seconds) * time.Second
}

func getProcessStats() model.DaemonProcessStats {
	// Get current process stats (will be implemented per-platform as needed)
	// For now, return zero values; the actual memory check will work with system calls
	pid := os.Getpid()
	return model.DaemonProcessStats{
		PID:          pid,
		PrivateBytes: getPrivateMemoryBytes(pid),
	}
}

func getPrivateMemoryBytes(pid int) uint64 {
	// Platform-specific memory retrieval; delegated to existing impl if available
	// For daemon purposes, we'll use processMemoryBytes if it exists
	return processMemoryBytes(pid)
}
