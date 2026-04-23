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

	docID, _ := request.Payload["rf"].(string)
	allRFs, _ := request.Payload["all"].(bool)
	summary, _ := request.Payload["summary"].(bool)

	if docID != "" {
		result, err := a.traceRF(ctx, registration.Root, db, docID)
		if err != nil {
			return model.Envelope{}, err
		}
		if result == nil {
			return model.Envelope{
				Ok:        true,
				Workspace: registration.Name,
				Backend:   "trace",
				Items:     []model.TraceResult{},
				Warnings:  []string{fmt.Sprintf("Trace target %q not found in doc index", docID)},
			}, nil
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
	traceID := strings.ToUpper(strings.TrimSpace(rfID))
	if traceID == "" {
		return nil, nil
	}

	mentioningDocs, err := store.FindDocRecordsByMention(ctx, db, "doc_id", traceID)
	if err != nil {
		return nil, err
	}

	doc, err := resolveTraceDoc(ctx, root, db, traceID, mentioningDocs)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, nil
	}

	implValues, err := store.GetMentionsByType(ctx, db, doc.Path, "implements")
	if err != nil {
		return nil, err
	}
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
			syms, _ := store.SymbolsByFile(ctx, db, file, 1, 0)
			link.Verified = len(syms) > 0 || traceFileExists(root, file)
			link.Kind = "file"
		}
		explicit = append(explicit, link)
	}

	tests := make([]model.TraceLink, 0, len(testValues)+len(mentioningDocs))
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
	tests = append(tests, traceDocEvidenceTests(root, mentioningDocs)...)
	tests = dedupeTraceLinks(tests)

	inferred := make([]model.TraceLink, 0)
	if len(explicit) == 0 && !isTPDocID(traceID) {
		inferred = a.inferTraceLinks(ctx, db, doc)
	}

	status, coverage := computeTraceStatus(explicit, inferred, tests)

	return &model.TraceResult{
		RF:       doc.DocID,
		Title:    doc.Title,
		Status:   status,
		Coverage: coverage,
		Explicit: explicit,
		Inferred: inferred,
		Tests:    tests,
		Drift:    []model.TraceDrift{},
	}, nil
}

func resolveTraceDoc(ctx context.Context, root string, db *sql.DB, traceID string, mentioningDocs []model.DocRecord) (*model.DocRecord, error) {
	if isTPDocID(traceID) {
		return resolveTraceTPDoc(root, traceID, mentioningDocs), nil
	}
	return resolveTraceRFDoc(ctx, root, db, traceID, mentioningDocs)
}

func resolveTraceRFDoc(ctx context.Context, root string, db *sql.DB, traceID string, mentioningDocs []model.DocRecord) (*model.DocRecord, error) {
	rfDocs, err := store.GetRFDocRecords(ctx, db)
	if err != nil {
		return nil, err
	}

	var doc *model.DocRecord
	for i := range rfDocs {
		if !isSpecificRFDocPath(rfDocs[i].Path) {
			continue
		}
		if strings.EqualFold(rfDocs[i].DocID, traceID) {
			doc = &rfDocs[i]
			break
		}
	}
	if doc == nil {
		for i := range rfDocs {
			if isRFIndexPath(rfDocs[i].Path) {
				continue
			}
			if strings.EqualFold(rfDocs[i].DocID, traceID) {
				doc = &rfDocs[i]
				break
			}
		}
	}
	if doc == nil {
		for i := range mentioningDocs {
			if isSpecificRFDocPath(mentioningDocs[i].Path) {
				doc = &mentioningDocs[i]
				break
			}
		}
	}
	if doc == nil {
		for i := range mentioningDocs {
			if mentioningDocs[i].Layer == "04" && !isRFIndexPath(mentioningDocs[i].Path) {
				doc = &mentioningDocs[i]
				break
			}
		}
	}
	if doc == nil && len(mentioningDocs) > 0 {
		doc = &mentioningDocs[0]
	}
	if doc == nil {
		fallbackDoc, ok := traceRFDocFromDisk(root, traceID)
		if !ok {
			return nil, nil
		}
		doc = &fallbackDoc
	}

	virtualDoc := *doc
	virtualDoc.DocID = traceID
	if title := embeddedDocIDTitle(root, virtualDoc.Path, traceID); title != "" {
		virtualDoc.Title = title
	}
	return &virtualDoc, nil
}

