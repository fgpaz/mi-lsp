package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/output"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

type workspaceWhichResult struct {
	Name       string
	Root       string
	Executable string
	Source     string
}

func newWorkspaceWhichCommand(state *rootState) *cobra.Command {
	var cwd string
	command := &cobra.Command{
		Use:   "which",
		Short: "Show the resolved workspace name, root, and executable",
		Long: `Resolve a workspace with the same read-only rules as nav.

Precedence matches the rest of the CLI: explicit --workspace, then the registered root or Git checkout that contains --cwd (or the process cwd), then last_workspace. A linked worktree resolves to its own physical root, never to last_workspace. This command does not start the daemon, open an index, or use the network, and it does not change the registry.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved, err := resolveWorkspaceWhich(state.workspace, cwd)
			if err != nil {
				return err
			}
			opts := state.queryOptions(cmd, "workspace.which", nil)
			env := model.Envelope{
				Ok:        true,
				Workspace: resolved.Name,
				Backend:   "registry",
				Items: []map[string]any{{
					"name":       resolved.Name,
					"root":       resolved.Root,
					"executable": resolved.Executable,
					"source":     resolved.Source,
				}},
			}
			env = output.ApplyEnvelopeLimits(env, opts)
			return state.printPreparedEnvelope(env, opts)
		},
	}
	command.Flags().StringVar(&cwd, "cwd", "", "Directory to resolve when --workspace is omitted")
	return command
}

// resolveWorkspaceWhich reuses the read-only resolver that nav uses. A second
// resolver here answered last_workspace for unregistered Git checkouts and
// linked worktrees, which is the wrong workspace for the caller.
func resolveWorkspaceWhich(explicit string, cwd string) (workspaceWhichResult, error) {
	explicit = literalWorkspacePath(explicit)
	lookupCWD := literalWorkspacePath(cwd)
	if lookupCWD == "" {
		if processCWD, cwdErr := os.Getwd(); cwdErr == nil {
			lookupCWD = literalWorkspacePath(processCWD)
		}
	}
	resolution, err := workspace.ResolveWorkspaceSelectionReadOnlyPhysical(explicit, lookupCWD)
	if err != nil {
		return workspaceWhichResult{}, err
	}
	return workspaceWhichResult{
		Name:       strings.TrimSpace(resolution.Registration.Name),
		Root:       literalWorkspacePath(resolution.Registration.Root),
		Executable: currentExecutablePath(),
		Source:     string(resolution.Source),
	}, nil
}

// literalWorkspacePath keeps a Windows path unchanged.
// strconv.Unquote and fmt-built JSON treat \r as a carriage return and drop the backslash before \m and \p.
func literalWorkspacePath(path string) string {
	return strings.TrimSpace(path)
}

func currentExecutablePath() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Clean(path)
}
