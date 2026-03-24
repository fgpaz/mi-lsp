package cli

import "github.com/spf13/cobra"

func newIndexCommand(state *rootState) *cobra.Command {
	var clean bool
	command := &cobra.Command{
		Use:   "index [path]",
		Short: "Index the current or selected workspace into repo-local SQLite",
		Long: `Parse source files and store symbols in the repo-local .mi-lsp/index.db.
Supports C#, TypeScript, and Go via built-in tree-sitter parsers.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) > 0 {
				path = args[0]
			}
			return state.executeOperation(cmd, "index.run", map[string]any{"path": path, "clean": clean}, false)
		},
	}
	command.Flags().BoolVar(&clean, "clean", false, "Reset the workspace index before indexing")
	return command
}

func newInfoCommand(state *rootState) *cobra.Command {
	return &cobra.Command{
		Use:   "info [workspace]",
		Short: "Show workspace info and index stats",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				state.workspace = args[0]
			}
			return state.executeOperation(cmd, "info", nil, false)
		},
	}
}
