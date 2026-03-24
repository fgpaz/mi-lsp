package service

import (
	"context"
	"errors"
	"os"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func (a *App) workspaceAdd(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	path, _ := request.Payload["path"].(string)
	alias, _ := request.Payload["alias"].(string)
	if path == "" {
		return model.Envelope{}, errors.New("path is required")
	}
	registration, project, err := workspace.DetectWorkspaceLayout(path, alias)
	if err != nil {
		return model.Envelope{}, err
	}
	if alias == "" {
		alias = registration.Name
	}
	registration.Name = alias
	project.Project.Name = alias
	registration = workspace.ApplyProjectTopology(registration, project)
	if _, err := workspace.RegisterWorkspace(alias, registration); err != nil {
		return model.Envelope{}, err
	}
	if err := workspace.SaveProjectFile(registration.Root, project); err != nil {
		return model.Envelope{}, err
	}

	item := workspaceSummaryItem(registration, project)
	warnings := []string{}

	// Check for no_index flag (backward compatibility)
	noIndex, _ := request.Payload["no_index"].(bool)
	if !noIndex {
		indexResult, indexErr := indexer.IndexWorkspace(ctx, registration.Root, false)
		if indexErr != nil {
			// Don't fail the add — just warn
			warnings = append(warnings, "auto-index failed: "+indexErr.Error())
		} else {
			item["index_symbols"] = indexResult.Stats.Symbols
			item["index_files"] = indexResult.Stats.Files
			item["index_ms"] = indexResult.Stats.Ms
		}
	}

	return model.Envelope{Ok: true, Workspace: alias, Backend: "registry", Items: []map[string]any{item}, Warnings: warnings}, nil
}

func (a *App) workspaceScan() (model.Envelope, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return model.Envelope{}, err
	}
	workspaces, err := workspace.ScanCandidates(cwd)
	if err != nil {
		return model.Envelope{}, err
	}
	items := make([]map[string]any, 0, len(workspaces))
	for _, registration := range workspaces {
		project, loadErr := workspace.LoadProjectTopology(registration.Root, registration)
		if loadErr != nil {
			items = append(items, map[string]any{"name": registration.Name, "root": registration.Root, "kind": registration.Kind, "languages": registration.Languages})
			continue
		}
		registration = workspace.ApplyProjectTopology(registration, project)
		items = append(items, workspaceSummaryItem(registration, project))
	}
	return model.Envelope{Ok: true, Backend: "scanner", Items: items}, nil
}

func (a *App) workspaceList() (model.Envelope, error) {
	workspaces, err := workspace.ListWorkspaces()
	if err != nil {
		return model.Envelope{}, err
	}
	items := make([]map[string]any, 0, len(workspaces))
	for _, registration := range workspaces {
		project, loadErr := workspace.LoadProjectTopology(registration.Root, registration)
		if loadErr != nil {
			items = append(items, map[string]any{"name": registration.Name, "root": registration.Root, "kind": registration.Kind, "languages": registration.Languages})
			continue
		}
		registration = workspace.ApplyProjectTopology(registration, project)
		items = append(items, workspaceSummaryItem(registration, project))
	}
	return model.Envelope{Ok: true, Backend: "registry", Items: items}, nil
}

func (a *App) workspaceStatus(ctx context.Context, name string) (model.Envelope, error) {
	registration, project, err := a.resolveWorkspaceWithProject(name)
	if err != nil {
		return model.Envelope{}, err
	}
	item := workspaceSummaryItem(registration, project)
	item["repos"] = project.Repos
	item["entrypoints"] = project.Entrypoints
	db, err := store.Open(registration.Root)
	if err != nil {
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "sqlite", Items: []any{item}, Warnings: []string{"workspace has no index yet"}}, nil
	}
	defer db.Close()
	stats, err := store.WorkspaceStats(ctx, db)
	if err != nil {
		return model.Envelope{}, err
	}
	return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "sqlite", Items: []any{item}, Stats: stats}, nil
}

func (a *App) workspaceRemove(request model.CommandRequest) (model.Envelope, error) {
	name, _ := request.Payload["name"].(string)
	if name == "" {
		return model.Envelope{}, errors.New("workspace name is required")
	}
	if err := workspace.RemoveWorkspace(name); err != nil {
		return model.Envelope{}, err
	}
	return model.Envelope{Ok: true, Backend: "registry", Items: []map[string]any{{"removed": name}}}, nil
}

func (a *App) resolveWorkspaceWithProject(name string) (model.WorkspaceRegistration, model.ProjectFile, error) {
	registration, err := a.ResolveWorkspace(name)
	if err != nil {
		return model.WorkspaceRegistration{}, model.ProjectFile{}, err
	}
	project, err := workspace.LoadProjectTopology(registration.Root, registration)
	if err != nil {
		return model.WorkspaceRegistration{}, model.ProjectFile{}, err
	}
	registration = workspace.ApplyProjectTopology(registration, project)
	return registration, project, nil
}

func workspaceSummaryItem(registration model.WorkspaceRegistration, project model.ProjectFile) map[string]any {
	return map[string]any{
		"name":               registration.Name,
		"root":               registration.Root,
		"languages":          registration.Languages,
		"kind":               registration.Kind,
		"solution":           registration.Solution,
		"repo_count":         len(project.Repos),
		"entrypoint_count":   len(project.Entrypoints),
		"default_repo":       project.Project.DefaultRepo,
		"default_entrypoint": project.Project.DefaultEntrypoint,
	}
}
