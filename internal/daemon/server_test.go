package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/service"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
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

func TestHandleRequestNavPrepareBypassesCacheWhenGenerationUnavailable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".mi-lsp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".mi-lsp", "index.db"), []byte("not sqlite"), 0o644); err != nil {
		t.Fatal(err)
	}
	name := "prepare-generation-error"
	if _, err := workspace.RegisterWorkspace(name, model.WorkspaceRegistration{Name: name, Root: root, Kind: model.WorkspaceKindSingle}); err != nil {
		t.Fatal(err)
	}
	defer workspace.RemoveWorkspace(name)
	server := &Server{app: service.New(root, nil), resultCache: newResultCache()}
	response, err := server.handleRequest(model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "nav.prepare", Context: model.QueryOptions{Workspace: name}, Payload: map[string]any{"task": "bypass"}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Error == nil || response.Error.Code != "governance_unavailable" {
		t.Fatalf("response = %#v", response)
	}
	_, _, entries := server.resultCache.stats()
	if entries != 0 {
		t.Fatalf("cache entries = %d, want 0 when generation is unavailable", entries)
	}
}

func TestHandleRequestNavPrepareMatchesDirectService(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := t.TempDir()
	name := "prepare-parity"
	writeMinimalGovernedIndexedWorkspace(t, root)
	if _, err := workspace.RegisterWorkspace(name, model.WorkspaceRegistration{Name: name, Root: root, Kind: model.WorkspaceKindSingle}); err != nil {
		t.Fatal(err)
	}
	defer workspace.RemoveWorkspace(name)
	request := model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "nav.prepare", Context: model.QueryOptions{Workspace: name}, Payload: map[string]any{"task": "parity", "affected_paths": []string{"src/allowed.go"}}}
	direct, err := service.New(root, nil).Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	assertSuccessfulPreparation(t, direct)
	server := &Server{app: service.New(root, nil), resultCache: newResultCache()}
	cacheMiss, err := server.handleRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	assertSuccessfulPreparation(t, cacheMiss)
	normalize := func(value model.Envelope) []byte {
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var object map[string]any
		if err := json.Unmarshal(body, &object); err != nil {
			t.Fatal(err)
		}
		delete(object, "stats")
		if items, ok := object["items"].([]any); ok {
			for _, item := range items {
				if evidence, ok := item.(map[string]any); ok {
					delete(evidence, "timings_ms")
					delete(evidence, "total_ms")
				}
			}
		}
		body, err = json.Marshal(object)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	if string(normalize(direct)) != string(normalize(cacheMiss)) {
		t.Fatalf("direct and daemon envelopes differ: direct=%s daemon=%s", normalize(direct), normalize(cacheMiss))
	}
	cacheHit, err := server.handleRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if string(normalize(cacheMiss)) != string(normalize(cacheHit)) {
		t.Fatalf("cache miss and hit envelopes differ: miss=%s hit=%s", normalize(cacheMiss), normalize(cacheHit))
	}
	hits, misses, entries := server.resultCache.stats()
	if hits != 1 || misses != 1 || entries != 1 {
		t.Fatalf("cache stats = hits:%d misses:%d entries:%d, want 1:1:1", hits, misses, entries)
	}
}

func writeMinimalGovernedIndexedWorkspace(t *testing.T, root string) {
	t.Helper()
	fixture := strings.Join([]string{
		"# Gobierno documental", "", "```yaml", "version: 1", "profile: spec_backend", "overlays:", "  - spec_core", "  - technical", "numbering_recommended: true", "hierarchy:",
		"  - id: governance", "    label: Governance", "    layer: \"00\"", "    family: functional", "    pack_stage: governance", "    paths:", "      - .docs/wiki/00_gobierno_documental.md",
		"context_chain:", "  - governance", "closure_chain:", "  - governance", "audit_chain:", "  - governance", "blocking_rules:", "  - missing_human_governance_doc", "projection:", "  output: .docs/wiki/_mi-lsp/read-model.toml", "  format: toml", "  auto_sync: true", "  versioned: true", "```",
	}, "\n")
	path := filepath.Join(root, ".docs", "wiki", "00_gobierno_documental.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	status := docgraph.InspectGovernance(root, true)
	if status.Blocked {
		t.Fatalf("minimal governance fixture blocked: %#v", status)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "allowed.go"), []byte("package allowed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, key := range []string{store.WorkspaceMetaLastIndexGeneration, store.WorkspaceMetaActiveDocsGeneration} {
		if err := store.UpsertWorkspaceMeta(context.Background(), db, key, "generation-parity"); err != nil {
			t.Fatal(err)
		}
	}
}

func assertSuccessfulPreparation(t *testing.T, envelope model.Envelope) {
	t.Helper()
	if !envelope.Ok || envelope.Error != nil {
		t.Fatalf("preparation failed: %#v", envelope)
	}
	items, ok := envelope.Items.([]model.SemanticPreparationEvidence)
	if !ok || len(items) != 1 {
		t.Fatalf("unexpected preparation items: %#v", envelope.Items)
	}
	if items[0].Schema != "semantic-preparation-evidence/v1" || len(items[0].AllowedPaths) != 1 || items[0].AllowedPaths[0] != "src/allowed.go" {
		t.Fatalf("unexpected preparation evidence: %#v", items[0])
	}
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
		manager:     manager,
		telemetry:   store,
		resultCache: newResultCache(),
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
	if response.Error == nil || response.Error.Kind != "daemon" || response.Error.Code != "backpressure_busy" || response.Error.Stage != "backend" {
		t.Fatalf("error = %+v, want daemon/backpressure_busy at backend", response.Error)
	}
}

func TestRuntimeKeyUsesWorkspaceRootNotAlias(t *testing.T) {
	root := t.TempDir()
	left := model.WorkspaceRegistration{Name: "alias-left", Root: root}
	right := model.WorkspaceRegistration{Name: "alias-right", Root: root}
	request := model.WorkerRequest{BackendType: "roslyn", EntrypointID: "default"}

	leftKey := runtimeKey(left, request)
	rightKey := runtimeKey(right, request)
	if leftKey != rightKey {
		t.Fatalf("runtime keys for same root aliases differ: %q vs %q", leftKey, rightKey)
	}
}

func TestRuntimeKeySeparatesDifferentWorktreeRoots(t *testing.T) {
	left := model.WorkspaceRegistration{Name: "mi-lsp-main", Root: t.TempDir()}
	right := model.WorkspaceRegistration{Name: "mi-lsp-feature", Root: t.TempDir()}
	request := model.WorkerRequest{BackendType: "roslyn", EntrypointID: "default"}

	leftKey := runtimeKey(left, request)
	rightKey := runtimeKey(right, request)
	if leftKey == rightKey {
		t.Fatalf("runtime keys for different roots should differ: %q", leftKey)
	}
}
