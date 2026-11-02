package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

type graphProjectDelta struct {
	batch                 model.GraphObservationBatch
	applied, dependencies []string
}

// graphDeltaAdapters is scoped to one canonical planning pass. Membership and
// emission consume the SAME native result, avoiding a second compiler request
// that could observe different files. Nothing here is a global membership cache.
func (a *App) graphDeltaAdapters() (indexer.GraphDeltaMembership, indexer.GraphObserver, func() []string) {
	results := map[string]graphProjectDelta{}
	extra := map[string]bool{}
	key := func(r indexer.GraphObservationRequest) string {
		b, _ := json.Marshal([]string{r.WorkspaceRoot, r.RepoRoot, r.RepoID, r.EntrypointPath, r.ProjectOrModule})
		return string(b)
	}
	members := func(ctx context.Context, r indexer.GraphObservationRequest, paths []string) ([]string, error) {
		result, err := a.graphProjectDelta(ctx, r, paths)
		if err != nil {
			return nil, err
		}
		results[key(r)] = result
		for _, p := range result.dependencies {
			workspacePath, err := filepath.Rel(r.WorkspaceRoot, filepath.Join(r.RepoRoot, filepath.FromSlash(p)))
			if err != nil {
				return nil, err
			}
			normalized, err := indexer.GraphDeltaPathsForRoot(r.WorkspaceRoot, r.WorkspaceRoot, []string{filepath.ToSlash(workspacePath)}, "")
			if err != nil || len(normalized) != 1 {
				return nil, overlayGraphUnavailable("invalid Roslyn dependency owner")
			}
			extra[normalized[0]] = true
		}
		return append([]string{}, result.applied...), nil
	}
	observe := func(ctx context.Context, r indexer.GraphObservationRequest) (model.GraphObservationBatch, error) {
		result, ok := results[key(r)]
		if !ok || !sameGraphDeltaPaths(result.applied, r.AffectedPaths) {
			return model.GraphObservationBatch{}, overlayGraphUnavailable("Roslyn membership and observation do not share an exact receipt")
		}
		if r.AppliedPaths == nil {
			return model.GraphObservationBatch{}, overlayGraphUnavailable("Roslyn acknowledgement receiver is absent")
		}
		*r.AppliedPaths = append([]string{}, result.applied...)
		// Canonical rebasing mutates nested node slices. Do not mutate a cached
		// receipt or let another consumer change its observation in place.
		encoded, err := json.Marshal(result.batch)
		if err != nil {
			return model.GraphObservationBatch{}, err
		}
		var batch model.GraphObservationBatch
		if err := json.Unmarshal(encoded, &batch); err != nil {
			return batch, err
		}
		return batch, nil
	}
	dependencies := func() []string {
		paths := make([]string, 0, len(extra))
		for p := range extra {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		return paths
	}
	return members, observe, dependencies
}

func (a *App) graphProjectDelta(ctx context.Context, r indexer.GraphObservationRequest, paths []string) (graphProjectDelta, error) {
	fail := func() (graphProjectDelta, error) {
		return graphProjectDelta{}, overlayGraphUnavailable("Roslyn project partition receipt is incomplete or inconsistent")
	}
	if a.Semantic == nil || r.Backend != "roslyn" {
		return fail()
	}
	response, err := a.Semantic.Call(ctx, model.WorkspaceRegistration{Name: filepath.Base(r.WorkspaceRoot), Root: r.WorkspaceRoot}, model.WorkerRequest{
		ProtocolVersion: model.ProtocolVersion, Method: "graph_observe", Workspace: r.WorkspaceRoot, WorkspaceName: filepath.Base(r.WorkspaceRoot),
		BackendType: "roslyn", RepoID: r.RepoID, RepoName: r.RepoName, RepoRoot: r.RepoRoot,
		EntrypointID: r.EntrypointID, EntrypointPath: r.EntrypointPath, EntrypointType: r.EntrypointKind,
		Payload: map[string]any{"repository_identity": r.RepositoryIdentity, "project_or_module": r.ProjectOrModule, "affected_paths": paths, "partition_by_project": true},
	})
	if err != nil {
		return graphProjectDelta{}, err
	}
	if !response.Ok || response.Partial || response.Backend != "roslyn" || response.Observation == nil {
		return fail()
	}
	batch := *response.Observation
	if batch.RepositoryIdentity != r.RepositoryIdentity || batch.WorkspaceIdentity != r.WorkspaceIdentity || batch.ProjectOrModule != r.ProjectOrModule {
		return fail()
	}
	if err := batch.ReadyForStaging(); err != nil {
		return graphProjectDelta{}, err
	}
	var receipt map[string]any
	for _, item := range response.Items {
		if item["observation_scope"] != "affected_paths/v2" {
			continue
		}
		if receipt != nil {
			return fail()
		}
		receipt = item
	}
	if receipt == nil || receipt["project_or_module"] != r.ProjectOrModule || receipt["source_fingerprint"] != batch.SourceFingerprint.String() || receipt["config_fingerprint"] != batch.ConfigFingerprint.String() {
		return fail()
	}
	readPaths := func(name string) ([]string, bool) {
		value, ok := receipt[name]
		if !ok || value == nil {
			return nil, false
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, false
		}
		var result []string
		if json.Unmarshal(encoded, &result) != nil || result == nil {
			return nil, false
		}
		canonical, err := indexer.GraphDeltaPathsForRoot(r.RepoRoot, r.RepoRoot, result, "")
		if err != nil || !sameGraphDeltaPaths(result, canonical) {
			return nil, false
		}
		for _, p := range result {
			if filepath.ToSlash(filepath.Clean(filepath.FromSlash(p))) != p {
				return nil, false
			}
		}
		return result, true
	}
	wanted := map[string]bool{}
	for _, p := range paths {
		if wanted[p] {
			return fail()
		}
		wanted[p] = true
	}
	seen := map[string]bool{}
	var applied []string
	for _, name := range []string{"applied_paths", "missing_paths", "unowned_paths"} {
		partition, ok := readPaths(name)
		if !ok {
			return fail()
		}
		for _, p := range partition {
			if !wanted[p] || seen[p] {
				return fail()
			}
			seen[p] = true
		}
		if name == "applied_paths" {
			applied = partition
		}
	}
	if len(seen) != len(wanted) {
		return fail()
	}
	// V2 certifies semantic declaration closure. An older or malformed v2
	// actor omitting both fields is not evidence that the closure is empty.
	dependencies, ok := readPaths("dependency_paths")
	if !ok {
		return fail()
	}
	emitted, ok := readPaths("emitted_paths")
	if !ok || !sameGraphDeltaPaths(emitted, graphDeltaPathUnion(applied, dependencies)) {
		return fail()
	}
	return graphProjectDelta{batch: batch, applied: applied, dependencies: dependencies}, nil
}

// The singleflight owns this separate read-only pin. Cancelling the first
// request must not close a transaction still needed by another waiting query.
func (a *App) buildSelectiveWorktreeDelta(ctx context.Context, key string, registration model.WorkspaceRegistration, project model.ProjectFile, overlay workspace.WorktreeOverlay, baseID string, expectedTopology model.GraphTopologyWitness, worktreeTopologyDigest model.GraphDigest, initial []string, profileInput graphDocumentProfileInput, documentDomain []string) (*worktreeGraphCacheEntry, error) {
	db, err := store.OpenReadOnlyExistingWithWAL(overlay.BaseRoot, store.WorkspaceDBPath(overlay.BaseRoot))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	base, err := store.BeginGraphQuerySnapshot(ctx, db, baseID)
	if err != nil {
		return nil, err
	}
	defer base.Close()
	pinnedTopology, err := base.GraphTopologyWitness(ctx)
	if err != nil || pinnedTopology.Digest != expectedTopology.Digest || pinnedTopology.ProjectDigest != expectedTopology.ProjectDigest {
		return nil, overlayGraphTopologyIncomplete("pinned topology witness changed during selective graph construction")
	}
	pinProject := pinnedTopology.ProjectFile()
	affected := graphDeltaPathUnion(initial)
	previousGo, _, err := pinnedGoDeltaInputs(ctx, base, registration.Root, project, pinProject, pinnedTopology, affected)
	if err != nil {
		return nil, err
	}
	for round := 0; round < 16; round++ {
		// Non-nil empty native selectors preserve selective no-op semantics;
		// document-only edits must never fall back to a full native observation.
		nativeAffected, docAffected := []string{}, []string{}
		for _, path := range affected {
			if strings.EqualFold(filepath.Ext(path), ".md") {
				docAffected = append(docAffected, path)
			} else {
				nativeAffected = append(nativeAffected, path)
			}
		}
		priorPackages, previousPackageOwners, err := pinnedGoPackageOwners(ctx, base, registration.Root, project, pinProject, pinnedTopology, nativeAffected)
		if err != nil {
			return nil, err
		}
		members, observe, dependencies := a.graphDeltaAdapters()
		coverage := indexer.GraphDeltaCoverage{}
		batches, omissions, warnings, err := indexer.ObserveGraph(ctx, registration.Root, project, indexer.GraphIndexOptions{
			GoOffline: true, AffectedPaths: nativeAffected, DeltaCoverage: &coverage,
			GoDeltaObserver: a.observeSelectiveGoGraphRequest,
			RoslynObserver:  observe, RoslynDeltaMembers: members, PreviousGoModulePaths: previousGo, PreviousGoPackageOwners: previousPackageOwners,
		}, nil)
		if err != nil {
			return nil, err
		}
		candidatePackageOwners := make([]string, 0, len(priorPackages))
		candidateOwnerModules := map[string]map[string]bool{}
		addPackageOwner := func(owner, module string) {
			if candidateOwnerModules[owner] == nil {
				candidateOwnerModules[owner] = map[string]bool{}
			}
			candidateOwnerModules[owner][module] = true
			candidatePackageOwners = append(candidatePackageOwners, owner)
		}
		for _, owner := range priorPackages {
			addPackageOwner(owner.OwnerPath, owner.ProjectOrModule)
		}
		for _, batch := range batches {
			if batch.Backend != "go" {
				continue
			}
			for _, node := range batch.Nodes {
				if node.Key.SymbolKind == "package" && strings.HasPrefix(node.Key.OwnerPath, "@package/") {
					addPackageOwner(node.Key.OwnerPath, batch.ProjectOrModule)
				}
			}
		}
		pinnedPackageContexts, err := base.WorktreeGoPackageOwnerContexts(ctx, candidatePackageOwners)
		if err != nil {
			return nil, err
		}
		for _, owner := range pinnedPackageContexts {
			addPackageOwner(owner.OwnerPath, owner.ProjectOrModule)
		}
		for _, modules := range candidateOwnerModules {
			if len(modules) > 1 {
				return nil, overlayGraphUnavailable("ambiguous Go semantic package owner spans multiple modules")
			}
		}
		domains, err := verifyGoGraphDeltaBridges(ctx, registration.Root, base, project, pinProject, pinnedTopology, &coverage, batches)
		if err != nil {
			return nil, err
		}
		packageMasks, err := certifyGoPackageCoverage(registration.Root, &coverage, batches, priorPackages)
		if err != nil {
			return nil, err
		}
		controls, controlExpansion := certifyGoControlCoverage(registration.Root, &coverage, batches, previousGo)
		negative, err := certifyGoNegativeCoverage(ctx, base, registration.Root, &coverage, batches)
		if err != nil {
			return nil, err
		}
		noOpProofs, err := certifyGoPurePackageNoOps(ctx, base, registration.Root, &coverage, batches)
		if err != nil {
			return nil, err
		}
		noOpSelectors := make(map[string]bool, len(noOpProofs))
		for path := range noOpProofs {
			noOpSelectors[path] = true
		}
		for path := range controls {
			negative[path] = true
		}
		if err := attachGoWorktreeOwnerMasks(registration.Root, &coverage, domains, packageMasks); err != nil {
			return nil, err
		}
		if coverage.TopologyAmbiguous || len(coverage.Paths) != len(nativeAffected) {
			return nil, overlayGraphUnavailable("selective graph topology or path coverage is incomplete")
		}
		wanted := map[string]bool{}
		for _, path := range nativeAffected {
			wanted[path] = true
		}
		for _, path := range coverage.Paths {
			_, verifiedNoOp := noOpProofs[path.WorkspacePath]
			if !wanted[path.WorkspacePath] || path.Config || (len(path.Owners) == 0 && !verifiedNoOp) {
				return nil, overlayGraphUnavailable("selective graph has an uncertified affected owner")
			}
			delete(wanted, path.WorkspacePath)
			for _, owner := range path.Owners {
				if owner.ProjectOrModule == "" || (owner.Backend != "go" && owner.Backend != "roslyn") {
					return nil, overlayGraphUnavailable("selective graph owner has no canonical backend/module")
				}
				if _, err := worktreeRepoRelativePath(registration.Root, owner.RequestRoot); err != nil {
					return nil, overlayGraphUnavailable("selective graph owner root is outside the physical workspace")
				}
				workspacePath, err := filepath.Rel(registration.Root, filepath.Join(owner.RequestRoot, filepath.FromSlash(owner.RequestPath)))
				if err != nil || filepath.ToSlash(workspacePath) != path.WorkspacePath {
					return nil, overlayGraphUnavailable("selective graph owner coordinates do not match the pinned workspace")
				}
				// Go receipt paths and graph batches are canonicalized from the
				// exact planned request before coverage enters this workspace domain.
				if owner.Backend == "go" && filepath.Clean(owner.RequestRoot) != filepath.Clean(registration.Root) {
					mappedRoot, mapErr := worktreeRepoRelativePath(registration.Root, owner.RequestRoot)
					if mapErr != nil || filepath.ToSlash(filepath.Clean(mappedRoot)) == "." {
						return nil, overlayGraphUnavailable("Go request root has no proven workspace mapping")
					}
				}
			}
		}
		if len(wanted) != 0 {
			return nil, overlayGraphUnavailable("selective graph receipt omitted affected paths")
		}
		for _, omission := range omissions {
			if omission.Backend == "go" || omission.Backend == "roslyn" {
				return nil, overlayGraphUnavailable("selected worktree backend omitted source coverage")
			}
		}
		// Reconcile only the exact uncovered warning whose missing/excluded
		// compiler receipt was independently joined to pinned declarations.
		keptWarnings := warnings[:0]
		for _, warning := range warnings {
			const uncovered = "explicit_incomplete: uncovered affected path "
			uncoveredPath := strings.TrimPrefix(warning, uncovered)
			if strings.HasPrefix(warning, uncovered) && (negative[uncoveredPath] || noOpSelectors[uncoveredPath]) {
				continue
			}
			keptWarnings = append(keptWarnings, warning)
			if strings.HasPrefix(warning, "explicit_incomplete:") {
				return nil, overlayGraphUnavailable("selective graph owner coverage is incomplete")
			}
		}
		warnings = keptWarnings
		for i := range batches {
			if err := batches[i].ReadyForStaging(); err != nil {
				return nil, err
			}
		}
		var documents *store.GraphDocumentDelta
		relativeDocOwners := []string{}
		selectors := []string{}
		currentNativeOwners := map[string]bool{}
		for _, path := range graphDeltaPathUnion(nativeAffected, packageMasks) {
			currentNativeOwners[path] = true
		}
		for _, batch := range batches {
			for _, node := range batch.Nodes {
				if currentNativeOwners[node.Key.OwnerPath] {
					selectors = append(selectors, node.Key.OwnerPath, node.Key.SemanticIdentity)
				}
			}
		}
		if len(docAffected) > 0 {
			domain := make(map[string]bool, len(documentDomain))
			for _, path := range documentDomain {
				domain[path] = true
			}
			for _, path := range docAffected {
				if !domain[path] {
					return nil, overlayGraphUnavailable("document dependency escaped the captured canonical profile domain")
				}
			}
			pinnedProfile, err := base.WorktreeDocumentProfileContext(ctx)
			if err != nil {
				return nil, err
			}
			if pinnedProfile.DocumentCount > 0 && !pinnedProfile.Compatible {
				return nil, overlayGraphUnavailable("pinned document profile witness is incompatible")
			}
			delta, err := observeWorktreeGraphDocuments(ctx, base, registration.Root, docAffected, graphDeltaPathUnion(nativeAffected, packageMasks), batches, profileInput, time.Now())
			if err != nil {
				return nil, err
			}
			documents = &delta
			docOwners := map[string]bool{}
			for _, path := range docAffected {
				docOwners[path] = true
			}
			changedCandidates := []model.DocRecord{}
			for _, node := range delta.Bundle.Nodes {
				if node.Identity.BackendType == "docgraph" && docOwners[node.Identity.OwnerPath] {
					selectors = append(selectors, node.Identity.SemanticIdentity)
					changedCandidates = append(changedCandidates, model.DocRecord{Path: node.Identity.OwnerPath, DocID: node.Identity.SemanticIdentity})
				}
			}
			priorDocs, err := base.WorktreeDocumentContextNodes(ctx, docAffected, nil, 4096)
			if err != nil {
				return nil, err
			}
			for _, node := range priorDocs {
				selectors = append(selectors, node.Identity.SemanticIdentity)
				changedCandidates = append(changedCandidates, model.DocRecord{Path: node.Identity.OwnerPath, DocID: node.Identity.SemanticIdentity})
			}
			// A new target can make an old unique basename/DocID binding
			// ambiguous without having an incoming edge of its own. Seed the
			// pinned alternate targets too, then close their incoming owners.
			collisionEdges := []model.DocEdge{}
			collisionIDs := []string{}
			for _, doc := range changedCandidates {
				collisionEdges = append(collisionEdges, model.DocEdge{Kind: "doc_related", ToPath: doc.Path})
				if doc.DocID != "" {
					collisionIDs = append(collisionIDs, doc.DocID)
				}
			}
			collisionNames, fallbackIDs := docgraph.DocumentEdgeLookupPlan(collisionEdges)
			collisionPaths, err := base.WorktreeDocumentBasenamePaths(ctx, collisionNames, 4096)
			if err != nil {
				return nil, err
			}
			collisionNodes, err := base.WorktreeDocumentContextNodes(ctx, collisionPaths, graphDeltaPathUnion(collisionIDs, fallbackIDs), 4096)
			if err != nil {
				return nil, err
			}
			for _, node := range collisionNodes {
				relativeDocOwners = append(relativeDocOwners, node.Identity.OwnerPath)
			}
			// A missing/ambiguous relative target has no incoming edge. Test
			// old AND new candidate paths through the SAME canonical resolver.
			// This selects potential dependents only, never claims a binding:
			// those owners are reparsed against complete effective context.
			pathTargets, err := base.WorktreeDocumentPathTargets(ctx, 4096)
			if err != nil {
				return nil, err
			}
			lookupEdges := []model.DocEdge{}
			ownersBySource := map[string][]string{}
			for _, target := range pathTargets {
				source := target.SourceDocument
				if source == "" {
					source = target.OwnerPath
				}
				ownersBySource[source] = append(ownersBySource[source], target.OwnerPath)
				// Original relation is not in unresolved metadata. doc_related
				// conservatively includes canonical semantic-ID fallback too.
				lookupEdges = append(lookupEdges,
					model.DocEdge{FromPath: source, ToPath: target.TargetValue, Kind: "doc_related"},
					// Persisted document kind also includes exact IDs/TP refs.
					// Try both interpretations only for dependency selection;
					// complete-context reparse determines the actual relation.
					model.DocEdge{FromPath: source, ToDocID: target.TargetValue, Kind: "doc_related"})
			}
			for _, edge := range docgraph.ResolveDocEdges(changedCandidates, lookupEdges, profileInput.Profile) {
				if edge.UnresolvedReason != "missing_doc_target" {
					relativeDocOwners = append(relativeDocOwners, ownersBySource[edge.FromPath]...)
				}
			}
		}
		// Partial declaration owners may add files not in the original edit.
		// Recompute their typed dependents and observe the expanded mask until
		// stable; never hide new owners based on an incomplete first pass.
		expanded, err := base.WorktreeCodeDependencyPaths(ctx, graphDeltaPathUnion(affected, dependencies(), controlExpansion, packageMasks, relativeDocOwners))
		if err != nil {
			return nil, err
		}
		expanded, err = base.WorktreeDocumentDependencyPaths(ctx, expanded, graphDeltaPathUnion(selectors))
		if err != nil {
			return nil, err
		}
		// Synthetic owners participate in dependency invalidation and final
		// masking, but are never physical compiler selectors/file receipts.
		semantic := map[string]bool{}
		for _, path := range packageMasks {
			semantic[path] = true
		}
		physical := expanded[:0]
		for _, path := range expanded {
			if !semantic[path] {
				physical = append(physical, path)
			}
		}
		expanded = physical
		withoutNoOps := func(paths []string) []string {
			filtered := make([]string, 0, len(paths))
			for _, path := range paths {
				if !noOpSelectors[path] {
					filtered = append(filtered, path)
				}
			}
			return graphDeltaPathUnion(filtered)
		}
		effectiveExpanded := withoutNoOps(expanded)
		nextAffected := graphDeltaPathUnion(effectiveExpanded)
		for path := range noOpSelectors {
			nextAffected = graphDeltaPathUnion(nextAffected, []string{path})
		}
		if !sameGraphDeltaPaths(nextAffected, affected) {
			affected = nextAffected
			continue
		}
		// A verified no-record path remains a compiler/cache selector but is
		// excluded from the physical base mask; packageMasks remain semantic masks.
		expanded = effectiveExpanded
		current, linked, err := workspace.TryResolveWorktreeOverlay(registration.Root)
		if err != nil || !linked || current.Fingerprint != overlay.Fingerprint {
			return nil, overlayGraphUnavailable("worktree changed during selective graph observation")
		}
		return &worktreeGraphCacheEntry{key: key, generation: baseID, baseTopologyDigest: pinnedTopology.Digest, worktreeTopologyDigest: worktreeTopologyDigest, profileFingerprint: profileInput.Fingerprint, batches: batches, domains: domains, documents: documents, affected: graphDeltaPathUnion(expanded), warnings: warnings}, nil
	}
	return nil, overlayGraphUnavailable("worktree semantic owner closure did not converge within its bound")
}

type graphGoPinRequest struct {
	repo            model.WorkspaceRepo
	requestRoot     string
	physicalPrefix  string
	projectOrModule string
	physicalModule  string
}

func graphGoPinRequests(root string, project, pinProject model.ProjectFile, topology model.GraphTopologyWitness) ([]graphGoPinRequest, error) {
	if project.Project.Kind != pinProject.Project.Kind {
		return nil, overlayGraphUnavailable("worktree and pinned project kinds differ")
	}
	selections, err := worktreeGraphSelections(root, project)
	if err != nil {
		return nil, err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	var requests []graphGoPinRequest
	for _, selection := range selections {
		if selection.language != "go" {
			continue
		}
		prefix, err := worktreeRepoRelativePath(root, selection.repo.Root)
		if err != nil {
			return nil, overlayGraphUnavailable("Go repository root is not a safe workspace-relative pin")
		}
		var pinned []model.WorkspaceRepo
		for _, repo := range pinProject.Repos {
			if repo.ID == selection.repo.ID {
				pinned = append(pinned, repo)
			}
		}
		if len(pinned) != 1 {
			return nil, overlayGraphUnavailable("Go repository is not uniquely declared by the pinned project")
		}
		pinnedPrefix, err := worktreeRepoRelativePath(root, pinned[0].Root)
		if err != nil || pinnedPrefix != prefix || (pinned[0].RepositoryIdentity != "" && selection.repo.RepositoryIdentity != "" && pinned[0].RepositoryIdentity != selection.repo.RepositoryIdentity) {
			return nil, overlayGraphUnavailable("Go repository physical root differs from pinned project metadata")
		}
		requestRoot := rootAbs
		if project.Project.Kind != model.WorkspaceKindContainer {
			requestRoot = filepath.Join(rootAbs, filepath.FromSlash(prefix))
		}
		requestPrefix, err := filepath.Rel(rootAbs, requestRoot)
		if err != nil || requestPrefix == ".." || strings.HasPrefix(requestPrefix, ".."+string(filepath.Separator)) || filepath.IsAbs(requestPrefix) {
			return nil, overlayGraphUnavailable("Go request root escapes the physical workspace")
		}
		moduleAbs := filepath.Join(rootAbs, filepath.FromSlash(selection.entrypoint.Path))
		module, err := filepath.Rel(requestRoot, moduleAbs)
		if err != nil || module == ".." || strings.HasPrefix(module, ".."+string(filepath.Separator)) || filepath.IsAbs(module) || filepath.Base(module) != "go.mod" {
			return nil, overlayGraphUnavailable("Go module has no unambiguous request-root coordinate")
		}
		physicalModule, err := filepath.Rel(rootAbs, moduleAbs)
		if err != nil || physicalModule == ".." || strings.HasPrefix(physicalModule, ".."+string(filepath.Separator)) || filepath.IsAbs(physicalModule) {
			return nil, overlayGraphUnavailable("Go module escapes the physical workspace")
		}
		request := graphGoPinRequest{repo: selection.repo, requestRoot: requestRoot, physicalPrefix: filepath.ToSlash(filepath.Clean(requestPrefix)), projectOrModule: filepath.ToSlash(module), physicalModule: filepath.ToSlash(physicalModule)}
		witnessMatches := 0
		for _, witnessed := range topology.Modules {
			if witnessed.RequestRoot != request.physicalPrefix || witnessed.ProjectOrModule != request.projectOrModule || witnessed.GoModPath != request.physicalModule || witnessed.SelectorPath != request.physicalModule {
				continue
			}
			selectorMatches := false
			switch witnessed.SelectorKind {
			case "implicit_repo":
				selectorMatches = witnessed.SelectorID == "implicit:go.mod" && request.projectOrModule == "go.mod"
			case "container_root":
				selectorMatches = witnessed.SelectorID == "workspace-root:go.mod" && request.projectOrModule == "go.mod"
			case "explicit":
				selectorMatches = witnessed.SelectorID == selection.entrypoint.ID || witnessed.SelectorID == selection.entrypoint.Path
			case "configured":
				selectorMatches = witnessed.SelectorID == selection.repo.DefaultEntrypoint || witnessed.SelectorID == selection.entrypoint.ID || witnessed.SelectorID == selection.entrypoint.Path
			}
			if !selectorMatches {
				continue
			}
			for _, repoID := range witnessed.RepositoryIDs {
				if repoID == selection.repo.ID {
					witnessMatches++
				}
			}
		}
		if witnessMatches != 1 {
			return nil, overlayGraphTopologyIncomplete("selected Go module has no exact generation-owned topology witness")
		}
		requests = append(requests, request)
	}
	if len(requests) == 0 {
		return nil, overlayGraphUnavailable("pinned project has no Go request domain")
	}
	sort.Slice(requests, func(i, j int) bool {
		if requests[i].requestRoot != requests[j].requestRoot {
			return requests[i].requestRoot < requests[j].requestRoot
		}
		return requests[i].projectOrModule < requests[j].projectOrModule
	})
	return requests, nil
}

func requestPathForWorkspace(root, requestRoot, workspacePath string) (string, bool) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	requestAbs, err := filepath.Abs(requestRoot)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(requestAbs, filepath.Join(rootAbs, filepath.FromSlash(workspacePath)))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return filepath.ToSlash(filepath.Clean(rel)), true
}

func pinnedGoDeltaInputs(ctx context.Context, base *store.GraphQuerySnapshot, root string, project, pinProject model.ProjectFile, topology model.GraphTopologyWitness, paths []string) (map[string][]string, map[string][]string, error) {
	requests, err := graphGoPinRequests(root, project, pinProject, topology)
	if err != nil {
		return nil, nil, err
	}
	previousGo := map[string][]string{}
	previousPackages := map[string][]string{}
	for _, workspacePath := range paths {
		for _, request := range requests {
			raw, inside := requestPathForWorkspace(root, request.requestRoot, workspacePath)
			if !inside {
				continue
			}
			if filepath.Base(workspacePath) == "go.mod" || filepath.Base(workspacePath) == "go.sum" {
				moduleDir := filepath.ToSlash(filepath.Dir(request.projectOrModule))
				control := filepath.ToSlash(filepath.Join(moduleDir, filepath.Base(workspacePath)))
				if raw == control || raw == request.projectOrModule {
					owners, err := base.WorktreeContextOwnerPaths(ctx, "go", request.projectOrModule)
					if err != nil {
						return nil, nil, err
					}
					for _, owner := range owners {
						physical := filepath.ToSlash(filepath.Join(request.physicalPrefix, filepath.FromSlash(owner)))
						previousGo[request.physicalModule] = append(previousGo[request.physicalModule], physical)
					}
				}
			}
			if !strings.EqualFold(filepath.Ext(workspacePath), ".go") {
				continue
			}
			owners, err := base.WorktreeGoPackageOwners(ctx, []string{raw})
			if err != nil {
				return nil, nil, err
			}
			for _, owner := range owners {
				for _, candidate := range requests {
					if candidate.requestRoot == request.requestRoot && candidate.projectOrModule == owner.ProjectOrModule {
						previousPackages[candidate.physicalModule] = append(previousPackages[candidate.physicalModule], owner.OwnerPath)
						break
					}
				}
			}
		}
	}
	for module := range previousGo {
		previousGo[module] = graphDeltaPathUnion(previousGo[module])
	}
	for module := range previousPackages {
		previousPackages[module] = graphDeltaPathUnion(previousPackages[module])
	}
	return previousGo, previousPackages, nil
}

func pinnedGoPackageOwners(ctx context.Context, base *store.GraphQuerySnapshot, root string, project, pinProject model.ProjectFile, topology model.GraphTopologyWitness, paths []string) ([]store.GraphWorktreePackageOwner, map[string][]string, error) {
	requests, err := graphGoPinRequests(root, project, pinProject, topology)
	if err != nil {
		return nil, nil, err
	}
	seen := map[string]bool{}
	var prior []store.GraphWorktreePackageOwner
	previous := map[string][]string{}
	for _, workspacePath := range paths {
		if !strings.EqualFold(filepath.Ext(workspacePath), ".go") {
			continue
		}
		for _, request := range requests {
			raw, inside := requestPathForWorkspace(root, request.requestRoot, workspacePath)
			if !inside {
				continue
			}
			owners, err := base.WorktreeGoPackageOwners(ctx, []string{raw})
			if err != nil {
				return nil, nil, err
			}
			for _, owner := range owners {
				matched := false
				for _, candidate := range requests {
					if candidate.requestRoot == request.requestRoot && candidate.projectOrModule == owner.ProjectOrModule {
						key := owner.ProjectOrModule + "\x00" + owner.OwnerPath
						if !seen[key] {
							prior = append(prior, owner)
							seen[key] = true
						}
						previous[candidate.physicalModule] = append(previous[candidate.physicalModule], owner.OwnerPath)
						matched = true
						break
					}
				}
				if !matched {
					return nil, nil, overlayGraphUnavailable("pinned Go package owner has no matching request-root module")
				}
			}
		}
	}
	for module := range previous {
		previous[module] = graphDeltaPathUnion(previous[module])
	}
	sort.Slice(prior, func(i, j int) bool {
		if prior[i].ProjectOrModule != prior[j].ProjectOrModule {
			return prior[i].ProjectOrModule < prior[j].ProjectOrModule
		}
		return prior[i].OwnerPath < prior[j].OwnerPath
	})
	return prior, previous, nil
}

// verifyGoGraphDeltaBridges joins native request roots and raw NodeKeys to
// package identities already present in the exact pinned generation.
func verifyGoGraphDeltaBridges(ctx context.Context, root string, base *store.GraphQuerySnapshot, project, pinProject model.ProjectFile, topology model.GraphTopologyWitness, coverage *indexer.GraphDeltaCoverage, batches []model.GraphObservationBatch) ([]model.GraphWorktreeDomain, error) {
	workspaceRoot, rootErr := filepath.Abs(root)
	requests, requestErr := graphGoPinRequests(root, project, pinProject, topology)
	if rootErr != nil || requestErr != nil || coverage == nil || len(coverage.GoReceipts) != len(coverage.GoReceiptBridges) {
		return nil, overlayGraphUnavailable("Go receipt is missing its pinned graph-domain proof")
	}
	generation, err := base.Generation()
	if err != nil {
		return nil, overlayGraphUnavailable("pinned graph generation identity is unavailable")
	}
	var domains []model.GraphWorktreeDomain
	seen := map[string]bool{}
	for _, receipt := range coverage.GoReceipts {
		matched := 0
		for _, bridge := range coverage.GoReceiptBridges {
			if bridge.RawRoot != receipt.Root || bridge.RawProjectOrModule != receipt.ProjectOrModule {
				continue
			}
			matched++
			rawRoot, err := filepath.Abs(bridge.RawRoot)
			if err != nil || filepath.Clean(bridge.CanonicalRoot) != filepath.Clean(workspaceRoot) || filepath.Clean(rawRoot) != filepath.Clean(filepath.Clean(receipt.Root)) || bridge.CanonicalProjectOrModule != bridge.RawProjectOrModule || bridge.PhysicalProjectOrModule == "" || bridge.RawBatchDigest != bridge.CanonicalBatchDigest || bridge.RawSourceFingerprint != bridge.CanonicalSourceFingerprint || bridge.ConfigFingerprint == (model.GraphDigest{}) || bridge.ScopeFingerprint != receipt.ScopeFingerprint || bridge.CompletenessDigest != receipt.List.ResultDigest || !reflect.DeepEqual(bridge.CanonicalReceipt, receipt) {
				return nil, overlayGraphUnavailable("Go receipt bridge does not preserve its raw key domain")
			}
			var request *graphGoPinRequest
			for i := range requests {
				candidate := &requests[i]
				candidateRoot, err := filepath.Abs(candidate.requestRoot)
				if err == nil && filepath.Clean(candidateRoot) == filepath.Clean(rawRoot) && candidate.projectOrModule == receipt.ProjectOrModule {
					request = candidate
					break
				}
			}
			if request == nil || bridge.Domain.ProjectOrModule != request.projectOrModule || bridge.Domain.PhysicalPrefix != request.physicalPrefix || bridge.Domain.PhysicalModule != request.physicalModule || bridge.PhysicalProjectOrModule != request.physicalModule {
				return nil, overlayGraphUnavailable("Go raw request root has no exact pinned repository mapping")
			}
			for _, mapping := range bridge.PathMappings {
				want := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.FromSlash(request.physicalPrefix), filepath.FromSlash(mapping.Raw))))
				if mapping.Raw == "." {
					want = request.physicalPrefix
				}
				if mapping.Canonical != want {
					return nil, overlayGraphUnavailable("Go receipt path mapping does not match its physical workspace projection")
				}
			}
			var observed *model.GraphObservationBatch
			batchMatches := 0
			for i := range batches {
				batch := &batches[i]
				if batch.Backend == "go" && batch.ProjectOrModule == receipt.ProjectOrModule && batch.Digest == bridge.CanonicalBatchDigest && batch.SourceFingerprint == bridge.CanonicalSourceFingerprint && batch.ConfigFingerprint == bridge.ConfigFingerprint && batch.WorkspaceIdentity == bridge.WorkspaceIdentity && batch.RepositoryIdentity == bridge.RepositoryIdentity && batch.ReadyForStaging() == nil {
					observed = batch
					batchMatches++
				}
			}
			if batchMatches != 1 || observed == nil || generation.WorkspaceIdentity != bridge.WorkspaceIdentity || generation.RepositoryIdentity != bridge.RepositoryIdentity || bridge.Domain.WorkspaceIdentity != bridge.WorkspaceIdentity || bridge.Domain.RepositoryIdentity != bridge.RepositoryIdentity {
				return nil, overlayGraphUnavailable("Go canonical bridge is not bound to one ready batch in the pinned graph identity")
			}
			pinnedKeys, err := base.WorktreeGoPackageNodeKeysInDomain(ctx, bridge.RepositoryIdentity, receipt.ProjectOrModule)
			if err != nil {
				return nil, overlayGraphUnavailable("pinned Go package NodeKey witnesses are unavailable")
			}
			domain := bridge.Domain
			domain.PinGenerationID = generation.GenerationID
			domain.TopologyDigest = topology.Digest
			domain.PinnedPackageNodeKeys = append(domain.PinnedPackageNodeKeys, pinnedKeys...)
			sort.Slice(domain.PinnedPackageNodeKeys, func(i, j int) bool {
				return domain.PinnedPackageNodeKeys[i].String() < domain.PinnedPackageNodeKeys[j].String()
			})
			if err := domain.Validate(); err != nil {
				return nil, overlayGraphUnavailable("Go pinned key-domain proof is incomplete")
			}
			domainKey := domain.ProjectOrModule + "\x00" + domain.PhysicalModule
			if seen[domainKey] {
				return nil, overlayGraphUnavailable("Go pinned key domain is duplicated")
			}
			seen[domainKey] = true
			domains = append(domains, domain)
		}
		if matched != 1 {
			return nil, overlayGraphUnavailable("Go receipt has no unique pinned graph-domain bridge")
		}
	}
	return domains, nil
}

