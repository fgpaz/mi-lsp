package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/mcp"
)

func TestMCPCommandIsRegistered(t *testing.T) {
	root := NewRootCommand()
	command, _, err := root.Find([]string{"mcp"})
	if err != nil {
		t.Fatalf("Find mcp: %v", err)
	}
	if command == nil || command.Name() != "mcp" {
		t.Fatalf("command = %#v, want mcp", command)
	}
}

func TestMCPBuildArgv(t *testing.T) {
	cases := []struct {
		tool string
		args map[string]any
		want []string
	}{
		{
			tool: "nav_intent",
			args: map[string]any{"question": "how does X work", "workspace": "ws", "top": 5, "offset": 2, "repo": "child"},
			want: []string{"nav", "intent", "how does X work", "--format", "toon", "--client-name", "mi-lsp-mcp", "--top", "5", "--offset", "2", "--repo", "child", "--workspace", "ws"},
		},
		{
			tool: "nav_route",
			args: map[string]any{"task": "which doc governs X", "workspace": "ws", "includeCodeDiscovery": true},
			want: []string{"nav", "route", "which doc governs X", "--format", "toon", "--client-name", "mi-lsp-mcp", "--workspace", "ws", "--full", "--include-code-discovery"},
		},
		{
			tool: "nav_pack",
			args: map[string]any{"task": "reading", "workspace": "ws", "doc": "a.md", "fl": "FL-1", "rf": "RF-1"},
			want: []string{"nav", "pack", "reading", "--format", "toon", "--client-name", "mi-lsp-mcp", "--doc", "a.md", "--fl", "FL-1", "--rf", "RF-1", "--workspace", "ws"},
		},
		{
			tool: "nav_wiki",
			args: map[string]any{"op": "search", "query": "q", "workspace": "ws", "layer": "RS,RF", "includeContent": true, "top": 3, "allWorkspaces": true},
			want: []string{"nav", "wiki", "search", "q", "--format", "toon", "--client-name", "mi-lsp-mcp", "--workspace", "ws", "--layer", "RS,RF", "--include-content", "--top", "3", "--all-workspaces"},
		},
		{
			tool: "nav_wiki",
			args: map[string]any{"op": "route", "query": "t", "workspace": "ws", "allWorkspaces": true},
			want: []string{"nav", "wiki", "route", "t", "--format", "toon", "--client-name", "mi-lsp-mcp", "--workspace", "ws", "--all-workspaces"},
		},
		{
			tool: "nav_wiki",
			args: map[string]any{"op": "pack", "query": "t", "workspace": "ws", "doc": "d", "fl": "f", "rf": "r", "allWorkspaces": true},
			want: []string{"nav", "wiki", "pack", "t", "--format", "toon", "--client-name", "mi-lsp-mcp", "--workspace", "ws", "--doc", "d", "--fl", "f", "--rf", "r", "--all-workspaces"},
		},
		{
			tool: "nav_wiki",
			args: map[string]any{"op": "trace", "query": "RF-1", "workspace": "ws"},
			want: []string{"nav", "wiki", "trace", "RF-1", "--format", "toon", "--client-name", "mi-lsp-mcp", "--workspace", "ws"},
		},
		{
			tool: "nav_wiki",
			args: map[string]any{"op": "trace", "query": "RF-1", "all": true, "summary": true, "allWorkspaces": true, "workspace": "ws"},
			want: []string{"nav", "wiki", "trace", "--format", "toon", "--client-name", "mi-lsp-mcp", "--workspace", "ws", "--all", "--summary", "--all-workspaces"},
		},
		{
			tool: "nav_wiki",
			args: map[string]any{"op": "inventory", "workspace": "ws", "withLayerCounts": true, "allWorkspaces": true},
			want: []string{"nav", "wiki", "inventory", "--format", "toon", "--client-name", "mi-lsp-mcp", "--workspace", "ws", "--with-layer-counts", "--all-workspaces"},
		},
		{
			tool: "nav_wiki",
			args: map[string]any{"op": "map", "workspace": "ws"},
			want: []string{"nav", "wiki", "map", "--format", "toon", "--client-name", "mi-lsp-mcp", "--workspace", "ws"},
		},
		{
			tool: "nav_wiki",
			args: map[string]any{"op": "root", "role": "producto", "workspace": "ws"},
			want: []string{"nav", "wiki", "root", "--format", "toon", "--client-name", "mi-lsp-mcp", "--workspace", "ws", "--role", "producto"},
		},
		{
			tool: "nav_search",
			args: map[string]any{"pattern": "needle", "workspace": "ws", "includeContent": true, "regex": true, "contextLines": 4, "contextMode": "symbol", "allWorkspaces": true},
			want: []string{"nav", "search", "needle", "--format", "toon", "--client-name", "mi-lsp-mcp", "--include-content", "--regex", "--context-lines", "4", "--context-mode", "symbol", "--all-workspaces", "--workspace", "ws"},
		},
		{
			tool: "nav_find",
			args: map[string]any{"pattern": "Foo", "workspace": "ws", "exact": true, "kind": "class", "allWorkspaces": true, "offset": 1},
			want: []string{"nav", "find", "Foo", "--format", "toon", "--client-name", "mi-lsp-mcp", "--exact", "--kind", "class", "--all-workspaces", "--offset", "1", "--workspace", "ws"},
		},
		{
			tool: "nav_refs",
			args: map[string]any{"symbol": "Bar", "workspace": "ws", "file": "a.go", "line": 10, "entrypoint": "e", "project": "p", "solution": "s"},
			want: []string{"nav", "refs", "Bar", "--format", "toon", "--client-name", "mi-lsp-mcp", "--file", "a.go", "--line", "10", "--entrypoint", "e", "--project", "p", "--solution", "s", "--workspace", "ws"},
		},
		{
			tool: "nav_related",
			args: map[string]any{"symbol": "Bar", "workspace": "ws", "depth": "callers", "entrypoint": "e", "project": "p", "solution": "s"},
			want: []string{"nav", "related", "Bar", "--format", "toon", "--client-name", "mi-lsp-mcp", "--depth", "callers", "--entrypoint", "e", "--project", "p", "--solution", "s", "--workspace", "ws"},
		},
		{
			tool: "nav_flow_slice",
			args: map[string]any{"from": "A", "to": "B", "selector": "S", "limit": 3, "workspace": "ws"},
			want: []string{"nav", "flow-slice", "--format", "toon", "--client-name", "mi-lsp-mcp", "--from", "A", "--to", "B", "--selector", "S", "--limit", "3", "--workspace", "ws"},
		},
		{
			tool: "nav_change_pack",
			args: map[string]any{"ref": "HEAD~1", "paths": []any{"a.go", "b.go"}, "limit": 4, "workspace": "ws"},
			want: []string{"nav", "change-pack", "HEAD~1", "--format", "toon", "--client-name", "mi-lsp-mcp", "--path", "a.go", "--path", "b.go", "--limit", "4", "--workspace", "ws"},
		},
		{
			tool: "nav_affected",
			args: map[string]any{"paths": []any{"a.go", "b.go"}, "workspace": "ws", "changedRef": "origin/main", "fromGitDiff": true, "mode": "transitive", "includeTests": true, "includeDocs": true, "limit": 7},
			want: []string{"nav", "affected", "a.go", "b.go", "--format", "toon", "--client-name", "mi-lsp-mcp", "--changed-ref", "origin/main", "--from-git-diff", "--mode", "transitive", "--include-tests", "--include-docs", "--limit", "7", "--workspace", "ws"},
		},
		{
			tool: "nav_multi_read",
			args: map[string]any{"ranges": []any{"src/foo.ts:10-42", "src/bar.ts:1-2"}, "workspace": "ws"},
			want: []string{"nav", "multi-read", "src/foo.ts:10-42", "src/bar.ts:1-2", "--format", "toon", "--client-name", "mi-lsp-mcp", "--workspace", "ws"},
		},
		{
			tool: "nav_overview",
			args: map[string]any{"dir": "internal", "offset": 5, "workspace": "ws", "workspaceMap": false},
			want: []string{"nav", "overview", "internal", "--format", "toon", "--client-name", "mi-lsp-mcp", "--offset", "5", "--workspace", "ws"},
		},
		{
			tool: "nav_overview",
			args: map[string]any{"dir": "internal", "workspaceMap": true, "workspace": "ws"},
			want: []string{"nav", "workspace-map", "--format", "toon", "--client-name", "mi-lsp-mcp", "--workspace", "ws"},
		},
	}

	listed, err := mcp.ToolList()
	if err != nil {
		t.Fatalf("ToolList: %v", err)
	}
	seen := map[string]bool{}
	known := map[string]bool{}
	for _, info := range listed {
		known[info.Name] = true
	}
	for _, tc := range cases {
		t.Run(tc.tool+"/"+tc.want[1], func(t *testing.T) {
			got, err := mcp.BuildArgv(tc.tool, tc.args)
			if err != nil {
				t.Fatalf("BuildArgv: %v", err)
			}
			if strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
				t.Fatalf("argv = %#v\nwant %#v", got, tc.want)
			}
			requireFlag(t, got, "--workspace", "ws")
			for _, arg := range got {
				if strings.Contains(strings.ToLower(arg), "claude") {
					t.Fatalf("product-specific client leaked into argv: %#v", got)
				}
			}
		})
		if !known[tc.tool] {
			t.Fatalf("argv case for unknown tool %s", tc.tool)
		}
		seen[tc.tool] = true
	}
	if len(seen) != len(known) {
		t.Fatalf("argv cases cover %d tools, list has %d", len(seen), len(known))
	}
}

