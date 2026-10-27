package selfupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/processutil"
)

// The helper is the test binary itself, never an installed CLI or a built fixture.
func TestMain(m *testing.M) {
	switch os.Getenv("A15_TXN_HELPER") {
	case "version":
		if strings.Join(os.Args[1:], " ") != "version --format json" || os.Getenv("MI_LSP_UPDATE_CHILD") != "1" {
			os.Exit(71)
		}
		fmt.Print(`{"ok":true,"backend":"version","items":[{"version":"0.15.6"}]}`)
		os.Exit(0)
	case "hold":
		fmt.Fprintln(os.Stdout, "ready")
		var b [1]byte
		_, _ = os.Stdin.Read(b[:])
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestReadInstalledChildMarkerAndVersionEnvelope(t *testing.T) {
	t.Setenv("A15_TXN_HELPER", "version")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	version, err := ReadInstalled(context.Background(), exe)
	if err != nil || version != "0.15.6" {
		t.Fatalf("version=%q err=%v", version, err)
	}
}

func TestTransactionZipDeclaredBomb(t *testing.T) {
	// Central-directory uncompressed size is deliberately >4 GiB; no large allocation.
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, err := w.Create(cliName())
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Write([]byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	raw := b.Bytes()
	i := bytes.Index(raw, []byte{'P', 'K', 1, 2})
	if i < 0 {
		t.Fatal("missing central directory")
	}
	// A conservative >4 GiB declaration is enough to exceed any reasonable CLI extraction cap.
	binary.LittleEndian.PutUint32(raw[i+24:i+28], 0xfffffffe)
	archive := filepath.Join(t.TempDir(), "bomb.zip")
	if err = os.WriteFile(archive, raw, 0600); err != nil {
		t.Fatal(err)
	}
	target := transactionTarget(t)
	result, err := ApplyTransaction(context.Background(), transactionFor(t, target, archive))
	if err == nil || result.Changed {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	assertImage(t, target, "old-image")
}

func TestTransactionInjectedPromotionErrors(t *testing.T) {
	// transactionOps is an approved private seam; nil hooks use the real OS operation.
	for _, tc := range []struct {
		name  string
		fault error
		code  string
	}{
		{"permission", os.ErrPermission, "UPDATE_PERMISSION_DENIED"},
		{"in-use", syscall.Errno(32), "UPDATE_IN_USE_DEFERRED"},
		{"virus", syscall.Errno(225), "UPDATE_QUARANTINED"},
		{"virus-deleted", syscall.Errno(226), "UPDATE_QUARANTINED"},
		{"policy-block", syscall.Errno(1260), "UPDATE_QUARANTINED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := transactionTarget(t)
			archive := transactionArchive(t, "zip", []archiveEntry{{name: cliName(), body: "new-image", mode: 0755}})
			tx := transactionFor(t, target, archive)
			ops := transactionOps{rename: func(from, to string) error {
				if to == target {
					return &os.LinkError{Op: "rename", Old: from, New: to, Err: tc.fault}
				}
				return os.Rename(from, to)
			}}
			result, err := applyTransaction(context.Background(), tx, ops)
			if err == nil || result.Code != tc.code || result.Changed {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if strings.Contains(err.Error(), target) {
				t.Fatalf("path leaked: %v", err)
			}
			assertImage(t, target, "old-image")
		})
	}
}

func TestTransactionGenericReadbackErrorIsNotQuarantine(t *testing.T) {
	target := transactionTarget(t)
	archive := transactionArchive(t, "zip", []archiveEntry{{name: cliName(), body: "new-image", mode: 0755}})
	tx := transactionFor(t, target, archive)
	tx.Readback = func(_ context.Context, p string) (string, error) {
		b, e := os.ReadFile(p)
		if e != nil {
			return "", e
		}
		if string(b) == "old-image" {
			return "0.15.6", nil
		}
		return "", errors.New("ordinary parse failure")
	}
	result, err := ApplyTransaction(context.Background(), tx)
	if err == nil || result.Code != "UPDATE_READBACK_FAILED_ROLLED_BACK" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	assertImage(t, target, "old-image")
}

func TestTransactionCleanupOwnedBackup(t *testing.T) {
	target := transactionTarget(t)
	archive := transactionArchive(t, "zip", []archiveEntry{{name: cliName(), body: "new-image", mode: 0755}})
	result, err := ApplyTransaction(context.Background(), transactionFor(t, target, archive))
	if err != nil || !result.Changed {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	assertImage(t, target+".old", "old-image")
	if err = CleanupOld(target); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(target + ".old"); !os.IsNotExist(err) {
		t.Fatalf("own backup remains: %v", err)
	}
	assertImage(t, target, "new-image")
}

func TestTransactionUnixAtomicPromotionAndRollback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix rename semantics")
	}
	for _, rollback := range []bool{false, true} {
		t.Run(fmt.Sprint("rollback=", rollback), func(t *testing.T) {
			target := transactionTarget(t)
			archive := transactionArchive(t, "zip", []archiveEntry{{name: cliName(), body: "new-image", mode: 0755}})
			tx := transactionFor(t, target, archive)
			if rollback {
				tx.Readback = func(_ context.Context, p string) (string, error) {
					b, e := os.ReadFile(p)
					if e != nil {
						return "", e
					}
					if string(b) == "old-image" {
						return "0.15.6", nil
					}
					return "", errors.New("readback failed")
				}
			}
			stop := make(chan struct{})
			observed := make(chan error, 1)
			started := make(chan struct{})
			go func() {
				close(started)
				for {
					select {
					case <-stop:
						observed <- nil
						return
					default:
						b, e := os.ReadFile(target)
						if e != nil {
							observed <- e
							return
						}
						if string(b) != "old-image" && string(b) != "new-image" {
							observed <- fmt.Errorf("partial image %q", b)
							return
						}
					}
				}
			}()
			<-started
			_, err := ApplyTransaction(context.Background(), tx)
			close(stop)
			if e := <-observed; e != nil {
				t.Fatal(e)
			}
			if rollback && err == nil || !rollback && err != nil {
				t.Fatalf("apply err=%v", err)
			}
			if rollback {
				assertImage(t, target, "old-image")
			} else {
				assertImage(t, target, "new-image")
			}
		})
	}
}

func TestTransactionWindowsRunningImageRenameAndRollback(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows executable sharing semantics")
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
	defer func() { _ = stdin.Close(); _ = cmd.Wait() }()
	var ready [6]byte
	n, err := stdout.Read(ready[:])
	if err != nil || !strings.HasPrefix(string(ready[:n]), "ready") {
		t.Fatalf("helper ready=%q err=%v", ready[:n], err)
	}
	archive := transactionArchive(t, "zip", []archiveEntry{{name: "mi-lsp.exe", body: "new-image", mode: 0755}})
	tx := transactionFor(t, target, archive)
	tx.Readback = func(_ context.Context, p string) (string, error) {
		b, e := os.ReadFile(p)
		if e != nil {
			return "", e
		}
		if bytes.Equal(b, old) {
			return "0.15.6", nil
		}
		return "", errors.New("bad new image")
	}
	result, err := ApplyTransaction(context.Background(), tx)
	if err == nil || result.Code != "UPDATE_READBACK_FAILED_ROLLED_BACK" || result.Installed != "0.15.6" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	got, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(got, old) {
		t.Fatalf("old image not restored: %v", err)
	}
}
