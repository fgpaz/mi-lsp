package model

import (
	"fmt"
	"sort"
	"strings"
)

// WikiRoadmapSource is the optional, non-canonical local roadmap.
const WikiRoadmapSource = ".docs/roadmap/roadmap.md"

type WikiRoadmapMetadata struct {
	ID        string   `json:"id"`
	RS        string   `json:"rs"`
	Horizon   string   `json:"horizon"`
	Status    string   `json:"status"`
	Title     string   `json:"title,omitempty"`
	DependsOn []string `json:"depends_on,omitempty"`
	Canonical bool     `json:"canonical"`
	Source    string   `json:"source"`
}

type WikiRoadmapWarning struct {
	Code     string `json:"code"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
	Blocking bool   `json:"blocking"`
}

type WikiRoadmapParseResult struct {
	Source    string                         `json:"source"`
	Canonical bool                           `json:"canonical"`
	Entries   []WikiRoadmapMetadata          `json:"entries"`
	ByRS      map[string]WikiRoadmapMetadata `json:"by_rs"`
	Warnings  []WikiRoadmapWarning           `json:"warnings,omitempty"`
}

// ParseWikiRoadmapMarkdown parses a non-canonical Markdown table with English
// structural headers: id, rs, horizon, status, and optional depends_on/title.
func ParseWikiRoadmapMarkdown(content string) WikiRoadmapParseResult {
	result := WikiRoadmapParseResult{Source: WikiRoadmapSource, Canonical: false, Entries: []WikiRoadmapMetadata{}, ByRS: map[string]WikiRoadmapMetadata{}, Warnings: []WikiRoadmapWarning{}}
	lines := strings.Split(strings.ReplaceAll(content, "\r", ""), "\n")
	columns, header := map[string]int{}, -1
	for i, line := range lines {
		cells, ok := splitWikiRoadmapTableRow(line)
		if !ok {
			continue
		}
		found := map[string]int{}
		for col, cell := range cells {
			switch normalizeWikiRoadmapHeader(cell) {
			case "id", "rs", "horizon", "status", "depends_on", "title":
				found[normalizeWikiRoadmapHeader(cell)] = col
			}
		}
		if _, id := found["id"]; id {
			if _, rs := found["rs"]; rs {
				if _, h := found["horizon"]; h {
					if _, s := found["status"]; s {
						columns, header = found, i
						break
					}
				}
			}
		}
	}
	if header < 0 {
		if strings.TrimSpace(content) != "" {
			result.Warnings = append(result.Warnings, wikiRoadmapWarning("roadmap_table_missing", 0, "expected columns id, rs, horizon, status"))
		}
		return result
	}
	seenIDs, seenRS := map[string]bool{}, map[string]bool{}
	for i := header + 1; i < len(lines); i++ {
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
		max := 0
		for _, col := range columns {
			if col > max {
				max = col
			}
		}
		if len(cells) <= max {
			result.Warnings = append(result.Warnings, wikiRoadmapWarning("roadmap_row_incomplete", i+1, "roadmap row is missing a declared column"))
			continue
		}
		cell := func(name string) string {
			col, ok := columns[name]
			if !ok || col >= len(cells) {
				return ""
			}
			return strings.TrimSpace(cells[col])
		}
		id, rs := cell("id"), strings.ToUpper(cell("rs"))
		horizon, status := strings.ToLower(cell("horizon")), strings.ToLower(cell("status"))
		if id == "" {
			result.Warnings = append(result.Warnings, wikiRoadmapWarning("roadmap_id_missing", i+1, "roadmap row is missing id"))
			continue
		}
		if !validWikiRoadmapRS(rs) {
			result.Warnings = append(result.Warnings, wikiRoadmapWarning("roadmap_rs_invalid", i+1, "rs must use the RS- identifier prefix"))
			continue
		}
		if horizon != "short" && horizon != "medium" && horizon != "long" {
			result.Warnings = append(result.Warnings, wikiRoadmapWarning("roadmap_horizon_invalid", i+1, "horizon must be short, medium, or long"))
		}
		if status != "todo" && status != "doing" && status != "done" && status != "blocked" {
			result.Warnings = append(result.Warnings, wikiRoadmapWarning("roadmap_status_invalid", i+1, "status must be todo, doing, done, or blocked"))
		}
		if seenIDs[id] {
			result.Warnings = append(result.Warnings, wikiRoadmapWarning("roadmap_id_duplicate", i+1, "roadmap item id is duplicated"))
			continue
		}
		if seenRS[rs] {
			result.Warnings = append(result.Warnings, wikiRoadmapWarning("roadmap_rs_duplicate", i+1, fmt.Sprintf("more than one roadmap item references %s", rs)))
		}
		seenIDs[id], seenRS[rs] = true, true
		deps := []string{}
		for _, dep := range strings.Split(cell("depends_on"), ",") {
			dep = strings.TrimSpace(dep)
			if dep != "" {
				deps = append(deps, dep)
			}
		}
		sort.Strings(deps)
		entry := WikiRoadmapMetadata{ID: id, RS: rs, Horizon: horizon, Status: status, Title: cell("title"), DependsOn: deps, Canonical: false, Source: WikiRoadmapSource}
		result.Entries = append(result.Entries, entry)
		if !seenRS[rs] || len(result.ByRS[rs].RS) == 0 {
			result.ByRS[rs] = entry
		}
	}
	for _, e := range result.Entries {
		for _, dep := range e.DependsOn {
			if !seenIDs[dep] {
				result.Warnings = append(result.Warnings, wikiRoadmapWarning("roadmap_dependency_unknown", 0, fmt.Sprintf("roadmap item %s depends on unknown item %s", e.ID, dep)))
			}
		}
	}
	if cycle := wikiRoadmapCycle(result.Entries); len(cycle) > 0 {
		result.Warnings = append(result.Warnings, wikiRoadmapWarning("roadmap_dependency_cycle", 0, "roadmap dependency cycle: "+strings.Join(cycle, " -> ")))
	}
	return result
}

func wikiRoadmapCycle(entries []WikiRoadmapMetadata) []string {
	deps := map[string][]string{}
	for _, e := range entries {
		deps[e.ID] = e.DependsOn
	}
	state, stack := map[string]int{}, []string{}
	var cycle []string
	var visit func(string) bool
	visit = func(id string) bool {
		if state[id] == 1 {
			for i, item := range stack {
				if item == id {
					cycle = append(append([]string{}, stack[i:]...), id)
					return true
				}
			}
			return true
		}
		if state[id] == 2 {
			return false
		}
		state[id] = 1
		stack = append(stack, id)
		for _, dep := range deps[id] {
			if _, ok := deps[dep]; ok && visit(dep) {
				return true
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = 2
		return false
	}
	ids := make([]string, 0, len(deps))
	for id := range deps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if visit(id) {
			return cycle
		}
	}
	return nil
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
	for i, p := range parts {
		cells[i] = strings.TrimSpace(strings.Trim(p, " `"))
	}
	return cells, len(cells) > 1
}
func normalizeWikiRoadmapHeader(value string) string {
	value = strings.ToLower(strings.TrimSpace(strings.Trim(value, "*`_ ")))
	return strings.ReplaceAll(value, "-", "_")
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