func attachGoWorktreeOwnerMasks(root string, coverage *indexer.GraphDeltaCoverage, domains []model.GraphWorktreeDomain, packageMasks []string) error {
	if coverage == nil {
		return overlayGraphUnavailable("Go worktree owner coverage is absent")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return overlayGraphUnavailable("physical workspace root is unavailable")
	}
	packageMaskSet := map[string]bool{}
	for _, owner := range packageMasks {
		packageMaskSet[owner] = true
	}
	for i := range domains {
		domain := &domains[i]
		requestRoot := filepath.Join(rootAbs, filepath.FromSlash(domain.PhysicalPrefix))
		if domain.PhysicalPrefix == "." {
			requestRoot = rootAbs
		}
		masks := map[string]model.GraphWorktreeOwnerMask{}
		for _, path := range coverage.Paths {
			for _, owner := range path.Owners {
				if owner.Backend != "go" || owner.ProjectOrModule != domain.ProjectOrModule || !strings.EqualFold(filepath.Ext(owner.RequestPath), ".go") {
					continue
				}
				ownerRoot, err := filepath.Abs(owner.RequestRoot)
				if err != nil || filepath.Clean(ownerRoot) != filepath.Clean(requestRoot) {
					continue
				}
				physical, err := filepath.Rel(rootAbs, filepath.Join(ownerRoot, filepath.FromSlash(owner.RequestPath)))
				if err != nil || filepath.ToSlash(filepath.Clean(physical)) != path.WorkspacePath {
					return overlayGraphUnavailable("Go source owner mask does not match its physical workspace path")
				}
				mask := model.GraphWorktreeOwnerMask{Kind: "source", OwnerPath: filepath.ToSlash(filepath.Clean(owner.RequestPath)), WorkspacePath: path.WorkspacePath}
				masks[mask.Kind+"\x00"+mask.OwnerPath+"\x00"+mask.WorkspacePath] = mask
			}
		}
		for _, certificate := range coverage.GoPackageCertificates {
			if certificate.Module != domain.ProjectOrModule || !packageMaskSet[certificate.Owner] || !strings.HasPrefix(certificate.Owner, "@package/") {
				continue
			}
			key, err := model.NewNodeKey(model.NodeKey{RepositoryIdentity: domain.RepositoryIdentity, BackendType: "go", Language: "go", ProjectOrModule: domain.ProjectOrModule, OwnerPath: certificate.Owner, SymbolKind: "package", SemanticIdentity: "pkg:" + certificate.ImportPath})
			if err != nil {
				return overlayGraphUnavailable("Go package mask has no canonical NodeKey")
			}
			digest, err := key.Hash()
			if err != nil {
				return overlayGraphUnavailable("Go package mask has no canonical digest")
			}
			pinned := false
			for _, candidate := range domain.PinnedPackageNodeKeys {
				if candidate == digest {
					pinned = true
					break
				}
			}
			if pinned {
				mask := model.GraphWorktreeOwnerMask{Kind: "package", OwnerPath: certificate.Owner}
				masks[mask.Kind+"\x00"+mask.OwnerPath] = mask
			}
		}
		domain.OwnerMasks = domain.OwnerMasks[:0]
		for _, mask := range masks {
			domain.OwnerMasks = append(domain.OwnerMasks, mask)
		}
		sort.Slice(domain.OwnerMasks, func(i, j int) bool {
			if domain.OwnerMasks[i].Kind != domain.OwnerMasks[j].Kind {
				return domain.OwnerMasks[i].Kind < domain.OwnerMasks[j].Kind
			}
			if domain.OwnerMasks[i].OwnerPath != domain.OwnerMasks[j].OwnerPath {
				return domain.OwnerMasks[i].OwnerPath < domain.OwnerMasks[j].OwnerPath
			}
			return domain.OwnerMasks[i].WorkspacePath < domain.OwnerMasks[j].WorkspacePath
		})
		if err := domain.Validate(); err != nil {
			return overlayGraphUnavailable("Go worktree owner masks do not match their pinned domain")
		}
	}
	return nil
}

