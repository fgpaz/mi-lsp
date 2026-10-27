package cli

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/daemon"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/output"
	"github.com/fgpaz/mi-lsp/internal/worker"
)

func newWorkerCommand(state *rootState) *cobra.Command {
	command := &cobra.Command{
		Use:   "worker",
		Short: "Install or inspect the semantic workers",
		Long: `Manage the Roslyn-based .NET semantic worker used for references,
context, and dependency analysis. Includes install and status.`,
	}

	var rid, mode string
	installCommand := &cobra.Command{
		Use:   "install",
		Short: "Install a worker for the current RID",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := worker.InstallWorkerMode(state.repoRoot, rid, mode)
			if err != nil {
				return err
			}
			if rid == "" {
				rid = worker.ResolveRID()
			}
			opts := state.queryOptions(cmd, "worker.install", nil)
			return state.printEnvelope(model.Envelope{Ok: true, Backend: "worker-install", Items: []map[string]any{{"path": path, "rid": rid}}}, opts)
		},
	}
	installCommand.Flags().StringVar(&rid, "rid", "", "Runtime identifier override")
	installCommand.Flags().StringVar(&mode, "mode", "auto", "Worker mode: auto, framework-dependent, or self-contained")

	statusCommand := &cobra.Command{
		Use:   "status",
		Short: "Show worker installation and runtime status",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			opts := state.queryOptions(cmd, "worker.status", nil)
			response, err := daemon.NewClient().Execute(ctx, model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "worker.status", Context: opts})
			if err == nil {
				response = output.ApplyEnvelopeLimits(response, opts)
				return state.printEnvelope(response, opts)
			}
			return state.executeOperation(cmd, "worker.status", nil, false)
		},
	}

	command.AddCommand(installCommand, statusCommand)
	return command
}
