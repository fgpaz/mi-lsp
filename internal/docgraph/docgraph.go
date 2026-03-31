package docgraph

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

var (
	docIDPattern        = regexp.MustCompile(`\b(?:FL|RF|TP|TECH|CT|DB)-[A-Z0-9-]+\b`)
	markdownLinkPattern = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)
	inlineCodePattern   = regexp.MustCompile("`([^`]+)`")
	pascalSymbolPattern = regexp.MustCompile(`\b[A-Z][A-Za-z0-9_]+\b`)
)

type RFFrontMatter struct {
	ID         string   `yaml:"id"`
	Title      string   `yaml:"title"`
	Implements []string `yaml:"implements"`
	Tests      []string `yaml:"tests"`
}

func ProfilePath(root string) string {
	return filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml")
}

func DefaultProfile() model.DocsReadProfile {
	return model.DocsReadProfile{
		Version: 1,
		Families: []model.DocsReadFamily{
			{
				Name:           "functional",
				IntentKeywords: []string{"scope", "flow", "feature", "behavior", "rf", "fl", "test", "workflow", "journey"},
				Paths: []string{
					".docs/wiki/01_*.md",
					".docs/wiki/02_*.md",
					".docs/wiki/03_FL.md",
					".docs/wiki/03_FL/*.md",
					".docs/wiki/04_RF.md",
					".docs/wiki/04_RF/*.md",
					".docs/wiki/05_*.md",
					".docs/wiki/06_*.md",
					".docs/wiki/06_pruebas/*.md",
				},
			},
			{
				Name:           "technical",
				IntentKeywords: []string{"technical", "daemon", "worker", "runtime", "backend", "contract", "protocol", "search", "context", "refs", "service", "index", "routing"},
				Paths: []string{
					".docs/wiki/07_*.md",
					".docs/wiki/07_tech/*.md",
					".docs/wiki/08_*.md",
					".docs/wiki/08_db/*.md",
					".docs/wiki/09_*.md",
					".docs/wiki/09_contratos/*.md",
				},
			},
			{
				Name:           "ux",
				IntentKeywords: []string{"ux", "ui", "frontend", "visual", "design", "journey", "experience", "pattern", "interface"},
				Paths: []string{
					".docs/wiki/10_*.md",
					".docs/wiki/11_*.md",
					".docs/wiki/12_*.md",
					".docs/wiki/13_*.md",
					".docs/wiki/14_*.md",
					".docs/wiki/15_*.md",
					".docs/wiki/16_*.md",
				},
			},
		},
		GenericDocs: model.DocsGenericFallback{
			Paths: []string{"README.md", "README*.md", "docs/", ".docs/"},
		},
	}
}

func LoadProfile(root string) (model.DocsReadProfile, string, []string) {
	profile := DefaultProfile()
	path := ProfilePath(root)
	if _, err := os.Stat(path); err == nil {
		if _, err := toml.DecodeFile(path, &profile); err == nil {
			if profile.Version == 0 {
				profile.Version = 1
			}
			return profile, "project", nil
		}
		return DefaultProfile(), "default", []string{fmt.Sprintf("read-model parse failed; using defaults: %v", err)}
	}
	return profile, "default", nil
}

