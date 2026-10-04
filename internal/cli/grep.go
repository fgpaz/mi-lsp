package cli

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/grepx"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/telemetry"
)

// splitGrepArgs removes the two flags owned by mi-lsp (--rg-compat and
// --annotate) from the rg arguments. Everything after a bare `--` is rg's.
func splitGrepArgs(args []string) (rest []string, rgCompat bool, annotate bool) {
	rest = make([]string, 0, len(args))
	for i, arg := range args {
		if arg == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		switch arg {
		case "--rg-compat":
			rgCompat = true
		case "--annotate":
			annotate = true
		default:
			rest = append(rest, arg)
		}
	}
	return rest, rgCompat, annotate
}

// newGrepCommand is `mi-lsp grep [rg args...]` (contract grep-v1). It bypasses
// the envelope and the daemon: output and exit code are rg's, with an optional
// per-line annotation.
func newGrepCommand(state *rootState) *cobra.Command {
	return &cobra.Command{
		Use:                "grep [rg args...]",
		Short:              "ripgrep passthrough with per-line symbol annotation (grep-v1)",
		Long:               "Runs the real rg (MI_LSP_RG or PATH) with the arguments as given. When stdout goes to an agent, match lines in indexed code get a suffix \\t⟦<def|ref|com|str> <container>⟧. --rg-compat forces pure rg output; --annotate forces annotation.",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			code := runGrep(cmd.Context(), state, args)
			if code != 0 {
				// Same exit code as rg; nothing else may be printed.
				os.Exit(code)
			}
			return nil
		},
	}
}

// runGrep executes the grep passthrough and records telemetry. It returns the
// exit code to use.
func runGrep(ctx context.Context, state *rootState, args []string) int {
	if ctx == nil {
		ctx = context.Background()
	}
	rgArgs, rgCompat, annotate := splitGrepArgs(args)
	started := time.Now()
	result := grepx.Run(ctx, grepx.Options{
		Args:     rgArgs,
		RgCompat: rgCompat,
		Annotate: annotate,
	})
	recordGrepTelemetry(state, result, time.Since(started))
	return result.ExitCode
}

func recordGrepTelemetry(state *rootState, result grepx.Result, latency time.Duration) {
	defer func() { _ = recover() }()
	if state == nil || strings.EqualFold(os.Getenv("MI_LSP_TELEMETRY"), "off") {
		return
	}
	// Direct write to daemon.db; no retention pass, no daemon, no patterns.
	t := NewCLITelemetry(state.clientName, state.sessionID, state.verbose)
	defer t.Close()
	outcome := "raw"
	switch {
	case result.Mode == "annotated" && result.Annotated > 0:
		outcome = "annotated"
	case result.Mode == "no_index":
		outcome = "no_index"
	}
	t.RecordEvent(model.AccessEvent{
		OccurredAt:     time.Now(),
		ClientName:     firstNonEmpty(state.clientName, "manual-cli"),
		SessionID:      state.sessionID,
		WorkspaceInput: result.Workspace,
		Workspace:      result.Workspace,
		Operation:      "grep",
		Backend:        "rg",
		Route:          "direct",
		Success:        result.ExitCode == 0 || result.ExitCode == 1,
		LatencyMs:      latency.Milliseconds(),
		ResultCount:    result.Matches,
		RoutingOutcome: outcome,
		BytesOut:       int(result.BytesOut),
		Harness:        telemetry.HarnessFromEnv(),
	})
}
