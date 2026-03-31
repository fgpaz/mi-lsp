package service

import (
	"context"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

type workerStatusSemanticStub struct {
	statuses []model.WorkerStatus
}

func (s workerStatusSemanticStub) Call(context.Context, model.WorkspaceRegistration, model.WorkerRequest) (model.WorkerResponse, error) {
	return model.WorkerResponse{}, nil
}

func (s workerStatusSemanticStub) Status() []model.WorkerStatus {
	if len(s.statuses) == 0 {
		return nil
	}
	return append([]model.WorkerStatus(nil), s.statuses...)
}

func TestWorkerStatusUsesCanonicalDiagnosticContract(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	app := New(t.TempDir(), workerStatusSemanticStub{
		statuses: []model.WorkerStatus{{
			Workspace:   "multi-tedi",
			BackendType: "roslyn",
			RuntimeKey:  "roslyn::multi-tedi::default",
			PID:         1234,
		}},
	})

	env, err := app.Execute(context.Background(), model.CommandRequest{Operation: "worker.status"})
	if err != nil {
		t.Fatalf("worker.status: %v", err)
	}
	if !env.Ok {
		t.Fatalf("expected ok=true, got warnings: %v", env.Warnings)
	}
	if env.Backend != "worker" {
		t.Fatalf("backend = %q, want worker", env.Backend)
	}
	items, ok := env.Items.([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one diagnostic item, got %#v", env.Items)
	}
	item := items[0]
	if _, ok := item["selected_source"]; !ok {
		t.Fatalf("selected_source missing from %#v", item)
	}
	if _, ok := item["cli_path"]; !ok {
		t.Fatalf("cli_path missing from %#v", item)
	}
	if item["protocol_version"] != model.ProtocolVersion {
		t.Fatalf("protocol_version = %#v, want %q", item["protocol_version"], model.ProtocolVersion)
	}
	activeWorkers, ok := item["active_workers"].([]model.WorkerStatus)
	if !ok {
		t.Fatalf("active_workers type = %T, want []model.WorkerStatus", item["active_workers"])
	}
	if len(activeWorkers) != 1 || activeWorkers[0].Workspace != "multi-tedi" {
		t.Fatalf("active_workers = %#v, want one multi-tedi runtime", activeWorkers)
	}
}
