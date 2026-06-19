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
	workspace   model.WorkspaceRegistration
	request     model.WorkerRequest
	client      worker.RuntimeClient
	status      model.WorkerStatus
	memCachedAt time.Time
	activeCalls int
}

type Manager struct {
	repoRoot         string
	maxWorkers       int
	idleTimeout      time.Duration
	callTimeout      time.Duration
	softMemoryThresh uint64
	options          StartOptions
	mu               sync.Mutex
	runtimes         map[string]*managedRuntime
	stopCh           chan struct{}
	watchers         map[string]*FileWatcher
	watcherRoots     []string
	skippedWatchers  int
	watcherCtx       context.Context
	watcherCancel    context.CancelFunc
	failureCache     *failureCache
}

func NewManager(repoRoot string, maxWorkers int, idleTimeout time.Duration) *Manager {
	return NewManagerWithOptions(repoRoot, maxWorkers, idleTimeout, DefaultStartOptions())
}

func NewManagerWithOptions(repoRoot string, maxWorkers int, idleTimeout time.Duration, options StartOptions) *Manager {
	if maxWorkers <= 0 {
		maxWorkers = 3
	}
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
	m.EnsureFileWatcher(workspace)

	// Check failure cache for this runtime
	rk := runtimeKey(workspace, request)
	if cachedErr, ok := m.failureCache.get(rk); ok {
		return model.WorkerResponse{}, fmt.Errorf("cached backend failure: %w", cachedErr)
	}

	managed, err := m.getOrCreate(workspace, request, true)
	if err != nil {
		return model.WorkerResponse{}, err
	}
	defer m.releaseRuntime(managed)

	// Wrap context with per-call timeout
	callCtx, cancel := context.WithTimeout(ctx, m.callTimeout)
	defer cancel()

	response, err := managed.client.Call(callCtx, request)
	m.updateStatus(managed)

	// Cache deterministic solution-load failures to avoid repeated long timeouts
	if err != nil && isDeterministicSolutionFailure(err) {
		m.failureCache.set(rk, err, 5*time.Minute)
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

func (m *Manager) Shutdown() {
	close(m.stopCh)
	m.watcherCancel()
	m.StopWatchers()
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, managed := range m.runtimes {
		_ = managed.client.Close()
		delete(m.runtimes, key)
	}
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
	if m.options.WatchMode == WatchModeOff {
		return
	}
	key := canonicalWatcherRootKey(registration.Root)
	if key == "" {
		return
	}

	m.mu.Lock()
	if _, ok := m.watchers[key]; ok {
		m.touchWatcherLocked(key)
		m.mu.Unlock()
		return
	}
	if len(m.watchers) >= m.options.MaxWatchedRoots {
		m.evictWatcherLocked()
	}
	m.mu.Unlock()

	fw, err := NewFileWatcher(registration, 500*time.Millisecond)
	if err != nil {
		if os.Getenv("MI_LSP_VERBOSE") != "" {
			fmt.Printf("[mi-lsp:watcher] failed to create watcher for %s: %v\n", registration.Root, err)
		}
		return
	}
	if err := fw.Start(m.watcherCtx); err != nil {
		fw.Stop()
		if os.Getenv("MI_LSP_VERBOSE") != "" {
			fmt.Printf("[mi-lsp:watcher] failed to start watcher for %s: %v\n", registration.Root, err)
		}
		return
	}

	m.mu.Lock()
	if existing, ok := m.watchers[key]; ok {
		m.touchWatcherLocked(key)
		m.mu.Unlock()
		fw.Stop()
		_ = existing
		return
	}
	if len(m.watchers) >= m.options.MaxWatchedRoots {
		m.evictWatcherLocked()
	}
	m.watchers[key] = fw
	m.watcherRoots = append(m.watcherRoots, key)
	m.mu.Unlock()
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
	m.mu.Unlock()

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
		go victim.Stop()
	}
}

func (m *Manager) reapLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.reapIdle()
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
			if managed.activeCalls == 0 && now.Sub(managed.status.LastUsedAt) > effectiveIdleTimeout {
				_ = managed.client.Close()
				delete(m.runtimes, key)
			}
		}

		// Trigger garbage collection
		runtime.GC()
	} else {
		// Normal reap at full idle timeout
		for key, managed := range m.runtimes {
			if managed.activeCalls == 0 && now.Sub(managed.status.LastUsedAt) > m.idleTimeout {
				_ = managed.client.Close()
				delete(m.runtimes, key)
			}
		}
	}

	m.enforceIdleMemoryBoundsLocked(now)
}

func (m *Manager) getOrCreate(workspace model.WorkspaceRegistration, request model.WorkerRequest, markActive bool) (*managedRuntime, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := runtimeKey(workspace, request)
	if managed, ok := m.runtimes[key]; ok {
		managed.status.LastUsedAt = time.Now()
		if markActive {
			managed.activeCalls++
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

	if starter, ok := client.(interface{ Start() error }); ok {
		if err := starter.Start(); err != nil {
			return nil, err
		}
	}
	managed.status.PID = client.PID()
	managed.status.MemoryBytes = processMemoryBytes(managed.status.PID)
	managed.memCachedAt = time.Now()
	if markActive {
		managed.activeCalls++
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

func (m *Manager) updateStatus(managed *managedRuntime) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	managed.status.LastUsedAt = now
	if now.Sub(managed.memCachedAt) > memCacheTTL {
		managed.status.MemoryBytes = processMemoryBytes(managed.status.PID)
		managed.memCachedAt = now
	}
}

func (m *Manager) releaseRuntime(managed *managedRuntime) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if managed.activeCalls > 0 {
		managed.activeCalls--
	}
}

func (m *Manager) enforceIdleMemoryBoundsLocked(now time.Time) {
	maxRuntimeBytes := runtimeMemoryLimitBytes("MI_LSP_DAEMON_MAX_RUNTIME_MEMORY_MB")
	totalRuntimeBytes := runtimeMemoryLimitBytes("MI_LSP_DAEMON_TOTAL_RUNTIME_MEMORY_MB")
	if maxRuntimeBytes == 0 && totalRuntimeBytes == 0 {
		return
	}

	for key, managed := range m.runtimes {
		refreshManagedMemory(managed, now)
		if maxRuntimeBytes > 0 && managed.activeCalls == 0 && managed.status.MemoryBytes > maxRuntimeBytes {
			_ = managed.client.Close()
			delete(m.runtimes, key)
		}
	}

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
	for key, candidate := range runtimes {
		if candidate.activeCalls > 0 {
			continue
		}
		if victim == nil || candidate.status.LastUsedAt.Before(victim.status.LastUsedAt) {
			victim = candidate
			victimKey = key
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
		}
	}
	if len(items) == 0 {
		items = append(items, "roslyn")
	}
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
