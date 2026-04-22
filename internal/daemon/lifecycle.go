package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/worker"
)

const memCacheTTL = 10 * time.Second

type managedRuntime struct {
	workspace   model.WorkspaceRegistration
	request     model.WorkerRequest
	client      worker.RuntimeClient
	status      model.WorkerStatus
	memCachedAt time.Time
}

type Manager struct {
	repoRoot        string
	maxWorkers      int
	idleTimeout     time.Duration
	options         StartOptions
	mu              sync.Mutex
	runtimes        map[string]*managedRuntime
	stopCh          chan struct{}
	watchers        map[string]*FileWatcher
	watcherRoots    []string
	skippedWatchers int
	watcherCtx      context.Context
	watcherCancel   context.CancelFunc
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
	options = NormalizeStartOptions(options)
	watcherCtx, watcherCancel := context.WithCancel(context.Background())
	manager := &Manager{
		repoRoot:      repoRoot,
		maxWorkers:    maxWorkers,
		idleTimeout:   idleTimeout,
		options:       options,
		runtimes:      map[string]*managedRuntime{},
		stopCh:        make(chan struct{}),
		watchers:      map[string]*FileWatcher{},
		watcherCtx:    watcherCtx,
		watcherCancel: watcherCancel,
	}
	go manager.reapLoop()
	return manager
}

func (m *Manager) Call(ctx context.Context, workspace model.WorkspaceRegistration, request model.WorkerRequest) (model.WorkerResponse, error) {
	request.BackendType = normalizeBackendType(request.BackendType)
	m.EnsureFileWatcher(workspace)
	managed, err := m.getOrCreate(workspace, request)
	if err != nil {
		return model.WorkerResponse{}, err
	}
	response, err := managed.client.Call(ctx, request)
	m.updateStatus(managed)
	return response, err
}

func (m *Manager) Warm(workspace model.WorkspaceRegistration) []string {
	warnings := make([]string, 0)
	m.EnsureFileWatcher(workspace)
	for _, backendType := range backendsForWorkspace(workspace) {
		request := defaultWarmRequest(workspace, backendType)
		if _, err := m.getOrCreate(workspace, request); err != nil {
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
	for key, managed := range m.runtimes {
		if now.Sub(managed.status.LastUsedAt) > m.idleTimeout {
			_ = managed.client.Close()
			delete(m.runtimes, key)
		}
	}
}

func (m *Manager) getOrCreate(workspace model.WorkspaceRegistration, request model.WorkerRequest) (*managedRuntime, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := runtimeKey(workspace, request)
	if managed, ok := m.runtimes[key]; ok {
		managed.status.LastUsedAt = time.Now()
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
	m.runtimes[key] = managed
	return managed, nil
}

func (m *Manager) evictLeastRecentlyUsed() {
	var victimKey string
	var victim *managedRuntime
	for key, candidate := range m.runtimes {
		if victim == nil || candidate.status.LastUsedAt.Before(victim.status.LastUsedAt) {
			victim = candidate
			victimKey = key
		}
	}
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

func runtimeKey(workspace model.WorkspaceRegistration, request model.WorkerRequest) string {
	entrypoint := strings.TrimSpace(request.EntrypointID)
	if entrypoint == "" {
		entrypoint = strings.TrimSpace(request.RepoID)
	}
	if entrypoint == "" {
		entrypoint = filepath.Base(strings.TrimSpace(workspace.Root))
	}
	workspaceScope := strings.TrimSpace(workspace.Name)
	if workspaceScope == "" {
		workspaceScope = strings.TrimSpace(workspace.Root)
	}
	return normalizeBackendType(request.BackendType) + "::" + workspaceScope + "::" + entrypoint
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
