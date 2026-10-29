package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// These helpers concern only the transient startup reservation. They never
// release the server's OS lifetime lease or its instance-owner metadata.
func acquireStartReservationContext(ctx context.Context, timeout time.Duration) (*startLock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := daemonLockPath()
	if err != nil {
		return nil, err
	}
	return acquireStartReservationAtContext(ctx, path, timeout)
}

func acquireStartReservationAtContext(parent context.Context, path string, timeout time.Duration) (*startLock, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	guardPath := filepath.Join(filepath.Dir(path), "start.guard")
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var owner *startLock
		var retry bool
		err := withStartReservationGuardContext(ctx, guardPath, func() error {
			var err error
			owner, retry, err = acquireStartLockLocked(path, processExists, time.Now, randomStartLockNonce)
			return err
		})
		if err != nil {
			return nil, err
		}
		if owner != nil {
			return owner, nil
		}
		if !retry {
			return nil, errors.New("start reservation did not complete")
		}
		if err := waitStartReservationContext(ctx, startLockPollInterval); err != nil {
			return nil, err
		}
	}
}

func waitStartReservationContext(ctx context.Context, interval time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}

func withStartReservationGuardContext(ctx context.Context, path string, operation func() error) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	guard, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open start.guard: %w", err)
	}
	locked := false
	defer func() {
		if locked {
			resultErr = errors.Join(resultErr, unlockStartGuard(guard))
		}
		resultErr = errors.Join(resultErr, guard.Close())
	}()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		locked, err = tryLockStartGuard(guard)
		if err != nil {
			return fmt.Errorf("lock start.guard: %w", err)
		}
		if locked {
			break
		}
		if err := waitStartReservationContext(ctx, startLockPollInterval); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return operation()
}

// The single synchronous startup owner calls this before returning. Cancellation
// cannot create a detached cleanup writer or authorize deletion without the guard.
// If cancellation prevents guarded cleanup, PID/nonce metadata stays reserved.
// A later owner may reclaim it only through the guarded dead-owner rules.
func closeStartReservationContext(parent context.Context, owner *startLock) error {
	if owner == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	err := withStartReservationGuardContext(ctx, owner.guardPath, func() error {
		body, err := os.ReadFile(owner.path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		metadata, known := parseStartLockMetadata(body)
		if !known || metadata.pid != owner.pid || metadata.nonce != owner.nonce {
			return nil
		}
		if err := os.Remove(owner.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("startup reservation cleanup incomplete: %w", err)
	}
	return nil
}
