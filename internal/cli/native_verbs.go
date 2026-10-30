package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newNativeVerbCommands(state *rootState) []*cobra.Command {
	var findRepo string
	findSymbol := &cobra.Command{
		Use:   "find_symbol <query>",
		Short: "Resolve a symbol with semantic references when uniquely available",
		Long:  "Cold/catalog results are approximate and marked; ambiguity never selects an arbitrary symbol for semantic references.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{"query": strings.Join(args, " ")}
			if findRepo != "" {
				payload["repo"] = findRepo
			}
			return state.executeOperation(cmd, "find_symbol", payload, true)
		},
	}
	attachCatalogRepoFlag(findSymbol, &findRepo)

	var newName, renameFile, renameRepo, renameEntrypoint, renameSolution, renameProject string
	var renameLine, renameColumn int
	rename := &cobra.Command{
		Use:   "rename_symbol <symbol>",
		Short: "Apply a complete versioned gopls rename plan to a Go symbol",
		Long:  "Requires only symbol and --new-name. Without --file/--line/--column, the live gopls workspace-symbol result must uniquely resolve and pass prepareRename; ambiguity never edits. Supply all three coordinates to select explicitly. Only a complete, versioned semantic plan is applied; unsupported or stale plans leave files unchanged. Diagnostics are not a compilation check. change_signature is unavailable because no backend exposes a safe signature-change plan.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(newName) == "" {
				return fmt.Errorf("--new-name is required")
			}
			payload := semanticPayload(renameRepo, renameEntrypoint, renameSolution, renameProject)
			payload["symbol"] = args[0]
			payload["new_name"] = newName
			if cmd.Flags().Changed("file") {
				payload["file"] = renameFile
			}
			if cmd.Flags().Changed("line") {
				payload["line"] = renameLine
			}
			if cmd.Flags().Changed("column") {
				payload["column"] = renameColumn
			}
			return state.executeOperation(cmd, "rename_symbol", payload, true)
		},
	}
	rename.Flags().StringVar(&newName, "new-name", "", "New identifier")
	rename.Flags().StringVar(&renameFile, "file", "", "Workspace-relative or absolute Go source file")
	rename.Flags().IntVar(&renameLine, "line", 0, "1-based line containing the selected symbol")
	rename.Flags().IntVar(&renameColumn, "column", 0, "1-based UTF-16 column containing the selected symbol")
	attachSemanticSelectorFlags(rename, &renameRepo, &renameEntrypoint, &renameSolution, &renameProject)

	var maxFiles int
	var base string
	var explicitFiles []string
	getErrors := &cobra.Command{
		Use:   "get_errors [file ...]",
		Short: "Return semantic diagnostics for explicit files or worktree changes",
		Long:  "Accepts positional paths and repeatable --file paths; with neither, checks staged, unstaged, and untracked source files. Reports not_checked/partial explicitly; diagnostics do not prove that a project compiles.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			files := append(append([]string(nil), args...), explicitFiles...)
			if len(files) > 0 && base != "" {
				return fmt.Errorf("--base is only valid when using the changed-file default")
			}
			if maxFiles < 1 || maxFiles > 64 {
				return fmt.Errorf("--max-files must be between 1 and 64")
			}
			payload := map[string]any{"max_files": maxFiles}
			if len(files) > 0 {
				payload["files"] = files
			} else {
				payload["base"] = base
			}
			return state.executeOperation(cmd, "get_errors", payload, true)
		},
	}
	getErrors.Flags().StringArrayVar(&explicitFiles, "file", nil, "Explicit source file to check (repeatable; may be combined with positional paths)")
	getErrors.Flags().IntVar(&maxFiles, "max-files", 32, "Maximum changed or explicit files checked (1–64)")
	getErrors.Flags().StringVar(&base, "base", "", "Also include paths since merge-base of this ref and HEAD (changed-file default only)")

	return []*cobra.Command{findSymbol, rename, getErrors}
}
