package daemon

import (
	"encoding/json"
	"math"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/service"
	"github.com/fgpaz/mi-lsp/internal/telemetry"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

type AdminServer struct {
	listener net.Listener
	server   *http.Server
	manager  *Manager
	store    *TelemetryStore
	app      *service.App
	stateFn  func() model.DaemonState
}

type dashboardMetrics struct {
	ActiveRuntimes   int `json:"active_runtimes"`
	TrackedAccesses  int `json:"tracked_accesses"`
	DegradedBackends int `json:"degraded_backends"`
	RecentColdStarts int `json:"recent_cold_starts"`
}

type workspaceSummary struct {
	Workspace      string    `json:"workspace"`
	WorkspaceRoot  string    `json:"workspace_root,omitempty"`
	RuntimeCount   int       `json:"runtime_count"`
	AccessCount    int       `json:"access_count"`
	WarningCount   int       `json:"warning_count"`
	Backends       []string  `json:"backends"`
	LastActivityAt time.Time `json:"last_activity_at,omitempty"`
	Active         bool      `json:"active"`
}

func NewAdminServer(manager *Manager, store *TelemetryStore, app *service.App, stateFn func() model.DaemonState) (*AdminServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	admin := &AdminServer{listener: listener, manager: manager, store: store, app: app, stateFn: stateFn}
	mux := http.NewServeMux()
	mux.HandleFunc("/", admin.handleIndex)
	mux.HandleFunc("/api/status", admin.handleStatus)
	mux.HandleFunc("/api/workspaces", admin.handleWorkspaces)
	mux.HandleFunc("/api/workspaces/", admin.handleWorkspace)
	mux.HandleFunc("/api/accesses", admin.handleAccesses)
	mux.HandleFunc("/api/logs", admin.handleLogs)
	mux.HandleFunc("/api/metrics", admin.handleMetrics)
	admin.server = &http.Server{Handler: mux}
	go func() { _ = admin.server.Serve(listener) }()
	return admin, nil
}

func (a *AdminServer) URL() string {
	if a == nil || a.listener == nil {
		return ""
	}
	return "http://" + a.listener.Addr().String()
}

func (a *AdminServer) Shutdown() error {
	if a == nil || a.server == nil {
		return nil
	}
	return a.server.Close()
}

func (a *AdminServer) handleIndex(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	_, _ = writer.Write([]byte(adminHTML))
}

func (a *AdminServer) handleStatus(writer http.ResponseWriter, request *http.Request) {
	window, err := a.resolveWindow(request, "recent")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	a.writeJSON(writer, a.dashboardPayload(a.parseCount(request, "limit", 120, 500), window))
}

func (a *AdminServer) handleWorkspaces(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	window, err := a.resolveWindow(request, "recent")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	payload := a.dashboardPayload(a.parseCount(request, "limit", 200, 500), window)
	a.writeJSON(writer, map[string]any{"items": payload["workspaces"], "window": window.Name, "window_label": window.Label})
}

func (a *AdminServer) handleWorkspace(writer http.ResponseWriter, request *http.Request) {
	pathValue := strings.TrimSpace(strings.TrimPrefix(request.URL.Path, "/api/workspaces/"))
	if pathValue == "" {
		http.NotFound(writer, request)
		return
	}
	if strings.HasSuffix(pathValue, "/warm") {
		a.handleWorkspaceWarm(writer, request, strings.TrimSuffix(pathValue, "/warm"))
		return
	}
	if request.Method != http.MethodGet {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	workspaceName, err := neturl.PathUnescape(strings.Trim(pathValue, "/"))
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	window, err := a.resolveWindow(request, "recent")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	statuses := make([]model.WorkerStatus, 0)
	for _, status := range a.manager.Status() {
		if strings.EqualFold(status.Workspace, workspaceName) {
			statuses = append(statuses, status)
		}
	}
	filtered, err := QueryAccessEvents(a.store, ExportQuery{Since: window.Since, Workspace: workspaceName, Limit: 250})
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	summary := workspaceSummary{Workspace: workspaceName}
	if summaries := summarizeWorkspaces(statuses, filtered); len(summaries) > 0 {
		summary = summaries[0]
	}
	payload := map[string]any{"workspace": workspaceName, "summary": summary, "runtimes": statuses, "accesses": filtered, "window": window.Name, "window_label": window.Label}
	if a.app != nil {
		if registration, resolveErr := a.app.ResolveWorkspace(workspaceName); resolveErr == nil {
			if project, topologyErr := workspace.LoadProjectTopology(registration.Root, registration); topologyErr == nil {
				registration = workspace.ApplyProjectTopology(registration, project)
				payload["registration"] = registration
				payload["project"] = project
			}
		}
	}
	a.writeJSON(writer, payload)
}

func (a *AdminServer) handleWorkspaceWarm(writer http.ResponseWriter, request *http.Request, rawWorkspace string) {
	if request.Method != http.MethodPost {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	workspaceName, err := neturl.PathUnescape(strings.Trim(rawWorkspace, "/"))
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	if a.app == nil {
		http.Error(writer, "workspace warm is unavailable", http.StatusServiceUnavailable)
		return
	}
	registration, err := a.app.ResolveWorkspace(workspaceName)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusNotFound)
		return
	}
	warnings := a.manager.Warm(registration)
	statuses := make([]model.WorkerStatus, 0)
	for _, status := range a.manager.Status() {
		if strings.EqualFold(status.Workspace, registration.Name) {
			statuses = append(statuses, status)
		}
	}
	a.writeJSON(writer, map[string]any{"ok": true, "workspace": registration.Name, "runtimes": statuses, "warnings": warnings, "message": "workspace warmed"})
}

func (a *AdminServer) handleAccesses(writer http.ResponseWriter, request *http.Request) {
	window, err := a.resolveWindow(request, "recent")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	workspaceFilter := strings.TrimSpace(request.URL.Query().Get("workspace"))
	backendFilter := strings.TrimSpace(request.URL.Query().Get("backend"))
	clientFilter := strings.TrimSpace(request.URL.Query().Get("client"))
	items, err := QueryAccessEvents(a.store, ExportQuery{
		Since:     window.Since,
		Workspace: workspaceFilter,
		Backend:   backendFilter,
		Limit:     a.parseCount(request, "limit", 150, 500),
	})
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	if clientFilter != "" {
		filtered := make([]model.AccessEvent, 0, len(items))
		for _, access := range items {
			if strings.EqualFold(access.ClientName, clientFilter) {
				filtered = append(filtered, access)
			}
		}
		items = filtered
	}
	a.writeJSON(writer, map[string]any{"items": items, "window": window.Name, "window_label": window.Label})
}

func (a *AdminServer) handleLogs(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tail := a.parseCount(request, "tail", 120, 400)
	path, items, warning, err := a.readLogTail(tail)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	payload := map[string]any{"path": path, "tail": tail, "items": items}
	if warning != "" {
		payload["warnings"] = []string{warning}
	}
	a.writeJSON(writer, payload)
}

func (a *AdminServer) syncSnapshots() {
	if a.store == nil {
		return
	}
	state := a.stateFn()
	if state.RunID == 0 {
		return
	}
	_ = a.store.ReplaceRuntimeSnapshots(state.RunID, a.manager.Status())
}

func (a *AdminServer) dashboardPayload(accessLimit int, window telemetry.Window) map[string]any {
	accesses, _ := QueryAccessEvents(a.store, ExportQuery{Since: window.Since, Limit: accessLimit})
	runtimes := []model.WorkerStatus{}
	watchers := model.DaemonWatcherStats{}
	if a.manager != nil {
		runtimes = a.manager.Status()
		watchers = a.manager.WatcherStats()
	}
	return map[string]any{
		"state":           a.stateFn(),
		"daemon_process":  processStats(os.Getpid()),
		"watchers":        watchers,
		"metrics":         buildDashboardMetrics(runtimes, accesses),
		"active_runtimes": runtimes,
		"recent_accesses": accesses,
		"workspaces":      summarizeWorkspaces(runtimes, accesses),
		"window":          window.Name,
		"window_label":    window.Label,
		"generated_at":    time.Now(),
	}
}

func buildDashboardMetrics(runtimes []model.WorkerStatus, accesses []model.AccessEvent) dashboardMetrics {
	metrics := dashboardMetrics{ActiveRuntimes: len(runtimes), TrackedAccesses: len(accesses)}
	degraded := map[string]struct{}{}
	recentCutoff := time.Now().Add(-15 * time.Minute)
	for _, access := range accesses {
		if !access.Success || len(access.Warnings) > 0 || strings.TrimSpace(access.Error) != "" {
			key := strings.TrimSpace(access.RuntimeKey)
			if key == "" {
				key = strings.ToLower(strings.TrimSpace(access.Backend + "::" + telemetry.WorkspaceAnalyticsKey(access)))
			}
			if key != "" {
				degraded[key] = struct{}{}
			}
		}
		if access.OccurredAt.After(recentCutoff) && access.LatencyMs >= 1000 && !strings.HasPrefix(access.Operation, "system.") {
			metrics.RecentColdStarts++
		}
	}
	metrics.DegradedBackends = len(degraded)
	return metrics
}

func summarizeWorkspaces(runtimes []model.WorkerStatus, accesses []model.AccessEvent) []workspaceSummary {
	items := map[string]*workspaceSummary{}
	backendSets := map[string]map[string]struct{}{}
	ensure := func(key string, display string) *workspaceSummary {
		name := strings.TrimSpace(display)
		if name == "" {
			name = "unscoped"
		}
		analyticsKey := strings.TrimSpace(key)
		if analyticsKey == "" {
			analyticsKey = name
		}
		if existing, ok := items[analyticsKey]; ok {
			if existing.Workspace == "" && name != "" {
				existing.Workspace = name
			}
			return existing
		}
		summary := &workspaceSummary{Workspace: name, WorkspaceRoot: analyticsKey, Backends: []string{}}
		items[analyticsKey] = summary
		backendSets[analyticsKey] = map[string]struct{}{}
		return summary
	}
	for _, status := range runtimes {
		key := strings.TrimSpace(status.WorkspaceRoot)
		if key == "" {
			key = strings.TrimSpace(status.Workspace)
		}
		summary := ensure(key, status.Workspace)
		summary.RuntimeCount++
		summary.Active = true
		if status.LastUsedAt.After(summary.LastActivityAt) {
			summary.LastActivityAt = status.LastUsedAt
		}
		if status.BackendType != "" {
			backendSets[key][status.BackendType] = struct{}{}
		}
	}
	for _, access := range accesses {
		event := telemetry.NormalizeAccessEvent(access)
		key := telemetry.WorkspaceAnalyticsKey(event)
		summary := ensure(key, telemetry.WorkspaceDisplay(event))
		summary.AccessCount++
		if event.OccurredAt.After(summary.LastActivityAt) {
			summary.LastActivityAt = event.OccurredAt
		}
		if !event.Success || len(event.Warnings) > 0 || strings.TrimSpace(event.Error) != "" {
			summary.WarningCount++
		}
		if event.Backend != "" {
			backendSets[key][event.Backend] = struct{}{}
		}
	}
	names := make([]string, 0, len(items))
	for key := range items {
		names = append(names, key)
	}
	sort.Slice(names, func(i, j int) bool {
		left := items[names[i]]
		right := items[names[j]]
		if left.Active != right.Active {
			return left.Active
		}
		if !left.LastActivityAt.Equal(right.LastActivityAt) {
			return left.LastActivityAt.After(right.LastActivityAt)
		}
		return strings.ToLower(left.Workspace) < strings.ToLower(right.Workspace)
	})
	results := make([]workspaceSummary, 0, len(names))
	for _, name := range names {
		backends := make([]string, 0, len(backendSets[name]))
		for backend := range backendSets[name] {
			backends = append(backends, backend)
		}
		sort.Strings(backends)
		summary := items[name]
		summary.Backends = backends
		results = append(results, *summary)
	}
	return results
}

func (a *AdminServer) readLogTail(tail int) (string, []map[string]any, string, error) {
	state := a.stateFn()
	path := filepath.Join(state.RepoRoot, ".mi-lsp", "daemon.log")
	lines, truncated, err := ReadLogTailFile(path, tail, 1<<20)
	if err != nil {
		if os.IsNotExist(err) {
			return path, []map[string]any{}, "no daemon log file has been written yet", nil
		}
		return path, nil, "", err
	}
	if len(lines) == 0 {
		return path, []map[string]any{}, "daemon log is empty", nil
	}
	items := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		items = append(items, map[string]any{"line": line.Line, "text": line.Text})
	}
	warning := ""
	if truncated {
		warning = "daemon log tail read was capped to 1048576 bytes"
	}
	return path, items, warning, nil
}

