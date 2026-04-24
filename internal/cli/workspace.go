package cli

import "github.com/spf13/cobra"

func newWorkspaceCommand(state *rootState) *cobra.Command {
	command := &cobra.Command{
		Use:   "workspace",
		Short: "Manage workspace aliases and warm state",
		Long: `Register, list, and manage workspace aliases in the global registry.
A workspace is a root directory containing one or more .NET/TS repos
with an optional .mi-lsp/project.toml topology file.`,
	}

	var alias string
	var noIndex bool
	addCommand := &cobra.Command{
		Use:   "add <path>",
		Short: "Register a workspace path",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireArgs(args, 1, "path"); err != nil {
				return err
			}
			return state.executeOperation(cmd, "workspace.add", map[string]any{"path": args[0], "alias": alias, "no_index": noIndex}, false)
		},
	}
	addCommand.Flags().StringVar(&alias, "name", "", "Workspace alias")
	addCommand.Flags().BoolVar(&noIndex, "no-index", false, "Skip automatic indexing after registration")

	scanCommand := &cobra.Command{
		Use:   "scan",
		Short: "Auto-discover nearby workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			return state.executeOperation(cmd, "workspace.scan", nil, false)
		},
	}

	listCommand := &cobra.Command{
		Use:   "list",
		Short: "List registered workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			return state.executeOperation(cmd, "workspace.list", nil, false)
		},
	}

	warmCommand := &cobra.Command{
		Use:   "warm [workspace]",
		Short: "Warm a workspace in the daemon pool",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				state.workspace = args[0]
			}
			return state.executeOperation(cmd, "workspace.warm", nil, true)
		},
	}

	var statusNoAutoSync bool
	statusCommand := &cobra.Command{
		Use:   "status [workspace]",
		Short: "Show workspace registration and index status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				state.workspace = args[0]
			}
			payload := map[string]any{}
			if statusNoAutoSync {
				payload["auto_sync"] = false
			}
			return state.executeOperation(cmd, "workspace.status", payload, false)
		},
	}
	statusCommand.Flags().BoolVar(&statusNoAutoSync, "no-auto-sync", false, "Do not write read-model.toml while checking governance status")

	removeCommand := &cobra.Command{
		Use:   "remove <name>",
		Short: "Unregister a workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireArgs(args, 1, "name"); err != nil {
				return err
			}
			return state.executeOperation(cmd, "workspace.remove", map[string]any{"name": args[0]}, false)
		},
	}

	command.AddCommand(addCommand, scanCommand, listCommand, warmCommand, statusCommand, removeCommand)
	return command
}
