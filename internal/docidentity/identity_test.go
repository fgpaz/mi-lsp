package docidentity

import (
	"reflect"
	"testing"
)

func TestExtractUsesCompleteLiteralBoundaries(t *testing.T) {
	text := "RF-X-1.extra RF-X-1.md, RF-X-1-EXTRA and RF-X-10. [[RF-X-1.md]]"
	got := Extract(text)
	want := []string{"RF-X-1", "RF-X-1-EXTRA", "RF-X-10"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Extract() = %#v, want %#v", got, want)
	}
}

func TestMatchAcceptsMarkdownAndTerminalPunctuation(t *testing.T) {
	for _, text := range []string{
		"[[RF-X-1.md]]",
		"[texto](RF-X-1.md)",
		"Referencia RF-X-1.",
		"Referencia RF-X-1,",
	} {
		if !Match(text, "RF-X-1") {
			t.Errorf("Match(%q) = false", text)
		}
	}
	for _, text := range []string{"RF-X-1.extra", "RF-X-1.md.EXTRA", "RF-X-10", "RF-X-1-EXTRA"} {
		if Match(text, "RF-X-1") {
			t.Errorf("Match(%q) = true, want false", text)
		}
	}
}