func TestMCPArgvOmitsFalseAndEmpty(t *testing.T) {
	got, err := mcp.BuildArgv("nav_search", map[string]any{
		"pattern":        "n",
		"regex":          false,
		"includeContent": false,
		"workspace":      "",
	})
	if err != nil {
		t.Fatalf("BuildArgv: %v", err)
	}
	want := []string{"nav", "search", "n", "--format", "toon", "--client-name", "mi-lsp-mcp"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("argv = %#v, want %#v", got, want)
	}
}

func TestMCPArgvKeepsShellMetacharactersInOneElement(t *testing.T) {
	pattern := "needle; rm -rf / && echo pwned `id` $(whoami)"
	got, err := mcp.BuildArgv("nav_search", map[string]any{"pattern": pattern, "workspace": "ws"})
	if err != nil {
		t.Fatalf("BuildArgv: %v", err)
	}
	found := false
	for _, arg := range got {
		if arg == pattern {
			found = true
		}
		switch arg {
		case "cmd.exe", "cmd", "/c", "sh", "-c", "bash", "powershell.exe":
			t.Fatalf("shell token in argv: %#v", got)
		}
	}
	if !found {
		t.Fatalf("pattern was split or dropped: %#v", got)
	}
	bin := filepath.Join(t.TempDir(), "mi-lsp.exe")
	cmd, err := mcp.NewCommand(context.Background(), bin, got)
	if err != nil {
		t.Fatalf("NewCommand: %v", err)
	}
	if cmd.Path != bin {
		t.Fatalf("Path = %q, want %q", cmd.Path, bin)
	}
	if cmd.Stdin != nil {
		t.Fatal("child stdin must not be attached to the MCP stream")
	}
	if len(cmd.Args) != len(got)+1 {
		t.Fatalf("shell wrapper changed argc: %#v", cmd.Args)
	}
	if cmd.Args[0] != bin {
		t.Fatalf("Args[0] = %q", cmd.Args[0])
	}
	for i, arg := range got {
		if cmd.Args[i+1] != arg {
			t.Fatalf("Args[%d] = %q, want %q", i+1, cmd.Args[i+1], arg)
		}
	}
	assertNoConsoleShell(t, cmd)
}

