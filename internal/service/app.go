package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/worker"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

type SemanticCaller interface {
	Call(context.Context, model.WorkspaceRegistration, model.WorkerRequest) (model.WorkerResponse, error)
	Status() []model.WorkerStatus
}

type App struct {
	RepoRoot string
	Semantic SemanticCaller
	Config   Config
}

type semanticTarget struct {
	Repo       model.WorkspaceRepo
	Entrypoint model.WorkspaceEntrypoint
	Warnings   []string
	Synthetic  bool
}

func New(repoRoot string, semantic SemanticCaller) *App {
	if semantic == nil {
		semantic = worker.EphemeralCaller{RepoRoot: repoRoot}
	}
	return &App{RepoRoot: repoRoot, Semantic: semantic, Config: DefaultConfig()}
}

func (a *App) ResolveWorkspace(nameOrPath string) (model.WorkspaceRegistration, error) {
	return workspace.ResolveWorkspace(nameOrPath)
}

func (a *App) Execute(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	switch request.Operation {
	case "workspace.add":
		return a.workspaceAdd(ctx, request)
	case "workspace.init":
		return a.workspaceInit(ctx, request)
	case "workspace.scan":
		return a.workspaceScan()
	case "workspace.list":
		return a.workspaceList()
	case "workspace.status":
		return a.workspaceStatus(ctx, request.Context.Workspace, request.Context)
	case "workspace.remove":
		return a.workspaceRemove(request)
	case "workspace.warm":
		return model.Envelope{Ok: true, Backend: "daemon", Items: []string{}, Warnings: []string{"daemon is not running; warm is a no-op in direct mode"}}, nil
	case "index.run":
		return a.indexWorkspace(ctx, request)
	case "info":
		return a.info(ctx, request.Context.Workspace)
	case "nav.symbols":
		return a.symbols(ctx, request)
	case "nav.find":
		return a.find(ctx, request)
	case "nav.overview":
		return a.overview(ctx, request)
	case "nav.outline":
		return a.symbols(ctx, request)
	case "nav.search":
		return a.search(ctx, request)
	case "nav.governance":
		return a.governance(ctx, request)
	case "nav.route":
		return a.route(ctx, request)
	case "nav.ask":
		return a.ask(ctx, request)
	case "nav.pack":
		return a.pack(ctx, request)
	case "nav.service":
		return a.serviceSummary(ctx, request)
	case "nav.refs":
		return a.semantic(ctx, request, "find_refs")
	case "nav.context":
		return a.contextQuery(ctx, request)
	case "nav.deps":
		return a.semantic(ctx, request, "get_deps")
	case "nav.multi-read":
		return a.multiRead(ctx, request)
	case "nav.batch":
		return a.batch(ctx, request)
	case "nav.related":
		return a.related(ctx, request)
	case "nav.workspace-map":
		return a.workspaceMap(ctx, request)
	case "nav.diff-context":
		return a.diffContext(ctx, request)
	case "nav.trace":
		return a.trace(ctx, request)
	case "nav.intent":
		return a.intent(ctx, request)
	case "worker.install":
		return a.installWorker(request)
	case "worker.status":
		return a.workerStatus()
	default:
		return model.Envelope{}, fmt.Errorf("unknown operation %q; run mi-lsp --help for available commands", request.Operation)
	}
}

func (a *App) indexWorkspace(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	path, _ := request.Payload["path"].(string)
	if path == "" {
		path = request.Context.Workspace
	}
	registration, err := a.ResolveWorkspace(path)
	if err != nil {
		return model.Envelope{}, err
	}
	clean, _ := request.Payload["clean"].(bool)

	// Try incremental index if clean=false and index.db exists
	var result indexer.Result
	incremental := false
	if !clean {
		result, err = indexer.IncrementalIndex(ctx, registration.Root)
		if err == nil && result.Stats.Files > 0 {
			// Incremental succeeded and found changes
			incremental = true
		} else if err == nil && result.Stats.Files == 0 {
			// No changes detected
			return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "catalog", Items: []model.SymbolRecord{}, Stats: result.Stats, Warnings: []string{"no changes detected"}}, nil
		}
		// If incremental failed, fall through to full index
	}

	// Fall back to full index if incremental didn't succeed
	if !incremental {
		result, err = indexer.IndexWorkspace(ctx, registration.Root, clean)
		if err != nil {
			return model.Envelope{}, err
		}
	}

	// Add incremental flag to warnings if successful
	warnings := result.Warnings
	if incremental {
		warnings = appendStringIfMissing(warnings, "incremental=true")
	}

	return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "catalog", Items: result.Symbols, Stats: result.Stats, Warnings: warnings}, nil
}

