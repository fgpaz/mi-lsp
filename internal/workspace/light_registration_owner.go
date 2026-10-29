package workspace

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/runtimeinfo"
)

var errLightRegistrationOwnerUnknown = errors.New("light registration generation is absent or indeterminate")

func lightRegistrationHex(value string, size int) bool {
	if len(value) != size {
		return false
	}
	for _, c := range value {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

// Receipts fence generations; they do not establish process liveness or kernel
// ownership. A root-only legacy/busy reference has no right to finish a worker.
func lightRegistrationReceiptParts(receipt string) (root, nonce string, valid bool) {
	if !filepath.IsAbs(receipt) || filepath.Clean(receipt) != receipt {
		return "", "", false
	}
	root = receipt
	if lightRegistrationHex(filepath.Base(receipt), 32) {
		root, nonce = filepath.Dir(receipt), filepath.Base(receipt)
	}
	name := filepath.Base(root)
	const prefix = ".light-register-"
	if !strings.HasPrefix(name, prefix) || !lightRegistrationHex(strings.TrimPrefix(name, prefix), 24) {
		return "", "", false
	}
	return root, nonce, true
}

func lightRegistrationProfileReceipt(receipt string) (root, nonce string, valid bool) {
	root, nonce, valid = lightRegistrationReceiptParts(receipt)
	if !valid {
		return "", "", false
	}
	registry, err := RegistryPath()
	if err != nil || filepath.Dir(root) != filepath.Clean(filepath.Dir(registry)) {
		return "", "", false
	}
	return root, nonce, true
}

// The stable guard is OUTSIDE the removable root and is never unlinked. The
// timeout bounds acquisition only: cancellation never drops an acquired lock.
func withLightRegistrationGuard(ctx context.Context, root string, run func() error) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	wait, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	lease, err := runtimeinfo.AcquireFileLeaseContext(wait, root+".guard")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lease.Release()) }()
	return run()
}

func reserveLightRegistrationAt(ctx context.Context, root string) (receipt string, owner bool, err error) {
	receipt = root
	err = withLightRegistrationGuard(ctx, root, func() error {
		if _, e := os.Lstat(root); e == nil {
			return nil
		} else if !os.IsNotExist(e) {
			return e
		}
		var generation [16]byte
		if _, e := rand.Read(generation[:]); e != nil {
			return e
		}
		nonce := hex.EncodeToString(generation[:])
		if e := os.Mkdir(root, 0o700); e != nil {
			return e
		}
		// Partial I/O leaves a busy root, never an age-based reclamation right.
		if e := os.Mkdir(filepath.Join(root, nonce), 0o700); e != nil {
			return e
		}
		receipt, owner = filepath.Join(root, nonce), true
		return nil
	})
	return receipt, owner, err
}

func ownsLightRegistrationLocked(root, nonce string) bool {
	if !lightRegistrationHex(nonce, 32) {
		return false
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != nonce || !entries[0].IsDir() {
		return false
	}
	child := filepath.Join(root, nonce)
	info, err = os.Lstat(child)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	contents, err := os.ReadDir(child)
	return err == nil && len(contents) == 0
}

func finishLightRegistrationLocked(root, nonce string, registrationErr error) error {
	if !ownsLightRegistrationLocked(root, nonce) {
		return errLightRegistrationOwnerUnknown
	}
	if registrationErr != nil {
		// Never follow or overwrite a pre-existing failure receipt. On any partial
		// publication error retain the generation conservatively.
		f, err := os.OpenFile(root+".failure", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, writeErr := f.WriteString(registrationErr.Error())
		if err = errors.Join(writeErr, f.Close()); err != nil {
			return err
		}
	}
	if err := os.Remove(filepath.Join(root, nonce)); err != nil {
		return err
	}
	return os.Remove(root)
}

func finishLightRegistrationAt(ctx context.Context, receipt string, registrationErr error) error {
	root, nonce, valid := lightRegistrationReceiptParts(receipt)
	if !valid || nonce == "" {
		return errLightRegistrationOwnerUnknown
	}
	return withLightRegistrationGuard(ctx, root, func() error { return finishLightRegistrationLocked(root, nonce, registrationErr) })
}

// The worker owns the physical guard across registry work AND conditional finish.
// A duplicate/stale invocation cannot mutate the registry after its generation
// ended. Crash/unknown/legacy roots deliberately require explicit recovery; no
// PID or directory age is treated as authority to steal or delete a generation.
func runLightRegistrationAt(ctx context.Context, receipt string, register func() error) error {
	root, nonce, valid := lightRegistrationReceiptParts(receipt)
	if !valid || nonce == "" {
		return errLightRegistrationOwnerUnknown
	}
	return withLightRegistrationGuard(ctx, root, func() error {
		if !ownsLightRegistrationLocked(root, nonce) {
			return errLightRegistrationOwnerUnknown
		}
		registrationErr := register()
		return errors.Join(registrationErr, finishLightRegistrationLocked(root, nonce, registrationErr))
	})
}

// RunLightRegistration preserves existing registration eligibility/registry-lock
// policy. ctx bounds guard acquisition, not legacy synchronous filesystem/Git or
// registry-lock waits; it is NOT a total worker deadline or process-tree join.
func RunLightRegistration(ctx context.Context, selector, cwd, receipt string) error {
	root, nonce, valid := lightRegistrationProfileReceipt(receipt)
	if !valid || nonce == "" {
		return errLightRegistrationOwnerUnknown
	}
	return runLightRegistrationAt(ctx, receipt, func() error {
		gitRoot, ok := autoRegisterCandidateRoot(selector, cwd)
		if !ok {
			return errors.New("light registration root no longer resolves")
		}
		sum := sha256.Sum256([]byte(filepath.Clean(gitRoot)))
		if filepath.Base(root) != ".light-register-"+hex.EncodeToString(sum[:12]) {
			return errors.New("light registration root no longer matches reservation")
		}
		_, err := RegisterGitRootLight(selector, cwd)
		return err
	})
}

// Legacy internal wrapper discards status; new callers use the result API.
func lightRegistrationFailureAt(ctx context.Context, root string) string {
	failure, _ := lightRegistrationFailureResultAt(ctx, root)
	return failure
}

func lightRegistrationFailureResultAt(ctx context.Context, root string) (failure string, err error) {
	err = withLightRegistrationGuard(ctx, root, func() error {
		path := root + ".failure"
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("light registration failure receipt is not a regular file")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		failure = strings.TrimSpace(string(data))
		return nil
	})
	return failure, err
}
