package cli

import "github.com/spf13/cobra"

func newIndexCommand(state *rootState) *cobra.Command {
	var clean bool
	var docsOnly bool
	var goOffline bool
	var noEmbeddings bool
	var entrypoint string
	command := &cobra.Command{
		Use:   "index [path]",
		Short: "Index the current or selected workspace into repo-local SQLite",
		Long: `Parse source files and store symbols in the repo-local .mi-lsp/index.db.
Supports C#, TypeScript, and Go via built-in tree-sitter parsers.
Use --no-embeddings to skip wiki embedding work for this operation without changing project defaults or the documentation graph.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) > 0 {
				path = args[0]
			}
			mode := "full"
			if docsOnly {
				mode = "docs"
			}
			return state.executeOperation(cmd, "index.start", map[string]any{"path": path, "clean": clean, "docs_only": docsOnly, "go_offline": goOffline, "no_embeddings": noEmbeddings, "mode": mode, "entrypoint": entrypoint, "wait": true}, false)
		},
	}
	command.Flags().BoolVar(&clean, "clean", false, "Reset the workspace index before indexing")
	command.Flags().BoolVar(&goOffline, "go-offline", false, "Observe Go using only the local toolchain and cached dependencies (no downloads)")
	command.Flags().BoolVar(&noEmbeddings, "no-embeddings", false, "Skip wiki embeddings for this operation; keep docs graph and existing vectors unchanged")
	command.Flags().BoolVar(&docsOnly, "docs-only", false, "Rebuild only the documentation graph and reentry memory")
	command.Flags().StringVar(&entrypoint, "entrypoint", "", "Select a declared entrypoint ID or safe repository-relative Go/C# path")
	command.AddCommand(newIndexStartCommand(state), newIndexStatusCommand(state), newIndexCancelCommand(state), newIndexRunJobCommand(state))
	return command
}

func newIndexStartCommand(state *rootState) *cobra.Command {
	var mode string
	var clean bool
	var wait bool
	var goOffline bool
	var noEmbeddings bool
	var entrypoint string
	command := &cobra.Command{
		Use:   "start [path]",
		Short: "Start an index job for a workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) > 0 {
				path = args[0]
			}
			return state.executeOperation(cmd, "index.start", map[string]any{"path": path, "mode": mode, "clean": clean, "go_offline": goOffline, "no_embeddings": noEmbeddings, "entrypoint": entrypoint, "wait": wait}, false)
		},
	}
	command.Flags().StringVar(&mode, "mode", "full", "Index mode: full|docs|catalog")
	command.Flags().BoolVar(&clean, "clean", false, "Force a full replacement publish for the selected mode")
	command.Flags().BoolVar(&goOffline, "go-offline", false, "Observe Go using only the local toolchain and cached dependencies (no downloads)")
	command.Flags().BoolVar(&noEmbeddings, "no-embeddings", false, "Skip wiki embeddings for this operation; keep docs graph and existing vectors unchanged")
	command.Flags().BoolVar(&wait, "wait", false, "Run the job in the current process and wait for completion")
	command.Flags().StringVar(&entrypoint, "entrypoint", "", "Select a declared entrypoint ID or safe repository-relative Go/C# path")
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
	var noEmbeddings bool
	command := &cobra.Command{
		Use:    "run-job <job-id>",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{"job_id": args[0]}
			if cmd.Flags().Changed("no-embeddings") {
				payload["no_embeddings"] = noEmbeddings
			}
			return state.executeOperation(cmd, "index.run-job", payload, false)
		},
	}
	command.Flags().BoolVar(&noEmbeddings, "no-embeddings", false, "Internal persisted index job policy")
	_ = command.Flags().MarkHidden("no-embeddings")
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
