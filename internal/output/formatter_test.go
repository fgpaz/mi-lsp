package output

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestTOONContinuationIsFirstAndNextIsExecutable(t *testing.T) {
	envelope := model.Envelope{
		Ok:        true,
		Workspace: "mi-lsp",
		Operation: "nav.refs",
		Continuation: &model.Continuation{
			Reason: "more references",
			Next: model.ContinuationTarget{
				Op:        "nav.refs",
				Symbol:    "App.find",
				Workspace: "mi-lsp",
			},
		},
	}

	toonOutput, err := Render(envelope, "toon", false)
	if err != nil {
		t.Fatal(err)
	}
	text := string(toonOutput)
	if !strings.HasPrefix(text, "continuation:") {
		t.Fatalf("TOON does not start with continuation: %q", text)
	}
	var nextLine string
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "mi-lsp nav refs") {
			nextLine = line
			break
		}
	}
	if nextLine == "" || !strings.Contains(nextLine, "--workspace") {
		t.Fatalf("TOON continuation lacks an executable workspace-scoped command: %q", text)
	}

	jsonOutput, err := Render(envelope, "json", false)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(jsonOutput, &decoded); err != nil {
		t.Fatal(err)
	}
	continuation, ok := decoded["continuation"].(map[string]any)
	if !ok {
		t.Fatalf("JSON continuation shape changed: %#v", decoded["continuation"])
	}
	next, ok := continuation["next"].(map[string]any)
	if !ok || next["op"] != "nav.refs" {
		t.Fatalf("JSON continuation target changed: %#v", continuation["next"])
	}
}

func TestTOONQContinuationIncludesPageCursorInCommand(t *testing.T) {
	envelope := model.Envelope{
		Ok: true,
		Continuation: &model.Continuation{
			Reason: "q_page",
			Next:   model.ContinuationTarget{Op: "q", Query: "sym App.find | limit 5", Workspace: "mi-lsp"},
			Cursor: "opaque-cursor",
		},
	}
	output, err := Render(envelope, "toon", false)
	if err != nil {
		t.Fatal(err)
	}
	text := string(output)
	if !strings.HasPrefix(text, "continuation:") || !strings.Contains(text, "mi-lsp q") || !strings.Contains(text, "--page") || !strings.Contains(text, "opaque-cursor") {
		t.Fatalf("TOON continuation command does not include the q cursor: %q", text)
	}
}
