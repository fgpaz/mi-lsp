package runtimeinfo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileLease owns an exclusive advisory lock on a persistent guard file. The
// guard is deliberately never removed: unlinking it can split lock identity.
type FileLease struct {
	mu sync.Mutex
	file *os.File
	released bool
	err error
}

// AcquireFileLeaseContext acquires an exclusive OS lock on an absolute guard
// path. ctx applies only while acquiring; cancellation after return does not
// release the lease. The parent directory must already exist and be trusted.
func AcquireFileLeaseContext(ctx context.Context, absoluteGuardPath string) (*FileLease, error) {
	if ctx == nil { ctx = context.Background() }
	if err := ctx.Err(); err != nil { return nil, err }
	if !filepath.IsAbs(absoluteGuardPath) { return nil, errors.New("file lease path must be absolute") }
	path := filepath.Clean(absoluteGuardPath)
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil { return nil, fmt.Errorf("file lease parent: %w", err) }
	if !parent.IsDir() { return nil, errors.New("file lease parent is not a directory") }
	// Refuse pre-existing symlinks and non-regular files. Compare identity after
	// opening as a defense against replacement in the trusted directory.
	before, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) { return nil, fmt.Errorf("file lease guard: %w", err) }
	if err == nil && (!before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0) {
		return nil, errors.New("file lease guard is not a regular file")
	}
	f, err := openFileLeaseGuard(path)
	if err != nil { return nil, fmt.Errorf("open file lease guard: %w", err) }
	fail := func(e error) (*FileLease, error) { _ = f.Close(); return nil, e }
	opened, err := f.Stat()
	if err != nil { return fail(fmt.Errorf("stat file lease guard: %w", err)) }
	if !opened.Mode().IsRegular() { return fail(errors.New("file lease guard is not regular")) }
	after, err := os.Lstat(path)
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(opened, after) || (before != nil && !os.SameFile(before, opened)) {
		return fail(errors.New("file lease guard identity changed"))
	}
	for {
		if err := ctx.Err(); err != nil { return fail(err) }
		err = lockFile(f)
		if err == nil {
			// The pathname may have been replaced while we were waiting on the
			// OS lock. Verify the lock's handle still names the persistent guard.
			opened, statErr := f.Stat()
			current, pathErr := os.Lstat(path)
			if statErr != nil || pathErr != nil || !current.Mode().IsRegular() || !os.SameFile(opened, current) {
				_ = unlockFile(f)
				return fail(errors.New("file lease guard identity changed after lock"))
			}
			if e := ctx.Err(); e != nil { _ = unlockFile(f); return fail(e) }
			return &FileLease{file:f}, nil
		}
		if !isFileLeaseBusy(err) { return fail(fmt.Errorf("lock file lease guard: %w", err)) }
		t := time.NewTimer(15*time.Millisecond)
		select { case <-ctx.Done(): if !t.Stop() { <-t.C }; return fail(ctx.Err()); case <-t.C: }
	}
}

// Release unlocks and closes the owned handle exactly once. Repeated and
// concurrent calls return the same result.
func (l *FileLease) Release() error {
	if l == nil { return nil }
	l.mu.Lock(); defer l.mu.Unlock()
	if l.released { return l.err }
	l.released = true
	if l.file != nil {
		if err := unlockFile(l.file); err != nil { l.err = err }
		if err := l.file.Close(); l.err == nil && err != nil { l.err = err }
		l.file = nil
	}
	return l.err
}

