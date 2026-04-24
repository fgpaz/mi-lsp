package service

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

type packStage struct {
	Name     string
	Required bool
	Match    func(model.DocRecord) bool
}

type packAnchor struct {
	DocPath string
	DocID   string
}

func (a *App) pack(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	if blockedEnv, err := a.governanceGateEnvelope(ctx, request, "nav.pack"); err != nil {
		return model.Envelope{}, err
	} else if blockedEnv != nil {
		return *blockedEnv, nil
	}

	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}
	memory, _ := loadReentryMemory(ctx, registration.Root)

	task, _ := request.Payload["task"].(string)
	task = strings.TrimSpace(task)
	if task == "" {
		task, _ = request.Payload["question"].(string)
		task = strings.TrimSpace(task)
	}
	if task == "" {
		return model.Envelope{}, fmt.Errorf("task is required")
	}

	query := loadDocQueryContext(ctx, registration, task)
	defer query.Close()
	if query.dbErr != nil {
		return model.Envelope{}, query.dbErr
	}
	docs := query.docs
	profile := query.profile
	profileSource := query.profileSource
	profileWarnings := query.profileWarnings
	mode := "preview"
	if request.Context.Full {
		mode = "full"
	}
	result := model.PackResult{
		Task:   task,
		Mode:   mode,
		Why:    []string{fmt.Sprintf("read_model=%s", profileSource)},
		Docs:   []model.PackDoc{},
		Family: "technical",
	}

	warnings := append([]string{}, profileWarnings...)
	if canonicalWikiExists(registration.Root) && !hasIndexedCanonicalDocs(docs) {
		// Use Tier 1 canonical routing to produce a governed pack preview
		// instead of stalling with empty docs (RF-QRY-015).
		canonical, tier1Why := docgraph.Tier1CanonicalRoute(task, profile, registration.Root)
		tier1Docs := routeCanonicalToPackDocs(canonical)
		if len(tier1Docs) > 0 {
			result.Docs = tier1Docs
			result.PrimaryDoc = canonical.AnchorDoc.Path
			result.Why = append(result.Why, tier1Why...)
			result.Why = append(result.Why, "tier1=canonical_fallback")
		}
		hint := fmt.Sprintf("documentation index is empty; route resolved from governance. Rerun mi-lsp index --workspace %s for full pack", registration.Name)
		warnings = appendStringIfMissing(warnings, hint)
		env := model.Envelope{
			Ok:        true,
			Workspace: registration.Name,
			Backend:   "pack",
			Items:     []model.PackResult{result},
			Warnings:  warnings,
			Hint:      hint,
		}
		env = applyWikiRepoCompatHint(env, request, "nav.pack", registration.Name, task)
		env = attachMemoryPointer(env, memory)
		env.Continuation = buildPackContinuation(task, result, request.Context, memory)
		return applyCoachPolicy(env, request.Context), nil
	}

	// Route core backbone (RF-QRY-015): resolve canonical anchor for this task
	routeResult := query.canonicalRoute(request.Context, false)
	hardAnchor, family := resolvePackAnchor(request.Payload, task, docs, query.docByPath, profile)
	result.Family = family

	// Inject route core anchor when no explicit override is present (--rf/--fl/--doc always wins)
	if hardAnchor.DocPath == "" && hardAnchor.DocID == "" && routeResult.Canonical.AnchorDoc.Path != "" {
		hardAnchor.DocPath = routeResult.Canonical.AnchorDoc.Path
		result.Why = append(result.Why, "tier2=route_core")
	}

	primary, ok := selectPackPrimary(hardAnchor, docs, query.docByPath, query.ranked)
	if !ok {
		warnings = appendStringIfMissing(warnings, "no documentation pack candidates matched the task")
		env := model.Envelope{
			Ok:        true,
			Workspace: registration.Name,
			Backend:   "pack",
			Items:     []model.PackResult{result},
			Warnings:  warnings,
		}
		return applyWikiRepoCompatHint(env, request, "nav.pack", registration.Name, task), nil
	}
	result.PrimaryDoc = primary.Path
	result.Why = append(result.Why, "primary_doc="+primary.Path, "family="+family)

	if isAXIPreview(request.Context) {
		previewDocs := routeCanonicalToPackDocs(routeResult.Canonical)
		if len(previewDocs) > 0 {
			for i := range previewDocs {
				if doc, ok := query.docByPath[previewDocs[i].Path]; ok {
					targets, targetWarnings := packTargets(registration.Root, doc, task)
					previewDocs[i].Targets = targets
					warnings = append(warnings, targetWarnings...)
				}
			}
			result.Docs = previewDocs
			result.PrimaryDoc = routeResult.Canonical.AnchorDoc.Path
			result.Why = append(result.Why, "preview=route_core")
			result.NextQueries = buildPackNextQueries(registration.Name, task, request.Context.Full, result.Docs)
			env := model.Envelope{
				Ok:        true,
				Workspace: registration.Name,
				Backend:   "pack",
				Items:     []model.PackResult{result},
				Warnings:  warnings,
				Stats:     model.Stats{Files: len(result.Docs)},
			}
			env = applyWikiRepoCompatHint(env, request, "nav.pack", registration.Name, task)
			env = attachMemoryPointer(env, memory)
			env.Continuation = buildPackContinuation(task, result, request.Context, memory)
			return applyCoachPolicy(applyAXIPreviewHints(env, request.Context, "preview mode: rerun with --full for slices"), request.Context), nil
		}
	}

	packDocs, packWhy, packWarnings := buildReadingPack(ctx, query.db, registration.Root, task, family, primary, docs, query.docByPath, query.ranked, profile, request.Context.Full, effectivePackDocsLimit(request.Context, profile))
	result.Docs = packDocs
	result.Why = append(result.Why, packWhy...)
	result.NextQueries = buildPackNextQueries(registration.Name, task, request.Context.Full, result.Docs)
	warnings = append(warnings, packWarnings...)

	env := model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "pack",
		Items:     []model.PackResult{result},
		Warnings:  warnings,
		Stats:     model.Stats{Files: len(result.Docs)},
	}
	env = applyWikiRepoCompatHint(env, request, "nav.pack", registration.Name, task)
	env = attachMemoryPointer(env, memory)
	env.Continuation = buildPackContinuation(task, result, request.Context, memory)
	return applyCoachPolicy(applyAXIPreviewHints(env, request.Context, "preview mode: rerun with --full for slices"), request.Context), nil
}

