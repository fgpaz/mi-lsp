package wikisource

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
	"gopkg.in/yaml.v3"
)

const WikiContractV1 = "WIKI-CONTRACT-v1"

// ContractCodeLink is links.code[] in WIKI-CONTRACT-v1.
type ContractCodeLink struct {
	Path   string `yaml:"path"`
	Role   string `yaml:"role"`
	Symbol string `yaml:"symbol"`
}

// ContractLinks holds typed links declared on the source document only.
type ContractLinks struct {
	Fulfills     []string           `yaml:"fulfills"`
	Implements   []string           `yaml:"implements"`
	Verifies     []string           `yaml:"verifies"`
	Affects      []string           `yaml:"affects"`
	Supports     []string           `yaml:"supports"`
	Materializes []string           `yaml:"materializes"`
	Supersedes   []string           `yaml:"supersedes"`
	Code         []ContractCodeLink `yaml:"code"`
	Experiences  []string           `yaml:"experiences"`
	Stages       []string           `yaml:"stages"`
	Maps         []string           `yaml:"maps"`
	GroundsIn    []string           `yaml:"grounds_in"`
}

// ContractMetadata is parsed v1 header fields used by on-demand views.
type ContractMetadata struct {
	WikiContract string
	ID           string
	Kind         string
	Status       string
	Title        string
	Links        ContractLinks
	LegacyTOON   bool
}

// DerivedLink is an inverse edge computed in memory (never authored on the target).
type DerivedLink struct {
	InverseType string
	FromDocID   string
	ToDocID     string
	DeclaredOn  string
}

var contractLinkInverse = map[string]string{
	"fulfills":     "fulfilled_by",
	"implements":   "implemented_by",
	"verifies":     "verified_by",
	"affects":      "affected_by",
	"supports":     "supported_by",
	"materializes": "materialized_by",
	"supersedes":   "superseded_by",
	"experiences":  "experienced_by",
	"stages":       "staged_by",
	"maps":         "mapped_by",
	"grounds_in":   "grounded_by",
}

// ParseContractMetadata reads WIKI-CONTRACT-v1 front matter and legacy harness YAML blocks.
func ParseContractMetadata(content string) ContractMetadata {
	meta := ContractMetadata{}
	for _, block := range contractYAMLBlocks(content) {
		var header struct {
			WikiContract string        `yaml:"wiki_contract"`
			ID           string        `yaml:"id"`
			Kind         string        `yaml:"kind"`
			Status       string        `yaml:"status"`
			Title        string        `yaml:"title"`
			Links        ContractLinks `yaml:"links"`
			Imports      []string      `yaml:"imports"`
		}
		if err := yaml.Unmarshal([]byte(block), &header); err != nil {
			continue
		}
		if strings.TrimSpace(header.WikiContract) == WikiContractV1 {
			meta.WikiContract = WikiContractV1
			meta.ID = cleanScalar(header.ID)
			meta.Kind = cleanScalar(header.Kind)
			meta.Status = cleanScalar(header.Status)
			meta.Title = cleanScalar(header.Title)
			meta.Links = normalizeContractLinks(header.Links)
			return meta
		}
		// Partial v1-shaped block without wiki_contract key yet.
		if strings.TrimSpace(header.ID) != "" && strings.TrimSpace(header.Kind) != "" && header.Links.hasAny() {
			meta.ID = cleanScalar(header.ID)
			meta.Kind = cleanScalar(header.Kind)
			meta.Status = cleanScalar(header.Status)
			meta.Title = cleanScalar(header.Title)
			meta.Links = normalizeContractLinks(header.Links)
		}
	}
	if meta.Links.Verifies == nil && content != "" {
		meta.Links.Verifies = LegacyVerifiesFromImports(content)
	}
	meta.LegacyTOON = legacyTOONAdvisory(content)
	return meta
}

func (l ContractLinks) hasAny() bool {
	return len(l.Fulfills) > 0 || len(l.Implements) > 0 || len(l.Verifies) > 0 ||
		len(l.Affects) > 0 || len(l.Supports) > 0 || len(l.Materializes) > 0 ||
		len(l.Supersedes) > 0 || len(l.Code) > 0 || len(l.Experiences) > 0 ||
		len(l.Stages) > 0 || len(l.Maps) > 0 || len(l.GroundsIn) > 0
}

