package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/fgpaz/mi-lsp/internal/processutil"
)

type ReadbackFunc func(context.Context, string) (string, error)
type Transaction struct {
	ExecutablePath, ArchivePath, ExpectedArchiveSHA256, TargetVersion string
	Readback                                                          ReadbackFunc
}
type TransactionResult struct {
	Installed, Previous, Code string
	Changed                   bool
}
type transactionOps struct{ rename func(string, string) error }

const (
	txMaxArchive  = 512 << 20
	txMaxEntry    = 256 << 20
	txMaxExpanded = 512 << 20
)

var txSemver = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

// ReadInstalled invokes the image itself with the child marker; output and errors are never exposed.
func ReadInstalled(ctx context.Context, executable string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := processutil.CommandContext(ctx, executable, "version", "--format", "json")
	for _, v := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(v), "MI_LSP_UPDATE_CHILD=") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, "MI_LSP_UPDATE_CHILD=1")
	var output txLimitedBuffer
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		var errno syscall.Errno
		if errors.As(err, &errno) {
			return "", fmt.Errorf("installed version readback failed: %w", errno)
		}
		return "", errors.New("installed version readback failed")
	}
	if output.overflow {
		return "", errors.New("installed version readback failed")
	}
	var envelope struct {
		Ok    bool `json:"ok"`
		Items []struct {
			Version string `json:"version"`
		} `json:"items"`
	}
	if json.Unmarshal(output.data, &envelope) != nil || !envelope.Ok || len(envelope.Items) != 1 || !txSemver.MatchString(envelope.Items[0].Version) {
		return "", errors.New("invalid installed version response")
	}
	return envelope.Items[0].Version, nil
}

type txLimitedBuffer struct {
	data     []byte
	overflow bool
}

func (b *txLimitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if len(b.data)+n > 64<<10 {
		b.overflow = true
		return n, nil
	}
	b.data = append(b.data, p...)
	return n, nil
}

