package store

import (
	"context"
	"errors"
	"testing"
)

func TestWithWorkspaceWriteLockContextDoesNotRunCallbackWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false

	err := WithWorkspaceWriteLockContext(ctx, t.TempDir(), func() error {
		called = true
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if called {
		t.Fatal("callback ran with a canceled context")
	}
}
