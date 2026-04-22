package service

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

func (a *App) trace(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}
	db, err := store.Open(registration.Root)
	if err != nil {
		return model.Envelope{}, err
	}
	defer db.Close()

	rfID, _ := request.Payload["rf"].(string)
	allRFs, _ := request.Payload["all"].(bool)
	summary, _ := request.Payload["summary"].(bool)

	if rfID != "" {
		result, err := a.traceRF(ctx, registration.Root, db, rfID)
		if err != nil {
			return model.Envelope{}, err
		}
		if result == nil {
			return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "trace", Items: []model.TraceResult{}, Warnings: []string{fmt.Sprintf("RF %q not found in doc index", rfID)}}, nil
		}
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "trace", Items: []model.TraceResult{*result}}, nil
	}

	if allRFs {
		results, err := a.traceAllRFs(ctx, registration.Root, db)
		if err != nil {
			return model.Envelope{}, err
		}
		if summary {
			return a.traceSummary(registration.Name, results), nil
		}
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "trace", Items: results, Stats: model.Stats{Files: len(results)}}, nil
	}

	return model.Envelope{}, fmt.Errorf("rf ID required or use --all")
}

func (a *App) traceRF(ctx context.Context, root string, db *sql.DB, rfID string) (*model.TraceResult, error) {
	// Find the RF doc
	rfDocs, err := store.GetRFDocRecords(ctx, db)
	if err != nil {
		return nil, err
	}

	var doc *model.DocRecord
	rfIDUpper := strings.ToUpper(rfID)
	for i := range rfDocs {
		if !isSpecificRFDocPath(rfDocs[i].Path) {
			continue
		}
		if strings.EqualFold(rfDocs[i].DocID, rfIDUpper) || strings.EqualFold(rfDocs[i].DocID, rfID) {
			doc = &rfDocs[i]
			break
		}
	}
	if doc == nil {
		for i := range rfDocs {
			if isRFIndexPath(rfDocs[i].Path) {
				continue
			}
			if strings.EqualFold(rfDocs[i].DocID, rfIDUpper) || strings.EqualFold(rfDocs[i].DocID, rfID) {
				doc = &rfDocs[i]
				break
			}
		}
	}
	if doc == nil {
		embeddedDocs, err := store.FindDocRecordsByMention(ctx, db, "doc_id", rfIDUpper)
		if err != nil {
			return nil, err
		}
		for i := range embeddedDocs {
			if isSpecificRFDocPath(embeddedDocs[i].Path) {
				doc = &embeddedDocs[i]
				break
			}
		}
		if doc == nil {
			for i := range embeddedDocs {
				if embeddedDocs[i].Layer == "04" && !isRFIndexPath(embeddedDocs[i].Path) {
					doc = &embeddedDocs[i]
					break
				}
			}
		}
		if doc == nil && len(embeddedDocs) > 0 {
			doc = &embeddedDocs[0]
		}
		if doc == nil {
			return nil, nil
		}
		virtualDoc := *doc
		virtualDoc.DocID = rfIDUpper
		if title := embeddedRFTitle(root, virtualDoc.Path, rfIDUpper); title != "" {
			virtualDoc.Title = title
		}
		doc = &virtualDoc
	}

	// Get explicit implements links
	implValues, err := store.GetMentionsByType(ctx, db, doc.Path, "implements")
	if err != nil {
		return nil, err
	}

	// Get explicit test links
	testValues, err := store.GetMentionsByType(ctx, db, doc.Path, "test_file")
	if err != nil {
		return nil, err
	}

	explicit := make([]model.TraceLink, 0, len(implValues))
	for _, impl := range implValues {
		file, symbol := parseImplementsRef(impl)
		link := model.TraceLink{
			File:   file,
			Symbol: symbol,
			Source: "wiki-marker",
		}
		if symbol != "" {
			_, found, _ := store.VerifySymbolExists(ctx, db, file, symbol)
			link.Verified = found
			if found {
				link.Kind = "symbol"
			}
		} else {
			// Prefer the symbol catalog, then fall back to workspace file existence for
			// file-only links in repos whose implementation language is not indexed.
			syms, _ := store.SymbolsByFile(ctx, db, file, 1, 0)
			link.Verified = len(syms) > 0 || traceFileExists(root, file)
			link.Kind = "file"
		}
		explicit = append(explicit, link)
	}

	tests := make([]model.TraceLink, 0, len(testValues))
	for _, testFile := range testValues {
		link := model.TraceLink{
			File:   testFile,
			Source: "wiki-marker",
			Kind:   "test",
		}
		syms, _ := store.SymbolsByFile(ctx, db, testFile, 1, 0)
		link.Verified = len(syms) > 0 || traceFileExists(root, testFile)
		tests = append(tests, link)
	}

	// Heuristic: if no explicit links, infer from RF title/content
	inferred := make([]model.TraceLink, 0)
	if len(explicit) == 0 {
		inferred = a.inferTraceLinks(ctx, db, doc)
	}

	// Calculate status and coverage
	status, coverage := computeTraceStatus(explicit, inferred)

	return &model.TraceResult{
		RF:       doc.DocID,
		Title:    doc.Title,
		Status:   status,
		Coverage: coverage,
		Explicit: explicit,
		Inferred: inferred,
		Tests:    tests,
		Drift:    []model.TraceDrift{}, // v2 stub
	}, nil
}

