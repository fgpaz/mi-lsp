package daemon

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/telemetry"
)

type ExportQuery struct {
	Since       time.Time
	Workspace   string
	Backend     string
	ErrorsOnly  bool
	Limit       int
	WindowLabel string
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

type ExportSummary struct {
	TotalOps               int                      `json:"total_ops"`
	WindowLabel            string                   `json:"window_label,omitempty"`
	ByWorkspace            map[string]WorkspaceStat `json:"by_workspace"`
	ByOperation            map[string]WorkspaceStat `json:"by_operation,omitempty"`
	TopErrors              []ErrorFrequency         `json:"top_errors"`
	ByBackend              []BackendHistogram       `json:"by_backend,omitempty"`
	ByOperationPercentiles []OperationPercentiles   `json:"by_operation_percentiles,omitempty"`
}

func QueryAccessEvents(store *TelemetryStore, query ExportQuery) ([]model.AccessEvent, error) {
	if query.Limit < 0 {
		query.Limit = 500
	}

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
	if query.ErrorsOnly {
		conditions = append(conditions, "(success = 0 OR error_text IS NOT NULL AND error_text != '' OR error_code IS NOT NULL AND error_code != '')")
	}

	sql := `SELECT id, occurred_at, COALESCE(client_name, ''), COALESCE(session_id, ''), COALESCE(seq, 0), COALESCE(workspace, ''), COALESCE(workspace_input, ''), COALESCE(workspace_root, ''), COALESCE(workspace_alias, ''), COALESCE(repo, ''), operation, COALESCE(backend, ''), COALESCE(route, ''), COALESCE(format, ''), COALESCE(token_budget, 0), COALESCE(max_items, 0), COALESCE(max_chars, 0), COALESCE(compress, 0), success, latency_ms, COALESCE(warnings_json, '[]'), COALESCE(runtime_key, ''), COALESCE(entrypoint_id, ''), COALESCE(error_text, ''), COALESCE(error_kind, ''), COALESCE(error_code, ''), COALESCE(truncated, 0), COALESCE(result_count, 0) FROM access_events`
	if len(conditions) > 0 {
		sql += " WHERE " + strings.Join(conditions, " AND ")
	}
	sql += " ORDER BY occurred_at DESC, id DESC"
	if query.Limit > 0 {
		sql += " LIMIT ?"
		args = append(args, query.Limit)
	}

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

func ComputeExportSummary(events []model.AccessEvent) ExportSummary {
	summary := ExportSummary{
		TotalOps:    len(events),
		ByWorkspace: map[string]WorkspaceStat{},
		ByOperation: map[string]WorkspaceStat{},
	}

	workspaceBuckets := map[string]*bucket{}
	operationBuckets := map[string]*bucket{}
	errorMap := map[string]*struct {
		kind       string
		code       string
		text       string
		count      int
		workspaces map[string]struct{}
	}{}

	for _, raw := range events {
		event := telemetry.NormalizeAccessEvent(raw)
		updateBucket(workspaceBuckets, safeKey(telemetry.WorkspaceAnalyticsKey(event), "unscoped"), event)
		updateBucket(operationBuckets, safeKey(event.Operation, "unknown"), event)

		if event.Error != "" || event.ErrorCode != "" {
			key := safeKey(event.ErrorCode, safeKey(event.Error, "unknown_error"))
			if _, ok := errorMap[key]; !ok {
				errorMap[key] = &struct {
					kind       string
					code       string
					text       string
					count      int
					workspaces map[string]struct{}
				}{kind: event.ErrorKind, code: event.ErrorCode, text: event.Error, workspaces: map[string]struct{}{}}
			}
			entry := errorMap[key]
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
	}

	for key, b := range workspaceBuckets {
		summary.ByWorkspace[key] = summarizeBucket(b)
	}
	for key, b := range operationBuckets {
		summary.ByOperation[key] = summarizeBucket(b)
	}

	topErrors := make([]ErrorFrequency, 0, len(errorMap))
	for _, entry := range errorMap {
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
	return summary
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
	if len(event.Warnings) > 0 {
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
	b.WriteString("id,occurred_at,client_name,session_id,seq,workspace,workspace_input,workspace_root,workspace_alias,repo,operation,backend,route,format,token_budget,max_items,max_chars,compress,success,latency_ms,error,error_kind,error_code,result_count,truncated\n")
	for _, raw := range events {
		e := telemetry.NormalizeAccessEvent(raw)
		fmt.Fprintf(&b, "%d,%s,%s,%s,%d,%s,%s,%s,%s,%s,%s,%s,%s,%s,%d,%d,%d,%t,%t,%d,%s,%s,%s,%d,%t\n",
			e.ID, e.OccurredAt.Format(time.RFC3339), e.ClientName, e.SessionID, e.Seq,
			e.Workspace, csvEscape(e.WorkspaceInput), csvEscape(e.WorkspaceRoot), csvEscape(e.WorkspaceAlias), e.Repo, e.Operation, e.Backend,
			e.Route, e.Format, e.TokenBudget, e.MaxItems, e.MaxChars, e.Compress,
			e.Success, e.LatencyMs, csvEscape(e.Error), csvEscape(e.ErrorKind), csvEscape(e.ErrorCode), e.ResultCount, e.Truncated)
	}
	return b.String()
}

func csvEscape(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}
