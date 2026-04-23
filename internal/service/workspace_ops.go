package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func (a *App) workspaceAdd(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	return a.registerWorkspace(ctx, request, "registry")
}

func (a *App) workspaceInit(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	return a.registerWorkspace(ctx, request, "init")
}

func (a *App) registerWorkspace(ctx context.Context, request model.CommandRequest, backend string) (model.Envelope, error) {
	path, _ := request.Payload["path"].(string)
	alias, _ := request.Payload["alias"].(string)
	if path == "" {
		path = "."
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
	noIndex, _ := request.Payload["no_index"].(bool)
	if !noIndex {
		var indexResult indexer.Result
		indexErr := store.WithWorkspaceIndexLock(registration.Root, "workspace.auto-index", func() error {
			var err error
			indexResult, err = indexer.IndexWorkspace(ctx, registration.Root, false)
			return err
		})
		if indexErr != nil {
			warnings = append(warnings, "auto-index failed: "+indexErr.Error())
		} else {
			item["index_symbols"] = indexResult.Stats.Symbols
			item["index_files"] = indexResult.Stats.Files
			item["index_ms"] = indexResult.Stats.Ms
		}
	}
	if backend == "init" {
		if isAXIMode(request.Context) {
			item["view"] = "preview"
			item["next_steps"] = buildWorkspaceAXINextSteps(alias)
		} else {
			item["next_steps"] = []string{
				"mi-lsp nav governance --workspace " + alias + " --format compact",
				"mi-lsp nav ask \"how is this workspace organized?\" --workspace " + alias + " --format compact",
				"mi-lsp workspace status " + alias + " --format compact",
			}
		}
	}
	return model.Envelope{Ok: true, Workspace: alias, Backend: backend, Items: []map[string]any{item}, Warnings: warnings}, nil
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
		project, loadErr := workspace.LoadProjectSummary(registration.Root, registration)
		if loadErr != nil {
			items = append(items, map[string]any{"name": registration.Name, "root": registration.Root, "kind": registration.Kind, "languages": registration.Languages})
			continue
		}
		registration = workspace.ApplyProjectTopology(registration, project)
		items = append(items, workspaceSummaryItem(registration, project))
	}
	return model.Envelope{Ok: true, Backend: "registry", Items: items}, nil
}

