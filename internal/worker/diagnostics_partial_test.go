package worker

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestTSserverRetainsErrorsWhenSuggestionStageUnsupported(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "bad.ts")
	if e := os.WriteFile(file, []byte("const x: number = 'bad';"), 0600); e != nil {
		t.Fatal(e)
	}
	frames := bytes.Join([][]byte{
		lspTestFrame(t, map[string]any{"type": "response", "request_seq": 3, "success": true, "body": []any{}}),
		lspTestFrame(t, map[string]any{"type": "response", "request_seq": 4, "success": true, "body": []any{map[string]any{"startLocation": map[string]int{"line": 1, "offset": 7}, "category": "error", "message": "Type string is not assignable to number", "code": 2322}}}),
		lspTestFrame(t, map[string]any{"type": "response", "request_seq": 5, "success": false, "message": "Unrecognized JSON command"}),
	}, nil)
	c := &TsserverClient{workspace: model.WorkspaceRegistration{Root: root}, stdin: &testWriteCloser{}, reader: bufio.NewReader(bytes.NewReader(frames))}
	response, e := c.diagnostics(context.Background(), model.WorkerRequest{Payload: map[string]any{"file": file}})
	if e != nil || !response.Ok || !response.Partial || len(response.Items) != 1 || response.Items[0]["severity"] != "error" {
		t.Fatalf("response=%+v err=%v", response, e)
	}
}
