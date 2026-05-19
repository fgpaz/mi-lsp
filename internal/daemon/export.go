package daemon

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	toon "github.com/toon-format/toon-go"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/telemetry"
)

type ExportQuery struct {
	Since          time.Time
	Workspace      string
	Backend        string
	Operation      string
	SessionID      string
	ClientName     string
	Route          string
	Format         string
	Truncated      *bool
	PatternMode    string
	RoutingOutcome string
	FailureStage   string
	HintCode       string
	ErrorsOnly     bool
	Limit          int
	WindowLabel    string
}

type WorkspaceStat struct {
	Ops      int     `json:"ops"`
	Errors   int     `json:"errors"`
	Warnings int     `json:"warnings"`
	P50Ms    int64   `json:"p50_ms"`
	AvgMs    float64 `json:"avg_ms"`
}

type ErrorFrequency struct {
	ErrorKind  string   `json:"error_kind,omitempty"`
	ErrorCode  string   `json:"error_code,omitempty"`
	ErrorText  string   `json:"error_text"`
	Count      int      `json:"count"`
	Workspaces []string `json:"workspaces"`
}

type BackendHistogram struct {
	Backend string `json:"backend"`
	Count   int    `json:"count"`
}

type OperationPercentiles struct {
	Operation string `json:"operation"`
	Count     int    `json:"count"`
	P50Ms     int64  `json:"p50_ms"`
	P95Ms     int64  `json:"p95_ms"`
	P99Ms     int64  `json:"p99_ms"`
}