func (a *App) workspaceStatus(ctx context.Context, name string, opts model.QueryOptions) (model.Envelope, error) {
	registration, project, err := a.resolveWorkspaceWithProject(name)
	if err != nil {
		return model.Envelope{}, err
	}
	item := workspaceSummaryItem(registration, project)
	item["repos"] = project.Repos
	item["entrypoints"] = project.Entrypoints
	item["docs_read_model"] = workspaceProfileHint(registration.Root)
	governance := docgraph.InspectGovernance(registration.Root, true)
	item["governance_doc"] = governance.HumanDoc
	item["governance_projection"] = governance.ProjectionDoc
	item["governance_profile"] = governance.Profile
	item["governance_extends"] = governance.Extends
	item["governance_base"] = governance.EffectiveBase
	item["governance_overlays"] = governance.EffectiveOverlays
	item["governance_sync"] = governance.Sync
	item["governance_index_sync"] = governance.IndexSync
	item["governance_blocked"] = governance.Blocked
	item["governance_summary"] = governance.Summary
	memory, _ := loadReentryMemory(ctx, registration.Root)
	db, err := store.Open(registration.Root)
	if err != nil {
		item["index_ready"] = false
		item["docs_ready"] = false
		item["docs_index_ready"] = false
		item["doc_count"] = 0
		warnings := []string{"workspace has no index yet"}
		warnings = append(warnings, governance.Warnings...)
		if governance.Blocked {
			warnings = append(warnings, governance.Issues...)
		}
		if memory != nil && (!isAXIMode(opts) || opts.Full) {
			item["memory"] = buildWorkspaceStatusMemory(memory.Snapshot, memory.Stale)
		}
		envelope := model.Envelope{Ok: true, Workspace: registration.Name, Backend: "sqlite", Items: []any{applyWorkspaceStatusAXIView(item, registration.Name, opts)}, Warnings: warnings}
		envelope = attachMemoryPointer(envelope, memory)
		envelope.Continuation = buildStatusContinuation(opts, memory)
		return applyCoachPolicy(envelope, opts), nil
	}
	defer db.Close()
	stats, err := store.WorkspaceStats(ctx, db)
	if err != nil {
		return model.Envelope{}, err
	}
	docCount, err := store.CountDocRecords(ctx, db)
	if err != nil {
		return model.Envelope{}, err
	}
	docsIndexReady := docCount > 0
	item["index_ready"] = stats.Files > 0 || stats.Symbols > 0
	item["docs_ready"] = docsIndexReady
	item["docs_index_ready"] = docsIndexReady
	item["doc_count"] = docCount
	item["index_files"] = stats.Files
	item["index_symbols"] = stats.Symbols
	warnings := append([]string{}, governance.Warnings...)
	if governance.Blocked {
		warnings = append(warnings, governance.Issues...)
	}
	if w := entrypointLanguageMismatchWarning(ctx, db, project); w != "" {
		warnings = append(warnings, w)
	}
	if memory != nil && (!isAXIMode(opts) || opts.Full) {
		item["memory"] = buildWorkspaceStatusMemory(memory.Snapshot, memory.Stale)
	}
	if memory == nil {
		warnings = append(warnings, fmt.Sprintf("reentry memory snapshot absent; rerun 'mi-lsp index --workspace %s' to rebuild memory and memory_pointer", registration.Name))
	}
	if canonicalWikiExists(registration.Root) && !docsIndexReady {
		warnings = appendStringIfMissing(warnings, fmt.Sprintf("documentation index is empty; rerun 'mi-lsp index --workspace %s --docs-only' to rebuild docgraph and memory_pointer", registration.Name))
	}
	if docsIndexReady && !item["index_ready"].(bool) {
		warnings = appendStringIfMissing(warnings, fmt.Sprintf("code catalog is empty while documentation is ready; docs-only recovery rebuilt governed docs and memory_pointer, but nav.find/nav.symbols/semantic code features still need 'mi-lsp index --workspace %s'", registration.Name))
	}
	envelope := model.Envelope{Ok: true, Workspace: registration.Name, Backend: "sqlite", Items: []any{applyWorkspaceStatusAXIView(item, registration.Name, opts)}, Stats: stats, Warnings: warnings}
	envelope = attachMemoryPointer(envelope, memory)
	envelope.Continuation = buildStatusContinuation(opts, memory)
	return applyCoachPolicy(envelope, opts), nil
}

// entrypointLanguageMismatchWarning returns a warning when the default entrypoint is C#-only
// but TypeScript files significantly outnumber C# files in the index (ratio > 2x).
func entrypointLanguageMismatchWarning(ctx context.Context, db *sql.DB, project model.ProjectFile) string {
	ep := project.Project.DefaultEntrypoint
	if ep == "" {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(ep))
	if ext != ".sln" && ext != ".csproj" {
		return ""
	}
	counts, err := store.FilesCountByLanguage(ctx, db)
	if err != nil || len(counts) == 0 {
		return ""
	}
	tsCount := counts["typescript"]
	csCount := counts["csharp"]
	if tsCount == 0 || csCount*2 >= tsCount {
		return ""
	}
	return fmt.Sprintf(
		"typescript files (%d) outnumber csharp files (%d) 2x: default_entrypoint is C#-only — nav.ask and nav.pack may be biased toward backend docs; consider `mi-lsp workspace add <ts-root> --name <alias>-ts`",
		tsCount, csCount,
	)
}

func applyWorkspaceStatusAXIView(item map[string]any, alias string, opts model.QueryOptions) map[string]any {
	if !isAXIMode(opts) {
		return item
	}
	item["view"] = "full"
	item["next_steps"] = buildWorkspaceAXINextSteps(alias)
	if isAXIPreview(opts) {
		item["view"] = "preview"
		delete(item, "repos")
		delete(item, "entrypoints")
	}
	return item
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

func workspaceProfileHint(root string) string {
	if _, err := os.Stat(filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml")); err == nil {
		return ".docs/wiki/_mi-lsp/read-model.toml"
	}
	return "builtin-default"
}
