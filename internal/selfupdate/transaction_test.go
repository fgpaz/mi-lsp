package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type archiveEntry struct {
	name, body string
	mode       os.FileMode
	link       string
}

func transactionArchive(t *testing.T, format string, entries []archiveEntry) string {
	t.Helper()
	var b bytes.Buffer
	if format == "zip" {
		w := zip.NewWriter(&b)
		for _, e := range entries {
			h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
			h.SetMode(e.mode)
			f, err := w.CreateHeader(h)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		gz := gzip.NewWriter(&b)
		w := tar.NewWriter(gz)
		for _, e := range entries {
			h := &tar.Header{Name: e.name, Mode: 0755, Size: int64(len(e.body)), Typeflag: tar.TypeReg}
			if e.mode&os.ModeSymlink != 0 {
				h.Typeflag = tar.TypeSymlink
				h.Linkname = e.link
				h.Size = 0
			} else if e.link != "" {
				h.Typeflag = tar.TypeLink
				h.Linkname = e.link
				h.Size = 0
			}
			if err := w.WriteHeader(h); err != nil {
				t.Fatal(err)
			}
			if h.Size != 0 {
				if _, err := w.Write([]byte(e.body)); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "release."+format)
	if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func transactionTarget(t *testing.T) string {
	t.Helper()
	name := "mi-lsp"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("old-image"), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}
func transactionFor(t *testing.T, target, archive string) Transaction {
	t.Helper()
	b, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	return Transaction{ExecutablePath: target, ArchivePath: archive, ExpectedArchiveSHA256: fmt.Sprintf("%x", sha256.Sum256(b)), TargetVersion: "0.16.0", Readback: func(_ context.Context, p string) (string, error) {
		b, e := os.ReadFile(p)
		if e != nil {
			return "", e
		}
		if string(b) == "old-image" {
			return "0.15.6", nil
		}
		return "0.16.0", nil
	}}
}
func assertImage(t *testing.T, path, want string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil || string(b) != want {
		t.Fatalf("image = %q, err = %v; want %q", b, err, want)
	}
}
func cliName() string {
	if runtime.GOOS == "windows" {
		return "mi-lsp.exe"
	}
	return "mi-lsp"
}

func TestTransactionChecksumDoesNotTouchTarget(t *testing.T) {
	target := transactionTarget(t)
	archive := transactionArchive(t, "zip", []archiveEntry{{name: cliName(), body: "new-image", mode: 0755}})
	tx := transactionFor(t, target, archive)
	tx.ExpectedArchiveSHA256 = strings.Repeat("0", 64)
	result, err := ApplyTransaction(context.Background(), tx)
	if err == nil || result.Code != "UPDATE_CHECKSUM_MISMATCH" || result.Changed {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	assertImage(t, target, "old-image")
}

func TestTransactionArchiveRejectsUnsafeEntries(t *testing.T) {
	for _, format := range []string{"zip", "tar.gz"} {
		for _, tc := range []struct {
			name    string
			entries []archiveEntry
		}{
			{"traversal", []archiveEntry{{name: "../escape", body: "bad", mode: 0755}, {name: cliName(), body: "new-image", mode: 0755}}},
			{"symlink", []archiveEntry{{name: cliName(), body: "elsewhere", mode: os.ModeSymlink | 0777, link: "elsewhere"}}},
			{"duplicate", []archiveEntry{{name: cliName(), body: "one", mode: 0755}, {name: cliName(), body: "two", mode: 0755}}},
			{"wrong-name", []archiveEntry{{name: "other-" + cliName(), body: "new-image", mode: 0755}}},
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
	t.Run("tar-hardlink", func(t *testing.T) {
		target := transactionTarget(t)
		archive := transactionArchive(t, "tar.gz", []archiveEntry{{name: cliName(), link: "elsewhere", mode: 0755}})
		result, err := ApplyTransaction(context.Background(), transactionFor(t, target, archive))
		if err == nil || result.Changed {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		assertImage(t, target, "old-image")
	})
}

func TestTransactionReadbackRollbackAndInstalled(t *testing.T) {
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
		return "", fmt.Errorf("invalid version output with private data")
	}
	result, err := ApplyTransaction(context.Background(), tx)
	if err == nil || result.Code != "UPDATE_READBACK_FAILED_ROLLED_BACK" || result.Installed != "0.15.6" || result.Changed {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if strings.Contains(err.Error(), target) || strings.Contains(err.Error(), "private data") {
		t.Fatalf("unsanitized error: %v", err)
	}
	assertImage(t, target, "old-image")
}

func TestTransactionForeignOldIsNeverClobberedOrCleaned(t *testing.T) {
	target := transactionTarget(t)
	foreign := target + ".old"
	if err := os.WriteFile(foreign, []byte("foreign"), 0600); err != nil {
		t.Fatal(err)
	}
	archive := transactionArchive(t, "zip", []archiveEntry{{name: cliName(), body: "new-image", mode: 0755}})
	result, err := ApplyTransaction(context.Background(), transactionFor(t, target, archive))
	if err == nil || result.Changed {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	assertImage(t, target, "old-image")
	assertImage(t, foreign, "foreign")
	_ = CleanupOld(target)
	assertImage(t, target, "old-image")
	assertImage(t, foreign, "foreign")
}

func TestTransactionExtractsExactCLI(t *testing.T) {
	for _, format := range []string{"zip", "tar.gz"} {
		t.Run(format, func(t *testing.T) {
			target := transactionTarget(t)
			archive := transactionArchive(t, format, []archiveEntry{{name: cliName(), body: "new-image", mode: 0755}, {name: "worker/worker", body: "not-cli", mode: 0755}})
			result, err := ApplyTransaction(context.Background(), transactionFor(t, target, archive))
			if err != nil || !result.Changed || result.Installed != "0.16.0" || result.Previous != "0.15.6" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			assertImage(t, target, "new-image")
		})
	}
}
