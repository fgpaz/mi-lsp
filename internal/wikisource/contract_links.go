package wikisource

import (
	"regexp"
	"sort"
	"strings"

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
			WikiContract string `yaml:"wiki_contract"`
			ID           string `yaml:"id"`
			Kind         string `yaml:"kind"`
			Status       string `yaml:"status"`
			Title        string `yaml:"title"`
			Links        ContractLinks `yaml:"links"`
			Imports      []string `yaml:"imports"`
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

var docIDTokenPattern = regexp.MustCompile(`(?i)\b(RF|TP|FL|RS|ADR|TECH|DB|CT)-[A-Z0-9][A-Z0-9_-]*\b`)

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
	acceptanceHeadingPattern = regexp.MustCompile(`(?im)^##\s+Criterios de aceptación\s*$`)
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
	cases := make([]TestPlanCase, 0)
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
		if !strings.HasPrefix(trimmed, "-") {
			continue
		}
		item := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
		match := testCaseCriterionPattern.FindStringSubmatch(item)
		if len(match) < 2 {
			continue
		}
		n := parseInt(match[1])
		if n <= 0 {
			continue
		}
		disposition, evidence := splitDispositionEvidence(item)
		cases = append(cases, TestPlanCase{
			Criterion:  n,
			Line:       item,
			Disposition: disposition,
			Evidence:   evidence,
		})
	}
	return cases
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
	loc := heading.FindStringIndex(content)
	if loc == nil {
		return ""
	}
	rest := content[loc[1]:]
	next := regexp.MustCompile(`(?m)^##\s+`).FindStringIndex(rest)
	if next != nil {
		rest = rest[:next[0]]
	}
	return rest
}
