package selfupdate

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/processutil"
)

func TestTransactionFailedRestoreDoesNotClaimRollback(t *testing.T) {
	target := transactionTarget(t)
	archive := transactionArchive(t, "zip", []archiveEntry{{name: cliName(), body: "new-image", mode: 0755}})
	tx := transactionFor(t, target, archive)
	tx.Readback = func(_ context.Context, p string) (string, error) {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		if string(b) == "old-image" {
			return "0.15.6", nil
		}
		if err = os.Remove(target + ".old"); err != nil {
			return "", fmt.Errorf("fixture interference: %w", err)
		}
		return "", errors.New("readback failed, backup unavailable")
	}
	result, err := ApplyTransaction(context.Background(), tx)
	if err == nil || result.Code != "" || result.Changed || result.Installed != "" || result.Previous != "" {
		t.Fatalf("failed restore must not claim rollback: result=%+v err=%v", result, err)
	}
}

func TestTransactionInvalidArchiveBeforePromotionDoesNotClaimRollback(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		archive    bool
	}{
		{"malformed", "not a zip", false},
		{"no-cli", "not the executable", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := transactionTarget(t)
			var archive string
			if tc.archive {
				archive = transactionArchive(t, "zip", []archiveEntry{{name: "README.md", body: tc.body, mode: 0644}})
			} else {
				archive = filepath.Join(t.TempDir(), "broken.zip")
				if err := os.WriteFile(archive, []byte(tc.body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			result, err := ApplyTransaction(context.Background(), transactionFor(t, target, archive))
			if err == nil || result.Code != "" || result.Changed || result.Installed != "0.15.6" || result.Previous != "" {
				t.Fatalf("pre-promotion error misreported: result=%+v err=%v", result, err)
			}
			assertImage(t, target, "old-image")
		})
	}
}

// beforeBackup runs after the initial target hash and before backup; it is a private test seam.
func TestTransactionTargetChangedBeforeBackupIsNotOverwritten(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses POSIX replacement rename")
	}
	target := transactionTarget(t)
	archive := transactionArchive(t, "zip", []archiveEntry{{name: cliName(), body: "new-image", mode: 0755}})
	tx := transactionFor(t, target, archive)
	tx.Readback = func(_ context.Context, p string) (string, error) {
		b, e := os.ReadFile(p)
		if e != nil {
			return "", e
		}
		switch string(b) {
		case "old-image":
			return "0.15.6", nil
		case "external-image":
			return "0.15.7", nil
		case "new-image":
			return "0.16.0", nil
		}
		return "", errors.New("unknown image")
	}
	ops := transactionOps{beforeBackup: func() error {
		external := filepath.Join(filepath.Dir(target), "external")
		if err := os.WriteFile(external, []byte("external-image"), 0755); err != nil {
			return err
		}
		return os.Rename(external, target)
	}}
	result, err := applyTransaction(context.Background(), tx, ops)
	if err == nil || result.Changed || result.Previous != "" || result.Installed == "0.15.6" {
		t.Fatalf("concurrent image misreported: result=%+v err=%v", result, err)
	}
	assertImage(t, target, "external-image")
}

func TestCleanupOwnedBackupPOSIXKeepsRunningProcessAlive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX unlink semantics")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "mi-lsp")
	if err = os.WriteFile(target, old, 0755); err != nil {
		t.Fatal(err)
	}
	cmd := processutil.Command(target)
	cmd.Env = append(os.Environ(), "A15_TXN_HELPER=hold-check")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case e := <-done:
			if e != nil {
				t.Errorf("helper exit: %v", e)
			}
		case <-time.After(10 * time.Second):
			t.Error("helper exit timeout")
		}
	}()
	reader := bufio.NewReader(stdout)
	receive := func(expected string) {
		t.Helper()
		ch := make(chan string, 1)
		go func() {
			line, e := reader.ReadString('\n')
			if e != nil {
				ch <- "error: " + e.Error()
				return
			}
			ch <- strings.TrimSpace(line)
		}()
		select {
		case line := <-ch:
			if line != expected {
				t.Fatalf("helper response %q, want %q", line, expected)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("helper response timeout")
		}
	}
	receive("ready")
	archive := transactionArchive(t, "zip", []archiveEntry{{name: officialWrapper() + "/mi-lsp", body: "new-image", mode: 0755}})
	tx := transactionFor(t, target, archive)
	tx.Readback = func(_ context.Context, p string) (string, error) {
		b, e := os.ReadFile(p)
		if e != nil {
			return "", e
		}
		if bytes.Equal(b, old) {
			return "0.15.6", nil
		}
		if string(b) == "new-image" {
			return "0.16.0", nil
		}
		return "", errors.New("unknown image")
	}
	result, err := ApplyTransaction(context.Background(), tx)
	if err != nil || !result.Changed {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if err = CleanupOld(target); err != nil {
		t.Fatalf("owned POSIX backup cleanup: %v", err)
	}
	if _, err = os.Lstat(target + ".old"); !os.IsNotExist(err) {
		t.Fatalf("backup still exists: %v", err)
	}
	if _, err = stdin.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	receive("alive")
	assertImage(t, target, "new-image")
}

func TestCleanupOldChildMarkerDefersOwnedBackup(t *testing.T) {
	target := transactionTarget(t)
	archive := transactionArchive(t, "zip", []archiveEntry{{name: cliName(), body: "new-image", mode: 0755}})
	result, err := ApplyTransaction(context.Background(), transactionFor(t, target, archive))
	if err != nil || !result.Changed {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	t.Setenv("MI_LSP_UPDATE_CHILD", "1")
	_ = CleanupOld(target)
	assertImage(t, target+".old", "old-image")
	assertImage(t, target, "new-image")
}
