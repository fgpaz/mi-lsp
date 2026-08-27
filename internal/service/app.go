package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/livecontext"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/reentry"
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
	started := time.Now()
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
		envelope, err = a.workspaceList(request)
	case "workspace.doctor":
		envelope, err = a.workspaceDoctor(ctx)
	case "workspace.hygiene":
		envelope, err = a.workspaceHygiene(request)
	case "workspace.prune":
		envelope, err = a.workspacePrune(request)
	case "registry.gc":
		envelope, err = a.registryGC(request)
	case "workspace.status":
		envelope, err = a.workspaceStatus(ctx, request)
	case "workspace.remove":
		envelope, err = a.workspaceRemove(request)
	case "workspace.link":
		envelope, err = a.workspaceLink(request)
	case "workspace.warm":
		envelope = model.Envelope{Ok: true, Backend: "daemon", Items: []string{}, Warnings: []string{"daemon is not running; warm is a no-op in direct mode"}}
	case "index.start":
		envelope, err = a.indexStart(ctx, request)
	case "index.status":
		envelope, err = a.indexStatus(ctx, request)
	case "index.cancel":
		envelope, err = a.indexCancel(ctx, request)
	case "index.run-job":
		envelope, err = a.indexRunJob(ctx, request)
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
	case "nav.wiki.validate-harness":
		envelope, err = a.validateHarness(ctx, request)
	case "nav.wiki.validate-source":
		envelope, err = a.validateSource(ctx, request)
	case "nav.wiki.inventory":
		envelope, err = a.wikiInventory(ctx, request)
	case "nav.wiki.map":
		envelope, err = a.wikiMap(ctx, request)
	case "nav.wiki-root":
		envelope, err = a.wikiRoot(ctx, request)
	case "nav.evidence.inventory":
		envelope, err = a.evidenceInventory(ctx, request)
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
	case "nav.neighbors", "nav.callers", "nav.callees", "nav.path", "nav.explain", "nav.graph.stats", "nav.graph.status", "nav.graph.rank", "nav.graph.validate":
		envelope, err = a.graphQuery(ctx, request)
	case "nav.graph-impact":
		envelope, err = a.graphImpact(ctx, request)
	case "nav.workspace-map":
		envelope, err = a.workspaceMap(ctx, request)
	case "nav.diff-context":
		envelope, err = a.diffContext(ctx, request)
	case "nav.affected":
		envelope, err = a.affected(ctx, request)
	case "nav.flow-slice":
		envelope, err = a.flowSlice(ctx, request)
	case "nav.change-pack":
		envelope, err = a.changePack(ctx, request)
	case "nav.edit-plan":
		envelope, err = a.editPlan(ctx, request)
	case "nav.prepare":
		envelope, err = a.prepare(ctx, request)
	case "prepare.create", "prepare.verify", "prepare.refresh":
		envelope, err = a.preparationPacket(ctx, request, strings.TrimPrefix(request.Operation, "prepare."))
	case "nav.trace":
		envelope, err = a.trace(ctx, request)
	case "nav.wiki.trace":
		envelope, err = a.trace(ctx, request)
	case "nav.intent":
		envelope, err = a.intent(ctx, request)
	case "nav.recall":
		envelope, err = a.recall(ctx, request)
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
	if liveWikiCodeOperation(request.Operation) {
		envelope = a.enrichLiveWikiCodeContext(ctx, request, envelope)
	} else {
		envelope = a.enrichWikiCodeContext(ctx, request, envelope)
	}
	for _, warning := range resolutionWarnings {
		envelope.Warnings = appendStringIfMissing(envelope.Warnings, warning)
	}
	serviceMS := time.Since(started).Milliseconds()
	if serviceMS < 1 {
		serviceMS = 1
	}
	if envelope.Stats.Ms < serviceMS {
		envelope.Stats.Ms = serviceMS
	}
	return envelope, nil
}

var liveWikiCodeOperations = map[string]bool{
	"nav.trace":       true,
	"nav.wiki.trace":  true,
	"nav.pack":        true,
	"nav.wiki.pack":   true,
	"nav.related":     true,
	"nav.neighbors":   true,
	"nav.prepare":     true,
	"nav.change-pack": true,
}

func liveWikiCodeOperation(operation string) bool {
	return liveWikiCodeOperations[strings.TrimSpace(operation)]
}

// buildLiveWikiCodeContext is the one command-facing bridge pipeline. The
// scope is derived by the adapter, then one request-scoped overlay is built
// before the shared resolver is called. Neither step writes to the index.
func (a *App) buildLiveWikiCodeContext(ctx context.Context, op string, scope livecontext.WikiCodeScope, opts model.QueryOptions) (model.WikiCodeContext, error) {
	request := model.CommandRequest{Operation: op, Context: opts}
	return a.buildLiveWikiCodeContextForRequest(ctx, request, scope, nil)
}

func (a *App) buildLiveWikiCodeContextForRequest(ctx context.Context, request model.CommandRequest, scope livecontext.WikiCodeScope, existingGraphFreshness *model.GraphFreshness) (model.WikiCodeContext, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	workspaceSelector := strings.TrimSpace(request.Context.Workspace)
	if workspaceSelector == "" && a != nil {
		workspaceSelector = strings.TrimSpace(a.RepoRoot)
	}
	if workspaceSelector == "" {
		return model.WikiCodeContext{}, model.NewWikiCodeContextError("GPH_WIKI_BACKEND_UNAVAILABLE", "workspace is unavailable")
	}
	registration, err := a.ResolveWorkspace(workspaceSelector)
	if err != nil {
		return model.WikiCodeContext{}, model.NewWikiCodeContextError("GPH_WIKI_BACKEND_UNAVAILABLE", "workspace is unavailable")
	}
	if strings.TrimSpace(registration.Root) == "" {
		return model.WikiCodeContext{}, model.NewWikiCodeContextError("GPH_WIKI_BACKEND_UNAVAILABLE", "workspace root is unavailable")
	}
	var db *sql.DB
	if opened, openErr := openLiveWikiCodeDB(registration); openErr == nil {
		db = opened
	}
	if db != nil {
		defer db.Close()
	}

	maxDocuments := request.Context.MaxItems
	if maxDocuments < 1 {
		maxDocuments = 0
	}
	if scope.Kind == model.ScopeExactWiki && maxDocuments > livecontext.MaxExactDocuments {
		maxDocuments = livecontext.MaxExactDocuments
	}
	if scope.Kind == model.ScopeReverseCode && maxDocuments > livecontext.MaxReverseDocuments {
		maxDocuments = livecontext.MaxReverseDocuments
	}
	overlay, err := livecontext.BuildOverlay(ctx, livecontext.OverlayRequest{
		WorkspaceRoot: registration.Root,
		DB:            db,
		Scope:         scope,
		MaxDocuments:  maxDocuments,
	})
	if err != nil {
		return model.WikiCodeContext{}, err
	}
	// BuildOverlay is also the scope normalizer; the resolver must consume that
	// exact normalized scope so unsafe or non-canonical input cannot re-enter the
	// bridge through a second selector spelling.
	scope = overlay.Scope

	graphFreshness := model.GraphFreshness{State: model.GraphFreshnessUnknown}
	if existingGraphFreshness != nil && existingGraphFreshness.State != "" {
		graphFreshness = *existingGraphFreshness
	} else if db != nil {
		requestedGeneration := stringPayload(request.Payload, "generation")
		if freshness, freshnessErr := store.GraphFreshness(ctx, db, requestedGeneration); freshnessErr == nil {
			graphFreshness = freshness
		}
		// GraphFreshness intentionally reports unknown when no active generation
		// exists. Preserve an explicit stale runtime marker so direct bindings are
		// still returned with a typed graph omission.
		if graphFreshness.State == model.GraphFreshnessUnknown {
			if runtimeState, runtimeErr := store.GraphRuntimeState(ctx, db); runtimeErr == nil && runtimeState == store.GraphRuntimeStale {
				graphFreshness.State = model.GraphFreshnessStale
				graphFreshness.ReasonCode = "runtime_marked_stale"
			}
		}
	}

	var graphDB *sql.DB
	var graphSnapshot *store.GraphQuerySnapshot
	if graphFreshness.State == model.GraphFreshnessCurrent {
		if opened, openErr := openLiveWikiCodeDB(registration); openErr == nil {
			graphDB = opened
			graphSnapshot, err = store.BeginGraphQuerySnapshot(ctx, graphDB, graphFreshness.GenerationID)
			if err == nil {
				defer graphSnapshot.Close()
				defer graphDB.Close()
			} else {
				_ = graphDB.Close()
				graphDB = nil
				graphFreshness.State = model.GraphFreshnessUnknown
				graphFreshness.ReasonCode = "graph_snapshot_unavailable"
			}
		}
	}

	resolveRequest := WikiCodeResolveRequest{
		Direction:        liveWikiCodeDirection(scope),
		WorkspaceRoot:    registration.Root,
		DB:               db,
		DocSelectors:     appendLiveWikiSelectors(scope),
		DocIDs:           append([]string(nil), scope.DocIDs...),
		DocPaths:         append([]string(nil), scope.DocPaths...),
		TargetPath:       scope.TargetPath,
		TargetSymbol:     scope.TargetSymbol,
		Limit:            request.Context.MaxItems,
		TokenBudget:      request.Context.TokenBudget,
		Cursor:           stringPayload(request.Payload, "cursor"),
		GraphSnapshot:    graphSnapshot,
		GraphFreshness:   graphFreshness,
		GraphFreshnessPtr: nil,
	}
	result, err := ResolveWikiCodeContext(ctx, registration.Root, db, resolveRequest, overlay)
	if err != nil {
		return model.WikiCodeContext{}, err
	}
	ensureLivePrimaryDocument(&result, scope, overlay)
	appendLiveGovernanceAuthority(ctx, db, &result)
	// CodeEvidence is the original additive compatibility lane. The shared
	// resolver's typed lanes remain authoritative, but existing consumers still
	// expect direct/tests evidence in this field.
	if len(result.CodeEvidence) == 0 {
		result.CodeEvidence = make([]model.WikiCodeEvidence, 0, len(result.DirectCode)+len(result.Tests)+len(result.SupportingCode))
		result.CodeEvidence = append(result.CodeEvidence, result.DirectCode...)
		result.CodeEvidence = append(result.CodeEvidence, result.Tests...)
		result.CodeEvidence = append(result.CodeEvidence, result.SupportingCode...)
	}
	if liveOverlayWasTruncated(overlay) {
		result.Truncated = true
		if result.NextCursor == "" {
			result.NextCursor = liveWikiCodeDirection(scope) + ":overlay"
		}
		if result.Continuation == nil {
			result.Continuation = &model.WikiCodeContextContinuation{Direction: liveWikiCodeDirection(scope), Cursor: result.NextCursor}
		}
	}
	sanitizeLiveWikiCodeContext(&result)
	return result, nil
}

