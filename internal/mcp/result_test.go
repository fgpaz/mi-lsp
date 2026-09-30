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
