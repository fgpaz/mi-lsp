package daemon

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	startLockPollInterval     = 150 * time.Millisecond
	startLockLegacyStaleAfter = 5 * time.Minute
	startLockMetadataVersion  = 1
	startLockNonceBytes       = 16
	// Keep lock metadata in the cross-platform PID domain. Windows process
	// APIs use uint32, while int may be narrower on 32-bit builds; capping at
	// signed 32-bit max avoids uint32 aliasing and int truncation.
	startLockMaxPID = uint64(math.MaxInt32)
)

type startLock struct {
	path      string
	guardPath string
	pid       int
	nonce     string

	closeOnce sync.Once
	closeErr  error
}

type startLockMetadata struct {
	version int
	pid     int
	nonce   string
}

func acquireStartLock(timeout time.Duration) (*startLock, error) {
	path, err := daemonLockPath()
	if err != nil {
		return nil, err
	}
	return acquireStartLockAt(path, timeout, time.Now, processExists)
}

func acquireStartLockAt(path string, timeout time.Duration, now func() time.Time, ownerAlive func(int) bool) (*startLock, error) {
	return acquireStartLockAtWithNonce(path, timeout, now, ownerAlive, randomStartLockNonce)
}

func acquireStartLockAtWithNonce(path string, timeout time.Duration, now func() time.Time, ownerAlive func(int) bool, nonce func() (string, error)) (*startLock, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if now == nil {
		now = time.Now
	}
	if ownerAlive == nil {
		ownerAlive = processExists
	}
	if nonce == nil {
		nonce = randomStartLockNonce
	}
	guardPath := filepath.Join(filepath.Dir(path), "start.guard")
	deadline := now().Add(timeout)

	for {
		remaining := deadline.Sub(now())
		if remaining <= 0 {
			return nil, errors.New("timed out waiting for daemon start lock")
		}

		var owner *startLock
		var retry bool
		err := withStartGuard(guardPath, remaining, now, func() error {
			var err error
			owner, retry, err = acquireStartLockLocked(path, ownerAlive, now, nonce)
			return err
		})
		if err != nil {
			return nil, err
		}
		if owner != nil {
			return owner, nil
		}
		if !retry {
			return nil, errors.New("daemon start lock operation did not complete")
		}
		if remaining := deadline.Sub(now()); remaining <= 0 {
			return nil, errors.New("timed out waiting for daemon start lock")
		} else if remaining < startLockPollInterval {
			time.Sleep(remaining)
		} else {
			time.Sleep(startLockPollInterval)
		}
	}
}

// acquireStartLockLocked owns the guard for its entire operation. If reclaim
// succeeds, it immediately creates and metadata-fills the replacement before
// returning, so the guard is never released with an absent start.lock.
func acquireStartLockLocked(path string, ownerAlive func(int) bool, now func() time.Time, nonce func() (string, error)) (*startLock, bool, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err == nil {
		owner, createErr := finishStartLockCreate(path, file, nonce)
		if createErr != nil {
			return nil, false, createErr
		}
		return owner, false, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, false, err
	}

	reclaimed, err := reclaimStartLockLocked(path, now(), ownerAlive)
	if err != nil {
		return nil, false, err
	}
	if !reclaimed {
		return nil, true, nil
	}

	file, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, false, errors.New("start.lock reappeared while start.guard was held")
		}
		return nil, false, err
	}
	owner, err := finishStartLockCreate(path, file, nonce)
	if err != nil {
		return nil, false, err
	}
	return owner, false, nil
}

func finishStartLockCreate(path string, file *os.File, nonce func() (string, error)) (*startLock, error) {
	ownerNonce, err := nonce()
	if err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("generate start.lock nonce: %w", err)
	}
	if !validStartLockNonce(ownerNonce) {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, errors.New("generated start.lock nonce is invalid")
	}
	if err := writeStartLockMetadata(file, os.Getpid(), ownerNonce); err != nil {
		closeErr := file.Close()
		removeErr := os.Remove(path)
		return nil, errors.Join(err, closeErr, removeErr)
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	return &startLock{
		path:      path,
		guardPath: filepath.Join(filepath.Dir(path), "start.guard"),
		pid:       os.Getpid(),
		nonce:     ownerNonce,
	}, nil
}