func appendLiveGovernanceAuthority(ctx context.Context, db *sql.DB, result *model.WikiCodeContext) {
	if db == nil || result == nil || result.PrimaryDoc.Path == "" {
		return
	}
	docs, err := store.ListDocRecords(ctx, db)
	if err != nil {
		return
	}
	governance, ok := governanceDoc(docs)
	if !ok || governance.Path == result.PrimaryDoc.Path {
		return
	}
	for _, entry := range result.AuthorityChain {
		if entry.Path == governance.Path {
			return
		}
	}
	result.AuthorityChain = append(result.AuthorityChain, model.WikiCodeAuthorityEntry{
		DocID: governance.DocID, Path: governance.Path, Layer: governance.Layer, Role: "governance", ContentHash: governance.ContentHash,
	})
}

func loadLiveWikiCodeMemory(ctx context.Context, root string) (*loadedReentryMemory, error) {
	db, err := openLiveWikiCodeDB(model.WorkspaceRegistration{Root: root})
	if err != nil {
		return nil, nil
	}
	defer db.Close()
	snapshot, ok, err := store.LoadReentrySnapshot(ctx, db)
	if err != nil || !ok {
		return nil, err
	}
	stale := reentry.SnapshotStale(root, snapshot.SnapshotBuiltAt)
	return &loadedReentryMemory{Snapshot: snapshot, Pointer: buildMemoryPointer(snapshot, stale), Stale: stale}, nil
}

func openLiveWikiCodeDB(registration model.WorkspaceRegistration) (*sql.DB, error) {
	path := store.WorkspaceDBPath(registration.Root)
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	db, err := store.OpenReadOnlyExisting(registration.Root, path)
	if err != nil {
		return nil, err
	}
	// Keep enough read slots for a resolver's catalog queries while a separate
	// graph snapshot is pinned; the handle remains immutable/query-only.
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	return db, nil
}

func liveOverlayWasTruncated(overlay model.WikiCodeOverlay) bool {
	for _, omission := range overlay.Omissions {
		reason := strings.ToLower(strings.TrimSpace(omission.Reason))
		if strings.Contains(reason, "bound") || strings.Contains(reason, "truncated") || strings.Contains(reason, "omitted") {
			return true
		}
	}
	return false
}

func ensureLivePrimaryDocument(result *model.WikiCodeContext, scope livecontext.WikiCodeScope, overlay model.WikiCodeOverlay) {
	if result == nil || result.PrimaryDoc.Path != "" || scope.Kind != model.ScopeExactWiki {
		return
	}
	path := ""
	if len(scope.DocPaths) > 0 {
		path = scope.DocPaths[0]
	}
	if path == "" {
		for _, block := range scope.Blocks {
			if block.DocPath != "" {
				path = block.DocPath
				break
			}
		}
	}
	if path == "" {
		for _, binding := range overlay.Additions {
			if model.CanonicalWikiAuthority(binding.DocPath) {
				path = binding.DocPath
				break
			}
		}
	}
	if path == "" || !model.CanonicalWikiAuthority(path) {
		return
	}
	result.PrimaryDoc.Path = path
	for _, binding := range overlay.Additions {
		if binding.DocPath == path && binding.DocID != "" {
			result.PrimaryDoc.DocID = binding.DocID
			break
		}
	}
	if result.PrimaryDoc.DocID == "" && len(scope.DocIDs) == 1 {
		result.PrimaryDoc.DocID = scope.DocIDs[0]
	}
	result.AuthorityChain = append(result.AuthorityChain, model.WikiCodeAuthorityEntry{DocID: result.PrimaryDoc.DocID, Path: path, Role: "primary"})
}

func liveWikiCodeDirection(scope livecontext.WikiCodeScope) string {
	if scope.Kind == model.ScopeReverseCode {
		return model.WikiCodeDirectionCodeToWiki
	}
	return model.WikiCodeDirectionWikiToCode
}

func appendLiveWikiSelectors(scope livecontext.WikiCodeScope) []string {
	selectors := make([]string, 0, len(scope.DocPaths)+len(scope.DocIDs)+len(scope.Blocks))
	selectors = append(selectors, scope.DocPaths...)
	selectors = append(selectors, scope.DocIDs...)
	for _, block := range scope.Blocks {
		if block.DocPath != "" {
			selectors = append(selectors, block.DocPath+"#"+block.BlockID)
		} else if block.DocID != "" {
			selectors = append(selectors, block.DocID+"#"+block.BlockID)
		}
	}
	return selectors
}

func (a *App) enrichLiveWikiCodeContext(ctx context.Context, request model.CommandRequest, env model.Envelope) model.Envelope {
	if !env.Ok || env.Backend == "governance" || !liveWikiCodeOperation(request.Operation) {
		return env
	}
	// prepare builds the bridge from the already-executed route/pack results;
	// never invoke a second route/pack or resolver pass here.
	if request.Operation == "nav.prepare" && env.WikiCodeContext != nil {
		return env
	}
	if all, _ := request.Payload["all_workspaces"].(bool); all && (request.Operation == "nav.trace" || request.Operation == "nav.wiki.trace" || request.Operation == "nav.pack" || request.Operation == "nav.wiki.pack") {
		return a.enrichLiveWikiCodeContextAllWorkspaces(ctx, request, env)
	}
	if request.Operation == "nav.change-pack" {
		return a.enrichLiveChangePackContext(ctx, request, env)
	}
	scope, ok := liveWikiCodeScope(request, env)
	if !ok {
		return env
	}
	result, err := a.buildLiveWikiCodeContextForRequest(ctx, request, scope, env.GraphFreshness)
	if err != nil {
		return appendLiveWikiCodeWarning(env, err)
	}
	setLivePreferredPrimary(&result, livePreferredPrimary(request, env))
	sanitizeLiveWikiCodeContext(&result)
	if (request.Operation == "nav.trace" || request.Operation == "nav.wiki.trace") && result.PrimaryDoc.Path == "" && len(result.DirectCode) == 0 && len(result.Tests) == 0 && len(result.SupportingCode) == 0 && len(result.WikiContext) == 0 {
		return env
	}
	if request.Operation == "nav.related" {
		if items, ok := env.Items.([]symbolNeighborhood); ok && len(items) > 0 {
			items[0].WikiContext = append([]model.WikiCodeWikiContextItem(nil), result.WikiContext...)
			env.Items = items
		}
	}
	env.WikiCodeContext = &result
	if result.Truncated {
		env.Truncated = true
	}
	return env
}