func (a *AdminServer) parseCount(request *http.Request, key string, defaultValue int, maxValue int) int {
	value := strings.TrimSpace(request.URL.Query().Get(key))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return defaultValue
	}
	if parsed > maxValue {
		return maxValue
	}
	return parsed
}

func (a *AdminServer) resolveWindow(request *http.Request, defaultName string) (telemetry.Window, error) {
	if request == nil {
		return telemetry.ResolveWindow(defaultName, time.Now())
	}
	if raw := strings.TrimSpace(request.URL.Query().Get("window")); raw != "" {
		return telemetry.ResolveWindow(raw, time.Now())
	}
	if rawDays := strings.TrimSpace(request.URL.Query().Get("days")); rawDays != "" {
		return telemetry.ResolveWindow(rawDays+"d", time.Now())
	}
	return telemetry.ResolveWindow(defaultName, time.Now())
}

func (a *AdminServer) writeJSON(writer http.ResponseWriter, payload any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	encoder := json.NewEncoder(writer)
	_ = encoder.Encode(payload)
}

type MetricsSummary struct {
	Window      string             `json:"window,omitempty"`
	WindowLabel string             `json:"window_label,omitempty"`
	WindowDays  int                `json:"window_days"`
	Total       int                `json:"total"`
	ErrorRate   float64            `json:"error_rate"`
	TruncRate   float64            `json:"trunc_rate"`
	Operations  []OperationMetrics `json:"operations"`
	Workspaces  []WorkspaceMetrics `json:"workspaces"`
	Clients     []ClientMetrics    `json:"clients"`
	GeneratedAt time.Time          `json:"generated_at"`
}

