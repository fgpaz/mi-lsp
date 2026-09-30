package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestResultFromExecReturnsStructuredFindEnvelopeAndCompactText(t *testing.T) {
	stdout := `{"ok":true,"workspace":"mi-lsp-src","backend":"graph","items":[{"name":"SearchResult","kind":"type","file_path":"internal/mcp/server.go","line":40}],"truncated":false}`
	result := resultFromExec(ExecResult{Stdout: stdout})
	if result.IsError {
		t.Fatal("successful envelope marked as an MCP error")
	}
	if result.StructuredContent == nil {
		t.Fatal("structured envelope missing from MCP result")
	}
	if result.StructuredContent.Workspace != "mi-lsp-src" || result.StructuredContent.Backend != "graph" {
		t.Fatalf("structured envelope lost fields: %#v", result.StructuredContent)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("unexpected content: %#v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "workspace=mi-lsp-src") {
		t.Fatalf("content is not compact agent output: %q", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "SearchResult") || strings.Contains(result.Content[0].Text, "\"file_path\"") {
		t.Fatalf("content duplicated the structured envelope instead of rendering compactly: %q", result.Content[0].Text)
	}
}

func TestResultFromExecPreservesSearchMatchInStructuredAndCompactText(t *testing.T) {
	stdout := `{"ok":true,"workspace":"mi-lsp-src","backend":"text","items":[{"file":"internal/mcp/server.go","line":40,"text":"func Serve handles MCP results","score":0.9}],"truncated":false}`
	result := resultFromExec(ExecResult{Stdout: stdout})
	if result.StructuredContent == nil {
		t.Fatal("structured envelope missing from MCP result")
	}
	rawItems, err := json.Marshal(result.StructuredContent.Items)
	if err != nil {
		t.Fatalf("marshal structured search items: %v", err)
	}
	for _, expected := range []string{`"file":"internal/mcp/server.go"`, `"line":40`, `"text":"func Serve handles MCP results"`} {
		if !strings.Contains(string(rawItems), expected) {
			t.Fatalf("structured search items lost %s: %s", expected, rawItems)
		}
	}
	if len(result.Content) != 1 || !strings.Contains(result.Content[0].Text, "internal/mcp/server.go:40 func Serve handles MCP results") {
		t.Fatalf("compact MCP content lost the search match: %#v", result.Content)
	}
}

func TestResultFromExecPreservesFindNoMatchAndIndexErrors(t *testing.T) {
	indexHint := "Index the registered workspace with `mi-lsp index --workspace <workspace>` and retry `mi-lsp nav find <pattern> --workspace <workspace>`."
	tests := []struct {
		name     string
		stdout   string
		wantCode string
		wantHit  bool
	}{
		{
			name:    "valid catalog with a match",
			stdout:  `{"ok":true,"operation":"nav.find","workspace":"demo","backend":"catalog","items":[{"file_path":"sample.go","line":3,"name":"NeedleSymbol","kind":"function","language":"go"}],"truncated":false}`,
			wantHit: true,
		},
		{
			name:   "valid catalog with no matches",
			stdout: `{"ok":true,"operation":"nav.find","workspace":"demo","backend":"catalog","items":[],"truncated":false}`,
		},
		{
			name:     "registered workspace without catalog generation",
			stdout:   `{"ok":false,"operation":"nav.find","workspace":"demo","backend":"catalog","items":[],"error":{"kind":"index","code":"index_not_ready","message":"index_not_ready","stage":"catalog","hint_code":"index_not_ready","reason_code":"explicit_incomplete","detail":"` + indexHint + `"},"next_hint":"` + indexHint + `","truncated":false}`,
			wantCode: "index_not_ready",
		},
		{
			name:     "catalog generation state unavailable",
			stdout:   `{"ok":false,"operation":"nav.find","workspace":"demo","backend":"catalog","items":[],"error":{"kind":"index","code":"workspace_db_open_failed","message":"workspace_db_open_failed","stage":"catalog","hint_code":"workspace_db_open_failed","reason_code":"explicit_incomplete","detail":"` + indexHint + `"},"next_hint":"` + indexHint + `","truncated":false}`,
			wantCode: "workspace_db_open_failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := resultFromExec(ExecResult{Stdout: tc.stdout})
			if result.StructuredContent == nil {
				t.Fatal("structured find envelope missing from MCP result")
			}
			if result.StructuredContent.Workspace != "demo" || result.StructuredContent.Backend != "catalog" {
				t.Fatalf("structured envelope lost workspace/backend: %#v", result.StructuredContent)
			}
			items, ok := result.StructuredContent.Items.([]any)
			if !ok {
				t.Fatalf("structured items = %#v, want array", result.StructuredContent.Items)
			}
			if tc.wantCode == "" {
				if result.IsError || !result.StructuredContent.Ok || result.StructuredContent.Error != nil {
					t.Fatalf("zero-match result = %#v, want success without error", result)
				}
				if tc.wantHit {
					if len(items) != 1 || len(result.Content) != 1 || !strings.Contains(result.Content[0].Text, "NeedleSymbol") || !strings.Contains(result.Content[0].Text, "sample.go:3") {
						t.Fatalf("compact find result lost the symbol match: %#v", result.Content)
					}
					return
				}
				if len(items) != 0 {
					t.Fatalf("zero-match structured items = %#v, want empty array", items)
				}
				if len(result.Content) != 1 {
					t.Fatalf("zero-match content = %#v, want one compact block", result.Content)
				}
				if strings.Contains(result.Content[0].Text, "index_not_ready") {
					t.Fatalf("zero-match compact output contains an index error: %q", result.Content[0].Text)
				}
				return
			}
			if !result.IsError || result.StructuredContent.Ok {
				t.Fatalf("index error result = %#v, want MCP error and ok=false", result)
			}
			if result.StructuredContent.Error == nil || result.StructuredContent.Error.Code != tc.wantCode {
				t.Fatalf("structured error = %#v, want code %q", result.StructuredContent.Error, tc.wantCode)
			}
			if result.StructuredContent.Error.ReasonCode != "explicit_incomplete" || result.StructuredContent.Error.Detail != indexHint {
				t.Fatalf("structured reason/detail = %#v, want explicit_incomplete and actionable hint", result.StructuredContent.Error)
			}
			if result.StructuredContent.NextHint == nil || *result.StructuredContent.NextHint != indexHint {
				t.Fatalf("structured next_hint = %#v, want actionable index command", result.StructuredContent.NextHint)
			}
			if len(result.Content) != 1 || !strings.Contains(result.Content[0].Text, tc.wantCode) || !strings.Contains(result.Content[0].Text, "mi-lsp index --workspace <workspace>") {
				t.Fatalf("compact error content lost typed error or hint: %#v", result.Content)
			}
		})
	}
}

func TestBuildArgvRequestsJSONForStructuredMCPResults(t *testing.T) {
	argv, err := BuildArgv("nav_search", map[string]any{"query": "SearchResult"})
	if err != nil {
		t.Fatalf("BuildArgv: %v", err)
	}
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == "--format" && argv[i+1] == "json" {
			return
		}
	}
	t.Fatalf("argv does not request JSON envelope: %#v", argv)
}