func normalizeContractLinks(links ContractLinks) ContractLinks {
	links.Fulfills = uniqueValues(links.Fulfills)
	links.Implements = uniqueValues(links.implementsAlias())
	links.Verifies = uniqueValues(links.Verifies)
	links.Affects = uniqueValues(links.Affects)
	links.Supports = uniqueValues(links.Supports)
	links.Materializes = uniqueValues(links.Materializes)
	links.Supersedes = uniqueValues(links.Supersedes)
	links.Experiences = uniqueValues(links.Experiences)
	links.Stages = uniqueValues(links.Stages)
	links.Maps = uniqueValues(links.Maps)
	links.GroundsIn = uniqueValues(links.GroundsIn)
	return links
}

func (l ContractLinks) implementsAlias() []string {
	if len(l.Implements) > 0 {
		return l.Implements
	}
	return nil
}

func contractYAMLBlocks(content string) []string {
	blocks := []string{}
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "---") {
		rest := trimmed[3:]
		if end := strings.Index(rest, "\n---"); end >= 0 {
			blocks = append(blocks, rest[:end])
		}
	}
	for _, match := range toonFencePattern.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			blocks = append(blocks, match[1])
		}
	}
	fenced := regexp.MustCompile("(?s)```(?:yaml|yml)\\s*(.*?)\\s*```")
	for _, match := range fenced.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			blocks = append(blocks, match[1])
		}
	}
	return blocks
}

// LegacyVerifiesFromImports infers TP->RF verifies targets from harness imports during transition.
func LegacyVerifiesFromImports(content string) []string {
	ids := make([]string, 0)
	for _, block := range contractYAMLBlocks(content) {
		for _, value := range keyValues(block, "imports") {
			for _, token := range extractDocIDTokens(value) {
				if strings.HasPrefix(strings.ToUpper(token), "RF-") {
					ids = append(ids, token)
				}
			}
		}
	}
	return uniqueValues(ids)
}

var docIDTokenPattern = regexp.MustCompile(`(?i)\b(RF|TP|FL|RS|ARCH|ADR|TECH|DB|CT|DM|NEG|UJ|UXM)-[A-Z0-9][A-Z0-9_-]*\b`)

func extractDocIDTokens(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'[]`)
	value = strings.ReplaceAll(value, "[[", "")
	value = strings.ReplaceAll(value, "]]", "")
	out := make([]string, 0)
	for _, match := range docIDTokenPattern.FindAllString(value, -1) {
		out = append(out, strings.ToUpper(match))
	}
	return out
}

func legacyTOONAdvisory(content string) bool {
	if strings.Contains(content, "wiki_contract: "+WikiContractV1) {
		return false
	}
	return toonFencePattern.MatchString(content)
}

// DeriveInverseEdges builds in-memory inverse navigation from source-only link declarations.
func DeriveInverseEdges(byDocID map[string]ContractLinks) []DerivedLink {
	derived := make([]DerivedLink, 0)
	for sourceID, links := range byDocID {
		sourceID = strings.TrimSpace(sourceID)
		if sourceID == "" {
			continue
		}
		appendDerived := func(linkType string, targets []string) {
			inverse := contractLinkInverse[linkType]
			if inverse == "" {
				return
			}
			for _, target := range targets {
				target = strings.TrimSpace(target)
				if target == "" {
					continue
				}
				derived = append(derived, DerivedLink{
					InverseType: inverse,
					FromDocID:   target,
					ToDocID:     sourceID,
					DeclaredOn:  sourceID,
				})
			}
		}
		appendDerived("fulfills", links.Fulfills)
		appendDerived("implements", links.Implements)
		appendDerived("verifies", links.Verifies)
		appendDerived("affects", links.Affects)
		appendDerived("supports", links.Supports)
		appendDerived("materializes", links.Materializes)
		appendDerived("supersedes", links.Supersedes)
		appendDerived("experiences", links.Experiences)
		appendDerived("stages", links.Stages)
		appendDerived("maps", links.Maps)
		appendDerived("grounds_in", links.GroundsIn)
	}
	sort.Slice(derived, func(i, j int) bool {
		a, b := derived[i], derived[j]
		if a.FromDocID != b.FromDocID {
			return a.FromDocID < b.FromDocID
		}
		if a.InverseType != b.InverseType {
			return a.InverseType < b.InverseType
		}
		return a.ToDocID < b.ToDocID
	})
	return derived
}

