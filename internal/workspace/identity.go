package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// WorkspaceIdentity keeps the user-facing root separate from the physical
// identity used for comparisons and state keys.
type WorkspaceIdentity struct {
	DisplayRoot    string
	CanonicalRoot  string
	ComparableRoot string
	Exists         bool
}

const (
	WorkspaceSelectorNotFound     = "WKS_SELECTOR_NOT_FOUND"
	WorkspaceSelectorStale        = "WKS_SELECTOR_STALE"
	WorkspaceSelectorNotDirectory = "WKS_SELECTOR_NOT_DIRECTORY"
)

type WorkspaceSelectorError struct {
	Code     string
	Selector string
	Root     string
	Cause    error
}

func (e *WorkspaceSelectorError) Error() string {
	if e == nil {
		return "workspace selector failed"
	}
	selector := strings.TrimSpace(e.Selector)
	switch e.Code {
	case WorkspaceSelectorStale:
		return fmt.Sprintf("workspace selector %q is stale", selector)
	case WorkspaceSelectorNotFound:
		return fmt.Sprintf("workspace selector %q was not found", selector)
	case WorkspaceSelectorNotDirectory:
		return fmt.Sprintf("workspace selector %q is not a directory", selector)
	default:
		if selector == "" {
			return "workspace selector failed"
		}
		return fmt.Sprintf("workspace selector %q failed", selector)
	}
}

func (e *WorkspaceSelectorError) Unwrap() error { return e.Cause }

func AsWorkspaceSelectorError(err error) (*WorkspaceSelectorError, bool) {
	var selectorErr *WorkspaceSelectorError
	if errors.As(err, &selectorErr) {
		return selectorErr, true
	}
	return nil, false
}

// InspectWorkspaceIdentity resolves an existing path through symlinks/junctions
// while preserving the cleaned absolute spelling supplied by the caller.
func InspectWorkspaceIdentity(path string) (WorkspaceIdentity, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return WorkspaceIdentity{}, errors.New("workspace root is required")
	}
	absolute, err := filepath.Abs(trimmed)
	if err != nil {
		return WorkspaceIdentity{}, err
	}
	display := filepath.Clean(absolute)
	identity := WorkspaceIdentity{DisplayRoot: display, CanonicalRoot: display}
	info, statErr := os.Stat(display)
	if statErr == nil {
		identity.Exists = true
		if evaluated, evalErr := filepath.EvalSymlinks(display); evalErr == nil {
			identity.CanonicalRoot = filepath.Clean(evaluated)
		}
		if !info.IsDir() {
			return WorkspaceIdentity{}, &WorkspaceSelectorError{Code: WorkspaceSelectorNotDirectory, Selector: trimmed, Root: display}
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return WorkspaceIdentity{}, statErr
	}
	identity.ComparableRoot = comparableRoot(identity.CanonicalRoot)
	return identity, nil
}

func ComparableWorkspacePath(path string) (string, bool) {
	identity, err := InspectWorkspaceIdentity(path)
	if err != nil || strings.TrimSpace(identity.ComparableRoot) == "" {
		return "", false
	}
	return identity.ComparableRoot, true
}

func comparableRoot(path string) string {
	cleaned := filepath.Clean(path)
	if caseInsensitivePlatform() {
		return strings.ToLower(cleaned)
	}
	return cleaned
}

func caseInsensitivePlatform() bool {
	return runtime.GOOS == "windows" || runtime.GOOS == "darwin"
}

func IsCaseInsensitivePlatform() bool { return caseInsensitivePlatform() }

func WorkspaceDisplayAndCanonicalRoot(path string) (string, string, error) {
	identity, err := InspectWorkspaceIdentity(path)
	if err != nil {
		return "", "", err
	}
	return identity.DisplayRoot, identity.CanonicalRoot, nil
}