func (a *App) traceAllRFs(ctx context.Context, root string, db *sql.DB) ([]model.TraceResult, error) {
	rfDocs, err := store.GetRFDocRecords(ctx, db)
	if err != nil {
		return nil, err
	}
	results := make([]model.TraceResult, 0, len(rfDocs))
	for _, doc := range rfDocs {
		if doc.DocID == "" {
			continue
		}
		result, err := a.traceRF(ctx, root, db, doc.DocID)
		if err != nil {
			continue
		}
		if result != nil {
			results = append(results, *result)
		}
	}
	return results, nil
}

func traceFileExists(root string, file string) bool {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(file) == "" {
		return false
	}
	clean := filepath.Clean(filepath.FromSlash(file))
	if filepath.IsAbs(clean) {
		return false
	}
	path := filepath.Join(root, clean)
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (a *App) traceSummary(workspaceName string, results []model.TraceResult) model.Envelope {
	items := make([]map[string]any, 0, len(results))
	for _, r := range results {
		items = append(items, map[string]any{
			"rf":       r.RF,
			"title":    r.Title,
			"status":   r.Status,
			"coverage": fmt.Sprintf("%.2f", r.Coverage),
			"explicit": len(r.Explicit),
			"inferred": len(r.Inferred),
			"tests":    len(r.Tests),
		})
	}
	return model.Envelope{Ok: true, Workspace: workspaceName, Backend: "trace", Items: items, Stats: model.Stats{Files: len(results)}}
}

func isSpecificRFDocPath(path string) bool {
	normalized := filepath.ToSlash(path)
	return strings.Contains(normalized, "/04_RF/")
}

func isRFIndexPath(path string) bool {
	normalized := filepath.ToSlash(path)
	return normalized == ".docs/wiki/04_RF.md" || strings.HasSuffix(normalized, "/04_RF.md")
}

func embeddedRFTitle(root string, relativePath string, rfID string) string {
	if root == "" || relativePath == "" || rfID == "" {
		return ""
	}
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relativePath)))
	if err != nil {
		return ""
	}
	rfIDUpper := strings.ToUpper(rfID)
	for _, line := range strings.Split(string(content), "\n") {
		if !strings.Contains(strings.ToUpper(line), rfIDUpper) {
			continue
		}
		if title := rfTableTitle(line, rfID); title != "" {
			return title
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			return strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
	}
	return ""
}

func rfTableTitle(line string, rfID string) string {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") {
		return ""
	}
	cells := strings.Split(trimmed, "|")
	values := make([]string, 0, len(cells))
	for _, cell := range cells {
		value := strings.TrimSpace(cell)
		if value != "" {
			values = append(values, value)
		}
	}
	for i, value := range values {
		if strings.EqualFold(value, rfID) && i+1 < len(values) {
			return values[i+1]
		}
	}
	return ""
}

func (a *App) inferTraceLinks(ctx context.Context, db *sql.DB, doc *model.DocRecord) []model.TraceLink {
	keywords := docgraph.QuestionTokens(doc.Title + " " + doc.DocID)
	if len(keywords) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	var links []model.TraceLink

	for _, keyword := range keywords {
		if len(links) >= 5 {
			break
		}
		symbols, err := store.FindSymbols(ctx, db, keyword, "", false, 3, 0)
		if err != nil {
			continue
		}
		for _, sym := range symbols {
			key := sym.FilePath + ":" + sym.Name
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}

			confidence := computeConfidence(keyword, sym)
			if confidence < 0.3 {
				continue
			}

			links = append(links, model.TraceLink{
				File:       sym.FilePath,
				Symbol:     sym.Name,
				Kind:       sym.Kind,
				Source:     "heuristic",
				Verified:   true,
				Confidence: confidence,
			})
			if len(links) >= 5 {
				break
			}
		}
	}
	return links
}

func computeConfidence(keyword string, sym model.SymbolRecord) float64 {
	score := 0.4 // base for partial match
	nameLower := strings.ToLower(sym.Name)
	keyLower := strings.ToLower(keyword)

	if nameLower == keyLower {
		score = 0.9
	} else if strings.Contains(nameLower, keyLower) {
		score = 0.7
	}

	// Boost if the symbol kind suggests an implementation
	switch sym.Kind {
	case "method", "function":
		score += 0.05
	case "class", "interface":
		score += 0.03
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

func computeTraceStatus(explicit []model.TraceLink, inferred []model.TraceLink) (string, float64) {
	if len(explicit) == 0 && len(inferred) == 0 {
		return "missing", 0.0
	}

	if len(explicit) > 0 {
		verified := 0
		for _, link := range explicit {
			if link.Verified {
				verified++
			}
		}
		coverage := float64(verified) / float64(len(explicit))
		if coverage >= 1.0 {
			return "implemented", 1.0
		}
		if coverage > 0 {
			return "partial", coverage
		}
		return "missing", 0.0
	}

	// Only inferred: partial by definition
	return "partial", 0.5
}

func parseImplementsRef(ref string) (file string, symbol string) {
	ref = strings.TrimSpace(ref)
	if idx := strings.LastIndex(ref, ":"); idx > 0 {
		// Check it's not a Windows drive letter (e.g., "C:")
		if idx > 1 {
			return ref[:idx], ref[idx+1:]
		}
	}
	return ref, ""
}