func IndexWorkspaceDocs(root string, matcher *workspace.IgnoreMatcher) ([]model.DocRecord, []model.DocEdge, []model.DocMention, []string, error) {
	profile, _, warnings := LoadProfile(root)
	candidates, err := collectDocCandidates(root, profile, matcher)
	if err != nil {
		return nil, nil, nil, warnings, err
	}
	docs := make([]model.DocRecord, 0, len(candidates))
	edges := make([]model.DocEdge, 0)
	mentions := make([]model.DocMention, 0)
	seenDocID := map[string]string{}
	pendingDocIDEdges := make([]model.DocEdge, 0)

	for _, candidate := range candidates {
		content, err := os.ReadFile(candidate.path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("doc read failed for %s: %v", candidate.relativePath, err))
			continue
		}
		title := extractTitle(content)
		docID := firstDocID(title + "\n" + string(content))
		if docID != "" {
			seenDocID[docID] = candidate.relativePath
		}
		doc := model.DocRecord{
			Path:        candidate.relativePath,
			Title:       title,
			DocID:       docID,
			Layer:       candidate.layer,
			Family:      candidate.family,
			Snippet:     extractSnippet(content),
			SearchText:  normalizeSearchText(title + "\n" + candidate.relativePath + "\n" + string(content)),
			ContentHash: digest(content),
			IndexedAt:   time.Now().Unix(),
		}
		docs = append(docs, doc)

		docMentions, docEdges := extractReferences(root, candidate.relativePath, string(content))
		mentions = append(mentions, docMentions...)

		if candidate.layer == "04" || strings.HasPrefix(doc.DocID, "RF-") {
			if fm := extractFrontMatter(content); fm != nil {
				for _, impl := range fm.Implements {
					impl = strings.TrimSpace(impl)
					if impl != "" {
						mentions = append(mentions, model.DocMention{
							DocPath:      candidate.relativePath,
							MentionType:  "implements",
							MentionValue: impl,
						})
					}
				}
				for _, test := range fm.Tests {
					test = strings.TrimSpace(test)
					if test != "" {
						mentions = append(mentions, model.DocMention{
							DocPath:      candidate.relativePath,
							MentionType:  "test_file",
							MentionValue: test,
						})
					}
				}
			}
		}

		for _, edge := range docEdges {
			if edge.ToDocID != "" && edge.ToPath == "" {
				pendingDocIDEdges = append(pendingDocIDEdges, edge)
				continue
			}
			edges = append(edges, edge)
		}
	}

	for _, edge := range pendingDocIDEdges {
		if path := seenDocID[edge.ToDocID]; path != "" {
			edge.ToPath = path
			edges = append(edges, edge)
		}
	}

	sort.Slice(docs, func(i, j int) bool {
		if docs[i].Family == docs[j].Family {
			if docs[i].Layer == docs[j].Layer {
				return docs[i].Path < docs[j].Path
			}
			return docs[i].Layer < docs[j].Layer
		}
		return docs[i].Family < docs[j].Family
	})
	return docs, edges, mentions, warnings, nil
}

type docCandidate struct {
	path         string
	relativePath string
	family       string
	layer        string
	priority     int
}

func collectDocCandidates(root string, profile model.DocsReadProfile, matcher *workspace.IgnoreMatcher) ([]docCandidate, error) {
	seen := map[string]docCandidate{}
	addCandidate := func(absPath string, family string, priority int) {
		if matcher != nil && matcher.ShouldIgnore(root, absPath) {
			return
		}
		info, err := os.Stat(absPath)
		if err != nil || info.IsDir() {
			return
		}
		rel, err := filepath.Rel(root, absPath)
		if err != nil {
			return
		}
		rel = filepath.ToSlash(rel)
		candidate := docCandidate{
			path:         absPath,
			relativePath: rel,
			family:       family,
			layer:        detectLayer(rel),
			priority:     priority,
		}
		if existing, ok := seen[rel]; !ok || priority < existing.priority {
			seen[rel] = candidate
		}
	}

	for familyIdx, family := range profile.Families {
		for _, pattern := range family.Paths {
			if err := expandPattern(root, pattern, func(absPath string) {
				addCandidate(absPath, family.Name, familyIdx)
			}); err != nil {
				return nil, err
			}
		}
	}
	for _, pattern := range profile.GenericDocs.Paths {
		if err := expandPattern(root, pattern, func(absPath string) {
			addCandidate(absPath, "generic", len(profile.Families)+10)
		}); err != nil {
			return nil, err
		}
	}

	items := make([]docCandidate, 0, len(seen))
	for _, candidate := range seen {
		items = append(items, candidate)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].priority == items[j].priority {
			return items[i].relativePath < items[j].relativePath
		}
		return items[i].priority < items[j].priority
	})
	return items, nil
}

