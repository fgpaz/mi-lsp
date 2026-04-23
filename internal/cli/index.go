package cli

import "github.com/spf13/cobra"

func newIndexCommand(state *rootState) *cobra.Command {
	var clean bool
	var docsOnly bool
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
			mode := "full"
			if docsOnly {
				mode = "docs"
			}
			return state.executeOperation(cmd, "index.start", map[string]any{"path": path, "clean": clean, "docs_only": docsOnly, "mode": mode, "wait": true}, false)
		},
	}
	command.Flags().BoolVar(&clean, "clean", false, "Reset the workspace index before indexing")
	command.Flags().BoolVar(&docsOnly, "docs-only", false, "Rebuild only the documentation graph and reentry memory")
	command.AddCommand(newIndexStartCommand(state), newIndexStatusCommand(state), newIndexCancelCommand(state), newIndexRunJobCommand(state))
	return command
}

func newIndexStartCommand(state *rootState) *cobra.Command {
	var mode string
	var clean bool
	var wait bool
	command := &cobra.Command{
		Use:   "start [path]",
		Short: "Start an index job for a workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) > 0 {
				path = args[0]
			}
			return state.executeOperation(cmd, "index.start", map[string]any{"path": path, "mode": mode, "clean": clean, "wait": wait}, false)
		},
	}
	command.Flags().StringVar(&mode, "mode", "full", "Index mode: full|docs|catalog")
	command.Flags().BoolVar(&clean, "clean", false, "Force a full replacement publish for the selected mode")
	command.Flags().BoolVar(&wait, "wait", false, "Run the job in the current process and wait for completion")
	return command
}

func newIndexStatusCommand(state *rootState) *cobra.Command {
	return &cobra.Command{
		Use:   "status [job-id]",
		Short: "Show index job status for the selected workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			jobID := ""
			if len(args) > 0 {
				jobID = args[0]
			}
			return state.executeOperation(cmd, "index.status", map[string]any{"job_id": jobID}, false)
		},
	}
}

func newIndexCancelCommand(state *rootState) *cobra.Command {
	var force bool
	command := &cobra.Command{
		Use:   "cancel <job-id>",
		Short: "Request cancellation for an index job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return state.executeOperation(cmd, "index.cancel", map[string]any{"job_id": args[0], "force": force}, false)
		},
	}
	command.Flags().BoolVar(&force, "force", false, "Terminate the live index job process immediately when present")
	return command
}

func newIndexRunJobCommand(state *rootState) *cobra.Command {
	return &cobra.Command{
		Use:    "run-job <job-id>",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return state.executeOperation(cmd, "index.run-job", map[string]any{"job_id": args[0]}, false)
		},
	}
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