func resolveTraceTPDoc(root string, traceID string, mentioningDocs []model.DocRecord) *model.DocRecord {
	var doc *model.DocRecord
	for i := range mentioningDocs {
		if isSpecificTPDocPath(mentioningDocs[i].Path) {
			doc = &mentioningDocs[i]
			break
		}
	}
	if doc == nil {
		for i := range mentioningDocs {
			if isTPDocRecord(mentioningDocs[i]) {
				doc = &mentioningDocs[i]
				break
			}
		}
	}
	if doc == nil {
		fallbackDoc, ok := traceTPDocFromDisk(root, traceID)
		if !ok {
			return nil
		}
		doc = &fallbackDoc
	}

	virtualDoc := *doc
	virtualDoc.DocID = traceID
	if title := embeddedDocIDTitle(root, virtualDoc.Path, traceID); title != "" {
		virtualDoc.Title = title
	}
	return &virtualDoc
}

func traceRFDocFromDisk(root string, rfID string) (model.DocRecord, bool) {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(rfID) == "" {
		return model.DocRecord{}, false
	}
	candidates := traceGovernedDocCandidates(root, "functional")
	indexPath := filepath.Join(root, ".docs", "wiki", "04_RF.md")
	if _, err := os.Stat(indexPath); err == nil {
		candidates = appendUniqueTraceCandidate(candidates, filepath.ToSlash(filepath.Clean(".docs/wiki/04_RF.md")))
	}
	rfDir := filepath.Join(root, ".docs", "wiki", "04_RF")
	entries, err := os.ReadDir(rfDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
				continue
			}
			candidates = appendUniqueTraceCandidate(candidates, filepath.ToSlash(filepath.Join(".docs", "wiki", "04_RF", entry.Name())))
		}
	}
	legacyIndexPath := filepath.Join(root, ".docs", "wiki", "RF.md")
	if _, err := os.Stat(legacyIndexPath); err == nil {
		candidates = appendUniqueTraceCandidate(candidates, filepath.ToSlash(filepath.Clean(".docs/wiki/RF.md")))
	}
	legacyDir := filepath.Join(root, ".docs", "wiki", "RF")
	legacyEntries, err := os.ReadDir(legacyDir)
	if err == nil {
		for _, entry := range legacyEntries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
				continue
			}
			candidates = appendUniqueTraceCandidate(candidates, filepath.ToSlash(filepath.Join(".docs", "wiki", "RF", entry.Name())))
		}
	}
	best := model.DocRecord{}
	bestScore := -1
	for _, relativePath := range candidates {
		absolutePath := filepath.Join(root, filepath.FromSlash(relativePath))
		content, err := os.ReadFile(absolutePath)
		if err != nil {
			continue
		}
		if !strings.Contains(strings.ToUpper(string(content)), strings.ToUpper(rfID)) {
			continue
		}
		score := 1
		if isSpecificRFDocPath(relativePath) {
			score += 10
		}
		if !isRFIndexPath(relativePath) {
			score += 5
		}
		title := embeddedDocIDTitle(root, relativePath, rfID)
		if title == "" {
			title = rfID
		}
		if score > bestScore {
			bestScore = score
			best = model.DocRecord{
				Path:   relativePath,
				Title:  title,
				DocID:  strings.ToUpper(rfID),
				Layer:  "04",
				Family: "functional",
			}
		}
	}
	return best, bestScore >= 0
}