var (
	acceptanceHeadingPattern = regexp.MustCompile(`(?im)^##\s+criterios de aceptacion(?:\s+\(numerados\))?\s*$`)
	numberedCriterionPattern = regexp.MustCompile(`(?m)^\s*(\d+)\.\s+(.+)$`)
	testCaseCriterionPattern = regexp.MustCompile(`(?i)criterio\s+(\d+)`)
)

// ParseAcceptanceCriteria returns numbered acceptance criteria text from an RF body.
func ParseAcceptanceCriteria(content string) map[int]string {
	section := extractSectionAfterHeading(content, acceptanceHeadingPattern)
	if section == "" {
		return nil
	}
	criteria := map[int]string{}
	for _, match := range numberedCriterionPattern.FindAllStringSubmatch(section, -1) {
		if len(match) < 3 {
			continue
		}
		n := parseInt(match[1])
		if n <= 0 {
			continue
		}
		criteria[n] = strings.TrimSpace(match[2])
	}
	if len(criteria) == 0 {
		return nil
	}
	return criteria
}

// ParseTestPlanCases returns criterion numbers referenced in TP case lines (## Casos).
func ParseTestPlanCases(content string) []TestPlanCase {
	cases, _ := ParseTestPlanCasesWithWarnings(content)
	return cases
}

// ParseTestPlanCasesWithWarnings keeps unparseable case items distinct from coverage gaps.
func ParseTestPlanCasesWithWarnings(content string) ([]TestPlanCase, []string) {
	cases := make([]TestPlanCase, 0)
	var warnings []string
	lines := strings.Split(strings.ReplaceAll(content, "\r", ""), "\n")
	inCasos := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(trimmed, "## Casos") {
			inCasos = true
			continue
		}
		if inCasos && strings.HasPrefix(trimmed, "## ") {
			break
		}
		if !inCasos || trimmed == "" {
			continue
		}
		item := strings.TrimSpace(strings.TrimLeft(trimmed, "-*# "))
		match := testCaseCriterionPattern.FindStringSubmatch(item)
		if len(match) < 2 {
			if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "*") || numberedCriterionPattern.MatchString(trimmed) {
				warnings = append(warnings, "case_without_criterion:"+item)
			}
			continue
		}
		n := parseInt(match[1])
		if n <= 0 {
			warnings = append(warnings, "invalid_criterion:"+item)
			continue
		}
		disposition, evidence := splitDispositionEvidence(item)
		cases = append(cases, TestPlanCase{
			Criterion:   n,
			Line:        item,
			Disposition: disposition,
			Evidence:    evidence,
		})
	}
	if len(cases) == 0 {
		warnings = append(warnings, "no_parseable_cases")
	}
	return cases, warnings
}

type TestPlanCase struct {
	Criterion   int
	Line        string
	Disposition string
	Evidence    string
}

func splitDispositionEvidence(line string) (string, string) {
	lower := strings.ToLower(line)
	dispKey := "disposición:"
	idx := strings.Index(lower, dispKey)
	if idx < 0 {
		dispKey = "disposicion:"
		idx = strings.Index(lower, dispKey)
	}
	if idx < 0 {
		return "", ""
	}
	segment := strings.TrimSpace(line[idx+len(dispKey):])
	evidence := ""
	if evIdx := strings.Index(strings.ToLower(segment), "evidencia:"); evIdx >= 0 {
		disposition := strings.TrimSpace(segment[:evIdx])
		evidence = strings.TrimSpace(segment[evIdx+len("evidencia:"):])
		return strings.Trim(disposition, ";"), strings.Trim(evidence, ";")
	}
	return strings.Trim(segment, ";"), ""
}

func extractSectionAfterHeading(content string, heading *regexp.Regexp) string {
	start := 0
	found := false
	for _, line := range strings.SplitAfter(content, "\n") {
		start += len(line)
		if heading.MatchString(foldAccents(strings.TrimSpace(line))) {
			found = true
			break
		}
	}
	if !found {
		return ""
	}
	rest := content[start:]
	next := regexp.MustCompile(`(?m)^##\s+`).FindStringIndex(rest)
	if next != nil {
		rest = rest[:next[0]]
	}
	return rest
}

// ContractWikiIDIndex maps wiki artifact ids (v1 headers and legacy filename/H1 ids) to kind.
type ContractWikiIDIndex map[string]string

// ContractDocBody pairs a wiki-relative path with markdown content for index/validation.
type ContractDocBody struct {
	Path    string
	Content string
}

