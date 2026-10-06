package service

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

const decisionMapRel = "wiki/90-mapa-ids.md"

var decisionIDPattern = regexp.MustCompile(`^D-[0-9]+$`)

// annotatePublishedQuery stamps the published workspace generation when the
// envelope does not already carry one, and the frontmatter visibility mark on
// each file-shaped result. It does not drop or rewrite a generation another
// path already published, and it does not filter on the mark.
func annotatePublishedQuery(root string, env model.Envelope) model.Envelope {
	if strings.TrimSpace(env.GenerationID) == "" {
		generation, _ := store.ReadWorkspaceGenerationSnapshot(context.Background(), root)
		env.GenerationID = generation
	}
	switch items := env.Items.(type) {
	case []map[string]any:
		for i := range items {
			stampMapSensibilidad(root, items[i])
		}
		env.Items = items
	}
	return env
}

func stampMapSensibilidad(root string, item map[string]any) {
	if item == nil {
		return
	}
	item["sensibilidad"] = frontmatterSensibilidad(root, resultPath(item))
}

func resultPath(item map[string]any) string {
	for _, key := range []string{"file", "path", "doc_path"} {
		value, ok := item[key].(string)
		if ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// decisionIntentEnvelope reads wiki/90-mapa-ids.md when the question is a
// decision id from that map and returns the mapped range. A missing map, or an
// id the map does not list, leaves the caller on the ordinary intent path.
func decisionIntentEnvelope(registration model.WorkspaceRegistration, question string) (model.Envelope, bool) {
	id := strings.TrimSpace(question)
	if !decisionIDPattern.MatchString(id) {
		return model.Envelope{}, false
	}
	mapPath := filepath.Join(registration.Root, filepath.FromSlash(decisionMapRel))
	data, err := os.ReadFile(mapPath)
	if err != nil || len(data) == 0 || len(data) > 2<<20 {
		return model.Envelope{}, false
	}
	rel, start, end, ok := lookupDecisionRange(string(data), id)
	if !ok {
		return model.Envelope{}, false
	}
	abs, inside := workspaceFile(registration.Root, rel)
	if !inside {
		return model.Envelope{}, false
	}
	content, lineCount, truncated, readErr := readFileRange(abs, start, end, 200_000)
	item := map[string]any{
		"result_kind": "decision",
		"origin":      "decision-map",
		"decision_id": id,
		"file":        rel,
		"doc_path":    rel,
		"start_line":  start,
		"end_line":    end,
		"line":        start,
		"content":     content,
		"line_count":  lineCount,
		"source_map":  decisionMapRel,
	}
	warnings := []string{}
	if readErr != nil {
		item["content"] = ""
		item["line_count"] = 0
		warnings = append(warnings, "decision range unreadable")
	}
	if truncated {
		item["truncated"] = true
	}
	return model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "intent",
		Mode:      "docs",
		Items:     []map[string]any{item},
		Warnings:  warnings,
		Stats:     model.Stats{Files: 1},
	}, true
}

func lookupDecisionRange(markdown, id string) (string, int, int, bool) {
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
	for _, line := range strings.Split(markdown, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "|---") || strings.HasPrefix(line, "| ---") {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) < 3 || strings.TrimSpace(cells[1]) != id {
			continue
		}
		fr, err := parseFileRangeString(strings.TrimSpace(cells[2]))
		if err != nil || fr.StartLine < 1 {
			return "", 0, 0, false
		}
		end := fr.EndLine
		if end == 0 {
			end = fr.StartLine
		}
		if end < fr.StartLine || strings.TrimSpace(fr.File) == "" {
			return "", 0, 0, false
		}
		return filepath.ToSlash(fr.File), fr.StartLine, end, true
	}
	return "", 0, 0, false
}

func frontmatterSensibilidad(root, rel string) string {
	abs, ok := workspaceFile(root, rel)
	if !ok {
		return ""
	}
	file, err := os.Open(abs)
	if err != nil {
		return ""
	}
	defer file.Close()
	buf := make([]byte, 8192)
	n, _ := file.Read(buf)
	if n == 0 {
		return ""
	}
	return sensibilidadFromFrontmatter(string(buf[:n]))
}

func sensibilidadFromFrontmatter(head string) string {
	head = strings.TrimPrefix(head, "\uFEFF")
	head = strings.ReplaceAll(head, "\r\n", "\n")
	line, rest, found := strings.Cut(head, "\n")
	if !found || strings.TrimSpace(line) != "---" {
		return ""
	}
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return ""
	}
	for _, raw := range strings.Split(rest[:end], "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		key, value, ok := strings.Cut(raw, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "sensibilidad") {
			continue
		}
		return yamlScalar(value)
	}
	return ""
}

func yamlScalar(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "~" || strings.EqualFold(value, "null") {
		return ""
	}
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	if i := strings.Index(value, " #"); i >= 0 {
		value = strings.TrimSpace(value[:i])
	}
	return strings.TrimSpace(value)
}

func workspaceFile(root, rel string) (string, bool) {
	root = filepath.Clean(root)
	if root == "" || root == "." {
		return "", false
	}
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", false
	}
	var abs string
	if filepath.IsAbs(rel) {
		abs = filepath.Clean(rel)
	} else {
		cleaned := filepath.Clean(filepath.FromSlash(rel))
		if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
			return "", false
		}
		abs = filepath.Clean(filepath.Join(root, cleaned))
	}
	if abs != root && !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		return "", false
	}
	return abs, true
}
