package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/daemon"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/output"
	"github.com/fgpaz/mi-lsp/internal/service"
	"github.com/fgpaz/mi-lsp/internal/worker"
	"github.com/fgpaz/mi-lsp/internal/workspace"
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
	axi          bool
	classic      bool
	full         bool
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
	if sessionID == "" {
		sessionID = fmt.Sprintf("cli-%d", os.Getpid())
	}
	axiEnabled := envBool("MI_LSP_AXI")
	state := &rootState{
		repoRoot:    repoRoot,
		app:         service.New(repoRoot, nil),
		format:      "compact",
		tokenBudget: service.DefaultConfig().DefaultTokenBudget,
		maxItems:    service.DefaultConfig().DefaultMaxItems,
		clientName:  clientName,
		sessionID:   sessionID,
		axi:         axiEnabled,
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
		RunE: func(cmd *cobra.Command, args []string) error {
			if state.effectiveAXI(cmd, "root.home", nil) {
				return state.renderAXIHome(cmd)
			}
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			switch state.format {
			case "compact", "json", "text", "toon", "yaml":
			default:
				return fmt.Errorf("invalid --format %q; valid options: compact, json, text, toon, yaml", state.format)
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
			if isClassicRequested(cmd, state.classic) && flagChanged(cmd, "axi") && state.axi {
				return fmt.Errorf("--axi and --classic cannot be used together")
			}
			// TOON default when AXI is seeded and --format was not explicit
			if !flagChanged(cmd, "format") && state.axi {
				state.format = "toon"
			}
			return nil
		},
	}

	root.PersistentFlags().StringVar(&state.workspace, "workspace", "", "Workspace alias or path")
	root.PersistentFlags().StringVar(&state.format, "format", "compact", "Output format: compact|json|text|toon|yaml")
	root.PersistentFlags().IntVar(&state.tokenBudget, "token-budget", service.DefaultConfig().DefaultTokenBudget, "Approximate output token budget")
	root.PersistentFlags().IntVar(&state.maxItems, "max-items", service.DefaultConfig().DefaultMaxItems, "Maximum items in response")
	root.PersistentFlags().IntVar(&state.maxChars, "max-chars", 0, "Maximum output characters")
	root.PersistentFlags().BoolVar(&state.verbose, "verbose", false, "Verbose debug output")
	root.PersistentFlags().StringVar(&state.clientName, "client-name", state.clientName, "Logical client name for governance and telemetry")
	root.PersistentFlags().StringVar(&state.sessionID, "session-id", state.sessionID, "Logical client session identifier for governance and telemetry")
	root.PersistentFlags().StringVar(&state.backendHint, "backend", "", "Force a backend hint: roslyn|tsserver|pyright|catalog")
	root.PersistentFlags().BoolVar(&state.axi, "axi", axiEnabled, "Enable AXI discovery and preview mode")
	root.PersistentFlags().BoolVar(&state.classic, "classic", false, "Force classic CLI behavior on AXI-default surfaces")
	root.PersistentFlags().BoolVar(&state.full, "full", false, "Expand AXI preview responses to fuller detail")
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

func (s *rootState) queryOptions(cmd *cobra.Command, operation string, payload map[string]any) model.QueryOptions {
	axiEnabled := s.effectiveAXI(cmd, operation, payload)
	fullEnabled := axiEnabled && s.full
	callerCWD := ""
	if cwd, err := os.Getwd(); err == nil {
		callerCWD = cwd
	}
	return model.QueryOptions{
		Workspace:   s.workspace,
		CallerCWD:   callerCWD,
		Format:      s.effectiveFormat(cmd, operation, payload, axiEnabled),
		TokenBudget: s.tokenBudget,
		MaxItems:    s.effectiveMaxItems(cmd, operation, axiEnabled, fullEnabled),
		MaxChars:    s.maxChars,
		AXI:         axiEnabled,
		Full:        fullEnabled,
		Verbose:     s.verbose,
		ClientName:  s.clientName,
		SessionID:   s.sessionID,
		BackendHint: s.backendHint,
		Compress:    s.compress,
	}
}

func (s *rootState) executeOperation(cmd *cobra.Command, operation string, payload map[string]any, preferDaemon bool) error {
	opts := s.queryOptions(cmd, operation, payload)
	if offset, ok := offsetFromPayload(payload); ok {
		opts.Offset = offset
	}
	request := model.CommandRequest{
		ProtocolVersion: model.ProtocolVersion,
		Operation:       operation,
		Context:         opts,
		Payload:         payload,
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), timeoutForOperation(operation))
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
		route    string
	)
	daemonFailed := false
	if useDaemon {
		envelope, err = daemon.NewClient().Execute(ctx, request)
		if err != nil {
			daemonFailed = true
		} else {
			route = "daemon"
		}
	}
	if !useDaemon || daemonFailed {
		envelope, err = s.app.Execute(ctx, request)
		if err == nil && daemonFailed && envelope.Hint == "" {
			envelope.Hint = "daemon_unavailable; served from local text index"
		}
		if daemonFailed {
			route = "direct_fallback"
		} else {
			route = "direct"
		}
	}
	latency := time.Since(started)

	// Best-effort telemetry: direct/direct_fallback record at the caller.
	// Daemon-served requests are recorded canonically inside the daemon.
	if s.telemetry != nil && shouldRecordCLITelemetry(route, err) {
		s.telemetry.RecordOperation(request, envelope, err, latency, route)
	}

	if err != nil {
		return err
	}
	return s.printEnvelope(envelope, request.Context)
}

