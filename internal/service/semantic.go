package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func (a *App) semantic(ctx context.Context, request model.CommandRequest, method string) (model.Envelope, error) {
	registration, project, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}
	backendType := resolveBackendType(registration, request, method)
	started := time.Now()

	target, targetEnvelope, err := a.resolveSemanticTarget(ctx, registration, project, request, method, backendType)
	if err != nil {
		return model.Envelope{}, err
	}
	if targetEnvelope != nil {
		targetEnvelope.Stats.Ms = time.Since(started).Milliseconds()
		return *targetEnvelope, nil
	}

	if backendType == "catalog" || backendType == "text" {
		env, err := a.semanticFallback(ctx, registration, request, method, backendType, target.Warnings)
		env.Stats.Ms = time.Since(started).Milliseconds()
		return env, err
	}

	payload := clonePayload(request.Payload)
	if target.Entrypoint.Path != "" {
		switch target.Entrypoint.Kind {
		case model.EntrypointKindSolution:
			payload["solution"] = target.Entrypoint.Path
		case model.EntrypointKindProject:
			payload["project_path"] = target.Entrypoint.Path
		}
	}
	workerRequest := model.WorkerRequest{
		ProtocolVersion: model.ProtocolVersion,
		Method:          method,
		Workspace:       registration.Root,
		WorkspaceName:   registration.Name,
		BackendType:     backendType,
		RepoID:          target.Repo.ID,
		RepoName:        target.Repo.Name,
		RepoRoot:        filepath.Join(registration.Root, filepath.FromSlash(target.Repo.Root)),
		EntrypointID:    target.Entrypoint.ID,
		EntrypointPath:  target.Entrypoint.Path,
		EntrypointType:  target.Entrypoint.Kind,
		Payload:         payload,
	}
	response, err := a.Semantic.Call(ctx, registration, workerRequest)
	if err != nil {
		if backendType == "tsserver" && request.Context.BackendHint == "" {
			warnings := append([]string{}, target.Warnings...)
			warnings = append(warnings, semanticBackendWarning("tsserver", err))
			env, fallbackErr := a.semanticFallback(ctx, registration, request, method, "catalog", warnings)
			env.Stats.Ms = time.Since(started).Milliseconds()
			return env, fallbackErr
		}
		if backendType == "pyright" && request.Context.BackendHint == "" {
			warnings := append([]string{}, target.Warnings...)
			warnings = append(warnings, semanticBackendWarning("pyright", err))
			env, fallbackErr := a.semanticFallback(ctx, registration, request, method, "catalog", warnings)
			env.Stats.Ms = time.Since(started).Milliseconds()
			return env, fallbackErr
		}
		return model.Envelope{}, semanticBackendError(backendType, err)
	}
	response.Stats.Ms = time.Since(started).Milliseconds()
	if target.Repo.Name != "" {
		for _, item := range response.Items {
			if _, ok := item["repo"]; !ok {
				item["repo"] = target.Repo.Name
			}
		}
	}
	warnings := append([]string{}, target.Warnings...)
	warnings = append(warnings, response.Warnings...)
	return model.Envelope{
		Ok:        response.Ok,
		Workspace: registration.Name,
		Backend:   response.Backend,
		Items:     response.Items,
		Warnings:  warnings,
		Stats:     response.Stats,
	}, nil
}