func resolvePackAnchor(payload map[string]any, task string, docs []model.DocRecord, docByPath map[string]model.DocRecord, profile model.DocsReadProfile) (packAnchor, string) {
	anchor := packAnchor{}
	if docPath := strings.TrimSpace(stringPayload(payload, "doc")); docPath != "" {
		normalized := filepath.ToSlash(strings.TrimPrefix(docPath, "./"))
		if doc, ok := docByPath[normalized]; ok {
			anchor.DocPath = doc.Path
			if doc.Family != "" {
				return anchor, doc.Family
			}
		}
	}
	if rf := strings.TrimSpace(stringPayload(payload, "rf")); rf != "" {
		anchor.DocID = strings.ToUpper(rf)
		return anchor, "functional"
	}
	if fl := strings.TrimSpace(stringPayload(payload, "fl")); fl != "" {
		anchor.DocID = strings.ToUpper(fl)
		return anchor, "functional"
	}
	return anchor, docgraph.MatchFamily(task, profile)
}

func selectPackPrimary(anchor packAnchor, docs []model.DocRecord, docByPath map[string]model.DocRecord, ranked []scoredDoc) (model.DocRecord, bool) {
	if anchor.DocPath != "" {
		doc, ok := docByPath[anchor.DocPath]
		return doc, ok
	}
	if anchor.DocID != "" {
		for _, doc := range docs {
			if strings.EqualFold(doc.DocID, anchor.DocID) {
				return doc, true
			}
		}
	}
	for _, candidate := range ranked {
		if candidate.record.Family != "generic" {
			return candidate.record, true
		}
	}
	if len(ranked) > 0 {
		return ranked[0].record, true
	}
	return model.DocRecord{}, false
}