func TestMCPNewCommandRefusesShell(t *testing.T) {
	_, err := mcp.NewCommand(context.Background(), `C:\Windows\System32\cmd.exe`, []string{"nav", "search", "x"})
	if err == nil {
		t.Fatal("expected cmd.exe to be refused before spawn")
	}
	_, err = mcp.NewCommand(context.Background(), "/bin/sh", []string{"-c", "nav"})
	if err == nil {
		t.Fatal("expected sh to be refused before spawn")
	}
}

func TestMCPResolveBinary(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "mi-lsp.exe")
	t.Setenv("MI_LSP_BIN", bin)
	got, err := mcp.ResolveBinary()
	if err != nil {
		t.Fatalf("ResolveBinary: %v", err)
	}
	if got != bin {
		t.Fatalf("ResolveBinary = %q, want %q", got, bin)
	}

	t.Setenv("MI_LSP_BIN", filepath.Join(t.TempDir(), "mi-lsp.cmd"))
	if _, err := mcp.ResolveBinary(); err == nil {
		t.Fatal("expected a .cmd host to be refused")
	}

	t.Setenv("MI_LSP_BIN", "")
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable: %v", err)
	}
	got, err = mcp.ResolveBinary()
	if err != nil {
		t.Fatalf("ResolveBinary default: %v", err)
	}
	if got != exe {
		t.Fatalf("ResolveBinary = %q, want executable %q", got, exe)
	}
}