func expandPattern(root string, pattern string, visit func(string)) error {
	trimmed := filepath.ToSlash(strings.TrimSpace(pattern))
	if trimmed == "" {
		return nil
	}
	if strings.HasSuffix(trimmed, "/") {
		dir := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(trimmed, "/")))
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if entry.IsDir() {
					return nil
				}
				if strings.EqualFold(filepath.Ext(path), ".md") {
					visit(path)
				}
				return nil
			})
		}
		return nil
	}
	if strings.ContainsAny(trimmed, "*?[") {
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(trimmed)))
		if err != nil {
			return err
		}
		for _, match := range matches {
			visit(match)
		}
		return nil
	}
	absPath := filepath.Join(root, filepath.FromSlash(trimmed))
	visit(absPath)
	return nil
}

func extractReferences(root string, docPath string, content string) ([]model.DocMention, []model.DocEdge) {
	mentions := make([]model.DocMention, 0)
	edges := make([]model.DocEdge, 0)
	seenMentions := map[string]struct{}{}
	seenEdges := map[string]struct{}{}
	addMention := func(kind string, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := kind + "::" + value
		if _, ok := seenMentions[key]; ok {
			return
		}
		seenMentions[key] = struct{}{}
		mentions = append(mentions, model.DocMention{DocPath: docPath, MentionType: kind, MentionValue: value})
	}
	addEdge := func(edge model.DocEdge) {
		key := edge.FromPath + "::" + edge.Kind + "::" + edge.ToPath + "::" + edge.ToDocID + "::" + edge.Label
		if _, ok := seenEdges[key]; ok {
			return
		}
		seenEdges[key] = struct{}{}
		edges = append(edges, edge)
	}

	for _, match := range docIDPattern.FindAllString(content, -1) {
		addMention("doc_id", match)
		addEdge(model.DocEdge{FromPath: docPath, ToDocID: match, Kind: "doc_id", Label: match})
	}

	for _, match := range markdownLinkPattern.FindAllStringSubmatch(content, -1) {
		if len(match) < 2 {
			continue
		}
		link := strings.TrimSpace(match[1])
		if link == "" || strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") {
			continue
		}
		target := normalizeDocLink(docPath, link)
		addMention("doc_path", target)
		addEdge(model.DocEdge{FromPath: docPath, ToPath: target, Kind: "markdown_link", Label: link})
	}

	for _, match := range inlineCodePattern.FindAllStringSubmatch(content, -1) {
		if len(match) < 2 {
			continue
		}
		value := strings.TrimSpace(match[1])
		if value == "" {
			continue
		}
		switch {
		case strings.HasPrefix(value, "mi-lsp "):
			addMention("command", value)
		case strings.Contains(value, "/") || strings.Contains(value, "\\"):
			normalized := filepath.ToSlash(value)
			if likelyCodePath(normalized) {
				if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(normalized))); err == nil {
					addMention("file_path", normalized)
				}
			}
		default:
			if pascalSymbolPattern.MatchString(value) {
				for _, symbol := range pascalSymbolPattern.FindAllString(value, -1) {
					addMention("symbol", symbol)
				}
			}
		}
	}
	return mentions, edges
}

func normalizeDocLink(docPath string, link string) string {
	baseDir := filepath.ToSlash(filepath.Dir(docPath))
	if strings.HasPrefix(link, "./") || strings.HasPrefix(link, "../") {
		return filepath.ToSlash(filepath.Clean(filepath.Join(baseDir, filepath.FromSlash(link))))
	}
	return filepath.ToSlash(strings.TrimPrefix(link, "/"))
}

func likelyCodePath(value string) bool {
	ext := strings.ToLower(filepath.Ext(value))
	switch ext {
	case ".go", ".cs", ".ts", ".tsx", ".js", ".jsx", ".py", ".md", ".toml":
		return true
	default:
		return strings.Contains(value, "/") && !strings.Contains(value, " ")
	}
}

