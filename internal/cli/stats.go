package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/daemon"
	"github.com/fgpaz/mi-lsp/internal/model"
)

type statsBucket struct {
	Calls       int     `json:"calls"`
	OKPct       float64 `json:"ok_pct"`
	P50Ms       int64   `json:"p50_ms"`
	P90Ms       int64   `json:"p90_ms"`
	P50Bytes    int64   `json:"p50_bytes"`
	P90Bytes    int64   `json:"p90_bytes"`
	FallbackPct float64 `json:"fallback_pct"`
	ok          int
	fallbacks   int
	latencies   []int64
	bytes       []int64
}

type statsReport struct {
	Days       int                    `json:"days"`
	TotalCalls int                    `json:"total_calls"`
	ByClient   map[string]statsBucket `json:"by_client,omitempty"`
	ByOp       map[string]statsBucket `json:"by_op,omitempty"`
}

func newStatsCommand() *cobra.Command {
	var days int
	var byClient, byOp bool
	var format string
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Summarize local mi-lsp usage telemetry",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if days <= 0 {
				return fmt.Errorf("--days must be greater than zero")
			}
			if !byClient && !byOp {
				byClient = true
			}
			s, err := daemon.OpenTelemetryStore()
			if err != nil {
				return fmt.Errorf("cannot open telemetry store: %w", err)
			}
			defer s.Close()
			events, err := daemon.QueryAccessEvents(s, daemon.ExportQuery{Since: time.Now().Add(-time.Duration(days) * 24 * time.Hour)})
			if err != nil {
				return fmt.Errorf("query telemetry: %w", err)
			}
			report := statsReport{Days: days, TotalCalls: len(events)}
			if byClient {
				report.ByClient = aggregateStats(events, func(e model.AccessEvent) string {
					if strings.TrimSpace(e.ClientName) == "" {
						return "unknown"
					}
					return e.ClientName
				})
			}
			if byOp {
				report.ByOp = aggregateStats(events, func(e model.AccessEvent) string {
					if strings.TrimSpace(e.Operation) == "" {
						return "unknown"
					}
					return e.Operation
				})
			}
			var body []byte
			switch strings.ToLower(format) {
			case "json":
				body, err = json.MarshalIndent(report, "", "  ")
			case "compact":
				body, err = json.Marshal(report)
			default:
				return fmt.Errorf("invalid --format %q; valid options: json, compact", format)
			}
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(os.Stdout, string(body))
			return err
		},
	}
	cmd.Flags().BoolVar(&byClient, "by-client", false, "Break down calls, success, latency, output bytes and fallback by client")
	cmd.Flags().BoolVar(&byOp, "by-op", false, "Break down calls by operation")
	cmd.Flags().IntVar(&days, "days", 7, "Number of days to include")
	cmd.Flags().StringVar(&format, "format", "json", "Output format: json or compact")
	return cmd
}

func aggregateStats(events []model.AccessEvent, key func(model.AccessEvent) string) map[string]statsBucket {
	buckets := make(map[string]*statsBucket)
	for _, event := range events {
		name := key(event)
		bucket := buckets[name]
		if bucket == nil {
			bucket = &statsBucket{}
			buckets[name] = bucket
		}
		bucket.Calls++
		if event.Success {
			bucket.ok++
		}
		if event.Route == "direct_fallback" || strings.Contains(strings.ToLower(event.RoutingOutcome), "fallback") {
			bucket.fallbacks++
		}
		bucket.latencies = append(bucket.latencies, event.LatencyMs)
		bucket.bytes = append(bucket.bytes, int64(event.BytesOut))
	}
	result := make(map[string]statsBucket, len(buckets))
	for name, bucket := range buckets {
		if bucket.Calls > 0 {
			bucket.OKPct = 100 * float64(bucket.ok) / float64(bucket.Calls)
			bucket.FallbackPct = 100 * float64(bucket.fallbacks) / float64(bucket.Calls)
		}
		bucket.P50Ms = percentile(bucket.latencies, .50)
		bucket.P90Ms = percentile(bucket.latencies, .90)
		bucket.P50Bytes = percentile(bucket.bytes, .50)
		bucket.P90Bytes = percentile(bucket.bytes, .90)
		bucket.latencies = nil
		bucket.bytes = nil
		bucket.ok = 0
		bucket.fallbacks = 0
		result[name] = *bucket
	}
	return result
}

func percentile(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := int(float64(len(sorted)-1)*p + .5)
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}
