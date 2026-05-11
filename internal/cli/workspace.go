package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

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

	var listGroupByRoot bool
	listCommand := &cobra.Command{
		Use:   "list",
		Short: "List registered workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{}
			if listGroupByRoot {
				payload["group_by_root"] = true
			}
			return state.executeOperation(cmd, "workspace.list", payload, false)
		},
	}
	listCommand.Flags().BoolVar(&listGroupByRoot, "group-by-root", false, "Group aliases by exact workspace root without mutating the registry")

	doctorCommand := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose workspace aliases, worktrees, stale paths, and binary shadowing",
		RunE: func(cmd *cobra.Command, args []string) error {
			return state.executeOperation(cmd, "workspace.doctor", nil, false)
		},
	}

	var pruneStale bool
	var pruneApply bool
	var pruneDryRun bool
	pruneCommand := &cobra.Command{
		Use:   "prune",
		Short: "Prune stale workspace aliases from the registry",
		Long: `Prune stale workspace aliases from the global registry.

By default this command is a dry run. Use --apply to remove only aliases whose
		registered root no longer exists. It never deletes files or Git worktrees.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !pruneStale {
				return fmt.Errorf("workspace prune requires --stale")
			}
			if pruneApply && cmd.Flags().Changed("dry-run") && pruneDryRun {
				return fmt.Errorf("--apply cannot be combined with --dry-run")
			}
			return state.executeOperation(cmd, "workspace.prune", map[string]any{"stale": pruneStale, "apply": pruneApply}, false)
		},
	}
	pruneCommand.Flags().BoolVar(&pruneStale, "stale", false, "Prune aliases whose registered root no longer exists")
	pruneCommand.Flags().BoolVar(&pruneApply, "apply", false, "Apply the prune instead of previewing it")
	pruneCommand.Flags().BoolVar(&pruneDryRun, "dry-run", true, "Preview prune candidates without mutating the registry")

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

	command.AddCommand(addCommand, scanCommand, listCommand, doctorCommand, pruneCommand, warmCommand, statusCommand, removeCommand)
	return command
}
