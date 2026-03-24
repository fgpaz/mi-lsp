package worker

import (
	"context"
	"path/filepath"

	"github.com/fgpaz/mi-lsp/internal/model"
)

type RuntimeClient interface {
	Call(context.Context, model.WorkerRequest) (model.WorkerResponse, error)
	Close() error
	PID() int
}

func NewRuntimeClient(repoRoot string, workspace model.WorkspaceRegistration, request model.WorkerRequest) (RuntimeClient, error) {
	backendType := request.BackendType
	switch backendType {
	case "", "roslyn":
		return NewClient(repoRoot, workspace)
	case "tsserver":
		runtimeWorkspace := workspace
		if request.RepoRoot != "" {
			runtimeWorkspace.Root = request.RepoRoot
		}
		if request.RepoName != "" {
			runtimeWorkspace.Name = request.RepoName
		} else if request.RepoRoot != "" {
			runtimeWorkspace.Name = filepath.Base(request.RepoRoot)
		}
		return NewTsserverClient(runtimeWorkspace)
	case "pyright":
		runtimeWorkspace := workspace
		if request.RepoRoot != "" {
			runtimeWorkspace.Root = request.RepoRoot
		}
		if request.RepoName != "" {
			runtimeWorkspace.Name = request.RepoName
		} else if request.RepoRoot != "" {
			runtimeWorkspace.Name = filepath.Base(request.RepoRoot)
		}
		return NewPyrightClient(runtimeWorkspace)
	default:
		return nil, ErrUnsupportedBackend(backendType)
	}
}

type unsupportedBackendError string

func (e unsupportedBackendError) Error() string {
	return "unsupported backend: " + string(e)
}

func ErrUnsupportedBackend(backendType string) error {
	return unsupportedBackendError(backendType)
}
