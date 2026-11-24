package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/statedir"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// indexRevisionWarning reports a nav read answered from an index built at a
// different Git HEAD (RF-WKS-007-B04). Only the machine state location checks:
// its state survives checkouts and `git clean`, and no daemon watcher follows
// them. Detection only: the read never reindexes, so it stays read-only and
// predictable on build hosts.
func indexRevisionWarning(ctx context.Context, request model.CommandRequest) string {
	if !statedir.Machine() || !strings.HasPrefix(request.Operation, "nav.") {
		return ""
	}
	resolution, err := workspace.ResolveReadRoot(request.Context.Workspace, request.Context.CallerCWD)
	if err != nil || strings.TrimSpace(resolution.Registration.Root) == "" {
		return ""
	}
	root := resolution.Registration.Root
	indexed, err := store.ReadIndexedGitHead(ctx, root)
	if err != nil || indexed == "" {
		return ""
	}
	current := workspace.GitHead(ctx, root)
	if current == "" || current == indexed {
		return ""
	}
	return fmt.Sprintf("index_stale_revision: index built at %s but HEAD is %s; results may be stale; run `mi-lsp index --clean` to rebuild it at HEAD", indexed, current)
}
