package model

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// WikiContractLanguage is the language selector declared by WIKI-CONTRACT-v1 governance.
type WikiContractLanguage string

const (
	WikiContractLanguageES WikiContractLanguage = "es"
	WikiContractLanguageEN WikiContractLanguage = "en"
)

// WikiContractLanguageResult keeps an absent declaration distinct from an invalid one.
type WikiContractLanguageResult struct {
	Language WikiContractLanguage  `json:"language,omitempty"`
	Source   string                `json:"source,omitempty"`
	Declared bool                  `json:"declared"`
	Valid    bool                  `json:"valid"`
	Warnings []WikiContractFinding `json:"warnings,omitempty"`
}

// WikiContractFinding is an advisory, typed finding; language/diagram findings never block.
type WikiContractFinding struct {
	Code       string `json:"code"`
	DocID      string `json:"doc_id,omitempty"`
	Kind       string `json:"kind,omitempty"`
	SectionKey string `json:"section_key,omitempty"`
	Expected   string `json:"expected,omitempty"`
	Source     string `json:"source,omitempty"`
	Message    string `json:"message"`
	Blocking   bool   `json:"blocking"`
}

// WikiContractSection gives a stable semantic key a localized section title.
type WikiContractSection struct {
	Key   string `json:"key"`
	Title string `json:"title"`
}

// WikiContractCriterion is one numbered RF acceptance criterion.
type WikiContractCriterion struct {
	Number int    `json:"number"`
	Text   string `json:"text"`
}

// WikiContractCaseCriterion is a TP case's reference to one RF criterion.
type WikiContractCaseCriterion struct {
	Number      int    `json:"number"`
	Line        string `json:"line"`
	Disposition string `json:"disposition,omitempty"`
	Evidence    string `json:"evidence,omitempty"`
}

type wikiContractSectionPair struct {
	key string
	es  string
	en  string
}

// v1 section keys are semantic identifiers; titles are projections selected by governance.
// Source: home/ideas/wiki-modelo-v1/wiki-contract-v1.md and persona records
// wiki-idioma-en-governance-20261019 / wiki-diagramas-c4-secuencia-y-derivados-20261019.
var wikiContractLocalizedSections = map[string][]wikiContractSectionPair{
	"scope": {
		{"objective", "Objetivo", "Objective"},
		{"value_proposition", "Propuesta de valor", "Value proposition"},
		{"capabilities", "Capacidades", "Capabilities"},
		{"actors", "Actores", "Actors"},
		{"functional_areas", "Áreas funcionales", "Functional areas"},
		{"out_of_scope", "Fuera de alcance", "Out of scope"},
	},
	"outcome": {
		{"observable_outcome", "Resultado observable", "Observable outcome"},
		{"beneficiary", "Beneficiario", "Beneficiary"},
		{"success_criteria", "Criterios de éxito", "Success criteria"},
		{"out_of_scope", "Fuera de alcance del RS", "Out of scope for RS"},
	},
	"architecture": {
		{"context", "Contexto", "Context"},
		{"containers_c4", "Contenedores (C4)", "Containers (C4)"},
		{"key_decisions", "Decisiones clave", "Key decisions"},
	},
	"decision": {
		{"context", "Contexto", "Context"},
		{"decision", "Decisión", "Decision"},
		{"consequences", "Consecuencias", "Consequences"},
		{"rejected_alternatives", "Alternativas descartadas", "Rejected alternatives"},
	},
	"flow": {
		{"objective", "Objetivo", "Goal"},
		{"scope", "Alcance", "Scope"},
		{"actors", "Actores", "Actors"},
		{"pre_postconditions", "Pre/postcondiciones", "Pre/postconditions"},
		{"sequence", "Secuencia", "Sequence"},
		{"adr_constraints", "Restricciones ADR", "ADR constraints"},
	},
	"requirement": {
		{"purpose", "Propósito", "Purpose"},
		{"ordered_behavior", "Comportamiento ordenado", "Ordered behavior"},
		{"surfaces", "Superficies", "Surfaces"},
		{"typed_inputs_outputs", "Entradas/salidas tipadas", "Typed inputs/outputs"},
		{"errors", "Errores", "Errors"},
		{"acceptance_criteria", "Criterios de aceptación (numerados)", "Acceptance criteria (numbered)"},
	},
	"test": {
		{"scope", "Alcance", "Scope"},
		{"cases", "Casos (cada caso cita el número del criterio RF, con disposición y evidencia esperada)", "Cases"},
	},
	"technical": {
		{"summary", "Resumen", "Summary"},
		{"component_map", "Mapa de componentes", "Component map"},
	},
	"data_model": {
		{"entities", "Entidades", "Entities"},
		{"invariants", "Invariantes", "Invariants"},
		{"entity_owners", "Dueño de cada entidad", "Owner of each entity"},
	},
	"database": {
		{"store", "Almacén", "Store"},
		{"schema_or_convention", "Esquema o convención", "Schema or convention"},
		{"migration_risks", "Riesgos de migración", "Migration risks"},
	},
	"contract": {
		{"consumer", "Consumidor", "Consumer"},
		{"operations", "Operaciones", "Operations"},
		{"errors", "Errores", "Errors"},
		{"compatibility", "Compatibilidad", "Compatibility"},
	},
}

