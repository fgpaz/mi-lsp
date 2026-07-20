package milx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func helperExecutable(t *testing.T) (string, string) {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, DigestHex(data)
}

func helperManifest(t *testing.T) Manifest {
	path, digest := helperExecutable(t)
	_ = path
	return Manifest{Schema: ManifestSchema, ExtensionID: "test.helper", ExtensionVersion: "1.0.0", ExecutableSHA256: digest, ProtocolMin: 1, ProtocolMax: 1, Operations: []string{"cancel", "describe", "execute", "health", "prepare", "shutdown"}, InputSchemas: []string{"milx-pack/v1"}, OutputSchemas: []string{"milx-result/v1"}, Capabilities: []string{"analysis.emit"}, Deterministic: true}
}

func helperConfig(t *testing.T) HostConfig {
	path, _ := helperExecutable(t)
	return HostConfig{
		Manifest:           helperManifest(t),
		Executable:         path,
		Timeout:            5 * time.Second,
		Environment:        map[string]string{"MILX_TEST_HELPER": "1"},
		AllowedEnvironment: []string{"MILX_TEST_HELPER"},
		IsolationGuard:     &IsolationGuard{NetworkDenied: true, ProcessTreeContained: true},
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("MILX_TEST_HELPER") == "1" {
		runMILXHelper()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestHostLifecycleDescribePrepareExecuteAndCleanup(t *testing.T) {
	cfg := helperConfig(t)
	host, err := NewHost(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := host.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if host.State() != StateDescribed {
		t.Fatalf("state=%s", host.State())
	}
	packBytes, err := CanonicalJSON(Pack{Schema: "milx-pack/v1", GenerationID: "generation-1", Selection: json.RawMessage(`{"nodes":[]}`), Provenance: Provenance{GenerationID: "generation-1"}, Digest: DigestHex([]byte("pack")), Omissions: []Omission{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.Prepare(ctx, PrepareRequest{GenerationID: "generation-1", GraphSchemaVersion: 1, PackSchema: "milx-pack/v1", PackDigest: DigestHex(packBytes), PackBytes: packBytes}); err != nil {
		t.Fatal(err)
	}
	params := json.RawMessage(`{"value":1}`)
	result, err := host.Execute(ctx, ExecuteRequest{OperationName: "analysis", Parameters: params, ParametersDigest: DigestHex(params)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provenance.GenerationID != "generation-1" || result.Provenance.ExtensionID != cfg.Manifest.ExtensionID {
		t.Fatalf("bad provenance: %+v", result.Provenance)
	}
	if host.State() != StatePrepared {
		t.Fatalf("state=%s", host.State())
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if host.State() != StateShutdown {
		t.Fatalf("state=%s", host.State())
	}
	if _, err := os.Stat(host.TempDir()); !os.IsNotExist(err) {
		t.Fatalf("temp dir remains: %v", err)
	}
}

func TestHostRejectsNoGuardAndDigestBeforeSpawn(t *testing.T) {
	cfg := helperConfig(t)
	cfg.IsolationGuard = nil
	if _, err := NewHost(cfg); err == nil || !strings.Contains(err.Error(), "GPH_MILX_NETWORK_FORBIDDEN") {
		t.Fatalf("no guard error=%v", err)
	}
	cfg = helperConfig(t)
	cfg.Manifest.ExecutableSHA256 = strings.Repeat("0", 64)
	if _, err := NewHost(cfg); err == nil || !strings.Contains(err.Error(), "GPH_MILX_EXECUTABLE_DIGEST_MISMATCH") {
		t.Fatalf("digest error=%v", err)
	}
}

func TestHostRejectsUnknownCapabilityAndScrubsEnvironment(t *testing.T) {
	cfg := helperConfig(t)
	cfg.Manifest.Capabilities = []string{"analysis.emit", "network"}
	if _, err := NewHost(cfg); err == nil || !strings.Contains(err.Error(), "GPH_MILX_MANIFEST_INVALID") {
		t.Fatalf("capability error=%v", err)
	}
	cfg = helperConfig(t)
	cfg.Environment = map[string]string{"MILX_TEST_VISIBLE": "yes", "MILX_TEST_SECRET": "no"}
	cfg.AllowedEnvironment = []string{"MILX_TEST_VISIBLE", "MILX_TEST_SECRET"}
	if _, err := NewHost(cfg); err == nil || !strings.Contains(err.Error(), "GPH_MILX_CAPABILITY_DENIED") {
		t.Fatalf("secret environment error=%v", err)
	}
}

func TestAnalysisCacheExactKeyAndBound(t *testing.T) {
	cache := NewAnalysisCache(64)
	key := CacheKey{GenerationID: "g", ExtensionID: "e", ExtensionVersion: "1", ExecutableSHA256: strings.Repeat("a", 64), Operation: "analysis", ParametersDigest: strings.Repeat("b", 64), AuthorityProfileDigest: strings.Repeat("c", 64), OutputSchema: "milx-result/v1"}
	if err := cache.Put(key, []byte(`{"result":1}`)); err != nil {
		t.Fatal(err)
	}
	got, ok := cache.Get(key)
	if !ok || string(got) != `{"result":1}` {
		t.Fatalf("cache miss: %q %v", got, ok)
	}
	key.ParametersDigest = strings.Repeat("d", 64)
	if _, ok := cache.Get(key); ok {
		t.Fatal("cache key mutation was not invalidated")
	}
	if err := cache.Put(key, make([]byte, 65)); err == nil {
		t.Fatal("oversize cache value accepted")
	}
}

func TestHostSingleInflight(t *testing.T) {
	cfg := helperConfig(t)
	host, err := NewHost(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	pack := []byte(`{"digest":"` + strings.Repeat("a", 64) + `","generation_id":"g","omissions":[],"provenance":{"generation_id":"g"},"schema":"milx-pack/v1","selection":{}}`)
	if err := host.Prepare(context.Background(), PrepareRequest{GenerationID: "g", PackSchema: "milx-pack/v1", PackDigest: DigestHex(pack), PackBytes: pack}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var failures int
	var mu sync.Mutex
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := host.Execute(context.Background(), ExecuteRequest{OperationName: "analysis", Parameters: json.RawMessage(`{}`), ParametersDigest: DigestHex([]byte(`{}`))})
			if e != nil {
				mu.Lock()
				failures++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if failures != 1 {
		t.Fatalf("failures=%d, want one", failures)
	}
	_ = host.Shutdown(context.Background())
}

func runMILXHelper() {
	generationID := ""
	for {
		payload, err := ReadFrame(os.Stdin)
		if err != nil {
			return
		}
		var req Envelope
		if json.Unmarshal(payload, &req) != nil {
			return
		}
		response := Envelope{Schema: EnvelopeSchema, RequestID: req.RequestID, Operation: req.Operation, ProtocolVersion: ProtocolVersion, Status: "ok"}
		switch req.Operation {
		case "describe":
			path, _ := os.Executable()
			data, _ := os.ReadFile(path)
			m := Manifest{Schema: ManifestSchema, ExtensionID: "test.helper", ExtensionVersion: "1.0.0", ExecutableSHA256: DigestHex(data), ProtocolMin: 1, ProtocolMax: 1, Operations: []string{"cancel", "describe", "execute", "health", "prepare", "shutdown"}, InputSchemas: []string{"milx-pack/v1"}, OutputSchemas: []string{"milx-result/v1"}, Capabilities: []string{"analysis.emit"}, Deterministic: true}
			response.Payload, _ = CanonicalJSON(m)
		case "prepare":
			var p map[string]any
			_ = json.Unmarshal(req.Payload, &p)
			generationID, _ = p["generation_id"].(string)
			response.Payload, _ = CanonicalJSON(map[string]any{"prepared_id": "p", "accepted_capabilities": []string{"analysis.emit"}, "effective_budgets": map[string]any{}})
		case "execute":
			var p map[string]any
			_ = json.Unmarshal(req.Payload, &p)
			paramsDigest, _ := p["parameters_digest"].(string)
			result := json.RawMessage(`{"ok":1}`)
			response.Payload, _ = CanonicalJSON(Result{Schema: "milx-result/v1", Result: result, ResultDigest: DigestHex(result), Provenance: Provenance{GenerationID: generationID, ExtensionID: "test.helper", ExtensionVersion: "1.0.0", ParametersDigest: paramsDigest}, Omissions: []Omission{}})
		case "health":
			response.Payload, _ = CanonicalJSON(map[string]any{"status": "ok", "protocol_version": 1, "extension_id": "test.helper"})
		case "shutdown":
			response.Payload, _ = CanonicalJSON(map[string]any{"cleanup_status": "ok"})
			b, _ := CanonicalJSON(response)
			_ = WriteFrame(os.Stdout, b)
			return
		default:
			response.Payload, _ = CanonicalJSON(map[string]any{})
		}
		b, _ := CanonicalJSON(response)
		if WriteFrame(os.Stdout, b) != nil {
			return
		}
	}
}

var _ = sha256.Sum256
var _ = hex.EncodeToString
var _ = io.Discard
var _ = filepath.Clean
var _ = exec.Command