func buildReadingPack(
	ctx context.Context,
	db *sql.DB,
	root string,
	task string,
	family string,
	primary model.DocRecord,
	docs []model.DocRecord,
	docByPath map[string]model.DocRecord,
	ranked []scoredDoc,
	profile model.DocsReadProfile,
	full bool,
	maxDocs int,
) ([]model.PackDoc, []string, []string) {
	if maxDocs <= 0 {
		maxDocs = profile.ReadingPack.MaxDocs
	}
	if maxDocs <= 0 {
		maxDocs = 6
	}

	stageSpecs := packStagesForFamily(family, profile.ReadingPack)
	rankedByPath := make(map[string]scoredDoc, len(ranked))
	for _, item := range ranked {
		rankedByPath[item.record.Path] = item
	}
	linked := linkedDocsByStage(ctx, db, primary.Path, docByPath, stageSpecs)

	items := make([]model.PackDoc, 0, len(stageSpecs))
	reasons := []string{}
	warnings := []string{}
	seen := map[string]struct{}{}

	for _, stage := range stageSpecs {
		if len(items) >= maxDocs {
			break
		}
		candidates := filterDocsByStage(docs, stage)
		if len(candidates) == 0 {
			continue
		}
		chosen, why, ok := choosePackDocForStage(stage, candidates, primary, rankedByPath, linked)
		if !ok {
			continue
		}
		if _, exists := seen[chosen.Path]; exists {
			continue
		}
		seen[chosen.Path] = struct{}{}
		targets, targetWarnings := packTargets(root, chosen, task)
		warnings = append(warnings, targetWarnings...)
		item := model.PackDoc{
			Path:    chosen.Path,
			Title:   chosen.Title,
			DocID:   chosen.DocID,
			Layer:   chosen.Layer,
			Family:  chosen.Family,
			Stage:   stage.Name,
			Why:     why,
			Targets: targets,
		}
		if full {
			sliceText, startLine, endLine, err := packSlice(root, chosen, targets)
			if err != nil {
				warnings = appendStringIfMissing(warnings, fmt.Sprintf("slice unavailable for %s", chosen.Path))
			} else {
				item.SliceText = sliceText
				item.SliceStart = startLine
				item.SliceEnd = endLine
			}
		}
		items = append(items, item)
		reasons = append(reasons, fmt.Sprintf("stage[%s]=%s", stage.Name, chosen.Path))
	}

	return items, reasons, warnings
}

func effectivePackDocsLimit(opts model.QueryOptions, profile model.DocsReadProfile) int {
	maxDocs := opts.MaxItems
	if maxDocs <= 0 {
		maxDocs = profile.ReadingPack.MaxDocs
	}
	if maxDocs <= 0 {
		maxDocs = 6
	}
	if isAXIPreview(opts) && maxDocs > 3 {
		return 3
	}
	return maxDocs
}

func packStagesForFamily(family string, profile model.DocsReadingPackProfile) []packStage {
	stageOrder := profile.FunctionalStageOrder
	switch family {
	case "technical":
		stageOrder = profile.TechnicalStageOrder
	case "ux":
		stageOrder = profile.UXStageOrder
	}
	if len(stageOrder) == 0 {
		switch family {
		case "technical":
			stageOrder = []string{"scope", "architecture", "technical_baseline", "technical_detail", "physical_data", "contracts"}
		case "ux":
			stageOrder = []string{"scope", "architecture", "ux_global", "ux_research", "ux_spec", "ux_handoff"}
		default:
			stageOrder = []string{"scope", "outcome", "architecture", "flow", "requirements", "data", "tests"}
		}
	}

	stages := make([]packStage, 0, len(stageOrder))
	for _, name := range stageOrder {
		if stage, ok := makePackStage(name); ok {
			stages = append(stages, stage)
		}
	}
	return stages
}