func writeStartLockMetadata(file *os.File, pid int, nonce string) error {
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(file, "version=%d\npid=%d\nnonce=%s\n", startLockMetadataVersion, pid, nonce); err != nil {
		return err
	}
	return file.Sync()
}

func reclaimStartLockLocked(path string, now time.Time, ownerAlive func(int) bool) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, err
	}

	if len(body) == 0 {
		if !now.After(info.ModTime().Add(startLockLegacyStaleAfter)) {
			return false, nil
		}
	} else {
		metadata, known := parseStartLockMetadata(body)
		if !known {
			return false, nil
		}
		if ownerAlive == nil {
			ownerAlive = processExists
		}
		if ownerAlive(metadata.pid) {
			return false, nil
		}
	}

	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, err
	}
	return true, nil
}

func parseStartLockMetadata(body []byte) (startLockMetadata, bool) {
	lines := strings.Split(string(body), "\n")
	if len(lines) != 4 || lines[0] != "version=1" || lines[3] != "" {
		return startLockMetadata{}, false
	}
	if !strings.HasPrefix(lines[1], "pid=") || !strings.HasPrefix(lines[2], "nonce=") {
		return startLockMetadata{}, false
	}
	pidText := strings.TrimPrefix(lines[1], "pid=")
	nonce := strings.TrimPrefix(lines[2], "nonce=")
	if pidText == "" || !validStartLockNonce(nonce) {
		return startLockMetadata{}, false
	}
	for _, digit := range pidText {
		if digit < '0' || digit > '9' {
			return startLockMetadata{}, false
		}
	}
	pid64, err := strconv.ParseUint(pidText, 10, 32)
	if err != nil || pid64 == 0 || pid64 > startLockMaxPID {
		return startLockMetadata{}, false
	}
	return startLockMetadata{version: startLockMetadataVersion, pid: int(pid64), nonce: nonce}, true
}

func validStartLockNonce(nonce string) bool {
	if len(nonce) != startLockNonceBytes*2 || strings.ToLower(nonce) != nonce {
		return false
	}
	decoded, err := hex.DecodeString(nonce)
	return err == nil && len(decoded) == startLockNonceBytes
}

func randomStartLockNonce() (string, error) {
	bytes := make([]byte, startLockNonceBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func withStartGuard(path string, timeout time.Duration, now func() time.Time, operation func() error) error {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if now == nil {
		now = time.Now
	}
	guard, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open start.guard: %w", err)
	}
	defer func() {
		// The explicit close below is part of the returned error path. This defer
		// only protects callers from a panic in an injected operation.
		if guard != nil {
			_ = guard.Close()
		}
	}()

	deadline := now().Add(timeout)
	for {
		locked, lockErr := tryLockStartGuard(guard)
		if lockErr != nil {
			_ = guard.Close()
			guard = nil
			return fmt.Errorf("lock start.guard: %w", lockErr)
		}
		if locked {
			break
		}
		if !now().Before(deadline) {
			_ = guard.Close()
			guard = nil
			return errors.New("timed out waiting for start.guard")
		}
		remaining := deadline.Sub(now())
		if remaining < startLockPollInterval {
			time.Sleep(remaining)
		} else {
			time.Sleep(startLockPollInterval)
		}
	}

	operationErr := operation()
	unlockErr := unlockStartGuard(guard)
	closeErr := guard.Close()
	guard = nil
	return errors.Join(operationErr, unlockErr, closeErr)
}

func (l *startLock) Close() error {
	if l == nil {
		return nil
	}
	l.closeOnce.Do(func() {
		l.closeErr = withStartGuard(l.guardPath, 10*time.Second, time.Now, func() error {
			body, err := os.ReadFile(l.path)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return nil
				}
				return err
			}
			metadata, known := parseStartLockMetadata(body)
			if !known || metadata.pid != l.pid || metadata.nonce != l.nonce {
				return nil
			}
			if err := os.Remove(l.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return nil
		})
	})
	return l.closeErr
}
