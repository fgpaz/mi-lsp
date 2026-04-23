package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func (a *App) wikiSearch(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	if blockedEnv, err := a.governanceGateEnvelope(ctx, request, "nav.wiki.search"); err != nil {
		return model.Envelope{}, err
	} else if blockedEnv != nil {
		return *blockedEnv, nil
	}

	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}
	memory, _ := loadReentryMemory(ctx, registration.Root)

	queryText := strings.TrimSpace(firstNonEmpty(
		stringPayload(request.Payload, "query"),
		stringPayload(request.Payload, "pattern"),
		stringPayload(request.Payload, "task"),
	))
	if queryText == "" {
		return model.Envelope{}, fmt.Errorf("query is required")
	}

	query := loadDocQueryContext(ctx, registration, queryText)
	defer query.Close()
	if query.dbErr != nil {
		return model.Envelope{}, query.dbErr
	}

	warnings := append([]string{}, query.profileWarnings...)
	warnings = append(warnings, fmt.Sprintf("read_model=%s", query.profileSource))
	if len(query.docs) == 0 {
		hint := fmt.Sprintf("documentation index is empty; rerun 'mi-lsp index --workspace %s --docs-only' before wiki search", registration.Name)
		warnings = appendStringIfMissing(warnings, hint)
		env := model.Envelope{
			Ok:        true,
			Workspace: registration.Name,
			Backend:   "wiki.search",
			Items:     []model.WikiSearchResult{},
			Warnings:  warnings,
			Hint:      hint,
		}
		return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
	}

	layerFilter, unknownLayers := parseWikiLayerFilter(stringPayload(request.Payload, "layer"))
	for _, layer := range unknownLayers {
		warnings = appendStringIfMissing(warnings, fmt.Sprintf("unknown wiki layer %q ignored; valid layers: RF, FL, TP, CT, TECH, DB", layer))
	}

	top := intFromAny(request.Payload["top"], 0)
	if top <= 0 {
		top = request.Context.MaxItems
	}
	if top <= 0 {
		top = 10
	}
	offset := intFromAny(request.Payload["offset"], request.Context.Offset)
	includeContent, _ := request.Payload["include_content"].(bool)

	candidates := make([]model.WikiSearchResult, 0, min(top, len(query.ranked)))
	skipped := 0
	for _, candidate := range query.ranked {
		layer := wikiLayerForDoc(candidate.record)
		if len(layerFilter) > 0 {
			if _, ok := layerFilter[layer]; !ok {
				continue
			}
		}
		if skipped < offset {
			skipped++
			continue
		}
		item := wikiSearchResult(registration.Name, registration.Root, queryText, candidate, layer, includeContent, request.Context.MaxChars)
		candidates = append(candidates, item)
		if len(candidates) >= top {
			break
		}
	}

	hint := ""
	if len(candidates) == 0 {
		if len(layerFilter) > 0 {
			hint = "0 wiki matches for selected layers; broaden --layer or try nav wiki route"
		} else {
			hint = "0 wiki matches; try a doc id like RF-*, FL-*, CT-*, TECH-*, DB-* or run nav wiki route"
		}
	}

	env := model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "wiki.search",
		Items:     candidates,
		Warnings:  warnings,
		Hint:      hint,
		Stats:     model.Stats{Files: len(candidates)},
	}
	return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
}

func wikiSearchResult(workspaceName string, root string, queryText string, candidate scoredDoc, layer string, includeContent bool, maxChars int) model.WikiSearchResult {
	doc := candidate.record
	item := model.WikiSearchResult{
		DocID:       doc.DocID,
		Path:        doc.Path,
		Title:       doc.Title,
		Layer:       layer,
		Family:      doc.Family,
		Stage:       wikiStageForDoc(doc),
		Score:       candidate.score,
		Why:         append([]string{}, candidate.reason...),
		Snippet:     doc.Snippet,
		NextQueries: buildWikiSearchNextQueries(workspaceName, queryText, doc, layer),
	}
	if includeContent {
		item.Content = readWikiSearchContent(root, doc.Path, maxChars)
	}
	return item
}

