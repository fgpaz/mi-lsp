package cli

import (
	"encoding/json"
	"fmt"

	"github.com/fgpaz/mi-lsp/internal/repair"
	"github.com/spf13/cobra"
)

func newRepairCommand(state *rootState) *cobra.Command {
	var apply, reindex, worker bool
	var mode string
	cmd := &cobra.Command{Use: "repair", Short: "Diagnosticar y reparar componentes locales", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if mode != "auto" && mode != "framework-dependent" && mode != "self-contained" {
			return fmt.Errorf("modo de worker inválido: %s", mode)
		}
		report := repair.Run(repair.Options{Workspace: state.workspace, ToolRoot: state.repoRoot, WorkerMode: mode, Apply: apply, Reindex: reindex, Worker: worker})
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(report); err != nil {
			return err
		}
		if !report.OK {
			return envelopePrintedError{err: ErrExitCode(1)}
		}
		return nil
	}}
	cmd.Flags().BoolVar(&apply, "apply", false, "Aplicar acciones locales")
	cmd.Flags().BoolVar(&reindex, "reindex", false, "Reconstruir el índice seleccionado")
	cmd.Flags().BoolVar(&worker, "worker", false, "Instalar o actualizar el worker")
	cmd.Flags().StringVar(&mode, "worker-mode", "auto", "Modo: auto, framework-dependent o self-contained")
	return cmd
}