func appendStringIfMissing(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func (a *App) info(ctx context.Context, name string) (model.Envelope, error) {
	registration, project, err := a.resolveWorkspaceWithProject(name)
	if err != nil {
		return model.Envelope{}, err
	}
	db, err := store.Open(registration.Root)
	if err != nil {
		return model.Envelope{}, err
	}
	defer db.Close()
	stats, err := store.WorkspaceStats(ctx, db)
	if err != nil {
		return model.Envelope{}, err
	}
	item := workspaceSummaryItem(registration, project)
	item["repos"] = project.Repos
	item["entrypoints"] = project.Entrypoints
	return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "sqlite", Items: []map[string]any{item}, Stats: stats}, nil
}

func (a *App) symbols(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}
	file, _ := request.Payload["file"].(string)
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
	offset := intFromAny(request.Payload["offset"], 0)
	items, err := store.SymbolsByFile(ctx, db, relativeFile, request.Context.MaxItems, offset)
	if err != nil {
		return model.Envelope{}, err
	}
	return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "catalog", Items: items, Stats: model.Stats{Symbols: len(items)}}, nil
}

func (a *App) find(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	allWorkspaces, _ := request.Payload["all_workspaces"].(bool)
	if allWorkspaces {
		if strings.TrimSpace(stringPayload(request.Payload, "repo")) != "" {
			return model.Envelope{}, errors.New("--repo is not supported with --all-workspaces")
		}
		return a.findAllWorkspaces(ctx, request)
	}

	registration, project, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}
	pattern, _ := request.Payload["pattern"].(string)
	kind, _ := request.Payload["kind"].(string)
	exact, _ := request.Payload["exact"].(bool)
	offset := intFromAny(request.Payload["offset"], 0)
	scopedRepo, scopeEnvelope := resolveCatalogRepoScope(registration, project, request.Payload)
	if scopeEnvelope != nil {
		return *scopeEnvelope, nil
	}
	db, err := store.Open(registration.Root)
	if err != nil {
		return model.Envelope{}, err
	}
	defer db.Close()
	queryLimit := request.Context.MaxItems
	sqlOffset := offset
	if scopedRepo != nil {
		queryLimit = max((offset+request.Context.MaxItems)*10, 100)
		sqlOffset = 0
	}
	items, err := store.FindSymbols(ctx, db, pattern, kind, exact, queryLimit, sqlOffset)
	if err != nil {
		return model.Envelope{}, err
	}
	items = filterSymbolsByRepo(items, scopedRepo)
	if offset > 0 {
		if offset >= len(items) {
			items = []model.SymbolRecord{}
		} else {
			items = items[offset:]
		}
	}
	if request.Context.MaxItems > 0 && len(items) > request.Context.MaxItems {
		items = items[:request.Context.MaxItems]
	}
	return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "catalog", Items: items, Stats: model.Stats{Symbols: len(items)}}, nil
}

func (a *App) overview(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}
	dir, _ := request.Payload["dir"].(string)
	prefix := ""
	if dir != "" {
		prefix, err = makeRelative(registration.Root, dir)
		if err != nil {
			return model.Envelope{}, err
		}
		if prefix == "." {
			prefix = ""
		}
	}
	db, err := store.Open(registration.Root)
	if err != nil {
		return model.Envelope{}, err
	}
	defer db.Close()
	offset := intFromAny(request.Payload["offset"], 0)
	items, err := store.OverviewByPrefix(ctx, db, prefix, request.Context.MaxItems, offset)
	if err != nil {
		return model.Envelope{}, err
	}
	return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "catalog", Items: items, Stats: model.Stats{Symbols: len(items)}}, nil
}

