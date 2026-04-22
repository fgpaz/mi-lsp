package daemon

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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

func TestHandleRequestSystemStatusIncludesProcessAndWatcherStats(t *testing.T) {
	store := testStore(t)
	defer store.Close()
	manager := NewManagerWithOptions(t.TempDir(), 1, time.Minute, StartOptions{WatchMode: WatchModeLazy, MaxWatchedRoots: 2})
	defer manager.Shutdown()
	server := &Server{
		manager:   manager,
		telemetry: store,
		state: model.DaemonState{
			PID:             123,
			Endpoint:        "test",
			ProtocolVersion: model.ProtocolVersion,
			WatchMode:       WatchModeLazy,
			MaxWatchedRoots: 2,
		},
	}

	response, err := server.handleRequest(model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "system.status"})
	if err != nil {
		t.Fatalf("handleRequest(system.status): %v", err)
	}
	body, err := json.Marshal(response.Items)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var items []map[string]any
	if err := json.Unmarshal(body, &items); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if _, ok := items[0]["daemon_process"]; !ok {
		t.Fatalf("daemon_process missing from %#v", items[0])
	}
	watchers, ok := items[0]["watchers"].(map[string]any)
	if !ok {
		t.Fatalf("watchers type = %T, want object", items[0]["watchers"])
	}
	if watchers["mode"] != WatchModeLazy {
		t.Fatalf("watchers.mode = %v, want %s", watchers["mode"], WatchModeLazy)
	}
}

func TestBackpressureBusyEnvelopeIsTyped(t *testing.T) {
	server := &Server{options: StartOptions{MaxInflight: 1}, inflight: make(chan struct{}, 1)}
	server.inflight <- struct{}{}
	request := model.CommandRequest{Operation: "nav.context"}
	if !server.isBackpressureLimited(request) {
		t.Fatal("nav.context should be backpressure-limited")
	}
	response := server.backpressureEnvelope(request)
	if response.Ok {
		t.Fatal("busy envelope Ok = true, want false")
	}
	if !isBackpressureEnvelope(response) {
		t.Fatalf("isBackpressureEnvelope = false for %#v", response)
	}
	items, ok := response.Items.([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %#v, want one typed item", response.Items)
	}
	if items[0]["error_code"] != "backpressure_busy" {
		t.Fatalf("error_code = %v, want backpressure_busy", items[0]["error_code"])
	}
}
