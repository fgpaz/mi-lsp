package worker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/processutil"
)

// LSPConfig configures a generic LSP backend.
type LSPConfig struct {
	ServerCmd               string
	ServerArgs              []string
	InitOptions             map[string]any
	Backend                 string
	StrictCapabilities      bool
	RequireExplicitPosition bool
	ExpectedServerVersion   string
}

// LSPClient implements RuntimeClient via a standard LSP server.
type LSPClient struct {
	config             LSPConfig
	workspace          model.WorkspaceRegistration
	cmd                *exec.Cmd
	stdin              io.WriteCloser
	stdout             *bufio.Reader // In-memory protocol injection; live processes use transport.
	transportMu        sync.Mutex
	transport          *lspTransport
	requestCtx         context.Context
	capabilities       map[string]json.RawMessage
	mu                 sync.Mutex
	seqID              int
	started            bool
	semanticWarmupDone bool
	openDocs           map[string]openedDocument
	openOrder          []string
}

const (
	maxLSPDocumentBytes = 2 << 20
	maxLSPOpenDocuments = 32
)

var lspContentModified = errors.New("LSP content modified")

var lspContentModifiedRetryBackoff = []time.Duration{
	100 * time.Millisecond,
	200 * time.Millisecond,
	400 * time.Millisecond,
	800 * time.Millisecond,
	1600 * time.Millisecond,
}

var rustAnalyzerWarmupBackoff = []time.Duration{
	250 * time.Millisecond,
	500 * time.Millisecond,
	1000 * time.Millisecond,
	2000 * time.Millisecond,
	4000 * time.Millisecond,
}

type openedDocument struct {
	modTime time.Time
	size    int64
	version int
	content string
}

// NewLSPClient creates a new generic LSP client.
func NewLSPClient(config LSPConfig, workspace model.WorkspaceRegistration) *LSPClient {
	return &LSPClient{
		config:    config,
		workspace: workspace,
	}
}

type lspRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type lspResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *lspError       `json:"error,omitempty"`
}

type lspError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type lspNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// Start spawns the LSP server and performs the initialize/initialized handshake.
func (c *LSPClient) Start() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.startContext(ctx)
}

