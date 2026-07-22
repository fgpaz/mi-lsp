package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
)

func (a *App) graphObserver() indexer.GraphObserver {
	return func(ctx context.Context, request indexer.GraphObservationRequest) (model.GraphObservationBatch, error) {
		payload := map[string]any{
			"repository_identity": request.RepositoryIdentity,
			"project_or_module":   request.EntrypointPath,
		}
		workerRequest := model.WorkerRequest{
			ProtocolVersion: model.ProtocolVersion,
			Method:          "graph_observe",
			Workspace:       request.WorkspaceRoot,
			WorkspaceName:   filepath.Base(request.WorkspaceRoot),
			BackendType:     "roslyn",
			RepoID:          request.RepoID,
			RepoName:        request.RepoName,
			RepoRoot:        request.RepoRoot,
			EntrypointID:    request.EntrypointID,
			EntrypointPath:  request.EntrypointPath,
			EntrypointType:  request.EntrypointKind,
			Payload:         payload,
		}
		response, err := a.Semantic.Call(ctx, model.WorkspaceRegistration{Name: workerRequest.WorkspaceName, Root: request.WorkspaceRoot}, workerRequest)
		if err != nil {
			return model.GraphObservationBatch{}, sanitizedGraphObserverError(response.ErrorCode)
		}
		if !response.Ok {
			return model.GraphObservationBatch{}, sanitizedGraphObserverError(response.ErrorCode)
		}
		if strings.TrimSpace(response.Backend) != "roslyn" {
			return model.GraphObservationBatch{}, &model.GraphObservationError{Code: "GPH_BACKEND_MISMATCH", Field: "backend", Message: "unexpected graph backend"}
		}
		if response.Observation == nil {
			return model.GraphObservationBatch{}, &model.GraphObservationError{Code: "GPH_BACKEND_EMPTY", Field: "observation", Message: "graph observation was empty"}
		}
		batch := *response.Observation
		batch.Backend = "roslyn"
		batch.WorkspaceIdentity = request.WorkspaceIdentity
		batch.RepositoryIdentity = request.RepositoryIdentity
		if err := batch.ValidateCanonical(); err != nil {
			return model.GraphObservationBatch{}, fmt.Errorf("invalid graph observation: %w", err)
		}
		if err := model.SealGraphObservationBatch(&batch); err != nil {
			return model.GraphObservationBatch{}, fmt.Errorf("invalid graph observation: %w", err)
		}
		if err := batch.ReadyForStaging(); err != nil {
			return model.GraphObservationBatch{}, fmt.Errorf("graph observation is not ready for staging: %w", err)
		}
		return batch, nil
	}
}

func sanitizedGraphObserverError(code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		code = "GPH_BACKEND_UNAVAILABLE"
	}
	return &model.GraphObservationError{Code: code, Field: "backend", Message: "graph backend request failed"}
}