func (a *App) resolveSemanticTarget(ctx context.Context, registration model.WorkspaceRegistration, project model.ProjectFile, request model.CommandRequest, method string, backendType string) (semanticTarget, *model.Envelope, error) {
	if backendType == "catalog" || backendType == "text" {
		return semanticTarget{}, nil, nil
	}
	payload := request.Payload
	if entrypointSelector, _ := payload["entrypoint"].(string); strings.TrimSpace(entrypointSelector) != "" {
		entrypoint, ok := workspace.FindEntrypoint(project, entrypointSelector)
		if !ok {
			return semanticTarget{}, ambiguityEnvelope(registration, "unknown entrypoint selector", entrypointCandidates(project), "nav "+friendlyMethodName(method)+" --entrypoint <id>"), nil
		}
		repo, _ := workspace.FindRepo(project, entrypoint.RepoID)
		return semanticTarget{Repo: repo, Entrypoint: entrypoint}, nil, nil
	}
	if explicitSolution, _ := payload["solution"].(string); strings.TrimSpace(explicitSolution) != "" {
		repo, ok := a.resolveRepoForExplicitPath(project, explicitSolution)
		if !ok {
			repo, _ = workspace.FindRepo(project, project.Project.DefaultRepo)
		}
		return semanticTarget{Repo: repo, Entrypoint: explicitEntrypoint(repo, explicitSolution, model.EntrypointKindSolution), Synthetic: true}, nil, nil
	}
	if explicitProject, _ := payload["project_path"].(string); strings.TrimSpace(explicitProject) != "" {
		repo, ok := a.resolveRepoForExplicitPath(project, explicitProject)
		if !ok {
			repo, _ = workspace.FindRepo(project, project.Project.DefaultRepo)
		}
		return semanticTarget{Repo: repo, Entrypoint: explicitEntrypoint(repo, explicitProject, model.EntrypointKindProject), Synthetic: true}, nil, nil
	}
	if repoSelector, _ := payload["repo"].(string); strings.TrimSpace(repoSelector) != "" {
		repo, ok := workspace.FindRepo(project, repoSelector)
		if !ok {
			return semanticTarget{}, ambiguityEnvelope(registration, fmt.Sprintf("unknown repo selector %q", repoSelector), repoCandidates(project.Repos), "--repo <name>"), nil
		}
		return targetForRepo(project, repo, backendType, method)
	}
	if file, _ := payload["file"].(string); strings.TrimSpace(file) != "" {
		repo, ok := workspace.FindRepoByFile(project, registration.Root, file)
		if !ok {
			return semanticTarget{}, ambiguityEnvelope(registration, "file did not match any known repo", repoCandidates(project.Repos), "--repo <name>"), nil
		}
		return targetForRepo(project, repo, backendType, method)
	}
	if registration.Kind == model.WorkspaceKindSingle {
		repo, _ := workspace.FindRepo(project, project.Project.DefaultRepo)
		return targetForRepo(project, repo, backendType, method)
	}
	if symbol, _ := payload["symbol"].(string); strings.TrimSpace(symbol) != "" {
		db, err := store.Open(registration.Root)
		if err != nil {
			return semanticTarget{}, nil, err
		}
		defer db.Close()
		exact, _ := payload["exact"].(bool)
		candidates, err := store.CandidateReposForSymbol(ctx, db, symbol, exact, 12)
		if err != nil {
			return semanticTarget{}, nil, err
		}
		if len(candidates) == 1 {
			return targetForRepo(project, candidates[0], backendType, method)
		}
		if len(candidates) > 1 {
			return semanticTarget{}, ambiguityEnvelope(registration, fmt.Sprintf("symbol %q exists in multiple repos", symbol), repoCandidates(candidates), "--repo <name> or --file <path>"), nil
		}
	}
	defaultRepo, ok := workspace.FindRepo(project, project.Project.DefaultRepo)
	if !ok {
		return semanticTarget{}, ambiguityEnvelope(registration, "no default repo available for semantic routing", repoCandidates(project.Repos), "--repo <name>"), nil
	}
	return targetForRepo(project, defaultRepo, backendType, method)
}

func (a *App) resolveRepoForExplicitPath(project model.ProjectFile, selector string) (model.WorkspaceRepo, bool) {
	normalized := filepath.ToSlash(strings.TrimSpace(selector))
	for _, entrypoint := range project.Entrypoints {
		if strings.EqualFold(entrypoint.Path, normalized) {
			return workspace.FindRepo(project, entrypoint.RepoID)
		}
	}
	return workspace.FindRepo(project, project.Project.DefaultRepo)
}

func (a *App) semanticFallback(ctx context.Context, registration model.WorkspaceRegistration, request model.CommandRequest, method string, backendType string, warnings []string) (model.Envelope, error) {
	switch method {
	case "get_context":
		return a.catalogContextFallback(ctx, registration, request, warnings)
	case "find_refs":
		return a.textReferenceFallback(ctx, registration, request, warnings)
	default:
		return model.Envelope{Ok: false, Workspace: registration.Name, Backend: backendType, Items: []map[string]any{}, Warnings: append(warnings, "no fallback available for this semantic query")}, nil
	}
}

func (a *App) catalogContextFallback(ctx context.Context, registration model.WorkspaceRegistration, request model.CommandRequest, warnings []string) (model.Envelope, error) {
	file, _ := request.Payload["file"].(string)
	line := intFromAny(request.Payload["line"], 1)
	if file == "" {
		return model.Envelope{}, errors.New("file is required")
	}
	relativeFile, err := makeRelative(registration.Root, file)
	if err != nil {
		return model.Envelope{}, err
	}
	db, err := store.Open(registration.Root)
	if err != nil {
		return model.Envelope{}, err
	}
	defer db.Close()
	symbols, err := store.SymbolsByFile(ctx, db, relativeFile, DefaultConfig().DefaultSearchLimit)
	if err != nil {
		return model.Envelope{}, err
	}
	if len(symbols) == 0 {
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "catalog", Items: []map[string]any{}, Warnings: append(warnings, "catalog fallback found no symbol at that location")}, nil
	}
	best := symbols[0]
	for _, symbol := range symbols {
		if symbol.StartLine <= line {
			best = symbol
		}
	}
	items := []map[string]any{{
		"name":           best.Name,
		"kind":           best.Kind,
		"file":           best.FilePath,
		"line":           best.StartLine,
		"scope":          best.Scope,
		"signature":      best.Signature,
		"qualified_name": best.QualifiedName,
		"repo":           best.RepoName,
	}}
	warnings = append(warnings, "served from catalog fallback")
	return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "catalog", Items: items, Warnings: warnings, Stats: model.Stats{Symbols: len(items)}}, nil
}