var (
	wikiGovernanceFencePattern = regexp.MustCompile("(?is)```(?:yaml|yml)\\s*\\r?\\n(.*?)\\r?\\n```")
	wikiH2Pattern              = regexp.MustCompile(`(?m)^##\s+(.+?)\s*#*\s*$`)
	wikiNumberedCriterion      = regexp.MustCompile(`^\s*(\d+)\.\s+(.+?)\s*$`)
	wikiCaseCriterionES        = regexp.MustCompile(`(?i)\bcriterio\s+(\d+)\b`)
	wikiCaseCriterionEN        = regexp.MustCompile(`(?i)\bcriterion\s+(\d+)\b`)
	wikiMermaidFencePattern    = regexp.MustCompile("(?is)```mermaid\\b(.*?)```")
)

// ResolveWikiContractLanguage parses one selected governance document without performing I/O.
// Missing language defaults to Spanish with a warning; malformed/unsupported values stay invalid.
func ResolveWikiContractLanguage(source, content string) WikiContractLanguageResult {
	result := WikiContractLanguageResult{Source: strings.TrimSpace(source)}
	block, hasBlock, fenced := wikiGovernanceYAML(content)
	if strings.TrimSpace(content) == "" || !hasBlock {
		result.Language = WikiContractLanguageES
		result.Valid = true
		result.Warnings = append(result.Warnings, wikiLanguageFinding("language_missing_default_es", result.Source, "governance language is absent; defaulting to es"))
		return result
	}

	var raw map[string]any
	if err := yaml.Unmarshal([]byte(block), &raw); err != nil {
		// Plain Markdown without a language declaration is the same as an absent declaration;
		// an explicit YAML block/frontmatter that cannot be parsed is instead invalid.
		if !fenced && !wikiHasLanguageKey(block) {
			result.Language = WikiContractLanguageES
			result.Valid = true
			result.Warnings = append(result.Warnings, wikiLanguageFinding("language_missing_default_es", result.Source, "governance language is absent; defaulting to es"))
			return result
		}
		result.Warnings = append(result.Warnings, wikiLanguageFinding("governance_yaml_invalid", result.Source, fmt.Sprintf("governance YAML is invalid: %v", err)))
		return result
	}
	value, declared := raw["language"]
	result.Declared = declared
	if !declared {
		result.Language = WikiContractLanguageES
		result.Valid = true
		result.Warnings = append(result.Warnings, wikiLanguageFinding("language_missing_default_es", result.Source, "governance language is absent; defaulting to es"))
		return result
	}
	text, ok := value.(string)
	if !ok {
		result.Warnings = append(result.Warnings, wikiLanguageFinding("language_invalid", result.Source, "governance language must be es or en"))
		return result
	}
	switch WikiContractLanguage(strings.ToLower(strings.TrimSpace(text))) {
	case WikiContractLanguageES:
		result.Language = WikiContractLanguageES
	case WikiContractLanguageEN:
		result.Language = WikiContractLanguageEN
	default:
		result.Warnings = append(result.Warnings, wikiLanguageFinding("language_invalid", result.Source, "governance language must be es or en"))
		return result
	}
	result.Valid = true
	return result
}

