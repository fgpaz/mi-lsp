package telemetry

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const (
	Followed    = "followed"
	Ignored     = "ignored"
	NoneEmitted = "none_emitted"
)

// UsageReport is an aggregate. It never includes query bodies or file contents.
type UsageReport struct {
	Calls            int                `json:"calls"`
	ByHarness        map[string]int     `json:"by_harness"`
	Sessions         int                `json:"sessions"`
	TopOperations    []UsageOpStat      `json:"top_operations"`
	FollowThrough    UsageFollowThrough `json:"follow_through"`
	FallbackByReason map[string]int     `json:"fallback_by_reason"`
	FallbackRate     float64            `json:"fallback_rate"`
	LatencyP50Ms     int64              `json:"latency_p50_ms"`
	LatencyP95Ms     int64              `json:"latency_p95_ms"`
	EmptyResults     int                `json:"empty_results"`
	PartialResults   int                `json:"partial_results"`
	StaleGraphHits   int                `json:"stale_graph_hits"`
}

type UsageOpStat struct {
	Operation string `json:"operation"`
	Calls     int    `json:"calls"`
}

type UsageFollowThrough struct {
	Followed    int     `json:"followed"`
	Ignored     int     `json:"ignored"`
	NoneEmitted int     `json:"none_emitted"`
	Rate        float64 `json:"rate"`
}

type usageDecision struct {
	ContinuationOp     string `json:"continuation_op"`
	FallbackReasonCode string `json:"fallback_reason_code"`
	ResultEmpty        bool   `json:"result_empty"`
	PartialResult      bool   `json:"partial_result"`
	StaleGraph         bool   `json:"stale_graph"`
	MemoryStale        bool   `json:"memory_stale"`
}

// BuildUsageReport aggregates access events. Follow-through compares each
// call's continuation.next operation with the next call in the same session.
func BuildUsageReport(events []model.AccessEvent) UsageReport {
	report := UsageReport{
		ByHarness:        map[string]int{},
		FallbackByReason: map[string]int{},
	}
	sessions := map[string]struct{}{}
	ops := map[string]int{}
	latencies := make([]int64, 0, len(events))
	ordered := append([]model.AccessEvent(nil), events...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].SessionID != ordered[j].SessionID {
			return ordered[i].SessionID < ordered[j].SessionID
		}
		if ordered[i].Seq != ordered[j].Seq {
			return ordered[i].Seq < ordered[j].Seq
		}
		return ordered[i].OccurredAt.Before(ordered[j].OccurredAt)
	})
	for i, event := range ordered {
		report.Calls++
		harness := strings.TrimSpace(event.ClientName)
		if harness == "" {
			harness = "manual-cli"
		}
		report.ByHarness[harness]++
		if event.SessionID != "" {
			sessions[event.SessionID] = struct{}{}
		}
		op := strings.TrimSpace(event.Operation)
		if op == "" {
			op = "unknown"
		}
		ops[op]++
		latencies = append(latencies, event.LatencyMs)
		decision := parseUsageDecision(event.DecisionJSON)
		if decision.ResultEmpty || (event.Success && event.ResultCount == 0) {
			report.EmptyResults++
		}
		if decision.PartialResult || event.Truncated || (event.TruncationReason != "" && event.TruncationReason != "none") {
			report.PartialResults++
		}
		if decision.StaleGraph || decision.MemoryStale || strings.Contains(strings.ToLower(event.HintCode+" "+event.ErrorCode), "stale") {
			report.StaleGraphHits++
		}
		reason := decision.FallbackReasonCode
		if reason == "" {
			reason = strings.TrimSpace(event.FallbackReasonCode)
		}
		if model.ValidIntentFallbackReasonCode(reason) {
			report.FallbackByReason[reason]++
		}
		signal := NoneEmitted
		if decision.ContinuationOp != "" {
			signal = Ignored
			if i+1 < len(ordered) && ordered[i+1].SessionID == event.SessionID && sameSessionFollows(ordered[i+1], decision.ContinuationOp) {
				signal = Followed
			}
		}
		switch signal {
		case Followed:
			report.FollowThrough.Followed++
		case Ignored:
			report.FollowThrough.Ignored++
		default:
			report.FollowThrough.NoneEmitted++
		}
	}
	report.Sessions = len(sessions)
	denom := report.FollowThrough.Followed + report.FollowThrough.Ignored
	if denom > 0 {
		report.FollowThrough.Rate = float64(report.FollowThrough.Followed) / float64(denom)
	}
	if report.Calls > 0 {
		report.FallbackRate = float64(sumMap(report.FallbackByReason)) / float64(report.Calls)
	}
	report.LatencyP50Ms, report.LatencyP95Ms = percentile(latencies, 50), percentile(latencies, 95)
	report.TopOperations = topOps(ops, 10)
	return report
}

func parseUsageDecision(raw string) usageDecision {
	var decision usageDecision
	if strings.TrimSpace(raw) == "" {
		return decision
	}
	_ = json.Unmarshal([]byte(raw), &decision)
	return decision
}

func sameSessionFollows(next model.AccessEvent, continuationOp string) bool {
	op := strings.TrimSpace(next.Operation)
	target := strings.TrimSpace(continuationOp)
	if op == "" || target == "" {
		return false
	}
	if op == target {
		return true
	}
	normalized := strings.ReplaceAll(op, ".", " ")
	return strings.Contains(target, op) || strings.Contains(target, normalized)
}

func percentile(values []int64, p int) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := (len(sorted) - 1) * p / 100
	return sorted[idx]
}

func topOps(ops map[string]int, limit int) []UsageOpStat {
	out := make([]UsageOpStat, 0, len(ops))
	for op, count := range ops {
		out = append(out, UsageOpStat{Operation: op, Calls: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Calls != out[j].Calls {
			return out[i].Calls > out[j].Calls
		}
		return out[i].Operation < out[j].Operation
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func sumMap(values map[string]int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

// UsageSince parses a window like 7d or 24h. Zero means no lower bound.
func UsageSince(spec string, now time.Time) (time.Time, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return time.Time{}, nil
	}
	if len(spec) < 2 {
		return time.Time{}, errUsageWindow
	}
	unit := spec[len(spec)-1]
	number := spec[:len(spec)-1]
	var n int
	for _, r := range number {
		if r < '0' || r > '9' {
			return time.Time{}, errUsageWindow
		}
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return time.Time{}, errUsageWindow
	}
	switch unit {
	case 'd':
		return now.Add(-time.Duration(n) * 24 * time.Hour), nil
	case 'h':
		return now.Add(-time.Duration(n) * time.Hour), nil
	default:
		return time.Time{}, errUsageWindow
	}
}

var errUsageWindow = errString("invalid since window; use Nd or Nh")

type errString string

func (e errString) Error() string { return string(e) }
