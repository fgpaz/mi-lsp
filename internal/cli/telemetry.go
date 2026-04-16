package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/daemon"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/telemetry"
)

// CLITelemetry records access events directly to daemon.db when the daemon is not running.
// All operations are best-effort: failures are logged to stderr in verbose mode but never propagate.
type CLITelemetry struct {
	store      *daemon.TelemetryStore
	clientName string
	sessionID  string
	verbose    bool
}

func NewCLITelemetry(clientName, sessionID string, verbose bool) *CLITelemetry {
	store, err := daemon.OpenTelemetryStore()
	if err != nil {
		if verbose {
			fmt.Fprintf(os.Stderr, "mi-lsp: telemetry init failed: %v\n", err)
		}
		return &CLITelemetry{clientName: clientName, sessionID: sessionID, verbose: verbose}
	}
	return &CLITelemetry{store: store, clientName: clientName, sessionID: sessionID, verbose: verbose}
}

func (t *CLITelemetry) RecordOperation(request model.CommandRequest, envelope model.Envelope, opErr error, latency time.Duration, route string) {
	if t == nil || t.store == nil {
		return
	}
	backend := firstNonEmpty(envelope.Backend, inferTelemetryBackend(request))
	event := model.AccessEvent{
		OccurredAt:     time.Now(),
		ClientName:     firstNonEmpty(t.clientName, request.Context.ClientName, "manual-cli"),
		SessionID:      firstNonEmpty(t.sessionID, request.Context.SessionID),
		WorkspaceInput: request.Context.Workspace,
		Workspace:      firstNonEmpty(envelope.Workspace, request.Context.Workspace),
		Repo:           payloadStr(request.Payload, "repo"),
		Operation:      request.Operation,
		Backend:        backend,
		Route:          route,
		Format:         request.Context.Format,
		TokenBudget:    request.Context.TokenBudget,
		MaxItems:       request.Context.MaxItems,
		MaxChars:       request.Context.MaxChars,
		Compress:       request.Context.Compress,
		Success:        opErr == nil && envelope.Ok,
		LatencyMs:      latency.Milliseconds(),
		Warnings:       envelope.Warnings,
		RuntimeKey:     telemetry.RuntimeKeyForOperation(request, envelope),
	}
	if opErr != nil {
		event.Error = opErr.Error()
	}
	event = telemetry.EnrichAccessEvent(event, request, envelope, opErr)

	if err := t.store.RecordAccessDirect(event); err != nil && t.verbose {
		fmt.Fprintf(os.Stderr, "mi-lsp: telemetry record failed: %v\n", err)
	}
}

func (t *CLITelemetry) ApplyRetention(maxAgeDays int) (int64, error) {
	if t == nil || t.store == nil {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -maxAgeDays)
	eventsDeleted, err := t.store.PurgeOldEvents(cutoff)
	if err != nil {
		return 0, err
	}
	runsDeleted, err := t.store.PurgeOldRuns(cutoff)
	if err != nil {
		return eventsDeleted, err
	}
	return eventsDeleted + runsDeleted, nil
}

func (t *CLITelemetry) Close() error {
	if t == nil || t.store == nil {
		return nil
	}
	return t.store.Close()
}

func inferTelemetryBackend(request model.CommandRequest) string {
	if request.Operation == "nav.context" {
		file := payloadStr(request.Payload, "file")
		if !isSemanticTelemetryFile(file) {
			return "text"
		}
	}
	if explicit := strings.TrimSpace(request.Context.BackendHint); explicit != "" {
		return strings.ToLower(explicit)
	}
	switch request.Operation {
	case "nav.search":
		return "text"
	case "nav.find", "nav.overview", "nav.outline", "nav.symbols":
		return "catalog"
	case "nav.deps":
		return "roslyn"
	case "nav.context", "nav.refs":
		file := payloadStr(request.Payload, "file")
		if isTypeScriptTelemetryFile(file) {
			return "tsserver"
		}
		if isPythonTelemetryFile(file) {
			return "pyright"
		}
		if request.Operation == "nav.context" && !isSemanticTelemetryFile(file) {
			return "text"
		}
		return "roslyn"
	default:
		return ""
	}
}

func isSemanticTelemetryFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cs", ".ts", ".tsx", ".js", ".jsx", ".mts", ".cts", ".py", ".pyi":
		return true
	default:
		return false
	}
}

func isTypeScriptTelemetryFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ts", ".tsx", ".js", ".jsx", ".mts", ".cts":
		return true
	default:
		return false
	}
}

func isPythonTelemetryFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".py" || ext == ".pyi"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func payloadStr(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	v, _ := payload[key].(string)
	return strings.TrimSpace(v)
}

func retentionDays() int {
	raw := os.Getenv("MI_LSP_RETENTION_DAYS")
	if raw == "" {
		return 30
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days <= 0 {
		return 30
	}
	return days
}
