//go:build windows

package processutil

import (
	"os/exec"
	"testing"
)

func TestConfigureNonInteractiveCommandHidesConsoleWindow(t *testing.T) {
	cmd := exec.Command("cmd")

	ConfigureNonInteractiveCommand(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("expected SysProcAttr to be configured")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("expected HideWindow=true")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("expected CREATE_NO_WINDOW flag, got %#x", cmd.SysProcAttr.CreationFlags)
	}
}

func TestConfigureDetachedCommandKeepsHiddenAndDetachedFlags(t *testing.T) {
	cmd := exec.Command("cmd")

	ConfigureDetachedCommand(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("expected SysProcAttr to be configured")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("expected HideWindow=true")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("expected CREATE_NO_WINDOW flag, got %#x", cmd.SysProcAttr.CreationFlags)
	}
	if cmd.SysProcAttr.CreationFlags&detachedProcess == 0 {
		t.Fatalf("expected DETACHED_PROCESS flag, got %#x", cmd.SysProcAttr.CreationFlags)
	}
}
