package wikisource

import (
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// WikiLink is one typed author-declared WIKI-CONTRACT-v1 link. Document links
// use DocID or Path; code links use Path and an optional Symbol.
type WikiLink struct {
	Relation string
	DocID    string
	Path     string
	Symbol   string
}

var wikiContractLinkRelations = []string{
	"fulfills", "implements", "verifies", "affects", "supports",
	"materializes", "supersedes", "experiences", "stages", "maps",
	"grounds_in", "code",
}

// WikiContractLinks extracts the typed links needed by graph staging without
// reparsing unrelated source-block metadata.
func WikiContractLinks(content string) []WikiLink {
	return parseWikiContractLinks(content)
}

func parseWikiContractLinks(content string) []WikiLink {
	frontMatter, ok := leadingWikiFrontMatter(content)
	if !ok {
		return nil
	}
	var document map[string]any
	if err := yaml.Unmarshal(frontMatter, &document); err != nil || !wikiContractV1(document["wiki_contract"]) {
		return nil
	}
	links, ok := document["links"].(map[string]any)
	if !ok {
		return nil
	}

	parsed := make([]WikiLink, 0)
	for _, relation := range wikiContractLinkRelations {
		raw, exists := links[relation]
		if !exists {
			continue
		}
		if relation == "code" {
			for _, value := range wikiContractList(raw) {
				path, symbol := wikiCodeTarget(value)
				if !validateBindingPath(path) {
					continue
				}
				parsed = append(parsed, WikiLink{Relation: relation, Path: filepath.ToSlash(strings.TrimSpace(path)), Symbol: strings.TrimSpace(symbol)})
			}
			continue
		}
		for _, value := range wikiContractStrings(raw) {
			if strings.Contains(value, "/") || strings.HasSuffix(strings.ToLower(value), ".md") {
				if validateBindingPath(value) {
					parsed = append(parsed, WikiLink{Relation: relation, Path: filepath.ToSlash(value)})
				}
			} else {
				parsed = append(parsed, WikiLink{Relation: relation, DocID: value})
			}
		}
	}
	sort.Slice(parsed, func(i, j int) bool {
		a, b := parsed[i], parsed[j]
		if a.Relation != b.Relation {
			return a.Relation < b.Relation
		}
		if a.DocID != b.DocID {
			return a.DocID < b.DocID
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Symbol < b.Symbol
	})
	return parsed
}

func leadingWikiFrontMatter(content string) ([]byte, bool) {
	content = strings.TrimPrefix(content, "\ufeff")
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return nil, false
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return []byte(strings.Join(lines[1:i], "\n")), true
		}
	}
	return nil, false
}

func wikiContractV1(value any) bool {
	switch version := value.(type) {
	case int:
		return version == 1
	case int64:
		return version == 1
	case uint64:
		return version == 1
	case float64:
		return version == 1
	case string:
		value := strings.ToLower(strings.TrimSpace(version))
		if value == "wiki-contract-v1" || value == "wiki_contract-v1" {
			return true
		}
		return strings.TrimPrefix(value, "v") == "1"
	case map[string]any:
		return wikiContractV1(version["version"])
	default:
		return false
	}
}

func wikiContractList(value any) []any {
	if values, ok := value.([]any); ok {
		return values
	}
	if value == nil {
		return nil
	}
	return []any{value}
}

func wikiContractStrings(value any) []string {
	values := make([]string, 0)
	for _, item := range wikiContractList(value) {
		if text, ok := item.(string); ok {
			text = strings.TrimSpace(text)
			if text != "" {
				values = append(values, text)
			}
		}
	}
	return values
}

func wikiCodeTarget(value any) (string, string) {
	if path, ok := value.(string); ok {
		return strings.TrimSpace(path), ""
	}
	target, ok := value.(map[string]any)
	if !ok {
		return "", ""
	}
	path, _ := target["path"].(string)
	symbol, _ := target["symbol"].(string)
	return strings.TrimSpace(path), strings.TrimSpace(symbol)
}
