package cli

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
)

var (
	askDocIDPattern     = regexp.MustCompile(`(?i)\b(?:rf|fl|ct|tech|db|tp)-[a-z0-9-]+\b`)
	askPathPattern      = regexp.MustCompile(`(?i)(?:[a-z]:\\|[/\\])|\b[a-z0-9_-]+\.[a-z0-9]{1,5}\b`)
	askCommandPattern   = regexp.MustCompile(`(?i)\b(?:mi-lsp|nav\s+(?:search|context|related|intent|refs|workspace-map|service|find|trace|multi-read|batch|symbols|outline|overview))\b`)
	askNamespacePattern = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\.[A-Za-z_][A-Za-z0-9_.]*\b`)
	askCamelCasePattern = regexp.MustCompile(`\b[A-Z][a-z0-9]+[A-Z][A-Za-z0-9]*\b`)
)

var askOrientationQuestions = map[string]struct{}{
	"how is this workspace organized":  {},
	"how is this repo organized":       {},
	"how is this repository organized": {},
	"where do i start":                 {},
	"where should i start":             {},
	"what should i read first":         {},
	"what do i read first":             {},
	"what are the main services":       {},
	"what are the main repos":          {},
	"what are the main repositories":   {},
	"where is the documentation":       {},
	"where are the docs":               {},
	"how is this documented":           {},
	"como se organiza este workspace":  {},
	"como se organiza este repo":       {},
	"como se organiza este repositorio": {},
	"por donde empiezo":                 {},
	"por donde deberia empezar":         {},
	"que deberia leer primero":          {},
	"que leo primero":                   {},
	"cuales son los servicios principales":   {},
	"cuales son los repos principales":       {},
	"cuales son los repositorios principales": {},
	"donde esta la documentacion":            {},
	"donde estan los docs":                   {},
	"como esta documentado esto":             {},
}

type axiDecision struct {
	Supported bool
	Enabled   bool
}

func (s *rootState) effectiveAXI(cmd *cobra.Command, operation string, payload map[string]any) bool {
	return s.resolveAXIDecision(cmd, operation, payload).Enabled
}

func (s *rootState) resolveAXIDecision(cmd *cobra.Command, operation string, payload map[string]any) axiDecision {
	if !supportsAXISurface(operation) {
		return axiDecision{}
	}
	if isClassicRequested(cmd, s.classic) {
		return axiDecision{Supported: true, Enabled: false}
	}

	defaultEnabled := defaultAXIForOperation(operation, payload)
	if flagChanged(cmd, "axi") {
		if s.axi {
			return axiDecision{Supported: true, Enabled: true}
		}
		return axiDecision{Supported: true, Enabled: defaultEnabled}
	}
	if s.axi {
		return axiDecision{Supported: true, Enabled: true}
	}
	return axiDecision{Supported: true, Enabled: defaultEnabled}
}

func supportsAXISurface(operation string) bool {
	switch operation {
	case "root.home", "workspace.init", "workspace.status", "nav.ask", "nav.pack", "nav.route", "nav.workspace-map", "nav.search", "nav.intent":
		return true
	default:
		return false
	}
}

func defaultAXIForOperation(operation string, payload map[string]any) bool {
	switch operation {
	case "root.home", "workspace.init", "workspace.status", "nav.search", "nav.intent", "nav.pack", "nav.route":
		return true
	case "nav.ask":
		return shouldDefaultAXIAsk(questionFromPayload(payload))
	default:
		return false
	}
}

func shouldDefaultAXIAsk(question string) bool {
	normalized := normalizeAXIQuestion(question)
	if normalized == "" {
		return false
	}
	if hasAskAXIBlockers(question, normalized) {
		return false
	}
	_, ok := askOrientationQuestions[normalized]
	return ok
}

func hasAskAXIBlockers(raw string, normalized string) bool {
	if askDocIDPattern.MatchString(raw) || askPathPattern.MatchString(raw) || askCommandPattern.MatchString(raw) || askNamespacePattern.MatchString(raw) || askCamelCasePattern.MatchString(raw) {
		return true
	}
	blockedTerms := []string{
		"implemented",
		"implementation",
		"implementado",
		"implementacion",
		"code",
		"codigo",
		"symbol",
		"simbolo",
		"class",
		"method",
		"function",
		"func",
		"file",
		"files",
		"path",
		"paths",
	}
	for _, term := range blockedTerms {
		if strings.Contains(normalized, term) {
			return true
		}
	}
	return false
}

func normalizeAXIQuestion(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"á", "a",
		"é", "e",
		"í", "i",
		"ó", "o",
		"ú", "u",
		"ü", "u",
	)
	value = replacer.Replace(value)
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
	return strings.Join(fields, " ")
}

func questionFromPayload(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	question, _ := payload["question"].(string)
	return strings.TrimSpace(question)
}

func flagChanged(cmd *cobra.Command, name string) bool {
	return cmd != nil && cmd.Flags().Lookup(name) != nil && cmd.Flags().Changed(name)
}

func isClassicRequested(cmd *cobra.Command, classic bool) bool {
	return classic && flagChanged(cmd, "classic")
}