func TestServeAppliesDefaultWorkspaceOnlyWhenToolSelectorIsOmitted(t *testing.T) {
	for _, tc := range []struct {
		name      string
		arguments string
		want      string
	}{
		{name: "default", arguments: `{"pattern":"needle"}`, want: "configured-default"},
		{name: "null selector defaults", arguments: `{"pattern":"needle","workspace":null}`, want: "configured-default"},
		{name: "explicit", arguments: `{"pattern":"needle","workspace":"tool-workspace"}`, want: "tool-workspace"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nav_search","arguments":%s}}`, tc.arguments)
			var output bytes.Buffer
			var gotArgv []string
			err := Serve(context.Background(), ServeConfig{
				In:               strings.NewReader(input + "\n"),
				Out:              &output,
				DefaultWorkspace: "configured-default",
				Exec: func(_ context.Context, _ string, argv []string) ExecResult {
					gotArgv = append([]string(nil), argv...)
					return ExecResult{Stdout: `{"ok":true,"operation":"nav.search","workspace":"` + tc.want + `","items":[],"truncated":false}`}
				},
			})
			if err != nil {
				t.Fatalf("Serve: %v", err)
			}
			found := false
			for i := 0; i+1 < len(gotArgv); i++ {
				if gotArgv[i] == "--workspace" && gotArgv[i+1] == tc.want {
					found = true
				}
			}
			if !found {
				t.Fatalf("argv=%#v, want workspace %q", gotArgv, tc.want)
			}
		})
	}
}