func goReceiptBridgeForWorkspace(root string, coverage *indexer.GraphDeltaCoverage, receipt indexer.GoGraphDeltaReceipt) (indexer.GoGraphDeltaCanonicalBridge, bool) {
	workspaceRoot, err := filepath.Abs(root)
	if err != nil {
		return indexer.GoGraphDeltaCanonicalBridge{}, false
	}
	var found indexer.GoGraphDeltaCanonicalBridge
	matches := 0
	for _, bridge := range coverage.GoReceiptBridges {
		rawRoot, rawErr := filepath.Abs(bridge.RawRoot)
		canonicalRoot, canonicalErr := filepath.Abs(bridge.CanonicalRoot)
		if rawErr == nil && canonicalErr == nil && filepath.Clean(rawRoot) == filepath.Clean(receipt.Root) && filepath.Clean(canonicalRoot) == filepath.Clean(workspaceRoot) && bridge.RawProjectOrModule == receipt.ProjectOrModule {
			found = bridge
			matches++
		}
	}
	return found, matches == 1
}

// Package owners are semantic masks, never file membership. Bind every mask
// to the same successful native result and a real package node; old package
// owners without a replacement/absence certificate remain explicitly unsafe.
func certifyGoPackageCoverage(root string, coverage *indexer.GraphDeltaCoverage, batches []model.GraphObservationBatch, prior []store.GraphWorktreePackageOwner) ([]string, error) {
	approved := map[string]bool{}
	var masks []string
	for _, certificate := range coverage.GoPackageCertificates {
		if (certificate.State != "applied" && certificate.State != "absence-confirmed") || certificate.ImportPath == "" || certificate.Owner != "@package/"+certificate.ImportPath {
			continue
		}
		for _, receipt := range coverage.GoReceipts {
			if _, ok := goReceiptBridgeForWorkspace(root, coverage, receipt); !ok || receipt.ProjectOrModule != certificate.Module {
				continue
			}
			rawMatch := false
			for _, raw := range receipt.PackageOwnerCertificates {
				if raw == certificate {
					rawMatch = true
					break
				}
			}
			if !rawMatch {
				continue
			}
			for _, batch := range batches {
				if batch.Backend != "go" || batch.ProjectOrModule != receipt.ProjectOrModule || batch.ConfigFingerprint != receipt.ConfigFingerprint || batch.ReadyForStaging() != nil {
					continue
				}
				if certificate.State == "absence-confirmed" {
					provenance := false
					for _, old := range prior {
						if old.ProjectOrModule == certificate.Module && old.OwnerPath == certificate.Owner {
							provenance = true
							break
						}
					}
					inNamespace := certificate.ImportPath == receipt.ModulePath || strings.HasPrefix(certificate.ImportPath, receipt.ModulePath+"/")
					listed := false
					for _, path := range receipt.ListedImportPaths {
						if path == certificate.ImportPath {
							listed = true
							break
						}
					}
					proof := receipt.List
					if provenance && inNamespace && !listed && receipt.ModulePath != "" && filepath.Clean(proof.Dir) == filepath.Clean(receipt.ModuleRoot) && filepath.Clean(receipt.ModuleRoot) == filepath.Clean(filepath.Dir(filepath.Join(receipt.Root, filepath.FromSlash(certificate.Module)))) && receipt.LocalScopeComplete && proof.Command == "go" && proof.State == "complete" && proof.Complete && proof.ExitCode == 0 && proof.StdoutEOF && proof.JSONValid && proof.InvocationDigest != (model.GraphDigest{}) && receipt.ScopeFingerprint != (model.GraphDigest{}) && certificate.SourceDigest == (model.GraphDigest{}) && certificate.ScopeFingerprint == receipt.ScopeFingerprint && certificate.CompletenessDigest == proof.ResultDigest && proof.ResultDigest != (model.GraphDigest{}) {
						approved[certificate.Module+"\x00"+certificate.Owner] = true
						masks = append(masks, certificate.Owner)
					}
					continue
				}
				for _, node := range batch.Nodes {
					if node.Key.BackendType == "go" && node.Key.SymbolKind == "package" && node.Key.OwnerPath == certificate.Owner && node.Key.ProjectOrModule == certificate.Module && node.Key.SemanticIdentity == "pkg:"+certificate.ImportPath && node.SourceDigest == certificate.SourceDigest {
						approved[certificate.Module+"\x00"+certificate.Owner] = true
						masks = append(masks, certificate.Owner)
					}
				}
			}
		}
	}
	for _, owner := range prior {
		if !approved[owner.ProjectOrModule+"\x00"+owner.OwnerPath] {
			return nil, overlayGraphUnavailable("prior Go package has no certified replacement or absence proof")
		}
	}
	return graphDeltaPathUnion(masks), nil
}

