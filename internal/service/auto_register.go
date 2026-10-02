package service

import (
	"context"
	"fmt"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// autoRegisterWorkspace is the single entry point shared by the CLI and the
// daemon (both run App.Execute). When the cwd or requested path sits in a Git
// repo that is not registered yet, it persists the registration and starts the
// full index as a deduplicated background job, so the query itself answers
// immediately. Failures never block the query: they surface as warnings.
func (a *App) autoRegisterWorkspace(ctx context.Context, request model.CommandRequest) []string {
	if request.Context.NoAutoRegister || workspace.AutoRegisterDisabledByEnv() {
		return nil
	}
	result, err := workspace.AutoRegisterWorkspace(request.Context.Workspace, request.Context.CallerCWD)
	if err != nil {
		return []string{fmt.Sprintf("auto_register_failed: %v", err)}
	}
	if !result.Registered {
		return nil
	}
	warnings := []string{fmt.Sprintf("auto_registered: workspace %q registered at %s on first query (opt out: --no-auto-register or %s=1)", result.Alias, result.Root, workspace.AutoRegisterEnvVar)}
	// store.CreateIndexJob refuses a second active job per root, which dedups
	// concurrent first queries; the job runs in a detached process.
	indexRequest := model.CommandRequest{
		Operation: "index.start",
		Context:   model.QueryOptions{Workspace: result.Alias, CallerCWD: request.Context.CallerCWD},
		Payload:   map[string]any{},
	}
	envelope, indexErr := a.indexStart(ctx, indexRequest)
	switch {
	case indexErr != nil:
		warnings = append(warnings, "auto_register_index_failed: "+indexErr.Error())
	case envelope.Ok:
		warnings = append(warnings, "auto_register_index: full index running in background; results may be partial until it finishes")
	}
	return warnings
}