func TestMCPToolSchemas(t *testing.T) {
	listed, err := mcp.ToolList()
	if err != nil {
		t.Fatalf("ToolList: %v", err)
	}
	want := []string{
		"nav_intent", "nav_route", "nav_pack", "nav_wiki", "nav_search", "nav_find",
		"nav_refs", "nav_related", "nav_flow_slice", "nav_change_pack", "nav_affected",
		"nav_multi_read", "nav_overview",
	}
	if len(listed) != len(want) {
		t.Fatalf("tools = %d, want %d", len(listed), len(want))
	}
	prefer := regexp.MustCompile(`(?i)Prefer this|Use when|DEFAULT first move`)
	required := map[string][]string{
		"nav_intent":      {"question"},
		"nav_route":       {"task"},
		"nav_pack":        {"task"},
		"nav_wiki":        {"op"},
		"nav_search":      {"pattern"},
		"nav_find":        {"pattern"},
		"nav_refs":        {"symbol"},
		"nav_related":     {"symbol"},
		"nav_multi_read":  {"ranges"},
		"nav_flow_slice":  nil,
		"nav_change_pack": nil,
		"nav_affected":    nil,
		"nav_overview":    nil,
	}
	for i, info := range listed {
		if info.Name != want[i] {
			t.Fatalf("tool[%d] = %s, want %s", i, info.Name, want[i])
		}
		if !prefer.MatchString(info.Description) {
			t.Fatalf("%s description does not say when to prefer it", info.Name)
		}
		var schema struct {
			Type       string                    `json:"type"`
			Properties map[string]map[string]any `json:"properties"`
			Required   []string                  `json:"required"`
		}
		if err := json.Unmarshal(info.InputSchema, &schema); err != nil {
			t.Fatalf("%s schema: %v", info.Name, err)
		}
		if schema.Type != "object" {
			t.Fatalf("%s schema type = %s", info.Name, schema.Type)
		}
		workspace, ok := schema.Properties["workspace"]
		if !ok || workspace["type"] != "string" {
			t.Fatalf("%s workspace schema = %#v", info.Name, workspace)
		}
		for _, name := range schema.Required {
			if name == "workspace" {
				t.Fatalf("%s marks workspace required", info.Name)
			}
		}
		if strings.Join(schema.Required, ",") != strings.Join(required[info.Name], ",") {
			t.Fatalf("%s required = %#v, want %#v", info.Name, schema.Required, required[info.Name])
		}
	}
}

