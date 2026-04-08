package output

import (
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestApplyEnvelopeLimits_SetsPaginationHintWhenTruncated(t *testing.T) {
	env := model.Envelope{
		Ok:    true,
		Items: []string{"a", "b", "c"},
	}

	got := ApplyEnvelopeLimits(env, model.QueryOptions{MaxItems: 2, Offset: 4})
	if !got.Truncated {
		t.Fatalf("expected truncated=true")
	}
	if got.NextHint == nil {
		t.Fatalf("expected next_hint when truncated")
	}
	if !strings.Contains(*got.NextHint, "--offset 6") {
		t.Fatalf("next_hint = %q, want offset 6", *got.NextHint)
	}
}

func TestApplyEnvelopeLimits_PreservesExistingNextHint(t *testing.T) {
	existing := "rerun with --regex"
	env := model.Envelope{
		Ok:       true,
		Items:    []string{"a", "b", "c"},
		NextHint: &existing,
	}

	got := ApplyEnvelopeLimits(env, model.QueryOptions{MaxItems: 2, Offset: 4})
	if got.NextHint == nil || *got.NextHint != existing {
		t.Fatalf("next_hint = %#v, want %q", got.NextHint, existing)
	}
}
