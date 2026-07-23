package cli

import (
	"errors"
	"os"
	"testing"
)

func TestDaemonStopErrorMapsUnavailableDaemonFailures(t *testing.T) {
	missingPipe := &os.PathError{
		Op:   "open",
		Path: `\\.\pipe\mi-lsp-test`,
		Err:  os.ErrNotExist,
	}

	tests := []struct {
		name string
		err  error
	}{
		{name: "transport", err: errors.New(`dial unix /tmp/mi-lsp.sock: connect: connection refused`)},
		{name: "missing windows named pipe", err: missingPipe},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := daemonStopError(tt.err)
			if got == nil || got.Error() != "daemon is not running" {
				t.Fatalf("daemonStopError(%v) = %v, want daemon is not running", tt.err, got)
			}
		})
	}
}

func TestDaemonStopErrorPreservesNonTransportFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "permission denied", err: errors.New("permission denied")},
		{name: "missing non-pipe path", err: &os.PathError{Op: "open", Path: `C:\\missing`, Err: os.ErrNotExist}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := daemonStopError(tt.err); got != tt.err {
				t.Fatalf("daemonStopError(%v) = %v, want original error", tt.err, got)
			}
		})
	}
}

func TestDaemonStopErrorPreservesNil(t *testing.T) {
	if got := daemonStopError(nil); got != nil {
		t.Fatalf("daemonStopError(nil) = %v, want nil", got)
	}
}
