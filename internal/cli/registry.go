package cli

import "github.com/spf13/cobra"

func newRegistryCommand(state *rootState) *cobra.Command {
	command := &cobra.Command{
		Use:   "registry",
		Short: "Manage the workspace registry",
		Long: `Manage the global workspace registry at ~/.mi-lsp/registry.toml.
The registry tracks workspace aliases and their root paths.`,
	}

	var dryRun bool
	var apply bool
	gcCommand := &cobra.Command{
		Use:   "gc",
		Short: "Garbage-collect stale workspace registry entries",
		Long: `Remove workspace aliases whose root directories no longer exist.

By default this command performs a dry-run, listing candidates without modifying
the registry. Use --apply to write the changes and create a backup before removing.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if apply && cmd.Flags().Changed("dry-run") && dryRun {
				return cmd.UsageFunc()(cmd)
			}
			return state.executeOperation(cmd, "registry.gc", map[string]any{"dry_run": dryRun, "apply": apply}, false)
		},
	}
	gcCommand.Flags().BoolVar(&dryRun, "dry-run", true, "Preview removal candidates without modifying the registry (default true)")
	gcCommand.Flags().BoolVar(&apply, "apply", false, "Apply removal and create a backup of the registry before writing")

	command.AddCommand(gcCommand)
	return command
}
