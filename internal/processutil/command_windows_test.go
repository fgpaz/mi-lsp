//go:build windows

package processutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestWindowsConfigurationPreservesFlags(t *testing.T) {
	cmd := exec.Command("git", "--version")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200}
	ConfigureNonInteractiveCommand(cmd)
	if !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags&createNoWindow == 0 || cmd.SysProcAttr.CreationFlags&0x00000200 == 0 {
		t.Fatalf("noninteractive attributes: %+v", cmd.SysProcAttr)
	}
	ConfigureDetachedCommand(cmd)
	if !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags&(createNoWindow|detachedProcess|0x00000200) != createNoWindow|detachedProcess|0x00000200 {
		t.Fatalf("detached attributes: %+v", cmd.SysProcAttr)
	}
}

// The test binary plays both detached parent and console child. The parent also
// starts real git. No unconfigured console process is deliberately launched.
func TestWindowsDetachedConsoleProbe(t *testing.T) {
	switch os.Getenv("MILSP_A14_PROBE") {
	case "child":
		get := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleWindow")
		hwnd, _, _ := get.Call()
		if hwnd != 0 {
			t.Fatalf("child has console window: %#x", hwnd)
		}
		return
	case "parent":
		get := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleWindow")
		if hwnd, _, _ := get.Call(); hwnd != 0 {
			t.Fatalf("detached parent has console window: %#x", hwnd)
		}
		child := exec.Command(os.Args[0], "-test.run=^TestWindowsDetachedConsoleProbe$")
		child.Env = append(os.Environ(), "MILSP_A14_PROBE=child")
		ConfigureNonInteractiveCommand(child)
		if out, err := child.CombinedOutput(); err != nil {
			t.Fatalf("child: %v: %s", err, out)
		}
		git := exec.Command("git", "--version")
		ConfigureNonInteractiveCommand(git)
		if out, err := git.CombinedOutput(); err != nil || !strings.Contains(string(out), "git version") {
			t.Fatalf("git: %v: %s", err, out)
		}
		return
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("probe requires git: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "probe.log")
	out, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	parent := exec.Command(os.Args[0], "-test.run=^TestWindowsDetachedConsoleProbe$")
	parent.Env = append(os.Environ(), "MILSP_A14_PROBE=parent")
	parent.Stdout, parent.Stderr = out, out
	ConfigureDetachedCommand(parent)
	if err := parent.Run(); err != nil {
		data, _ := os.ReadFile(outPath)
		t.Fatalf("detached parent: %v: %s", err, data)
	}
}