func parseWikiLayerFilter(raw string) (map[string]struct{}, []string) {
	filter := map[string]struct{}{}
	unknown := []string{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}) {
		layer := strings.ToUpper(strings.TrimSpace(part))
		if layer == "" {
			continue
		}
		switch layer {
		case "RF", "FL", "TP", "CT", "TECH", "DB":
			filter[layer] = struct{}{}
		default:
			unknown = append(unknown, layer)
		}
	}
	if len(filter) == 0 {
		return nil, unknown
	}
	return filter, unknown
}

func wikiLayerForDoc(doc model.DocRecord) string {
	docID := strings.ToUpper(strings.TrimSpace(doc.DocID))
	path := filepath.ToSlash(doc.Path)
	switch {
	case strings.HasPrefix(docID, "FL-") || doc.Layer == "03" || strings.Contains(path, "/03_FL/"):
		return "FL"
	case strings.HasPrefix(docID, "RF-") || doc.Layer == "04" || strings.Contains(path, "/04_RF/"):
		return "RF"
	case strings.HasPrefix(docID, "TP-") || doc.Layer == "06" || strings.Contains(path, "/06_pruebas/"):
		return "TP"
	case strings.HasPrefix(docID, "CT-") || doc.Layer == "09" || strings.Contains(path, "/09_contratos/"):
		return "CT"
	case strings.HasPrefix(docID, "TECH-") || doc.Layer == "07" || strings.Contains(path, "/07_tech/"):
		return "TECH"
	case strings.HasPrefix(docID, "DB-") || doc.Layer == "08" || strings.Contains(path, "/08_db/"):
		return "DB"
	default:
		return strings.ToUpper(strings.TrimSpace(doc.Layer))
	}
}

func wikiStageForDoc(doc model.DocRecord) string {
	path := filepath.ToSlash(doc.Path)
	switch {
	case strings.Contains(path, "/03_FL/") || doc.Layer == "03":
		return "flow"
	case strings.Contains(path, "/04_RF/") || doc.Layer == "04":
		return "requirements"
	case strings.Contains(path, "/06_pruebas/") || doc.Layer == "06":
		return "tests"
	case strings.Contains(path, "/07_tech/"):
		return "technical_detail"
	case doc.Layer == "07":
		return "technical_baseline"
	case strings.Contains(path, "/08_db/") || doc.Layer == "08":
		return "physical_data"
	case strings.Contains(path, "/09_contratos/") || doc.Layer == "09":
		return "contracts"
	case doc.Layer == "01":
		return "scope"
	case doc.Layer == "02":
		return "architecture"
	default:
		return ""
	}
}

func buildWikiSearchNextQueries(workspaceName string, queryText string, doc model.DocRecord, layer string) []string {
	queries := make([]string, 0, 4)
	queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav wiki pack %q --workspace %s --doc %s --format toon", queryText, workspaceName, doc.Path))
	if layer == "RF" && strings.HasPrefix(strings.ToUpper(doc.DocID), "RF-") {
		queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav wiki trace %s --workspace %s --format toon", doc.DocID, workspaceName))
	}
	if doc.DocID != "" {
		queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav wiki search %q --workspace %s --format toon", doc.DocID, workspaceName))
	}
	if doc.Path != "" {
		queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav multi-read %s:1-120 --workspace %s --format toon", doc.Path, workspaceName))
	}
	queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav ask %q --workspace %s --format toon", queryText, workspaceName))
	if len(queries) > 4 {
		return queries[:4]
	}
	return queries
}

func readWikiSearchContent(root string, relPath string, maxChars int) string {
	relPath = filepath.ToSlash(strings.TrimSpace(relPath))
	if relPath == "" || strings.Contains(relPath, "..") {
		return ""
	}
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relPath)))
	if err != nil {
		return ""
	}
	limit := 4096
	if maxChars > 0 && maxChars < limit {
		limit = maxChars
	}
	text := string(content)
	if len(text) > limit {
		return text[:limit]
	}
	return text
}
