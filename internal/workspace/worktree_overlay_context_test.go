package workspace

import (
	"context"
	"errors"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestWorktreeGitResolutionReturnsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, ok, err := gitTopLevelContext(ctx, t.TempDir()); ok || !errors.Is(err, context.Canceled) {
		t.Fatalf("gitTopLevelContext() = (_, %v, %v), want canceled and not resolved", ok, err)
	}
	if _, _, _, err := linkedWorktreeContext(ctx, t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatalf("linkedWorktreeContext() error = %v, want context.Canceled", err)
	}
	if _, linked, err := TryResolveWorktreeOverlayContext(ctx, t.TempDir()); linked || !errors.Is(err, context.Canceled) {
		t.Fatalf("TryResolveWorktreeOverlayContext() = (_, %v, %v), want canceled and not linked", linked, err)
	}
	if _, err := ResolveWorktreeOverlayContext(ctx, t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveWorktreeOverlayContext() error = %v, want context.Canceled", err)
	}
	if _, found, err := RegisteredWorktreeBaseContext(ctx, WorktreeOverlay{}); found || !errors.Is(err, context.Canceled) {
		t.Fatalf("RegisteredWorktreeBaseContext() = (_, %v, %v), want canceled and not found", found, err)
	}
}

func TestRegisteredMainWorktreeContextCancellationIsNotNoMatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	registry := model.RegistryFile{Workspaces: map[string]model.WorkspaceRegistration{}}
	if _, found, err := registeredMainWorktreeContext(ctx, "", registry); found || !errors.Is(err, context.Canceled) {
		t.Fatalf("registeredMainWorktreeContext() = (_, %v, %v), want canceled and not found", found, err)
	}
}
