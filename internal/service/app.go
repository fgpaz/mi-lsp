package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
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
	RepoRoot        string
	Semantic        SemanticCaller
	Config          Config
	backendCooldown sync.Map
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
	normalizedRequest, resolutionWarnings, err := a.normalizeWorkspaceRequest(request)
	if err != nil {
		return model.Envelope{}, err
	}
	request = normalizedRequest

	var envelope model.Envelope
	switch request.Operation {
	case "workspace.add":
		envelope, err = a.workspaceAdd(ctx, request)
	case "workspace.init":
		envelope, err = a.workspaceInit(ctx, request)
	case "workspace.scan":
		envelope, err = a.workspaceScan()
	case "workspace.list":
		envelope, err = a.workspaceList()
	case "workspace.status":
		envelope, err = a.workspaceStatus(ctx, request.Context.Workspace, request.Context)
	case "workspace.remove":
		envelope, err = a.workspaceRemove(request)
	case "workspace.warm":
		envelope = model.Envelope{Ok: true, Backend: "daemon", Items: []string{}, Warnings: []string{"daemon is not running; warm is a no-op in direct mode"}}
	case "index.run":
		envelope, err = a.indexWorkspace(ctx, request)
	case "info":
		envelope, err = a.info(ctx, request.Context.Workspace)
	case "nav.symbols":
		envelope, err = a.symbols(ctx, request)
	case "nav.find":
		envelope, err = a.find(ctx, request)
	case "nav.overview":
		envelope, err = a.overview(ctx, request)
	case "nav.outline":
		envelope, err = a.symbols(ctx, request)
	case "nav.search":
		envelope, err = a.search(ctx, request)
	case "nav.wiki.search":
		envelope, err = a.wikiSearch(ctx, request)
	case "nav.governance":
		envelope, err = a.governance(ctx, request)
	case "nav.route":
		envelope, err = a.route(ctx, request)
	case "nav.wiki.route":
		envelope, err = a.route(ctx, request)
	case "nav.ask":
		envelope, err = a.ask(ctx, request)
	case "nav.pack":
		envelope, err = a.pack(ctx, request)
	case "nav.wiki.pack":
		envelope, err = a.pack(ctx, request)
	case "nav.service":
		envelope, err = a.serviceSummary(ctx, request)
	case "nav.refs":
		envelope, err = a.semantic(ctx, request, "find_refs")
	case "nav.context":
		envelope, err = a.contextQuery(ctx, request)
	case "nav.deps":
		envelope, err = a.semantic(ctx, request, "get_deps")
	case "nav.multi-read":
		envelope, err = a.multiRead(ctx, request)
	case "nav.batch":
		envelope, err = a.batch(ctx, request)
	case "nav.related":
		envelope, err = a.related(ctx, request)
	case "nav.workspace-map":
		envelope, err = a.workspaceMap(ctx, request)
	case "nav.diff-context":
		envelope, err = a.diffContext(ctx, request)
	case "nav.trace":
		envelope, err = a.trace(ctx, request)
	case "nav.wiki.trace":
		envelope, err = a.trace(ctx, request)
	case "nav.intent":
		envelope, err = a.intent(ctx, request)
	case "worker.install":
		envelope, err = a.installWorker(request)
	case "worker.status":
		envelope, err = a.workerStatus()
	default:
		err = fmt.Errorf("unknown operation %q; run mi-lsp --help for available commands", request.Operation)
	}
	if err != nil {
		return model.Envelope{}, err
	}
	for _, warning := range resolutionWarnings {
		envelope.Warnings = appendStringIfMissing(envelope.Warnings, warning)
	}
	return envelope, nil
}

func (a *App) normalizeWorkspaceRequest(request model.CommandRequest) (model.CommandRequest, []string, error) {
	if !operationRequiresWorkspaceResolution(request) {
		return request, nil, nil
	}
	if strings.TrimSpace(request.Context.Workspace) != "" {
		return request, nil, nil
	}
	resolution, err := workspace.ResolveWorkspaceSelection(request.Context.Workspace, request.Context.CallerCWD)
	if err != nil {
		return request, nil, err
	}
	request.Context.Workspace = resolution.Registration.Name
	return request, resolution.Warnings, nil
}

