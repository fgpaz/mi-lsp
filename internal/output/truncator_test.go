package output

import (
	"strings"
	"testing"
)

func TestCapRenderedNeverExceedsTinyReadbackBudget(t *testing.T) {
	input := []byte(`{"items":["large payload"],"continuation":{"next":{"op":"nav.search","query":"long continuation selector"}}}`)
	const limit = 24
	got := CapRendered(input, limit)
	if len(got) > limit {
		t.Fatalf("readback exceeded max_chars=%d: %d bytes: %q", limit, len(got), got)
	}
	if !strings.HasPrefix(string(got), "{") {
		t.Fatalf("expected bounded prefix, got %q", got)
	}
}
