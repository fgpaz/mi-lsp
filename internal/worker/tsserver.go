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
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/processutil"
)

type TsserverClient struct {
	workspace  model.WorkspaceRegistration
	nodePath   string
	tsserverJS string
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	reader     *bufio.Reader
	stderr     *boundedTailWriter
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
	nodePath, err := resolveNodeExecutable()
	if err != nil {
		return nil, err
	}
	tsserverPath, err := findTsserverPath(workspace.Root, nodePath)
	if err != nil {
		return nil, err
	}
	return &TsserverClient{workspace: workspace, nodePath: nodePath, tsserverJS: tsserverPath}, nil
}

func CanUseTsserver(workspaceRoot string) bool {
	nodePath, err := resolveNodeExecutable()
	if err != nil {
		return false
	}
	_, err = findTsserverPath(workspaceRoot, nodePath)
	return err == nil
}

func (c *TsserverClient) Start() error {
	if c.cmd != nil {
		return nil
	}
	if c.nodePath == "" {
		return tsserverExecutionError("spawn", displayProcessArgv("node"), errors.New("resolved Node executable is unavailable"), "")
	}
	args := []string{c.tsserverJS, "--useSingleInferredProjectPerProjectRoot", "true"}
	argv := displayProcessArgv(c.nodePath, args...)
	cmd := processutil.Command(c.nodePath, args...)
	processutil.ConfigureNonInteractiveCommand(cmd)
	stderr := &boundedTailWriter{limit: 4096}
	cmd.Stderr = stderr
	bindWorkerLifetime(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return tsserverExecutionError("stdin_pipe", argv, err, "")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return tsserverExecutionError("stdout_pipe", argv, err, "")
	}
	if err := cmd.Start(); err != nil {
		return tsserverErrorWithStderr("spawn", argv, err, stderr.String())
	}
	c.cmd = cmd
	c.stdin = stdin
	c.stdout = stdout
	c.reader = bufio.NewReader(stdout)
	c.stderr = stderr
	return nil
}

func (c *TsserverClient) Call(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error) {
	for !c.mu.TryLock() {
		select {
		case <-ctx.Done():
			return model.WorkerResponse{}, ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
	defer c.mu.Unlock()

	if err := c.Start(); err != nil {
		return model.WorkerResponse{}, err
	}

	// Interrupt blocking protocol reads/writes on the same request deadline.
	cmd, in, out := c.cmd, c.stdin, c.stdout
	interrupted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = in.Close()
		_ = out.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		close(interrupted)
	})
	defer func() {
		if !stop() {
			<-interrupted
			_ = c.Close()
		}
	}()
	switch request.Method {
	case "diagnostics":
		return c.diagnostics(ctx, request)
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
	c.stderr = nil
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
	if err := writeTSServerRequest(c.stdin, message); err != nil {
		return c.sequence, c.failure("request_write", command, err)
	}
	return c.sequence, nil
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
			return nil, c.failure("response_read", command, err)
		}
		if message.Type != "response" || message.RequestSeq != seq {
			continue
		}
		if !message.Success {
			return nil, c.failure("request_rejected", command, errors.New(strings.TrimSpace(message.Message)))
		}
		return message.Body, nil
	}
}

// writeTSServerRequest envía solicitudes JSON delimitadas por línea; las
// respuestas de tsserver mantienen el framing Content-Length de readTSFrame.
func writeTSServerRequest(writer io.Writer, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	body = append(body, '\n')
	written, err := writer.Write(body)
	if err != nil {
		return err
	}
	if written != len(body) {
		return io.ErrShortWrite
	}
	return nil
}

func readTSFrame(reader *bufio.Reader) (tsserverEnvelope, error) {
	contentLength := 0
	headerSeen := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) && !headerSeen {
				return tsserverEnvelope{}, errors.New("tsserver returned invalid content length")
			}
			return tsserverEnvelope{}, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			if headerSeen {
				break
			}
			continue
		}
		headerSeen = true
		if separator := strings.IndexByte(line, ':'); separator >= 0 && strings.EqualFold(strings.TrimSpace(line[:separator]), "Content-Length") {
			value := strings.TrimSpace(line[separator+1:])
			contentLength, err = strconv.Atoi(value)
			if err != nil {
				return tsserverEnvelope{}, err
			}
		}
	}
	if contentLength <= 0 || contentLength > 8<<20 {
		return tsserverEnvelope{}, errors.New("tsserver returned invalid content length")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		return tsserverEnvelope{}, err
	}
	var envelope tsserverEnvelope
	if err := json.Unmarshal(bytes.TrimSpace(body), &envelope); err != nil {
		return tsserverEnvelope{}, err
	}
	return envelope, nil
}