func appendLiveWikiCodeWarning(env model.Envelope, err error) model.Envelope {
	code := "unavailable"
	var contextErr *model.WikiCodeContextError
	if errors.As(err, &contextErr) && strings.TrimSpace(contextErr.Code) != "" {
		code = contextErr.Code
	}
	env.Warnings = appendStringIfMissing(env.Warnings, "wiki-code context unavailable: "+code)
	return env
}

func liveWikiCodeScope(request model.CommandRequest, env model.Envelope) (livecontext.WikiCodeScope, bool) {
	switch request.Operation {
	case "nav.trace", "nav.wiki.trace":
		return liveTraceScope(request, env)
	case "nav.pack", "nav.wiki.pack":
		return livePackScope(request, env)
	case "nav.related":
		return liveRelatedScope(env)
	case "nav.neighbors":
		return liveNeighborsScope(request)
	default:
		return livecontext.WikiCodeScope{}, false
	}
}

func liveTraceScope(request model.CommandRequest, env model.Envelope) (livecontext.WikiCodeScope, bool) {
	scope := livecontext.WikiCodeScope{Kind: model.ScopeExactWiki}
	addID := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if strings.HasPrefix(strings.ReplaceAll(value, "\\", "/"), ".docs/wiki/") || strings.HasSuffix(strings.ToLower(value), ".md") {
			addLiveWikiPath(&scope, value, request)
			return
		}
		scope.DocIDs = appendLiveUnique(scope.DocIDs, value)
	}
	traceID := stringPayload(request.Payload, "rf")
	addID(traceID)
	traceResults := traceResultsFromItems(env.Items)
	hasSourceBlock := false
	for _, result := range traceResults {
		if result.LookupStatus != nil && strings.TrimSpace(result.LookupStatus.BlockID) != "" {
			hasSourceBlock = true
			break
		}
	}
	if !hasSourceBlock {
		if path := liveTracePathFromID(request, traceID); path != "" {
			addLiveWikiPath(&scope, path, request)
		}
	}
	for _, result := range traceResults {
		retiredResult := result.LookupStatus != nil && isRetiredWikiPath(result.LookupStatus.Path)
		if result.DocID != "" && (!retiredResult || liveExplicitHistoricalRequest(request)) {
			addID(result.DocID)
		}
		if result.LookupStatus != nil {
			path := normalizeLiveBridgePath(result.LookupStatus.Path)
			blockID := strings.TrimSpace(result.LookupStatus.BlockID)
			if path != "" && strings.HasPrefix(path, ".docs/wiki/") && blockID != "" && (!isRetiredWikiPath(path) || liveExplicitHistoricalRequest(request)) {
				scope.Blocks = append(scope.Blocks, model.WikiCodeBlockScope{DocPath: path, BlockID: blockID})
			} else {
				addLiveWikiPath(&scope, path, request)
			}
		}
		if result.LookupStatus == nil || strings.TrimSpace(result.LookupStatus.BlockID) == "" {
			if path := liveTracePathFromID(request, result.DocID); path != "" {
				addLiveWikiPath(&scope, path, request)
			}
		}
		for _, link := range result.Explicit {
			if (link.Source == "doc_source" || link.Kind == "wiki-source") && (result.LookupStatus == nil || strings.TrimSpace(result.LookupStatus.BlockID) == "") {
				addLiveWikiPath(&scope, link.File, request)
			}
		}
	}
	return scope, len(scope.DocIDs) > 0 || len(scope.DocPaths) > 0 || len(scope.Blocks) > 0 || len(traceResults) > 0
}

func liveTracePathFromID(request model.CommandRequest, docID string) string {
	docID = strings.TrimSpace(docID)
	if docID == "" || strings.TrimSpace(request.Context.Workspace) == "" {
		return ""
	}
	registration, err := workspace.ResolveWorkspace(request.Context.Workspace)
	if err != nil || strings.TrimSpace(registration.Root) == "" {
		return ""
	}
	best := ""
	bestScore := -1
	for _, candidate := range traceGovernedDocCandidates(registration.Root, "functional") {
		if !model.CanonicalWikiAuthority(candidate) || isRetiredWikiPath(candidate) {
			continue
		}
		if embeddedDocIDTitle(registration.Root, candidate, docID) == "" {
			continue
		}
		score := 1
		if isSpecificRFDocPath(candidate) || isSpecificTPDocPath(candidate) || isSpecificRSDocPath(candidate) {
			score += 10
		}
		if !isRFIndexPath(candidate) && !isTPIndexPath(candidate) && !isRSIndexPath(candidate) {
			score += 5
		}
		if score > bestScore || (score == bestScore && (best == "" || candidate < best)) {
			best, bestScore = candidate, score
		}
	}
	return best
}

func livePackScope(request model.CommandRequest, env model.Envelope) (livecontext.WikiCodeScope, bool) {
	scope := livecontext.WikiCodeScope{Kind: model.ScopeExactWiki}
	addLiveWikiPath(&scope, stringPayload(request.Payload, "doc"), request)
	for _, result := range packResultsFromItems(env.Items) {
		addLiveWikiPath(&scope, result.PrimaryDoc, request)
		for _, doc := range result.Docs {
			addLiveWikiPath(&scope, doc.Path, request)
			if doc.DocID != "" && (!isRetiredWikiPath(doc.Path) || liveExplicitHistoricalRequest(request)) {
				scope.DocIDs = appendLiveUnique(scope.DocIDs, doc.DocID)
			}
		}
	}
	for _, key := range []string{"rf", "fl"} {
		if value := strings.TrimSpace(stringPayload(request.Payload, key)); value != "" {
			scope.DocIDs = appendLiveUnique(scope.DocIDs, value)
		}
	}
	return scope, len(scope.DocIDs) > 0 || len(scope.DocPaths) > 0
}

func livePreferredPrimary(request model.CommandRequest, env model.Envelope) model.DocRecord {
	switch request.Operation {
	case "nav.trace", "nav.wiki.trace":
		results := traceResultsFromItems(env.Items)
		if len(results) == 0 {
			return model.DocRecord{}
		}
		result := results[0]
		preferred := model.DocRecord{DocID: result.DocID, Layer: result.Layer}
		if result.LookupStatus != nil {
			preferred.Path = normalizeLiveBridgePath(result.LookupStatus.Path)
			if result.LookupStatus.DocID != "" {
				preferred.DocID = result.LookupStatus.DocID
			}
		}
		for _, link := range result.Explicit {
			if (link.Source == "doc_source" || link.Kind == "wiki-source") && preferred.Path == "" {
				preferred.Path = normalizeLiveBridgePath(link.File)
			}
		}
		return preferred
	case "nav.pack", "nav.wiki.pack":
		results := packResultsFromItems(env.Items)
		if len(results) == 0 {
			return model.DocRecord{}
		}
		result := results[0]
		preferred := model.DocRecord{Path: normalizeLiveBridgePath(result.PrimaryDoc)}
		for _, doc := range result.Docs {
			if doc.Path == preferred.Path {
				preferred.DocID = doc.DocID
				preferred.Title = doc.Title
				preferred.Layer = doc.Layer
				preferred.Family = doc.Family
				break
			}
		}
		return preferred
	default:
		return model.DocRecord{}
	}
}

