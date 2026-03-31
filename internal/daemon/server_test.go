package daemon

import (
	"context"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/service"
)

type workerStatusServerSemanticStub struct {
	statuses []model.WorkerStatus
}

func (s workerStatusServerSemanticStub) Call(context.Context, model.WorkspaceRegistration, model.WorkerRequest) (model.WorkerResponse, error) {
	return model.WorkerResponse{}, nil
}

func (s workerStatusServerSemanticStub) Status() []model.WorkerStatus {
	if len(s.statuses) == 0 {
		return nil
	}
	return append([]model.WorkerStatus(nil), s.statuses...)
}

func TestHandleRequestWorkerStatusDelegatesToCanonicalContract(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	server := &Server{
		app: service.New(t.TempDir(), workerStatusServerSemanticStub{
			statuses: []model.WorkerStatus{{
				Workspace:   "multi-tedi",
				BackendType: "roslyn",
				RuntimeKey:  "roslyn::multi-tedi::default",
				PID:         4321,
			}},
		}),
	}

	response, err := server.handleRequest(model.CommandRequest{
		ProtocolVersion: model.ProtocolVersion,
		Operation:       "worker.status",
	})
	if err != nil {
		t.Fatalf("handleRequest(worker.status): %v", err)
	}
	if !response.Ok {
		t.Fatalf("expected ok=true, got warnings: %v", response.Warnings)
	}
	if response.Backend != "worker" {
		t.Fatalf("backend = %q, want worker", response.Backend)
	}
	items, ok := response.Items.([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one diagnostic item, got %#v", response.Items)
	}
	item := items[0]
	if _, ok := item["selected_source"]; !ok {
		t.Fatalf("selected_source missing from %#v", item)
	}
	activeWorkers, ok := item["active_workers"].([]model.WorkerStatus)
	if !ok {
		t.Fatalf("active_workers type = %T, want []model.WorkerStatus", item["active_workers"])
	}
	if len(activeWorkers) != 1 || activeWorkers[0].PID != 4321 {
		t.Fatalf("active_workers = %#v, want one runtime with pid 4321", activeWorkers)
	}
}
