package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestWriteProcessFailureKeepsHumanLineAndTrailer(t *testing.T) {
	prev := os.Args
	os.Args = []string{"mi-lsp", "zzz-no-existe"}
	t.Cleanup(func() { os.Args = prev })

	var buf bytes.Buffer
	WriteProcessFailure(&buf, errors.New(`unknown command "zzz-no-existe" for "mi-lsp"`))
	text := buf.String()
	if !strings.Contains(text, `unknown command "zzz-no-existe"`) {
		t.Fatalf("human line missing: %s", text)
	}
	if !strings.Contains(text, "reason_code=unsupported_operation") {
		t.Fatalf("trailer missing: %s", text)
	}
}

func TestWriteProcessFailureJSONEnvelope(t *testing.T) {
	prev := os.Args
	os.Args = []string{"mi-lsp", "--format", "json", "zzz-no-existe"}
	t.Cleanup(func() { os.Args = prev })

	var buf bytes.Buffer
	WriteProcessFailure(&buf, errors.New(`unknown command "zzz-no-existe" for "mi-lsp"`))
	var env struct {
		Ok    bool `json:"ok"`
		Error struct {
			ReasonCode string `json:"reason_code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("json: %v body=%s", err, buf.String())
	}
	if env.Ok {
		t.Fatal("ok true")
	}
	if env.Error.ReasonCode != "unsupported_operation" {
		t.Fatalf("reason_code=%q", env.Error.ReasonCode)
	}
}
