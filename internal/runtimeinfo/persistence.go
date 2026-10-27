package runtimeinfo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WithFileLock uses a non-blocking OS lock held for the duration of fn.
func WithFileLock(path string, fn func() error) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("lock path must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := lockFile(file); err != nil {
		return fmt.Errorf("acquire file lock: %w", err)
	}
	defer unlockFile(file)
	return fn()
}

// WriteJSONAtomic replaces the destination only after a complete synced write.
// The caller is responsible for validating existing state and taking the lock.
func WriteJSONAtomic(path string, value any) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("JSON path must be absolute")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".mi-lsp-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
