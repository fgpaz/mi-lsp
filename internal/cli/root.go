package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/daemon"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/output"
	"github.com/fgpaz/mi-lsp/internal/service"
	"github.com/fgpaz/mi-lsp/internal/worker"
)

type rootState struct {
	repoRoot     string
	app          *service.App
	workspace    string
	format       string
	tokenBudget  int
	maxItems     int
	maxChars     int
	verbose      bool
	clientName   string
	sessionID    string
	backendHint  string
	telemetry    *CLITelemetry
	noAutoDaemon bool
	compress     bool
}

func NewRootCommand() *cobra.Command {
	repoRoot, _ := worker.ResolveToolRoot(".")
	clientName := os.Getenv("MI_LSP_CLIENT_NAME")
	if clientName == "" {
		clientName = "manual-cli"
	}
	sessionID := os.Getenv("MI_LSP_SESSION_ID")
	state := &rootState{
		repoRoot:    repoRoot,
		app:         service.New(repoRoot, nil),
		format:      "compact",
		tokenBudget: service.DefaultConfig().DefaultTokenBudget,
		maxItems:    service.DefaultConfig().DefaultMaxItems,
		clientName:  clientName,
		sessionID:   sessionID,
	}

	// Initialize CLI telemetry (best-effort, never blocks startup).
	state.telemetry = NewCLITelemetry(clientName, sessionID, false)
	if state.telemetry != nil {
		if purged, err := state.telemetry.ApplyRetention(retentionDays()); err == nil && purged > 0 {
			// Retention applied silently on startup.
			_ = purged
		}
	}

	root := &cobra.Command{
		Use:           "mi-lsp",
		Short:         "Semantic CLI for multi-workspace .NET and TS repositories",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			switch state.format {
			case "compact", "json", "text":
			default:
				return fmt.Errorf("invalid --format %q; valid options: compact, json, text", state.format)
			}
			if cmd.Flags().Changed("token-budget") && state.tokenBudget <= 0 {
				return fmt.Errorf("--token-budget must be > 0")
			}
			if cmd.Flags().Changed("max-items") && state.maxItems <= 0 {
				return fmt.Errorf("--max-items must be > 0")
			}
			switch state.backendHint {
			case "", "roslyn", "tsserver", "pyright", "catalog", "text":
			default:
				return fmt.Errorf("invalid --backend %q; valid options: roslyn, tsserver, pyright, catalog, text", state.backendHint)
			}
			return nil
		},
	}

	root.PersistentFlags().StringVar(&state.workspace, "workspace", "", "Workspace alias or path")
	root.PersistentFlags().StringVar(&state.format, "format", "compact", "Output format: compact|json|text")
	root.PersistentFlags().IntVar(&state.tokenBudget, "token-budget", service.DefaultConfig().DefaultTokenBudget, "Approximate output token budget")
	root.PersistentFlags().IntVar(&state.maxItems, "max-items", service.DefaultConfig().DefaultMaxItems, "Maximum items in response")
	root.PersistentFlags().IntVar(&state.maxChars, "max-chars", 0, "Maximum output characters")
	root.PersistentFlags().BoolVar(&state.verbose, "verbose", false, "Verbose debug output")
	root.PersistentFlags().StringVar(&state.clientName, "client-name", state.clientName, "Logical client name for governance and telemetry")
	root.PersistentFlags().StringVar(&state.sessionID, "session-id", state.sessionID, "Logical client session identifier for governance and telemetry")
	root.PersistentFlags().StringVar(&state.backendHint, "backend", "", "Force a backend hint: roslyn|tsserver|pyright|catalog")
	root.PersistentFlags().BoolVar(&state.noAutoDaemon, "no-auto-daemon", false, "Disable automatic daemon startup for semantic queries")
	root.PersistentFlags().BoolVar(&state.compress, "compress", false, "Aggressive compression: strips parent, scope, implements from compact output")

	root.AddCommand(
		newInitCommand(state),
		newWorkspaceCommand(state),
		newNavCommand(state),
		newIndexCommand(state),
		newInfoCommand(state),
		newDaemonCommand(state),
		newAdminCommand(state),
		newWorkerCommand(state),
	)
	return root
}

func (s *rootState) queryOptions() model.QueryOptions {
	return model.QueryOptions{
		Workspace:   s.workspace,
		Format:      s.format,
		TokenBudget: s.tokenBudget,
		MaxItems:    s.maxItems,
		MaxChars:    s.maxChars,
		Verbose:     s.verbose,
		ClientName:  s.clientName,
		SessionID:   s.sessionID,
		BackendHint: s.backendHint,
		Compress:    s.compress,
	}
}

func (s *rootState) executeOperation(cmd *cobra.Command, operation string, payload map[string]any, preferDaemon bool) error {
	request := model.CommandRequest{
		ProtocolVersion: model.ProtocolVersion,
		Operation:       operation,
		Context:         s.queryOptions(),
		Payload:         payload,
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
	defer cancel()

	useDaemon := shouldUseDaemon(operation, preferDaemon)

	// Only heavy daemon-backed queries should auto-start the daemon.
	if useDaemon && !s.noAutoDaemon && shouldAutoStartDaemon(operation) {
		if err := daemon.EnsureDaemon(s.repoRoot); err != nil {
			// Log warning but don't fail; fall back to direct mode
			if s.verbose {
				fmt.Fprintf(os.Stderr, "[mi-lsp] daemon auto-start failed: %v; using direct mode\n", err)
			}
		}
	}

	started := time.Now()
	var (
		envelope model.Envelope
		err      error
	)
	if useDaemon {
		envelope, err = daemon.NewClient().Execute(ctx, request)
	}
	if !useDaemon || err != nil {
		envelope, err = s.app.Execute(ctx, request)
	}
	latency := time.Since(started)

	// Best-effort telemetry: record the operation even if it failed.
	if s.telemetry != nil {
		s.telemetry.RecordOperation(request, envelope, err, latency)
	}

	if err != nil {
		return err
	}
	return s.printEnvelope(envelope)
}

func shouldUseDaemon(operation string, requested bool) bool {
	if !requested {
		return false
	}
	switch operation {
	case "nav.find", "nav.search", "nav.intent", "nav.symbols", "nav.outline", "nav.overview", "nav.multi-read", "nav.trace":
		return false
	default:
		return true
	}
}

// shouldAutoStartDaemon returns true for heavy operations that benefit from warm runtimes.
func shouldAutoStartDaemon(operation string) bool {
	switch operation {
	case "nav.refs", "nav.context", "nav.deps", "nav.related", "nav.ask", "nav.service", "nav.workspace-map", "nav.diff-context", "nav.batch":
		return true
	}
	return false
}

func (s *rootState) printEnvelope(envelope model.Envelope) error {
	envelope = output.ApplyEnvelopeLimits(envelope, s.queryOptions())
	rendered, err := output.Render(envelope, s.format, s.compress)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, string(rendered))
	return err
}

func requireArgs(args []string, count int, label string) error {
	if len(args) < count {
		return fmt.Errorf("%s is required; see --help for usage", label)
	}
	return nil
}
