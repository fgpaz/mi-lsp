package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/daemon"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/telemetry"
)

func newAdminCommand(state *rootState) *cobra.Command {
	command := &cobra.Command{
		Use:   "admin",
		Short: "Inspect or open the local governance UI",
		Long: `Access the local governance UI served by the daemon.
Provides runtime dashboards, access logs, and workspace status.`,
	}

	statusCommand := &cobra.Command{
		Use:   "status",
		Short: "Return daemon state including admin URL",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			response, err := daemon.NewClient().Execute(ctx, model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "system.status", Context: state.queryOptions()})
			if err != nil {
				return daemon.BuildStatusError()
			}
			return state.printEnvelope(response)
		},
	}

	openCommand := &cobra.Command{
		Use:   "open",
		Short: "Open the governance UI in the system browser",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			response, err := daemon.NewClient().Execute(ctx, model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "system.status", Context: state.queryOptions()})
			if err != nil {
				return daemon.BuildStatusError()
			}
			baseURL, err := adminURLFromEnvelope(response)
			if err != nil {
				return err
			}
			finalURL, err := buildAdminURL(baseURL, state.workspace, "overview")
			if err != nil {
				return err
			}
			if err := openURL(finalURL); err != nil {
				return err
			}
			return state.printEnvelope(model.Envelope{Ok: true, Backend: "admin", Items: []map[string]any{{"admin_url": finalURL}}})
		},
	}

	exportCommand := newExportCommand(state)
	command.AddCommand(statusCommand, openCommand, exportCommand)
	return command
}

func newExportCommand(state *rootState) *cobra.Command {
	var (
		sinceFlag      string
		workspaceFlag  string
		backendFlag    string
		errorsOnly     bool
		formatFlag     string
		summaryFlag    bool
		limitFlag      int
		recentFlag     bool
		percentileFlag bool
		byBackendFlag  bool
	)

	command := &cobra.Command{
		Use:   "export",
		Short: "Export telemetry data from all workspaces",
		Long:  "Query and export access_events from daemon.db for diagnostics, error detection, and usage analysis across all workspaces.",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := daemon.OpenTelemetryStore()
			if err != nil {
				return fmt.Errorf("cannot open telemetry store: %w", err)
			}
			defer store.Close()

			window, err := resolveExportWindow(cmd, sinceFlag, recentFlag)
			if err != nil {
				return err
			}

			query := daemon.ExportQuery{
				Since:       window.Since,
				Workspace:   workspaceFlag,
				Backend:     backendFlag,
				ErrorsOnly:  errorsOnly,
				Limit:       limitFlag,
				WindowLabel: window.Label,
			}

			events, err := daemon.QueryAccessEvents(store, query)
			if err != nil {
				return fmt.Errorf("query failed: %w", err)
			}

			if summaryFlag {
				summary := daemon.ComputeExportSummary(events)
				summary.WindowLabel = window.Label
				if byBackendFlag {
					summary.ByBackend = daemon.ComputeBackendHistogram(events)
				}
				if percentileFlag {
					summary.ByOperationPercentiles = daemon.ComputeOperationPercentiles(events)
				}
				fmt.Fprint(os.Stdout, daemon.RenderSummaryTable(summary))
				return nil
			}

			switch formatFlag {
			case "json":
				body, err := json.MarshalIndent(events, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(os.Stdout, string(body))
			case "csv":
				fmt.Fprint(os.Stdout, daemon.RenderCSV(events))
			case "compact":
				for _, e := range events {
					status := "ok"
					if !e.Success {
						status = "FAIL"
					}
					extra := ""
					if e.ErrorCode != "" {
						extra += " code=" + e.ErrorCode
					}
					if e.Error != "" {
						extra += " err=" + e.Error
					}
					fmt.Fprintf(os.Stdout, "[%s] %s %s ws=%s backend=%s %dms%s\n",
						status, e.OccurredAt.Format("2006-01-02T15:04"), e.Operation,
						e.Workspace, e.Backend, e.LatencyMs, extra)
				}
			default:
				return fmt.Errorf("invalid --format %q; valid options: json, csv, compact", formatFlag)
			}
			return nil
		},
	}

	command.Flags().StringVar(&sinceFlag, "since", "7d", "Time window: e.g. 7d, 24h, 30d")
	command.Flags().BoolVar(&recentFlag, "recent", false, "Shortcut for the recent 24h telemetry window")
	command.Flags().StringVar(&workspaceFlag, "workspace", "", "Filter by workspace name")
	command.Flags().StringVar(&backendFlag, "backend", "", "Filter by backend type")
	command.Flags().BoolVar(&errorsOnly, "errors", false, "Show only failed operations")
	command.Flags().StringVar(&formatFlag, "format", "json", "Output format: json, csv, compact")
	command.Flags().BoolVar(&summaryFlag, "summary", false, "Show aggregated summary table instead of raw events")
	command.Flags().IntVar(&limitFlag, "limit", 500, "Maximum number of events to return")
	command.Flags().BoolVar(&percentileFlag, "percentile", false, "Show p50/p95/p99 latency breakdown by operation (requires --summary)")
	command.Flags().BoolVar(&byBackendFlag, "by-backend", false, "Show backend usage histogram (requires --summary)")

	return command
}

func resolveExportWindow(cmd *cobra.Command, sinceFlag string, recentFlag bool) (telemetry.Window, error) {
	if recentFlag && cmd != nil && cmd.Flags().Changed("since") {
		return telemetry.Window{}, fmt.Errorf("--recent cannot be combined with an explicit --since")
	}
	if recentFlag {
		return telemetry.ResolveWindow("recent", time.Now())
	}
	window, err := telemetry.ResolveWindow(sinceFlag, time.Now())
	if err != nil {
		return telemetry.Window{}, fmt.Errorf("invalid --since %q: %w", sinceFlag, err)
	}
	return window, nil
}

func parseSinceDuration(s string) (time.Time, error) {
	window, err := telemetry.ResolveWindow(s, time.Now())
	if err != nil {
		return time.Time{}, fmt.Errorf("unsupported duration format %q; use e.g. 7d, 24h, 30m", s)
	}
	return window.Since, nil
}

func adminURLFromEnvelope(response model.Envelope) (string, error) {
	body, err := json.Marshal(response.Items)
	if err != nil {
		return "", err
	}
	var items []map[string]any
	if err := json.Unmarshal(body, &items); err != nil || len(items) == 0 {
		return "", errors.New("daemon status did not include state metadata")
	}
	stateBody, err := json.Marshal(items[0]["state"])
	if err != nil {
		return "", err
	}
	var state model.DaemonState
	if err := json.Unmarshal(stateBody, &state); err != nil {
		return "", err
	}
	if state.AdminURL == "" {
		return "", errors.New("daemon did not expose an admin url")
	}
	return state.AdminURL, nil
}

func buildAdminURL(baseURL string, workspace string, panel string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	if workspace != "" {
		query.Set("workspace", workspace)
	}
	if panel != "" {
		query.Set("panel", panel)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func openURL(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("cmd", "/c", "start", "", url)
	case "darwin":
		command = exec.Command("open", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	return command.Start()
}
