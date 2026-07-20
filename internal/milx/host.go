package milx

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

type IsolationGuard struct {
	NetworkDenied        bool
	ProcessTreeContained bool
}
type HostConfig struct {
	Manifest           Manifest
	Executable         string
	Args               []string
	Timeout            time.Duration
	Environment        map[string]string
	AllowedEnvironment []string
	IsolationGuard     *IsolationGuard
}
type PrepareRequest struct {
	GenerationID       string
	GraphSchemaVersion uint32
	PackSchema         string
	PackDigest         string
	PackBytes          []byte
}
type ExecuteRequest struct {
	OperationName    string
	Parameters       json.RawMessage
	ParametersDigest string
}

type Host struct {
	cfg          HostConfig
	mu           sync.Mutex
	process      *managedProcess
	tempDir      string
	state        LifecycleState
	requestSeq   uint64
	inflight     chan struct{}
	ioMu         sync.Mutex
	generationID string
}

func NewHost(cfg HostConfig) (*Host, error) {
	if cfg.IsolationGuard == nil || !cfg.IsolationGuard.NetworkDenied || !cfg.IsolationGuard.ProcessTreeContained {
		return nil, NewError("GPH_MILX_NETWORK_FORBIDDEN", "host", false, "", "verified isolation guard is required")
	}
	if cfg.Timeout <= 0 {
		return nil, NewError("GPH_MILX_STATE_INVALID", "host", false, "", "host timeout must be positive")
	}
	if err := cfg.Manifest.Validate(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(cfg.Executable) {
		return nil, NewError("GPH_MILX_MANIFEST_INVALID", "host", false, "", "executable path must be absolute")
	}
	data, err := os.ReadFile(cfg.Executable)
	if err != nil {
		return nil, NewError("GPH_MILX_EXECUTABLE_DIGEST_MISMATCH", "manifest", false, "", "executable cannot be verified")
	}
	if err := VerifyExecutableDigest(data, cfg.Manifest.ExecutableSHA256); err != nil {
		return nil, err
	}
	allowed := map[string]bool{}
	for _, name := range cfg.AllowedEnvironment {
		if !idEnv(name) || strings.Contains(strings.ToLower(name), "secret") {
			return nil, NewError("GPH_MILX_CAPABILITY_DENIED", "environment", false, "", "environment capability is not permitted")
		}
		allowed[name] = true
	}
	for name := range cfg.Environment {
		if !allowed[name] {
			return nil, NewError("GPH_MILX_CAPABILITY_DENIED", "environment", false, "", "environment variable is not allowlisted")
		}
	}
	return &Host{cfg: cfg, state: StateSpawned, inflight: make(chan struct{}, 1)}, nil
}
func idEnv(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if !(r == '_' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
func (h *Host) State() LifecycleState { h.mu.Lock(); defer h.mu.Unlock(); return h.state }
func (h *Host) TempDir() string       { h.mu.Lock(); defer h.mu.Unlock(); return h.tempDir }

func (h *Host) Start(ctx context.Context) error {
	h.mu.Lock()
	if h.process != nil || h.state != StateSpawned {
		h.mu.Unlock()
		return NewError("GPH_MILX_STATE_INVALID", "start", false, "", "host is already started")
	}
	dir, err := os.MkdirTemp("", "milx-host-")
	if err != nil {
		h.mu.Unlock()
		return NewError("GPH_MILX_CLEANUP_FAILED", "start", false, "", "host workspace could not be created")
	}
	env := make([]string, 0, len(h.cfg.Environment))
	names := make([]string, 0, len(h.cfg.Environment))
	for n := range h.cfg.Environment {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		env = append(env, n+"="+h.cfg.Environment[n])
	}
	p, err := startManagedProcess(ctx, h.cfg.Executable, h.cfg.Args, dir, env)
	if err != nil {
		_ = os.RemoveAll(dir)
		h.mu.Unlock()
		return NewError("GPH_MILX_PROCESS_FAILED", "spawn", false, "", "extension process could not be started")
	}
	h.process, h.tempDir = p, dir
	h.mu.Unlock()
	if _, err := h.call(ctx, "describe", []byte(`{}`), StateSpawned); err != nil {
		_ = h.Shutdown(context.Background())
		return err
	}
	return nil
}
func (h *Host) Prepare(ctx context.Context, req PrepareRequest) error {
	if len(req.PackBytes) == 0 || req.PackSchema != "milx-pack/v1" || req.GenerationID == "" || req.PackDigest != DigestHex(req.PackBytes) {
		return NewError("GPH_MILX_OUTPUT_INVALID", "prepare", false, "", "pack schema, generation, or digest is invalid")
	}
	var pack Pack
	if err := DecodeCanonical(req.PackBytes, &pack); err != nil || pack.Schema != "milx-pack/v1" {
		return NewError("GPH_MILX_OUTPUT_INVALID", "prepare", false, "", "pack is not canonical")
	}
	payload, _ := CanonicalJSON(map[string]any{"generation_id": req.GenerationID, "graph_schema_version": req.GraphSchemaVersion, "pack_schema": req.PackSchema, "pack_digest": req.PackDigest, "pack_bytes": json.RawMessage(req.PackBytes)})
	if _, err := h.call(ctx, "prepare", payload, StateDescribed); err != nil {
		return err
	}
	h.mu.Lock()
	h.generationID = req.GenerationID
	h.mu.Unlock()
	return nil
}
func (h *Host) Execute(ctx context.Context, req ExecuteRequest) (Result, error) {
	var zero Result
	select {
	case h.inflight <- struct{}{}:
		defer func() { <-h.inflight }()
	default:
		return zero, NewError("GPH_MILX_STATE_INVALID", "execute", true, "", "another execution is already in flight")
	}
	if req.OperationName == "" || len(req.Parameters) == 0 || req.ParametersDigest != DigestHex(req.Parameters) || DecodeCanonical(req.Parameters, &map[string]any{}) != nil {
		return zero, NewError("GPH_MILX_OUTPUT_INVALID", "execute", false, "", "execute parameters are invalid")
	}
	payload, _ := CanonicalJSON(map[string]any{"operation_name": req.OperationName, "parameters": json.RawMessage(req.Parameters), "parameters_digest": req.ParametersDigest})
	raw, err := h.call(ctx, "execute", payload, StatePrepared)
	if err != nil {
		return zero, err
	}
	if err := json.Unmarshal(raw, &zero); err != nil || zero.Schema != "milx-result/v1" || zero.ResultDigest != DigestHex(zero.Result) {
		return Result{}, NewError("GPH_MILX_OUTPUT_INVALID", "execute", false, "", "extension result is invalid")
	}
	if zero.Provenance.GenerationID != h.generationID || zero.Provenance.ExtensionID != h.cfg.Manifest.ExtensionID || zero.Provenance.ExtensionVersion != h.cfg.Manifest.ExtensionVersion || zero.Provenance.ParametersDigest != req.ParametersDigest {
		return Result{}, NewError("GPH_MILX_OUTPUT_INVALID", "execute", false, "", "extension result provenance is invalid")
	}
	return zero, nil
}
func (h *Host) Shutdown(ctx context.Context) error {
	h.mu.Lock()
	p := h.process
	h.mu.Unlock()
	if p == nil {
		return nil
	}
	_, err := h.call(ctx, "shutdown", []byte(`{}`), h.State())
	if err != nil {
		_ = p.killTree()
	}
	h.terminate(p)
	return err
}

func (h *Host) terminate(p *managedProcess) {
	_ = p.close()
	_ = p.wait()
	h.mu.Lock()
	if h.process != p {
		h.mu.Unlock()
		return
	}
	dir := h.tempDir
	h.process, h.tempDir, h.state = nil, "", StateShutdown
	h.mu.Unlock()
	_ = os.RemoveAll(dir)
}

func (h *Host) call(ctx context.Context, operation string, payload []byte, expected LifecycleState) ([]byte, error) {
	h.ioMu.Lock()
	defer h.ioMu.Unlock()
	h.mu.Lock()
	if h.process == nil || !ValidTransition(expected, operation) || h.state != expected {
		h.mu.Unlock()
		return nil, NewError("GPH_MILX_STATE_INVALID", operation, false, "", "invalid host lifecycle state")
	}
	p := h.process
	h.requestSeq++
	id := fmt.Sprintf("r-%d", h.requestSeq)
	h.mu.Unlock()
	if err := ValidateOperationPayload(operation, payload); err != nil {
		return nil, NewError("GPH_MILX_OUTPUT_INVALID", operation, false, "", "operation payload is invalid")
	}
	env, _ := CanonicalJSON(Envelope{Schema: EnvelopeSchema, RequestID: id, Operation: operation, ProtocolVersion: ProtocolVersion, Payload: payload})
	if err := WriteFrame(p.stdin, env); err != nil {
		return nil, NewError("GPH_MILX_PROCESS_FAILED", operation, false, "", "extension request could not be written")
	}
	result := make(chan struct {
		b   []byte
		err error
	}, 1)
	go func() {
		b, e := ReadFrame(p.stdout)
		result <- struct {
			b   []byte
			err error
		}{b, e}
	}()
	timer := time.NewTimer(h.cfg.Timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		_ = p.killTree()
		h.terminate(p)
		return nil, NewError("GPH_MILX_TIMEOUT", operation, true, "", "extension request canceled")
	case <-timer.C:
		_ = p.killTree()
		h.terminate(p)
		return nil, NewError("GPH_MILX_TIMEOUT", operation, true, "", "extension request timed out")
	case r := <-result:
		if r.err != nil {
			h.terminate(p)
			return nil, NewError("GPH_MILX_PROCESS_FAILED", operation, false, "", "extension response could not be read")
		}
		var response Envelope
		if json.Unmarshal(r.b, &response) != nil || ValidateEnvelope(response, true) != nil || response.RequestID != id || response.Operation != operation {
			_ = p.killTree()
			h.terminate(p)
			return nil, NewError("GPH_MILX_OUTPUT_INVALID", operation, false, "", "extension response envelope is invalid")
		}
		if response.Status != "ok" {
			_ = p.killTree()
			h.terminate(p)
			return nil, NewError("GPH_MILX_PROCESS_FAILED", operation, false, "", "extension rejected the request")
		}
		h.mu.Lock()
		if operation == "describe" {
			var m Manifest
			if json.Unmarshal(response.Payload, &m) != nil || !reflect.DeepEqual(m, h.cfg.Manifest) {
				h.mu.Unlock()
				_ = p.killTree()
				h.terminate(p)
				return nil, NewError("GPH_MILX_MANIFEST_INVALID", operation, false, "", "extension manifest does not match")
			}
			h.state = StateDescribed
		} else if operation == "prepare" {
			h.state = StatePrepared
		} else if operation == "execute" {
			h.state = StatePrepared
		}
		h.mu.Unlock()
		return response.Payload, nil
	}
}

var _ = bytes.Equal
var _ = subtle.ConstantTimeCompare
var _ = io.EOF