func makePackStage(name string) (packStage, bool) {
	switch name {
	case "governance":
		return packStage{Name: name, Required: true, Match: func(doc model.DocRecord) bool { return doc.Layer == "00" }}, true
	case "scope":
		return packStage{Name: name, Required: true, Match: func(doc model.DocRecord) bool { return doc.Layer == "01" }}, true
	case "outcome":
		return packStage{Name: name, Match: func(doc model.DocRecord) bool {
			normalized := filepath.ToSlash(doc.Path)
			return doc.Layer == "RS" ||
				strings.HasPrefix(strings.ToUpper(doc.DocID), "RS-") ||
				strings.Contains(normalized, "/02_resultados/") ||
				strings.HasSuffix(normalized, "/02_resultados_soluciones_usuario.md")
		}}, true
	case "architecture":
		return packStage{Name: name, Required: true, Match: func(doc model.DocRecord) bool { return doc.Layer == "02" }}, true
	case "flow":
		return packStage{Name: name, Required: true, Match: func(doc model.DocRecord) bool { return doc.Layer == "03" }}, true
	case "requirements":
		return packStage{Name: name, Required: true, Match: func(doc model.DocRecord) bool { return doc.Layer == "04" }}, true
	case "data":
		return packStage{Name: name, Match: func(doc model.DocRecord) bool { return doc.Layer == "05" }}, true
	case "tests":
		return packStage{Name: name, Match: func(doc model.DocRecord) bool { return doc.Layer == "06" }}, true
	case "technical_baseline":
		return packStage{Name: name, Required: true, Match: func(doc model.DocRecord) bool { return doc.Layer == "07" && !strings.Contains(doc.Path, "/07_tech/") }}, true
	case "technical_detail":
		return packStage{Name: name, Match: func(doc model.DocRecord) bool { return strings.Contains(doc.Path, "/07_tech/") }}, true
	case "physical_data":
		return packStage{Name: name, Match: func(doc model.DocRecord) bool { return doc.Layer == "08" || strings.Contains(doc.Path, "/08_db/") }}, true
	case "contracts":
		return packStage{Name: name, Match: func(doc model.DocRecord) bool {
			return doc.Layer == "09" || strings.Contains(doc.Path, "/09_contratos/")
		}}, true
	case "ux_global":
		return packStage{Name: name, Match: func(doc model.DocRecord) bool { return doc.Layer >= "10" && doc.Layer <= "16" }}, true
	case "ux_research":
		return packStage{Name: name, Match: func(doc model.DocRecord) bool { return doc.Layer >= "17" && doc.Layer <= "19" }}, true
	case "ux_spec":
		return packStage{Name: name, Match: func(doc model.DocRecord) bool { return doc.Layer >= "20" && doc.Layer <= "23" }}, true
	case "ux_handoff":
		return packStage{Name: name, Match: func(doc model.DocRecord) bool { return strings.Contains(doc.Path, "/23_uxui/") }}, true
	default:
		return packStage{}, false
	}
}

func filterDocsByStage(docs []model.DocRecord, stage packStage) []model.DocRecord {
	items := make([]model.DocRecord, 0)
	for _, doc := range docs {
		if stage.Match(doc) {
			items = append(items, doc)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Path < items[j].Path
	})
	return items
}

func linkedDocsByStage(ctx context.Context, db *sql.DB, primaryPath string, docByPath map[string]model.DocRecord, stages []packStage) map[string][]model.DocRecord {
	result := make(map[string][]model.DocRecord, len(stages))
	edges, err := store.DocEdgesFrom(ctx, db, primaryPath)
	if err != nil {
		return result
	}
	for _, edge := range edges {
		if edge.ToPath == "" {
			continue
		}
		doc, ok := docByPath[edge.ToPath]
		if !ok {
			continue
		}
		for _, stage := range stages {
			if stage.Match(doc) {
				result[stage.Name] = append(result[stage.Name], doc)
				break
			}
		}
	}
	return result
}

