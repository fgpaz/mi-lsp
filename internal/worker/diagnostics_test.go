package worker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

type diagnosticTestWriter struct{ onWrite func([]byte) }

func (w diagnosticTestWriter) Write(p []byte) (int, error) {
	if w.onWrite != nil {
		w.onWrite(p)
	}
	return len(p), nil
}
func (diagnosticTestWriter) Close() error { return nil }

func diagnosticTestFile(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	file := filepath.Join(root, "bad.go")
	if e := os.WriteFile(file, []byte("package bad\nvar x int = \"wrong\"\n"), 0600); e != nil {
		t.Fatal(e)
	}
	return root, file
}
func TestDiagnosticPullReportsAreFreshAndTyped(t *testing.T) {
	for _, tc := range []struct {
		name, report string
		wantErr      bool
		wantItems    int
	}{
		{"error", `{"kind":"full","items":[{"range":{"start":{"line":1,"character":4}},"severity":1,"message":"cannot use string as int","source":"gopls","code":"typecheck"}]}`, false, 1},
		{"clean", `{"kind":"full","items":[]}`, false, 0},
		{"null", `null`, true, 0},
		{"unchanged_without_previous_report", `{"kind":"unchanged","resultId":"old"}`, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, file := diagnosticTestFile(t)
			transport := &lspTransport{in: diagnosticTestWriter{}, out: io.NopCloser(bytes.NewReader(nil)), done: make(chan struct{}), responses: make(chan lspResponse, 1), diagnostics: map[string]publishedDiagnostics{}, diagnosticWake: make(chan struct{}, 1)}
			transport.responses <- lspResponse{ID: 1, Result: json.RawMessage(tc.report)}
			c := NewLSPClient(LSPConfig{Backend: "gopls"}, model.WorkspaceRegistration{Root: root})
			c.transport = transport
			c.capabilities = map[string]json.RawMessage{"diagnosticProvider": json.RawMessage(`{}`)}
			response, e := c.diagnostics(context.Background(), model.WorkerRequest{Payload: map[string]any{"file": file}})
			if (e != nil) != tc.wantErr || len(response.Items) != tc.wantItems {
				t.Fatalf("response=%+v err=%v", response, e)
			}
			if len(response.Items) > 0 && (response.Items[0]["line"] != 2 || response.Items[0]["column"] != 5 || response.Items[0]["severity"] != "error") {
				t.Fatalf("%+v", response)
			}
		})
	}
}
func TestDiagnosticPushMailboxOnlyAcceptsRequestedURI(t *testing.T) {
	transport := &lspTransport{diagnostics: map[string]publishedDiagnostics{"file:///wanted.go": {}}, diagnosticWake: make(chan struct{}, 1)}
	transport.acceptDiagnostics(json.RawMessage(`{"params":{"uri":"file:///elsewhere.go","version":1,"diagnostics":[]}}`))
	if len(transport.diagnostics) != 1 {
		t.Fatal("unrelated publication retained")
	}
	transport.acceptDiagnostics(json.RawMessage(`{"params":{"uri":"file:///wanted.go","version":2,"diagnostics":[]}}`))
	if !transport.diagnostics["file:///wanted.go"].received {
		t.Fatal("empty publication was confused with no report")
	}
}
func TestDiagnosticPushNeverClaimsCompletion(t *testing.T) {
	root, file := diagnosticTestFile(t)
	transport := &lspTransport{done: make(chan struct{}), responses: make(chan lspResponse), diagnostics: map[string]publishedDiagnostics{}, diagnosticWake: make(chan struct{}, 1)}
	transport.in = diagnosticTestWriter{onWrite: func(p []byte) {
		var message lspNotification
		if json.Unmarshal(p, &message) == nil && message.Method == "textDocument/didOpen" {
			raw, _ := json.Marshal(map[string]any{"params": map[string]any{"uri": buildFileURI(file), "version": 1, "diagnostics": []any{}}})
			transport.acceptDiagnostics(raw)
		}
	}}
	c := NewLSPClient(LSPConfig{Backend: "pyright"}, model.WorkspaceRegistration{Root: root})
	c.transport = transport
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	response, e := c.diagnostics(ctx, model.WorkerRequest{Payload: map[string]any{"file": file}})
	if e != nil || !response.Ok || !response.Partial || response.Reason != "push_snapshot_no_completion_barrier" {
		t.Fatalf("%+v %v", response, e)
	}
}
func TestTSserverSynchronousDiagnosticsUseLinePositions(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "bad.ts")
	if e := os.WriteFile(file, []byte("const x: number = 'bad';"), 0600); e != nil {
		t.Fatal(e)
	}
	responses := bytes.Join([][]byte{
		lspTestFrame(t, map[string]any{"type": "response", "request_seq": 3, "success": true, "body": []any{}}),
		lspTestFrame(t, map[string]any{"type": "response", "request_seq": 4, "success": true, "body": []any{map[string]any{"startLocation": map[string]int{"line": 1, "offset": 7}, "category": "error", "message": "Type string is not assignable to number", "code": 2322}}}),
		lspTestFrame(t, map[string]any{"type": "response", "request_seq": 5, "success": true, "body": []any{}}),
	}, nil)
	c := &TsserverClient{workspace: model.WorkspaceRegistration{Root: root}, stdin: &testWriteCloser{}, reader: bufio.NewReader(bytes.NewReader(responses))}
	response, e := c.diagnostics(context.Background(), model.WorkerRequest{Payload: map[string]any{"file": file}})
	if e != nil || len(response.Items) != 1 || response.Items[0]["column"] != 7 {
		t.Fatalf("%+v %v", response, e)
	}
}
func TestDiagnosticWorkerRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	target := filepath.Join(other, "secret.go")
	if e := os.WriteFile(target, []byte("not read"), 0600); e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(root, "link.go")
	if e := os.Symlink(target, link); e != nil {
		t.Skip("symlink unavailable")
	}
	if _, e := diagnosticFile(root, map[string]any{"file": link}); e == nil {
		t.Fatal("symlink escaped")
	}
}