func (s *rootState) effectiveFormat(cmd *cobra.Command, operation string, payload map[string]any, axiEnabled bool) string {
	if cmd != nil && cmd.Flags().Changed("format") {
		return s.format
	}
	if axiEnabled {
		return "toon"
	}
	return s.format
}

func (s *rootState) effectiveMaxItems(cmd *cobra.Command, operation string, axiEnabled bool, fullEnabled bool) int {
	if cmd != nil && cmd.Flags().Changed("max-items") {
		return s.maxItems
	}
	if axiEnabled && !fullEnabled {
		switch operation {
		case "nav.search", "nav.intent":
			if s.maxItems > 5 {
				return 5
			}
		}
	}
	return s.maxItems
}

func shouldUseDaemon(operation string, requested bool) bool {
	if !requested {
		return false
	}
	switch operation {
	case "nav.find", "nav.search", "nav.intent", "nav.symbols", "nav.outline", "nav.overview", "nav.multi-read", "nav.trace", "nav.pack", "nav.route", "nav.governance", "nav.ask", "nav.workspace-map":
		return false
	default:
		return true
	}
}

// shouldAutoStartDaemon returns true for heavy operations that benefit from warm runtimes.
func shouldAutoStartDaemon(operation string) bool {
	switch operation {
	case "nav.refs", "nav.context", "nav.deps", "nav.related", "nav.service", "nav.diff-context", "nav.batch":
		return true
	case "nav.workspace-map":
		return false
	}
	return false
}

func timeoutForOperation(operation string) time.Duration {
	switch operation {
	case "index.run":
		return 15 * time.Minute
	case "workspace.add", "workspace.init":
		return 5 * time.Minute
	default:
		return 2 * time.Minute
	}
}

func shouldRecordCLITelemetry(route string, opErr error) bool {
	if route == "daemon" && opErr == nil {
		return false
	}
	return true
}

func offsetFromPayload(payload map[string]any) (int, bool) {
	if len(payload) == 0 {
		return 0, false
	}
	raw, ok := payload["offset"]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

func (s *rootState) printEnvelope(envelope model.Envelope, opts model.QueryOptions) error {
	envelope = output.ApplyEnvelopeLimits(envelope, opts)
	rendered, err := output.Render(envelope, opts.Format, opts.Compress)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, string(rendered))
	return err
}

func envBool(name string) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (s *rootState) renderAXIHome(cmd *cobra.Command) error {
	env, err := s.buildAXIHomeEnvelope(cmd)
	if err != nil {
		return err
	}
	return s.printEnvelope(env, s.queryOptions(cmd, "root.home", nil))
}

func (s *rootState) buildAXIHomeEnvelope(cmd *cobra.Command) (model.Envelope, error) {
	registrations, err := workspace.ListWorkspaces()
	if err != nil {
		registrations = nil
	}

	homeItem := map[string]any{
		"view":                  "home",
		"mode":                  "axi",
		"registered_workspaces": len(registrations),
	}
	warnings := []string{}

	if cwd, err := os.Getwd(); err == nil {
		homeItem["current_dir"] = cwd
	}

	if daemonState, daemonReady := probeDaemonHome(cmd.Context()); daemonReady {
		homeItem["daemon_ready"] = true
		homeItem["daemon_endpoint"] = daemonState.Endpoint
		homeItem["daemon_admin_url"] = daemonState.AdminURL
	} else {
		homeItem["daemon_ready"] = false
	}

	workerInfo := worker.InspectWorkerRuntime(s.repoRoot, worker.ResolveRID())
	homeItem["worker_ready"] = workerInfo.Selected.Compatible
	homeItem["worker_source"] = workerInfo.Selected.Source
	homeItem["worker_path"] = workerInfo.Selected.Path
	if workerInfo.InstallHint != "" {
		homeItem["worker_hint"] = workerInfo.InstallHint
	}

	registration, project, source, resolved, resolveWarnings := s.resolveHomeWorkspace(registrations)
	warnings = append(warnings, resolveWarnings...)
	if resolved {
		homeItem["workspace"] = registration.Name
		homeItem["workspace_root"] = registration.Root
		homeItem["workspace_kind"] = registration.Kind
		homeItem["workspace_source"] = source
		homeItem["repo_count"] = len(project.Repos)
		homeItem["entrypoint_count"] = len(project.Entrypoints)
		homeItem["default_repo"] = project.Project.DefaultRepo
		homeItem["default_entrypoint"] = project.Project.DefaultEntrypoint
		homeItem["docs_read_model"] = homeDocsReadModel(registration.Root)
		homeItem["next_steps"] = []string{
			"mi-lsp workspace status " + registration.Name,
			"mi-lsp nav governance --workspace " + registration.Name + " --format toon",
			"mi-lsp nav ask \"how is this workspace organized?\" --workspace " + registration.Name,
			"mi-lsp nav workspace-map --workspace " + registration.Name + " --axi --full",
		}
	} else {
		homeItem["workspace_detected"] = false
		homeItem["next_steps"] = []string{
			"mi-lsp init .",
			"mi-lsp workspace scan",
			"mi-lsp workspace list",
		}
	}

	return model.Envelope{
		Ok:       true,
		Backend:  "axi-home",
		Items:    []map[string]any{homeItem},
		Warnings: warnings,
	}, nil
}

