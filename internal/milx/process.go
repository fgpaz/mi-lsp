package milx

import (
	"context"
	"io"
	"os/exec"
	"sync"
)

const maxDiagnosticStderr = 4096

type boundedStderr struct{ b []byte }

func (w *boundedStderr) Write(p []byte) (int, error) {
	if len(p) >= maxDiagnosticStderr {
		w.b = append(w.b[:0], p[len(p)-maxDiagnosticStderr:]...)
		return len(p), nil
	}
	if overflow := len(w.b) + len(p) - maxDiagnosticStderr; overflow > 0 {
		copy(w.b, w.b[overflow:])
		w.b = w.b[:len(w.b)-overflow]
	}
	w.b = append(w.b, p...)
	return len(p), nil
}

type managedProcess struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	stderr   boundedStderr
	waitOnce sync.Once
	waitErr  error
}

func startManagedProcess(ctx context.Context, executable string, args []string, dir string, env []string) (*managedProcess, error) {
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = dir
	cmd.Env = env
	configureProcess(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	p := &managedProcess{cmd: cmd, stdin: stdin, stdout: stdout}
	cmd.Stderr = &p.stderr // Bounded internal-only diagnostics; never returned to callers.
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, err
	}
	return p, nil
}
func (p *managedProcess) killTree() error { return killManagedProcess(p.cmd) }
func (p *managedProcess) close() error    { _ = p.stdin.Close(); _ = p.stdout.Close(); return nil }
func (p *managedProcess) wait() error {
	p.waitOnce.Do(func() { p.waitErr = p.cmd.Wait() })
	return p.waitErr
}