func ApplyTransaction(ctx context.Context, tx Transaction) (TransactionResult, error) {
	return applyTransaction(ctx, tx, transactionOps{})
}
func applyTransaction(ctx context.Context, tx Transaction, ops transactionOps) (result TransactionResult, err error) {
	fail := func(code string) (TransactionResult, error) {
		result.Code = code
		return result, errors.New("update transaction failed")
	}
	if tx.Readback == nil {
		tx.Readback = ReadInstalled
	}
	if ops.rename == nil {
		ops.rename = os.Rename
	}
	if tx.ExecutablePath == "" || !txSemver.MatchString(tx.TargetVersion) {
		return fail("UPDATE_READBACK_FAILED_ROLLED_BACK")
	}
	info, e := os.Lstat(tx.ExecutablePath)
	if e != nil {
		return fail(txErrorCode(e))
	}
	if !info.Mode().IsRegular() {
		return fail("UPDATE_PERMISSION_DENIED")
	}
	oldVersion, e := tx.Readback(ctx, tx.ExecutablePath)
	if e != nil || !txSemver.MatchString(oldVersion) {
		return fail("UPDATE_READBACK_FAILED_ROLLED_BACK")
	}
	result.Installed = oldVersion
	archive, e := os.Open(tx.ArchivePath)
	if e != nil {
		return fail(txErrorCode(e))
	}
	defer archive.Close()
	ai, e := archive.Stat()
	if e != nil || !ai.Mode().IsRegular() || ai.Size() > txMaxArchive {
		return fail("UPDATE_CHECKSUM_MISMATCH")
	}
	h := sha256.New()
	if _, e = io.Copy(h, archive); e != nil {
		return fail("UPDATE_CHECKSUM_MISMATCH")
	}
	expected, e := hex.DecodeString(tx.ExpectedArchiveSHA256)
	if e != nil || len(expected) != sha256.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), tx.ExpectedArchiveSHA256) {
		return fail("UPDATE_CHECKSUM_MISMATCH")
	}
	if _, e = archive.Seek(0, io.SeekStart); e != nil {
		return fail("UPDATE_CHECKSUM_MISMATCH")
	}
	// The stage is created next to the target, guaranteeing a same-filesystem rename.
	stage, e := os.CreateTemp(filepath.Dir(tx.ExecutablePath), ".mi-lsp-stage-*")
	if e != nil {
		return fail(txErrorCode(e))
	}
	stageName := stage.Name()
	defer os.Remove(stageName)
	e = txExtract(archive, ai.Size(), filepath.Base(tx.ArchivePath), tx.TargetVersion, stage)
	if e == nil {
		e = stage.Chmod(0755)
	}
	if e == nil {
		e = stage.Sync()
	}
	closeErr := stage.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		return fail(txErrorCode(e))
	}
	oldHash, e := txFileHash(tx.ExecutablePath)
	if e != nil {
		return fail(txErrorCode(e))
	}
	backup := tx.ExecutablePath + ".old"
	marker := backup + ".update-owner"
	if _, e = os.Lstat(backup); e == nil || !os.IsNotExist(e) {
		return fail("UPDATE_IN_USE_DEFERRED")
	}
	owner := txOwner{Schema: "mi-lsp-transaction-owner/v1", Source: txPathIdentity(tx.ExecutablePath), Hash: oldHash}
	mf, e := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return fail(txErrorCode(e))
	}
	encoded, _ := json.Marshal(owner)
	_, e = mf.Write(encoded)
	if e == nil {
		e = mf.Sync()
	}
	ce := mf.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		_ = os.Remove(marker)
		return fail(txErrorCode(e))
	}
	if e = txBackup(tx.ExecutablePath, backup); e != nil {
		_ = os.Remove(marker)
		return fail(txErrorCode(e))
	}
	// Rollback replaces the target atomically on Unix and uses a no-overwrite restore on Windows.
	restore := func() error {
		if runtime.GOOS == "windows" {
			if e := os.Remove(tx.ExecutablePath); e != nil && !os.IsNotExist(e) {
				return e
			}
		}
		return os.Rename(backup, tx.ExecutablePath)
	}
	if e = ops.rename(stageName, tx.ExecutablePath); e != nil {
		code := txErrorCode(e)
		if runtime.GOOS == "windows" {
			if restore() != nil {
				return fail(code)
			}
		} else {
			_ = os.Remove(backup)
		}
		_ = os.Remove(marker)
		if v, re := tx.Readback(ctx, tx.ExecutablePath); re == nil {
			result.Installed = v
		} else {
			result.Installed = ""
		}
		return fail(code)
	}
	version, re := tx.Readback(ctx, tx.ExecutablePath)
	if re != nil || version != tx.TargetVersion {
		code := "UPDATE_READBACK_FAILED_ROLLED_BACK"
		if _, statErr := os.Stat(tx.ExecutablePath); os.IsNotExist(statErr) {
			code = "UPDATE_QUARANTINED"
		} else if re != nil && txErrorCode(re) == "UPDATE_QUARANTINED" {
			code = "UPDATE_QUARANTINED"
		}
		if restore() != nil {
			result.Installed = ""
			return fail(code)
		}
		_ = os.Remove(marker)
		if actual, e := tx.Readback(ctx, tx.ExecutablePath); e == nil {
			result.Installed = actual
		} else {
			result.Installed = ""
		}
		return fail(code)
	}
	result.Installed = version
	result.Previous = oldVersion
	result.Changed = true
	return result, nil
}

func txErrorCode(err error) string {
	if errors.Is(err, os.ErrPermission) {
		return "UPDATE_PERMISSION_DENIED"
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		if runtime.GOOS == "windows" {
			switch uintptr(errno) {
			case 32, 33:
				return "UPDATE_IN_USE_DEFERRED"
			case 225, 226, 1260:
				return "UPDATE_QUARANTINED"
			}
		} else if uintptr(errno) == 26 {
			return "UPDATE_IN_USE_DEFERRED"
		}
	}
	return "UPDATE_READBACK_FAILED_ROLLED_BACK"
}

type txOwner struct {
	Schema string `json:"schema"`
	Source string `json:"source"`
	Hash   string `json:"hash"`
}