func (a *App) textReferenceFallback(ctx context.Context, registration model.WorkspaceRegistration, request model.CommandRequest, warnings []string) (model.Envelope, error) {
	project, err := workspace.LoadProjectTopology(registration.Root, registration)
	if err != nil {
		return model.Envelope{}, err
	}
	symbol, _ := request.Payload["symbol"].(string)
	if symbol == "" {
		return model.Envelope{}, errors.New("symbol is required")
	}
	items, err := searchPattern(ctx, registration.Root, project, symbol, false, request.Context.MaxItems)
	if err != nil {
		return model.Envelope{}, err
	}
	warnings = append(warnings, "served from text fallback; results are textual occurrences, not semantic references")
	return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "text", Items: items, Warnings: warnings, Stats: model.Stats{Files: len(items)}}, nil
}

func resolveBackendType(registration model.WorkspaceRegistration, request model.CommandRequest, method string) string {
	explicit := strings.ToLower(strings.TrimSpace(request.Context.BackendHint))
	if explicit != "" {
		return explicit
	}
	if method == "get_deps" {
		return "roslyn"
	}
	file, _ := request.Payload["file"].(string)
	if isTypeScriptFile(file) {
		return "tsserver"
	}
	if isPythonFile(file) {
		return "pyright"
	}
	if method == "find_refs" && !hasLanguage(registration, "csharp") && hasLanguage(registration, "typescript") {
		return "tsserver"
	}
	if method == "find_refs" && hasLanguage(registration, "python") && !hasLanguage(registration, "csharp") && !hasLanguage(registration, "typescript") {
		return "pyright"
	}
	return "roslyn"
}

func targetForRepo(project model.ProjectFile, repo model.WorkspaceRepo, backendType string, method string) (semanticTarget, *model.Envelope, error) {
	if backendType == "tsserver" {
		return semanticTarget{Repo: repo, Entrypoint: explicitEntrypoint(repo, repo.Root, "repo"), Synthetic: true}, nil, nil
	}
	if backendType == "pyright" {
		return semanticTarget{Repo: repo, Entrypoint: explicitEntrypoint(repo, repo.Root, "repo"), Synthetic: true}, nil, nil
	}
	entrypoint, ok := workspace.DefaultEntrypointForRepo(project, repo.ID)
	if ok {
		return semanticTarget{Repo: repo, Entrypoint: entrypoint}, nil, nil
	}
	return semanticTarget{}, &model.Envelope{
		Ok:        false,
		Backend:   backendType,
		Workspace: project.Project.Name,
		Items:     repoCandidates([]model.WorkspaceRepo{repo}),
		Warnings:  []string{fmt.Sprintf("repo %q has no semantic entrypoint for %s", repo.Name, friendlyMethodName(method))},
	}, nil
}

func explicitEntrypoint(repo model.WorkspaceRepo, path string, kind string) model.WorkspaceEntrypoint {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
	if normalizedPath == "" {
		normalizedPath = repo.Root
	}
	entrypointID := repo.ID + "::" + strings.ReplaceAll(strings.Trim(normalizedPath, "/"), "/", "-")
	entrypointID = strings.Trim(entrypointID, "-")
	if entrypointID == "" {
		entrypointID = repo.ID + "::default"
	}
	return model.WorkspaceEntrypoint{
		ID:      entrypointID,
		RepoID:  repo.ID,
		Path:    normalizedPath,
		Kind:    kind,
		Default: true,
	}
}

func ambiguityEnvelope(registration model.WorkspaceRegistration, warning string, candidates []map[string]any, rerunHint string) *model.Envelope {
	hint := "rerun with " + rerunHint
	return &model.Envelope{
		Ok:        false,
		Workspace: registration.Name,
		Backend:   "router",
		Items:     candidates,
		Warnings:  []string{warning},
		NextHint:  &hint,
	}
}

func repoCandidates(repos []model.WorkspaceRepo) []map[string]any {
	items := make([]map[string]any, 0, len(repos))
	for _, repo := range repos {
		items = append(items, map[string]any{"repo": repo.Name, "repo_id": repo.ID, "root": repo.Root, "default_entrypoint": repo.DefaultEntrypoint})
	}
	return items
}

func entrypointCandidates(project model.ProjectFile) []map[string]any {
	items := make([]map[string]any, 0, len(project.Entrypoints))
	for _, entrypoint := range project.Entrypoints {
		items = append(items, map[string]any{"entrypoint_id": entrypoint.ID, "repo_id": entrypoint.RepoID, "path": entrypoint.Path, "kind": entrypoint.Kind, "default": entrypoint.Default})
	}
	return items
}

func friendlyMethodName(method string) string {
	switch method {
	case "find_refs":
		return "refs"
	case "get_context":
		return "context"
	case "get_deps":
		return "deps"
	default:
		return method
	}
}

func hasLanguage(registration model.WorkspaceRegistration, target string) bool {
	for _, language := range registration.Languages {
		if strings.EqualFold(language, target) {
			return true
		}
	}
	return false
}

func isTypeScriptFile(path string) bool {
	extension := strings.ToLower(filepath.Ext(path))
	switch extension {
	case ".ts", ".tsx", ".js", ".jsx", ".mts", ".cts":
		return true
	default:
		return false
	}
}

func isPythonFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".py" || ext == ".pyi"
}
