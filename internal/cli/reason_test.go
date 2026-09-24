package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestEnsureFallbackReasonMapsTerminalFailures(t *testing.T) {
	cases := []struct {
		name    string
		code    string
		kind    string
		message string
		want    string
	}{
		{name: "unknown command", message: `unknown command "frob" for "mi-lsp"`, want: "unsupported_operation"},
		{name: "unknown flag", message: "unknown flag: --bogus", want: "unsupported_operation"},
		{name: "unknown operation", code: "nav_generic", kind: "validation", message: "unknown operation nav.prepare", want: "unsupported_operation"},
		{name: "invalid flag value", message: `invalid --format "xml"; valid options: compact, json, text, toon, yaml`, want: "unsupported_operation"},
		{name: "missing executable", message: `exec: "dotnet": executable file not found in %PATH%`, want: "unavailable_binary"},
		{name: "sdk code", code: "dotnet_sdk_missing", message: "A compatible .NET SDK was not found", want: "unavailable_binary"},
		{name: "worker binary missing", message: `worker binary bundled (C:\repos\mios\mi-lsp.exe) not found`, want: "unavailable_binary"},
		{name: "workspace not found", code: "workspace_resolution_failed", kind: "workspace", message: "workspace not found in registry and path does not exist", want: "invalid_workspace"},
		{name: "workspace unresolved", message: "workspace unresolved for selector", want: "invalid_workspace"},
		{name: "invalid workspace", message: `invalid workspace C:\repos\mios`, want: "invalid_workspace"},
		{name: "timeout", message: "context deadline exceeded", want: "explicit_incomplete"},
		{name: "truncation failure", code: "char_budget", message: "response exceeded 80 chars", want: "explicit_incomplete"},
		{name: "other failure", code: "operation_failed", kind: "backend_runtime", message: "backend returned an error", want: "explicit_incomplete"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := model.Envelope{
				Ok: false,
				Error: &model.EnvelopeError{
					Kind:    tc.kind,
					Code:    tc.code,
					Message: tc.message,
					Stage:   "backend",
				},
			}
			ensureFallbackReason(&env)
			err := env.Error
			if err.ReasonCode != tc.want {
				t.Fatalf("reason_code = %q, want %q", err.ReasonCode, tc.want)
			}
			if err.Code != tc.code || err.Kind != tc.kind || err.Message != tc.message || err.Stage != "backend" {
				t.Fatalf("classified error changed identity: %+v", *err)
			}
			if err.Detail == "" || strings.Contains(err.Detail, "\n") || len([]rune(err.Detail)) > 300 {
				t.Fatalf("detail = %q, want one non-empty line of at most 300 chars", err.Detail)
			}
			ensureFallbackReason(&env)
			if env.Error.ReasonCode != tc.want || env.Error.Detail != err.Detail {
				t.Fatalf("second pass changed reason=%q detail=%q", env.Error.ReasonCode, env.Error.Detail)
			}
		})
	}
}

func TestEnsureFallbackReasonKeepsAllowedCodeAndSanitizesDetail(t *testing.T) {
	env := model.Envelope{
		Ok: false,
		Error: &model.EnvelopeError{
			Kind:       "workspace",
			Code:       "workspace_resolution_failed",
			Message:    "unknown flag: --workspace",
			Stage:      "selector_validation",
			ReasonCode: "invalid_workspace",
			Detail:     "kept token=super-secret\nprompt=do not echo argv=mi-lsp nav search --query secret",
		},
	}
	ensureFallbackReason(&env)
	err := env.Error
	if err.ReasonCode != "invalid_workspace" {
		t.Fatalf("reason_code = %q, want the existing allowlisted code", err.ReasonCode)
	}
	if err.Message != "unknown flag: --workspace" || err.Code != "workspace_resolution_failed" || err.Kind != "workspace" || err.Stage != "selector_validation" {
		t.Fatalf("identity fields changed: %+v", *err)
	}
	if strings.Contains(err.Detail, "super-secret") || strings.Contains(err.Detail, "do not echo") || strings.Contains(err.Detail, "nav search") || strings.Contains(err.Detail, "\n") {
		t.Fatalf("detail echoed secret, prompt, or argv: %q", err.Detail)
	}
	if !strings.Contains(err.Detail, "kept") || !strings.Contains(err.Detail, "token=[redacted]") || !strings.Contains(err.Detail, "prompt=[redacted]") {
		t.Fatalf("detail = %q, want redacted kept text", err.Detail)
	}
	again := err.Detail
	ensureFallbackReason(&env)
	if env.Error.ReasonCode != "invalid_workspace" || env.Error.Detail != again {
		t.Fatalf("sanitize was not idempotent: %q", env.Error.Detail)
	}
}