func (c *LSPClient) startContext(ctx context.Context) error {
	c.requestCtx = ctx
	if c.transport != nil && !c.transport.alive() {
		c.started = false
		c.openDocs = nil
		c.openOrder = nil
		c.semanticWarmupDone = false
	}
	if c.started {
		return nil
	}
	cmd := processutil.Command(c.config.ServerCmd, c.config.ServerArgs...)
	processutil.ConfigureNonInteractiveCommand(cmd)
	configureOwnedLSP(cmd)
	bindWorkerLifetime(cmd)
	cmd.Dir = c.workspace.Root
	if c.config.Backend == KotlinBackend {
		cmd.Env = append(os.Environ(), "GRADLE_OPTS="+os.Getenv("GRADLE_OPTS")+" -Dorg.gradle.daemon=false")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = nil // suppress stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start LSP server %s: %w", c.config.ServerCmd, err)
	}
	c.cmd = cmd
	c.stdin = stdin
	c.transportMu.Lock()
	c.transport = newLSPTransport(cmd, stdin, stdout)
	c.transportMu.Unlock()

	// Send initialize request
	rootURI := buildFileURI(c.workspace.Root)
	initParams := map[string]any{
		"processId":        nil,
		"rootUri":          rootURI,
		"workspaceFolders": []map[string]any{{"uri": rootURI, "name": c.workspace.Name}},
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"hover": map[string]any{
					"contentFormat": []string{"plaintext"},
				},
				"references":         map[string]any{},
				"definition":         map[string]any{},
				"declaration":        map[string]any{},
				"diagnostic":         map[string]any{"dynamicRegistration": false},
				"publishDiagnostics": map[string]any{"versionSupport": true},
			},
		},
	}
	if c.config.InitOptions != nil {
		initParams["initializationOptions"] = c.config.InitOptions
	}

	result, err := c.sendRequest("initialize", initParams)
	if err != nil {
		c.transport.stop()
		return fmt.Errorf("LSP initialize failed: %w", err)
	}

	var initialized struct {
		Capabilities map[string]json.RawMessage `json:"capabilities"`
		ServerInfo   struct {
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(result, &initialized); err != nil {
		c.transport.stop()
		return err
	}
	if c.config.ExpectedServerVersion != "" && initialized.ServerInfo.Version != c.config.ExpectedServerVersion {
		c.transport.stop()
		return errors.New("unavailable: incompatible LSP server version")
	}
	c.capabilities = initialized.Capabilities
	// Send initialized notification
	if err := c.sendNotification("initialized", map[string]any{}); err != nil {
		c.transport.stop()
		return fmt.Errorf("LSP initialized notification failed: %w", err)
	}

	c.started = true
	return nil
}

// Call dispatches a mi-lsp request to the LSP server.
func (c *LSPClient) Call(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error) {
	for !c.mu.TryLock() {
		select {
		case <-ctx.Done():
			return model.WorkerResponse{}, ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
	defer c.mu.Unlock()
	c.requestCtx = ctx

	if err := c.startContext(ctx); err != nil {
		return model.WorkerResponse{}, err
	}

	switch request.Method {
	case "diagnostics":
		return c.diagnostics(ctx, request)
	case "find_refs":
		return c.findReferences(ctx, request)
	case "get_context":
		return c.getContext(ctx, request)
	case "find_definition":
		return c.definition(ctx, request)
	case "document_symbols":
		return c.documentSymbols(ctx, request)
	default:
		return model.WorkerResponse{}, fmt.Errorf("LSP backend does not support method %q", request.Method)
	}
}

// Close shuts down the LSP server.
func (c *LSPClient) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for !c.mu.TryLock() {
		select {
		case <-ctx.Done():
			c.transportMu.Lock()
			if c.transport != nil {
				c.transport.stop()
			}
			c.transportMu.Unlock()
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
	defer c.mu.Unlock()
	c.requestCtx = ctx
	if c.transport != nil {
		if c.started && c.transport.alive() {
			for file := range c.openDocs {
				_ = c.sendNotification("textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": buildFileURI(file)}})
			}
			_, _ = c.sendRequest("shutdown", nil)
			_ = c.sendNotification("exit", nil)
		}
		c.transport.stop()
	}
	c.started = false
	c.openDocs = nil
	c.openOrder = nil
	return nil
}

// PID returns the LSP server process ID.
func (c *LSPClient) PID() int {
	c.transportMu.Lock()
	defer c.transportMu.Unlock()
	if c.transport == nil || !c.transport.alive() {
		return 0
	}
	return c.transport.cmd.Process.Pid
}

func (c *LSPClient) findReferences(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error) {
	file, line, col, symbol, err := c.resolveLocation(request)
	if err != nil {
		return model.WorkerResponse{}, err
	}

	if err := c.openDocument(file); err != nil {
		return model.WorkerResponse{}, err
	}

	params := map[string]any{
		"textDocument": map[string]any{"uri": buildFileURI(file)},
		"position":     lspPositionMap(line, col),
		"context":      map[string]any{"includeDeclaration": true},
	}

	result, err := c.sendSemanticRequest("textDocument/references", params)
	if err != nil {
		return model.WorkerResponse{}, err
	}

	var locations []lspLocation
	if result != nil {
		if err := json.Unmarshal(result, &locations); err != nil {
			// Try null result
			return model.WorkerResponse{}, fmt.Errorf("invalid LSP references response: %w", err)
		}
	}

	items := make([]map[string]any, 0, len(locations))
	for _, loc := range locations {
		if !strings.HasPrefix(loc.URI, "file://") {
			return model.WorkerResponse{}, errors.New("unsupported_operation: non-file reference URI")
		}
		items = append(items, map[string]any{
			"name":   symbol,
			"file":   filepath.Clean(uriToPath(loc.URI)),
			"line":   loc.Range.Start.Line + 1,
			"column": loc.Range.Start.Character + 1,
		})
	}

	return model.WorkerResponse{
		Ok:      true,
		Backend: c.backendName(),
		Items:   items,
		Stats:   model.Stats{Symbols: len(items)},
	}, nil
}

func (c *LSPClient) getContext(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error) {
	file, line, col, _, err := c.resolveLocation(request)
	if err != nil {
		return model.WorkerResponse{}, err
	}

	if err := c.openDocument(file); err != nil {
		return model.WorkerResponse{}, err
	}

	params := map[string]any{
		"textDocument": map[string]any{"uri": buildFileURI(file)},
		"position":     lspPositionMap(line, col),
	}

	result, err := c.sendSemanticRequest("textDocument/hover", params)
	if err != nil {
		return model.WorkerResponse{}, err
	}

	items := []map[string]any{}
	if result != nil && string(result) != "null" {
		var hover lspHoverResult
		if err := json.Unmarshal(result, &hover); err == nil && hover.Contents.Value != "" {
			items = append(items, map[string]any{
				"file":      filepath.Clean(file),
				"line":      line,
				"column":    col,
				"signature": hover.Contents.Value,
				"kind":      hover.Contents.Kind,
			})
		}
	}

	return model.WorkerResponse{
		Ok:      true,
		Backend: c.backendName(),
		Items:   items,
		Stats:   model.Stats{Symbols: len(items)},
	}, nil
}

func (c *LSPClient) resolveLocation(request model.WorkerRequest) (string, int, int, string, error) {
	fileValue, _ := request.Payload["file"].(string)
	if fileValue == "" {
		return "", 0, 0, "", errors.New("file is required for LSP queries")
	}
	absoluteFile := fileValue
	if !filepath.IsAbs(absoluteFile) {
		absoluteFile = filepath.Join(c.workspace.Root, filepath.FromSlash(fileValue))
	}
	lineValue := intFromPayload(request.Payload, "line", 1)
	symbol, _ := request.Payload["symbol"].(string)
	if c.config.RequireExplicitPosition {
		if !strings.EqualFold(filepath.Ext(absoluteFile), ".kt") {
			return "", 0, 0, "", errors.New("unsupported_operation: Kotlin scripts are excluded")
		}
		if _, ok := request.Payload["line"]; !ok {
			return "", 0, 0, "", errors.New("explicit line is required")
		}
		if _, ok := request.Payload["column"]; !ok {
			return "", 0, 0, "", errors.New("explicit UTF-16 column is required")
		}
		column := intFromPayload(request.Payload, "column", 0)
		if lineValue < 1 || column < 1 {
			return "", 0, 0, "", errors.New("invalid UTF-16 position")
		}
		if err := validateUTF16Position(absoluteFile, lineValue, column); err != nil {
			return "", 0, 0, "", err
		}
		return filepath.Clean(absoluteFile), lineValue, column, symbol, nil
	}
	lineValue, offset := inferLineAndOffset(absoluteFile, lineValue, symbol)
	offset, err := utf16ColumnForByteOffset(absoluteFile, lineValue, offset)
	return absoluteFile, lineValue, offset, symbol, err
}

func (c *LSPClient) openDocument(filePath string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("cannot stat file for didOpen: %w", err)
	}
	if info.Size() > maxLSPDocumentBytes {
		return fmt.Errorf("file too large for LSP didOpen: %d bytes exceeds %d", info.Size(), maxLSPDocumentBytes)
	}
	if c.openDocs == nil {
		c.openDocs = map[string]openedDocument{}
	}
	key := filepath.Clean(filePath)
	content, err := readKotlinMetadata(filePath)
	if err != nil {
		return fmt.Errorf("cannot read file for didOpen: %w", err)
	}
	if cached, ok := c.openDocs[key]; ok {
		if cached.content == string(content) {
			c.touchOpenDocument(key)
			return nil
		}
		version := cached.version + 1
		params := map[string]any{"textDocument": map[string]any{"uri": buildFileURI(filePath), "version": version}, "contentChanges": []map[string]any{{"text": string(content)}}}
		if err := c.sendNotification("textDocument/didChange", params); err != nil {
			return err
		}
		c.openDocs[key] = openedDocument{modTime: info.ModTime(), size: info.Size(), version: version, content: string(content)}
		c.touchOpenDocument(key)
		return nil
	}
	params := map[string]any{
		"textDocument": map[string]any{
			"uri":        buildFileURI(filePath),
			"languageId": languageIDForPath(filePath),
			"version":    1,
			"text":       string(content),
		},
	}
	if err := c.sendNotification("textDocument/didOpen", params); err != nil {
		return err
	}
	c.openDocs[key] = openedDocument{modTime: info.ModTime(), size: info.Size(), version: 1, content: string(content)}
	c.touchOpenDocument(key)
	c.evictOpenDocuments()
	return nil
}

func (c *LSPClient) touchOpenDocument(key string) {
	for i, existing := range c.openOrder {
		if existing == key {
			copy(c.openOrder[i:], c.openOrder[i+1:])
			c.openOrder[len(c.openOrder)-1] = key
			return
		}
	}
	c.openOrder = append(c.openOrder, key)
}

func (c *LSPClient) evictOpenDocuments() {
	for len(c.openOrder) > maxLSPOpenDocuments {
		victim := c.openOrder[0]
		c.openOrder = append([]string(nil), c.openOrder[1:]...)
		_ = c.sendNotification("textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": buildFileURI(victim)}})
		delete(c.openDocs, victim)
	}
}

func languageIDForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".py":
		return "python"
	case ".pyi":
		return "python"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".cs":
		return "csharp"
	case ".go":
		return "go"
	case ".rs":
		return "rust"
	case ".kt":
		return "kotlin"
	default:
		return "plaintext"
	}
}

func (c *LSPClient) backendName() string {
	if c.config.Backend != "" {
		return c.config.Backend
	}
	base := filepath.Base(c.config.ServerCmd)
	if idx := strings.LastIndex(base, "."); idx > 0 {
		base = base[:idx]
	}
	return base
}

func (c *LSPClient) sendSemanticRequest(method string, params any) (json.RawMessage, error) {
	if c.config.StrictCapabilities {
		capability := map[string]string{"textDocument/references": "referencesProvider", "textDocument/definition": "definitionProvider", "textDocument/hover": "hoverProvider", "textDocument/documentSymbol": "documentSymbolProvider"}[method]
		value := bytes.TrimSpace(c.capabilities[capability])
		if capability == "" || len(value) == 0 || bytes.Equal(value, []byte("false")) || bytes.Equal(value, []byte("null")) {
			return nil, fmt.Errorf("unsupported_operation: %s not advertised", method)
		}
	}
	result, err := c.sendRequest(method, params)
	if c.semanticWarmupDone || c.backendName() != "rust-analyzer" {
		return result, err
	}
	c.semanticWarmupDone = true
	warmupCtx := c.requestCtx
	if warmupCtx == nil {
		warmupCtx = context.Background()
	}
	for _, delay := range rustAnalyzerWarmupBackoff {
		if err != nil || !emptyLSPQueryResult(method, result) {
			return result, err
		}
		select {
		case <-warmupCtx.Done():
			return nil, warmupCtx.Err()
		case <-time.After(delay):
		}
		result, err = c.sendRequest(method, params)
	}
	return result, err
}

