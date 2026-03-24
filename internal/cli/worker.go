package cli

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/daemon"
	"github.com/fgpaz/mi-lsp/internal/model"
)

func newWorkerCommand(state *rootState) *cobra.Command {
	command := &cobra.Command{
		Use:   "worker",
		Short: "Install or inspect the semantic workers",
		Long: `Manage the Roslyn-based .NET semantic worker used for references,
context, and dependency analysis. Includes install and status.`,
	}

	var rid string
	installCommand := &cobra.Command{
		Use:   "install",
		Short: "Install a self-contained worker for the current RID",
		RunE: func(cmd *cobra.Command, args []string) error {
			return state.executeOperation(cmd, "worker.install", map[string]any{"rid": rid}, false)
		},
	}
	installCommand.Flags().StringVar(&rid, "rid", "", "Runtime identifier override")

	statusCommand := &cobra.Command{
		Use:   "status",
		Short: "Show worker installation and runtime status",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			response, err := daemon.NewClient().Execute(ctx, model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "worker.status", Context: state.queryOptions()})
			if err == nil {
				return state.printEnvelope(response)
			}
			return state.executeOperation(cmd, "worker.status", nil, false)
		},
	}

	command.AddCommand(installCommand, statusCommand)
	return command
}
