package cli

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const maxFallbackDetailChars = 300

const (
	reasonUnsupportedOperation = "unsupported_operation"
	reasonUnavailableBinary    = "unavailable_binary"
	reasonInvalidWorkspace     = "invalid_workspace"
	reasonExplicitIncomplete   = "explicit_incomplete"
)

var (
	fallbackAssignedSecret = regexp.MustCompile(`(?i)\b(api[_-]?key|token|password|secret|authorization)\b\s*[:=]\s*\S+`)
	fallbackPromptOrArgv   = regexp.MustCompile(`(?i)\b(prompt|argv)\b\s*[:=].*`)
	fallbackBearer         = regexp.MustCompile(`(?i)\bbearer\s+\S+`)
	fallbackSK             = regexp.MustCompile(`\bsk-[A-Za-z0-9]{8,}`)
	fallbackUserInfo       = regexp.MustCompile(`(?i)(://[^/\s:]+:)[^@\s]+@`)
)

// ensureFallbackReason stamps error.reason_code and error.detail on failure
// envelopes. Existing code, message, kind, and stage are left unchanged.
func ensureFallbackReason(env *model.Envelope) {
	if env == nil || env.Ok || env.Error == nil {
		return
	}
	if !isFallbackReasonCode(env.Error.ReasonCode) {
		env.Error.ReasonCode = classifyFallbackReason(env.Error.Code, env.Error.Kind, env.Error.Message)
	}
	if strings.TrimSpace(env.Error.Detail) == "" {
		env.Error.Detail = fallbackDetail(env.Error.Message, env.Error.Code, env.Error.ReasonCode)
		return
	}
	env.Error.Detail = sanitizeFallbackDetail(env.Error.Detail)
	if env.Error.Detail == "" {
		env.Error.Detail = canonicalFallbackDetail(env.Error.ReasonCode)
	}
}

// WriteProcessFailure prints a CLI failure that never became an envelope.
// The human error line stays first; a single trailer follows for plugins.
func WriteProcessFailure(w io.Writer, err error) {
	if err == nil || w == nil {
		return
	}
	fmt.Fprintln(w, err)
	code := classifyFallbackReason("", "", err.Error())
	detail := fallbackDetail(err.Error(), "", code)
	fmt.Fprintf(w, "reason_code=%s detail=%s\n", code, detail)
}

func isFallbackReasonCode(code string) bool {
	switch code {
	case reasonUnsupportedOperation, reasonUnavailableBinary, reasonInvalidWorkspace, reasonExplicitIncomplete:
		return true
	default:
		return false
	}
}

func classifyFallbackReason(code, kind, message string) string {
	blob := strings.ToLower(code + "\n" + kind + "\n" + message)
	switch {
	case isUnsupportedOperation(blob):
		return reasonUnsupportedOperation
	case isUnavailableBinary(blob):
		return reasonUnavailableBinary
	case isInvalidWorkspace(blob):
		return reasonInvalidWorkspace
	default:
		return reasonExplicitIncomplete
	}
}

func isUnsupportedOperation(blob string) bool {
	return containsAny(blob,
		"unknown command",
		"unknown flag",
		"unknown shorthand flag",
		"unknown operation",
		"unrecognized command",
		"unrecognized flag",
		"unrecognized argument",
		"invalid argument",
		"invalid flag",
		"invalid --",
	)
}

func isUnavailableBinary(blob string) bool {
	if containsAny(blob,
		"executable file not found",
		"executable not found",
		"dotnet_sdk_missing",
		"no installed .net sdks",
		"no installed dotnet sdks",
		"compatible .net sdk was not found",
		"not found in $path",
		"not found in %path%",
		"not found in path",
	) {
		return true
	}
	if strings.Contains(blob, "binary") && containsAny(blob, "not found", "missing", "unavailable", "enoent", "no such file") {
		return true
	}
	if strings.Contains(blob, "backend executable") && containsAny(blob, "not found", "missing", "unavailable", "enoent") {
		return true
	}
	if strings.Contains(blob, "fork/exec") && containsAny(blob, "not found", "no such file", "enoent") {
		return true
	}
	if strings.Contains(blob, "cannot find the file") && containsAny(blob, "createprocess", ".exe", "executable", "binary", "dotnet") {
		return true
	}
	return false
}

func isInvalidWorkspace(blob string) bool {
	if containsAny(blob,
		"workspace_resolution_failed",
		"workspace_unresolved",
		"no project.toml",
		"not a mi-lsp workspace",
		"is not registered",
	) {
		return true
	}
	return strings.Contains(blob, "workspace") && containsAny(blob,
		"not found",
		"unresolved",
		"invalid",
		"not registered",
		"does not exist",
		"no such",
	)
}

func containsAny(blob string, parts ...string) bool {
	for _, part := range parts {
		if strings.Contains(blob, part) {
			return true
		}
	}
	return false
}

func fallbackDetail(message, code, reason string) string {
	if detail := sanitizeFallbackDetail(message); detail != "" {
		return detail
	}
	if detail := sanitizeFallbackDetail(code); detail != "" {
		return detail
	}
	return canonicalFallbackDetail(reason)
}

func canonicalFallbackDetail(reason string) string {
	switch reason {
	case reasonUnsupportedOperation:
		return "the requested operation is not supported"
	case reasonUnavailableBinary:
		return "the required backend binary is unavailable"
	case reasonInvalidWorkspace:
		return "the requested workspace is invalid"
	default:
		return "the result is explicitly incomplete"
	}
}

func sanitizeFallbackDetail(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	text = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, text)
	text = strings.Join(strings.Fields(text), " ")
	text = fallbackPromptOrArgv.ReplaceAllString(text, "$1=[redacted]")
	text = fallbackBearer.ReplaceAllString(text, "[redacted]")
	text = fallbackAssignedSecret.ReplaceAllString(text, "$1=[redacted]")
	text = fallbackSK.ReplaceAllString(text, "[redacted]")
	text = fallbackUserInfo.ReplaceAllString(text, "$1[redacted]@")
	runes := []rune(text)
	if len(runes) > maxFallbackDetailChars {
		text = strings.TrimSpace(string(runes[:maxFallbackDetailChars]))
	}
	return text
}
