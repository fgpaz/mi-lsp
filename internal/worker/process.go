package worker

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/fgpaz/mi-lsp/internal/model"
)

type Client struct {
	workspace model.WorkspaceRegistration
	spec      LaunchSpec
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	mu        sync.Mutex
}

func NewClient(repoRoot string, workspace model.WorkspaceRegistration) (*Client, error) {
	spec, err := ResolveLaunchSpec(repoRoot)
	if err != nil {
		return nil, err
	}
	return &Client{workspace: workspace, spec: spec}, nil
}

func (c *Client) Start() error {
	if c.cmd != nil {
		return nil
	}
	cmd := exec.Command(c.spec.Command, c.spec.Args...)
	cmd.Dir = c.spec.WorkDir
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
	return nil
}

func (c *Client) Call(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

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

func (c *Client) Close() error {
	if c.cmd == nil || c.cmd.Process == nil {
		return nil
	}
	_ = c.stdin.Close()
	err := c.cmd.Process.Kill()
	_, _ = c.cmd.Process.Wait()
	c.cmd = nil
	c.stdin = nil
	c.stdout = nil
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