func (a *App) search(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	allWorkspaces, _ := request.Payload["all_workspaces"].(bool)
	if allWorkspaces {
		if strings.TrimSpace(stringPayload(request.Payload, "repo")) != "" {
			return model.Envelope{}, errors.New("--repo is not supported with --all-workspaces")
		}
		return a.searchAllWorkspaces(ctx, request)
	}

	registration, project, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}
	pattern, _ := request.Payload["pattern"].(string)
	useRegex, _ := request.Payload["regex"].(bool)
	if pattern == "" {
		return model.Envelope{}, errors.New("pattern is required")
	}
	scopedRepo, scopeEnvelope := resolveCatalogRepoScope(registration, project, request.Payload)
	if scopeEnvelope != nil {
		return *scopeEnvelope, nil
	}
	searchRoot := registration.Root
	if scopedRepo != nil {
		searchRoot = filepath.Join(registration.Root, filepath.FromSlash(scopedRepo.Root))
	}
	items, err := searchPatternScoped(ctx, registration.Root, searchRoot, project, pattern, useRegex, request.Context.MaxItems)
	if err != nil {
		return model.Envelope{}, err
	}
	warnings := []string{}
	if len(items) == 0 && !useRegex && looksRegexLikePattern(pattern) {
		warnings = append(warnings, "no literal matches; pattern looks regex-like, rerun with --regex")
	}

	includeContent, _ := request.Payload["include_content"].(bool)
	if includeContent && len(items) > 0 {
		contextLines := intFromAny(request.Payload["context_lines"], 20)
		contextMode, _ := request.Payload["context_mode"].(string)
		if contextMode == "" {
			contextMode = "hybrid"
		}
		enrichWarnings := enrichSearchResultsWithContent(ctx, registration, items, contextLines, contextMode)
		warnings = append(warnings, enrichWarnings...)
	}

	hint := ""
	var nextHint *string
	if len(items) == 0 {
		if ctx.Err() != nil {
			hint = fmt.Sprintf("0 matches for %q: search timed out (context cancelled)", pattern)
		} else if !useRegex && looksRegexLikePattern(pattern) {
			hint = fmt.Sprintf("0 matches for %q: pattern looks regex-like, rerun with --regex", pattern)
			rerun := "rerun with --regex"
			nextHint = &rerun
		} else {
			hint = fmt.Sprintf("0 matches for %q in workspace %s", pattern, registration.Name)
		}
	}

	env := model.Envelope{Ok: true, Workspace: registration.Name, Backend: "text", Items: items, Warnings: warnings, Hint: hint, NextHint: nextHint, Stats: model.Stats{Files: len(items)}}
	if isAXIPreview(request.Context) && env.NextHint == nil {
		env = applyAXIPreviewHints(env, request.Context, axiPreviewSummaryHint)
	}
	return env, nil
}

func (a *App) installWorker(request model.CommandRequest) (model.Envelope, error) {
	rid, _ := request.Payload["rid"].(string)
	path, err := worker.InstallWorker(a.RepoRoot, rid)
	if err != nil {
		return model.Envelope{}, err
	}
	if rid == "" {
		rid = worker.ResolveRID()
	}
	items := []map[string]any{{"path": path, "rid": rid}}
	return model.Envelope{Ok: true, Backend: "worker-install", Items: items}, nil
}