var contractRequiredH2 = map[string][]string{
	"scope":        {"Objetivo", "Propuesta de valor", "Capacidades", "Actores", "Áreas funcionales", "Fuera de alcance"},
	"outcome":      {"Resultado observable", "Beneficiario", "Criterios de éxito", "Fuera de alcance del RS"},
	"architecture": {"Contexto", "Contenedores (C4)", "Decisiones clave"},
	"decision":     {"Contexto", "Decisión", "Consecuencias", "Alternativas descartadas"},
	"flow":         {"Objetivo", "Alcance", "Actores", "Pre/postcondiciones", "Secuencia", "Restricciones ADR"},
	"requirement":  {"Propósito", "Comportamiento ordenado", "Superficies", "Entradas/salidas tipadas", "Errores", "Criterios de aceptación (numerados)"},
	"test":         {"Alcance", "Casos (cada caso cita el número del criterio RF, con disposición y evidencia esperada)"},
	"technical":    {"Resumen", "Mapa de componentes"},
	"data_model":   {"Entidades", "Invariantes", "Dueño de cada entidad"},
	"database":     {"Almacén", "Esquema o convención", "Riesgos de migración"},
	"contract":     {"Consumidor", "Operaciones", "Errores", "Compatibilidad"},
}

var contractLinkExpectedKinds = map[string][]string{
	"fulfills":     {"outcome"},
	"implements":   {"flow"},
	"verifies":     {"requirement"},
	"affects":      {"flow", "requirement"},
	"supports":     {"requirement"},
	"materializes": {"data_model"},
	"experiences":  {"outcome"},
	"stages":       {"flow"},
	"maps":         {"requirement"},
	"grounds_in":   {"business"},
}

var h2HeadingPattern = regexp.MustCompile(`(?m)^##\s+(.+?)\s*$`)

// BuildContractWikiIDIndex collects document ids from v1 headers, declared legacy ids, and filename stems.
func BuildContractWikiIDIndex(docs []ContractDocBody) ContractWikiIDIndex {
	index := ContractWikiIDIndex{}
	for _, doc := range docs {
		meta := ParseContractMetadata(doc.Content)
		kind := strings.ToLower(strings.TrimSpace(meta.Kind))
		if meta.WikiContract == WikiContractV1 && meta.ID != "" {
			registerContractWikiID(index, meta.ID, kind, doc.Path)
		}
		if declared := DeclaredDocID(doc.Content); declared != "" {
			if kind == "" {
				kind = InferContractKindFromDocID(declared)
			}
			registerContractWikiID(index, declared, kind, doc.Path)
		}
		base := strings.TrimSuffix(filepath.Base(filepath.ToSlash(doc.Path)), filepath.Ext(doc.Path))
		if ids := extractDocIDTokens(base); len(ids) == 1 && strings.EqualFold(ids[0], base) {
			k := InferContractKindFromDocID(ids[0])
			registerContractWikiID(index, strings.ToUpper(ids[0]), k, doc.Path)
		}
		normPath := strings.ToLower(filepath.ToSlash(doc.Path))
		if strings.HasSuffix(normPath, "data-model.md") || strings.HasSuffix(normPath, "06_modelo_datos.md") || strings.HasSuffix(normPath, "05_modelo_datos.md") {
			registerContractWikiID(index, "data-model", "data_model", doc.Path)
		}
	}
	return index
}

func registerContractWikiID(index ContractWikiIDIndex, id, kind, _ string) {
	RegisterContractWikiID(index, id, kind)
}

// RegisterContractWikiID adds or updates an id in the contract validation index.
func RegisterContractWikiID(index ContractWikiIDIndex, id, kind string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	key := strings.ToUpper(id)
	if existing, ok := index[key]; ok && existing != "" && kind == "" {
		return
	}
	if kind == "" {
		kind = InferContractKindFromDocID(id)
	}
	if kind == "" && strings.EqualFold(id, "data-model") {
		kind = "data_model"
	}
	index[key] = kind
}

// LookupKind returns the kind for a wiki id in the index (case-insensitive).
func (index ContractWikiIDIndex) LookupKind(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if kind, ok := index[strings.ToUpper(id)]; ok {
		return kind
	}
	if strings.EqualFold(id, "data-model") {
		if kind, ok := index["DATA-MODEL"]; ok {
			return kind
		}
	}
	return ""
}

