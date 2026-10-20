package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

type lspDiagnostic struct {
	Range    lspRange `json:"range"`
	Severity int      `json:"severity"`
	Message  string   `json:"message"`
	Source   string   `json:"source"`
	Code     any      `json:"code"`
}
type publishedDiagnostics struct {
	URI         string          `json:"uri"`
	Version     *int            `json:"version"`
	Diagnostics []lspDiagnostic `json:"diagnostics"`
	received    bool
}

// Only a currently requested URI is retained; unrelated workspace publications
// cannot grow a mailbox or satisfy another file's request.
func (t *lspTransport) acceptDiagnostics(raw json.RawMessage) {
	var notification struct {
		Params publishedDiagnostics `json:"params"`
	}
	if json.Unmarshal(raw, &notification) != nil {
		return
	}
	t.diagnosticMu.Lock()
	if _, watching := t.diagnostics[notification.Params.URI]; watching {
		notification.Params.received = true
		t.diagnostics[notification.Params.URI] = notification.Params
	}
	t.diagnosticMu.Unlock()
	select {
	case t.diagnosticWake <- struct{}{}:
	default:
	}
}

func (c *LSPClient) diagnostics(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error) {
	file, err := diagnosticFile(c.workspace.Root, request.Payload)
	if err != nil {
		return model.WorkerResponse{}, err
	}
	uri := buildFileURI(file)
	if c.transport == nil {
		return model.WorkerResponse{}, errors.New("unavailable: diagnostics transport absent")
	}
	c.transport.diagnosticMu.Lock()
	c.transport.diagnostics[uri] = publishedDiagnostics{}
	c.transport.diagnosticMu.Unlock()
	defer func() {
		c.transport.diagnosticMu.Lock()
		delete(c.transport.diagnostics, uri)
		c.transport.diagnosticMu.Unlock()
	}()
	// Always advance the version, even for identical bytes. Cached publications
	// from a previous edit must never become fresh evidence for this invocation.
	if old, ok := c.openDocs[file]; ok {
		old.content = "\x00" // force a didChange without changing disk
		c.openDocs[file] = old
	}
	if err := c.openDocument(file); err != nil {
		return model.WorkerResponse{}, err
	}
	version := c.openDocs[file].version
	capability := bytes.TrimSpace(c.capabilities["diagnosticProvider"])
	if len(capability) > 0 && string(capability) != "false" && string(capability) != "null" {
		raw, err := c.sendRequest("textDocument/diagnostic", map[string]any{"textDocument": map[string]any{"uri": uri}})
		if err != nil {
			return model.WorkerResponse{}, err
		}
		var report struct {
			Kind  string          `json:"kind"`
			Items []lspDiagnostic `json:"items"`
		}
		if err := json.Unmarshal(raw, &report); err != nil {
			return model.WorkerResponse{}, err
		}
		if report.Kind != "full" || report.Items == nil {
			return model.WorkerResponse{}, errors.New("unavailable: full diagnostic report absent")
		}
		response := diagnosticResponse(c.backendName(), file, report.Items)
		if c.backendName() == "rust-analyzer" {
			response.Partial = true
			response.Reason = "native_diagnostics_only"
			response.Warnings = []string{"partial: rust-analyzer native diagnostics only; cargo check/build scripts/proc macros are disabled"}
		}
		return response, nil
	}
	switch c.backendName() {
	case "gopls", "pyright", "rust-analyzer": // documented push-based servers
	default:
		return model.WorkerResponse{}, errors.New("unsupported_operation: backend has no diagnostics protocol")
	}
	for {
		c.transport.diagnosticMu.Lock()
		publication := c.transport.diagnostics[uri]
		c.transport.diagnosticMu.Unlock()
		if publication.received && (publication.Version == nil || *publication.Version == version) {
			response := diagnosticResponse(c.backendName(), file, publication.Diagnostics)
			// Push servers can publish incremental/phase reports, and some omit the
			// document version. There is no protocol completion barrier: never claim
			// a complete clean result from a first publication.
			response.Partial = true
			response.Reason = "push_snapshot_no_completion_barrier"
			response.Warnings = []string{"partial: publishDiagnostics snapshot; backend supplies no completion barrier"}
			if c.backendName() == "rust-analyzer" {
				response.Warnings = append(response.Warnings, "partial: rust-analyzer native diagnostics only; cargo check/build scripts/proc macros are disabled")
			}
			if publication.Version == nil {
				response.Warnings = append(response.Warnings, "partial: diagnostic publication has no document version")
			}
			return response, nil
		}
		select {
		case <-ctx.Done():
			return model.WorkerResponse{}, fmt.Errorf("unavailable: diagnostic publication timeout: %w", ctx.Err())
		case <-c.transport.done:
			return model.WorkerResponse{}, errors.New("unavailable: diagnostics server exited")
		case <-c.transport.diagnosticWake:
		}
	}
}

