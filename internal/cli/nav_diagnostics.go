package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newDiagnosticsCommand is registered by the nav surface owner with
// command.AddCommand(newDiagnosticsCommand(state)).
func newDiagnosticsCommand(state *rootState) *cobra.Command {
	var changed bool
	var base string
	var maxFiles, offset int
	command := &cobra.Command{
		Use:   "diagnostics [file]",
		Short: "Real semantic errors and warnings for a file or git changes",
		Long:  "Read semantic backend diagnostics, never a build/test or text fallback. --changed unions staged, unstaged and untracked (gitignore-respecting) paths; --base also includes changes from merge-base(base, HEAD). Sources are read from the current working tree, not index blobs. Partial/unavailable reports are not clean results.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if changed == (len(args) == 1) {
				return fmt.Errorf("pass one file or --changed, exclusively")
			}
			if base != "" && !changed {
				return fmt.Errorf("--base requires --changed")
			}
			if maxFiles < 1 || maxFiles > 64 {
				return fmt.Errorf("--max-files must be between 1 and 64")
			}
			if offset < 0 {
				return fmt.Errorf("--offset must be nonnegative")
			}
			payload := map[string]any{"changed": changed, "base": base, "max_files": maxFiles, "offset": offset}
			if len(args) == 1 {
				payload["file"] = args[0]
			}
			return state.executeNavOperation(cmd, "nav.diagnostics", payload, true)
		},
	}
	command.Flags().BoolVar(&changed, "changed", false, "Diagnose staged, unstaged and untracked paths in the selected worktree")
	command.Flags().StringVar(&base, "base", "", "Also include paths changed since merge-base of this ref and HEAD")
	command.Flags().IntVar(&maxFiles, "max-files", 32, "Maximum files diagnosed (1–64; other files are explicit omissions)")
	command.Flags().IntVar(&offset, "offset", 0, "Skip diagnostic items after sorting errors before warnings (fresh snapshot)")
	return command
}
