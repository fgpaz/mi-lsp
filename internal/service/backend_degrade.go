package service

import (
	"strings"
	"time"
)

type backendCooldownEntry struct {
	Until  time.Time
	Reason string
}

func backendCooldownKey(workspaceRoot string, repoRoot string, backendType string) string {
	root := strings.TrimSpace(repoRoot)
	if root == "" {
		root = strings.TrimSpace(workspaceRoot)
	}
	return strings.ToLower(root) + "::" + strings.ToLower(strings.TrimSpace(backendType))
}

func (a *App) activeBackendCooldown(workspaceRoot string, repoRoot string, backendType string) (string, bool) {
	value, ok := a.backendCooldown.Load(backendCooldownKey(workspaceRoot, repoRoot, backendType))
	if !ok {
		return "", false
	}
	entry, ok := value.(backendCooldownEntry)
	if !ok || time.Now().After(entry.Until) {
		a.backendCooldown.Delete(backendCooldownKey(workspaceRoot, repoRoot, backendType))
		return "", false
	}
	return entry.Reason, true
}

func (a *App) markBackendCooldown(workspaceRoot string, repoRoot string, backendType string, reason string, ttl time.Duration) {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	a.backendCooldown.Store(backendCooldownKey(workspaceRoot, repoRoot, backendType), backendCooldownEntry{
		Until:  time.Now().Add(ttl),
		Reason: reason,
	})
}

func (a *App) clearBackendCooldown(workspaceRoot string, repoRoot string, backendType string) {
	a.backendCooldown.Delete(backendCooldownKey(workspaceRoot, repoRoot, backendType))
}

func shouldCooldownSemanticBackend(backendType string, err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch backendType {
	case "tsserver":
		return strings.Contains(message, "tsserver is unavailable") ||
			strings.Contains(message, "node is required") ||
			strings.Contains(message, "cannot find module") ||
			strings.Contains(message, "no such file or directory")
	case "pyright":
		return strings.Contains(message, "pyright") && strings.Contains(message, "unavailable")
	default:
		return false
	}
}