func TestEnsureFallbackReasonReplacesUnknownCodeAndBoundsDetail(t *testing.T) {
	long := "operation failed " + strings.Repeat("x", 500)
	env := model.Envelope{
		Ok: false,
		Error: &model.EnvelopeError{
			Code:       "operation_failed",
			Message:    long + "\nsecret=abc prompt=hidden argv=--token sek",
			ReasonCode: "python_unavailable",
		},
	}
	ensureFallbackReason(&env)
	if env.Error.ReasonCode != "explicit_incomplete" {
		t.Fatalf("reason_code = %q, want explicit_incomplete", env.Error.ReasonCode)
	}
	detail := env.Error.Detail
	if strings.Contains(detail, "\n") || len([]rune(detail)) > 300 {
		t.Fatalf("detail len=%d text=%q", len([]rune(detail)), detail)
	}
	if strings.Contains(detail, "secret=abc") || strings.Contains(detail, "hidden") || strings.Contains(detail, "--token") {
		t.Fatalf("detail echoed secret, prompt, or argv: %q", detail)
	}
}

func TestEnsureFallbackReasonIgnoresSuccessAndPreservesWindowsPath(t *testing.T) {
	ensureFallbackReason(nil)
	okEnv := model.Envelope{Ok: true, Error: &model.EnvelopeError{Message: "unknown flag: --bogus"}}
	ensureFallbackReason(&okEnv)
	if okEnv.Error.ReasonCode != "" || okEnv.Error.Detail != "" {
		t.Fatalf("success envelope was stamped: %+v", *okEnv.Error)
	}
	noErr := model.Envelope{Ok: false}
	ensureFallbackReason(&noErr)
	if noErr.Error != nil {
		t.Fatal("nil error was allocated")
	}

	const winPath = `C:\repos\mios\file.go`
	env := model.Envelope{Error: &model.EnvelopeError{Message: "invalid workspace " + winPath}}
	ensureFallbackReason(&env)
	if env.Error.ReasonCode != "invalid_workspace" {
		t.Fatalf("reason_code = %q", env.Error.ReasonCode)
	}
	if !strings.Contains(env.Error.Detail, winPath) || strings.Contains(env.Error.Detail, "C:\r") {
		t.Fatalf("detail interpreted windows path: %q", env.Error.Detail)
	}
}

func TestWriteProcessFailureKeepsHumanLineAndTrailer(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		reason string
	}{
		{name: "unknown flag", err: errors.New("unknown flag: --bogus"), reason: "unsupported_operation"},
		{name: "missing binary", err: errors.New(`exec: "mi-lsp": executable file not found in %PATH%`), reason: "unavailable_binary"},
		{name: "workspace", err: errors.New("workspace not found in registry and path does not exist"), reason: "invalid_workspace"},
		{name: "timeout", err: errors.New("context deadline exceeded"), reason: "explicit_incomplete"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			WriteProcessFailure(&buf, tc.err)
			got := buf.String()
			if !strings.HasPrefix(got, tc.err.Error()+"\n") {
				t.Fatalf("human line changed: %q", got)
			}
			rest := strings.TrimPrefix(got, tc.err.Error()+"\n")
			prefix := "reason_code=" + tc.reason + " detail="
			if !strings.HasPrefix(rest, prefix) || !strings.HasSuffix(rest, "\n") {
				t.Fatalf("trailer = %q", rest)
			}
			detail := strings.TrimSuffix(strings.TrimPrefix(rest, prefix), "\n")
			if detail == "" || strings.Contains(detail, "\n") || len([]rune(detail)) > 300 {
				t.Fatalf("detail = %q", detail)
			}
			if strings.Count(got, "\n") != strings.Count(tc.err.Error(), "\n")+2 {
				t.Fatalf("unexpected extra lines: %q", got)
			}
		})
	}

	var buf bytes.Buffer
	WriteProcessFailure(nil, errors.New("unknown flag: --bogus"))
	WriteProcessFailure(&buf, nil)
	if buf.Len() != 0 {
		t.Fatalf("nil error wrote %q", buf.String())
	}
}

func TestWriteProcessFailureDetailDoesNotEchoSecrets(t *testing.T) {
	err := errors.New("unknown flag: --bogus token=super-secret prompt=do not echo this argv=mi-lsp nav search --query secret " + `C:\repos\mios\file.go`)
	var buf bytes.Buffer
	WriteProcessFailure(&buf, err)
	got := buf.String()
	if !strings.HasPrefix(got, err.Error()+"\n") {
		t.Fatalf("human line changed: %q", got)
	}
	trailer := strings.TrimPrefix(got, err.Error()+"\n")
	if strings.Contains(trailer, "super-secret") || strings.Contains(trailer, "do not echo") || strings.Contains(trailer, "nav search") {
		t.Fatalf("trailer echoed secret, prompt, or argv: %q", trailer)
	}
	if strings.Contains(trailer, "\r") {
		t.Fatalf("trailer interpreted backslash-r: %q", trailer)
	}
}
