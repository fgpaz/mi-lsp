package cli

import "github.com/spf13/cobra"

func newInitCommand(state *rootState) *cobra.Command {
	var alias string
	var noIndex bool
	command := &cobra.Command{
		Use:   "init [path]",
		Short: "Detect, register, and index the current workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			return state.executeOperation(cmd, "workspace.init", map[string]any{"path": path, "alias": alias, "no_index": noIndex}, false)
		},
	}
	command.Flags().StringVar(&alias, "name", "", "Workspace alias")
	command.Flags().BoolVar(&noIndex, "no-index", false, "Skip automatic indexing after registration")
	return command
}
