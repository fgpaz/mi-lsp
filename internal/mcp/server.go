// Package mcp is a stdio MCP codec for mi-lsp nav tools.
// The loop speaks newline-delimited JSON-RPC 2.0 and does not open the index.
// tools/call execs this binary, or MI_LSP_BIN, with an argv vector. It never
// starts a shell and does not import the service, indexer, or store.
package mcp

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
	"strings"
	"unicode/utf8"
)

const (
	serverName     = "mi-lsp"
	serverVersion  = "mi-lsp-v1.1"
	maxRawBytes    = 4_000_000
	maxDetailRunes = 300
	codeParseError = -32700
	codeInvalid    = -32600
	codeMethod     = -32601
	codeParams     = -32602
	codeInternal   = -32603
)

// ExecFunc runs a prepared argv vector. bin is the executable path, never a shell.
type ExecFunc func(ctx context.Context, bin string, argv []string) ExecResult

// ExecResult is the captured child outcome. Stdout and Stderr are capped.
type ExecResult struct {
	Stdout      string
	Stderr      string
	Code        int
	TimedOut    bool
	SpawnFailed bool
	Truncated   bool
}

// ServeConfig wires the stdio codec. Exec and Bin are optional.
// A nil Exec uses DefaultExec. An empty Bin resolves MI_LSP_BIN or os.Executable.
type ServeConfig struct {
	In   io.Reader
	Out  io.Writer
	Bin  string
	Exec ExecFunc
}

type rpcMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params"`
}

type rpcErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcErrorBody   `json:"error,omitempty"`
}

type initializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    capabilities `json:"capabilities"`
	ServerInfo      serverInfo   `json:"serverInfo"`
}

