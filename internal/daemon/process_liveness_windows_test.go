//go:build windows

package daemon

import (
	"errors"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestProcessExistsWithOpenProcessErrorsFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "access denied is possibly alive", err: windows.ERROR_ACCESS_DENIED, want: true},
		{name: "invalid parameter means missing", err: windows.ERROR_INVALID_PARAMETER, want: false},
		{name: "unknown error is possibly alive", err: errors.New("ambiguous OpenProcess failure"), want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := processExistsWith(
				4242,
				func(uint32) (syscall.Handle, error) { return 0, test.err },
				func(syscall.Handle, *uint32) error {
					t.Fatal("GetExitCodeProcess should not be called")
					return nil
				},
				func(syscall.Handle) error { return nil },
			)
			if got != test.want {
				t.Fatalf("processExistsWith = %v, want %v", got, test.want)
			}
		})
	}
}

func TestProcessExistsWithValidHandleExitState(t *testing.T) {
	tests := []struct {
		name     string
		exitCode uint32
		want     bool
	}{
		{name: "active", exitCode: stillActive, want: true},
		{name: "terminated", exitCode: 0, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := processExistsWith(
				4242,
				func(uint32) (syscall.Handle, error) { return syscall.Handle(0x1234), nil },
				func(_ syscall.Handle, exitCode *uint32) error {
					*exitCode = test.exitCode
					return nil
				},
				func(syscall.Handle) error { return nil },
			)
			if got != test.want {
				t.Fatalf("processExistsWith = %v, want %v", got, test.want)
			}
		})
	}
}

func TestProcessExistsWithExitCodeFailureFailsClosed(t *testing.T) {
	got := processExistsWith(
		4242,
		func(uint32) (syscall.Handle, error) { return syscall.Handle(0x1234), nil },
		func(syscall.Handle, *uint32) error { return errors.New("GetExitCodeProcess failure") },
		func(syscall.Handle) error { return nil },
	)
	if !got {
		t.Fatal("processExistsWith = false, want true when exit code is unavailable")
	}
}