func operationRequiresWorkspaceResolution(request model.CommandRequest) bool {
	switch request.Operation {
	case "workspace.add", "workspace.init", "workspace.scan", "workspace.list", "workspace.remove", "workspace.warm", "worker.install", "worker.status":
		return false
	case "nav.find", "nav.search":
		allWorkspaces, _ := request.Payload["all_workspaces"].(bool)
		return !allWorkspaces
	case "index.run":
		return strings.TrimSpace(stringPayload(request.Payload, "path")) == ""
	case "workspace.status", "info", "nav.symbols", "nav.overview", "nav.outline", "nav.governance", "nav.route", "nav.wiki.route", "nav.ask", "nav.pack", "nav.wiki.pack", "nav.wiki.search", "nav.service", "nav.refs", "nav.context", "nav.deps", "nav.multi-read", "nav.batch", "nav.related", "nav.workspace-map", "nav.diff-context", "nav.trace", "nav.wiki.trace", "nav.intent":
		return true
	default:
		return false
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
			if store.IsCorruptionError(err) {
				backupPath, backupErr := store.QuarantineCorruptDB(registration.Root)
				if backupErr != nil {
					return model.Envelope{}, fmt.Errorf("%w; corrupt db quarantine failed: %v", err, backupErr)
				}
				result, err = indexer.IndexWorkspace(ctx, registration.Root, true)
				if err != nil {
					return model.Envelope{}, fmt.Errorf("%w; rebuild after quarantining %s also failed: %v", err, backupPath, err)
				}
				result.Warnings = appendStringIfMissing(result.Warnings, "corrupt index database was quarantined to "+backupPath)
				result.Warnings = appendStringIfMissing(result.Warnings, "full rebuild completed after corruption recovery")
			} else {
				return model.Envelope{}, err
			}
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
	scopedRepo, scopeWarnings, scopeEnvelope := resolveCatalogRepoScope(registration, project, request.Payload)
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
	return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "catalog", Items: items, Stats: model.Stats{Symbols: len(items)}, Warnings: scopeWarnings}, nil
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
	memory, _ := loadReentryMemory(ctx, registration.Root)
	pattern, _ := request.Payload["pattern"].(string)
	useRegex, _ := request.Payload["regex"].(bool)
	includeContent, _ := request.Payload["include_content"].(bool)
	if pattern == "" {
		return model.Envelope{}, errors.New("pattern is required")
	}
	scopedRepo, scopeWarnings, scopeEnvelope := resolveCatalogRepoScope(registration, project, request.Payload)
	if scopeEnvelope != nil {
		envelope := *scopeEnvelope
		envelope.Coach = buildSearchScopeCoach(registration.Name, pattern, includeContent, useRegex, envelope)
		envelope = attachMemoryPointer(envelope, memory)
		envelope.Continuation = buildSearchContinuation(pattern, project, stringPayload(request.Payload, "repo"), nil, memory)
		return applyCoachPolicy(envelope, request.Context), nil
	}
	searchRoot := registration.Root
	if scopedRepo != nil {
		searchRoot = filepath.Join(registration.Root, filepath.FromSlash(scopedRepo.Root))
	}
	items, err := searchPatternScoped(ctx, registration.Root, searchRoot, project, pattern, useRegex, request.Context.MaxItems)
	warnings := []string{}
	regexAutoHealed := false
	if err != nil {
		if useRegex && isRegexParseError(err) {
			items, err = searchPatternScoped(ctx, registration.Root, searchRoot, project, pattern, false, request.Context.MaxItems)
			if err != nil {
				return model.Envelope{}, err
			}
			warnings = append(warnings, "invalid regex detected; retried automatically as literal search")
			useRegex = false
			regexAutoHealed = true
		} else {
			return model.Envelope{}, err
		}
	}
	if len(items) == 0 && !useRegex && looksRegexLikePattern(pattern) {
		warnings = append(warnings, "no literal matches; pattern looks regex-like, rerun with --regex")
	}
	warnings = append(warnings, scopeWarnings...)

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
	env.Coach = buildSearchCoach(registration.Name, project, pattern, includeContent, stringPayload(request.Payload, "repo"), useRegex, regexAutoHealed, items, request.Context)
	env = attachMemoryPointer(env, memory)
	env.Continuation = buildSearchContinuation(pattern, project, stringPayload(request.Payload, "repo"), items, memory)
	if isAXIPreview(request.Context) && env.NextHint == nil {
		env = applyAXIPreviewHints(env, request.Context, axiPreviewSummaryHint)
	}
	return applyCoachPolicy(env, request.Context), nil
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

func resolveCatalogRepoScope(registration model.WorkspaceRegistration, project model.ProjectFile, payload map[string]any) (*model.WorkspaceRepo, []string, *model.Envelope) {
	repoSelector := strings.TrimSpace(stringPayload(payload, "repo"))
	if repoSelector == "" {
		return nil, nil, nil
	}
	resolution := resolveRepoSelector(project, repoSelector)
	if resolution.Envelope != nil {
		envelope := *resolution.Envelope
		envelope.Workspace = registration.Name
		return nil, nil, &envelope
	}
	return &resolution.Repo, append([]string{}, resolution.Warnings...), nil
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

type repoSelectorResolution struct {
	Repo     model.WorkspaceRepo
	Warnings []string
	Envelope *model.Envelope
}

func resolveRepoSelector(project model.ProjectFile, selector string) repoSelectorResolution {
	if repo, ok := workspace.FindRepo(project, selector); ok {
		return repoSelectorResolution{Repo: repo}
	}
	candidates := rankRepoCandidates(project, selector)
	if len(candidates) == 1 && candidates[0].Score >= 100 {
		return repoSelectorResolution{
			Repo:     candidates[0].Repo,
			Warnings: []string{fmt.Sprintf("repo selector %q resolved automatically to %q", selector, candidates[0].Repo.Name)},
		}
	}
	if len(candidates) > 0 {
		items := make([]map[string]any, 0, len(candidates))
		labels := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			items = append(items, map[string]any{
				"repo":               candidate.Repo.Name,
				"repo_id":            candidate.Repo.ID,
				"root":               candidate.Repo.Root,
				"default_entrypoint": candidate.Repo.DefaultEntrypoint,
				"match_reason":       candidate.Reason,
			})
			labels = append(labels, candidate.Repo.Name)
		}
		warning := fmt.Sprintf("unknown repo selector %q; closest matches: %s", selector, strings.Join(labels, ", "))
		next := "rerun with --repo " + candidates[0].Repo.Name
		return repoSelectorResolution{
			Envelope: &model.Envelope{
				Ok:       false,
				Backend:  "router",
				Items:    items,
				Warnings: []string{warning},
				NextHint: &next,
			},
		}
	}
	return repoSelectorResolution{
		Envelope: ambiguityEnvelope(projectRegistrationHint(project), fmt.Sprintf("unknown repo selector %q", selector), repoCandidates(project.Repos), "--repo <name>"),
	}
}

type repoCandidate struct {
	Repo   model.WorkspaceRepo
	Reason string
	Score  int
}

func rankRepoCandidates(project model.ProjectFile, selector string) []repoCandidate {
	needle := normalizeRepoSelector(selector)
	if needle == "" {
		return nil
	}
	candidates := make([]repoCandidate, 0, len(project.Repos))
	for _, repo := range project.Repos {
		score, reason := scoreRepoCandidate(repo, needle)
		if score <= 0 {
			continue
		}
		candidates = append(candidates, repoCandidate{Repo: repo, Reason: reason, Score: score})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return strings.ToLower(candidates[i].Repo.Name) < strings.ToLower(candidates[j].Repo.Name)
		}
		return candidates[i].Score > candidates[j].Score
	})
	if len(candidates) > 3 {
		candidates = candidates[:3]
	}
	return candidates
}