// A filename is only a candidate. Certify controls against a consumed-input
// receipt bound to a successful stageable compiler batch and the exact root.
func certifyGoControlCoverage(root string, coverage *indexer.GraphDeltaCoverage, batches []model.GraphObservationBatch, previous map[string][]string) (map[string]bool, []string) {
	certified := map[string]bool{}
	var expansion []string
	for _, control := range coverage.GoControlCertificates {
		if control.ExpansionReason == "" {
			continue
		}
		matchedControl := false
		for _, receipt := range coverage.GoReceipts {
			bridge, bridgeOK := goReceiptBridgeForWorkspace(root, coverage, receipt)
			if !bridgeOK || bridge.PhysicalProjectOrModule != control.Module {
				continue
			}
			bound := false
			for _, batch := range batches {
				if batch.Backend == "go" && batch.ProjectOrModule == receipt.ProjectOrModule && batch.ConfigFingerprint == receipt.ConfigFingerprint && batch.ReadyForStaging() == nil {
					bound = true
					break
				}
			}
			if !bound {
				continue
			}
			if len(control.RawInputs) != len(control.Inputs) {
				continue
			}
			for i, input := range control.Inputs {
				rawInput := control.RawInputs[i]
				present := input.Present && input.Consumed && input.State == "consumed"
				absent := !input.Present && !input.Consumed && input.State == "absence-confirmed" && filepath.Base(input.Path) == "go.sum"
				if (!present && !absent) || input.Digest == (model.GraphDigest{}) {
					continue
				}
				verified := false
				for _, raw := range receipt.ControlInputs {
					if raw == rawInput {
						verified = true
						break
					}
				}
				if !verified {
					continue
				}
				for i := range coverage.Paths {
					path := &coverage.Paths[i]
					if path.WorkspacePath != input.Path || !path.Config {
						continue
					}
					path.Config = false
					path.Owners = []indexer.GraphDeltaOwner{{Backend: "go", RequestRoot: receipt.Root, ProjectOrModule: receipt.ProjectOrModule, RequestPath: rawInput.Path}}
					certified[input.Path], matchedControl = true, true
				}
			}
		}
		if matchedControl {
			expansion = graphDeltaPathUnion(expansion, control.ExpandedPaths, previous[control.Module])
		}
	}
	return certified, expansion
}

