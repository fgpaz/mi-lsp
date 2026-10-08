package worker

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
)

type testWriteCloser struct {
	bytes.Buffer
}

func (testWriteCloser) Close() error { return nil }

func TestLSPClientRetriesContentModified(t *testing.T) {
	responses := bytes.Join([][]byte{
		lspTestFrame(t, map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"error":   map[string]any{"code": -32801, "message": "content modified"},
		}),
		lspTestFrame(t, map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"result":  []any{map[string]any{"uri": "file:///src/main.rs"}},
		}),
	}, nil)
	stdin := &testWriteCloser{}
	client := &LSPClient{
		stdin:  stdin,
		stdout: bufio.NewReader(bytes.NewReader(responses)),
	}

	result, err := client.sendRequest("textDocument/references", map[string]any{"position": map[string]int{"line": 1, "character": 2}})
	if err != nil {
		t.Fatalf("sendRequest() error = %v", err)
	}
	if !bytes.Contains(result, []byte("file:///src/main.rs")) {
		t.Fatalf("sendRequest() result = %s, want semantic location", result)
	}
	if client.seqID != 2 {
		t.Fatalf("request id after retry = %d, want 2", client.seqID)
	}

	reader := bufio.NewReader(bytes.NewReader(stdin.Bytes()))
	for wantID := 1; wantID <= 2; wantID++ {
		frame, err := readLSPMessage(reader)
		if err != nil {
			t.Fatalf("read request %d: %v", wantID, err)
		}
		var request lspRequest
		if err := json.Unmarshal(frame, &request); err != nil {
			t.Fatalf("decode request %d: %v", wantID, err)
		}
		if request.ID != wantID || request.Method != "textDocument/references" {
			t.Fatalf("request %d = %#v, want id %d and references method", wantID, request, wantID)
		}
	}
}

func TestLSPClientWarmsRustAnalyzerOnEmptySemanticResult(t *testing.T) {
	responses := bytes.Join([][]byte{
		lspTestFrame(t, map[string]any{"jsonrpc": "2.0", "id": 1, "result": []any{}}),
		lspTestFrame(t, map[string]any{"jsonrpc": "2.0", "id": 2, "result": []any{map[string]any{"uri": "file:///src/main.rs"}}}),
	}, nil)
	stdin := &testWriteCloser{}
	client := &LSPClient{
		config: LSPConfig{ServerCmd: "rust-analyzer"},
		stdin:  stdin,
		stdout: bufio.NewReader(bytes.NewReader(responses)),
	}

	result, err := client.sendSemanticRequest("textDocument/references", map[string]any{"position": map[string]int{"line": 1, "character": 2}})
	if err != nil {
		t.Fatalf("sendSemanticRequest() error = %v", err)
	}
	if !bytes.Contains(result, []byte("file:///src/main.rs")) {
		t.Fatalf("sendSemanticRequest() result = %s, want warmed semantic location", result)
	}
	if !client.semanticWarmupDone || client.seqID != 2 {
		t.Fatalf("warmup state done=%t request id=%d, want done=true and id=2", client.semanticWarmupDone, client.seqID)
	}
}

func lspTestFrame(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal LSP frame: %v", err)
	}
	return []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body))
}
