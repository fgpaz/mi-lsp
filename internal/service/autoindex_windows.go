//go:build windows

package service

// lowerProcessPriority is a no-op on Windows.
func lowerProcessPriority() {}
