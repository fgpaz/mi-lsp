package telemetry

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const maxHarnessLen = 64

// HarnessFromEnv returns the sanitized MI_LSP_HARNESS value. Plugins set it
// with the agent role (for example the subagent agent_type); it never carries
// content.
func HarnessFromEnv() string {
	return SanitizeHarness(os.Getenv("MI_LSP_HARNESS"))
}

// SanitizeHarness keeps a short identifier-like label: letters, digits and
// the separators `_ . : / -`. Anything else is dropped so free text, paths
// with spaces or secrets cannot reach telemetry through this field.
func SanitizeHarness(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '_', r == '.', r == ':', r == '/', r == '-':
			b.WriteRune(r)
		}
		if b.Len() >= maxHarnessLen {
			break
		}
	}
	return b.String()
}

// EnvelopeBytes estimates the response size as the JSON encoding of the
// envelope. It measures size only and never retains content.
func EnvelopeBytes(envelope model.Envelope) int {
	body, err := json.Marshal(envelope)
	if err != nil {
		return 0
	}
	return len(body)
}