// GraphPathNoOpProof is deliberately separate from owner/mask state: an empty
// pinned footprint plus a ready current package replacement is a no-record no-op,
// never evidence of old physical membership or a tombstone.
type GraphPathNoOpProof struct {
	Path, Module, PackageOwner string
	Generation                 model.GraphDigest
	SourceDigest               model.GraphDigest
}

func certifyGoPurePackageNoOps(ctx context.Context, base *store.GraphQuerySnapshot, root string, coverage *indexer.GraphDeltaCoverage, batches []model.GraphObservationBatch) (map[string]GraphPathNoOpProof, error) {
	proofs := map[string]GraphPathNoOpProof{}
	generation, err := base.Generation()
	if err != nil {
		return nil, err
	}
selectedLoop:
	for _, selected := range coverage.Paths {
		if selected.Config || len(selected.Owners) != 0 || !strings.EqualFold(filepath.Ext(selected.WorkspacePath), ".go") {
			continue
		}
		for _, receipt := range coverage.GoReceipts {
			bridge, bridgeOK := goReceiptBridgeForWorkspace(root, coverage, receipt)
			if !bridgeOK || !receipt.LocalScopeComplete || receipt.ProjectOrModule == "" || !receipt.List.Complete || receipt.List.State != "complete" || receipt.List.ExitCode != 0 || !receipt.List.StdoutEOF || !receipt.List.JSONValid || receipt.List.InvocationDigest == (model.GraphDigest{}) || receipt.List.ResultDigest == (model.GraphDigest{}) || receipt.ScopeFingerprint == (model.GraphDigest{}) {
				continue
			}
			rawSelectedPath, inside := requestPathForWorkspace(root, receipt.Root, selected.WorkspacePath)
			if !inside {
				continue
			}
			hasRecords, err := base.WorktreeGraphPathHasRecords(ctx, rawSelectedPath)
			if err != nil {
				return nil, err
			}
			if hasRecords {
				continue
			}
			moduleRoot := filepath.Clean(receipt.ModuleRoot)
			wantModuleRoot := filepath.Clean(filepath.Dir(filepath.Join(receipt.Root, filepath.FromSlash(receipt.ProjectOrModule))))
			if moduleRoot != wantModuleRoot || filepath.Clean(receipt.List.Dir) != moduleRoot {
				continue
			}
			for _, missing := range receipt.Missing {
				workspacePath := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.FromSlash(bridge.Domain.PhysicalPrefix), filepath.FromSlash(missing))))
				if workspacePath != selected.WorkspacePath {
					continue
				}
				for _, current := range receipt.CurrentPackages {
					packageDir := filepath.Clean(filepath.Join(receipt.Root, filepath.FromSlash(current.Dir)))
					selectedDir := filepath.Clean(filepath.Dir(filepath.Join(receipt.Root, filepath.FromSlash(rawSelectedPath))))
					if packageDir != selectedDir || current.ImportPath == "" {
						continue
					}
					owner := "@package/" + current.ImportPath
					for _, cert := range receipt.PackageOwnerCertificates {
						if cert.State != "applied" || cert.Owner != owner || cert.ImportPath != current.ImportPath || cert.Module != receipt.ProjectOrModule || cert.SourceDigest == (model.GraphDigest{}) || cert.ScopeFingerprint != receipt.ScopeFingerprint || cert.CompletenessDigest != receipt.List.ResultDigest {
							continue
						}
						for _, batch := range batches {
							if batch.Backend != "go" || batch.ProjectOrModule != receipt.ProjectOrModule || batch.ConfigFingerprint != receipt.ConfigFingerprint || batch.ReadyForStaging() != nil {
								continue
							}
							for _, node := range batch.Nodes {
								if node.Key.BackendType == "go" && node.Key.SymbolKind == "package" && node.Key.OwnerPath == owner && node.Key.ProjectOrModule == receipt.ProjectOrModule && node.Key.SemanticIdentity == "pkg:"+current.ImportPath && node.SourceDigest == cert.SourceDigest {
									proofs[selected.WorkspacePath] = GraphPathNoOpProof{Path: selected.WorkspacePath, Module: receipt.ProjectOrModule, PackageOwner: owner, Generation: generation.GenerationID, SourceDigest: cert.SourceDigest}
									continue selectedLoop
								}
							}
						}
					}
				}
			}
		}
	}
	return proofs, nil
}

