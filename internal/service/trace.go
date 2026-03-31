package service

import (
	"context"
	"database/sql"
	"fmt"
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
		result, err := a.traceRF(ctx, db, rfID)
		if err != nil {
			return model.Envelope{}, err
		}
		if result == nil {
			return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "trace", Items: []model.TraceResult{}, Warnings: []string{fmt.Sprintf("RF %q not found in doc index", rfID)}}, nil
		}
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "trace", Items: []model.TraceResult{*result}}, nil
	}

	if allRFs {
		results, err := a.traceAllRFs(ctx, db)
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

func (a *App) traceRF(ctx context.Context, db *sql.DB, rfID string) (*model.TraceResult, error) {
	// Find the RF doc
	rfDocs, err := store.GetRFDocRecords(ctx, db)
	if err != nil {
		return nil, err
	}

	var doc *model.DocRecord
	rfIDUpper := strings.ToUpper(rfID)
	for i := range rfDocs {
		if strings.EqualFold(rfDocs[i].DocID, rfIDUpper) || strings.EqualFold(rfDocs[i].DocID, rfID) {
			doc = &rfDocs[i]
			break
		}
	}
	if doc == nil {
		return nil, nil
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
			// Just check if file exists in index
			syms, _ := store.SymbolsByFile(ctx, db, file, 1)
			link.Verified = len(syms) > 0
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
		syms, _ := store.SymbolsByFile(ctx, db, testFile, 1)
		link.Verified = len(syms) > 0
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

func (a *App) traceAllRFs(ctx context.Context, db *sql.DB) ([]model.TraceResult, error) {
	rfDocs, err := store.GetRFDocRecords(ctx, db)
	if err != nil {
		return nil, err
	}
	results := make([]model.TraceResult, 0, len(rfDocs))
	for _, doc := range rfDocs {
		if doc.DocID == "" {
			continue
		}
		result, err := a.traceRF(ctx, db, doc.DocID)
		if err != nil {
			continue
		}
		if result != nil {
			results = append(results, *result)
		}
	}
	return results, nil
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
		symbols, err := store.FindSymbols(ctx, db, keyword, "", false, 3)
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