func txPathIdentity(p string) string {
	p, _ = filepath.Abs(p)
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	h := sha256.Sum256([]byte(filepath.Clean(p)))
	return hex.EncodeToString(h[:])
}
func txFileHash(p string) (string, error) {
	f, e := os.Open(p)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CleanupOld removes only an unchanged, owned backup when the platform proves it is not live.
func CleanupOld(executable string) error {
	backup := executable + ".old"
	marker := backup + ".update-owner"
	raw, e := os.ReadFile(marker)
	if e != nil {
		return errors.New("backup ownership unavailable")
	}
	if len(raw) > 4096 {
		return errors.New("backup ownership invalid")
	}
	var owner txOwner
	if json.Unmarshal(raw, &owner) != nil || owner.Schema != "mi-lsp-transaction-owner/v1" || owner.Source != txPathIdentity(executable) {
		return errors.New("backup ownership invalid")
	}
	info, e := os.Lstat(backup)
	if e != nil || !info.Mode().IsRegular() {
		return errors.New("backup unavailable")
	}
	hash, e := txFileHash(backup)
	if e != nil || hash != owner.Hash {
		return errors.New("backup identity changed")
	}
	safe, e := txBackupSafe(backup, info)
	if e != nil || !safe {
		return errors.New("backup liveness uncertain")
	}
	if e = os.Remove(backup); e != nil {
		return errors.New("backup removal deferred")
	}
	if e = os.Remove(marker); e != nil {
		return errors.New("backup marker removal deferred")
	}
	return nil
}

func txArchiveName(filename string) string {
	if strings.HasSuffix(filename, ".tar.gz") {
		return strings.TrimSuffix(filename, ".tar.gz")
	}
	return strings.TrimSuffix(filename, ".zip")
}
func txExtract(f *os.File, size int64, filename, version string, dst io.Writer) error {
	cli := "mi-lsp"
	osName, arch := runtime.GOOS, runtime.GOARCH
	if osName == "windows" {
		cli += ".exe"
		osName = "win"
	}
	if arch == "amd64" {
		arch = "x64"
	}
	wrapper := txArchiveName(filename)
	canonical := "mi-lsp_" + strings.TrimPrefix(version, "v") + "_" + osName + "-" + arch
	allowed := wrapper + "/" + cli
	seen := map[string]bool{}
	found := false
	var total int64
	check := func(name string, mode os.FileMode, entrySize int64) (bool, error) {
		if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || strings.Contains(name, ":") || path.Clean(name) != strings.TrimSuffix(name, "/") || name == ".." || strings.HasPrefix(name, "../") {
			return false, errors.New("unsafe archive path")
		}
		key := strings.TrimSuffix(name, "/")
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if seen[key] {
			return false, errors.New("duplicate archive path")
		}
		seen[key] = true
		if mode&os.ModeType != 0 && !mode.IsDir() {
			return false, errors.New("unsafe archive entry")
		}
		if entrySize < 0 || entrySize > txMaxEntry || total > txMaxExpanded-entrySize {
			return false, errors.New("archive size limit")
		}
		total += entrySize
		selected := name == cli || (wrapper == canonical && name == allowed)
		if selected {
			if found || !mode.IsRegular() {
				return false, errors.New("ambiguous archive executable")
			}
			found = true
		} else if path.Base(name) == cli {
			return false, errors.New("unexpected executable path")
		}
		return selected, nil
	}
	if strings.HasSuffix(filename, ".zip") {
		zr, e := zip.NewReader(f, size)
		if e != nil {
			return e
		}
		for _, entry := range zr.File {
			if entry.UncompressedSize64 > txMaxEntry {
				return errors.New("archive size limit")
			}
			selected, e := check(entry.Name, entry.Mode(), int64(entry.UncompressedSize64))
			if e != nil {
				return e
			}
			if !selected {
				continue
			}
			r, e := entry.Open()
			if e != nil {
				return e
			}
			n, e := io.Copy(dst, io.LimitReader(r, txMaxEntry+1))
			ce := r.Close()
			if e != nil {
				return e
			}
			if ce != nil {
				return ce
			}
			if n != int64(entry.UncompressedSize64) || n > txMaxEntry {
				return errors.New("archive size mismatch")
			}
		}
	} else if strings.HasSuffix(filename, ".tar.gz") {
		gz, e := gzip.NewReader(f)
		if e != nil {
			return e
		}
		defer gz.Close()
		tr := tar.NewReader(gz)
		for {
			h, e := tr.Next()
			if e == io.EOF {
				break
			}
			if e != nil {
				return e
			}
			if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA && h.Typeflag != tar.TypeDir {
				return errors.New("unsafe archive entry")
			}
			mode := os.FileMode(h.Mode)
			if h.Typeflag == tar.TypeDir {
				mode |= os.ModeDir
			}
			selected, e := check(h.Name, mode, h.Size)
			if e != nil {
				return e
			}
			if selected {
				n, e := io.Copy(dst, io.LimitReader(tr, txMaxEntry+1))
				if e != nil || n != h.Size {
					return errors.New("archive size mismatch")
				}
			}
		}
	} else {
		return errors.New("unknown archive format")
	}
	if !found {
		return errors.New("archive executable absent")
	}
	return nil
}