// Negative receipts are not ownership. Join every actual declaration context
// in the pinned base to a successful compiler receipt before authorizing a Go
// mask. No-declaration/other-backend paths remain explicitly uncertified here.
func certifyGoNegativeCoverage(ctx context.Context, base *store.GraphQuerySnapshot, root string, coverage *indexer.GraphDeltaCoverage, batches []model.GraphObservationBatch) (map[string]bool, error) {
	certified := map[string]bool{}
	contains := func(paths []string, path string) bool {
		for _, p := range paths {
			if p == path {
				return true
			}
		}
		return false
	}
	for i := range coverage.Paths {
		path := &coverage.Paths[i]
		if path.Config || len(path.Owners) != 0 {
			continue
		}
		type ownerKey struct{ backend, module, owner string }
		oldByKey := map[ownerKey]store.GraphWorktreeDeclaredOwner{}
		for _, receipt := range coverage.GoReceipts {
			bridge, bridgeOK := goReceiptBridgeForWorkspace(root, coverage, receipt)
			if !bridgeOK {
				continue
			}
			rawPath, inside := requestPathForWorkspace(root, receipt.Root, path.WorkspacePath)
			if !inside {
				continue
			}
			physical := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.FromSlash(bridge.Domain.PhysicalPrefix), filepath.FromSlash(rawPath))))
			if physical != path.WorkspacePath {
				continue
			}
			old, err := base.WorktreeDeclaredCodeOwners(ctx, rawPath)
			if err != nil {
				return nil, err
			}
			for _, prior := range old {
				key := ownerKey{backend: prior.Backend, module: prior.ProjectOrModule, owner: rawPath}
				oldByKey[key] = prior
			}
		}
		if len(oldByKey) == 0 {
			continue
		}
		var owners []indexer.GraphDeltaOwner
		for key := range oldByKey {
			if key.backend != "go" {
				owners = nil
				break
			}
			matched := false
			for _, receipt := range coverage.GoReceipts {
				bridge, bridgeOK := goReceiptBridgeForWorkspace(root, coverage, receipt)
				if !bridgeOK || receipt.ProjectOrModule != key.module {
					continue
				}
				rawPath, inside := requestPathForWorkspace(root, receipt.Root, path.WorkspacePath)
				if !inside || (!contains(receipt.Missing, rawPath) && !contains(receipt.Excluded, rawPath)) {
					continue
				}
				physical := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.FromSlash(bridge.Domain.PhysicalPrefix), filepath.FromSlash(rawPath))))
				if physical != path.WorkspacePath {
					continue
				}
				for _, batch := range batches {
					if batch.Backend != "go" || batch.ProjectOrModule != receipt.ProjectOrModule || batch.ConfigFingerprint != receipt.ConfigFingerprint {
						continue
					}
					if err := batch.ReadyForStaging(); err != nil {
						return nil, err
					}
					owners = append(owners, indexer.GraphDeltaOwner{Backend: "go", RequestRoot: receipt.Root, ProjectOrModule: receipt.ProjectOrModule, RequestPath: rawPath})
					matched = true
					break
				}
				if matched {
					break
				}
			}
			if !matched {
				owners = nil
				break
			}
		}
		if len(owners) == len(oldByKey) {
			path.Owners = owners
			certified[path.WorkspacePath] = true
		}
	}
	return certified, nil
}

func sameGraphDeltaPaths(a, b []string) bool {
	a, b = append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	return equalStrings(a, b)
}

func graphDeltaPathUnion(parts ...[]string) []string {
	set := map[string]bool{}
	for _, paths := range parts {
		for _, p := range paths {
			set[p] = true
		}
	}
	result := make([]string, 0, len(set))
	for p := range set {
		result = append(result, p)
	}
	sort.Strings(result)
	return result
}
