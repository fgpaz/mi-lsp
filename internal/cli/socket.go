package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const (
	cliSocketName         = "mi-lsp.sock"
	cliSocketMode         = 0o600
	cliSocketMaxRequest   = 1 << 20
	cliSocketRequestLimit = 35 * time.Second
)

type cliSocketRequest struct {
	Operation string          `json:"op"`
	Input     json.RawMessage `json:"input"`
}

type cliSocketServer struct {
	listener *net.UnixListener
	path     string
	created  os.FileInfo
}

func newCLISocketServer() (*cliSocketServer, error) {
	dir, err := safeRuntimeDirectory(os.Getenv("XDG_RUNTIME_DIR"))
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, cliSocketName)
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("socket path must be absolute")
	}
	if _, err := os.Lstat(path); err == nil {
		return nil, fmt.Errorf("socket path already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("socket path cannot be inspected")
	}

	listener, err := listenOwnerSocket(path)
	if err != nil {
		return nil, fmt.Errorf("cannot listen on the requested Unix socket")
	}
	unixListener, ok := listener.(*net.UnixListener)
	if !ok {
		_ = listener.Close()
		return nil, fmt.Errorf("Unix socket listener unavailable")
	}
	unixListener.SetUnlinkOnClose(false)

	server := &cliSocketServer{listener: unixListener, path: path}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		server.close()
		return nil, fmt.Errorf("socket path is not a Unix socket")
	}
	server.created = info
	if err := os.Chmod(path, cliSocketMode); err != nil {
		server.close()
		return nil, fmt.Errorf("cannot secure Unix socket permissions")
	}
	info, err = os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != cliSocketMode || !os.SameFile(server.created, info) {
		server.close()
		return nil, fmt.Errorf("Unix socket permissions are not owner-only")
	}
	server.created = info
	return server, nil
}

func safeRuntimeDirectory(path string) (string, error) {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("XDG_RUNTIME_DIR must be an absolute directory")
	}
	path = filepath.Clean(path)
	if path == string(filepath.Separator) {
		return "", fmt.Errorf("XDG_RUNTIME_DIR is unsafe")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("XDG_RUNTIME_DIR is missing or unsafe")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return "", fmt.Errorf("XDG_RUNTIME_DIR must not resolve through a symlink")
	}
	return path, nil
}

func (s *cliSocketServer) close() {
	if s == nil {
		return
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
	if s.created == nil || s.path == "" {
		return
	}
	current, err := os.Lstat(s.path)
	if err == nil && os.SameFile(s.created, current) && current.Mode()&os.ModeSocket != 0 {
		_ = os.Remove(s.path)
	}
}

func serveCLISocket(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	serverCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	server, err := newCLISocketServer()
	if err != nil {
		return err
	}
	defer server.close()

	var mu sync.Mutex
	active := make(map[net.Conn]struct{})
	var handlers sync.WaitGroup
	stopAccepting := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = server.listener.Close()
			mu.Lock()
			for conn := range active {
				_ = conn.Close()
			}
			mu.Unlock()
		case <-stopAccepting:
		}
	}()

	for {
		conn, err := server.listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				break
			}
			cancel()
			close(stopAccepting)
			mu.Lock()
			for activeConn := range active {
				_ = activeConn.Close()
			}
			mu.Unlock()
			handlers.Wait()
			return fmt.Errorf("Unix socket accept failed")
		}

		mu.Lock()
		active[conn] = struct{}{}
		mu.Unlock()
		handlers.Add(1)
		go func(conn net.Conn) {
			defer handlers.Done()
			defer conn.Close()
			defer func() {
				mu.Lock()
				delete(active, conn)
				mu.Unlock()
			}()
			handleCLISocketConnection(serverCtx, conn)
		}(conn)
	}

	cancel()
	close(stopAccepting)
	mu.Lock()
	for conn := range active {
		_ = conn.Close()
	}
	mu.Unlock()
	handlers.Wait()
	return nil
}

func handleCLISocketConnection(ctx context.Context, conn net.Conn) {
	_ = conn.SetDeadline(time.Now().Add(cliSocketRequestLimit))
	request, err := readCLISocketRequest(bufio.NewReaderSize(conn, 4096))
	if err != nil {
		writeCLISocketFailure(conn, reasonExplicitIncomplete, "invalid socket request")
		return
	}
	args, err := cliSocketArguments(request)
	if err != nil {
		writeCLISocketFailure(conn, reasonUnsupportedOperation, "operation or input is not supported")
		return
	}
	response, err := runCLISocketOperation(ctx, args)
	if err != nil {
		writeCLISocketFailure(conn, reasonExplicitIncomplete, "operation did not produce a valid response")
		return
	}
	_, _ = io.Copy(conn, bytes.NewReader(response))
}

