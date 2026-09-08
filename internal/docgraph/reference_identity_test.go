package docgraph

import (
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestExtractReferencesUsesSharedCompleteIdentifierGrammar(t *testing.T) {
	mentions, edges := extractReferences(t.TempDir(), "wiki/source.md", "RF-X-1.extra RF-X-1.md, RF-X-1-EXTRA RF-X-10.")
	want := map[string]bool{"RF-X-1": true, "RF-X-1-EXTRA": true, "RF-X-10": true}
	got := map[string]bool{}
	for _, mention := range mentions {
		if mention.MentionType == model.DocMentionTypeDocID {
			got[mention.MentionValue] = true
		}
	}
	for id := range want {
		if !got[id] {
			t.Errorf("missing complete reference %q in %#v", id, got)
		}
	}
	if got["RF-X-1.extra"] {
		t.Error("dotted continuation was extracted as a document ID")
	}
	if len(edges) != len(got) {
		t.Fatalf("edges=%d mentions=%d, want one edge per ID", len(edges), len(got))
	}
}
