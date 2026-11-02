package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sort"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
)

// observeSelectiveGoGraphRequest executes the exact Go request selected by the
// indexer planner and returns its unmodified native receipt for canonical sealing.
func (a *App) observeSelectiveGoGraphRequest(ctx context.Context, request indexer.GraphObservationRequest, goRequest indexer.GoGraphObservationRequest, affected []string) (model.GraphObservationBatch, indexer.GoGraphDeltaReceipt, error) {
	var empty indexer.GoGraphDeltaReceipt
	if request.Backend != "go" || len(affected) == 0 || request.ProjectOrModule != goRequest.ProjectOrModule || filepath.Clean(request.RepoRoot) != filepath.Clean(goRequest.Root) || request.RepositoryIdentity != goRequest.RepositoryIdentity || request.WorkspaceIdentity != request.RepositoryIdentity {
		return model.GraphObservationBatch{}, empty, overlayGraphUnavailable("Go selective request does not match canonical planner context")
	}
	batch, receipt, err := indexer.ObserveGoGraphDeltaWithReceipt(ctx, goRequest, affected)
	if err != nil {
		return batch, receipt, err
	}
	if receipt.ProjectOrModule != request.ProjectOrModule || filepath.Clean(receipt.Root) != filepath.Clean(goRequest.Root) || batch.RepositoryIdentity != request.RepositoryIdentity || batch.WorkspaceIdentity != request.WorkspaceIdentity {
		return model.GraphObservationBatch{}, empty, overlayGraphUnavailable("Go observer receipt does not match the exact planned request")
	}
	if err := batch.ReadyForStaging(); err != nil {
		return model.GraphObservationBatch{}, empty, err
	}
	return batch, receipt, nil
}

// observeSelectiveGraphRequest uses an already planned canonical request. In
// particular ProjectOrModule and affected are relative to its repository, not
// guessed from workspace entrypoint paths. Canonical planning/rebasing remains
// owned by the indexer; this adapter must not mint a different NodeKey domain.
// The caller skips empty selections instead of falling back to a full graph.
func (a *App) observeSelectiveGraphRequest(ctx context.Context, request indexer.GraphObservationRequest, affected []string) (model.GraphObservationBatch, error) {
	if err := ctx.Err(); err != nil {
		return model.GraphObservationBatch{}, err
	}
	if len(affected) == 0 {
		return model.GraphObservationBatch{}, overlayGraphUnavailable("empty selective observation must be skipped")
	}
	paths := append([]string(nil), affected...)
	sort.Strings(paths)
	var batch model.GraphObservationBatch
	var err error
	switch request.Backend {
	case "go":
		batch, err = indexer.ObserveGoGraphDelta(ctx, indexer.GoGraphObservationRequest{
			Root: request.RepoRoot, RepositoryIdentity: request.RepositoryIdentity,
			ProjectOrModule: request.ProjectOrModule, GoOffline: true,
		}, paths)
	case "roslyn":
		if a.Semantic == nil {
			return batch, overlayGraphUnavailable("selective Roslyn observer is unavailable")
		}
		workerRequest := model.WorkerRequest{
			ProtocolVersion: model.ProtocolVersion, Method: "graph_observe",
			Workspace: request.WorkspaceRoot, WorkspaceName: filepath.Base(request.WorkspaceRoot),
			BackendType: "roslyn", RepoID: request.RepoID, RepoName: request.RepoName, RepoRoot: request.RepoRoot,
			EntrypointID: request.EntrypointID, EntrypointPath: request.EntrypointPath, EntrypointType: request.EntrypointKind,
			Payload: map[string]any{"repository_identity": request.RepositoryIdentity, "project_or_module": request.ProjectOrModule, "affected_paths": paths},
		}
		response, callErr := a.Semantic.Call(ctx, model.WorkspaceRegistration{Name: workerRequest.WorkspaceName, Root: request.WorkspaceRoot}, workerRequest)
		if callErr != nil {
			return batch, callErr
		}
		if !response.Ok || response.Partial || response.Backend != "roslyn" || response.Observation == nil || !graphDeltaScopeAcknowledged(response.Items, paths) {
			return batch, overlayGraphUnavailable("Roslyn did not acknowledge a complete selective observation")
		}
		batch = *response.Observation
	default:
		return batch, overlayGraphUnavailable("selected backend has no stageable selective graph observer")
	}
	if err != nil {
		return batch, err
	}
	if batch.WorkspaceIdentity != request.WorkspaceIdentity || batch.RepositoryIdentity != request.RepositoryIdentity || batch.ProjectOrModule != request.ProjectOrModule {
		return batch, overlayGraphUnavailable("selective observer returned a mismatched graph identity")
	}
	if err := batch.ReadyForStaging(); err != nil {
		return batch, err
	}
	if request.AppliedPaths != nil {
		*request.AppliedPaths = append([]string{}, paths...)
	}
	return batch, nil
}

// Old workers may ignore optional payload fields. Require exactly one explicit
// acknowledgement and the complete applied path set, not merely an Ok flag.
func graphDeltaScopeAcknowledged(items []map[string]any, paths []string) bool {
	found := false
	for _, item := range items {
		if item["observation_scope"] != "affected_paths/v1" {
			continue
		}
		if found {
			return false
		}
		found = true
		raw, err := json.Marshal(item["applied_paths"])
		if err != nil {
			return false
		}
		var applied []string
		if json.Unmarshal(raw, &applied) != nil {
			return false
		}
		sort.Strings(applied)
		if !equalStrings(applied, paths) {
			return false
		}
	}
	return found
}