func readCLISocketRequest(reader *bufio.Reader) (cliSocketRequest, error) {
	var line []byte
	for {
		part, err := reader.ReadSlice('\n')
		if len(line)+len(part) > cliSocketMaxRequest {
			return cliSocketRequest{}, fmt.Errorf("request too large")
		}
		line = append(line, part...)
		if err == nil {
			break
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return cliSocketRequest{}, err
		}
	}
	if len(line) == 0 || line[len(line)-1] != '\n' {
		return cliSocketRequest{}, fmt.Errorf("request must be newline-delimited")
	}
	line = bytes.TrimSuffix(line, []byte{'\n'})
	line = bytes.TrimSuffix(line, []byte{'\r'})
	if !utf8.Valid(line) {
		return cliSocketRequest{}, fmt.Errorf("request must be UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	var request cliSocketRequest
	if err := decoder.Decode(&request); err != nil {
		return cliSocketRequest{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return cliSocketRequest{}, fmt.Errorf("request must contain one JSON document")
	}
	if strings.TrimSpace(request.Operation) == "" {
		return cliSocketRequest{}, fmt.Errorf("operation is required")
	}
	if len(request.Input) == 0 {
		request.Input = json.RawMessage(`{}`)
	}
	var input map[string]json.RawMessage
	if err := json.Unmarshal(request.Input, &input); err != nil || input == nil {
		return cliSocketRequest{}, fmt.Errorf("input must be an object")
	}
	return request, nil
}

type cliQSocketInput struct {
	Pipeline  string  `json:"pipeline"`
	Workspace *string `json:"workspace"`
	Budget    *int    `json:"budget"`
	MaxBytes  *int    `json:"max_bytes"`
	TimeoutMS *int    `json:"timeout_ms"`
	SessionID *string `json:"session_id"`
	Page      *string `json:"page"`
	Fresh     *bool   `json:"fresh"`
	Dedupe    *bool   `json:"dedupe"`
}

func cliSocketArguments(request cliSocketRequest) ([]string, error) {
	base := []string{"--format=json", "--client-name=mi-mcp", "--no-auto-register"}
	switch request.Operation {
	case "q":
		var input cliQSocketInput
		if err := decodeSocketInput(request.Input, &input); err != nil || input.Pipeline == "" {
			return nil, fmt.Errorf("invalid q input")
		}
		args := append([]string{}, base...)
		if input.Workspace != nil {
			args = append(args, "--workspace="+*input.Workspace)
		}
		args = append(args, "q")
		if input.Budget != nil {
			args = append(args, "--budget="+fmt.Sprint(*input.Budget))
		}
		if input.MaxBytes != nil {
			args = append(args, "--max-bytes="+fmt.Sprint(*input.MaxBytes))
		}
		if input.TimeoutMS != nil {
			args = append(args, "--timeout-ms="+fmt.Sprint(*input.TimeoutMS))
		}
		if input.SessionID != nil {
			args = append(args, "--session-id="+*input.SessionID)
		}
		if input.Page != nil {
			args = append(args, "--page="+*input.Page)
		}
		if input.Fresh != nil {
			args = append(args, fmt.Sprintf("--fresh=%t", *input.Fresh))
		}
		if input.Dedupe != nil {
			args = append(args, fmt.Sprintf("--dedupe=%t", *input.Dedupe))
		}
		args = append(args, "--", input.Pipeline)
		return args, nil
	case "version", "schema":
		if err := requireEmptySocketInput(request.Input); err != nil {
			return nil, err
		}
		return append(base, request.Operation), nil
	case "--help", "--version":
		if err := requireEmptySocketInput(request.Input); err != nil {
			return nil, err
		}
		return append(base, request.Operation), nil
	default:
		return nil, fmt.Errorf("unsupported operation")
	}
}

func decodeSocketInput(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("input must contain one JSON object")
	}
	return nil
}

func requireEmptySocketInput(raw json.RawMessage) error {
	var input map[string]json.RawMessage
	if err := json.Unmarshal(raw, &input); err != nil || input == nil || len(input) != 0 {
		return fmt.Errorf("operation does not accept input")
	}
	return nil
}

func runCLISocketOperation(ctx context.Context, args []string) ([]byte, error) {
	binary, err := os.Executable()
	if err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, binary, args...)
	var stdout bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = io.Discard
	runErr := command.Run()
	output := bytes.TrimSpace(stdout.Bytes())
	if len(output) == 0 {
		return nil, fmt.Errorf("operation produced no output")
	}

	var envelope struct {
		OK *bool `json:"ok"`
	}
	isJSON := json.Valid(output)
	if isJSON {
		var compact bytes.Buffer
		if err := json.Compact(&compact, output); err != nil {
			return nil, err
		}
		output = compact.Bytes()
		_ = json.Unmarshal(output, &envelope)
		if envelope.OK != nil && !*envelope.OK {
			return append(output, '\n'), nil
		}
	}
	if runErr != nil {
		return nil, runErr
	}

	var response bytes.Buffer
	response.WriteString(`{"ok":true,"data":`)
	if isJSON {
		response.Write(output)
	} else {
		encoded, err := json.Marshal(string(output))
		if err != nil {
			return nil, err
		}
		response.Write(encoded)
	}
	response.WriteByte('}')
	response.WriteByte('\n')
	return response.Bytes(), nil
}

func writeCLISocketFailure(writer io.Writer, reasonCode, detail string) {
	payload := model.Envelope{
		Ok: false,
		Error: &model.EnvelopeError{
			ReasonCode: reasonCode,
			Detail:     detail,
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	encoded = append(encoded, '\n')
	_, _ = io.Copy(writer, bytes.NewReader(encoded))
}