func TestMCPStdioHandshake(t *testing.T) {
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		``,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"ping"}`,
	}, "\n") + "\n"
	command := newMCPCommand(nil)
	var out bytes.Buffer
	command.SetIn(strings.NewReader(input))
	command.SetOut(&out)
	command.SetErr(&bytes.Buffer{})
	command.SetContext(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- command.RunE(command, nil)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunE: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handshake blocked")
	}
	lines := splitLines(out.String())
	if len(lines) != 3 {
		t.Fatalf("responses = %d, want 3 (initialized must not be answered)\n%s", len(lines), out.String())
	}
	initMsg := decodeRPC(t, lines[0])
	if initMsg["id"] != float64(1) {
		t.Fatalf("initialize id = %#v", initMsg["id"])
	}
	initResult := objectField(t, initMsg, "result")
	if initResult["protocolVersion"] != "2024-11-05" {
		t.Fatalf("protocolVersion = %#v", initResult["protocolVersion"])
	}
	info := objectField(t, initResult, "serverInfo")
	if info["name"] != "mi-lsp" || info["version"] != "mi-lsp-v1.1" {
		t.Fatalf("serverInfo = %#v", info)
	}
	capabilities := objectField(t, initResult, "capabilities")
	if _, ok := capabilities["tools"].(map[string]any); !ok {
		t.Fatalf("capabilities = %#v", capabilities)
	}

	listMsg := decodeRPC(t, lines[1])
	if listMsg["id"] != float64(2) {
		t.Fatalf("tools/list id = %#v", listMsg["id"])
	}
	listResult := objectField(t, listMsg, "result")
	tools, ok := listResult["tools"].([]any)
	if !ok || len(tools) != 13 {
		t.Fatalf("tools = %#v", listResult["tools"])
	}
	if tools[0].(map[string]any)["name"] != "nav_intent" {
		t.Fatalf("first tool = %#v", tools[0])
	}
	if tools[len(tools)-1].(map[string]any)["name"] != "nav_overview" {
		t.Fatalf("last tool = %#v", tools[len(tools)-1])
	}

	pingMsg := decodeRPC(t, lines[2])
	if pingMsg["id"] != float64(3) {
		t.Fatalf("ping id = %#v", pingMsg["id"])
	}
	if _, ok := pingMsg["error"]; ok {
		t.Fatalf("ping error = %#v", pingMsg["error"])
	}
	pingResult := objectField(t, pingMsg, "result")
	if len(pingResult) != 0 {
		t.Fatalf("ping result = %#v", pingResult)
	}
}

func TestMCPToolCallUsesRecorder(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "mi-lsp.exe")
	pattern := "needle; rm -rf / && echo pwned"
	var gotBin string
	var gotArgv []string
	var calls int
	var hadDeadline bool
	input := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"nav_search","arguments":{"pattern":"` + pattern + `","workspace":"ws","regex":false,"contextLines":4}}}` + "\n"
	out := serveMCP(t, input, mcp.ServeConfig{
		Bin: bin,
		Exec: func(ctx context.Context, command string, argv []string) mcp.ExecResult {
			calls++
			gotBin = command
			gotArgv = append([]string{}, argv...)
			_, hadDeadline = ctx.Deadline()
			return mcp.ExecResult{Stdout: "toon-body\n", Stderr: "warning", Code: 0}
		},
	})
	if calls != 1 {
		t.Fatalf("exec calls = %d", calls)
	}
	if hadDeadline {
		t.Fatal("tool call added its own deadline; the child owns nav timeouts")
	}
	if gotBin != bin {
		t.Fatalf("bin = %q, want %q", gotBin, bin)
	}
	want := []string{"nav", "search", pattern, "--format", "toon", "--client-name", "mi-lsp-mcp", "--context-lines", "4", "--workspace", "ws"}
	if strings.Join(gotArgv, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("argv = %#v\nwant %#v", gotArgv, want)
	}
	msg := decodeRPC(t, splitLines(out)[0])
	result := objectField(t, msg, "result")
	if result["isError"] != false {
		t.Fatalf("isError = %#v", result["isError"])
	}
	content := result["content"].([]any)[0].(map[string]any)
	if content["text"] != "toon-body\n" {
		t.Fatalf("text = %#v", content["text"])
	}
}

func TestMCPToolCallDoesNotExecUnknownToolOrShell(t *testing.T) {
	calls := 0
	exec := func(ctx context.Context, bin string, argv []string) mcp.ExecResult {
		calls++
		return mcp.ExecResult{Code: 0, Stdout: "nope"}
	}
	unknown := serveMCP(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nav_missing","arguments":{}}}`+"\n", mcp.ServeConfig{Exec: exec})
	unknownResult := objectField(t, decodeRPC(t, splitLines(unknown)[0]), "result")
	if unknownResult["isError"] != true || !strings.Contains(unknownResult["content"].([]any)[0].(map[string]any)["text"].(string), "Unknown tool:") {
		t.Fatalf("unknown tool result = %#v", unknownResult)
	}

	shell := serveMCP(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"nav_search","arguments":{"pattern":"x"}}}`+"\n", mcp.ServeConfig{
		Bin:  `C:\Windows\System32\cmd.exe`,
		Exec: exec,
	})
	shellResult := objectField(t, decodeRPC(t, splitLines(shell)[0]), "result")
	text := shellResult["content"].([]any)[0].(map[string]any)["text"].(string)
	if shellResult["isError"] != true || !strings.Contains(text, "unavailable_binary") {
		t.Fatalf("shell result = %#v", shellResult)
	}
	if calls != 0 {
		t.Fatalf("exec calls = %d, want 0", calls)
	}

	badOp := serveMCP(t, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"nav_wiki","arguments":{"op":"nope"}}}`+"\n", mcp.ServeConfig{Exec: exec})
	badResult := objectField(t, decodeRPC(t, splitLines(badOp)[0]), "result")
	badText := badResult["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(badText, "unsupported_operation") {
		t.Fatalf("bad op text = %s", badText)
	}
}