func setLivePreferredPrimary(result *model.WikiCodeContext, preferred model.DocRecord) {
	if result == nil || preferred.Path == "" || !model.CanonicalWikiAuthority(preferred.Path) {
		return
	}
	if result.PrimaryDoc.Path == preferred.Path {
		if preferred.DocID != "" {
			result.PrimaryDoc.DocID = preferred.DocID
		}
		if result.PrimaryDoc.Title == "" {
			result.PrimaryDoc.Title = preferred.Title
		}
		if result.PrimaryDoc.Layer == "" {
			result.PrimaryDoc.Layer = preferred.Layer
		}
		if result.PrimaryDoc.Family == "" {
			result.PrimaryDoc.Family = preferred.Family
		}
		for index := range result.AuthorityChain {
			if result.AuthorityChain[index].Path == preferred.Path {
				if preferred.DocID != "" {
					result.AuthorityChain[index].DocID = preferred.DocID
				}
				if preferred.Layer != "" {
					result.AuthorityChain[index].Layer = preferred.Layer
				}
			}
		}
		return
	}
	result.PrimaryDoc = preferred
	chain := make([]model.WikiCodeAuthorityEntry, 0, len(result.AuthorityChain)+1)
	for _, entry := range result.AuthorityChain {
		if entry.Role == "primary" || entry.Path == preferred.Path {
			continue
		}
		chain = append(chain, entry)
	}
	chain = append(chain, model.WikiCodeAuthorityEntry{DocID: preferred.DocID, Path: preferred.Path, Layer: preferred.Layer, Role: "primary"})
	result.AuthorityChain = chain
}

func liveRelatedScope(env model.Envelope) (livecontext.WikiCodeScope, bool) {
	items, ok := env.Items.([]symbolNeighborhood)
	if !ok || len(items) == 0 || items[0].Definition == nil {
		return livecontext.WikiCodeScope{}, false
	}
	path := normalizeLiveBridgePath(items[0].Definition.File)
	if path == "" {
		return livecontext.WikiCodeScope{}, false
	}
	return livecontext.WikiCodeScope{Kind: model.ScopeReverseCode, TargetPath: path, TargetSymbol: strings.TrimSpace(items[0].Definition.Name)}, true
}

func liveNeighborsScope(request model.CommandRequest) (livecontext.WikiCodeScope, bool) {
	selector := strings.TrimSpace(stringPayload(request.Payload, "selector"))
	if selector == "" {
		return livecontext.WikiCodeScope{}, false
	}
	if isLiveWikiSelector(selector) {
		scope := livecontext.WikiCodeScope{Kind: model.ScopeExactWiki}
		addLiveWikiSelector(&scope, selector, request)
		return scope, len(scope.DocPaths) > 0 || len(scope.DocIDs) > 0
	}
	path, symbol := splitLiveBridgeSelector(selector)
	path = normalizeLiveBridgePath(path)
	if path == "" || (!strings.Contains(path, "/") && filepath.Ext(path) == "") {
		return livecontext.WikiCodeScope{}, false
	}
	if symbol == "" {
		symbol = strings.TrimSpace(stringPayload(request.Payload, "symbol"))
	}
	return livecontext.WikiCodeScope{Kind: model.ScopeReverseCode, TargetPath: path, TargetSymbol: symbol}, true
}

func addLiveWikiSelector(scope *livecontext.WikiCodeScope, selector string, request model.CommandRequest) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return
	}
	normalized := strings.ReplaceAll(selector, "\\", "/")
	if strings.HasPrefix(normalized, ".docs/wiki/") || strings.HasSuffix(strings.ToLower(normalized), ".md") {
		addLiveWikiPath(scope, selector, request)
		return
	}
	scope.DocIDs = appendLiveUnique(scope.DocIDs, selector)
}

func addLiveWikiPath(scope *livecontext.WikiCodeScope, value string, request model.CommandRequest) {
	path := normalizeLiveBridgePath(value)
	if path == "" || !strings.HasPrefix(path, ".docs/wiki/") {
		return
	}
	if isRetiredWikiPath(path) && !liveExplicitHistoricalRequest(request) {
		return
	}
	scope.DocPaths = appendLiveUnique(scope.DocPaths, path)
}

func liveExplicitHistoricalRequest(request model.CommandRequest) bool {
	if value, ok := request.Payload["historical"].(bool); ok && value {
		return true
	}
	switch request.Operation {
	case "nav.trace", "nav.wiki.trace":
		return strings.TrimSpace(stringPayload(request.Payload, "rf")) != ""
	case "nav.pack", "nav.wiki.pack":
		return strings.TrimSpace(stringPayload(request.Payload, "doc")) != "" || strings.TrimSpace(stringPayload(request.Payload, "rf")) != "" || strings.TrimSpace(stringPayload(request.Payload, "fl")) != ""
	default:
		return false
	}
}

func isLiveWikiSelector(value string) bool {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	return strings.HasPrefix(value, ".docs/wiki/") || strings.HasSuffix(strings.ToLower(value), ".md") || isLiveWikiDocumentID(value)
}

func isLiveWikiDocumentID(value string) bool {
	upper := strings.ToUpper(strings.TrimSpace(value))
	for _, prefix := range []string{"RS-", "RF-", "FL-", "TP-", "CT-", "TECH-", "DB-"} {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	return false
}

func splitLiveBridgeSelector(selector string) (string, string) {
	if index := strings.LastIndex(selector, "#"); index > 0 && index < len(selector)-1 {
		return selector[:index], selector[index+1:]
	}
	return selector, ""
}

func normalizeLiveBridgePath(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" || strings.ContainsRune(value, 0) || strings.ContainsAny(value, "\r\n") || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.HasPrefix(value, "~/") || value == "~" || (len(value) > 1 && value[1] == ':') {
		return ""
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." || part == "" {
			return ""
		}
	}
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
}

func appendLiveUnique(items []string, value string) []string {
	for _, item := range items {
		if item == value || strings.EqualFold(item, value) {
			return items
		}
	}
	return append(items, value)
}

func traceResultsFromItems(items any) []model.TraceResult {
	switch typed := items.(type) {
	case []model.TraceResult:
		return typed
	case []any:
		result := make([]model.TraceResult, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(model.TraceResult); ok {
				result = append(result, value)
			}
		}
		return result
	default:
		return nil
	}
}

func packResultsFromItems(items any) []model.PackResult {
	switch typed := items.(type) {
	case []model.PackResult:
		return typed
	case []any:
		result := make([]model.PackResult, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(model.PackResult); ok {
				result = append(result, value)
			}
		}
		return result
	default:
		return nil
	}
}

func sanitizeLiveWikiCodeContext(result *model.WikiCodeContext) {
	if result == nil {
		return
	}
	result.PrimaryDoc.Path = sanitizeLivePublicPath(result.PrimaryDoc.Path)
	result.PrimaryDoc.Snippet = ""
	result.PrimaryDoc.SearchText = ""
	result.PrimaryDoc.IndexedAt = 0
	for index := range result.AuthorityChain {
		result.AuthorityChain[index].Path = sanitizeLivePublicPath(result.AuthorityChain[index].Path)
	}
	for index := range result.CodeEvidence {
		sanitizeLiveEvidence(&result.CodeEvidence[index])
	}
	for _, collection := range [][]model.WikiCodeEvidence{result.DirectCode, result.Tests, result.SupportingCode, result.Candidates} {
		for index := range collection {
			sanitizeLiveEvidence(&collection[index])
		}
	}
	for index := range result.GraphPaths {
		result.GraphPaths[index].From = sanitizeLivePublicPath(result.GraphPaths[index].From)
		result.GraphPaths[index].To = sanitizeLivePublicPath(result.GraphPaths[index].To)
	}
	for index := range result.Omissions {
		result.Omissions[index].Source = sanitizeLivePublicPath(result.Omissions[index].Source)
		for candidate := range result.Omissions[index].Candidates {
			result.Omissions[index].Candidates[candidate] = sanitizeLivePublicPath(result.Omissions[index].Candidates[candidate])
		}
	}
	for index := range result.WikiContext {
		sanitizeLiveWikiContextItem(&result.WikiContext[index])
	}
	model.SortWikiCodeContext(result)
	result.DeterminismDigest = model.WikiCodeContextDigest(*result)
}

func sanitizeLiveEvidence(item *model.WikiCodeEvidence) {
	if item == nil {
		return
	}
	item.Path = sanitizeLivePublicPath(item.Path)
	item.DocPath = sanitizeLivePublicPath(item.DocPath)
	item.SourceDoc = sanitizeLivePublicPath(item.SourceDoc)
}