type capabilities struct {
	Tools map[string]any `json:"tools"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type toolCallResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Serve reads MCP messages until stdin EOF. It keeps no session beyond the loop.
func Serve(ctx context.Context, cfg ServeConfig) error {
	if ctx == nil {
		ctx = context.Background()
	}
	in := cfg.In
	if in == nil {
		in = os.Stdin
	}
	out := cfg.Out
	if out == nil {
		out = os.Stdout
	}
	execFn := cfg.Exec
	if execFn == nil {
		execFn = DefaultExec
	}
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	session := &stdioSession{enc: enc, bin: cfg.Bin, exec: execFn}
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := strings.TrimPrefix(strings.TrimSpace(scanner.Text()), "\uFEFF")
		if line == "" {
			continue
		}
		if err := session.handle(ctx, line); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		_ = session.writeError(json.RawMessage("null"), codeParseError, "Parse error")
		return err
	}
	return nil
}

type stdioSession struct {
	enc  *json.Encoder
	bin  string
	exec ExecFunc
}

func (s *stdioSession) handle(ctx context.Context, line string) error {
	var msg rpcMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return s.writeError(json.RawMessage("null"), codeParseError, "Parse error")
	}
	if msg.JSONRPC != "" && msg.JSONRPC != "2.0" {
		return s.writeError(responseID(msg.ID), codeInvalid, "Invalid request")
	}
	if msg.Method == "" {
		return s.writeError(responseID(msg.ID), codeInvalid, "Invalid request")
	}
	switch msg.Method {
	case "initialize":
		if msg.ID == nil {
			return nil
		}
		return s.writeResult(*msg.ID, initializeResult{
			ProtocolVersion: negotiateProtocol(msg.Params),
			Capabilities:    capabilities{Tools: map[string]any{}},
			ServerInfo:      serverInfo{Name: serverName, Version: serverVersion},
		})
	case "notifications/initialized", "initialized":
		return nil
	case "ping":
		if msg.ID == nil {
			return nil
		}
		return s.writeResult(*msg.ID, map[string]any{})
	case "tools/list":
		if msg.ID == nil {
			return nil
		}
		tools, err := ToolList()
		if err != nil {
			return s.writeError(*msg.ID, codeInternal, "Internal error")
		}
		return s.writeResult(*msg.ID, map[string]any{"tools": tools})
	case "tools/call":
		if msg.ID == nil {
			return nil
		}
		result, err := s.callTool(ctx, msg.Params)
		if err != nil {
			return s.writeError(*msg.ID, codeParams, "Invalid params")
		}
		return s.writeResult(*msg.ID, result)
	default:
		if msg.ID == nil {
			return nil
		}
		return s.writeError(*msg.ID, codeMethod, "Method not found: "+displayName(msg.Method))
	}
}

func (s *stdioSession) callTool(ctx context.Context, params json.RawMessage) (toolCallResult, error) {
	name, args, err := decodeCall(params)
	if err != nil {
		return toolCallResult{}, err
	}
	argv, err := BuildArgv(name, args)
	if err != nil {
		if errors.Is(err, ErrUnknownTool) {
			label := displayName(name)
			if label == "" {
				label = "(empty)"
			}
			return textResult("Unknown tool: "+label, true), nil
		}
		return textResult("unsupported_operation: "+sanitizeDetail(err.Error()), true), nil
	}
	bin := s.bin
	if strings.TrimSpace(bin) == "" {
		resolved, resolveErr := ResolveBinary()
		if resolveErr != nil {
			return resultFromExec(ExecResult{Code: -1, SpawnFailed: true, Stderr: resolveErr.Error()}), nil
		}
		bin = resolved
	}
	if err := refuseShell(bin); err != nil {
		return resultFromExec(ExecResult{Code: -1, SpawnFailed: true, Stderr: err.Error()}), nil
	}
	if err := ctx.Err(); err != nil {
		return resultFromExec(ExecResult{Code: -1, Stderr: "request cancelled"}), nil
	}
	return resultFromExec(s.exec(ctx, bin, argv)), nil
}

func decodeCall(raw json.RawMessage) (string, map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", nil, errors.New("missing params")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := dec.Decode(&params); err != nil {
		return "", nil, err
	}
	args, err := decodeArguments(params.Arguments)
	if err != nil {
		return "", nil, err
	}
	return params.Name, args, nil
}

func decodeArguments(raw json.RawMessage) (map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return map[string]any{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var args map[string]any
	if err := dec.Decode(&args); err != nil {
		return nil, err
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, errors.New("invalid arguments")
	}
	if args == nil {
		args = map[string]any{}
	}
	return args, nil
}

func negotiateProtocol(raw json.RawMessage) string {
	const latest = "2025-06-18"
	supported := map[string]bool{
		"2024-11-05": true,
		"2025-03-26": true,
		"2025-06-18": true,
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return latest
	}
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(raw, &params); err != nil || params.ProtocolVersion == "" {
		return latest
	}
	if supported[params.ProtocolVersion] {
		return params.ProtocolVersion
	}
	return latest
}

func resultFromExec(res ExecResult) toolCallResult {
	if res.Code == 0 && !res.SpawnFailed && !res.TimedOut {
		text := res.Stdout
		if res.Truncated {
			text += "\n[truncated]"
		}
		return textResult(text, false)
	}
	code, detail := classify(res)
	return textResult(reasonJSON(code, detail), true)
}

func textResult(text string, isError bool) toolCallResult {
	return toolCallResult{
		Content: []toolContent{{Type: "text", Text: text}},
		IsError: isError,
	}
}

func classify(res ExecResult) (string, string) {
	detail := sanitizeDetail(res.Stderr)
	lower := strings.ToLower(res.Stderr)
	switch {
	case res.SpawnFailed:
		if detail == "" {
			detail = "mi-lsp binary could not be started"
		}
		return "unavailable_binary", detail
	case res.TimedOut:
		return "explicit_incomplete", "mi-lsp did not return before the timeout"
	case containsAny(lower, "unknown command", "unknown flag", "unrecognized", "invalid argument for"):
		if detail == "" {
			detail = "mi-lsp rejected the requested operation or flag"
		}
		return "unsupported_operation", detail
	case strings.Contains(lower, "workspace") && containsAny(lower, "not found", "unknown", "unresolved", "invalid", "no such"):
		if detail == "" {
			detail = "mi-lsp could not resolve the requested workspace"
		}
		return "invalid_workspace", detail
	case containsAny(lower, "not indexed", "not registered", "no project.toml", "not a mi-lsp workspace"):
		if detail == "" {
			detail = "cwd is not an indexed mi-lsp workspace"
		}
		return "invalid_workspace", detail
	default:
		if detail == "" {
			detail = fmt.Sprintf("mi-lsp exited with code %d", res.Code)
		}
		return "explicit_incomplete", detail
	}
}

func reasonJSON(code, detail string) string {
	payload, err := json.Marshal(struct {
		ReasonCode string `json:"reasonCode"`
		Detail     string `json:"detail"`
	}{code, detail})
	if err != nil {
		return `{"reasonCode":"explicit_incomplete","detail":"mi-lsp failed"}`
	}
	return string(payload)
}

func containsAny(text string, parts ...string) bool {
	for _, part := range parts {
		if strings.Contains(text, part) {
			return true
		}
	}
	return false
}

func sanitizeDetail(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	return trimRunes(text, maxDetailRunes)
}

func displayName(name string) string {
	return trimRunes(strings.Join(strings.Fields(name), " "), 80)
}

func trimRunes(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	index := 0
	for runes := 0; runes < limit && index < len(text); runes++ {
		_, size := utf8.DecodeRuneInString(text[index:])
		index += size
	}
	return text[:index]
}

func (s *stdioSession) writeResult(id json.RawMessage, result any) error {
	// An empty result object must survive omitempty, which drops map[string]any{}.
	raw, err := marshalNoHTML(result)
	if err != nil {
		return err
	}
	return s.enc.Encode(rpcResponse{JSONRPC: "2.0", ID: responseRaw(id), Result: raw})
}

func marshalNoHTML(value any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func (s *stdioSession) writeError(id json.RawMessage, code int, message string) error {
	return s.enc.Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      responseRaw(id),
		Error:   &rpcErrorBody{Code: code, Message: message},
	})
}

func responseID(id *json.RawMessage) json.RawMessage {
	if id == nil {
		return json.RawMessage("null")
	}
	return *id
}

func responseRaw(id json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(id)) == 0 {
		return json.RawMessage("null")
	}
	return id
}

// ResolveBinary returns MI_LSP_BIN when it is set, otherwise this executable.
// Shell interpreters and cmd/bat script hosts are refused.
func ResolveBinary() (string, error) {
	if bin := strings.TrimSpace(os.Getenv("MI_LSP_BIN")); bin != "" {
		if err := refuseShell(bin); err != nil {
			return "", err
		}
		return bin, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if err := refuseShell(exe); err != nil {
		return "", err
	}
	return exe, nil
}

// NewCommand builds an argv exec.Cmd. It does not run the process and does not
// insert a shell, cmd.exe, or sh.
func NewCommand(ctx context.Context, bin string, argv []string) (*exec.Cmd, error) {
	if err := refuseShell(bin); err != nil {
		return nil, err
	}
	if strings.TrimSpace(bin) == "" {
		return nil, errors.New("binary path is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// argv... passes the slice as separate arguments. It does not join a command line.
	cmd := exec.CommandContext(ctx, bin, argv...)
	cmd.Stdin = nil
	configureChild(cmd)
	return cmd, nil
}

// DefaultExec is the production door. The child is the mi-lsp CLI, which uses
// the shared daemon when it is up and direct execution when it is not.
func DefaultExec(ctx context.Context, bin string, argv []string) ExecResult {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd, err := NewCommand(ctx, bin, argv)
	if err != nil {
		return ExecResult{Code: -1, SpawnFailed: true, Stderr: err.Error()}
	}
	var stdout, stderr capBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = os.Environ()
	err = cmd.Run()
	res := ExecResult{
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		Truncated: stdout.over || stderr.over,
	}
	if ctx.Err() == context.DeadlineExceeded {
		res.TimedOut = true
		res.Code = -1
		return res
	}
	if ctx.Err() == context.Canceled {
		res.Code = -1
		if res.Stderr == "" {
			res.Stderr = "request cancelled"
		}
		return res
	}
	if err == nil {
		res.Code = 0
		return res
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.Code = exitErr.ExitCode()
		return res
	}
	res.SpawnFailed = true
	res.Code = -1
	if res.Stderr == "" {
		res.Stderr = err.Error()
	}
	return res
}

func refuseShell(bin string) error {
	base := commandBase(bin)
	switch base {
	case "cmd", "cmd.exe", "sh", "sh.exe", "bash", "bash.exe",
		"powershell", "powershell.exe", "pwsh", "pwsh.exe", "command.com":
		return fmt.Errorf("refusing to spawn shell %s", base)
	}
	switch strings.ToLower(extOf(base)) {
	case ".cmd", ".bat":
		return fmt.Errorf("refusing to spawn script host %s", base)
	}
	return nil
}

func commandBase(path string) string {
	trimmed := strings.TrimSpace(path)
	trimmed = strings.ReplaceAll(trimmed, "\\", "/")
	if index := strings.LastIndex(trimmed, "/"); index >= 0 {
		trimmed = trimmed[index+1:]
	}
	return strings.ToLower(trimmed)
}

func extOf(base string) string {
	if index := strings.LastIndex(base, "."); index >= 0 {
		return base[index:]
	}
	return ""
}

type capBuffer struct {
	buf  bytes.Buffer
	max  int
	over bool
}

func (c *capBuffer) Write(p []byte) (int, error) {
	if c.max <= 0 {
		c.max = maxRawBytes
	}
	if c.buf.Len() >= c.max {
		c.over = true
		return len(p), nil
	}
	remain := c.max - c.buf.Len()
	if len(p) > remain {
		c.over = true
		if _, err := c.buf.Write(p[:remain]); err != nil {
			return 0, err
		}
		return len(p), nil
	}
	return c.buf.Write(p)
}

func (c *capBuffer) String() string { return c.buf.String() }
