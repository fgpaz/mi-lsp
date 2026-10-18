package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newServeCommand(_ *rootState) *cobra.Command {
	var socket bool
	command := &cobra.Command{
		Use:   "serve",
		Short: "Serve CLI operations over an optional Unix socket",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !socket {
				return fmt.Errorf("serve requires --socket")
			}
			ctx, cancel := socketSignalContext(cmd.Context())
			defer cancel()
			return serveCLISocket(ctx)
		},
	}
	command.Flags().BoolVar(&socket, "socket", false, "Serve owner-only Unix socket at XDG_RUNTIME_DIR/mi-lsp.sock (JSONL requests up to 1 MiB)")
	return command
}