func sanitizeLiveWikiContextItem(item *model.WikiCodeWikiContextItem) {
	if item == nil {
		return
	}
	item.Path = sanitizeLivePublicPath(item.Path)
	for index := range item.Parents {
		sanitizeLiveWikiContextItem(&item.Parents[index])
	}
}

func sanitizeLivePublicPath(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" || strings.ContainsRune(value, 0) || strings.ContainsAny(value, "\r\n") || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.HasPrefix(value, "~/") || value == "~" || (len(value) > 1 && value[1] == ':') {
		return ""
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == ".." {
			return ""
		}
	}
	return value
}

func (a *App) enrichLiveWikiCodeContextAllWorkspaces(ctx context.Context, request model.CommandRequest, env model.Envelope) model.Envelope {
	items, ok := env.Items.([]any)
	if !ok {
		return env
	}
	for index := range items {
		switch item := items[index].(type) {
		case model.TraceResult:
			if item.Workspace == "" {
				continue
			}
			child := request
			child.Context.Workspace = item.Workspace
			child.Payload = cloneIntentPayload(request.Payload)
			child.Payload["all_workspaces"] = false
			result, resultErr := a.buildLiveWikiCodeContextForRequest(ctx, child, liveScopeFromTraceResult(child, item), nil)
			if resultErr != nil {
				env = appendLiveWikiCodeWarning(env, resultErr)
				continue
			}
			setLivePreferredPrimary(&result, livePreferredPrimary(child, model.Envelope{Items: []model.TraceResult{item}}))
			sanitizeLiveWikiCodeContext(&result)
			if result.PrimaryDoc.Path == "" && len(result.DirectCode) == 0 && len(result.Tests) == 0 && len(result.SupportingCode) == 0 && len(result.WikiContext) == 0 {
				continue
			}
			item.WikiCodeContext = &result
			items[index] = item
			if result.Truncated {
				env.Truncated = true
			}
		case model.PackResult:
			if item.Workspace == "" {
				continue
			}
			child := request
			child.Context.Workspace = item.Workspace
			child.Payload = cloneIntentPayload(request.Payload)
			child.Payload["all_workspaces"] = false
			result, resultErr := a.buildLiveWikiCodeContextForRequest(ctx, child, liveScopeFromPackResult(child, item), nil)
			if resultErr != nil {
				env = appendLiveWikiCodeWarning(env, resultErr)
				continue
			}
			setLivePreferredPrimary(&result, livePreferredPrimary(child, model.Envelope{Items: []model.PackResult{item}}))
			sanitizeLiveWikiCodeContext(&result)
			item.WikiCodeContext = &result
			items[index] = item
			if result.Truncated {
				env.Truncated = true
			}
		}
	}
	env.Items = items
	return env
}

func liveScopeFromTraceResult(request model.CommandRequest, result model.TraceResult) livecontext.WikiCodeScope {
	scope, _ := liveTraceScope(request, model.Envelope{Items: []model.TraceResult{result}})
	return scope
}

func liveScopeFromPackResult(request model.CommandRequest, result model.PackResult) livecontext.WikiCodeScope {
	scope, _ := livePackScope(request, model.Envelope{Items: []model.PackResult{result}})
	return scope
}

func (a *App) enrichLiveChangePackContext(ctx context.Context, request model.CommandRequest, env model.Envelope) model.Envelope {
	packets, ok := env.Items.([]ChangePackPacket)
	if !ok || len(packets) == 0 {
		return env
	}
	packet := &packets[0]
	targets := changePackLiveTargets(*packet)
	var first *model.WikiCodeContext
	classifications := make([]string, 0, len(targets))
	nextQueries := make([]string, 0)
	hasApplicableBinding := false
	for _, target := range targets {
		scope := livecontext.WikiCodeScope{Kind: model.ScopeReverseCode, TargetPath: target.path, TargetSymbol: target.symbol}
		result, resultErr := a.buildLiveWikiCodeContextForRequest(ctx, request, scope, env.GraphFreshness)
		if resultErr != nil {
			env = appendLiveWikiCodeWarning(env, resultErr)
			continue
		}
		if first == nil {
			copyResult := result
			first = &copyResult
		}
		for _, item := range result.WikiContext {
			if item.Status != "redirect" {
				hasApplicableBinding = true
				break
			}
		}
		for _, classification := range result.Classifications {
			classifications = appendLiveUnique(classifications, classification)
		}
		for _, query := range result.NextQueries {
			nextQueries = appendLiveUnique(nextQueries, query)
		}
	}
	if first == nil {
		return env
	}
	if !hasApplicableBinding {
		classifications = appendLiveUnique(classifications, model.WikiCodeClassificationUnmappedChangedCode)
	}
	sortLiveClassifications(classifications)
	sort.Strings(nextQueries)
	maxNextQueries := request.Context.MaxItems
	if maxNextQueries < 1 {
		maxNextQueries = 12
	}
	if maxNextQueries > 32 {
		maxNextQueries = 32
	}
	if len(nextQueries) > maxNextQueries {
		nextQueries = nextQueries[:maxNextQueries]
		first.Truncated = true
	}
	first.Classifications = classifications
	if !hasApplicableBinding {
		first.Classification = model.WikiCodeClassificationUnmappedChangedCode
	} else if len(classifications) > 0 {
		first.Classification = classifications[0]
	}
	first.NextQueries = nextQueries
	if first.Truncated && first.Continuation == nil {
		first.NextCursor = model.WikiCodeDirectionCodeToWiki + ":budget"
		first.Continuation = &model.WikiCodeContextContinuation{Direction: model.WikiCodeDirectionCodeToWiki, Cursor: first.NextCursor}
	}
	model.SortWikiCodeContext(first)
	first.DeterminismDigest = model.WikiCodeContextDigest(*first)
	packet.Classifications = classifications
	packet.NextQueries = nextQueries
	if !hasApplicableBinding {
		packet.Classification = model.WikiCodeClassificationUnmappedChangedCode
	} else if len(classifications) > 0 {
		packet.Classification = classifications[0]
	}
	env.WikiCodeContext = first
	if first.Truncated {
		env.Truncated = true
	}
	env.Items = packets
	return env
}

type changePackLiveTarget struct {
	path   string
	symbol string
}

func changePackLiveTargets(packet ChangePackPacket) []changePackLiveTarget {
	targets := make([]changePackLiveTarget, 0, len(packet.ChangedPaths))
	seen := make(map[string]struct{})
	for _, path := range packet.ChangedPaths {
		path = normalizeLiveBridgePath(path)
		if path == "" {
			continue
		}
		symbol := ""
		for _, changed := range packet.ChangedSymbols {
			if normalizeLiveBridgePath(changed.File) == path {
				symbol = strings.TrimSpace(changed.Name)
				break
			}
		}
		key := path + "\x00" + symbol
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, changePackLiveTarget{path: path, symbol: symbol})
	}
	if len(targets) == 0 {
		for _, changed := range packet.ChangedSymbols {
			path := normalizeLiveBridgePath(changed.File)
			if path == "" {
				continue
			}
			symbol := strings.TrimSpace(changed.Name)
			key := path + "\x00" + symbol
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			targets = append(targets, changePackLiveTarget{path: path, symbol: symbol})
		}
	}
	return targets
}

func sortLiveClassifications(values []string) {
	order := func(value string) int {
		switch value {
		case model.WikiCodeClassificationDirectSDDBinding:
			return 10
		case model.WikiCodeClassificationSupportingCode:
			return 20
		case model.WikiCodeClassificationSharedTechnical:
			return 30
		case model.WikiCodeClassificationGeneratedOrVendor:
			return 40
		case model.WikiCodeClassificationMechanicalNoImpact:
			return 50
		case model.WikiCodeClassificationUnmappedChangedCode:
			return 60
		default:
			return 100
		}
	}
	sort.SliceStable(values, func(i, j int) bool {
		left, right := order(values[i]), order(values[j])
		if left != right {
			return left < right
		}
		return values[i] < values[j]
	})
}

