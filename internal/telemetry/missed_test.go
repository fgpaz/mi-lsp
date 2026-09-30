package telemetry

import "testing"

func TestClassifyToolCall(t *testing.T) {
	cases := []struct {
		obs  ToolObservation
		want string
	}{
		{ToolObservation{Tool: "Grep", Ext: ".go", PatternShape: "symbol"}, Missed},
		{ToolObservation{Tool: "Glob", Ext: ".cs", Wiki: true, PatternShape: "glob-or-regex"}, Missed},
		{ToolObservation{Tool: "Read", Ext: ".go", LargeRead: true, PatternShape: "path"}, Missed},
		{ToolObservation{Tool: "Grep", Ext: ".png", PatternShape: "literal"}, Acceptable},
		{ToolObservation{Tool: "Grep", Ext: ".md", PatternShape: "literal"}, Acceptable},
		{ToolObservation{Tool: "Read", Ext: ".go", LargeRead: false, PatternShape: "path"}, Unknown},
	}
	for _, tc := range cases {
		if got := ClassifyToolCall(tc.obs); got != tc.want {
			t.Fatalf("%s %s = %s, want %s", tc.obs.Tool, tc.obs.Ext, got, tc.want)
		}
	}
}

func TestAggregateRepeatsBecomeMissed(t *testing.T) {
	obs := []ToolObservation{
		{Harness: "claude-code", Tool: "Grep", Ext: ".txt", PatternShape: "literal", Repo: "mi-lsp"},
		{Harness: "claude-code", Tool: "Grep", Ext: ".txt", PatternShape: "literal", Repo: "mi-lsp"},
		{Harness: "claude-code", Tool: "Grep", Ext: ".txt", PatternShape: "literal", Repo: "mi-lsp"},
	}
	got := AggregateMisses(obs)
	if len(got) != 1 || got[0].Class != Missed || got[0].Count != 3 {
		t.Fatalf("aggregate = %+v", got)
	}
	if got[0].Recommendation == "" {
		t.Fatal("missing recommendation")
	}
	edits := AggregateMisses([]ToolObservation{
		{Harness: "claude-code", Tool: "Edit", PatternShape: "empty", Repo: "mi-lsp"},
		{Harness: "claude-code", Tool: "Edit", PatternShape: "empty", Repo: "mi-lsp"},
		{Harness: "claude-code", Tool: "Edit", PatternShape: "empty", Repo: "mi-lsp"},
	})
	if len(edits) != 1 || edits[0].Class != Unknown {
		t.Fatalf("edit aggregate = %+v", edits)
	}
}
