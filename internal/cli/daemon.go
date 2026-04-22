package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/daemon"
	"github.com/fgpaz/mi-lsp/internal/model"
)

func newDaemonCommand(state *rootState) *cobra.Command {
	command := &cobra.Command{
		Use:   "daemon",
		Short: "Manage the optional background daemon",
		Long: `Control the optional background daemon that keeps semantic workers
alive between CLI invocations. The daemon manages a pool of Roslyn
and tsserver processes, reducing cold-start latency.`,
	}

	var idleTimeout string
	var maxWorkers int
	var watchMode string
	var maxWatchedRoots int
	var maxInflight int
	startCommand := &cobra.Command{
		Use:   "start",
		Short: "Start the background daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			timeout, err := time.ParseDuration(idleTimeout)
			if err != nil {
				return err
			}
			options, err := daemonOptions(watchMode, maxWatchedRoots, maxInflight)
			if err != nil {
				return err
			}
			stateBody, started, err := daemon.SpawnBackgroundWithOptions(state.repoRoot, maxWorkers, timeout, options)
			if err != nil {
				return err
			}
			items := []map[string]any{{
				"started":         started,
				"already_running": stateBody.AlreadyRunning,
				"state":           stateBody,
			}}
			return state.printEnvelope(model.Envelope{Ok: true, Backend: "daemon", Items: items}, state.queryOptions(cmd, "daemon.start", nil))
		},
	}
	startCommand.Flags().StringVar(&idleTimeout, "idle-timeout", "30m", "Worker idle eviction timeout")
	startCommand.Flags().IntVar(&maxWorkers, "max-workers", 3, "Maximum active semantic workers")
	startCommand.Flags().StringVar(&watchMode, "watch-mode", "", "File watcher mode: off|lazy|eager (default: env or lazy)")
	startCommand.Flags().IntVar(&maxWatchedRoots, "max-watched-roots", 0, "Maximum active watched roots (default: env or 8)")
	startCommand.Flags().IntVar(&maxInflight, "max-inflight", 0, "Maximum concurrent daemon-served heavy requests (default: env or 16)")

	statusCommand := &cobra.Command{
		Use:   "status",
		Short: "Check daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			response, err := daemon.NewClient().Execute(ctx, model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "system.status", Context: state.queryOptions(cmd, "system.status", nil)})
			if err != nil {
				return daemon.BuildStatusError()
			}
			return state.printEnvelope(response, state.queryOptions(cmd, "system.status", nil))
		},
	}

	stopCommand := &cobra.Command{
		Use:   "stop",
		Short: "Stop the daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			response, err := daemon.NewClient().Execute(ctx, model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "system.stop", Context: state.queryOptions(cmd, "system.stop", nil)})
			if err != nil {
				return err
			}
			return state.printEnvelope(response, state.queryOptions(cmd, "system.stop", nil))
		},
	}

	restartCommand := &cobra.Command{
		Use:   "restart",
		Short: "Stop and restart the background daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			_, stopErr := daemon.NewClient().Execute(ctx, model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "system.stop", Context: state.queryOptions(cmd, "system.stop", nil)})
			cancel()
			if stopErr == nil {
				time.Sleep(500 * time.Millisecond)
			}
			timeout, err := time.ParseDuration(idleTimeout)
			if err != nil {
				return err
			}
			options, err := daemonOptions(watchMode, maxWatchedRoots, maxInflight)
			if err != nil {
				return err
			}
			stateBody, started, err := daemon.SpawnBackgroundWithOptions(state.repoRoot, maxWorkers, timeout, options)
			if err != nil {
				return err
			}
			items := []map[string]any{{
				"restarted":       true,
				"started":         started,
				"already_running": stateBody.AlreadyRunning,
				"state":           stateBody,
			}}
			return state.printEnvelope(model.Envelope{Ok: true, Backend: "daemon", Items: items}, state.queryOptions(cmd, "daemon.restart", nil))
		},
	}

	serveCommand := &cobra.Command{
		Use:    "serve",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			timeout, err := time.ParseDuration(idleTimeout)
			if err != nil {
				return err
			}
			options, err := daemonOptions(watchMode, maxWatchedRoots, maxInflight)
			if err != nil {
				return err
			}
			server, err := daemon.NewServerWithOptions(state.repoRoot, maxWorkers, timeout, options)
			if err != nil {
				return err
			}
			defer server.Shutdown()

			serverCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() {
				<-cmd.Context().Done()
				server.Shutdown()
				cancel()
			}()

			return server.Serve(serverCtx)
		},
	}
	serveCommand.Flags().StringVar(&idleTimeout, "idle-timeout", "30m", "Worker idle eviction timeout")
	serveCommand.Flags().IntVar(&maxWorkers, "max-workers", 3, "Maximum active semantic workers")
	serveCommand.Flags().StringVar(&watchMode, "watch-mode", "", "File watcher mode: off|lazy|eager")
	serveCommand.Flags().IntVar(&maxWatchedRoots, "max-watched-roots", 0, "Maximum active watched roots")
	serveCommand.Flags().IntVar(&maxInflight, "max-inflight", 0, "Maximum concurrent daemon-served heavy requests")

	debugStateCommand := &cobra.Command{
		Use:    "print-state",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			response, err := daemon.NewClient().Execute(ctx, model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "system.status", Context: state.queryOptions(cmd, "system.status", nil)})
			if err != nil {
				return err
			}
			body, err := json.MarshalIndent(response, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(body))
			return nil
		},
	}

	openCommand := &cobra.Command{
		Use:   "open",
		Short: "Open the governance UI in the system browser",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			response, err := daemon.NewClient().Execute(ctx, model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "system.status", Context: state.queryOptions(cmd, "system.status", nil)})
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
			return state.printEnvelope(model.Envelope{Ok: true, Backend: "admin", Items: []map[string]any{{"admin_url": finalURL}}}, state.queryOptions(cmd, "daemon.admin", nil))
		},
	}

	var tailLines int
	logsCommand := &cobra.Command{
		Use:   "logs",
		Short: "Show daemon log tail (RF-DAE-002)",
		RunE: func(cmd *cobra.Command, args []string) error {
			logPath := filepath.Join(state.repoRoot, ".mi-lsp", "daemon.log")
			lines, _, err := daemon.ReadLogTailFile(logPath, tailLines, 1<<20)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("daemon log not found at %s; has the daemon ever started?", logPath)
				}
				return err
			}
			if len(lines) == 0 {
				return fmt.Errorf("daemon log not found or empty at %s", logPath)
			}
			for _, line := range lines {
				fmt.Println(line.Text)
			}
			return nil
		},
	}
	logsCommand.Flags().IntVar(&tailLines, "tail", 50, "Number of lines to show")

	var smokeCallers int
	var smokeMaxWorkingSetMB int
	var smokeMaxPrivateMB int
	var smokeMaxHandles int
	perfSmokeCommand := &cobra.Command{
		Use:   "perf-smoke",
		Short: "Run daemon memory and parallel-caller smoke checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := daemonOptions(watchMode, maxWatchedRoots, maxInflight)
			if err != nil {
				return err
			}
			timeout, err := time.ParseDuration(idleTimeout)
			if err != nil {
				return err
			}
			result, err := daemon.RunPerfSmoke(cmd.Context(), state.repoRoot, daemon.PerfSmokeOptions{
				Callers:       smokeCallers,
				MaxWorkingSet: uint64(smokeMaxWorkingSetMB) * 1024 * 1024,
				MaxPrivate:    uint64(smokeMaxPrivateMB) * 1024 * 1024,
				MaxHandles:    uint64(smokeMaxHandles),
				StartOptions:  options,
				MaxWorkers:    maxWorkers,
				IdleTimeout:   timeout,
			})
			if err != nil {
				return err
			}
			env := model.Envelope{Ok: result.Passed, Backend: "daemon", Items: []daemon.PerfSmokeResult{result}, Warnings: result.Warnings}
			return state.printEnvelope(env, state.queryOptions(cmd, "daemon.perf-smoke", nil))
		},
	}
	perfSmokeCommand.Flags().IntVar(&smokeCallers, "callers", 16, "Parallel status callers")
	perfSmokeCommand.Flags().IntVar(&smokeMaxWorkingSetMB, "max-working-set-mb", 250, "Maximum daemon working set in MB")
	perfSmokeCommand.Flags().IntVar(&smokeMaxPrivateMB, "max-private-mb", 300, "Maximum daemon private bytes in MB")
	perfSmokeCommand.Flags().IntVar(&smokeMaxHandles, "max-handles", 5000, "Maximum daemon handle/fd count")
	perfSmokeCommand.Flags().StringVar(&watchMode, "watch-mode", "", "File watcher mode: off|lazy|eager")
	perfSmokeCommand.Flags().IntVar(&maxWatchedRoots, "max-watched-roots", 0, "Maximum active watched roots")
	perfSmokeCommand.Flags().IntVar(&maxInflight, "max-inflight", 0, "Maximum concurrent daemon-served heavy requests")

	command.AddCommand(startCommand, statusCommand, stopCommand, restartCommand, serveCommand, debugStateCommand, openCommand, logsCommand, perfSmokeCommand)
	return command
}

func daemonOptions(watchMode string, maxWatchedRoots int, maxInflight int) (daemon.StartOptions, error) {
	if err := daemon.ValidateWatchMode(watchMode); err != nil {
		return daemon.StartOptions{}, err
	}
	options := daemon.DefaultStartOptions()
	if watchMode != "" {
		options.WatchMode = watchMode
	}
	if maxWatchedRoots > 0 {
		options.MaxWatchedRoots = maxWatchedRoots
	}
	if maxInflight > 0 {
		options.MaxInflight = maxInflight
	}
	return daemon.NormalizeStartOptions(options), nil
}
