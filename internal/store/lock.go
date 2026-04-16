package store

import (
	"path/filepath"
	"strings"
	"sync"
)

var workspaceWriteLocks sync.Map

func WithWorkspaceWriteLock(root string, fn func() error) error {
	key := strings.ToLower(filepath.Clean(strings.TrimSpace(root)))
	value, _ := workspaceWriteLocks.LoadOrStore(key, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	return fn()
}
