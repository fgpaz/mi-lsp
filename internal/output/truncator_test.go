package output

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func TestApplyEnvelopeLimits_TruncationRecordsOmissions(t *testing.T) {
	env := model.Envelope{
		Ok:    true,
		Items: []string{"a", "b", "c"},
	}

	got := ApplyEnvelopeLimits(env, model.QueryOptions{MaxItems: 1})
	if len(got.Omissions) == 0 {
		t.Fatalf("expected omission metadata for max_items truncation")
	}
	if got.Omissions[0].ErrorCode != "max_items" {
		t.Fatalf("omission error_code = %q, want max_items", got.Omissions[0].ErrorCode)
	}
}

func TestCapRenderedLeavesShortTextUnchanged(t *testing.T) {
	in := []byte("ok=true backend=nav\ncontinuation: reason=low_evidence\n  next nav.related\n")
	if !bytes.Equal(CapRendered(in, 0), in) || !bytes.Equal(CapRendered(in, -5), in) {
		t.Fatal("non-positive maxChars changed the text")
	}
	if !bytes.Equal(CapRendered(in, len(in)), in) || !bytes.Equal(CapRendered(in, len(in)+10), in) {
		t.Fatal("short text changed")
	}
}

func TestCapRenderedTextKeepsContinuationAndWindowsPath(t *testing.T) {
	const winPath = `C:\repos\mios\file.go`
	env := cappedContinuationEnvelope(winPath)
	rendered, err := Render(env, "text", false)
	if err != nil {
		t.Fatalf("render text: %v", err)
	}
	const maxChars = 180
	if len(rendered) <= maxChars {
		t.Fatalf("fixture is not over budget: %d", len(rendered))
	}
	capped := CapRendered(rendered, maxChars)
	text := string(capped)
	marker := fmt.Sprintf("...[truncated %d chars; continuation preserved]...", len(rendered)-maxChars)
	if !strings.Contains(text, marker) {
		t.Fatalf("missing marker %q in %s", marker, text)
	}
	if !strings.Contains(text, "continuation: reason=low_evidence") || !strings.Contains(text, "next nav.related") {
		t.Fatalf("continuation.next dropped: %s", text)
	}
	if !strings.Contains(text, "path="+winPath) {
		t.Fatalf("windows path dropped or interpreted: %s", text)
	}
	if strings.Contains(text, "C:\r") || strings.Contains(text, "SHOULD_DROP_MP") {
		t.Fatalf("cap interpreted escapes or kept the suffix: %s", text)
	}
}

func TestCapRenderedJSONKeepsContinuationAndWindowsPath(t *testing.T) {
	const winPath = `C:\repos\mios\file.go`
	env := cappedContinuationEnvelope(winPath)
	rendered, err := Render(env, "json", false)
	if err != nil {
		t.Fatalf("render json: %v", err)
	}
	const maxChars = 220
	if len(rendered) <= maxChars {
		t.Fatalf("fixture is not over budget: %d", len(rendered))
	}
	capped := CapRendered(rendered, maxChars)
	text := string(capped)
	marker := fmt.Sprintf("...[truncated %d chars; continuation preserved]...", len(rendered)-maxChars)
	if !strings.Contains(text, marker) {
		t.Fatalf("missing marker %q in %s", marker, text)
	}
	if strings.Contains(text, "\r") || strings.Contains(text, "SHOULD_DROP_MP") {
		t.Fatalf("cap interpreted escapes or kept the suffix: %s", text)
	}
	if !strings.Contains(text, `C:\\repos\\mios\\file.go`) {
		t.Fatalf("json path escapes changed: %s", text)
	}
	got := decodeCappedContinuation(t, text)
	if got.Next.Op != "nav.related" || got.Next.Path != winPath || got.Reason != "low_evidence" {
		t.Fatalf("continuation = %+v", got)
	}
}

func TestCapRenderedKeepsContinuationWhenItExceedsBudget(t *testing.T) {
	const winPath = `C:\repos\mios\file.go`
	env := cappedContinuationEnvelope(winPath)
	env.Continuation.Next.Query = strings.Repeat("y", 400)
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			rendered, err := Render(env, format, false)
			if err != nil {
				t.Fatalf("render %s: %v", format, err)
			}
			const maxChars = 40
			capped := CapRendered(rendered, maxChars)
			text := string(capped)
			if len(capped) <= maxChars {
				t.Fatalf("continuation was truncated to the budget: %d", len(capped))
			}
			if !strings.Contains(text, fmt.Sprintf("...[truncated %d chars; continuation preserved]...", len(rendered)-maxChars)) {
				t.Fatalf("missing marker in %s", text)
			}
			if format == "text" {
				if !strings.Contains(text, "next nav.related") || !strings.Contains(text, "path="+winPath) || !strings.Contains(text, strings.Repeat("y", 400)) {
					t.Fatalf("text continuation was cut: %s", text)
				}
				return
			}
			got := decodeCappedContinuation(t, text)
			if got.Next.Op != "nav.related" || got.Next.Path != winPath || got.Next.Query != strings.Repeat("y", 400) {
				t.Fatalf("json continuation was cut: %+v", got.Next)
			}
		})
	}
}

func cappedContinuationEnvelope(winPath string) model.Envelope {
	return model.Envelope{
		Ok:        true,
		Backend:   "nav",
		Workspace: "mi-lsp",
		Items:     []map[string]any{{"name": "status"}},
		Hint:      strings.Repeat("x", 4000),
		Continuation: &model.Continuation{
			Reason: "low_evidence",
			Next: model.ContinuationTarget{
				Op:     "nav.related",
				Path:   winPath,
				Symbol: "Widget",
			},
		},
		MemoryPointer: &model.MemoryPointer{DocID: "SHOULD_DROP_MP", Why: "after continuation"},
	}
}

func decodeCappedContinuation(t *testing.T, text string) model.Continuation {
	t.Helper()
	idx := strings.LastIndex(text, `"continuation"`)
	if idx < 0 {
		t.Fatalf("missing continuation in %s", text)
	}
	rest := strings.TrimLeft(text[idx+len(`"continuation"`):], " \t\r\n")
	if !strings.HasPrefix(rest, ":") {
		t.Fatalf("continuation is not a key: %s", rest)
	}
	rest = strings.TrimLeft(rest[1:], " \t\r\n")
	var got model.Continuation
	if err := json.NewDecoder(strings.NewReader(rest)).Decode(&got); err != nil {
		t.Fatalf("decode continuation: %v\n%s", err, text)
	}
	return got
}

func TestApplyEnvelopeLimits_CharBudgetRecordsOmission(t *testing.T) {
	env := model.Envelope{
		Ok:    true,
		Items: []string{strings.Repeat("a", 80), strings.Repeat("b", 80)},
	}

	got := ApplyEnvelopeLimits(env, model.QueryOptions{MaxChars: 80})
	if !got.Truncated {
		t.Fatalf("expected truncated=true")
	}
	found := false
	for _, omission := range got.Omissions {
		if omission.ErrorCode == "char_budget" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected char_budget omission, got %#v", got.Omissions)
	}
}
