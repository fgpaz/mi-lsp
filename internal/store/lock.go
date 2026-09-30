package store

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
)

var workspaceWriteLocks sync.Map

type workspaceWriteLock struct {
	gate chan struct{}
}

func workspaceLock(root string) *workspaceWriteLock {
	key := strings.ToLower(filepath.Clean(strings.TrimSpace(root)))
	value, _ := workspaceWriteLocks.LoadOrStore(key, &workspaceWriteLock{gate: make(chan struct{}, 1)})
	return value.(*workspaceWriteLock)
}

func WithWorkspaceWriteLock(root string, fn func() error) error {
	lock := workspaceLock(root)
	lock.gate <- struct{}{}
	defer func() { <-lock.gate }()
	return fn()
}

func WithWorkspaceWriteLockContext(ctx context.Context, root string, fn func() error) error {
	if ctx == nil {
		return context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	lock := workspaceLock(root)
	select {
	case lock.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-lock.gate
			return err
		}
		defer func() { <-lock.gate }()
		return fn()
	case <-ctx.Done():
		return ctx.Err()
	}
}