func choosePackDocForStage(stage packStage, candidates []model.DocRecord, primary model.DocRecord, rankedByPath map[string]scoredDoc, linked map[string][]model.DocRecord) (model.DocRecord, []string, bool) {
	if stage.Match(primary) {
		return primary, []string{"primary_doc"}, true
	}

	if docs := linked[stage.Name]; len(docs) > 0 {
		best := docs[0]
		bestScore := -1
		reasons := []string{"linked_from_primary"}
		for _, doc := range docs {
			score := 0
			if ranked, ok := rankedByPath[doc.Path]; ok {
				score = ranked.score
				reasons = append(reasons, ranked.reason...)
			}
			if score > bestScore {
				best = doc
				bestScore = score
			}
		}
		return best, reasons, true
	}

	bestScore := -1
	best := model.DocRecord{}
	var reasons []string
	for _, candidate := range candidates {
		ranked, ok := rankedByPath[candidate.Path]
		if !ok {
			continue
		}
		if ranked.score > bestScore {
			bestScore = ranked.score
			best = candidate
			reasons = append([]string{}, ranked.reason...)
		}
	}
	if best.Path != "" {
		return best, reasons, true
	}
	if stage.Required {
		return candidates[0], []string{"stage_required"}, true
	}
	return model.DocRecord{}, nil, false
}

func canonicalWikiExists(root string) bool {
	info, err := os.Stat(filepath.Join(root, ".docs", "wiki"))
	return err == nil && info.IsDir()
}

func hasIndexedCanonicalDocs(docs []model.DocRecord) bool {
	for _, doc := range docs {
		if strings.HasPrefix(doc.Path, ".docs/wiki/") && doc.Family != "generic" {
			return true
		}
	}
	return false
}

func packTargets(root string, doc model.DocRecord, task string) ([]model.PackTarget, []string) {
	absPath := filepath.Join(root, filepath.FromSlash(doc.Path))
	file, err := os.Open(absPath)
	if err != nil {
		return []model.PackTarget{{Heading: doc.Title, Line: 1, Reason: "title"}}, []string{fmt.Sprintf("targets unavailable for %s", doc.Path)}
	}
	defer file.Close()

	tokens := docgraph.QuestionTokens(task)
	scanner := bufio.NewScanner(file)
	targets := make([]model.PackTarget, 0, 3)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "#") {
			continue
		}
		heading := strings.TrimSpace(strings.TrimLeft(line, "#"))
		if heading == "" {
			continue
		}
		reason := "heading"
		if len(tokens) > 0 && containsAnyToken(strings.ToLower(heading), tokens) {
			reason = "heading_match"
		} else if len(targets) > 0 {
			continue
		}
		targets = append(targets, model.PackTarget{Heading: heading, Line: lineNo, Reason: reason})
		if len(targets) >= 3 {
			break
		}
	}
	if len(targets) == 0 {
		targets = append(targets, model.PackTarget{Heading: doc.Title, Line: 1, Reason: "title"})
	}
	return targets, nil
}

func packSlice(root string, doc model.DocRecord, targets []model.PackTarget) (string, int, int, error) {
	startLine := 1
	if len(targets) > 0 && targets[0].Line > 0 {
		startLine = targets[0].Line
	}
	endLine := startLine + 12
	content, lineCount, err := readFileLineRange(filepath.Join(root, filepath.FromSlash(doc.Path)), startLine, endLine)
	if err != nil {
		return "", 0, 0, err
	}
	actualEnd := endLine
	if lineCount > 0 && actualEnd > lineCount {
		actualEnd = lineCount
	}
	return content, startLine, actualEnd, nil
}

func buildPackNextQueries(workspaceName string, task string, full bool, docs []model.PackDoc) []string {
	queries := make([]string, 0, 3)
	if !full {
		queries = append(queries, fmt.Sprintf("mi-lsp nav pack %q --workspace %s --full", task, workspaceName))
	}
	if len(docs) > 0 && docs[0].DocID != "" {
		queries = append(queries, fmt.Sprintf("mi-lsp nav search %q --include-content --workspace %s", docs[0].DocID, workspaceName))
	} else if len(docs) > 0 {
		queries = append(queries, fmt.Sprintf("mi-lsp nav search %q --include-content --workspace %s", docs[0].Title, workspaceName))
	}
	if len(docs) > 0 {
		queries = append(queries, fmt.Sprintf("mi-lsp nav context %s %d --workspace %s", docs[len(docs)-1].Path, 1, workspaceName))
	}
	return queries
}

func containsAnyToken(value string, tokens []string) bool {
	for _, token := range tokens {
		if strings.Contains(value, token) {
			return true
		}
	}
	return false
}