func diagnosticResponse(backend, file string, diagnostics []lspDiagnostic) model.WorkerResponse {
	items := make([]map[string]any, 0, len(diagnostics))
	for _, d := range diagnostics {
		// An omitted severity must not bury a potentially important error.
		severity := "error"
		switch d.Severity {
		case 2:
			severity = "warning"
		case 3:
			severity = "information"
		case 4:
			severity = "hint"
		}
		item := map[string]any{"path": file, "line": d.Range.Start.Line + 1, "column": d.Range.Start.Character + 1, "severity": severity, "message": d.Message}
		if d.Source != "" {
			item["source"] = d.Source
		}
		if d.Code != nil {
			item["code"] = d.Code
		}
		items = append(items, item)
	}
	return model.WorkerResponse{Ok: true, Backend: backend, Items: items, Stats: model.Stats{Files: 1, Symbols: len(items)}}
}

// Validate again at the worker trust boundary. No process arguments or file
// contents can be supplied through a diagnostics payload.
func diagnosticFile(root string, payload map[string]any) (string, error) {
	file, _ := payload["file"].(string)
	if file == "" || strings.ContainsRune(file, 0) {
		return "", errors.New("invalid_path: file required")
	}
	if !filepath.IsAbs(file) {
		file = filepath.Join(root, filepath.FromSlash(file))
	}
	file = filepath.Clean(file)
	lexicalRoot, e := filepath.Abs(root)
	if e != nil {
		return "", errors.New("invalid_path: workspace root unavailable")
	}
	lexicalRel, e := filepath.Rel(lexicalRoot, file)
	if e != nil || lexicalRel == ".." || strings.HasPrefix(lexicalRel, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid_path: source outside workspace")
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", errors.New("invalid_path: workspace root unavailable")
	}
	physical, err := filepath.EvalSymlinks(file)
	if os.IsNotExist(err) {
		return "", errors.New("file_missing: source no longer exists")
	}
	if err != nil {
		return "", errors.New("unavailable: source path could not be resolved")
	}
	rel, err := filepath.Rel(root, physical)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errors.New("invalid_path: source outside workspace")
	}
	info, err := os.Stat(physical)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("invalid_path: source is not regular")
	}
	if info.Size() > maxLSPDocumentBytes {
		return "", errors.New("work_limit: diagnostic source exceeds 2 MiB")
	}
	return file, nil
}

func (c *TsserverClient) diagnostics(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error) {
	file, err := diagnosticFile(c.workspace.Root, request.Payload)
	if err != nil {
		return model.WorkerResponse{}, err
	}
	// open with current disk bytes so the long-lived server never uses a prior
	// open buffer. close/open is cheaper than a project reload.
	content, err := os.ReadFile(file)
	if err != nil {
		return model.WorkerResponse{}, err
	}
	if _, err := c.sendEvent("close", map[string]any{"file": file}); err != nil {
		return model.WorkerResponse{}, err
	}
	if _, err := c.sendEvent("open", map[string]any{"file": file, "fileContent": string(content), "projectRootPath": c.workspace.Root}); err != nil {
		return model.WorkerResponse{}, err
	}
	result := model.WorkerResponse{Ok: true, Backend: "tsserver", Items: []map[string]any{}, Stats: model.Stats{Files: 1}}
	for stage, command := range []string{"syntacticDiagnosticsSync", "semanticDiagnosticsSync", "suggestionDiagnosticsSync"} {
		failure := func(err error) (model.WorkerResponse, error) {
			if stage == 0 && len(result.Items) == 0 {
				return result, err
			}
			result.Partial = true
			result.Reason = "tsserver_incomplete_diagnostic_stages"
			result.Warnings = []string{"partial: tsserver " + command + " unavailable; retaining diagnostics from completed stages"}
			result.Stats.Symbols = len(result.Items)
			return result, nil
		}
		raw, err := c.sendRequest(ctx, command, map[string]any{"file": file, "includeLinePosition": true})
		if err != nil {
			return failure(err)
		}
		var diagnostics []struct {
			StartLocation tsLocation `json:"startLocation"`
			Message       string     `json:"message"`
			Source        string     `json:"source"`
			Category      string     `json:"category"`
			Code          *int       `json:"code"`
		}
		if string(raw) == "null" || len(raw) == 0 {
			return failure(errors.New("unavailable: tsserver diagnostic report absent"))
		}
		if err := json.Unmarshal(raw, &diagnostics); err != nil {
			return failure(err)
		}
		for _, d := range diagnostics {
			if d.StartLocation.Line < 1 || d.StartLocation.Offset < 1 {
				return failure(errors.New("unavailable: tsserver diagnostic position absent"))
			}
			severity := d.Category
			if severity == "suggestion" || severity == "message" {
				severity = "information"
			}
			item := map[string]any{"path": file, "line": d.StartLocation.Line, "column": d.StartLocation.Offset, "severity": severity, "message": d.Message, "source": firstNonEmpty(d.Source, "tsserver")}
			if d.Code != nil {
				item["code"] = *d.Code
			}
			result.Items = append(result.Items, item)
		}
	}
	result.Stats.Symbols = len(result.Items)
	return result, nil
}
