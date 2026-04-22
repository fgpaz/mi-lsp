package worker

import (
	"bufio"
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
	ServerCmd   string
	ServerArgs  []string
	InitOptions map[string]any
}

// LSPClient implements RuntimeClient via a standard LSP server.
type LSPClient struct {
	config    LSPConfig
	workspace model.WorkspaceRegistration
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	mu        sync.Mutex
	seqID     int
	started   bool
	openDocs  map[string]openedDocument
	openOrder []string
}

const (
	maxLSPDocumentBytes = 2 << 20
	maxLSPOpenDocuments = 32
)

type openedDocument struct {
	modTime time.Time
	size    int64
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
	if c.started {
		return nil
	}
	cmd := exec.Command(c.config.ServerCmd, c.config.ServerArgs...)
	processutil.ConfigureNonInteractiveCommand(cmd)
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
	c.stdout = bufio.NewReaderSize(stdout, 64*1024)

	// Send initialize request
	rootURI := buildFileURI(c.workspace.Root)
	initParams := map[string]any{
		"processId": nil,
		"rootUri":   rootURI,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"hover": map[string]any{
					"contentFormat": []string{"plaintext"},
				},
				"references":  map[string]any{},
				"definition":  map[string]any{},
				"declaration": map[string]any{},
			},
		},
	}
	if c.config.InitOptions != nil {
		initParams["initializationOptions"] = c.config.InitOptions
	}

	_, err = c.sendRequest("initialize", initParams)
	if err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("LSP initialize failed: %w", err)
	}

	// Send initialized notification
	if err := c.sendNotification("initialized", map[string]any{}); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("LSP initialized notification failed: %w", err)
	}

	c.started = true
	return nil
}

// Call dispatches a mi-lsp request to the LSP server.
func (c *LSPClient) Call(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.Start(); err != nil {
		return model.WorkerResponse{}, err
	}

	switch request.Method {
	case "find_refs":
		return c.findReferences(ctx, request)
	case "get_context":
		return c.getContext(ctx, request)
	default:
		return model.WorkerResponse{}, fmt.Errorf("LSP backend does not support method %q", request.Method)
	}
}

// Close shuts down the LSP server.
func (c *LSPClient) Close() error {
	if c.cmd == nil || c.cmd.Process == nil {
		return nil
	}
	if c.started {
		// Try graceful shutdown
		_, _ = c.sendRequest("shutdown", nil)
		_ = c.sendNotification("exit", nil)
	}
	_ = c.stdin.Close()
	err := c.cmd.Process.Kill()
	_, _ = c.cmd.Process.Wait()
	c.cmd = nil
	c.stdin = nil
	c.stdout = nil
	c.started = false
	return err
}

// PID returns the LSP server process ID.
func (c *LSPClient) PID() int {
	if c.cmd == nil || c.cmd.Process == nil {
		return 0
	}
	return c.cmd.Process.Pid
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

	result, err := c.sendRequest("textDocument/references", params)
	if err != nil {
		return model.WorkerResponse{}, err
	}

	var locations []lspLocation
	if result != nil {
		if err := json.Unmarshal(result, &locations); err != nil {
			// Try null result
			locations = nil
		}
	}

	items := make([]map[string]any, 0, len(locations))
	for _, loc := range locations {
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

	result, err := c.sendRequest("textDocument/hover", params)
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
	lineValue, offset := inferLineAndOffset(absoluteFile, lineValue, symbol)
	return absoluteFile, lineValue, offset, symbol, nil
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
	if cached, ok := c.openDocs[key]; ok && cached.size == info.Size() && cached.modTime.Equal(info.ModTime()) {
		c.touchOpenDocument(key)
		return nil
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("cannot read file for didOpen: %w", err)
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
	c.openDocs[key] = openedDocument{modTime: info.ModTime(), size: info.Size()}
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
	default:
		return "plaintext"
	}
}

func (c *LSPClient) backendName() string {
	base := filepath.Base(c.config.ServerCmd)
	if idx := strings.LastIndex(base, "."); idx > 0 {
		base = base[:idx]
	}
	return base
}

func (c *LSPClient) sendRequest(method string, params any) (json.RawMessage, error) {
	c.seqID++
	req := lspRequest{
		JSONRPC: "2.0",
		ID:      c.seqID,
		Method:  method,
		Params:  params,
	}
	if err := writeLSPMessage(c.stdin, req); err != nil {
		return nil, err
	}
	return c.readResponse(c.seqID)
}

func (c *LSPClient) sendNotification(method string, params any) error {
	notif := lspNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return writeLSPMessage(c.stdin, notif)
}

func (c *LSPClient) readResponse(id int) (json.RawMessage, error) {
	for {
		raw, err := readLSPMessage(c.stdout)
		if err != nil {
			return nil, err
		}
		var resp lspResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			continue
		}
		// Skip notifications (no ID)
		if resp.ID == 0 && resp.Method != "" {
			continue
		}
		if resp.ID != id {
			continue
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("LSP error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
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
