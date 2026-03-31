package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/processutil"
)

type Client struct {
	workspace model.WorkspaceRegistration
	specs     []LaunchSpec
	specIndex int
	spec      LaunchSpec
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    *bytes.Buffer
	mu        sync.Mutex
}

func NewClient(repoRoot string, workspace model.WorkspaceRegistration) (*Client, error) {
	specs, err := ResolveLaunchSpecs(repoRoot)
	if err != nil {
		return nil, err
	}
	client := &Client{
		workspace: workspace,
		specs:     specs,
		specIndex: 0,
	}
	if len(specs) > 0 {
		client.spec = specs[0]
	}
	return client, nil
}

func (c *Client) Start() error {
	if c.cmd != nil {
		return nil
	}
	if len(c.specs) == 0 {
		return errors.New("no roslyn worker launch specs configured")
	}
	startIndex := c.specIndex
	if startIndex < 0 || startIndex >= len(c.specs) {
		startIndex = 0
	}

	errorsSeen := make([]string, 0, len(c.specs)-startIndex)
	for index := startIndex; index < len(c.specs); index++ {
		if err := c.startSpec(index); err != nil {
			errorsSeen = append(errorsSeen, err.Error())
			continue
		}
		return nil
	}
	if len(errorsSeen) == 0 {
		return errors.New("failed to start roslyn worker")
	}
	return errors.New(strings.Join(errorsSeen, "; "))
}

func (c *Client) startSpec(index int) error {
	spec := c.specs[index]
	cmd := exec.Command(spec.Command, spec.Args...)
	cmd.Dir = spec.WorkDir
	processutil.ConfigureNonInteractiveCommand(cmd)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return decorateLaunchError(spec, fmt.Errorf("stdin pipe: %w", err), stderr.String())
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return decorateLaunchError(spec, fmt.Errorf("stdout pipe: %w", err), stderr.String())
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return decorateLaunchError(spec, err, stderr.String())
	}

	c.specIndex = index
	c.spec = spec
	c.cmd = cmd
	c.stdin = stdin
	c.stdout = stdout
	c.stderr = &stderr
	return nil
}

func (c *Client) Call(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for attempt := 0; attempt < 2; attempt++ {
		response, err := c.callOnce(ctx, request)
		if err == nil {
			return response, nil
		}
		if attempt == 0 && c.shouldRetryCurrentError(err) && c.advanceToNextSpec() {
			continue
		}
		if c.shouldAnnotateCurrentError(err) {
			return response, decorateLaunchError(c.spec, err, c.stderrText())
		}
		return response, err
	}

	return model.WorkerResponse{}, errors.New("roslyn worker call exhausted retries")
}

func (c *Client) callOnce(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error) {
	if err := c.Start(); err != nil {
		return model.WorkerResponse{}, err
	}
	if err := WriteFrame(c.stdin, request); err != nil {
		return model.WorkerResponse{}, err
	}

	responseChannel := make(chan model.WorkerResponse, 1)
	errorChannel := make(chan error, 1)
	go func() {
		var response model.WorkerResponse
		if err := ReadFrame(c.stdout, &response); err != nil {
			errorChannel <- err
			return
		}
		responseChannel <- response
	}()

	select {
	case <-ctx.Done():
		return model.WorkerResponse{}, ctx.Err()
	case err := <-errorChannel:
		return model.WorkerResponse{}, err
	case response := <-responseChannel:
		if !response.Ok && response.Error != "" {
			return response, fmt.Errorf("%s", response.Error)
		}
		return response, nil
	}
}

func (c *Client) shouldRetryCurrentError(err error) bool {
	if err == nil || c.specIndex >= len(c.specs)-1 {
		return false
	}
	if c.shouldAnnotateCurrentError(err) {
		return true
	}
	return false
}

func (c *Client) shouldAnnotateCurrentError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(strings.Join([]string{err.Error(), c.stderrText()}, "\n")))
	if message == "" {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	switch {
	case strings.Contains(message, "dll was not found"):
		return true
	case strings.Contains(message, "protocol version mismatch"):
		return true
	case strings.Contains(message, "no compatible roslyn worker available"):
		return true
	case strings.Contains(message, "bundled/global worker"):
		return true
	case strings.Contains(message, "broken pipe"):
		return true
	case strings.Contains(message, "closed pipe"):
		return true
	case strings.Contains(message, "file already closed"):
		return true
	default:
		return false
	}
}

func (c *Client) advanceToNextSpec() bool {
	nextIndex := c.specIndex + 1
	if nextIndex >= len(c.specs) {
		return false
	}
	_ = c.closeProcess()
	c.specIndex = nextIndex
	c.spec = c.specs[nextIndex]
	return true
}

func (c *Client) stderrText() string {
	if c.stderr == nil {
		return ""
	}
	return strings.TrimSpace(c.stderr.String())
}

func decorateLaunchError(spec LaunchSpec, err error, stderr string) error {
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(err.Error())
	stderr = strings.TrimSpace(stderr)
	if stderr != "" && !strings.Contains(strings.ToLower(message), strings.ToLower(stderr)) {
		message = firstNonEmpty(stderr, message)
	}
	if strings.Contains(strings.ToLower(message), "worker binary") && strings.Contains(strings.ToLower(message), "mi-lsp worker install") {
		return errors.New(message)
	}
	source := firstNonEmpty(spec.Source, "candidate")
	path := firstNonEmpty(spec.CandidatePath, spec.Command)
	message = fmt.Sprintf("worker binary %s (%s) failed: %s", source, path, message)
	if !strings.Contains(strings.ToLower(message), "mi-lsp worker install") {
		message += ". Run `mi-lsp worker install` to refresh the bundled/global worker."
	}
	return errors.New(message)
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeProcess()
}

func (c *Client) closeProcess() error {
	if c.cmd == nil || c.cmd.Process == nil {
		c.cmd = nil
		c.stdin = nil
		c.stdout = nil
		c.stderr = nil
		return nil
	}
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	err := c.cmd.Process.Kill()
	_, _ = c.cmd.Process.Wait()
	c.cmd = nil
	c.stdin = nil
	c.stdout = nil
	c.stderr = nil
	return err
}

func (c *Client) PID() int {
	if c.cmd == nil || c.cmd.Process == nil {
		return 0
	}
	return c.cmd.Process.Pid
}

type EphemeralCaller struct {
	RepoRoot string
}

func (e EphemeralCaller) Call(ctx context.Context, workspace model.WorkspaceRegistration, request model.WorkerRequest) (model.WorkerResponse, error) {
	backendType := request.BackendType
	if backendType == "" {
		backendType = "roslyn"
		request.BackendType = backendType
	}
	client, err := NewRuntimeClient(e.RepoRoot, workspace, request)
	if err != nil {
		return model.WorkerResponse{}, err
	}
	defer client.Close()
	return client.Call(ctx, request)
}

func (e EphemeralCaller) Status() []model.WorkerStatus {
	return nil
}