func (a *App) normalizeWorkspaceRequest(request model.CommandRequest) (model.CommandRequest, []string, error) {
	if !operationRequiresWorkspaceResolution(request) {
		return request, nil, nil
	}
	if strings.TrimSpace(request.Context.Workspace) != "" {
		selector := strings.TrimSpace(request.Context.Workspace)
		warnings := []string{}
		if mismatch, ok := workspace.ExplicitWorkspaceCWDMismatchFor(selector, request.Context.CallerCWD); ok {
			warnings = append(warnings, mismatch.Warning)
			if isHarnessClientName(request.Context.ClientName) && !request.Context.AllowCrossWorkspace {
				if cwdCanonLinkAllows(mismatch.CWDWorkspaceAlias, selector, "") {
					warnings = append(warnings, fmt.Sprintf("workspace used a registry canon link to reach alias %q from cwd workspace %q", selector, mismatch.CWDWorkspaceAlias))
				} else {
					return request, nil, fmt.Errorf("workspace cross-workspace refused: --workspace %q resolves to root %q, but caller cwd %q is inside workspace %q at root %q; recommended command: mi-lsp %s --format toon; pass --allow-cross-workspace only when this cross-workspace query is intentional",
						mismatch.Selector,
						mismatch.SelectedRoot,
						mismatch.CallerCWD,
						mismatch.CWDWorkspaceAlias,
						mismatch.CWDWorkspaceRoot,
						recommendedWorkspaceCommand(request.Operation, mismatch.CWDWorkspaceAlias),
					)
				}
			}
		}
		if strings.TrimSpace(request.Context.WorkspaceSource) == "" {
			if resolution, err := workspace.ResolveWorkspaceSelection(selector, request.Context.CallerCWD); err == nil {
				request.Context.WorkspaceSource = string(resolution.Source)
			} else {
				request.Context.WorkspaceSource = string(workspace.ResolutionSourceExplicit)
			}
		}
		return request, warnings, nil
	}
	resolution, err := workspace.ResolveWorkspaceSelection(request.Context.Workspace, request.Context.CallerCWD)
	if err != nil {
		return request, nil, err
	}
	// Synthetic Git resolutions have a physical root but no registry alias. Keep
	// the root as the downstream selector so workspace operations do not turn
	// the synthetic name into an alias lookup.
	if resolution.Source == workspace.ResolutionSourceGitTopLevel && !workspaceAliasRegistered(resolution.Registration.Name) {
		request.Context.Workspace = resolution.Registration.Root
	} else {
		request.Context.Workspace = resolution.Registration.Name
	}
	request.Context.WorkspaceSource = string(resolution.Source)
	return request, resolution.Warnings, nil
}

func operationRequiresWorkspaceResolution(request model.CommandRequest) bool {
	if allWorkspaces, _ := request.Payload["all_workspaces"].(bool); allWorkspaces && strings.HasPrefix(request.Operation, "nav.") {
		return false
	}
	switch request.Operation {
	case "workspace.add", "workspace.init", "workspace.scan", "workspace.list", "workspace.doctor", "workspace.hygiene", "workspace.prune", "workspace.remove", "workspace.warm", "registry.gc", "worker.install", "worker.status":
		return false
	case "nav.find", "nav.search":
		allWorkspaces, _ := request.Payload["all_workspaces"].(bool)
		return !allWorkspaces
	case "index.run", "index.start":
		return strings.TrimSpace(stringPayload(request.Payload, "path")) == ""
	case "index.status", "index.cancel", "index.run-job", "workspace.status", "workspace.link", "info", "nav.symbols", "nav.overview", "nav.outline", "nav.governance", "nav.wiki-root", "nav.route", "nav.wiki.route", "nav.ask", "nav.pack", "nav.wiki.pack", "nav.wiki.search", "nav.wiki.validate-harness", "nav.wiki.validate-source", "nav.wiki.inventory", "nav.wiki.map", "nav.evidence.inventory", "nav.service", "nav.refs", "nav.context", "nav.deps", "nav.multi-read", "nav.batch", "nav.related", "nav.workspace-map", "nav.diff-context", "nav.affected", "nav.flow-slice", "nav.change-pack", "nav.edit-plan", "nav.prepare", "prepare.create", "prepare.verify", "prepare.refresh", "nav.trace", "nav.wiki.trace", "nav.intent", "nav.recall", "nav.neighbors", "nav.callers", "nav.callees", "nav.path", "nav.explain", "nav.graph.stats", "nav.graph.status", "nav.graph.rank", "nav.graph.validate", "nav.graph-impact":

		return true
	default:
		return false
	}
}

func cwdCanonLinkAllows(cwdAlias, selectedAlias, role string) bool {
	registry, err := workspace.LoadRegistry()
	if err != nil {
		return false
	}
	_, cwd, ok := workspace.FindRegisteredWorkspace(registry, cwdAlias)
	if !ok {
		return false
	}
	return workspace.RegistrationHasCanonLink(cwd, selectedAlias, role)
}

func isHarnessClientName(clientName string) bool {
	switch strings.ToLower(strings.TrimSpace(clientName)) {
	case "claude-code", "codex", "claude-ai", "opencode", "copilot", "jetbrains", "cursor", "neovim", "emacs", "vim":
		return true
	default:
		return false
	}
}

func recommendedWorkspaceCommand(operation string, alias string) string {
	alias = strings.TrimSpace(alias)
	switch strings.TrimSpace(operation) {
	case "workspace.status":
		return fmt.Sprintf("workspace status %s", alias)
	case "nav.governance":
		return fmt.Sprintf("nav governance --workspace %s", alias)
	case "nav.workspace-map":
		return fmt.Sprintf("nav workspace-map --workspace %s", alias)
	default:
		if strings.HasPrefix(operation, "nav.") {
			return fmt.Sprintf("%s --workspace %s", strings.ReplaceAll(operation, ".", " "), alias)
		}
		return fmt.Sprintf("%s --workspace %s", strings.ReplaceAll(operation, ".", " "), alias)
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
	docsOnly, _ := request.Payload["docs_only"].(bool)

	var envelope model.Envelope
	err = store.WithWorkspaceIndexLock(registration.Root, "index.run", func() error {
		if docsOnly {
			result, err := indexer.IndexWorkspaceDocsOnly(ctx, registration.Root)
			if err != nil {
				return err
			}
			result.Warnings, err = a.appendWikiEmbeddingWarnings(ctx, registration.Root, result.Warnings, nil)
			if err != nil {
				return err
			}
			envelope = model.Envelope{Ok: true, Workspace: registration.Name, Backend: "docgraph", Items: []model.DocRecord{}, Stats: result.Stats, Warnings: result.Warnings}
			return nil
		}

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
				warnings := appendStringIfMissing(result.Warnings, "no changes detected")
				warnings, err = a.appendWikiEmbeddingWarnings(ctx, registration.Root, warnings, nil)
				if err != nil {
					return err
				}
				envelope = model.Envelope{Ok: true, Workspace: registration.Name, Backend: "catalog", Items: []model.SymbolRecord{}, Stats: result.Stats, Warnings: warnings}
				return nil
			}
			// If incremental failed, fall through to full index
		}

		// Fall back to full index if incremental didn't succeed
		if !incremental {
			result, err = indexer.IndexWorkspaceWithGraphProgress(ctx, registration.Root, clean, "", nil, indexer.GraphIndexOptions{RoslynObserver: a.graphObserver()})
			if err != nil {
				if store.IsCorruptionError(err) {
					backupPath, backupErr := store.QuarantineCorruptDB(registration.Root)
					if backupErr != nil {
						return fmt.Errorf("%w; corrupt db quarantine failed: %v", err, backupErr)
					}
					result, err = indexer.IndexWorkspace(ctx, registration.Root, true)
					if err != nil {
						return fmt.Errorf("%w; rebuild after quarantining %s also failed: %v", err, backupPath, err)
					}
					result.Warnings = appendStringIfMissing(result.Warnings, "corrupt index database was quarantined to "+backupPath)
					result.Warnings = appendStringIfMissing(result.Warnings, "full rebuild completed after corruption recovery")
				} else {
					return err
				}
			}
		}

		// Add incremental flag to warnings if successful
		warnings := result.Warnings
		if incremental {
			warnings = appendStringIfMissing(warnings, "incremental=true")
		}
		warnings, err = a.appendWikiEmbeddingWarnings(ctx, registration.Root, warnings, nil)
		if err != nil {
			return err
		}
		envelope = model.Envelope{Ok: true, Workspace: registration.Name, Backend: "catalog", Items: result.Symbols, Stats: result.Stats, Warnings: warnings}
		return nil
	})
	if err != nil {
		if lockErr, ok := err.(*store.IndexLockError); ok {
			return model.Envelope{}, fmt.Errorf("index already running for workspace %s: %w", registration.Name, lockErr)
		}
		return model.Envelope{}, err
	}
	return envelope, nil
}

