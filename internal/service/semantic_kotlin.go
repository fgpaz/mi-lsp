package service

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/worker"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// Kotlin deliberately bypasses catalog/name anchoring and all text fallbacks.
func (a *App) kotlinSemantic(ctx context.Context, registration model.WorkspaceRegistration, project model.ProjectFile, request model.CommandRequest, method string) (model.Envelope, error) {
	unavailable := func(code, message string) model.Envelope {
		return model.Envelope{Ok: false, Workspace: registration.Name, Backend: worker.KotlinBackend, Items: []map[string]any{}, Reason: code, Error: &model.EnvelopeError{Kind: "semantic", Code: code, ReasonCode: code, Message: message, Stage: "kotlin_runtime"}}
	}
	switch method {
	case "get_context", "find_refs", "find_definition", "document_symbols":
	default:
		return unavailable("unsupported_operation", "Kotlin calls/dependencies/compiler graph are not supplied by K1"), nil
	}
	file, _ := request.Payload["file"].(string)
	if strings.TrimSpace(file) == "" || !isKotlinFile(file) {
		return unavailable("unavailable", "explicit .kt file required; .kts scripts excluded"), nil
	}
	selector, _ := request.Payload["entrypoint"].(string)
	if selector == "" {
		selector, _ = request.Payload["project_path"].(string)
	}
	if strings.TrimSpace(selector) == "" {
		return unavailable("unavailable", "select an explicit Gradle module build file with --entrypoint"), nil
	}
	entry, ok := workspace.FindEntrypoint(project, selector)
	if !ok {
		return unavailable("unavailable", "unknown module entrypoint; declare build.gradle/build.gradle.kts in workspace topology"), nil
	}
	repo, ok := workspace.FindRepo(project, entry.RepoID)
	if !ok {
		return unavailable("unavailable", "entrypoint repo unavailable"), nil
	}
	req := model.WorkerRequest{ProtocolVersion: model.ProtocolVersion, Method: method, Workspace: registration.Root, WorkspaceName: registration.Name, BackendType: worker.KotlinBackend, RepoID: repo.ID, RepoName: repo.Name, RepoRoot: filepath.Join(registration.Root, filepath.FromSlash(repo.Root)), EntrypointID: entry.ID, EntrypointPath: entry.Path, EntrypointType: entry.Kind, Payload: clonePayload(request.Payload)}
	response, err := a.callSemanticWorker(ctx, registration, req, method)
	if err != nil {
		code := "unavailable"
		if strings.HasPrefix(err.Error(), "unsupported_operation:") {
			code = "unsupported_operation"
		}
		return unavailable(code, err.Error()), nil
	}
	if !response.Ok {
		return unavailable(response.ErrorCode, response.Error), nil
	}
	for _, item := range response.Items {
		item["position_encoding"] = "utf-16"
		item["coverage"] = "partial"
		if path, _ := item["file"].(string); path != "" {
			if relative, err := makeRelative(registration.Root, path); err == nil && !strings.HasPrefix(relative, "..") {
				item["file"] = relative
			}
		}
	}
	return model.Envelope{Ok: true, Partial: true, Reason: "explicit_incomplete", Workspace: registration.Name, Backend: worker.KotlinBackend, Items: response.Items, Warnings: response.Warnings, Stats: response.Stats}, nil
}
