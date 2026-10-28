package model

import (
	"regexp"
	"strings"
)

// WikiAnchorStatusResolved and WikiAnchorStatusMissing classify one citation after it was checked
// against the H3 anchors declared by its target document.
const (
	WikiAnchorStatusResolved = "resolved"
	WikiAnchorStatusMissing  = "anchor_missing"
)

// WikiAnchorRef is one `<DOC-ID>#<ANCLA>` citation in prose, resolved to the file:line of the
// H3 that declares the anchor. TargetLine is zero when the anchor is missing.
type WikiAnchorRef struct {
	Ref        string `json:"ref"`
	SourcePath string `json:"source_path"`
	SourceLine int    `json:"source_line"`
	TargetPath string `json:"target_path"`
	TargetLine int    `json:"target_line,omitempty"`
	Status     string `json:"status"`
}

// WikiAnchorFocus is the anchor a trace was asked for (`nav wiki trace <DOC-ID>#<ANCLA>`). Path and
// Line point at the H3 that declares it; Line is zero when the anchor is missing.
type WikiAnchorFocus struct {
	Ref    string `json:"ref"`
	Path   string `json:"path,omitempty"`
	Line   int    `json:"line,omitempty"`
	Status string `json:"status"`
}

// WikiAnchorCitation is one parsed `<DOC-ID>#<ANCLA>` token; Line is 1-based.
type WikiAnchorCitation struct {
	DocID  string
	Anchor string
	Line   int
}

// wikiAnchorRule says which H3 prefixes a kind declares and, for kinds that bind anchors to one
// section, which H2 must enclose them. A sectioned rule whose section cannot be resolved (for
// example an invalid governance language) declares nothing rather than accepting anywhere.
type wikiAnchorRule struct {
	prefix    string
	sectioned bool
	section   WikiContractSection
}

var (
	wikiAnchorDeclarationPattern = regexp.MustCompile(`^###\s+(P[0-9]+|E[0-9]+|UXD-[0-9]+|UXQ-[0-9]+)\s+·\s+\S`)
	wikiAnchorCitationPattern    = regexp.MustCompile(`((?:RS|RF|FL|TP|TECH|CT|DB|DM|ARCH|ADR|UJ|UXM|NEG)-[A-Z0-9][A-Z0-9_-]*)#((?:P|E)[0-9]+|UXD-[0-9]+|UXQ-[0-9]+)`)
)

// DeclaredWikiAnchors returns the first 1-based line of each anchor declared as an H3 prefix
// (`### P3 · título`) in the section the kind allows. A flow declares P<n> under its sequence,
// a requirement declares E<n> under its errors, and a ux_module declares UXD-NN/UXQ-NN anywhere.
// Documents without anchors return an empty map and no findings. A second declaration of the same
// anchor is reported as anchor_duplicate and never replaces the first.
func DeclaredWikiAnchors(docID, body, kind string, language WikiContractLanguage) (map[string]int, []WikiContractFinding) {
	rule, ok := wikiAnchorRuleFor(kind, language)
	if !ok {
		return map[string]int{}, nil
	}
	declared := map[string]int{}
	findings := make([]WikiContractFinding, 0)
	currentH2 := ""
	for index, line := range wikiAnchorLines(body) {
		if match := wikiH2Pattern.FindStringSubmatch(line); len(match) > 1 {
			currentH2 = match[1]
			continue
		}
		match := wikiAnchorDeclarationPattern.FindStringSubmatch(line)
		if len(match) < 2 || !rule.accepts(match[1]) || !rule.inSection(currentH2) {
			continue
		}
		anchor := match[1]
		if _, exists := declared[anchor]; exists {
			findings = append(findings, WikiContractFinding{
				Code: "anchor_duplicate", DocID: docID, Kind: strings.ToLower(strings.TrimSpace(kind)),
				Message: "anchor_duplicate:" + docID + ":" + anchor, Blocking: false,
			})
			continue
		}
		declared[anchor] = index + 1
	}
	return declared, findings
}

// ParseWikiAnchorCitations returns each `<DOC-ID>#<ANCLA>` token in prose. Front matter and fenced
// code are ignored, and a token glued to a longer identifier (`xFL-A#P1`, `FL-A#P1a`) is not a citation.
func ParseWikiAnchorCitations(body string) []WikiAnchorCitation {
	citations := make([]WikiAnchorCitation, 0)
	for index, line := range wikiAnchorLines(body) {
		for _, match := range wikiAnchorCitationPattern.FindAllStringSubmatchIndex(line, -1) {
			if !wikiAnchorBoundaries(line, match[0], match[1]) {
				continue
			}
			citations = append(citations, WikiAnchorCitation{
				DocID:  line[match[2]:match[3]],
				Anchor: line[match[4]:match[5]],
				Line:   index + 1,
			})
		}
	}
	return citations
}

func wikiAnchorRuleFor(kind string, language WikiContractLanguage) (wikiAnchorRule, bool) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "flow":
		return wikiAnchorRule{prefix: "P", sectioned: true, section: sectionByKey(WikiContractRequiredSections("flow", language), "sequence")}, true
	case "requirement":
		return wikiAnchorRule{prefix: "E", sectioned: true, section: sectionByKey(WikiContractRequiredSections("requirement", language), "errors")}, true
	case "ux_module":
		return wikiAnchorRule{prefix: "UX"}, true
	}
	return wikiAnchorRule{}, false
}

func (rule wikiAnchorRule) accepts(anchor string) bool {
	return strings.HasPrefix(anchor, rule.prefix)
}

func (rule wikiAnchorRule) inSection(currentH2 string) bool {
	if !rule.sectioned {
		return true
	}
	if rule.section.Key == "" {
		return false
	}
	return wikiContractSectionPresent(map[string]struct{}{normalizeWikiContractHeading(currentH2): {}}, rule.section)
}

// wikiAnchorBoundaries rejects a match glued to a longer token on either side, using the same
// boundary rule as the document ID grammar: letters, digits, underscore, hyphen, and a leading dot.
func wikiAnchorBoundaries(line string, start, end int) bool {
	if start > 0 && wikiAnchorContinues(line[start-1], true) {
		return false
	}
	if end < len(line) && wikiAnchorContinues(line[end], false) {
		return false
	}
	return true
}

func wikiAnchorContinues(b byte, leading bool) bool {
	if b == '_' || b == '-' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') {
		return true
	}
	return leading && b == '.'
}

// wikiAnchorLines returns the body split into lines with front matter and fenced code blanked.
// Blanking instead of stripping keeps every line number equal to its line in the file.
func wikiAnchorLines(body string) []string {
	body = strings.TrimPrefix(strings.ReplaceAll(body, "\r", ""), "\ufeff")
	lines := strings.Split(body, "\n")
	frontMatterEnd := -1
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				frontMatterEnd = i
				break
			}
		}
	}
	fence := ""
	for i, line := range lines {
		if i <= frontMatterEnd {
			lines[i] = ""
			continue
		}
		trimmed := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			lines[i] = ""
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence = trimmed[:3]
			lines[i] = ""
		}
	}
	return lines
}