// WikiContractRequiredSections returns localized headings paired with stable semantic keys.
func WikiContractRequiredSections(kind string, language WikiContractLanguage) []WikiContractSection {
	if language != WikiContractLanguageES && language != WikiContractLanguageEN {
		return nil
	}
	pairs := wikiContractLocalizedSections[strings.ToLower(strings.TrimSpace(kind))]
	out := make([]WikiContractSection, 0, len(pairs))
	for _, pair := range pairs {
		title := pair.es
		if language == WikiContractLanguageEN {
			title = pair.en
		}
		out = append(out, WikiContractSection{Key: pair.key, Title: title})
	}
	return out
}

// ValidateWikiContractSections reports missing localized H2s and unnumbered RF criteria.
func ValidateWikiContractSections(docID, kind, body string, language WikiContractLanguage) []WikiContractFinding {
	sections := WikiContractRequiredSections(kind, language)
	if len(sections) == 0 {
		return nil
	}
	present := wikiContractHeadingSet(body)
	findings := make([]WikiContractFinding, 0)
	for _, section := range sections {
		if wikiContractSectionPresent(present, section) {
			continue
		}
		findings = append(findings, WikiContractFinding{
			Code: "section_missing", DocID: strings.TrimSpace(docID), Kind: strings.ToLower(strings.TrimSpace(kind)),
			SectionKey: section.Key, Expected: section.Title,
			Message:  fmt.Sprintf("section_missing:%s:%s", strings.TrimSpace(docID), wikiContractDisplayTitle(section.Title)),
			Blocking: false,
		})
	}
	if strings.EqualFold(strings.TrimSpace(kind), "requirement") && wikiContractSectionPresent(present, sectionByKey(sections, "acceptance_criteria")) {
		if len(ParseWikiContractAcceptanceCriteria(body, language)) == 0 {
			section := sectionByKey(sections, "acceptance_criteria")
			findings = append(findings, WikiContractFinding{
				Code: "acceptance_criteria_unnumbered", DocID: strings.TrimSpace(docID), Kind: "requirement",
				SectionKey: "acceptance_criteria", Expected: section.Title,
				Message: strings.TrimSpace(docID) + ": acceptance_criteria_unnumbered", Blocking: false,
			})
		}
	}
	return findings
}