func findTsserverPath(workspaceRoot, nodePath string) (string, error) {
	for current := workspaceRoot; current != ""; current = filepath.Dir(current) {
		candidate := filepath.Join(current, "node_modules", "typescript", "lib", "tsserver.js")
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return resolveTsserverScript(nodePath, candidate)
		}
		next := filepath.Dir(current)
		if next == current {
			break
		}
	}
	globalRoot, err := globalTsserverNpmRoot(nodePath)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(strings.TrimSpace(globalRoot), "typescript", "lib", "tsserver.js")
	if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() {
		return resolveTsserverScript(nodePath, candidate)
	}
	return "", errors.New("tsserver unavailable: typescript/lib/tsserver.js was not found in the workspace or global npm root")
}

func resolveNodeExecutable() (string, error) {
	candidate, err := exec.LookPath("node")
	if err != nil {
		return "", tsserverExecutionError("node_lookup", displayProcessArgv("node"), err, "")
	}
	absolute, err := filepath.Abs(candidate)
	if err != nil {
		return "", tsserverExecutionError("node_path", displayProcessArgv("node"), err, "")
	}
	if runtime.GOOS == "windows" {
		extension := strings.ToLower(filepath.Ext(absolute))
		if extension == ".cmd" || extension == ".bat" {
			return "", errors.New("tsserver node_lookup failed: PATH resolved node to a shell shim; an executable node.exe is required")
		}
	}
	if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
		return resolved, nil
	}
	return absolute, nil
}

func findNpmCLIPath(nodePath string) (string, error) {
	searchDirs := []string{filepath.Dir(nodePath)}
	if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
		searchDirs = append(searchDirs, filepath.Join(appData, "npm"))
	}
	for _, dir := range searchDirs {
		candidate := filepath.Join(dir, "node_modules", "npm", "bin", "npm-cli.js")
		info, err := os.Stat(candidate)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
			return resolved, nil
		}
		return candidate, nil
	}
	return "", errors.New("npm root discovery requires npm-cli.js beside node or under the AppData npm prefix; no .cmd shim was executed")
}

func globalNpmRoot() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	command := processutil.CommandContext(ctx, "npm", "root", "-g")
	processutil.ConfigureNonInteractiveCommand(command)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func globalTsserverNpmRoot(nodePath string) (string, error) {
	if runtime.GOOS != "windows" {
		return globalNpmRoot()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	npmCLI, err := findNpmCLIPath(nodePath)
	if err != nil {
		return "", err
	}
	args := []string{npmCLI, "root", "-g"}
	command := processutil.CommandContext(ctx, nodePath, args...)
	var stderr boundedTailWriter
	stderr.limit = 4096
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return "", tsserverErrorWithStderr("global_root", displayProcessArgv(nodePath, args...), err, stderr.String())
	}
	root := ""
	for _, line := range strings.Split(string(output), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			root = line
		}
	}
	if root == "" {
		return "", errors.New("tsserver global_root failed: npm returned an empty module root")
	}
	return root, nil
}

const nodeRealpathScript = `process.stdout.write(require("fs").realpathSync(process.argv[1]))`

func resolveTsserverScript(nodePath, candidate string) (string, error) {
	absolute, err := filepath.Abs(candidate)
	if err != nil {
		return "", tsserverExecutionError("script_path", displayProcessArgv(nodePath, candidate), err, "")
	}
	if runtime.GOOS != "windows" {
		resolved, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return "", tsserverExecutionError("script_realpath", displayProcessArgv(nodePath, candidate), err, "")
		}
		return resolved, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	args := []string{"-e", nodeRealpathScript, absolute}
	command := processutil.CommandContext(ctx, nodePath, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", tsserverExecutionError("script_realpath", displayProcessArgv(nodePath, "-e", "<fixed-realpath-script>", absolute), err, string(output))
	}
	resolved := strings.TrimSpace(string(output))
	if resolved == "" {
		return "", errors.New("tsserver script_realpath failed: Node returned an empty realpath")
	}
	return resolved, nil
}