func (a *App) workerStatus() (model.Envelope, error) {
	info := worker.InspectWorkerRuntime(a.RepoRoot, worker.ResolveRID())
	cliPath, err := os.Executable()
	if err != nil {
		cliPath = ""
	}
	items := []map[string]any{{
		"dotnet":               info.Dotnet,
		"rid":                  info.RID,
		"tool_root":            info.ToolRoot,
		"tool_root_kind":       info.ToolRootKind,
		"cli_path":             cliPath,
		"protocol_version":     model.ProtocolVersion,
		"install_hint":         info.InstallHint,
		"active_workers":       a.Semantic.Status(),
		"selected":             info.Selected,
		"selected_source":      info.Selected.Source,
		"selected_path":        info.Selected.Path,
		"selected_compatible":  info.Selected.Compatible,
		"selected_error":       info.Selected.Error,
		"bundled":              info.Bundled.Path,
		"bundled_error":        info.Bundled.Error,
		"bundled_compatible":   info.Bundled.Compatible,
		"installed":            info.Installed.Path,
		"installed_error":      info.Installed.Error,
		"installed_compatible": info.Installed.Compatible,
		"dev_local":            info.DevLocal.Path,
		"dev_local_error":      info.DevLocal.Error,
	}}
	return model.Envelope{Ok: true, Backend: "worker", Items: items}, nil
}

func resolveCatalogRepoScope(registration model.WorkspaceRegistration, project model.ProjectFile, payload map[string]any) (*model.WorkspaceRepo, *model.Envelope) {
	repoSelector := strings.TrimSpace(stringPayload(payload, "repo"))
	if repoSelector == "" {
		return nil, nil
	}
	repo, ok := workspace.FindRepo(project, repoSelector)
	if !ok {
		return nil, ambiguityEnvelope(registration, fmt.Sprintf("unknown repo selector %q", repoSelector), repoCandidates(project.Repos), "--repo <name>")
	}
	return &repo, nil
}