// ParseWikiContractAcceptanceCriteria reads numbered criteria under the localized RF section.
func ParseWikiContractAcceptanceCriteria(body string, language WikiContractLanguage) []WikiContractCriterion {
	sections := WikiContractRequiredSections("requirement", language)
	section := sectionByKey(sections, "acceptance_criteria")
	if section.Key == "" {
		return nil
	}
	content, found := wikiContractSectionBody(body, section)
	if !found {
		return nil
	}
	byNumber := map[int]string{}
	for _, line := range strings.Split(content, "\n") {
		match := wikiNumberedCriterion.FindStringSubmatch(line)
		if len(match) != 3 {
			continue
		}
		number, err := strconv.Atoi(match[1])
		if err != nil || number <= 0 {
			continue
		}
		if _, exists := byNumber[number]; !exists {
			byNumber[number] = strings.TrimSpace(match[2])
		}
	}
	numbers := make([]int, 0, len(byNumber))
	for number := range byNumber {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	criteria := make([]WikiContractCriterion, 0, len(numbers))
	for _, number := range numbers {
		criteria = append(criteria, WikiContractCriterion{Number: number, Text: byNumber[number]})
	}
	return criteria
}

// ParseWikiContractTestCases parses only references in the localized TP cases section.
func ParseWikiContractTestCases(body string, language WikiContractLanguage) ([]WikiContractCaseCriterion, []WikiContractFinding) {
	sections := WikiContractRequiredSections("test", language)
	section := sectionByKey(sections, "cases")
	if section.Key == "" {
		return nil, nil
	}
	content, found := wikiContractSectionBody(body, section)
	if !found {
		return nil, []WikiContractFinding{{Code: "section_missing", Kind: "test", SectionKey: "cases", Expected: section.Title, Message: "section_missing:cases:" + section.Title, Blocking: false}}
	}
	pattern := wikiCaseCriterionES
	if language == WikiContractLanguageEN {
		pattern = wikiCaseCriterionEN
	}
	cases := make([]WikiContractCaseCriterion, 0)
	findings := make([]WikiContractFinding, 0)
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r", ""), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		item := strings.TrimSpace(strings.TrimLeft(trimmed, "-*# "))
		match := pattern.FindStringSubmatch(item)
		isListItem := strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "*") || wikiNumberedCriterion.MatchString(trimmed)
		if len(match) < 2 {
			if isListItem {
				findings = append(findings, WikiContractFinding{Code: "case_without_criterion", Kind: "test", SectionKey: "cases", Message: "case_without_criterion:" + item, Blocking: false})
			}
			continue
		}
		number, err := strconv.Atoi(match[1])
		if err != nil || number <= 0 {
			findings = append(findings, WikiContractFinding{Code: "invalid_criterion", Kind: "test", SectionKey: "cases", Message: "invalid_criterion:" + item, Blocking: false})
			continue
		}
		disposition, evidence := wikiContractDispositionEvidence(item, language)
		cases = append(cases, WikiContractCaseCriterion{Number: number, Line: item, Disposition: disposition, Evidence: evidence})
	}
	if len(cases) == 0 {
		findings = append(findings, WikiContractFinding{Code: "no_parseable_cases", Kind: "test", SectionKey: "cases", Message: "no_parseable_cases", Blocking: false})
	}
	return cases, findings
}

// WikiContractDiagramFindings checks required diagram presence only; it is not a Mermaid linter.
func WikiContractDiagramFindings(path, kind, body string, language WikiContractLanguage) []WikiContractFinding {
	if language != WikiContractLanguageES && language != WikiContractLanguageEN {
		return nil
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	findings := make([]WikiContractFinding, 0)
	switch {
	case kind == "architecture" && strings.EqualFold(filepath.Base(filepath.ToSlash(path)), "03_architecture.md"):
		sections := WikiContractRequiredSections("architecture", language)
		for _, key := range []string{"context", "containers_c4"} {
			section := sectionByKey(sections, key)
			content, _ := wikiContractSectionBody(body, section)
			marker := "C4Context"
			if key == "containers_c4" {
				marker = "C4Container"
			}
			if !wikiMermaidContains(content, marker) {
				findings = append(findings, WikiContractFinding{
					Code: "diagram_missing", Kind: kind, SectionKey: key, Expected: marker,
					Message: "diagram_missing:architecture:" + key, Blocking: false,
				})
			}
		}
	case kind == "flow":
		sections := WikiContractRequiredSections("flow", language)
		section := sectionByKey(sections, "sequence")
		content, _ := wikiContractSectionBody(body, section)
		if !wikiMermaidContains(content, "sequenceDiagram") {
			findings = append(findings, WikiContractFinding{
				Code: "diagram_missing", Kind: kind, SectionKey: "sequence", Expected: "sequenceDiagram",
				Message: "diagram_missing:flow:sequence", Blocking: false,
			})
		}
	}
	return findings
}

func wikiLanguageFinding(code, source, message string) WikiContractFinding {
	return WikiContractFinding{Code: code, Source: source, Message: message, Blocking: false}
}

func wikiGovernanceYAML(content string) (block string, found, fenced bool) {
	content = strings.TrimPrefix(strings.ReplaceAll(content, "\r", ""), "\ufeff")
	if match := wikiGovernanceFencePattern.FindStringSubmatch(content); len(match) > 1 {
		return strings.TrimSpace(match[1]), true, true
	}
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "---\n") {
		rest := trimmed[4:]
		if end := strings.Index(rest, "\n---"); end >= 0 {
			return strings.TrimSpace(rest[:end]), true, true
		}
	}
	if trimmed == "" {
		return "", false, false
	}
	return trimmed, true, false
}

