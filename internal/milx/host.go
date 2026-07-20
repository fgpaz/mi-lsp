package milx

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
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

// isolationGuard is intentionally unexported: production callers cannot forge
// the platform proof required to start an extension host. Platform adapters
// must create it only after enforcing network denial and process-tree containment.
type IsolationGuard struct {
	networkDenied        bool
	processTreeContained bool
}

func (g *IsolationGuard) verify() error {
	if g == nil || !g.networkDenied || !g.processTreeContained {
		return NewError("GPH_MILX_NETWORK_FORBIDDEN", "host", false, "", "verified isolation guard is required")
	}
	return nil
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

const (
	defaultHostTimeout = 60 * time.Second
	maxHostTimeout     = 300 * time.Second
	cancelGrace        = 2 * time.Second
)

func NewHost(cfg HostConfig) (*Host, error) {
	if err := cfg.IsolationGuard.verify(); err != nil {
		return nil, err
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultHostTimeout
	}
	if cfg.Timeout < 0 || cfg.Timeout > maxHostTimeout {
		return nil, NewError("GPH_MILX_STATE_INVALID", "host", false, "", "host timeout is outside the permitted range")
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
		lower := strings.ToLower(name)
		if !idEnv(name) || forbiddenEnvironmentName(lower) {
			return nil, NewError("GPH_MILX_CAPABILITY_DENIED", "environment", false, "", "environment capability is not permitted")
		}
		allowed[name] = true
	}
	for name := range cfg.Environment {
		if !allowed[name] || forbiddenEnvironmentName(strings.ToLower(name)) {
			return nil, NewError("GPH_MILX_CAPABILITY_DENIED", "environment", false, "", "environment variable is not allowlisted")
		}
	}
	return &Host{cfg: cfg, state: StateSpawned, inflight: make(chan struct{}, 1)}, nil
}
func forbiddenEnvironmentName(s string) bool {
	for _, word := range []string{"token", "secret", "key", "password", "credential", "proxy"} {
		if strings.Contains(s, word) {
			return true
		}
	}
	return false
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
	if len(req.PackBytes) == 0 || len(req.PackBytes) > MaxFrameBytes || req.PackSchema != "milx-pack/v1" || req.GenerationID == "" || req.PackDigest != DigestHex(req.PackBytes) {
		return NewError("GPH_MILX_OUTPUT_INVALID", "prepare", false, "", "pack schema, generation, or digest is invalid")
	}
	var pack Pack
	if err := DecodeCanonical(req.PackBytes, &pack); err != nil || VerifyPack(pack) != nil || pack.GenerationID != req.GenerationID || pack.Schema != req.PackSchema || req.PackDigest != DigestHex(req.PackBytes) {
		return NewError("GPH_MILX_OUTPUT_INVALID", "prepare", false, "", "pack is not canonical or does not match the request")
	}
	payload, err := CanonicalJSON(map[string]any{"generation_id": req.GenerationID, "graph_schema_version": req.GraphSchemaVersion, "pack_schema": req.PackSchema, "pack_digest": req.PackDigest, "pack_bytes": json.RawMessage(req.PackBytes)})
	if err != nil {
		return NewError("GPH_MILX_OUTPUT_INVALID", "prepare", false, "", "prepare payload exceeds frame limit")
	}
	raw, err := h.call(ctx, "prepare", payload, StateDescribed)
	if err != nil {
		return err
	}
	var ack struct {
		PreparedID           string         `json:"prepared_id"`
		AcceptedCapabilities []string       `json:"accepted_capabilities"`
		EffectiveBudgets     map[string]any `json:"effective_budgets"`
	}
	if DecodeCanonical(raw, &ack) != nil || ack.PreparedID == "" || ack.EffectiveBudgets == nil {
		return NewError("GPH_MILX_OUTPUT_INVALID", "prepare", false, "", "prepare response is invalid")
	}
	allowed := map[string]bool{}
	for _, c := range h.cfg.Manifest.Capabilities {
		allowed[c] = true
	}
	for _, c := range ack.AcceptedCapabilities {
		if !allowed[c] {
			return NewError("GPH_MILX_OUTPUT_INVALID", "prepare", false, "", "prepare response grants an unmanifested capability")
		}
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
	if req.OperationName == "" || !contains(h.cfg.Manifest.Operations, req.OperationName) || len(req.Parameters) == 0 || req.ParametersDigest != DigestHex(req.Parameters) || DecodeCanonical(req.Parameters, &map[string]any{}) != nil {
		return zero, NewError("GPH_MILX_OUTPUT_INVALID", "execute", false, "", "execute parameters are invalid")
	}
	payload, err := CanonicalJSON(map[string]any{"operation_name": req.OperationName, "parameters": json.RawMessage(req.Parameters), "parameters_digest": req.ParametersDigest})
	if err != nil {
		return zero, NewError("GPH_MILX_OUTPUT_INVALID", "execute", false, "", "execute payload exceeds frame limit")
	}
	raw, err := h.call(ctx, "execute", payload, StatePrepared)
	if err != nil {
		return zero, err
	}
	h.mu.Lock()
	generationID := h.generationID
	h.mu.Unlock()
	if DecodeCanonical(raw, &zero) != nil || ValidateDerivedResult(zero, generationID, h.cfg.Manifest.ExtensionID, h.cfg.Manifest.ExtensionVersion, req.ParametersDigest) != nil {
		return Result{}, NewError("GPH_MILX_OUTPUT_INVALID", "execute", false, "", "extension result is invalid")
	}
	return zero, nil
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
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
		return err
	}
	if err := h.terminate(p); err != nil {
		return cleanupFailure("shutdown")
	}
	return nil
}

func cleanupFailure(operation string) error {
	return NewError("GPH_MILX_CLEANUP_FAILED", operation, false, "", "extension cleanup could not be completed")
}

func (h *Host) cleanup(p *managedProcess) error {
	return errors.Join(p.killTree(), h.terminate(p))
}

func (h *Host) terminate(p *managedProcess) error {
	closeErr := p.close()
	waitErr := p.wait()
	h.mu.Lock()
	if h.process != p {
		h.mu.Unlock()
		return errors.Join(closeErr, waitErr)
	}
	dir := h.tempDir
	h.process, h.tempDir, h.state = nil, "", StateShutdown
	h.mu.Unlock()
	return errors.Join(closeErr, waitErr, os.RemoveAll(dir))
}

func (h *Host) call(ctx context.Context, operation string, payload []byte, expected LifecycleState) ([]byte, error) {
	h.ioMu.Lock()
	defer h.ioMu.Unlock()
	h.mu.Lock()
	if h.process == nil || !ValidTransition(expected, operation) || h.state != expected || !contains(h.cfg.Manifest.Operations, operation) {
		h.mu.Unlock()
		return nil, NewError("GPH_MILX_STATE_INVALID", operation, false, "", "invalid host lifecycle state or operation")
	}
	p := h.process
	h.requestSeq++
	id := fmt.Sprintf("r-%d", h.requestSeq)
	if operation == "execute" {
		h.state = StateExecuting
	}
	h.mu.Unlock()
	if err := ValidateOperationPayload(operation, payload); err != nil {
		return h.cleanupAfterFailure(p, operation, "GPH_MILX_OUTPUT_INVALID", false, "operation payload is invalid")
	}
	env, err := CanonicalJSON(Envelope{Schema: EnvelopeSchema, RequestID: id, Operation: operation, ProtocolVersion: ProtocolVersion, Payload: payload})
	if err != nil || WriteFrame(p.stdin, env) != nil {
		return h.cleanupAfterFailure(p, operation, "GPH_MILX_PROCESS_FAILED", false, "extension request could not be written")
	}
	result := make(chan struct {
		b   []byte
		err error
	}, 1)
	go func() {
		b, e := readResponseFor(p.stdout, id)
		result <- struct {
			b   []byte
			err error
		}{b, e}
	}()
	timer := time.NewTimer(h.cfg.Timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return h.cancelAndCleanup(p, id, operation, result)
	case <-timer.C:
		return h.cancelAndCleanup(p, id, operation, result)
	case r := <-result:
		return h.finishResponse(p, operation, id, r.b, r.err)
	}
}
func (h *Host) cleanupAfterFailure(p *managedProcess, operation, code string, retryable bool, summary string) ([]byte, error) {
	if err := h.cleanup(p); err != nil {
		return nil, cleanupFailure(operation)
	}
	return nil, NewError(code, operation, retryable, "", summary)
}

func readResponseFor(r io.Reader, id string) ([]byte, error) {
	for {
		b, err := ReadFrame(r)
		if err != nil {
			return nil, err
		}
		var e Envelope
		if json.Unmarshal(b, &e) == nil && e.RequestID != id && e.Operation == "cancel" {
			continue
		}
		return b, nil
	}
}
func (h *Host) cancelAndCleanup(p *managedProcess, id, operation string, result chan struct {
	b   []byte
	err error
}) ([]byte, error) {
	h.mu.Lock()
	h.requestSeq++
	cancelID := fmt.Sprintf("r-%d", h.requestSeq)
	h.mu.Unlock()
	payload, _ := CanonicalJSON(map[string]any{"target_request_id": id})
	env, _ := CanonicalJSON(Envelope{Schema: EnvelopeSchema, RequestID: cancelID, Operation: "cancel", ProtocolVersion: ProtocolVersion, Payload: payload})
	_ = WriteFrame(p.stdin, env)
	grace := time.NewTimer(cancelGrace)
	defer grace.Stop()
	select {
	case <-result:
		return h.cleanupAfterFailure(p, operation, "GPH_MILX_TIMEOUT", true, "extension request canceled")
	case <-grace.C:
	}
	return h.cleanupAfterFailure(p, operation, "GPH_MILX_TIMEOUT", true, "extension request timed out")
}
func (h *Host) finishResponse(p *managedProcess, operation, id string, b []byte, readErr error) ([]byte, error) {
	if readErr != nil {
		return h.cleanupAfterFailure(p, operation, "GPH_MILX_PROCESS_FAILED", false, "extension response could not be read")
	}
	var response Envelope
	if DecodeCanonical(b, &response) != nil || ValidateEnvelope(response, true) != nil || response.RequestID != id || response.Operation != operation {
		return h.cleanupAfterFailure(p, operation, "GPH_MILX_OUTPUT_INVALID", false, "extension response envelope is invalid")
	}
	if response.Status != "ok" {
		return h.cleanupAfterFailure(p, operation, "GPH_MILX_PROCESS_FAILED", false, "extension rejected the request")
	}
	h.mu.Lock()
	if operation == "describe" {
		var m Manifest
		if DecodeCanonical(response.Payload, &m) != nil || !reflect.DeepEqual(m, h.cfg.Manifest) {
			h.mu.Unlock()
			return h.cleanupAfterFailure(p, operation, "GPH_MILX_MANIFEST_INVALID", false, "extension manifest does not match")
		}
		h.state = StateDescribed
	}
	if operation == "prepare" {
		h.state = StatePrepared
	}
	if operation == "execute" {
		h.state = StatePrepared
	}
	if operation == "health" {
		var health struct {
			Status          string `json:"status"`
			ProtocolVersion uint32 `json:"protocol_version"`
			ExtensionID     string `json:"extension_id"`
		}
		if DecodeCanonical(response.Payload, &health) != nil || health.Status != "ok" || health.ProtocolVersion != ProtocolVersion || health.ExtensionID != h.cfg.Manifest.ExtensionID {
			h.mu.Unlock()
			return h.cleanupAfterFailure(p, operation, "GPH_MILX_OUTPUT_INVALID", false, "health response is invalid")
		}
	}
	if operation == "shutdown" {
		var shutdown struct {
			CleanupStatus string `json:"cleanup_status"`
		}
		if DecodeCanonical(response.Payload, &shutdown) != nil || shutdown.CleanupStatus != "ok" {
			h.mu.Unlock()
			return h.cleanupAfterFailure(p, operation, "GPH_MILX_CLEANUP_FAILED", false, "shutdown response is invalid")
		}
	}
	h.mu.Unlock()
	return response.Payload, nil
}

var _ = bytes.Equal
var _ = subtle.ConstantTimeCompare
var _ = io.EOF