type UsageRecommendation struct {
	ID       string   `json:"id"`
	Severity string   `json:"severity"`
	Reason   string   `json:"reason"`
	Command  string   `json:"command,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
}

type ExportSummary struct {
	TotalOps               int                      `json:"total_ops"`
	WindowLabel            string                   `json:"window_label,omitempty"`
	ByWorkspace            map[string]WorkspaceStat `json:"by_workspace"`
	ByOperation            map[string]WorkspaceStat `json:"by_operation,omitempty"`
	ByRoute                map[string]WorkspaceStat `json:"by_route,omitempty"`
	ByClient               map[string]WorkspaceStat `json:"by_client,omitempty"`
	ByHintCode             map[string]WorkspaceStat `json:"by_hint_code,omitempty"`
	ByFailureStage         map[string]WorkspaceStat `json:"by_failure_stage,omitempty"`
	TopErrors              []ErrorFrequency         `json:"top_errors"`
	ByBackend              []BackendHistogram       `json:"by_backend,omitempty"`
	ByOperationPercentiles []OperationPercentiles   `json:"by_operation_percentiles,omitempty"`
	Recommendations        []UsageRecommendation    `json:"recommendations,omitempty"`
}

func QueryAccessEvents(store *TelemetryStore, query ExportQuery) ([]model.AccessEvent, error) {
	if query.Limit < 0 {
		query.Limit = 500
	}

	sql, args := buildAccessEventsSQL(query, true)
	rows, err := store.db.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	capHint := query.Limit
	switch {
	case capHint <= 0:
		capHint = 64
	case capHint > 512:
		capHint = 512
	}
	items := make([]model.AccessEvent, 0, capHint)
	for rows.Next() {
		item, err := scanAccessEvent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func QueryAccessSummary(store *TelemetryStore, query ExportQuery) (ExportSummary, error) {
	if query.Limit < 0 {
		query.Limit = 500
	}
	sql, args := buildAccessEventsSQL(query, false)
	rows, err := store.db.Query(sql, args...)
	if err != nil {
		return ExportSummary{}, err
	}
	defer rows.Close()

	acc := newSummaryAccumulator()
	for rows.Next() {
		item, err := scanAccessEvent(rows)
		if err != nil {
			return ExportSummary{}, err
		}
		acc.add(item)
	}
	if err := rows.Err(); err != nil {
		return ExportSummary{}, err
	}
	return acc.summary(), nil
}

func buildAccessEventsSQL(query ExportQuery, ordered bool) (string, []any) {
	var conditions []string
	var args []any

	if !query.Since.IsZero() {
		conditions = append(conditions, "occurred_at >= ?")
		args = append(args, query.Since.Unix())
	}
	if query.Workspace != "" {
		conditions = append(conditions, "(COALESCE(NULLIF(workspace_root, ''), NULLIF(workspace, ''), '') = ? OR COALESCE(workspace_alias, '') = ? OR COALESCE(workspace, '') = ?)")
		args = append(args, query.Workspace, query.Workspace, query.Workspace)
	}
	if query.Backend != "" {
		conditions = append(conditions, "backend = ?")
		args = append(args, query.Backend)
	}
	if query.Operation != "" {
		conditions = append(conditions, "operation = ?")
		args = append(args, query.Operation)
	}
	if query.SessionID != "" {
		conditions = append(conditions, "COALESCE(session_id, '') = ?")
		args = append(args, query.SessionID)
	}
	if query.ClientName != "" {
		conditions = append(conditions, "COALESCE(client_name, '') = ?")
		args = append(args, query.ClientName)
	}
	if query.Route != "" {
		conditions = append(conditions, "COALESCE(route, '') = ?")
		args = append(args, query.Route)
	}
	if query.Format != "" {
		conditions = append(conditions, "COALESCE(format, '') = ?")
		args = append(args, query.Format)
	}
	if query.Truncated != nil {
		conditions = append(conditions, "COALESCE(truncated, 0) = ?")
		args = append(args, boolToInt(*query.Truncated))
	}
	if query.PatternMode != "" {
		conditions = append(conditions, "COALESCE(NULLIF(pattern_mode, ''), 'none') = ?")
		args = append(args, query.PatternMode)
	}
	if query.RoutingOutcome != "" {
		conditions = append(conditions, "COALESCE(NULLIF(routing_outcome, ''), CASE WHEN COALESCE(route, '') = 'direct_fallback' THEN 'direct_fallback' WHEN COALESCE(backend, '') = 'router' THEN 'router_error' WHEN COALESCE(repo, '') != '' THEN 'narrowed_repo' ELSE 'direct' END) = ?")
		args = append(args, query.RoutingOutcome)
	}
	if query.FailureStage != "" {
		conditions = append(conditions, "COALESCE(NULLIF(failure_stage, ''), 'none') = ?")
		args = append(args, query.FailureStage)
	}
	if query.HintCode != "" {
		conditions = append(conditions, "COALESCE(hint_code, '') = ?")
		args = append(args, query.HintCode)
	}
	if query.ErrorsOnly {
		conditions = append(conditions, "(success = 0 OR error_text IS NOT NULL AND error_text != '' OR error_code IS NOT NULL AND error_code != '')")
	}

	sql := `SELECT id, occurred_at, COALESCE(client_name, ''), COALESCE(session_id, ''), COALESCE(seq, 0), COALESCE(workspace, ''), COALESCE(workspace_input, ''), COALESCE(workspace_root, ''), COALESCE(workspace_alias, ''), COALESCE(repo, ''), operation, COALESCE(backend, ''), COALESCE(route, ''), COALESCE(format, ''), COALESCE(token_budget, 0), COALESCE(max_items, 0), COALESCE(max_chars, 0), COALESCE(compress, 0), success, latency_ms, COALESCE(warnings_json, '[]'), COALESCE(runtime_key, ''), COALESCE(entrypoint_id, ''), COALESCE(error_text, ''), COALESCE(error_kind, ''), COALESCE(error_code, ''), COALESCE(truncated, 0), COALESCE(result_count, 0), COALESCE(warning_count, 0), COALESCE(pattern_mode, ''), COALESCE(routing_outcome, ''), COALESCE(failure_stage, ''), COALESCE(hint_code, ''), COALESCE(truncation_reason, ''), COALESCE(decision_json, '') FROM access_events`
	if len(conditions) > 0 {
		sql += " WHERE " + strings.Join(conditions, " AND ")
	}
	if ordered {
		sql += " ORDER BY occurred_at DESC, id DESC"
	}
	if query.Limit > 0 {
		sql += " LIMIT ?"
		args = append(args, query.Limit)
	}
	return sql, args
}

func ComputeExportSummary(events []model.AccessEvent) ExportSummary {
	acc := newSummaryAccumulator()
	for _, raw := range events {
		acc.add(raw)
	}
	return acc.summary()
}

type summaryAccumulator struct {
	total               int
	workspaceBuckets    map[string]*bucket
	operationBuckets    map[string]*bucket
	backendBuckets      map[string]*bucket
	routeBuckets        map[string]*bucket
	clientBuckets       map[string]*bucket
	hintBuckets         map[string]*bucket
	failureStageBuckets map[string]*bucket
	errorMap            map[string]*errorBucket
}

type errorBucket struct {
	kind       string
	code       string
	text       string
	count      int
	workspaces map[string]struct{}
}

func newSummaryAccumulator() *summaryAccumulator {
	return &summaryAccumulator{
		workspaceBuckets:    map[string]*bucket{},
		operationBuckets:    map[string]*bucket{},
		backendBuckets:      map[string]*bucket{},
		routeBuckets:        map[string]*bucket{},
		clientBuckets:       map[string]*bucket{},
		hintBuckets:         map[string]*bucket{},
		failureStageBuckets: map[string]*bucket{},
		errorMap:            map[string]*errorBucket{},
	}
}

func (a *summaryAccumulator) add(raw model.AccessEvent) {
	event := telemetry.NormalizeAccessEvent(raw)
	a.total++
	updateBucket(a.workspaceBuckets, safeKey(telemetry.WorkspaceAnalyticsKey(event), "unscoped"), event)
	updateBucket(a.operationBuckets, safeKey(event.Operation, "unknown"), event)
	updateBucket(a.backendBuckets, safeKey(strings.TrimSpace(event.Backend), "unknown"), event)
	updateBucket(a.routeBuckets, safeKey(event.Route, "unknown"), event)
	updateBucket(a.clientBuckets, safeKey(event.ClientName, "unknown"), event)
	if strings.TrimSpace(event.HintCode) != "" {
		updateBucket(a.hintBuckets, event.HintCode, event)
	}
	updateBucket(a.failureStageBuckets, safeKey(event.FailureStage, "none"), event)

	if event.Error == "" && event.ErrorCode == "" {
		return
	}
	key := safeKey(event.ErrorCode, safeKey(event.Error, "unknown_error"))
	if _, ok := a.errorMap[key]; !ok {
		a.errorMap[key] = &errorBucket{kind: event.ErrorKind, code: event.ErrorCode, text: event.Error, workspaces: map[string]struct{}{}}
	}
	entry := a.errorMap[key]
	entry.count++
	if entry.kind == "" {
		entry.kind = event.ErrorKind
	}
	if entry.code == "" {
		entry.code = event.ErrorCode
	}
	if entry.text == "" {
		entry.text = event.Error
	}
	entry.workspaces[safeKey(telemetry.WorkspaceAnalyticsKey(event), "unscoped")] = struct{}{}
}

func (a *summaryAccumulator) summary() ExportSummary {
	summary := ExportSummary{
		TotalOps:       a.total,
		ByWorkspace:    map[string]WorkspaceStat{},
		ByOperation:    map[string]WorkspaceStat{},
		ByRoute:        map[string]WorkspaceStat{},
		ByClient:       map[string]WorkspaceStat{},
		ByHintCode:     map[string]WorkspaceStat{},
		ByFailureStage: map[string]WorkspaceStat{},
	}
	for key, b := range a.workspaceBuckets {
		summary.ByWorkspace[key] = summarizeBucket(b)
	}
	for key, b := range a.operationBuckets {
		summary.ByOperation[key] = summarizeBucket(b)
	}
	summary.ByBackend = backendHistogramFromBuckets(a.backendBuckets)
	summary.ByOperationPercentiles = operationPercentilesFromBuckets(a.operationBuckets)
	for key, b := range a.routeBuckets {
		summary.ByRoute[key] = summarizeBucket(b)
	}
	for key, b := range a.clientBuckets {
		summary.ByClient[key] = summarizeBucket(b)
	}
	for key, b := range a.hintBuckets {
		summary.ByHintCode[key] = summarizeBucket(b)
	}
	for key, b := range a.failureStageBuckets {
		summary.ByFailureStage[key] = summarizeBucket(b)
	}

	topErrors := make([]ErrorFrequency, 0, len(a.errorMap))
	for _, entry := range a.errorMap {
		wsList := make([]string, 0, len(entry.workspaces))
		for ws := range entry.workspaces {
			wsList = append(wsList, ws)
		}
		sort.Strings(wsList)
		topErrors = append(topErrors, ErrorFrequency{ErrorKind: entry.kind, ErrorCode: entry.code, ErrorText: entry.text, Count: entry.count, Workspaces: wsList})
	}
	sort.Slice(topErrors, func(i, j int) bool { return topErrors[i].Count > topErrors[j].Count })
	if len(topErrors) > 10 {
		topErrors = topErrors[:10]
	}
	summary.TopErrors = topErrors
	summary.Recommendations = ComputeUsageRecommendations(summary)
	return summary
}

func ComputeUsageRecommendations(summary ExportSummary) []UsageRecommendation {
	var recommendations []UsageRecommendation
	add := func(rec UsageRecommendation) {
		for _, existing := range recommendations {
			if existing.ID == rec.ID {
				return
			}
		}
		recommendations = append(recommendations, rec)
	}

	if stat, ok := summary.ByOperation["nav.search"]; ok {
		if stat.Errors > 0 {
			add(UsageRecommendation{
				ID:       "search_errors",
				Severity: "high",
				Reason:   "nav.search has backend/runtime errors in this window; inspect failed searches before broad agent use",
				Command:  "mi-lsp admin export --since 7d --operation nav.search --errors --format toon",
				Evidence: []string{fmt.Sprintf("nav.search errors=%d ops=%d", stat.Errors, stat.Ops)},
			})
		}
		if stat.P50Ms >= 5000 {
			add(UsageRecommendation{
				ID:       "search_latency",
				Severity: "medium",
				Reason:   "nav.search median latency is high; prefer narrower patterns or --repo for follow-up queries",
				Command:  "mi-lsp admin export --since 7d --operation nav.search --summary --by-hint --percentile --format toon",
				Evidence: []string{fmt.Sprintf("nav.search p50_ms=%d ops=%d", stat.P50Ms, stat.Ops)},
			})
		}
	}

	if stat, ok := summary.ByHintCode["search_timeout"]; ok && stat.Ops > 0 {
		add(UsageRecommendation{
			ID:       "search_timeout",
			Severity: "high",
			Reason:   "search timeouts were observed; use repo narrowing or more specific patterns before retrying",
			Command:  "mi-lsp admin export --since 7d --hint-code search_timeout --format toon",
			Evidence: []string{fmt.Sprintf("search_timeout ops=%d", stat.Ops)},
		})
	}

	if stat, ok := summary.ByHintCode["workspace_resolution_failed"]; ok && stat.Ops > 0 {
		add(UsageRecommendation{
			ID:       "workspace_resolution_failed",
			Severity: "high",
			Reason:   "workspace selectors failed to resolve; run doctor before handing aliases to agents",
			Command:  "mi-lsp workspace doctor --format toon",
			Evidence: []string{fmt.Sprintf("workspace_resolution_failed ops=%d", stat.Ops)},
		})
	}

	if stat, ok := summary.ByHintCode["repo_selector_invalid"]; ok && stat.Ops > 0 {
		add(UsageRecommendation{
			ID:       "repo_selector_invalid",
			Severity: "medium",
			Reason:   "repo selectors were invalid; use workspace-map or list concrete repos before scoped search",
			Command:  "mi-lsp nav workspace-map --workspace <alias> --format toon",
			Evidence: []string{fmt.Sprintf("repo_selector_invalid ops=%d", stat.Ops)},
		})
	}

	if stat, ok := summary.ByFailureStage["backend"]; ok && stat.Errors > 0 {
		add(UsageRecommendation{
			ID:       "backend_failures",
			Severity: "medium",
			Reason:   "backend-stage failures were observed; inspect top errors and runtime/tool availability",
			Command:  "mi-lsp admin export --since 7d --errors --summary --by-failure-stage --format toon",
			Evidence: []string{fmt.Sprintf("backend errors=%d ops=%d", stat.Errors, stat.Ops)},
		})
	}

	for _, top := range summary.TopErrors {
		message := strings.ToLower(top.ErrorText + " " + top.ErrorCode)
		if strings.Contains(message, "workspace not found") || strings.Contains(message, "path does not exist") {
			add(UsageRecommendation{
				ID:       "prune_stale_workspaces",
				Severity: "high",
				Reason:   "telemetry includes missing workspace roots or aliases; stale registry entries are likely",
				Command:  "mi-lsp workspace prune --stale --dry-run",
				Evidence: []string{fmt.Sprintf("%dx %s", top.Count, safeKey(top.ErrorCode, top.ErrorText))},
			})
		}
		if strings.Contains(message, "access is denied") || strings.Contains(message, "permission denied") || strings.Contains(message, "process_spawn_access_denied") {
			add(UsageRecommendation{
				ID:       "process_spawn_access_denied",
				Severity: "high",
				Reason:   "a local backend/tool process hit permission errors; verify PATH/MI_LSP_RG or rely on fallback evidence",
				Command:  "mi-lsp admin export --since 7d --errors --format toon",
				Evidence: []string{fmt.Sprintf("%dx %s", top.Count, safeKey(top.ErrorCode, top.ErrorText))},
			})
		}
	}

	sort.SliceStable(recommendations, func(i, j int) bool {
		return recommendationRank(recommendations[i].Severity) > recommendationRank(recommendations[j].Severity)
	})
	if len(recommendations) > 8 {
		recommendations = recommendations[:8]
	}
	return recommendations
}

func recommendationRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

type bucket struct {
	ops       int
	errors    int
	warnings  int
	latencies []int64
}

func updateBucket(buckets map[string]*bucket, key string, event model.AccessEvent) {
	if _, ok := buckets[key]; !ok {
		buckets[key] = &bucket{}
	}
	b := buckets[key]
	b.ops++
	b.latencies = append(b.latencies, event.LatencyMs)
	if !event.Success {
		b.errors++
	}
	if event.WarningCount > 0 {
		b.warnings += event.WarningCount
	} else if len(event.Warnings) > 0 {
		b.warnings += len(event.Warnings)
	}
}

func summarizeBucket(b *bucket) WorkspaceStat {
	sort.Slice(b.latencies, func(i, j int) bool { return b.latencies[i] < b.latencies[j] })
	var totalMs int64
	for _, ms := range b.latencies {
		totalMs += ms
	}
	var avgMs float64
	if b.ops > 0 {
		avgMs = math.Round(float64(totalMs)/float64(b.ops)*10) / 10
	}
	return WorkspaceStat{
		Ops:      b.ops,
		Errors:   b.errors,
		Warnings: b.warnings,
		P50Ms:    percentile(b.latencies, 0.50),
		AvgMs:    avgMs,
	}
}

func safeKey(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func percentile(sorted []int64, pct float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Floor(float64(len(sorted)-1) * pct))
	return sorted[idx]
}

func backendHistogramFromBuckets(buckets map[string]*bucket) []BackendHistogram {
	result := make([]BackendHistogram, 0, len(buckets))
	for backend, bucket := range buckets {
		result = append(result, BackendHistogram{Backend: backend, Count: bucket.ops})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Count == result[j].Count {
			return result[i].Backend < result[j].Backend
		}
		return result[i].Count > result[j].Count
	})

	return result
}

func operationPercentilesFromBuckets(buckets map[string]*bucket) []OperationPercentiles {
	result := make([]OperationPercentiles, 0, len(buckets))
	for op, b := range buckets {
		sort.Slice(b.latencies, func(i, j int) bool { return b.latencies[i] < b.latencies[j] })
		result = append(result, OperationPercentiles{
			Operation: op,
			Count:     b.ops,
			P50Ms:     percentile(b.latencies, 0.50),
			P95Ms:     percentile(b.latencies, 0.95),
			P99Ms:     percentile(b.latencies, 0.99),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Count == result[j].Count {
			return result[i].Operation < result[j].Operation
		}
		return result[i].Count > result[j].Count
	})

	return result
}

// ComputeBackendHistogram groups events by backend and counts occurrences
func ComputeBackendHistogram(events []model.AccessEvent) []BackendHistogram {
	counts := map[string]int{}
	for _, e := range events {
		backend := safeKey(strings.TrimSpace(e.Backend), "unknown")
		counts[backend]++
	}

	result := make([]BackendHistogram, 0, len(counts))
	for backend, count := range counts {
		result = append(result, BackendHistogram{Backend: backend, Count: count})
	}

	// Sort by count descending, then by backend name ascending
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count == result[j].Count {
			return result[i].Backend < result[j].Backend
		}
		return result[i].Count > result[j].Count
	})

	return result
}

// ComputeOperationPercentiles calculates p50/p95/p99 latency per operation
func ComputeOperationPercentiles(events []model.AccessEvent) []OperationPercentiles {
	opBuckets := map[string]*bucket{}

	for _, e := range events {
		op := safeKey(e.Operation, "unknown")
		if _, ok := opBuckets[op]; !ok {
			opBuckets[op] = &bucket{}
		}
		opBuckets[op].latencies = append(opBuckets[op].latencies, e.LatencyMs)
		opBuckets[op].ops++
		if !e.Success {
			opBuckets[op].errors++
		}
	}

	result := make([]OperationPercentiles, 0, len(opBuckets))
	for op, b := range opBuckets {
		// Sort latencies to compute percentiles
		sort.Slice(b.latencies, func(i, j int) bool { return b.latencies[i] < b.latencies[j] })

		result = append(result, OperationPercentiles{
			Operation: op,
			Count:     b.ops,
			P50Ms:     percentile(b.latencies, 0.50),
			P95Ms:     percentile(b.latencies, 0.95),
			P99Ms:     percentile(b.latencies, 0.99),
		})
	}

	// Sort by count descending, then by operation name ascending
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count == result[j].Count {
			return result[i].Operation < result[j].Operation
		}
		return result[i].Count > result[j].Count
	})

	return result
}

func RenderSummaryTable(summary ExportSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Total operations: %d\n", summary.TotalOps)
	if summary.WindowLabel != "" {
		fmt.Fprintf(&b, "Window: %s\n", summary.WindowLabel)
	}
	b.WriteString("\n")
	renderStatsTable(&b, "By workspace", "Workspace", summary.ByWorkspace)
	if len(summary.ByOperation) > 0 {
		b.WriteString("\n")
		renderStatsTable(&b, "By operation", "Operation", summary.ByOperation)
	}
	if len(summary.ByRoute) > 0 {
		b.WriteString("\n")
		renderStatsTable(&b, "By route", "Route", summary.ByRoute)
	}
	if len(summary.ByClient) > 0 {
		b.WriteString("\n")
		renderStatsTable(&b, "By client", "Client", summary.ByClient)
	}
	if len(summary.ByHintCode) > 0 {
		b.WriteString("\n")
		renderStatsTable(&b, "By hint code", "Hint code", summary.ByHintCode)
	}
	if len(summary.ByFailureStage) > 0 {
		b.WriteString("\n")
		renderStatsTable(&b, "By failure stage", "Failure stage", summary.ByFailureStage)
	}

	if len(summary.ByBackend) > 0 {
		fmt.Fprintf(&b, "\n By backend:\n")
		fmt.Fprintf(&b, " %-20s | %6s\n", "Backend", "Count")
		fmt.Fprintf(&b, " %s\n", strings.Repeat("-", 30))
		for _, bh := range summary.ByBackend {
			fmt.Fprintf(&b, " %-20s | %6d\n", bh.Backend, bh.Count)
		}
	}

	if len(summary.ByOperationPercentiles) > 0 {
		fmt.Fprintf(&b, "\n Latency percentiles by operation:\n")
		fmt.Fprintf(&b, " %-25s | %5s | %6s | %6s | %6s\n", "Operation", "Count", "P50ms", "P95ms", "P99ms")
		fmt.Fprintf(&b, " %s\n", strings.Repeat("-", 65))
		for _, op := range summary.ByOperationPercentiles {
			fmt.Fprintf(&b, " %-25s | %5d | %6d | %6d | %6d\n", op.Operation, op.Count, op.P50Ms, op.P95Ms, op.P99Ms)
		}
	}

	if len(summary.TopErrors) > 0 {
		fmt.Fprintf(&b, "\n Top errors:\n")
		for _, e := range summary.TopErrors {
			label := safeKey(e.ErrorCode, safeKey(e.ErrorText, "unknown_error"))
			if e.ErrorKind != "" {
				label = e.ErrorKind + "/" + label
			}
			fmt.Fprintf(&b, "  %dx %q (%s)\n", e.Count, label, strings.Join(e.Workspaces, ", "))
		}
	}
	if len(summary.Recommendations) > 0 {
		fmt.Fprintf(&b, "\n Recommendations:\n")
		for _, rec := range summary.Recommendations {
			fmt.Fprintf(&b, "  [%s] %s: %s\n", rec.Severity, rec.ID, rec.Reason)
			if rec.Command != "" {
				fmt.Fprintf(&b, "      %s\n", rec.Command)
			}
		}
	}
	return b.String()
}

func renderStatsTable(builder *strings.Builder, title string, header string, stats map[string]WorkspaceStat) {
	if len(stats) == 0 {
		return
	}
	fmt.Fprintf(builder, " %s\n", title)
	names := make([]string, 0, len(stats))
	for key := range stats {
		names = append(names, key)
	}
	sort.Slice(names, func(i, j int) bool {
		if stats[names[i]].Ops == stats[names[j]].Ops {
			return names[i] < names[j]
		}
		return stats[names[i]].Ops > stats[names[j]].Ops
	})

	fmt.Fprintf(builder, " %-20s | %5s | %6s | %4s | %6s\n", header, "Ops", "Errors", "Warn", "P50ms")
	fmt.Fprintf(builder, " %s\n", strings.Repeat("-", 60))
	for _, name := range names {
		stat := stats[name]
		fmt.Fprintf(builder, " %-20s | %5d | %6d | %4d | %6d\n", name, stat.Ops, stat.Errors, stat.Warnings, stat.P50Ms)
	}
}

func RenderCSV(events []model.AccessEvent) string {
	var b strings.Builder
	b.WriteString("id,occurred_at,client_name,session_id,seq,workspace,workspace_input,workspace_root,workspace_alias,repo,operation,backend,route,format,token_budget,max_items,max_chars,compress,success,latency_ms,error,error_kind,error_code,result_count,truncated,warning_count,pattern_mode,routing_outcome,failure_stage,hint_code,truncation_reason,decision_json\n")
	for _, raw := range events {
		e := telemetry.NormalizeAccessEvent(raw)
		fmt.Fprintf(&b, "%d,%s,%s,%s,%d,%s,%s,%s,%s,%s,%s,%s,%s,%s,%d,%d,%d,%t,%t,%d,%s,%s,%s,%d,%t,%d,%s,%s,%s,%s,%s,%s\n",
			e.ID, e.OccurredAt.Format(time.RFC3339), e.ClientName, e.SessionID, e.Seq,
			e.Workspace, csvEscape(e.WorkspaceInput), csvEscape(e.WorkspaceRoot), csvEscape(e.WorkspaceAlias), e.Repo, e.Operation, e.Backend,
			e.Route, e.Format, e.TokenBudget, e.MaxItems, e.MaxChars, e.Compress,
			e.Success, e.LatencyMs, csvEscape(e.Error), csvEscape(e.ErrorKind), csvEscape(e.ErrorCode), e.ResultCount, e.Truncated,
			e.WarningCount, e.PatternMode, e.RoutingOutcome, e.FailureStage, e.HintCode, e.TruncationReason, csvEscape(e.DecisionJSON))
	}
	return b.String()
}

func RenderTOON(events []model.AccessEvent) (string, error) {
	normalized := make([]model.AccessEvent, len(events))
	for i, event := range events {
		normalized[i] = telemetry.NormalizeAccessEvent(event)
	}
	return renderTOONMap(map[string]any{
		"ok":      true,
		"backend": "admin-export",
		"items":   normalized,
		"stats": map[string]any{
			"events": len(normalized),
		},
	})
}

func RenderSummaryTOON(summary ExportSummary) (string, error) {
	return renderTOONMap(map[string]any{
		"ok":      true,
		"backend": "admin-export-summary",
		"items":   []ExportSummary{summary},
		"stats": map[string]any{
			"events": summary.TotalOps,
		},
	})
}

func renderTOONMap(payload map[string]any) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return "", err
	}
	rendered, err := toon.Marshal(generic)
	if err != nil {
		return "", err
	}
	return string(rendered) + "\n", nil
}

func csvEscape(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}