func wikiHasLanguageKey(content string) bool {
	return regexp.MustCompile(`(?m)^\s*language\s*:`).MatchString(content)
}

func wikiContractHeadingSet(body string) map[string]struct{} {
	body = wikiStripFrontMatter(body)
	present := make(map[string]struct{})
	for _, match := range wikiH2Pattern.FindAllStringSubmatch(body, -1) {
		if len(match) > 1 {
			present[normalizeWikiContractHeading(match[1])] = struct{}{}
		}
	}
	return present
}

func wikiContractSectionPresent(present map[string]struct{}, section WikiContractSection) bool {
	if section.Key == "" {
		return false
	}
	title := normalizeWikiContractHeading(section.Title)
	if _, ok := present[title]; ok {
		return true
	}
	// The v1 acceptance/cases titles carry explanatory parentheticals; legacy content commonly
	// uses the stable short heading, which is the same semantic section.
	if i := strings.Index(title, " ("); i > 0 {
		_, ok := present[strings.TrimSpace(title[:i])]
		return ok
	}
	return false
}

func wikiContractSectionBody(body string, section WikiContractSection) (string, bool) {
	if section.Key == "" {
		return "", false
	}
	body = wikiStripFrontMatter(body)
	lines := strings.Split(body, "\n")
	start := -1
	for i, line := range lines {
		match := wikiH2Pattern.FindStringSubmatch(line)
		if len(match) > 1 && wikiContractSectionPresent(map[string]struct{}{normalizeWikiContractHeading(match[1]): {}}, section) {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return "", false
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "## ") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n"), true
}

func wikiContractDisplayTitle(title string) string {
	if i := strings.Index(title, " ("); i > 0 {
		return strings.TrimSpace(title[:i])
	}
	return title
}

func sectionByKey(sections []WikiContractSection, key string) WikiContractSection {
	for _, section := range sections {
		if section.Key == key {
			return section
		}
	}
	return WikiContractSection{}
}

func normalizeWikiContractHeading(value string) string {
	value = strings.ToLower(strings.TrimSpace(strings.TrimRight(value, "# ")))
	value = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n").Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func wikiStripFrontMatter(body string) string {
	body = strings.TrimPrefix(strings.ReplaceAll(body, "\r", ""), "\ufeff")
	trimmed := strings.TrimLeft(body, "\n")
	if !strings.HasPrefix(trimmed, "---\n") {
		return body
	}
	if end := strings.Index(trimmed[4:], "\n---"); end >= 0 {
		return trimmed[4+end+4:]
	}
	return body
}

func wikiMermaidContains(sectionBody, marker string) bool {
	for _, match := range wikiMermaidFencePattern.FindAllStringSubmatch(sectionBody, -1) {
		if len(match) > 1 && strings.Contains(strings.ToLower(match[1]), strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func wikiContractDispositionEvidence(line string, language WikiContractLanguage) (string, string) {
	lower := strings.ToLower(line)
	dispositionKey, evidenceKey := "disposición:", "evidencia:"
	if language == WikiContractLanguageEN {
		dispositionKey, evidenceKey = "disposition:", "evidence:"
	}
	index := strings.Index(lower, dispositionKey)
	if index < 0 && language == WikiContractLanguageES {
		dispositionKey = "disposicion:"
		index = strings.Index(lower, dispositionKey)
	}
	if index < 0 {
		return "", ""
	}
	segment := strings.TrimSpace(line[index+len(dispositionKey):])
	if evidenceIndex := strings.Index(strings.ToLower(segment), evidenceKey); evidenceIndex >= 0 {
		return strings.Trim(strings.TrimSpace(segment[:evidenceIndex]), ";"), strings.Trim(strings.TrimSpace(segment[evidenceIndex+len(evidenceKey):]), ";")
	}
	return strings.Trim(segment, ";"), ""
}