func traceTPDocFromDisk(root string, traceID string) (model.DocRecord, bool) {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(traceID) == "" {
		return model.DocRecord{}, false
	}
	candidates := traceGovernedDocCandidates(root, "functional")
	indexPath := filepath.Join(root, ".docs", "wiki", "06_matriz_pruebas_RF.md")
	if _, err := os.Stat(indexPath); err == nil {
		candidates = appendUniqueTraceCandidate(candidates, filepath.ToSlash(filepath.Clean(".docs/wiki/06_matriz_pruebas_RF.md")))
	}
	tpDir := filepath.Join(root, ".docs", "wiki", "06_pruebas")
	entries, err := os.ReadDir(tpDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
				continue
			}
			candidates = appendUniqueTraceCandidate(candidates, filepath.ToSlash(filepath.Join(".docs", "wiki", "06_pruebas", entry.Name())))
		}
	}
	legacyIndexPath := filepath.Join(root, ".docs", "wiki", "TP.md")
	if _, err := os.Stat(legacyIndexPath); err == nil {
		candidates = appendUniqueTraceCandidate(candidates, filepath.ToSlash(filepath.Clean(".docs/wiki/TP.md")))
	}
	legacyDir := filepath.Join(root, ".docs", "wiki", "TP")
	legacyEntries, err := os.ReadDir(legacyDir)
	if err == nil {
		for _, entry := range legacyEntries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
				continue
			}
			candidates = appendUniqueTraceCandidate(candidates, filepath.ToSlash(filepath.Join(".docs", "wiki", "TP", entry.Name())))
		}
	}
	best := model.DocRecord{}
	bestScore := -1
	for _, relativePath := range candidates {
		absolutePath := filepath.Join(root, filepath.FromSlash(relativePath))
		content, err := os.ReadFile(absolutePath)
		if err != nil {
			continue
		}
		if !strings.Contains(strings.ToUpper(string(content)), strings.ToUpper(traceID)) {
			continue
		}
		score := 1
		if isSpecificTPDocPath(relativePath) {
			score += 10
		}
		if !isTPIndexPath(relativePath) {
			score += 5
		}
		title := embeddedDocIDTitle(root, relativePath, traceID)
		if title == "" {
			title = traceID
		}
		if score > bestScore {
			bestScore = score
			best = model.DocRecord{
				Path:   relativePath,
				Title:  title,
				DocID:  strings.ToUpper(traceID),
				Layer:  "06",
				Family: "functional",
			}
		}
	}
	return best, bestScore >= 0
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
	return strings.Contains(normalized, "/04_RF/") || strings.Contains(normalized, "/RF/")
}

func isRFIndexPath(path string) bool {
	normalized := filepath.ToSlash(path)
	return normalized == ".docs/wiki/04_RF.md" ||
		normalized == ".docs/wiki/RF.md" ||
		strings.HasSuffix(normalized, "/04_RF.md") ||
		strings.HasSuffix(normalized, "/RF.md")
}

func isSpecificTPDocPath(path string) bool {
	normalized := filepath.ToSlash(path)
	return strings.Contains(normalized, "/06_pruebas/") || strings.Contains(normalized, "/TP/")
}

func isTPIndexPath(path string) bool {
	normalized := filepath.ToSlash(path)
	return normalized == ".docs/wiki/06_matriz_pruebas_RF.md" ||
		normalized == ".docs/wiki/TP.md" ||
		strings.HasSuffix(normalized, "/06_matriz_pruebas_RF.md") ||
		strings.HasSuffix(normalized, "/TP.md")
}

func isTPDocID(docID string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(docID)), "TP-")
}

func isTPDocRecord(doc model.DocRecord) bool {
	return isTPDocID(doc.DocID) || doc.Layer == "06" || isSpecificTPDocPath(doc.Path) || isTPIndexPath(doc.Path)
}

func embeddedDocIDTitle(root string, relativePath string, docID string) string {
	if root == "" || relativePath == "" || docID == "" {
		return ""
	}
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relativePath)))
	if err != nil {
		return ""
	}
	docIDUpper := strings.ToUpper(docID)
	lines := strings.Split(string(content), "\n")
	for idx, line := range lines {
		if !strings.Contains(strings.ToUpper(line), docIDUpper) {
			continue
		}
		if title := tableRowTitle(lines, idx, docID); title != "" {
			return title
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			return strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
	}
	return ""
}

func tableRowTitle(lines []string, rowIdx int, docID string) string {
	if rowIdx < 0 || rowIdx >= len(lines) {
		return ""
	}
	values := markdownTableValues(lines[rowIdx])
	if len(values) == 0 {
		return ""
	}

	headers := nearestMarkdownTableHeader(lines, rowIdx)
	if len(headers) == len(values) {
		for idx, header := range headers {
			switch normalizeTableHeader(header) {
			case "titulo", "title", "descripcion", "description", "objetivo", "objective":
				if value := strings.TrimSpace(values[idx]); value != "" {
					return value
				}
			}
		}
	}

	return rfTableTitle(lines[rowIdx], docID)
}

func markdownTableValues(line string) []string {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") {
		return nil
	}
	cells := strings.Split(trimmed, "|")
	values := make([]string, 0, len(cells))
	for _, cell := range cells {
		value := strings.TrimSpace(cell)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func nearestMarkdownTableHeader(lines []string, rowIdx int) []string {
	seenSeparator := false
	for idx := rowIdx - 1; idx >= 0; idx-- {
		values := markdownTableValues(lines[idx])
		if len(values) == 0 {
			if seenSeparator && strings.TrimSpace(lines[idx]) == "" {
				break
			}
			continue
		}
		if isMarkdownTableSeparator(values) {
			seenSeparator = true
			continue
		}
		if seenSeparator {
			return values
		}
	}
	return nil
}

func isMarkdownTableSeparator(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return false
		}
		for _, r := range trimmed {
			if r != '-' && r != ':' {
				return false
			}
		}
	}
	return true
}

