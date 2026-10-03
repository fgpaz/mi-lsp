package output

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestWindowAgentTextCentersOnMatch(t *testing.T) {
	text := "    " + strings.Repeat("a", 300) + "NEEDLE" + strings.Repeat("b", 300)
	col := 4 + 300 + 1
	got := windowAgentText(text, col, agentSnippetRunes)
	if utf8.RuneCountInString(got) > agentSnippetRunes {
		t.Fatalf("snippet has %d runes, want <= %d", utf8.RuneCountInString(got), agentSnippetRunes)
	}
	if !strings.Contains(got, "NEEDLE") || !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("snippet=%q want window around match with both ellipses", got)
	}
}

func TestWindowAgentTextShortAndUnknownColumn(t *testing.T) {
	if got := windowAgentText("   \tfunc   Run()  ", 0, agentSnippetRunes); got != "func Run()" {
		t.Fatalf("short snippet=%q", got)
	}
	long := strings.Repeat("x", 400)
	got := windowAgentText(long, 0, agentSnippetRunes)
	if utf8.RuneCountInString(got) != agentSnippetRunes || strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("unknown column snippet=%q want head truncation", got)
	}
	tail := strings.Repeat("y", 400) + "END"
	got = windowAgentText(tail, 401, agentSnippetRunes)
	if !strings.HasSuffix(got, "END") || utf8.RuneCountInString(got) > agentSnippetRunes {
		t.Fatalf("tail snippet=%q want window ending at line end", got)
	}
}

func TestRenderAgentGroupsConsecutiveHitsByFile(t *testing.T) {
	env := model.Envelope{Workspace: "demo", Items: []map[string]any{
		{"file": "a/one.go", "line": 3, "text": "\tfoo()", "origin": "text"},
		{"file": "a/one.go", "line": 9, "text": "bar(foo)", "origin": "text"},
		{"file": "b/two.go", "line": 1, "text": "foo", "origin": "text"},
		{"file": "c/three.go", "line": 4, "text": "foo()", "caller": map[string]any{"name": "Run"}, "origin": "catalog"},
	}}
	got := renderAgent(env)
	want := strings.Join([]string{
		"workspace=demo",
		"a/one.go",
		"  3: foo()",
		"  9: bar(foo)",
		"b/two.go:1 foo",
		"c/three.go:4 foo() [in Run] (origin=catalog)",
	}, "\n")
	if got != want {
		t.Fatalf("renderAgent=\n%s\nwant\n%s", got, want)
	}
}

func TestRenderAgentGroupedIsCheaperThanRgLines(t *testing.T) {
	items := []map[string]any{}
	rg := []string{}
	for line := 1; line <= 6; line++ {
		items = append(items, map[string]any{"file": "internal/service/long_name_file.go", "line": line, "text": "foo()", "origin": "text"})
		rg = append(rg, "internal/service/long_name_file.go:"+string(rune('0'+line))+":foo()")
	}
	got := renderAgent(model.Envelope{Workspace: "d", Items: items})
	if len(got) >= len(strings.Join(rg, "\n")) {
		t.Fatalf("agent output %d bytes, rg-like %d bytes", len(got), len(strings.Join(rg, "\n")))
	}
}

func TestRenderAgentHandlesJSONNumbersAndWindowsLongHits(t *testing.T) {
	text := strings.Repeat("a", 300) + "needle" + strings.Repeat("b", 300)
	got := renderAgent(model.Envelope{Workspace: "d", Items: []map[string]any{
		{"file": "x.js", "line": float64(7), "col": float64(301), "text": text},
	}})
	if !strings.HasPrefix(got, "workspace=d\nx.js:7 …") || !strings.Contains(got, "needle") {
		t.Fatalf("renderAgent=%q", got)
	}
}