type boundedTailWriter struct {
	mu     sync.Mutex
	limit  int
	buffer []byte
}

func (w *boundedTailWriter) Write(p []byte) (int, error) {
	written := len(p)
	if w.limit <= 0 {
		return written, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(p) >= w.limit {
		w.buffer = append(w.buffer[:0], p[len(p)-w.limit:]...)
		return written, nil
	}
	if overflow := len(w.buffer) + len(p) - w.limit; overflow > 0 {
		copy(w.buffer, w.buffer[overflow:])
		w.buffer = w.buffer[:len(w.buffer)-overflow]
	}
	w.buffer = append(w.buffer, p...)
	return written, nil
}

func (w *boundedTailWriter) String() string {
	if w == nil {
		return ""
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.buffer)
}

var (
	tsserverBearerTokenRE     = regexp.MustCompile(`(?i)\bbearer\s+(?:"[^"]*"|'[^']*'|[^\s,;]+)`)
	tsserverSensitiveValueRE  = regexp.MustCompile(`(?i)\b(bearer|authorization|access[_ -]?token|token|password|secret|api[_ -]?key)\b(\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;]+)`)
	tsserverWindowsUserPathRE = regexp.MustCompile(`(?i)([a-z]:[\\/](?:users|documents and settings)[\\/])[^\\/\s"'<>|]+`)
	tsserverUnixHomePathRE    = regexp.MustCompile(`((?:/home|/Users)/)[^/\s"'<>|]+`)
)

func SanitizeTsserverErrorDetail(detail string) string {
	detail = strings.ToValidUTF8(detail, "�")
	detail = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, detail)
	detail = strings.Join(strings.Fields(detail), " ")
	detail = tsserverBearerTokenRE.ReplaceAllString(detail, "Bearer <redacted>")
	detail = tsserverSensitiveValueRE.ReplaceAllString(detail, "$1$2<redacted>")
	detail = tsserverWindowsUserPathRE.ReplaceAllString(detail, "${1}<user>")
	detail = tsserverUnixHomePathRE.ReplaceAllString(detail, "${1}<user>")
	const maxDetail = 512
	if len(detail) > maxDetail {
		detail = detail[:maxDetail]
		for !utf8.ValidString(detail) {
			detail = detail[:len(detail)-1]
		}
		detail += "…"
	}
	return detail
}

func displayProcessArgv(program string, args ...string) string {
	values := make([]string, 0, len(args)+1)
	values = append(values, SanitizeTsserverErrorDetail(program))
	for _, arg := range args {
		if strings.Contains(arg, "realpathSync") {
			arg = "<fixed-realpath-script>"
		} else {
			arg = SanitizeTsserverErrorDetail(arg)
		}
		values = append(values, arg)
	}
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = strconv.Quote(value)
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

func tsserverExecutionError(stage, argv string, cause error, output string) error {
	parts := []string{"tsserver " + stage + " failed"}
	if cause != nil {
		if detail := SanitizeTsserverErrorDetail(cause.Error()); detail != "" {
			parts = append(parts, "cause="+detail)
		}
	}
	if detail := SanitizeTsserverErrorDetail(output); detail != "" {
		parts = append(parts, "process_output="+detail)
	}
	parts = append(parts, "process_args="+SanitizeTsserverErrorDetail(argv))
	return errors.New(strings.Join(parts, "; "))
}

func tsserverErrorWithStderr(stage, argv string, cause error, stderr string) error {
	base := tsserverExecutionError(stage, argv, cause, "")
	if detail := SanitizeTsserverErrorDetail(stderr); detail != "" {
		return errors.New(base.Error() + "; stderr=" + detail)
	}
	return base
}

func (c *TsserverClient) failure(stage, command string, cause error) error {
	if command != "" {
		stage += " command=" + command
	}
	argv := displayProcessArgv(c.nodePath, c.tsserverJS, "--useSingleInferredProjectPerProjectRoot", "true")
	stderr := ""
	if c.stderr != nil {
		stderr = c.stderr.String()
	}
	return tsserverErrorWithStderr(stage, argv, cause, stderr)
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