func (s *rootState) resolveHomeWorkspace(registrations []model.WorkspaceRegistration) (model.WorkspaceRegistration, model.ProjectFile, string, bool, []string) {
	if strings.TrimSpace(s.workspace) != "" {
		reg, err := s.app.ResolveWorkspace(s.workspace)
		if err != nil {
			return model.WorkspaceRegistration{}, model.ProjectFile{}, "", false, []string{err.Error()}
		}
		project, err := workspace.LoadProjectTopology(reg.Root, reg)
		if err != nil {
			return model.WorkspaceRegistration{}, model.ProjectFile{}, "", false, []string{err.Error()}
		}
		return workspace.ApplyProjectTopology(reg, project), project, "flag", true, nil
	}

	if cwd, err := os.Getwd(); err == nil {
		if reg, project, ok := resolveRegisteredWorkspaceByRoot(cwd, registrations); ok {
			return reg, project, "cwd-registered", true, nil
		}
		if reg, project, err := workspace.DetectWorkspaceLayout(cwd, ""); err == nil {
			reg = workspace.ApplyProjectTopology(reg, project)
			return reg, project, "cwd-detected", true, nil
		}
	}

	reg, err := s.app.ResolveWorkspace("")
	if err != nil {
		return model.WorkspaceRegistration{}, model.ProjectFile{}, "", false, nil
	}
	project, err := workspace.LoadProjectTopology(reg.Root, reg)
	if err != nil {
		return model.WorkspaceRegistration{}, model.ProjectFile{}, "", false, []string{err.Error()}
	}
	return workspace.ApplyProjectTopology(reg, project), project, "default-registry", true, nil
}

func resolveRegisteredWorkspaceByRoot(cwd string, registrations []model.WorkspaceRegistration) (model.WorkspaceRegistration, model.ProjectFile, bool) {
	_ = registrations
	resolution, err := workspace.ResolveWorkspaceSelection("", cwd)
	if err != nil || resolution.Source != workspace.ResolutionSourceCallerCWD {
		return model.WorkspaceRegistration{}, model.ProjectFile{}, false
	}
	registration := resolution.Registration
	project, err := workspace.LoadProjectTopology(registration.Root, registration)
	if err != nil {
		return model.WorkspaceRegistration{}, model.ProjectFile{}, false
	}
	return workspace.ApplyProjectTopology(registration, project), project, true
}

func probeDaemonHome(parent context.Context) (model.DaemonState, bool) {
	ctx, cancel := context.WithTimeout(parent, 1200*time.Millisecond)
	defer cancel()
	response, err := daemon.NewClient().Execute(ctx, model.CommandRequest{
		ProtocolVersion: model.ProtocolVersion,
		Operation:       "system.status",
		Context:         model.QueryOptions{ClientName: "manual-cli", Format: "json"},
	})
	if err != nil || !response.Ok {
		return model.DaemonState{}, false
	}
	body, err := json.Marshal(response.Items)
	if err != nil {
		return model.DaemonState{}, false
	}
	var decoded []map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil || len(decoded) == 0 {
		return model.DaemonState{}, false
	}
	stateBody, err := json.Marshal(decoded[0]["state"])
	if err != nil {
		return model.DaemonState{}, false
	}
	var state model.DaemonState
	if err := json.Unmarshal(stateBody, &state); err != nil {
		return model.DaemonState{}, false
	}
	return state, true
}

func homeDocsReadModel(root string) string {
	if _, err := os.Stat(filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml")); err == nil {
		return ".docs/wiki/_mi-lsp/read-model.toml"
	}
	return "builtin-default"
}

func requireArgs(args []string, count int, label string) error {
	if len(args) < count {
		return fmt.Errorf("%s is required; see --help for usage", label)
	}
	return nil
}