// InferContractKindFromDocID maps id prefixes to WIKI-CONTRACT-v1 kinds (incl. NEG business projections).
func InferContractKindFromDocID(docID string) string {
	id := strings.ToUpper(strings.TrimSpace(docID))
	if strings.EqualFold(id, "data-model") {
		return "data_model"
	}
	if strings.HasPrefix(id, "NEG-") {
		return "business"
	}
	if idx := strings.Index(id, "-"); idx > 0 {
		prefix := id[:idx]
		switch prefix {
		case "RS":
			return "outcome"
		case "RF":
			return "requirement"
		case "FL":
			return "flow"
		case "TP":
			return "test"
		case "ARCH":
			return "architecture"
		case "ADR":
			return "decision"
		case "TECH":
			return "technical"
		case "DB":
			return "database"
		case "CT":
			return "contract"
		case "DM":
			return "data_model"
		case "UJ":
			return "journey"
		case "UXM":
			return "ux_module"
		}
	}
	return ""
}

// ContractLinkExpectedKinds returns allowed target kinds for a link type (nil = any, e.g. supersedes).
func ContractLinkExpectedKinds(linkType string) []string {
	return contractLinkExpectedKinds[linkType]
}

// ContractLinkTargetKindMismatch reports whether target kind fits the contract table for linkType.
func ContractLinkTargetKindMismatch(linkType, targetID, targetKind string) bool {
	expected := ContractLinkExpectedKinds(linkType)
	if len(expected) == 0 {
		return false
	}
	if targetKind == "" {
		targetKind = InferContractKindFromDocID(targetID)
	}
	for _, want := range expected {
		if targetKind == want {
			return false
		}
	}
	return true
}

// ContractBodySectionWarnings checks required Spanish H2 sections and RF acceptance numbering.
func ContractBodySectionWarnings(docID, kind, body string) []string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	required := contractRequiredH2[kind]
	if len(required) == 0 {
		return nil
	}
	present := map[string]struct{}{}
	for _, raw := range extractMarkdownH2Headings(body) {
		present[normalizeContractSectionHeading(raw)] = struct{}{}
	}
	var warnings []string
	src := strings.TrimSpace(docID)
	if src == "" {
		src = "?"
	}
	for _, label := range required {
		key := normalizeContractSectionHeading(label)
		if _, ok := present[key]; !ok {
			warnings = append(warnings, fmtSectionMissing(src, displaySectionLabel(label)))
		}
	}
	if kind == "requirement" {
		if criteria := ParseAcceptanceCriteria(body); len(criteria) == 0 {
			if _, hasHeading := present[normalizeContractSectionHeading("Criterios de aceptación")]; hasHeading {
				warnings = append(warnings, src+": acceptance_criteria_unnumbered")
			}
		}
	}
	return warnings
}

func fmtSectionMissing(docID, heading string) string {
	return "section_missing:" + docID + ":" + heading
}

func displaySectionLabel(label string) string {
	if idx := strings.Index(label, "("); idx > 0 {
		return strings.TrimSpace(label[:idx])
	}
	return label
}

func extractMarkdownH2Headings(body string) []string {
	body = stripYAMLFrontMatter(body)
	out := make([]string, 0)
	for _, match := range h2HeadingPattern.FindAllStringSubmatch(body, -1) {
		if len(match) > 1 {
			out = append(out, strings.TrimSpace(match[1]))
		}
	}
	return out
}

func stripYAMLFrontMatter(content string) string {
	trimmed := strings.TrimPrefix(strings.ReplaceAll(content, "\r", ""), "\ufeff")
	if !strings.HasPrefix(trimmed, "---") {
		return trimmed
	}
	rest := trimmed[3:]
	if end := strings.Index(rest, "\n---"); end >= 0 {
		return rest[end+4:]
	}
	return trimmed
}

func normalizeContractSectionHeading(heading string) string {
	heading = foldAccents(strings.ToLower(strings.TrimSpace(heading)))
	if idx := strings.Index(heading, "("); idx > 0 {
		heading = strings.TrimSpace(heading[:idx])
	}
	switch {
	case heading == "precondiciones/postcondiciones" || strings.Contains(heading, "pre/post"):
		return "pre/postcondiciones"
	case strings.Contains(heading, "contenedores") && strings.Contains(heading, "c4"):
		return "contenedores (c4)"
	case strings.HasPrefix(heading, "casos"):
		return "casos"
	}
	return heading
}

func foldAccents(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, _ := transform.String(t, s)
	return out
}
