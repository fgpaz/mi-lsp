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

func TestApplyEnvelopeLimits_SingleItemDoesNotCollapseOnCharBudget(t *testing.T) {
	bulky := strings.Repeat("x", 4000)
	env := model.Envelope{
		Ok:    true,
		Items: []map[string]any{{"name": "status", "payload": bulky}},
	}

	got := ApplyEnvelopeLimits(env, model.QueryOptions{MaxItems: 50, MaxChars: 200})
	items, ok := got.Items.([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected single item preserved, got %#v", got.Items)
	}
	if !got.Truncated {
		t.Fatalf("expected truncated=true on char-budget overflow")
	}
	if got.NextHint == nil {
		t.Fatalf("expected next_hint to be set")
	}
	if strings.Contains(*got.NextHint, "--offset") {
		t.Fatalf("next_hint must not suggest --offset for single-item char-budget truncation; got %q", *got.NextHint)
	}
	if !strings.Contains(*got.NextHint, "token-budget") && !strings.Contains(*got.NextHint, "max-chars") {
		t.Fatalf("next_hint must guide user to raise budget or change format; got %q", *got.NextHint)
	}
}

func TestApplyEnvelopeLimits_SingleItemDoesNotEmitOffsetHintWhenTokenBudgetTight(t *testing.T) {
	bulky := strings.Repeat("y", 20000)
	env := model.Envelope{
		Ok:    true,
		Items: []map[string]any{{"workspace": "salud", "payload": bulky}},
	}

	got := ApplyEnvelopeLimits(env, model.QueryOptions{MaxItems: 50, TokenBudget: 4000})
	items, ok := got.Items.([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected single item preserved under TokenBudget, got %#v", got.Items)
	}
	if got.NextHint != nil && strings.Contains(*got.NextHint, "--offset") {
		t.Fatalf("next_hint must not suggest --offset; got %q", *got.NextHint)
	}
}

func TestApplyEnvelopeLimits_PaginatedTruncationKeepsOffsetHint(t *testing.T) {
	env := model.Envelope{
		Ok:    true,
		Items: []string{"a", "b", "c", "d", "e"},
	}

	got := ApplyEnvelopeLimits(env, model.QueryOptions{MaxItems: 2, Offset: 4, MaxChars: 200})
	if got.NextHint == nil || !strings.Contains(*got.NextHint, "--offset 6") {
		t.Fatalf("paginated truncation must preserve offset hint; got %#v", got.NextHint)
	}
}
