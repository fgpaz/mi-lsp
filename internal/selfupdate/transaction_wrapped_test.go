package selfupdate

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/processutil"
)

func officialWrapper() string { return "mi-lsp_0.16.0_" + runtime.GOOS + "-" + runtime.GOARCH }

func TestTransactionOfficialWrappedArchive(t *testing.T) {
	for _, format := range []string{"zip", "tar.gz"} {
		t.Run(format, func(t *testing.T) {
			target := transactionTarget(t)
			archive := transactionArchive(t, format, []archiveEntry{{name: officialWrapper() + "/" + cliName(), body: "new-image", mode: 0755}, {name: officialWrapper() + "/README.md", body: "readme", mode: 0644}, {name: officialWrapper() + "/worker/worker", body: "worker", mode: 0755}})
			result, err := ApplyTransaction(context.Background(), transactionFor(t, target, archive))
			if err != nil || !result.Changed || result.Installed != "0.16.0" || result.Previous != "0.15.6" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			assertImage(t, target, "new-image")
		})
	}
}

func TestTransactionRejectsWrongOrAmbiguousWrapper(t *testing.T) {
	for _, format := range []string{"zip", "tar.gz"} {
		for _, tc := range []struct {
			name    string
			entries []archiveEntry
		}{
			{"unknown-wrapper", []archiveEntry{{name: "unknown/" + cliName(), body: "new-image", mode: 0755}}},
			{"wrong-version-wrapper", []archiveEntry{{name: "mi-lsp_9.9.9_" + runtime.GOOS + "-" + runtime.GOARCH + "/" + cliName(), body: "new-image", mode: 0755}}},
			{"root-and-wrapped", []archiveEntry{{name: cliName(), body: "root", mode: 0755}, {name: officialWrapper() + "/" + cliName(), body: "wrapped", mode: 0755}}},
		} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				target := transactionTarget(t)
				archive := transactionArchive(t, format, tc.entries)
				result, err := ApplyTransaction(context.Background(), transactionFor(t, target, archive))
				if err == nil || result.Changed {
					t.Fatalf("result=%+v err=%v", result, err)
				}
				assertImage(t, target, "old-image")
			})
		}
	}
}

func TestCleanupOldChangedOwnedBackupIsDeferred(t *testing.T) {
	target := transactionTarget(t)
	archive := transactionArchive(t, "zip", []archiveEntry{{name: officialWrapper() + "/" + cliName(), body: "new-image", mode: 0755}})
	result, err := ApplyTransaction(context.Background(), transactionFor(t, target, archive))
	if err != nil || !result.Changed {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if err = os.WriteFile(target+".old", []byte("changed-by-others"), 0755); err != nil {
		t.Fatal(err)
	}
	_ = CleanupOld(target)
	assertImage(t, target+".old", "changed-by-others")
	assertImage(t, target, "new-image")
}

func TestCleanupOldRunningOwnedBackupIsDeferredWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows running image")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "mi-lsp.exe")
	if err = os.WriteFile(target, old, 0755); err != nil {
		t.Fatal(err)
	}
	cmd := processutil.Command(target)
	cmd.Env = append(os.Environ(), "A15_TXN_HELPER=hold")
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
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("helper exit timeout")
		}
	}()
	ready := make(chan error, 1)
	go func() {
		line, e := bufio.NewReader(stdout).ReadString('\n')
		if e == nil && line != "ready\n" && line != "ready\r\n" {
			e = fmt.Errorf("unexpected helper response %q", line)
		}
		ready <- e
	}()
	select {
	case err = <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("helper readiness timeout")
	}
	archive := transactionArchive(t, "zip", []archiveEntry{{name: officialWrapper() + "/mi-lsp.exe", body: "new-image", mode: 0755}})
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
		return "", fmt.Errorf("unexpected fixture image")
	}
	result, err := ApplyTransaction(context.Background(), tx)
	if err != nil || !result.Changed {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	_ = CleanupOld(target)
	got, err := os.ReadFile(target + ".old")
	if err != nil || !bytes.Equal(got, old) {
		t.Fatalf("live owned backup not preserved: %v", err)
	}
	assertImage(t, target, "new-image")
}