func detectLayer(path string) string {
	path = filepath.ToSlash(path)
	base := filepath.Base(path)
	if len(base) >= 2 && base[0] >= '0' && base[0] <= '9' && base[1] >= '0' && base[1] <= '9' {
		return base[:2]
	}
	switch {
	case strings.Contains(path, "/03_FL/"):
		return "03"
	case strings.Contains(path, "/04_RF/"):
		return "04"
	case strings.Contains(path, "/06_pruebas/"):
		return "06"
	case strings.Contains(path, "/07_tech/"):
		return "07"
	case strings.Contains(path, "/08_db/"):
		return "08"
	case strings.Contains(path, "/09_contratos/"):
		return "09"
	default:
		return "generic"
	}
}

func extractTitle(content []byte) string {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			return strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
	}
	return ""
}

func extractSnippet(content []byte) string {
	text := strings.ReplaceAll(string(content), "\r", "")
	lines := strings.Split(text, "\n")
	parts := make([]string, 0, 4)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "|") || strings.HasPrefix(line, "```") {
			continue
		}
		parts = append(parts, line)
		if len(strings.Join(parts, " ")) >= 200 {
			break
		}
	}
	snippet := strings.Join(parts, " ")
	if len(snippet) > 220 {
		return strings.TrimSpace(snippet[:220]) + "..."
	}
	return snippet
}

func normalizeSearchText(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")
	return strings.Join(strings.Fields(value), " ")
}

func firstDocID(value string) string {
	if match := docIDPattern.FindString(value); match != "" {
		return match
	}
	return ""
}

func MatchFamily(question string, profile model.DocsReadProfile) string {
	normalized := normalizeSearchText(question)
	bestFamily := "technical"
	bestScore := -1
	for _, family := range profile.Families {
		score := 0
		for _, keyword := range family.IntentKeywords {
			keyword = normalizeSearchText(keyword)
			if keyword != "" && strings.Contains(normalized, keyword) {
				score += 3
			}
		}
		if score > bestScore {
			bestScore = score
			bestFamily = family.Name
		}
	}
	return bestFamily
}

func QuestionTokens(question string) []string {
	normalized := normalizeSearchText(question)
	parts := strings.Fields(normalized)
	stopwords := map[string]struct{}{
		"the": {}, "and": {}, "for": {}, "with": {}, "this": {}, "that": {},
		"como": {}, "para": {}, "donde": {}, "porque": {}, "sobre": {}, "desde": {},
		"una": {}, "uno": {}, "que": {}, "del": {}, "las": {}, "los": {}, "con": {},
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) < 3 {
			continue
		}
		if _, ok := stopwords[part]; ok {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	return result
}

func extractFrontMatter(content []byte) *RFFrontMatter {
	text := string(content)
	if !strings.HasPrefix(strings.TrimSpace(text), "---") {
		return nil
	}
	text = strings.TrimSpace(text)
	rest := text[3:]
	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return nil
	}
	yamlContent := rest[:endIdx]

	var fm RFFrontMatter
	if match := regexp.MustCompile(`(?m)^id:\s*(.+)`).FindStringSubmatch(yamlContent); len(match) > 1 {
		fm.ID = strings.TrimSpace(match[1])
	}
	if match := regexp.MustCompile(`(?m)^title:\s*(.+)`).FindStringSubmatch(yamlContent); len(match) > 1 {
		fm.Title = strings.TrimSpace(match[1])
	}
	fm.Implements = parseYAMLArray(yamlContent, "implements")
	fm.Tests = parseYAMLArray(yamlContent, "tests")

	if fm.ID == "" && len(fm.Implements) == 0 && len(fm.Tests) == 0 {
		return nil
	}
	return &fm
}

func parseYAMLArray(yamlContent string, key string) []string {
	lines := strings.Split(yamlContent, "\n")
	var result []string
	inArray := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, key+":") {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, key+":"))
			if value != "" && !strings.HasPrefix(value, "[") {
				result = append(result, value)
				return result
			}
			inArray = true
			continue
		}
		if inArray {
			if strings.HasPrefix(trimmed, "- ") {
				item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
				if item != "" {
					result = append(result, item)
				}
			} else if trimmed != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				break
			}
		}
	}
	return result
}

func digest(content []byte) string {
	sum := sha1.Sum(content)
	return hex.EncodeToString(sum[:])
}