type OperationMetrics struct {
	Operation string  `json:"operation"`
	Count     int     `json:"count"`
	AvgMs     float64 `json:"avg_ms"`
	P50Ms     int64   `json:"p50_ms"`
	P95Ms     int64   `json:"p95_ms"`
	ErrorRate float64 `json:"error_rate"`
	TruncRate float64 `json:"trunc_rate"`
}

type WorkspaceMetrics struct {
	Workspace string  `json:"workspace"`
	Count     int     `json:"count"`
	AvgMs     float64 `json:"avg_ms"`
}

type ClientMetrics struct {
	ClientName string `json:"client_name"`
	Count      int    `json:"count"`
}

func percentileMs(sortedLatencies []int64, pct float64) int64 {
	if len(sortedLatencies) == 0 {
		return 0
	}
	idx := int(math.Floor(float64(len(sortedLatencies)-1) * pct))
	return sortedLatencies[idx]
}

func (a *AdminServer) handleMetrics(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	window, err := a.resolveWindow(request, "recent")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	rows, err := a.store.ComputeMetrics(window.Since)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}

	type opBucket struct {
		latencies  []int64
		errCount   int
		truncCount int
	}
	type wsBucket struct {
		count   int
		totalMs int64
	}
	opBuckets := map[string]*opBucket{}
	wsBuckets := map[string]*wsBucket{}
	clientCounts := map[string]int{}
	totalErrors := 0
	totalTrunc := 0

	for _, row := range rows {
		op := row.operation
		if _, ok := opBuckets[op]; !ok {
			opBuckets[op] = &opBucket{}
		}
		opBuckets[op].latencies = append(opBuckets[op].latencies, row.latencyMs)
		if !row.success {
			opBuckets[op].errCount++
			totalErrors++
		}
		if row.truncated {
			opBuckets[op].truncCount++
			totalTrunc++
		}

		ws := row.workspace
		if ws == "" {
			ws = "unscoped"
		}
		if _, ok := wsBuckets[ws]; !ok {
			wsBuckets[ws] = &wsBucket{}
		}
		wsBuckets[ws].count++
		wsBuckets[ws].totalMs += row.latencyMs

		cn := row.clientName
		if cn == "" {
			cn = "manual-cli"
		}
		clientCounts[cn]++
	}

	total := len(rows)
	ops := make([]OperationMetrics, 0, len(opBuckets))
	for op, bucket := range opBuckets {
		n := len(bucket.latencies)
		var totalMs int64
		for _, ms := range bucket.latencies {
			totalMs += ms
		}
		var avgMs float64
		if n > 0 {
			avgMs = float64(totalMs) / float64(n)
		}
		var errRate, truncRate float64
		if n > 0 {
			errRate = math.Round(float64(bucket.errCount)/float64(n)*1000) / 10
			truncRate = math.Round(float64(bucket.truncCount)/float64(n)*1000) / 10
		}
		ops = append(ops, OperationMetrics{
			Operation: op,
			Count:     n,
			AvgMs:     math.Round(avgMs*10) / 10,
			P50Ms:     percentileMs(bucket.latencies, 0.50),
			P95Ms:     percentileMs(bucket.latencies, 0.95),
			ErrorRate: errRate,
			TruncRate: truncRate,
		})
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].Count > ops[j].Count })

	wsNames := make([]string, 0, len(wsBuckets))
	for name := range wsBuckets {
		wsNames = append(wsNames, name)
	}
	sort.Slice(wsNames, func(i, j int) bool { return wsBuckets[wsNames[i]].count > wsBuckets[wsNames[j]].count })
	workspaces := make([]WorkspaceMetrics, 0, len(wsNames))
	for _, name := range wsNames {
		bucket := wsBuckets[name]
		var avg float64
		if bucket.count > 0 {
			avg = float64(bucket.totalMs) / float64(bucket.count)
		}
		workspaces = append(workspaces, WorkspaceMetrics{Workspace: name, Count: bucket.count, AvgMs: math.Round(avg*10) / 10})
	}

	clientNames := make([]string, 0, len(clientCounts))
	for name := range clientCounts {
		clientNames = append(clientNames, name)
	}
	sort.Slice(clientNames, func(i, j int) bool { return clientCounts[clientNames[i]] > clientCounts[clientNames[j]] })
	clients := make([]ClientMetrics, 0, len(clientNames))
	for _, name := range clientNames {
		clients = append(clients, ClientMetrics{ClientName: name, Count: clientCounts[name]})
	}

	var overallErrRate, overallTruncRate float64
	if total > 0 {
		overallErrRate = math.Round(float64(totalErrors)/float64(total)*1000) / 10
		overallTruncRate = math.Round(float64(totalTrunc)/float64(total)*1000) / 10
	}

	days := int(math.Round(window.Duration.Hours() / 24))
	a.writeJSON(writer, MetricsSummary{
		Window:      window.Name,
		WindowLabel: window.Label,
		WindowDays:  days,
		Total:       total,
		ErrorRate:   overallErrRate,
		TruncRate:   overallTruncRate,
		Operations:  ops,
		Workspaces:  workspaces,
		Clients:     clients,
		GeneratedAt: time.Now(),
	})
}
