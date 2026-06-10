package cli

import (
	"context"
	"runtime/debug"
	"time"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/daemon"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/service"
)

func newDoctorCommand(state *rootState) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run self-checks to diagnose mi-lsp health",
		Long: `Run read-only self-checks to diagnose potential issues with mi-lsp.
Each check reports a severity level (P1/P2/P3) and whether it passed.
The command exits with code 0 if no P1 issues are found, and 1 if any P1 issues exist.

Checks include:
  - stale-aliases: Verifies registry aliases point to existing paths
  - daemon-version-drift: Compares CLI and daemon versions
  - daemon-db-size: Checks if daemon.db exceeds size threshold (500 MB)
  - watched-dirs-handle-limit: Verifies watched dirs do not exceed OS handle limit
  - binary-dirty: Verifies executable is not marked +dirty from release
  - governance-blocked: Checks for governance blocking conditions
  - truncation-rate: Monitors if truncation rate is high`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			// Populate daemon info if available
			opts := &service.RunDoctorOptions{}
			daemonVersion, watchedDirs := getDoctorDaemonInfo(ctx)
			opts.DaemonVersion = daemonVersion
			opts.DaemonWatchedDirs = watchedDirs

			report, err := service.RunDoctor(ctx, opts)
			if err != nil {
				return err
			}

			// Convert DoctorReport to Envelope format
			envelope := model.Envelope{
				Ok:      report.ExitCode == 0,
				Backend: "doctor",
				Items:   report.Checks,
				Stats: model.Stats{
					Ms: 0, // Would be filled in by the caller if needed
				},
			}
			if report.Summary != "" {
				envelope.Warnings = append(envelope.Warnings, report.Summary)
			}

			// Print the envelope
			queryOpts := state.queryOptions(cmd, "doctor", nil)
			if err := state.printEnvelope(envelope, queryOpts); err != nil {
				return err
			}

			// Exit with the appropriate code
			if report.ExitCode != 0 {
				return &envelopePrintedError{err: ErrExitCode(report.ExitCode)}
			}

			return nil
		},
	}
}

// ErrExitCode creates an error that signals a specific exit code.
// The root command will handle this and exit with the correct code.
type exitCodeError struct {
	code int
}

func (e exitCodeError) Error() string {
	if e.code == 0 {
		return ""
	}
	return ""
}

func ErrExitCode(code int) error {
	return exitCodeError{code: code}
}

// IsExitCodeError checks if an error is an exit code error.
func IsExitCodeError(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	code := extractExitCode(err)
	if code > 0 {
		return code, true
	}
	return 0, false
}

func extractExitCode(err error) int {
	// Try to cast to exitCodeError
	if e, ok := err.(exitCodeError); ok {
		return e.code
	}
	return 0
}

// getDoctorDaemonInfo tries to fetch daemon version and watched dirs.
func getDoctorDaemonInfo(ctx context.Context) (string, int) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	response, err := daemon.NewClient().Execute(ctx, model.CommandRequest{
		ProtocolVersion: model.ProtocolVersion,
		Operation:       "system.status",
		Context: model.QueryOptions{
			Format: "compact",
		},
	})
	if err != nil {
		return "", 0
	}

	var version string
	var watchedDirs int

	// Try to extract version and watcher stats from response
	if items, ok := response.Items.([]any); ok && len(items) > 0 {
		if item, ok := items[0].(map[string]any); ok {
			if state, ok := item["state"].(map[string]any); ok {
				// Extract version
				if v, ok := state["version"].(string); ok {
					version = v
				}
				// Extract watcher stats if available
				if watchers, ok := state["watchers"].(map[string]any); ok {
					if wd, ok := watchers["watched_dirs"].(float64); ok {
						watchedDirs = int(wd)
					}
				}
			}
		}
	}

	return version, watchedDirs
}

// getCLIVersionForComparison returns the CLI version for comparison with daemon.
func getCLIVersionForComparison() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	version := info.Main.Version
	if version == "" {
		return "unknown"
	}
	return version
}

// getVCSModifiedForDoctor returns the vcs.modified build setting.
func getVCSModifiedForDoctor() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.modified" {
			return setting.Value
		}
	}
	return ""
}