func (a *App) graphQuery(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	// Normalize and reject all budgets before resolving or opening SQLite.
	q, err := graphRequestFromPayload(request)
	if err != nil {
		return model.Envelope{}, err
	}
	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, &model.GraphQueryError{Code: "GPH_QUERY_BACKEND_UNAVAILABLE", Message: "graph backend is unavailable"}
	}
	db, err := openWorkspaceDB(registration, request.Operation, true)
	if err != nil {
		return model.Envelope{}, store.SanitizeGraphQueryError(err)
	}
	defer db.Close()
	envelope, err := GraphQuery(ctx, db, q)
	if err != nil {
		if fallback, ok := liveNeighborsStaleFallback(ctx, request, q, db, registration, err); ok {
			return fallback, nil
		}
		return model.Envelope{}, store.SanitizeGraphQueryError(err)
	}
	envelope.Workspace = registration.Name
	envelope.Backend = "sqlite-direct"
	return envelope, nil
}

func liveNeighborsStaleFallback(ctx context.Context, request model.CommandRequest, q model.GraphQueryRequest, db *sql.DB, registration model.WorkspaceRegistration, queryErr error) (model.Envelope, bool) {
	if q.Operation != "nav.neighbors" {
		return model.Envelope{}, false
	}
	var graphErr *model.GraphQueryError
	if !errors.As(queryErr, &graphErr) || graphErr.Code != "GPH_QUERY_GRAPH_INVALID" {
		return model.Envelope{}, false
	}
	if _, ok := liveNeighborsScope(request); !ok {
		return model.Envelope{}, false
	}
	freshness, freshnessErr := store.GraphFreshness(ctx, db, q.Generation)
	if freshnessErr != nil || freshness.State != model.GraphFreshnessStale {
		if runtimeState, runtimeErr := store.GraphRuntimeState(ctx, db); runtimeErr == nil && runtimeState == store.GraphRuntimeStale {
			freshness = model.GraphFreshness{State: model.GraphFreshnessStale, ReasonCode: "runtime_marked_stale"}
		} else {
			return model.Envelope{}, false
		}
	}
	return model.Envelope{
		Ok:             true,
		Workspace:      registration.Name,
		Backend:        "sqlite-direct",
		Mode:           "query_only",
		Items:          []model.GraphQueryItem{},
		Warnings:       []string{"graph neighbors unavailable; bridge served with stale graph freshness"},
		Operation:      q.Operation,
		GenerationID:   freshness.GenerationID,
		GraphFreshness: &freshness,
	}, true
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
	db, err := openWorkspaceDB(registration, "info", true) // readOnly
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
	db, err := openWorkspaceDB(registration, "nav.symbols", true) // readOnly
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
	db, err := openWorkspaceDB(registration, "nav.find", true) // readOnly
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
	db, err := openWorkspaceDB(registration, "nav.overview", true) // readOnly
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
	if shouldDegradeUnscopedContainerSearch(project, request, includeContent) {
		envelope := buildContainerSearchScopePreview(registration.Name, project, pattern, includeContent, useRegex)
		envelope = attachMemoryPointer(envelope, memory)
		return applyCoachPolicy(envelope, request.Context), nil
	}
	searchRoot := registration.Root
	if scopedRepo != nil {
		searchRoot = filepath.Join(registration.Root, filepath.FromSlash(scopedRepo.Root))
	}
	searchLimit := searchTextLimit(request.Context, pattern, useRegex)
	searchDiagnostics := &searchPatternDiagnostics{}
	searchCtx, searchCancel := withSearchTimeout(ctx, a.Config.SearchTimeout)
	defer searchCancel()
	items, err := searchPatternScopedWithDiagnostics(searchCtx, registration.Root, searchRoot, project, pattern, useRegex, searchLimit, searchDiagnostics)
	warnings := []string{}
	regexAutoHealed := false
	if err != nil {
		if useRegex && isRegexParseError(err) {
			items, err = searchPatternScopedWithDiagnostics(searchCtx, registration.Root, searchRoot, project, pattern, false, searchLimit, searchDiagnostics)
			if err != nil {
				return model.Envelope{}, err
			}
			warnings = append(warnings, "invalid regex detected; retried automatically as literal search")
			useRegex = false
			regexAutoHealed = true
		} else if code := classifySearchRuntimeFailure(err); code != "" {
			if searchDiagnostics.RipgrepFallbackCode == "" {
				searchDiagnostics.RipgrepFallbackCode = code
			}
			items, err = searchPatternFallbackWithDiagnostics(searchCtx, registration.Root, searchRoot, project, pattern, useRegex, searchLimit, searchDiagnostics)
			if err != nil {
				return model.Envelope{}, err
			}
		} else {
			return model.Envelope{}, err
		}
	}
	warnings = appendSearchDiagnosticsWarnings(warnings, searchDiagnostics)
	if len(items) == 0 && !useRegex && looksRegexLikePattern(pattern) {
		warnings = append(warnings, "no literal matches; pattern looks regex-like, rerun with --regex")
	}
	warnings = append(warnings, scopeWarnings...)

	if !useRegex && isIdentifierLikeQuery(pattern) {
		rankIdentifierSearchItems(pattern, items)
		if request.Context.MaxItems > 0 && len(items) > request.Context.MaxItems {
			items = items[:request.Context.MaxItems]
		}
	}

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
	if searchDiagnostics.TimedOut {
		if len(items) > 0 {
			hint = fmt.Sprintf("search timed out after returning %d partial result(s) for %q", len(items), pattern)
		} else {
			hint = fmt.Sprintf("0 matches for %q: search timed out before matches", pattern)
		}
		rerun := "narrow with --repo or a more specific pattern"
		nextHint = &rerun
	} else if len(items) == 0 {
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
	env.Coach = buildSearchCoach(registration.Name, project, pattern, includeContent, stringPayload(request.Payload, "repo"), useRegex, regexAutoHealed, searchDiagnostics.TimedOut, items, request.Context)
	env = attachMemoryPointer(env, memory)
	env.Continuation = buildSearchContinuation(pattern, project, stringPayload(request.Payload, "repo"), items, memory)
	if isAXIPreview(request.Context) && env.NextHint == nil {
		env = applyAXIPreviewHints(env, request.Context, axiPreviewSummaryHint)
	}
	return applyCoachPolicy(env, request.Context), nil
}

func shouldDegradeUnscopedContainerSearch(project model.ProjectFile, request model.CommandRequest, includeContent bool) bool {
	if !includeContent || request.Context.Full {
		return false
	}
	if strings.TrimSpace(stringPayload(request.Payload, "repo")) != "" {
		return false
	}
	return project.Project.Kind == model.WorkspaceKindContainer || len(project.Repos) > 1
}

func buildContainerSearchScopePreview(alias string, project model.ProjectFile, pattern string, includeContent bool, useRegex bool) model.Envelope {
	items := make([]map[string]any, 0, len(project.Repos))
	actions := make([]model.CoachAction, 0, min(len(project.Repos), 2))
	for _, repo := range project.Repos {
		command := searchCommand(alias, pattern, includeContent, repo.Name, useRegex, false)
		items = append(items, map[string]any{
			"repo":      repo.Name,
			"repo_id":   repo.ID,
			"root":      repo.Root,
			"languages": repo.Languages,
			"command":   command,
		})
		if len(actions) < 2 {
			actions = append(actions, coachAction("narrow", "Search repo "+repo.Name, command))
		}
	}
	rerun := "rerun with --repo <name> --include-content after choosing a repo; use --full only to force broad container search"
	if len(project.Repos) == 1 {
		rerun = searchCommand(alias, pattern, includeContent, project.Repos[0].Name, useRegex, false)
	} else if len(project.Repos) > 1 {
		rerun = searchCommand(alias, pattern, includeContent, project.Repos[0].Name, useRegex, false)
	}
	return model.Envelope{
		Ok:        true,
		Workspace: alias,
		Backend:   "planner",
		Mode:      "scope-preview",
		Items:     items,
		Warnings: []string{
			"container search with --include-content and no --repo was degraded to a scope preview to avoid broad token-heavy output",
		},
		Hint:     "choose a repo before requesting inline content",
		NextHint: &rerun,
		Stats:    model.Stats{Files: len(items)},
		Coach: &model.Coach{
			Trigger:    coachTriggerScopeNarrowingRequired,
			Message:    "Inline content search across a container workspace is token-expensive; choose the repo first.",
			Confidence: "high",
			Actions:    actions,
		},
		Continuation: &model.Continuation{
			Reason: "scope_narrowing_required",
			Next: model.ContinuationTarget{
				Op:    "nav.search",
				Query: pattern,
				Repo:  firstProjectRepoName(project),
			},
		},
	}
}

func firstProjectRepoName(project model.ProjectFile) string {
	if len(project.Repos) == 0 {
		return ""
	}
	return project.Repos[0].Name
}

func searchTextLimit(opts model.QueryOptions, pattern string, useRegex bool) int {
	limit := opts.MaxItems
	if limit <= 0 {
		limit = DefaultConfig().DefaultMaxItems
	}
	if useRegex || !isIdentifierLikeQuery(pattern) {
		return limit
	}
	if isAXIPreview(opts) {
		return max(limit, min(DefaultConfig().DefaultSearchLimit, max(limit*10, 50)))
	}
	return max(limit, DefaultConfig().DefaultSearchLimit)
}

func appendSearchDiagnosticsWarnings(warnings []string, diagnostics *searchPatternDiagnostics) []string {
	if diagnostics == nil {
		return warnings
	}
	if diagnostics.RipgrepFallbackCode != "" {
		warnings = append(warnings, fmt.Sprintf("text search backend failure (backend_runtime/%s): rg unavailable; served from go text fallback; verify MI_LSP_RG/PATH permissions", diagnostics.RipgrepFallbackCode))
	}
	if diagnostics.TimedOut {
		if diagnostics.PartialCount > 0 {
			warnings = append(warnings, fmt.Sprintf("search timed out; returned %d partial result(s); narrow with --repo or a more specific pattern", diagnostics.PartialCount))
		} else {
			warnings = append(warnings, "search timed out before matches; narrow with --repo or a more specific pattern")
		}
	}
	return warnings
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
		if !intentSafeRepoName(repo.Name) {
			return unpublishableRepoResolution()
		}
		return repoSelectorResolution{Repo: repo}
	}
	candidates := rankRepoCandidates(project, selector)
	if len(candidates) == 1 && candidates[0].Score >= 100 {
		if !intentSafeRepoName(candidates[0].Repo.Name) {
			return unpublishableRepoResolution()
		}
		return repoSelectorResolution{
			Repo:     candidates[0].Repo,
			Warnings: []string{fmt.Sprintf("repo_selector_resolved; candidate: %s", publicRepoCandidateName(candidates[0].Repo))},
		}
	}
	if len(candidates) > 0 {
		items := make([]map[string]any, 0, len(candidates))
		for _, candidate := range candidates {
			items = append(items, publicRepoCandidateItem(candidate))
		}
		warning := fmt.Sprintf("repo_selector_invalid; candidates: %s", strings.Join(publicRepoCandidateNames(candidates), ", "))
		next := "rerun with --repo " + publicRepoCandidateName(candidates[0].Repo)
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
	allCandidates := repoCandidates(project.Repos)
	if len(allCandidates) > 3 {
		allCandidates = allCandidates[:3]
	}
	return repoSelectorResolution{
		Envelope: ambiguityEnvelope(projectRegistrationHint(project), "repo_selector_invalid", allCandidates, "--repo <name>"),
	}
}

func unpublishableRepoResolution() repoSelectorResolution {
	next := "rerun with --repo <safe-name>"
	return repoSelectorResolution{Envelope: &model.Envelope{
		Ok:       false,
		Backend:  "router",
		Items:    []map[string]any{},
		Warnings: []string{"repo_selector_unpublishable"},
		NextHint: &next,
	}}
}

func publicRepoCandidateName(repo model.WorkspaceRepo) string {
	if !intentSafeRepoName(repo.Name) {
		return "__MI_LSP_REPO_CANDIDATE__"
	}
	return strings.TrimSpace(repo.Name)
}

func publicRepoCandidateItem(candidate repoCandidate) map[string]any {
	if !intentSafeRepoName(candidate.Repo.Name) {
		return map[string]any{
			"repo":         "__MI_LSP_REPO_CANDIDATE__",
			"match_reason": "repo_candidate_unpublishable",
		}
	}
	return map[string]any{
		"repo":               candidate.Repo.Name,
		"repo_id":            candidate.Repo.ID,
		"root":               candidate.Repo.Root,
		"default_entrypoint": candidate.Repo.DefaultEntrypoint,
		"match_reason":       candidate.Reason,
	}
}

func publicRepoCandidateNames(candidates []repoCandidate) []string {
	labels := make([]string, 0, minInt(len(candidates), 3))
	for _, candidate := range candidates {
		labels = append(labels, publicRepoCandidateName(candidate.Repo))
	}
	return labels
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
	var staleWarnings []string
	workspaces, staleWarnings = filterExistingWorkspaceRoots(workspaces)
	if len(workspaces) == 0 {
		return model.Envelope{Ok: true, Backend: "text", Items: []map[string]any{}, Warnings: append([]string{"no existing workspace roots registered"}, staleWarnings...)}, nil
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

			searchCtx, cancel := withSearchTimeout(ctx, a.Config.SearchTimeout)
			defer cancel()
			diagnostics := &searchPatternDiagnostics{}
			items, err := searchPatternScopedWithDiagnostics(searchCtx, wsReg.Root, wsReg.Root, project, pattern, useRegex, maxItems, diagnostics)
			if err != nil {
				results <- searchResult{ws: wsReg, err: err}
				return
			}

			for _, item := range items {
				item["workspace"] = wsReg.Name
			}

			warnings := appendSearchDiagnosticsWarnings(nil, diagnostics)
			if len(items) == 0 && !useRegex && looksRegexLikePattern(pattern) {
				warnings = append(warnings, fmt.Sprintf("%s: no literal matches; pattern looks regex-like, rerun with --regex", wsReg.Name))
			}

			results <- searchResult{ws: wsReg, items: items, warnings: warnings}
		}(ws)
	}

	wg.Wait()
	close(results)

	var allItems []map[string]any
	allWarnings := append([]string{}, staleWarnings...)

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
	var staleWarnings []string
	workspaces, staleWarnings = filterExistingWorkspaceRoots(workspaces)
	if len(workspaces) == 0 {
		return model.Envelope{Ok: true, Backend: "catalog", Items: []model.SymbolRecord{}, Warnings: append([]string{"no existing workspace roots registered"}, staleWarnings...)}, nil
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

			db, err := openWorkspaceDB(wsReg, "nav.find --all-workspaces", true) // readOnly
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

	return model.Envelope{Ok: true, Backend: "catalog", Items: allItems, Warnings: staleWarnings, Stats: model.Stats{Symbols: len(allItems)}}, nil
}

func filterExistingWorkspaceRoots(workspaces []model.WorkspaceRegistration) ([]model.WorkspaceRegistration, []string) {
	filtered := make([]model.WorkspaceRegistration, 0, len(workspaces))
	skipped := make([]string, 0)
	for _, ws := range workspaces {
		if _, err := os.Stat(ws.Root); err != nil {
			skipped = append(skipped, fmt.Sprintf("%s=%s", ws.Name, ws.Root))
			continue
		}
		filtered = append(filtered, ws)
	}
	if len(skipped) == 0 {
		return filtered, nil
	}
	const maxExamples = 5
	examples := skipped
	if len(examples) > maxExamples {
		examples = examples[:maxExamples]
	}
	warning := fmt.Sprintf("skipped %d registered workspace(s) with missing roots; run `mi-lsp workspace prune --stale --dry-run` to inspect", len(skipped))
	if len(examples) > 0 {
		warning += ": " + strings.Join(examples, ", ")
		if len(skipped) > len(examples) {
			warning += fmt.Sprintf(", +%d more", len(skipped)-len(examples))
		}
	}
	return filtered, []string{warning}
}
