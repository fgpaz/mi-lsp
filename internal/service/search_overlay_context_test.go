package service

import (
	"context"
	"errors"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestSearchWorktreeOverlayDoesNotTreatCancellationAsNonLinked(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	items, linked, err := searchWorktreeOverlay(ctx, t.TempDir(), t.TempDir(), model.ProjectFile{}, "needle", false, 10, nil)
	if linked || !errors.Is(err, context.Canceled) || len(items) != 0 {
		t.Fatalf("searchWorktreeOverlay() = (%v, %v, %v), want no items, not healthy non-linked fallback, and context.Canceled", items, linked, err)
	}
}
