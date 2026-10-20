package output

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func renderDiagnostics(env model.Envelope) string {
	summary := env.Diagnostics
	lines := []string{fmt.Sprintf("diagnostics: %d error(s), %d warning(s); %d/%d files checked", summary.Errors, summary.Warnings, summary.FilesChecked, summary.FilesRequested)}
	if env.Partial || !env.Ok {
		lines[0] += "; partial/unavailable (not clean)"
	}
	// Daemon JSON decoding turns typed items into generic objects.
	raw, _ := json.Marshal(env.Items)
	var items []model.Diagnostic
	_ = json.Unmarshal(raw, &items)
	for _, d := range items {
		code := ""
		if d.Code != nil {
			code = fmt.Sprintf(" [%v]", d.Code)
		}
		path := d.Path
		if strings.ContainsAny(path, "\r\n\t") {
			path = strconv.Quote(path)
		}
		lines = append(lines, fmt.Sprintf("%s:%d:%d %s%s %s", path, d.Line, d.Column, d.Severity, code, strings.Join(strings.Fields(d.Message), " ")))
	}
	for _, o := range env.Omissions {
		if strings.ContainsAny(o.Path, "\r\n\t") {
			o.Path = strconv.Quote(o.Path)
		}
		lines = append(lines, renderEnvelopeOmission(o))
	}
	for _, w := range env.Warnings {
		lines = append(lines, "warning: "+w)
	}
	if env.Truncated {
		lines = append(lines, "truncated=true; counts describe observed snapshot before output limits")
	}
	if env.NextHint != nil {
		lines = append(lines, "next: "+*env.NextHint)
	}
	return strings.Join(lines, "\n")
}
