package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestMCPServerListsNativeToolsOverNDJSON(t *testing.T) {
	input := strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}\n")
	var output bytes.Buffer
	if err := serveMCP(context.Background(), input, &output, &rootState{}, nil); err != nil {
		t.Fatalf("serveMCP() error = %v", err)
	}
	var response struct {
		Result struct {
			Tools []mcpTool `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &response); err != nil {
		t.Fatalf("decode tools/list response: %v", err)
	}
	if got := len(response.Result.Tools); got != 13 {
		t.Fatalf("tools/list returned %d tools, want 13", got)
	}
	if response.Result.Tools[3].Name != "nav_wiki" || response.Result.Tools[12].Name != "nav_overview" {
		t.Fatalf("unexpected tool ordering: first wiki=%q, last overview=%q", response.Result.Tools[3].Name, response.Result.Tools[12].Name)
	}
}

func TestMCPToolCallMappingPreservesOperationArguments(t *testing.T) {
	operation, payload, ok := mcpOperation("nav_search", map[string]any{
		"pattern": "needle", "includeContent": true, "contextLines": float64(4), "workspace": "repo-alias",
	})
	if !ok || operation != "nav.search" {
		t.Fatalf("mcpOperation() = (%q, %t), want nav.search", operation, ok)
	}
	if payload["pattern"] != "needle" || payload["include_content"] != true || payload["context_lines"] != float64(4) || payload["workspace"] != "repo-alias" {
		t.Fatalf("mapped payload = %#v", payload)
	}
	if _, exists := payload["includeContent"]; exists {
		t.Fatal("camelCase includeContent key was not normalized")
	}
}

func TestMCPInitializeNegotiatesOnlySupportedVersion(t *testing.T) {
	state := &rootState{}
	matching := handleMCPMessage(context.Background(), mcpMessage{
		JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize",
		Params: json.RawMessage(`{"protocolVersion":"2024-11-05"}`),
	}, state, nil)
	matchingResult := matching.Result.(map[string]any)
	if got := matchingResult["protocolVersion"]; got != mcpProtocolVersion {
		t.Fatalf("negotiated version = %v, want %s", got, mcpProtocolVersion)
	}
	unsupported := handleMCPMessage(context.Background(), mcpMessage{
		JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "initialize",
		Params: json.RawMessage(`{"protocolVersion":"2099-01-01"}`),
	}, state, nil)
	unsupportedResult := unsupported.Result.(map[string]any)
	if got := unsupportedResult["protocolVersion"]; got != mcpProtocolVersion {
		t.Fatalf("fallback version = %v, want supported %s", got, mcpProtocolVersion)
	}
}

func TestMCPRejectsInvalidJSONRPCVersion(t *testing.T) {
	response := handleMCPMessage(context.Background(), mcpMessage{
		JSONRPC: "1.0", ID: json.RawMessage("3"), Method: "tools/list",
	}, &rootState{}, nil)
	if response.Error == nil || response.Error.Code != -32600 {
		t.Fatalf("invalid JSON-RPC version error = %#v, want -32600", response.Error)
	}
}