func normalizeTableHeader(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("`", "", "*", "", "_", "", "-", "", " ", "").Replace(value)
	return value
}

func rfTableTitle(line string, rfID string) string {
	values := markdownTableValues(line)
	if len(values) == 0 {
		return ""
	}
	for i, value := range values {
		if strings.EqualFold(value, rfID) && i+1 < len(values) {
			return values[i+1]
		}
	}
	return ""
}

func traceDocEvidenceTests(root string, mentioningDocs []model.DocRecord) []model.TraceLink {
	tests := make([]model.TraceLink, 0, len(mentioningDocs))
	for _, doc := range mentioningDocs {
		if !isTPDocRecord(doc) {
			continue
		}
		tests = append(tests, model.TraceLink{
			File:     doc.Path,
			Kind:     "test",
			Source:   "wiki-doc",
			Verified: traceFileExists(root, doc.Path),
		})
	}
	return tests
}

func dedupeTraceLinks(links []model.TraceLink) []model.TraceLink {
	if len(links) <= 1 {
		return links
	}
	seen := make(map[string]struct{}, len(links))
	deduped := make([]model.TraceLink, 0, len(links))
	for _, link := range links {
		key := link.File + "::" + link.Symbol + "::" + link.Kind + "::" + link.Source
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, link)
	}
	return deduped
}

func traceGovernedDocCandidates(root string, family string) []string {
	profile, _, _ := docgraph.LoadProfile(root)
	candidates := make([]string, 0)
	for _, docFamily := range profile.Families {
		if docFamily.Name != family {
			continue
		}
		for _, pattern := range docFamily.Paths {
			for _, candidate := range traceExpandPattern(root, pattern) {
				candidates = appendUniqueTraceCandidate(candidates, candidate)
			}
		}
	}
	return candidates
}

func traceExpandPattern(root string, pattern string) []string {
	trimmed := filepath.ToSlash(strings.TrimSpace(pattern))
	if trimmed == "" {
		return nil
	}
	if strings.HasSuffix(trimmed, "/") {
		dir := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(trimmed, "/")))
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		results := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
				continue
			}
			results = append(results, filepath.ToSlash(filepath.Join(strings.TrimSuffix(trimmed, "/"), entry.Name())))
		}
		return results
	}
	if strings.ContainsAny(trimmed, "*?[") {
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(trimmed)))
		if err != nil {
			return nil
		}
		results := make([]string, 0, len(matches))
		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil || info.IsDir() {
				continue
			}
			rel, err := filepath.Rel(root, match)
			if err != nil {
				continue
			}
			results = append(results, filepath.ToSlash(rel))
		}
		return results
	}
	if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(trimmed))); err == nil && !info.IsDir() {
		return []string{trimmed}
	}
	return nil
}

func appendUniqueTraceCandidate(candidates []string, candidate string) []string {
	candidate = filepath.ToSlash(strings.TrimSpace(candidate))
	if candidate == "" {
		return candidates
	}
	for _, existing := range candidates {
		if existing == candidate {
			return candidates
		}
	}
	return append(candidates, candidate)
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
	score := 0.4
	nameLower := strings.ToLower(sym.Name)
	keyLower := strings.ToLower(keyword)

	if nameLower == keyLower {
		score = 0.9
	} else if strings.Contains(nameLower, keyLower) {
		score = 0.7
	}

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

func computeTraceStatus(explicit []model.TraceLink, inferred []model.TraceLink, tests []model.TraceLink) (string, float64) {
	verifiedTests := 0
	for _, link := range tests {
		if link.Verified {
			verifiedTests++
		}
	}

	if len(explicit) == 0 && len(inferred) == 0 {
		if verifiedTests > 0 {
			return "partial", 0.5
		}
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
		if verifiedTests > 0 {
			return "partial", 0.5
		}
		return "missing", 0.0
	}

	return "partial", 0.5
}

func parseImplementsRef(ref string) (file string, symbol string) {
	ref = strings.TrimSpace(ref)
	if idx := strings.LastIndex(ref, ":"); idx > 0 {
		if idx > 1 {
			return ref[:idx], ref[idx+1:]
		}
	}
	return ref, ""
}
