package worker

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestWriteTSServerRequestUsesJSONLines(t *testing.T) {
	request := map[string]any{
		"seq":       1,
		"type":      "request",
		"command":   "references",
		"arguments": map[string]any{"file": "/workspace/app.ts", "line": 3, "offset": 22},
	}
	want, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')

	var output bytes.Buffer
	if err := writeTSServerRequest(&output, request); err != nil {
		t.Fatalf("write request: %v", err)
	}
	if !bytes.Equal(output.Bytes(), want) {
		t.Fatalf("request bytes = %q, want JSON line %q", output.Bytes(), want)
	}
	if bytes.Contains(output.Bytes(), []byte("Content-Length:")) {
		t.Fatalf("tsserver request unexpectedly used Content-Length framing: %q", output.Bytes())
	}
}

func TestWriteTSServerRequestRejectsShortWrite(t *testing.T) {
	err := writeTSServerRequest(shortTSWriter{}, map[string]any{"seq": 1, "type": "request"})
	if err != io.ErrShortWrite {
		t.Fatalf("short write error = %v, want %v", err, io.ErrShortWrite)
	}
}

type shortTSWriter struct{}

func (shortTSWriter) Write(data []byte) (int, error) {
	return len(data) - 1, nil
}

func TestReadTSFrameParsesContentLengthFramedOutput(t *testing.T) {
	body := []byte(`{"refs":[{"file":"/workspace/app.ts","start":{"line":3,"offset":22},"end":{"line":3,"offset":42}}]}`)
	response := []byte(fmt.Sprintf(`{"seq":2,"type":"response","command":"references","request_seq":1,"success":true,"body":%s}`, body))
	frame := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(response), response)

	message, err := readTSFrame(bufio.NewReader(strings.NewReader(frame)))
	if err != nil {
		t.Fatalf("read tsserver output: %v", err)
	}
	if message.Type != "response" || message.RequestSeq != 1 || !message.Success {
		t.Fatalf("message = %#v, want successful response for request 1", message)
	}
	var decoded struct {
		Refs []struct {
			File string `json:"file"`
		} `json:"refs"`
	}
	if err := json.Unmarshal(message.Body, &decoded); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if len(decoded.Refs) != 1 || decoded.Refs[0].File != "/workspace/app.ts" {
		t.Fatalf("references = %#v, want one app.ts reference", decoded.Refs)
	}
}

func TestReadTSFrameParsesCaseInsensitiveContentLength(t *testing.T) {
	body := []byte(`{"type":"response","request_seq":1,"success":true}`)
	frame := fmt.Sprintf("content-length: %d\r\n\r\n%s", len(body), body)

	message, err := readTSFrame(bufio.NewReader(strings.NewReader(frame)))
	if err != nil {
		t.Fatalf("read tsserver output: %v", err)
	}
	if message.Type != "response" || message.RequestSeq != 1 || !message.Success {
		t.Fatalf("message = %#v, want successful response for request 1", message)
	}
}

func TestReadTSFrameRejectsMissingContentLength(t *testing.T) {
	_, err := readTSFrame(bufio.NewReader(strings.NewReader("\r\n")))
	if err == nil || err.Error() != "tsserver returned invalid content length" {
		t.Fatalf("missing Content-Length error = %v", err)
	}
}
