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
	"strconv"
	"strings"
	"sync"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/processutil"
)

type TsserverClient struct {
	workspace  model.WorkspaceRegistration
	tsserverJS string
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	reader     *bufio.Reader
	mu         sync.Mutex
	sequence   int
}

type tsserverEnvelope struct {
	Seq        int             `json:"seq"`
	Type       string          `json:"type"`
	Command    string          `json:"command,omitempty"`
	RequestSeq int             `json:"request_seq,omitempty"`
	Success    bool            `json:"success,omitempty"`
	Message    string          `json:"message,omitempty"`
	Body       json.RawMessage `json:"body,omitempty"`
	Event      string          `json:"event,omitempty"`
}

type tsLocation struct {
	Line   int `json:"line"`
	Offset int `json:"offset"`
}

type tsReferencesBody struct {
	Refs []struct {
		File         string     `json:"file"`
		Start        tsLocation `json:"start"`
		End          tsLocation `json:"end"`
		LineText     string     `json:"lineText"`
		IsDefinition bool       `json:"isDefinition"`
	} `json:"refs"`
	SymbolName string `json:"symbolName"`
}

type tsQuickInfoBody struct {
	Kind          string     `json:"kind"`
	KindModifiers string     `json:"kindModifiers"`
	Start         tsLocation `json:"start"`
	End           tsLocation `json:"end"`
	DisplayString string     `json:"displayString"`
	Documentation any        `json:"documentation"`
}

func NewTsserverClient(workspace model.WorkspaceRegistration) (*TsserverClient, error) {
	tsserverPath, err := findTsserverPath(workspace.Root)
	if err != nil {
		return nil, err
	}
	return &TsserverClient{workspace: workspace, tsserverJS: tsserverPath}, nil
}

func CanUseTsserver(workspaceRoot string) bool {
	if _, err := exec.LookPath("node"); err != nil {
		return false
	}
	_, err := findTsserverPath(workspaceRoot)
	return err == nil
}

func (c *TsserverClient) Start() error {
	if c.cmd != nil {
		return nil
	}
	if _, err := exec.LookPath("node"); err != nil {
		return errors.New("node is required for tsserver backend")
	}
	cmd := exec.Command("node", c.tsserverJS, "--useSingleInferredProjectPerProjectRoot", "true")
	processutil.ConfigureNonInteractiveCommand(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	c.cmd = cmd
	c.stdin = stdin
	c.stdout = stdout
	c.reader = bufio.NewReader(stdout)
	return nil
}

func (c *TsserverClient) Call(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error) {
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
		return model.WorkerResponse{}, fmt.Errorf("tsserver backend does not support method %q", request.Method)
	}
}

func (c *TsserverClient) Close() error {
	if c.cmd == nil || c.cmd.Process == nil {
		return nil
	}
	_ = c.stdin.Close()
	err := c.cmd.Process.Kill()
	_, _ = c.cmd.Process.Wait()
	c.cmd = nil
	c.stdin = nil
	c.stdout = nil
	c.reader = nil
	return err
}

func (c *TsserverClient) PID() int {
	if c.cmd == nil || c.cmd.Process == nil {
		return 0
	}
	return c.cmd.Process.Pid
}

func (c *TsserverClient) findReferences(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error) {
	file, line, offset, symbol, err := c.requestLocation(request.Payload)
	if err != nil {
		return model.WorkerResponse{}, err
	}
	if err := c.openFile(file); err != nil {
		return model.WorkerResponse{}, err
	}
	body, err := c.sendRequest(ctx, "references", map[string]any{"file": file, "line": line, "offset": offset})
	if err != nil {
		return model.WorkerResponse{}, err
	}
	var payload tsReferencesBody
	if err := json.Unmarshal(body, &payload); err != nil {
		return model.WorkerResponse{}, err
	}
	items := make([]map[string]any, 0, len(payload.Refs))
	for _, ref := range payload.Refs {
		items = append(items, map[string]any{
			"name":       firstNonEmpty(payload.SymbolName, symbol),
			"file":       filepath.Clean(ref.File),
			"line":       ref.Start.Line,
			"column":     ref.Start.Offset,
			"text":       ref.LineText,
			"definition": ref.IsDefinition,
		})
	}
	return model.WorkerResponse{Ok: true, Backend: "tsserver", Items: items, Stats: model.Stats{Symbols: len(items)}}, nil
}