func scoreRepoCandidate(repo model.WorkspaceRepo, needle string) (int, string) {
	fields := []struct {
		Value  string
		Reason string
		Base   int
	}{
		{Value: repo.Name, Reason: "name", Base: 130},
		{Value: repo.ID, Reason: "id", Base: 120},
		{Value: repo.Root, Reason: "root", Base: 100},
		{Value: filepath.Base(filepath.Clean(repo.Root)), Reason: "root_basename", Base: 95},
	}
	bestScore := 0
	bestReason := ""
	for _, field := range fields {
		value := normalizeRepoSelector(field.Value)
		if value == "" {
			continue
		}
		switch {
		case value == needle:
			return field.Base + 40, field.Reason + "_exact"
		case strings.HasPrefix(value, needle):
			score := field.Base + 20 - (len(value) - len(needle))
			if score > bestScore {
				bestScore = score
				bestReason = field.Reason + "_prefix"
			}
		case strings.Contains(value, needle) && len(needle) >= 3:
			score := field.Base + 5
			if score > bestScore {
				bestScore = score
				bestReason = field.Reason + "_contains"
			}
		default:
			distance := levenshteinDistance(needle, value)
			if distance <= 2 {
				score := field.Base - (distance * 10)
				if score > bestScore {
					bestScore = score
					bestReason = field.Reason + "_fuzzy"
				}
			}
		}
	}
	return bestScore, bestReason
}

func normalizeRepoSelector(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("\\", "", "/", "", "-", "", "_", "", ".", "", " ", "")
	return replacer.Replace(trimmed)
}

func levenshteinDistance(left string, right string) int {
	if left == right {
		return 0
	}
	if left == "" {
		return len(right)
	}
	if right == "" {
		return len(left)
	}
	prev := make([]int, len(right)+1)
	for j := 0; j <= len(right); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(left); i++ {
		current := make([]int, len(right)+1)
		current[0] = i
		for j := 1; j <= len(right); j++ {
			cost := 0
			if left[i-1] != right[j-1] {
				cost = 1
			}
			current[j] = minInt3(
				current[j-1]+1,
				prev[j]+1,
				prev[j-1]+cost,
			)
		}
		prev = current
	}
	return prev[len(right)]
}

func minInt3(a int, b int, c int) int {
	return int(math.Min(float64(a), math.Min(float64(b), float64(c))))
}

func projectRegistrationHint(project model.ProjectFile) model.WorkspaceRegistration {
	return model.WorkspaceRegistration{Name: project.Project.Name}
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