func emptyLSPQueryResult(method string, result json.RawMessage) bool {
	trimmed := bytes.TrimSpace(result)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return true
	}
	switch method {
	case "textDocument/references":
		var locations []json.RawMessage
		return json.Unmarshal(trimmed, &locations) == nil && len(locations) == 0
	case "textDocument/hover":
		var hover lspHoverResult
		return json.Unmarshal(trimmed, &hover) == nil && strings.TrimSpace(hover.Contents.Value) == ""
	default:
		return false
	}
}

func (c *LSPClient) sendRequest(method string, params any) (json.RawMessage, error) {
	ctx := c.requestCtx
	if ctx == nil {
		ctx = context.Background()
	}
	for attempt := 0; ; attempt++ {
		c.seqID++
		var result json.RawMessage
		var err error
		if c.transport != nil {
			result, err = c.transport.request(ctx, c.seqID, method, params)
		} else {
			if c.stdin == nil || c.stdout == nil {
				return nil, errors.New("LSP transport unavailable")
			}
			err = writeLSPMessage(c.stdin, lspRequest{JSONRPC: "2.0", ID: c.seqID, Method: method, Params: params})
			if err == nil {
				result, err = c.readResponse(c.seqID)
			}
		}
		if !errors.Is(err, lspContentModified) || attempt >= len(lspContentModifiedRetryBackoff) {
			return result, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(lspContentModifiedRetryBackoff[attempt]):
		}
	}
}
func (c *LSPClient) sendNotification(method string, params any) error {
	ctx := c.requestCtx
	if ctx == nil {
		ctx = context.Background()
	}
	if c.transport == nil {
		return errors.New("LSP transport unavailable")
	}
	return c.transport.write(ctx, lspNotification{JSONRPC: "2.0", Method: method, Params: params})
}

// readResponse retains the bounded in-memory protocol seam. Runtime I/O is
// always owned by lspTransport and obeys the original request context.
func (c *LSPClient) readResponse(id int) (json.RawMessage, error) {
	for {
		raw, err := readLSPMessage(c.stdout)
		if err != nil {
			return nil, err
		}
		var response lspResponse
		if err = json.Unmarshal(raw, &response); err != nil {
			return nil, err
		}
		if response.ID != id {
			continue
		}
		if response.Error != nil {
			if response.Error.Code == -32801 {
				return nil, lspContentModified
			}
			return nil, errors.New("LSP request failed")
		}
		return response.Result, nil
	}
}

// LSP types for deserialization
type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}

type lspLocation struct {
	URI   string   `json:"uri"`
	Range lspRange `json:"range"`
}

type lspHoverResult struct {
	Contents lspMarkupContent `json:"contents"`
}

type lspMarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}