func TestMCPToolCallClassifiesChildFailure(t *testing.T) {
	out := serveMCP(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nav_route","arguments":{"task":"compaction","workspace":"missing"}}}`+"\n", mcp.ServeConfig{
		Bin: filepath.Join(t.TempDir(), "mi-lsp.exe"),
		Exec: func(ctx context.Context, bin string, argv []string) mcp.ExecResult {
			return mcp.ExecResult{Code: 1, Stderr: "workspace not found: missing"}
		},
	})
	result := objectField(t, decodeRPC(t, splitLines(out)[0]), "result")
	if result["isError"] != true {
		t.Fatalf("result = %#v", result)
	}
	var payload struct {
		ReasonCode string `json:"reasonCode"`
		Detail     string `json:"detail"`
	}
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("detail json: %v (%s)", err, text)
	}
	if payload.ReasonCode != "invalid_workspace" {
		t.Fatalf("reasonCode = %s", payload.ReasonCode)
	}
}

func TestMCPUnknownMethod(t *testing.T) {
	out := serveMCP(t, "not-json\n"+`{"jsonrpc":"2.0","id":9,"method":"resources/list"}`+"\n", mcp.ServeConfig{})
	lines := splitLines(out)
	if len(lines) != 2 {
		t.Fatalf("lines = %#v", lines)
	}
	parseErr := decodeRPC(t, lines[0])
	parseBody := objectField(t, parseErr, "error")
	if parseBody["code"] != float64(-32700) {
		t.Fatalf("parse error = %#v", parseBody)
	}
	missing := decodeRPC(t, lines[1])
	body := objectField(t, missing, "error")
	if body["code"] != float64(-32601) {
		t.Fatalf("method error = %#v", body)
	}
}

func TestMCPBuildArgvRejectsStringPaths(t *testing.T) {
	_, err := mcp.BuildArgv("nav_affected", map[string]any{"paths": "a.go"})
	if err == nil {
		t.Fatal("expected paths string to be rejected")
	}
	if errors.Is(err, mcp.ErrUnknownTool) {
		t.Fatal("paths type error must not look like an unknown tool")
	}
}

func serveMCP(t *testing.T, input string, cfg mcp.ServeConfig) string {
	t.Helper()
	var out bytes.Buffer
	cfg.In = strings.NewReader(input)
	cfg.Out = &out
	done := make(chan error, 1)
	go func() {
		done <- mcp.Serve(context.Background(), cfg)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve: %v\n%s", err, out.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve blocked")
	}
	return out.String()
}

func splitLines(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func decodeRPC(t *testing.T, line string) map[string]any {
	t.Helper()
	var msg map[string]any
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		t.Fatalf("json: %v (%s)", err, line)
	}
	if msg["jsonrpc"] != "2.0" {
		t.Fatalf("jsonrpc = %#v", msg["jsonrpc"])
	}
	return msg
}

func objectField(t *testing.T, msg map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := msg[key].(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v", key, msg[key])
	}
	return value
}

func requireFlag(t *testing.T, argv []string, flag, value string) {
	t.Helper()
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag && argv[i+1] == value {
			return
		}
	}
	t.Fatalf("missing %s %s in %#v", flag, value, argv)
}

func assertNoConsoleShell(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	base := strings.ToLower(filepath.Base(cmd.Path))
	switch base {
	case "cmd.exe", "sh", "bash", "powershell.exe", "pwsh.exe":
		t.Fatalf("command path is a shell: %s", cmd.Path)
	}
	if cmd.SysProcAttr == nil {
		if runtime.GOOS == "windows" {
			t.Fatal("expected SysProcAttr on windows")
		}
		return
	}
	value := reflect.ValueOf(cmd.SysProcAttr)
	if value.Kind() != reflect.Ptr || value.IsNil() {
		t.Fatalf("SysProcAttr = %#v", cmd.SysProcAttr)
	}
	fields := value.Elem()
	if hide := fields.FieldByName("HideWindow"); hide.IsValid() && !hide.Bool() {
		t.Fatal("HideWindow is false")
	}
	if flags := fields.FieldByName("CreationFlags"); flags.IsValid() && flags.Uint()&0x08000000 == 0 {
		t.Fatalf("CREATE_NO_WINDOW not set: %#x", flags.Uint())
	}
}
