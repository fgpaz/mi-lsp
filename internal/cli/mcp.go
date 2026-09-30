package cli

import (
	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/mcp"
)

func newMCPCommand(state *rootState) *cobra.Command {
	return &cobra.Command{
		Use:           "mcp",
		Short:         "Serve nav tools over stdio MCP",
		Long:          "Serve nav_* tools as newline-delimited JSON-RPC 2.0 on stdio. Each tools/call execs this binary, or MI_LSP_BIN, with an argv vector and does not start a shell.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			defaultWorkspace := ""
			if state != nil {
				defaultWorkspace = state.workspace
			}
			return mcp.Serve(cmd.Context(), mcp.ServeConfig{
				In:               cmd.InOrStdin(),
				Out:              cmd.OutOrStdout(),
				DefaultWorkspace: defaultWorkspace,
			})
		},
	}
}