func (c *TsserverClient) getContext(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error) {
	file, line, offset, _, err := c.requestLocation(request.Payload)
	if err != nil {
		return model.WorkerResponse{}, err
	}
	if err := c.openFile(file); err != nil {
		return model.WorkerResponse{}, err
	}
	body, err := c.sendRequest(ctx, "quickinfo", map[string]any{"file": file, "line": line, "offset": offset})
	if err != nil {
		return model.WorkerResponse{}, err
	}
	var payload tsQuickInfoBody
	if err := json.Unmarshal(body, &payload); err != nil {
		return model.WorkerResponse{}, err
	}
	items := []map[string]any{{
		"file":          filepath.Clean(file),
		"line":          payload.Start.Line,
		"column":        payload.Start.Offset,
		"kind":          payload.Kind,
		"scope":         payload.KindModifiers,
		"signature":     payload.DisplayString,
		"documentation": formatDocumentation(payload.Documentation),
	}}
	return model.WorkerResponse{Ok: true, Backend: "tsserver", Items: items, Stats: model.Stats{Symbols: len(items)}}, nil
}

func (c *TsserverClient) requestLocation(payload map[string]any) (string, int, int, string, error) {
	fileValue, _ := payload["file"].(string)
	if fileValue == "" {
		return "", 0, 0, "", errors.New("file is required for tsserver queries")
	}
	absoluteFile := fileValue
	if !filepath.IsAbs(absoluteFile) {
		absoluteFile = filepath.Join(c.workspace.Root, filepath.FromSlash(fileValue))
	}
	lineValue := intFromPayload(payload, "line", 1)
	symbol, _ := payload["symbol"].(string)
	lineValue, offset := inferLineAndOffset(absoluteFile, lineValue, symbol)
	return absoluteFile, lineValue, offset, symbol, nil
}

func (c *TsserverClient) openFile(filePath string) error {
	_, err := c.sendEvent("open", map[string]any{"file": filePath})
	return err
}

func (c *TsserverClient) sendEvent(command string, arguments map[string]any) (int, error) {
	c.sequence++
	message := map[string]any{
		"seq":       c.sequence,
		"type":      "request",
		"command":   command,
		"arguments": arguments,
	}
	return c.sequence, writeTSFrame(c.stdin, message)
}

func (c *TsserverClient) sendRequest(ctx context.Context, command string, arguments map[string]any) (json.RawMessage, error) {
	seq, err := c.sendEvent(command, arguments)
	if err != nil {
		return nil, err
	}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		message, err := readTSFrame(c.reader)
		if err != nil {
			return nil, err
		}
		if message.Type != "response" || message.RequestSeq != seq {
			continue
		}
		if !message.Success {
			return nil, errors.New(strings.TrimSpace(message.Message))
		}
		return message.Body, nil
	}
}

func writeTSFrame(writer io.Writer, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := writer.Write([]byte(header)); err != nil {
		return err
	}
	_, err = writer.Write(body)
	return err
}

func readTSFrame(reader *bufio.Reader) (tsserverEnvelope, error) {
	contentLength := 0
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return tsserverEnvelope{}, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLength, err = strconv.Atoi(value)
			if err != nil {
				return tsserverEnvelope{}, err
			}
		}
	}
	if contentLength <= 0 {
		return tsserverEnvelope{}, errors.New("tsserver returned invalid content length")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		return tsserverEnvelope{}, err
	}
	var envelope tsserverEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return tsserverEnvelope{}, err
	}
	return envelope, nil
}

func findTsserverPath(workspaceRoot string) (string, error) {
	for current := workspaceRoot; current != ""; current = filepath.Dir(current) {
		candidate := filepath.Join(current, "node_modules", "typescript", "lib", "tsserver.js")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		next := filepath.Dir(current)
		if next == current {
			break
		}
	}
	globalRoot, err := globalNpmRoot()
	if err == nil {
		candidate := filepath.Join(strings.TrimSpace(globalRoot), "typescript", "lib", "tsserver.js")
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
	}
	return "", errors.New("tsserver is unavailable; install typescript locally or globally")
}

func globalNpmRoot() (string, error) {
	command := exec.Command("npm", "root", "-g")
	processutil.ConfigureNonInteractiveCommand(command)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func intFromPayload(payload map[string]any, key string, defaultValue int) int {
	switch value := payload[key].(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return defaultValue
	}
}

func inferOffset(filePath string, lineNumber int, symbol string) int {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return 1
	}
	lines := strings.Split(string(content), "\n")
	if lineNumber <= 0 || lineNumber > len(lines) {
		return 1
	}
	line := strings.TrimRight(lines[lineNumber-1], "\r")
	if symbol != "" {
		if index := strings.Index(line, symbol); index >= 0 {
			return index + 1
		}
	}
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" {
		return 1
	}
	return len(line) - len(trimmed) + 1
}

func formatDocumentation(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, fmt.Sprintf("%v", item))
		}
		return strings.Join(parts, " ")
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
