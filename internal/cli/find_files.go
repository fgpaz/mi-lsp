package cli

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/service"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// newFindFilesCommand is registered by the root surface owner. It intentionally
// bypasses semantic dispatch: no daemon, workers, registration or index refresh.
func newFindFilesCommand(state *rootState) *cobra.Command {
	var top, budget int
	var literal, refresh bool
	finder := service.NewFileFinder()
	cmd := &cobra.Command{
		Use:   "find-files <name|glob>",
		Short: "Find relative filenames quickly, respecting Git ignore rules (JSON)",
		Long: `Find filenames, not contents. Default boundary is the current directory;
--workspace accepts a directory or registered alias. Never follows symlinks.
Plain text is a case-insensitive substring: exact basename > basename prefix >
path segment > remainder. Globs (* ? [abc]) match basenames; with '/' they match
relative paths, and ** matches zero or more directories. '*.go' finds extensions.
Use --literal for metacharacters in filenames, and -- before names starting '-'.
Git includes tracked files (even if ignored) and nonignored untracked files;
otherwise existing rg/fd supplies nested .gitignore semantics. No shell is used.
Listings may be cached for 5 seconds; --refresh bypasses that snapshot.
Output is find-files/v1 JSON only, capped by --top and --budget-bytes.`,
		Example: "  mi-lsp find-files '*.go' --top 8 --budget-bytes 2048\n  mi-lsp find-files 'src/**/config?.json'\n  mi-lsp find-files -- '-draft'",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fail := func(err error) error {
				var typed *model.FindFilesError
				if !errors.As(err, &typed) {
					typed = &model.FindFilesError{Code: "find_files_failed", Message: "filename search failed"}
				}
				if writeErr := json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
					Error *model.FindFilesError `json:"error"`
				}{typed}); writeErr != nil {
					return writeErr
				}
				return envelopePrintedError{err: typed}
			}
			if flagChanged(cmd, "format") && state.format != "json" {
				return fail(&model.FindFilesError{Code: "unsupported_format", Message: "find-files returns JSON; use --format json"})
			}
			root := state.workspace
			if root == "" {
				root = "."
			}
			if info, err := os.Stat(root); err != nil || !info.IsDir() {
				cwd, err := os.Getwd()
				if err != nil {
					return fail(&model.FindFilesError{Code: "invalid_workspace", Message: "current directory unavailable"})
				}
				resolution, err := workspace.ResolveWorkspaceSelectionReadOnly(root, cwd)
				if err != nil {
					return fail(&model.FindFilesError{Code: "invalid_workspace", Message: "workspace directory or alias unavailable"})
				}
				root = resolution.Registration.Root
			}
			limit, topLimit := budget, top
			// Honor inherited output ceilings without routing through a formatter
			// which might expand/truncate JSON into an invalid or larger envelope.
			if flagChanged(cmd, "max-items") && state.maxItems < topLimit {
				topLimit = state.maxItems
			}
			if flagChanged(cmd, "max-chars") && state.maxChars > 0 && state.maxChars < limit {
				limit = state.maxChars
			}
			if flagChanged(cmd, "token-budget") && state.tokenBudget > 0 && state.tokenBudget < limit/4 {
				limit = state.tokenBudget * 4
			}
			result, err := finder.FindFiles(cmd.Context(), root, args[0], service.FindFilesOptions{Top: topLimit, MaxBytes: limit, Literal: literal, Refresh: refresh})
			if err != nil {
				return fail(err)
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}
	cmd.Flags().IntVar(&top, "top", 20, "Maximum filenames (1..1000), before byte budget")
	cmd.Flags().IntVar(&budget, "budget-bytes", 4096, "Maximum complete JSON bytes including newline (512..1048576)")
	cmd.Flags().BoolVar(&literal, "literal", false, "Treat glob metacharacters as literal filename text")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Bypass the five-second filename listing cache")
	return cmd
}