func filterSymbolsByRepo(items []model.SymbolRecord, repo *model.WorkspaceRepo) []model.SymbolRecord {
	if repo == nil {
		return items
	}
	filtered := make([]model.SymbolRecord, 0, len(items))
	for _, item := range items {
		if strings.EqualFold(item.RepoName, repo.Name) || strings.EqualFold(item.RepoID, repo.ID) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func stringPayload(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func clonePayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(payload))
	for key, value := range payload {
		cloned[key] = value
	}
	return cloned
}

func makeRelative(root, file string) (string, error) {
	absoluteFile := file
	if !filepath.IsAbs(file) {
		absoluteFile = filepath.Join(root, file)
	}
	relative, err := filepath.Rel(root, absoluteFile)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(relative), nil
}

func intFromAny(value any, defaultValue int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return defaultValue
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (a *App) searchAllWorkspaces(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	workspaces, err := workspace.ListWorkspaces()
	if err != nil {
		return model.Envelope{}, fmt.Errorf("failed to list workspaces: %w", err)
	}
	if len(workspaces) == 0 {
		return model.Envelope{Ok: true, Backend: "text", Items: []map[string]any{}, Warnings: []string{"no workspaces registered"}}, nil
	}

	pattern, _ := request.Payload["pattern"].(string)
	useRegex, _ := request.Payload["regex"].(bool)
	if pattern == "" {
		return model.Envelope{}, errors.New("pattern is required")
	}

	maxItems := request.Context.MaxItems
	if maxItems <= 0 {
		maxItems = DefaultConfig().DefaultSearchLimit
	}

	type searchResult struct {
		ws       model.WorkspaceRegistration
		items    []map[string]any
		warnings []string
		err      error
	}

	results := make(chan searchResult, len(workspaces))
	var wg sync.WaitGroup
	const maxConcurrent = 4

	semaphore := make(chan struct{}, maxConcurrent)

	for _, ws := range workspaces {
		wg.Add(1)
		go func(wsReg model.WorkspaceRegistration) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			project, _ := workspace.LoadProjectFile(wsReg.Root)

			items, err := searchPattern(ctx, wsReg.Root, project, pattern, useRegex, maxItems)
			if err != nil {
				results <- searchResult{ws: wsReg, err: err}
				return
			}

			for _, item := range items {
				item["workspace"] = wsReg.Name
			}

			warnings := []string{}
			if len(items) == 0 && !useRegex && looksRegexLikePattern(pattern) {
				warnings = append(warnings, fmt.Sprintf("%s: no literal matches; pattern looks regex-like, rerun with --regex", wsReg.Name))
			}

			results <- searchResult{ws: wsReg, items: items, warnings: warnings}
		}(ws)
	}

	wg.Wait()
	close(results)

	var allItems []map[string]any
	var allWarnings []string

	for result := range results {
		if result.err != nil {
			allWarnings = append(allWarnings, fmt.Sprintf("%s: search failed: %v", result.ws.Name, result.err))
			continue
		}
		allItems = append(allItems, result.items...)
		allWarnings = append(allWarnings, result.warnings...)
	}

	if len(allItems) > maxItems {
		allItems = allItems[:maxItems]
	}

	includeContent, _ := request.Payload["include_content"].(bool)
	if includeContent && len(allItems) > 0 {
		contextLines := intFromAny(request.Payload["context_lines"], 20)
		contextMode, _ := request.Payload["context_mode"].(string)
		if contextMode == "" {
			contextMode = "hybrid"
		}

		for _, item := range allItems {
			wsName, _ := item["workspace"].(string)
			if wsName != "" {
				wsReg, err := workspace.ResolveWorkspace(wsName)
				if err == nil {
					enrichSingleSearchResult(ctx, wsReg.Root, nil, item, contextLines, contextMode)
				}
			}
		}
	}

	var nextHint *string
	if len(allItems) == 0 && !useRegex && looksRegexLikePattern(pattern) {
		rerun := "rerun with --regex"
		nextHint = &rerun
	}

	env := model.Envelope{Ok: true, Backend: "text", Items: allItems, Warnings: allWarnings, NextHint: nextHint, Stats: model.Stats{Files: len(allItems)}}
	if isAXIPreview(request.Context) && env.NextHint == nil {
		env = applyAXIPreviewHints(env, request.Context, axiPreviewSummaryHint)
	}
	return env, nil
}

func (a *App) findAllWorkspaces(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	workspaces, err := workspace.ListWorkspaces()
	if err != nil {
		return model.Envelope{}, fmt.Errorf("failed to list workspaces: %w", err)
	}
	if len(workspaces) == 0 {
		return model.Envelope{Ok: true, Backend: "catalog", Items: []model.SymbolRecord{}, Warnings: []string{"no workspaces registered"}}, nil
	}

	pattern, _ := request.Payload["pattern"].(string)
	kind, _ := request.Payload["kind"].(string)
	exact, _ := request.Payload["exact"].(bool)

	maxItems := request.Context.MaxItems
	if maxItems <= 0 {
		maxItems = DefaultConfig().DefaultSearchLimit
	}

	type findResult struct {
		ws    model.WorkspaceRegistration
		items []model.SymbolRecord
		err   error
	}

	results := make(chan findResult, len(workspaces))
	var wg sync.WaitGroup
	const maxConcurrent = 4

	semaphore := make(chan struct{}, maxConcurrent)

	for _, ws := range workspaces {
		wg.Add(1)
		go func(wsReg model.WorkspaceRegistration) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			db, err := store.Open(wsReg.Root)
			if err != nil {
				results <- findResult{ws: wsReg, err: err}
				return
			}
			defer db.Close()

			items, err := store.FindSymbols(ctx, db, pattern, kind, exact, maxItems, 0)
			if err != nil {
				results <- findResult{ws: wsReg, err: err}
				return
			}

			for i := range items {
				items[i].Workspace = wsReg.Name
			}

			results <- findResult{ws: wsReg, items: items}
		}(ws)
	}

	wg.Wait()
	close(results)

	var allItems []model.SymbolRecord

	for result := range results {
		if result.err != nil {
			continue
		}
		allItems = append(allItems, result.items...)
	}

	if len(allItems) > maxItems {
		allItems = allItems[:maxItems]
	}

	return model.Envelope{Ok: true, Backend: "catalog", Items: allItems, Stats: model.Stats{Symbols: len(allItems)}}, nil
}
