package model

import (
	"fmt"
	"strings"
)

// WikiRoadmapSource is the optional, non-canonical backup location from the v1.1 brief.
const WikiRoadmapSource = ".docs/roadmap/roadmap.md"

// WikiRoadmapMetadata is display-only RS metadata; it never represents a graph edge.
type WikiRoadmapMetadata struct {
	ID        string `json:"id,omitempty"`
	RS        string `json:"rs"`
	Horizon   string `json:"horizon,omitempty"`
	State     string `json:"state,omitempty"`
	Canonical bool   `json:"canonical"`
	Source    string `json:"source"`
}

// WikiRoadmapWarning is a typed non-blocking note about an optional roadmap source.
type WikiRoadmapWarning struct {
	Code     string `json:"code"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
	Blocking bool   `json:"blocking"`
}

// WikiRoadmapParseResult maps RS identifiers to non-canonical metadata.
type WikiRoadmapParseResult struct {
	Source    string                         `json:"source"`
	Canonical bool                           `json:"canonical"`
	ByRS      map[string]WikiRoadmapMetadata `json:"by_rs"`
	Warnings  []WikiRoadmapWarning           `json:"warnings,omitempty"`
}

// ParseWikiRoadmapMarkdown reads the minimal v0 table format:
// | id | RS | horizon/horizonte | state/estado |.
// The format mirrors the four fields specified by wiki-model v1.1; no links are emitted.
func ParseWikiRoadmapMarkdown(content string) WikiRoadmapParseResult {
	result := WikiRoadmapParseResult{
		Source: WikiRoadmapSource, Canonical: false,
		ByRS:     make(map[string]WikiRoadmapMetadata),
		Warnings: []WikiRoadmapWarning{},
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r", ""), "\n")
	headerLine := -1
	columns := map[string]int{}
	for i, line := range lines {
		cells, ok := splitWikiRoadmapTableRow(line)
		if !ok {
			continue
		}
		found := make(map[string]int)
		for column, cell := range cells {
			switch normalizeWikiRoadmapHeader(cell) {
			case "id":
				found["id"] = column
			case "rs":
				found["rs"] = column
			case "horizon", "horizonte":
				found["horizon"] = column
			case "state", "estado":
				found["state"] = column
			}
		}
		if len(found) == 4 {
			headerLine, columns = i, found
			break
		}
	}
	if headerLine < 0 {
		if strings.TrimSpace(content) != "" {
			result.Warnings = append(result.Warnings, WikiRoadmapWarning{
				Code: "roadmap_table_missing", Message: "roadmap_table_missing: expected columns id, RS, horizon, state", Blocking: false,
			})
		}
		return result
	}

	seen := make(map[string]bool)
	ambiguous := make(map[string]bool)
	for i := headerLine + 1; i < len(lines); i++ {
		cells, ok := splitWikiRoadmapTableRow(lines[i])
		if !ok {
			if strings.TrimSpace(lines[i]) == "" {
				continue
			}
			break
		}
		if wikiRoadmapSeparatorRow(cells) {
			continue
		}
		maxColumn := columns["id"]
		for _, column := range columns {
			if column > maxColumn {
				maxColumn = column
			}
		}
		if len(cells) <= maxColumn {
			result.Warnings = append(result.Warnings, wikiRoadmapWarning("roadmap_row_incomplete", i+1, "roadmap row is missing one or more declared columns"))
			continue
		}
		id := strings.TrimSpace(cells[columns["id"]])
		rs := strings.ToUpper(strings.TrimSpace(cells[columns["rs"]]))
		horizon := strings.TrimSpace(cells[columns["horizon"]])
		state := strings.TrimSpace(cells[columns["state"]])
		if id == "" {
			result.Warnings = append(result.Warnings, wikiRoadmapWarning("roadmap_id_missing", i+1, "roadmap row is missing id"))
			continue
		}
		if !validWikiRoadmapRS(rs) {
			result.Warnings = append(result.Warnings, wikiRoadmapWarning("roadmap_rs_invalid", i+1, "roadmap row RS must use the RS- identifier prefix"))
			continue
		}
		if horizon == "" {
			result.Warnings = append(result.Warnings, wikiRoadmapWarning("roadmap_horizon_missing", i+1, "roadmap row is missing horizon"))
		}
		if state == "" {
			result.Warnings = append(result.Warnings, wikiRoadmapWarning("roadmap_state_missing", i+1, "roadmap row is missing state"))
		}
		if seen[rs] {
			delete(result.ByRS, rs)
			ambiguous[rs] = true
			result.Warnings = append(result.Warnings, wikiRoadmapWarning("roadmap_rs_duplicate", i+1, fmt.Sprintf("roadmap has more than one row for %s; metadata omitted as ambiguous", rs)))
			continue
		}
		if ambiguous[rs] {
			continue
		}
		seen[rs] = true
		result.ByRS[rs] = WikiRoadmapMetadata{
			ID: id, RS: rs, Horizon: horizon, State: state,
			Canonical: false, Source: WikiRoadmapSource,
		}
	}
	return result
}

func splitWikiRoadmapTableRow(line string) ([]string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.Contains(trimmed, "|") {
		return nil, false
	}
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")
	parts := strings.Split(trimmed, "|")
	cells := make([]string, len(parts))
	for i, part := range parts {
		cells[i] = strings.TrimSpace(strings.Trim(part, " `"))
	}
	return cells, len(cells) > 1
}

func normalizeWikiRoadmapHeader(value string) string {
	value = strings.ToLower(strings.TrimSpace(strings.Trim(value, "*`_ ")))
	value = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n").Replace(value)
	return value
}

func wikiRoadmapSeparatorRow(cells []string) bool {
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			continue
		}
		for _, r := range cell {
			if r != '-' && r != ':' && r != ' ' {
				return false
			}
		}
	}
	return true
}

func validWikiRoadmapRS(value string) bool {
	if !strings.HasPrefix(value, "RS-") || len(value) < 4 {
		return false
	}
	for _, r := range value[3:] {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func wikiRoadmapWarning(code string, line int, message string) WikiRoadmapWarning {
	return WikiRoadmapWarning{Code: code, Line: line, Message: message, Blocking: false}
}
