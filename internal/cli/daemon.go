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
	startCommand := &cobra.Command{
		Use:   "start",
		Short: "Start the background daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			timeout, err := time.ParseDuration(idleTimeout)
			if err != nil {
				return err
			}
			stateBody, started, err := daemon.SpawnBackground(state.repoRoot, maxWorkers, timeout)
			if err != nil {
				return err
			}
			items := []map[string]any{{
				"started":         started,
				"already_running": stateBody.AlreadyRunning,
				"state":           stateBody,
			}}
			return state.printEnvelope(model.Envelope{Ok: true, Backend: "daemon", Items: items})
		},
	}
	startCommand.Flags().StringVar(&idleTimeout, "idle-timeout", "30m", "Worker idle eviction timeout")
	startCommand.Flags().IntVar(&maxWorkers, "max-workers", 3, "Maximum active semantic workers")

	statusCommand := &cobra.Command{
		Use:   "status",
		Short: "Check daemon status",
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

	stopCommand := &cobra.Command{
		Use:   "stop",
		Short: "Stop the daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			response, err := daemon.NewClient().Execute(ctx, model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "system.stop", Context: state.queryOptions()})
			if err != nil {
				return err
			}
			return state.printEnvelope(response)
		},
	}

	restartCommand := &cobra.Command{
		Use:   "restart",
		Short: "Stop and restart the background daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			_, stopErr := daemon.NewClient().Execute(ctx, model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "system.stop", Context: state.queryOptions()})
			cancel()
			if stopErr == nil {
				time.Sleep(500 * time.Millisecond)
			}
			timeout, err := time.ParseDuration(idleTimeout)
			if err != nil {
				return err
			}
			stateBody, started, err := daemon.SpawnBackground(state.repoRoot, maxWorkers, timeout)
			if err != nil {
				return err
			}
			items := []map[string]any{{
				"restarted":       true,
				"started":         started,
				"already_running": stateBody.AlreadyRunning,
				"state":           stateBody,
			}}
			return state.printEnvelope(model.Envelope{Ok: true, Backend: "daemon", Items: items})
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
			server, err := daemon.NewServer(state.repoRoot, maxWorkers, timeout)
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

	debugStateCommand := &cobra.Command{
		Use:    "print-state",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			response, err := daemon.NewClient().Execute(ctx, model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "system.status", Context: state.queryOptions()})
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

	var tailLines int
	logsCommand := &cobra.Command{
		Use:   "logs",
		Short: "Show daemon log tail (RF-DAE-002)",
		RunE: func(cmd *cobra.Command, args []string) error {
			logPath := filepath.Join(state.repoRoot, ".mi-lsp", "daemon.log")
			data, err := os.ReadFile(logPath)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("daemon log not found at %s; has the daemon ever started?", logPath)
				}
				return err
			}
			lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
			n := tailLines
			if n <= 0 || n > len(lines) {
				n = len(lines)
			}
			start := len(lines) - n
			for _, line := range lines[start:] {
				fmt.Println(line)
			}
			return nil
		},
	}
	logsCommand.Flags().IntVar(&tailLines, "tail", 50, "Number of lines to show")

	command.AddCommand(startCommand, statusCommand, stopCommand, restartCommand, serveCommand, debugStateCommand, openCommand, logsCommand)
	return command
}
