package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/wikisource"
)

func (a *App) wikiGraph(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	mode, payload, render, err := wikiGraphPayload(request)
	if err != nil {
		return model.Envelope{}, err
	}
	if render != "" && render != "mermaid" {
		return model.Envelope{}, &model.GraphQueryError{Code: "GPH_QUERY_BUDGET_INVALID", Field: "render", Message: "render must be mermaid"}
	}
	registration, _, err := a.resolveWorkspaceWithProjectForNavigation(request)
	if err != nil {
		return model.Envelope{}, &model.GraphQueryError{Code: "GPH_QUERY_BACKEND_UNAVAILABLE", Message: "graph backend is unavailable"}
	}
	db, err := openWorkspaceDB(registration, request.Operation, true)
	if err != nil {
		return model.Envelope{}, store.SanitizeGraphQueryError(err)
	}
	defer db.Close()

	envelope := model.Envelope{
		Ok: true, Workspace: registration.Name, Backend: "sqlite-direct",
		ContractVersion: "wiki-graph/v1", Mode: mode, Items: []any{},
	}
	envelope.DocGenerationID, _, _ = store.WorkspaceMetaValue(ctx, db, store.WorkspaceMetaActiveDocsGeneration)
	envelope.CodeGenerationID, _, _ = store.WorkspaceMetaValue(ctx, db, store.WorkspaceMetaActiveCatalogGeneration)

	switch mode {
	case "neighbors", "path", "explain":
		child := request
		child.Operation = map[string]string{"neighbors": "nav.neighbors", "path": "nav.path", "explain": "nav.explain"}[mode]
		child.Payload = payload
		q, queryErr := graphRequestFromPayload(child)
		if queryErr != nil {
			return model.Envelope{}, queryErr
		}
		result, queryErr := GraphQuery(ctx, db, q)
		if queryErr != nil {
			return model.Envelope{}, store.SanitizeGraphQueryError(queryErr)
		}
		copyWikiGraphEnvelope(&envelope, result)
		nodes, edges, graphErr := wikiGraphItemsFromEnvelope(ctx, db, result.GenerationID, result.Items)
		if graphErr != nil {
			return model.Envelope{}, store.SanitizeGraphQueryError(graphErr)
		}
		envelope.Nodes, envelope.Edges = nodes, edges

	case "affected":
		child := request
		child.Operation = "nav.graph-impact"
		child.Payload = payload
		if _, exists := payload["include_docs"]; !exists {
			child.Payload["include_docs"] = true
		}
		if strings.TrimSpace(stringPayload(payload, "mode")) == "" {
			child.Payload["mode"] = model.GraphImpactModeDirect
		}
		q, queryErr := graphImpactRequestFromPayload(child)
		if queryErr != nil {
			return model.Envelope{}, queryErr
		}
		impact, queryErr := GraphImpact(ctx, db, q)
		if queryErr != nil {
			return model.Envelope{}, store.SanitizeGraphQueryError(queryErr)
		}
		if graphErr := applyWikiGraphImpact(ctx, db, impact, &envelope); graphErr != nil {
			return model.Envelope{}, store.SanitizeGraphQueryError(graphErr)
		}

	case "change-pack":
		child := request
		child.Operation = "nav.change-pack"
		child.Payload = changePackPayload(payload)
		result, queryErr := a.changePack(ctx, child)
		if queryErr != nil {
			return model.Envelope{}, queryErr
		}
		copyWikiGraphEnvelope(&envelope, result)
		envelope.Items = result.Items
		envelope.Backend = result.Backend
		envelope.Stats = result.Stats
		if packets, ok := result.Items.([]ChangePackPacket); ok && len(packets) > 0 {
			paths := packets[0].ChangedPaths
			if len(paths) == 0 {
				paths = graphImpactPathsFromPayload(payload["paths"])
			}
			if len(paths) > 0 {
				impactRequest := model.GraphImpactRequest{
					Generation: result.GenerationID, Mode: model.GraphImpactModeDirect,
					Depth: intFromAny(payload["depth"], 0), Limit: intFromAny(payload["limit"], model.GraphQueryDefaultLimit),
					TokenBudget: intFromAny(payload["token_budget"], model.GraphQueryDefaultToken),
					Direction:   stringPayload(payload, "direction"), IncludeDocs: true, IncludeTests: true, Paths: paths,
				}
				impactRequest, normalizeErr := impactRequest.Normalize()
				if normalizeErr == nil {
					impact, impactErr := GraphImpact(ctx, db, impactRequest)
					if impactErr == nil {
						if graphErr := applyWikiGraphImpact(ctx, db, impact, &envelope); graphErr != nil {
							return model.Envelope{}, store.SanitizeGraphQueryError(graphErr)
						}
					} else {
						envelope.Warnings = appendStringIfMissing(envelope.Warnings, "change_pack_graph_projection_unavailable:"+sanitizeIntentError(impactErr))
					}
				} else {
					envelope.Warnings = appendStringIfMissing(envelope.Warnings, "change_pack_graph_projection_unavailable:"+sanitizeIntentError(normalizeErr))
				}
			}
		}

	case "matrix":
		language := ResolveWikiContractLanguageAtRoot(registration.Root)
		envelope.Language = &language
		for _, warning := range language.Warnings {
			envelope.Warnings = appendStringIfMissing(envelope.Warnings, warning.Message)
		}
		rows, graphItems, generationID, freshness, truncated, warnings, matrixErr := wikiGraphMatrix(ctx, db, payload)
		if matrixErr != nil {
			return model.Envelope{}, matrixErr
		}
		envelope.Matrix = rows
		envelope.GenerationID = generationID
		envelope.GraphFreshness = &freshness
		envelope.Truncated = truncated
		envelope.Warnings = appendUniqueStrings(envelope.Warnings, warnings...)
		if generationID != "" {
			limit := intFromAny(payload["limit"], model.GraphQueryDefaultLimit)
			if limit < 1 || limit > model.GraphQueryMaxLimit {
				limit = model.GraphQueryDefaultLimit
			}
			omissions, totalUnresolved, unresolvedErr := wikiGraphUnresolvedOmissions(ctx, db, generationID, limit)
			if unresolvedErr != nil {
				return model.Envelope{}, store.SanitizeGraphQueryError(unresolvedErr)
			}
			envelope.Omissions = append(envelope.Omissions, omissions...)
			for _, omission := range omissions {
				envelope.Unresolved = append(envelope.Unresolved, wikiGraphUnresolvedFromOmission(omission))
			}
			if totalUnresolved > len(omissions) {
				envelope.Truncated = true
				envelope.Warnings = appendStringIfMissing(envelope.Warnings, "unresolved_truncated: unresolved targets exceed the response limit")
			}
			nodes, edges, graphErr := wikiGraphItemsFromEnvelope(ctx, db, generationID, graphItems)
			if graphErr != nil {
				return model.Envelope{}, store.SanitizeGraphQueryError(graphErr)
			}
			envelope.Nodes, envelope.Edges = nodes, edges
		}
		if render != "" && freshness.State != model.GraphFreshnessCurrent {
			envelope.Warnings = appendStringIfMissing(envelope.Warnings, "mermaid_unavailable: graph freshness is "+freshness.State)
		}

	default:
		return model.Envelope{}, &model.GraphQueryError{Code: "GPH_QUERY_SELECTOR_INVALID", Field: "mode", Message: "mode must be neighbors, path, explain, affected, change-pack, or matrix"}
	}

	applyWikiRoadmapMetadata(registration.Root, &envelope)
	if render != "" {
		if render != "mermaid" {
			return model.Envelope{}, &model.GraphQueryError{Code: "GPH_QUERY_BUDGET_INVALID", Field: "render", Message: "render must be mermaid"}
		}
		if envelope.GraphFreshness == nil || envelope.GraphFreshness.State != model.GraphFreshnessCurrent {
			state := "unavailable"
			if envelope.GraphFreshness != nil {
				state = envelope.GraphFreshness.State
			}
			envelope.Warnings = appendStringIfMissing(envelope.Warnings, "mermaid_unavailable: graph freshness is "+state)
		} else if mermaid := wikiGraphMermaid(envelope.GenerationID, envelope.Nodes, envelope.Edges); mermaid != nil {
			envelope.Mermaid = mermaid
		} else {
			envelope.Warnings = appendStringIfMissing(envelope.Warnings, "mermaid_unavailable: the graph page has no resolved edges")
		}
	}
	return envelope, nil
}

func attachWikiContractV11Details(root string, records []model.DocRecord, envelope *model.Envelope) {
	language := ResolveWikiContractLanguageAtRoot(root)
	envelope.ContractVersion = "wiki-contract/v1"
	envelope.Language = &language
	checks := make([]model.WikiContractValidationCheck, 0, len(records))
	warnings := make([]string, 0)
	for _, warning := range language.Warnings {
		warnings = appendStringIfMissing(warnings, warning.Message)
	}
	for _, record := range records {
		if strings.TrimSpace(record.Path) == "" {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(record.Path)))
		if readErr != nil {
			finding := model.WikiContractFinding{
				Code: "document_unreadable", DocID: record.DocID, Source: record.Path,
				Message: "document_unreadable:" + record.Path, Blocking: false,
			}
			checks = append(checks, model.WikiContractValidationCheck{DocID: record.DocID, Path: record.Path, Kind: wikiContractKind(record, wikisource.ParsedDoc{}), Findings: []model.WikiContractFinding{finding}})
			continue
		}
		content := string(body)
		parsed := wikisource.Parse(record.Path, content, 0)
		kind := wikiContractKind(record, parsed)
		if kind == "" {
			continue
		}
		check := model.WikiContractValidationCheck{
			DocID: record.DocID, Path: record.Path, Kind: kind,
			Sections: model.WikiContractRequiredSections(kind, language.Language),
		}
		if language.Valid {
			check.Findings = append(check.Findings, model.ValidateWikiContractSections(record.DocID, kind, content, language.Language)...)
			check.Findings = append(check.Findings, model.WikiContractDiagramFindings(record.Path, kind, content, language.Language)...)
			if kind == "requirement" {
				check.Criteria = model.ParseWikiContractAcceptanceCriteria(content, language.Language)
			}
			if kind == "test" {
				cases, caseFindings := model.ParseWikiContractTestCases(content, language.Language)
				check.Cases = cases
				check.Findings = append(check.Findings, caseFindings...)
			}
		}
		for _, finding := range check.Findings {
			if finding.Code == "diagram_missing" {
				warnings = appendStringIfMissing(warnings, finding.Message)
			}
		}
		checks = append(checks, check)
	}
	envelope.Checks = checks
	envelope.Warnings = appendUniqueStrings(envelope.Warnings, warnings...)
}

func wikiGraphPayload(request model.CommandRequest) (string, map[string]any, string, error) {
	query, ok := request.Payload["query"].(map[string]any)
	if !ok {
		return "", nil, "", &model.GraphQueryError{Code: "GPH_QUERY_SELECTOR_INVALID", Field: "query", Message: "query must be an object with a mode"}
	}
	mode := strings.ToLower(strings.TrimSpace(stringPayload(query, "mode")))
	switch mode {
	case "neighbors", "path", "explain", "affected", "change-pack", "matrix":
	default:
		return "", nil, "", &model.GraphQueryError{Code: "GPH_QUERY_SELECTOR_INVALID", Field: "query.mode", Message: "query.mode is unsupported"}
	}
	payload := make(map[string]any, len(request.Payload)+len(query))
	for key, value := range request.Payload {
		if key != "query" && key != "render" {
			payload[key] = value
		}
	}
	for key, value := range query {
		if key != "mode" {
			payload[key] = value
		}
	}
	if relations, ok := payload["relations"]; ok {
		if _, exists := payload["edge"]; !exists {
			payload["edge"] = relations
		}
	}
	switch mode {
	case "neighbors":
		if strings.TrimSpace(stringPayload(payload, "selector")) == "" {
			payload["selector"] = firstNonEmpty(stringPayload(payload, "from"), stringPayload(payload, "ref"))
		}
	case "explain":
		if strings.TrimSpace(stringPayload(payload, "selector")) == "" {
			payload["selector"] = firstNonEmpty(stringPayload(payload, "ref"), stringPayload(payload, "from"))
		}
	case "affected":
		impactMode := strings.TrimSpace(stringPayload(payload, "impact_mode"))
		if impactMode == "" {
			impactMode = model.GraphImpactModeDirect
		}
		payload["mode"] = impactMode
	case "change-pack":
		if _, exists := payload["paths"]; !exists {
			if changed, ok := payload["changed_paths"]; ok {
				payload["paths"] = changed
			}
		}
	}
	return mode, payload, strings.ToLower(strings.TrimSpace(stringPayload(request.Payload, "render"))), nil
}

func changePackPayload(payload map[string]any) map[string]any {
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		out[key] = value
	}
	if _, exists := out["paths"]; !exists {
		if changed, ok := out["changed_paths"]; ok {
			out["paths"] = changed
		}
	}
	return out
}

func copyWikiGraphEnvelope(dst *model.Envelope, src model.Envelope) {
	dst.Ok = src.Ok
	dst.Workspace = src.Workspace
	dst.Backend = src.Backend
	dst.GenerationID = src.GenerationID
	dst.GraphSchemaVersion = src.GraphSchemaVersion
	dst.DeterminismDigest = src.DeterminismDigest
	dst.GraphFreshness = src.GraphFreshness
	dst.Truncated = src.Truncated
	dst.Continuation = src.Continuation
	dst.Stats = src.Stats
	dst.Hint = src.Hint
	dst.Omissions = append(dst.Omissions, src.Omissions...)
	for _, omission := range src.Omissions {
		dst.Unresolved = append(dst.Unresolved, wikiGraphUnresolvedFromOmission(omission))
	}
	dst.Warnings = appendUniqueStrings(dst.Warnings, src.Warnings...)
}

func wikiGraphUnresolvedFromOmission(omission model.EnvelopeOmission) model.WikiGraphUnresolved {
	return model.WikiGraphUnresolved{
		OwnerPath:      firstNonEmpty(omission.OwnerPath, omission.Path),
		SourceDocument: omission.SourceDocument, SourceBlock: omission.SourceBlock,
		TargetKind: omission.TargetKind, TargetValue: firstNonEmpty(omission.TargetValue, omission.Path),
		ReasonCode: firstNonEmpty(omission.Reason, omission.ErrorCode, "unresolved"), Provenance: "candidate",
	}
}

func wikiGraphUnresolvedOmissions(ctx context.Context, db *sql.DB, generationID string, limit int) ([]model.EnvelopeOmission, int, error) {
	snapshot, err := store.BeginGraphQuerySnapshot(ctx, db, generationID)
	if err != nil {
		return nil, 0, err
	}
	defer snapshot.Close()
	omissions, err := snapshot.UnresolvedOmissions(ctx, limit)
	if err != nil {
		return nil, 0, err
	}
	total, err := snapshot.UnresolvedCount(ctx)
	return omissions, total, err
}

func wikiGraphItemsFromEnvelope(ctx context.Context, db *sql.DB, generationID string, rawItems any) ([]model.WikiGraphNode, []model.WikiGraphEdge, error) {
	items, err := decodeGraphQueryItems(rawItems)
	if err != nil {
		return nil, nil, err
	}
	snapshot, err := store.BeginGraphQuerySnapshot(ctx, db, generationID)
	if err != nil {
		return nil, nil, err
	}
	defer snapshot.Close()
	nodes := make(map[string]model.WikiGraphNode)
	edges := make(map[string]model.WikiGraphEdge)
	for _, item := range items {
		if item.EdgeCrossRID == "" && item.EdgeID == 0 && item.Kind != "edge" {
			if node := resolveWikiGraphNode(ctx, snapshot, item.CrossRID); node.ID != "" {
				nodes[node.ID] = node
			} else if item.CrossRID != "" {
				nodes[item.CrossRID] = wikiGraphNodeFromQueryItem(item, item.CrossRID)
			}
			continue
		}
		edgeID := firstNonEmpty(item.EdgeCrossRID, item.CrossRID)
		fromID, toID := item.FromCrossRID, item.ToCrossRID
		for _, endpoint := range []string{fromID, toID} {
			if node := resolveWikiGraphNode(ctx, snapshot, endpoint); node.ID != "" {
				nodes[node.ID] = node
			} else if endpoint != "" {
				nodes[endpoint] = model.WikiGraphNode{ID: endpoint, Kind: "unknown"}
			}
		}
		if edgeID == "" || fromID == "" || toID == "" {
			continue
		}
		edges[edgeID] = model.WikiGraphEdge{ID: edgeID, From: fromID, Relation: item.Relation, To: toID, Provenance: wikiGraphProvenance(item.Status)}
	}
	nodeItems, edgeItems := sortedWikiGraph(nodes, edges)
	return nodeItems, edgeItems, nil
}

func applyWikiGraphImpact(ctx context.Context, db *sql.DB, impact model.GraphImpactEnvelope, envelope *model.Envelope) error {
	envelope.GenerationID = impact.GenerationID
	envelope.GraphSchemaVersion = impact.GraphSchemaVersion
	envelope.DeterminismDigest = impact.DeterminismDigest
	envelope.GraphFreshness = &impact.GraphFreshness
	envelope.Truncated = envelope.Truncated || impact.Truncated
	envelope.Continuation = impact.Continuation
	envelope.Warnings = appendUniqueStrings(envelope.Warnings, impact.Warnings...)
	for _, omission := range impact.Omissions {
		envelope.Unresolved = append(envelope.Unresolved, model.WikiGraphUnresolved{
			OwnerPath: omission.Path, TargetKind: "path", TargetValue: omission.Path,
			Candidates: append([]string(nil), omission.Candidates...), ReasonCode: omission.Code, Provenance: "candidate",
		})
		envelope.Omissions = append(envelope.Omissions, model.EnvelopeOmission{
			Path: omission.Path, Reason: omission.Reason, ErrorCode: omission.Code,
		})
	}
	items := append(append([]model.GraphImpactItem(nil), impact.Items...), impact.Inferred...)
	nodes, edges, err := wikiGraphImpactItems(ctx, db, impact.GenerationID, items)
	if err != nil {
		return err
	}
	envelope.Nodes, envelope.Edges = nodes, edges
	envelope.Stats = model.Stats{Symbols: len(items), TokensEstimate: impact.Stats.TokenUnits}
	return nil
}

func wikiGraphImpactItems(ctx context.Context, db *sql.DB, generationID string, items []model.GraphImpactItem) ([]model.WikiGraphNode, []model.WikiGraphEdge, error) {
	snapshot, err := store.BeginGraphQuerySnapshot(ctx, db, generationID)
	if err != nil {
		return nil, nil, err
	}
	defer snapshot.Close()
	nodes := make(map[string]model.WikiGraphNode)
	edges := make(map[string]model.WikiGraphEdge)
	for _, item := range items {
		if item.CrossRID != "" {
			if node := resolveWikiGraphNode(ctx, snapshot, item.CrossRID); node.ID != "" {
				nodes[node.ID] = node
			} else {
				nodes[item.CrossRID] = model.WikiGraphNode{ID: item.CrossRID, Kind: item.SymbolKind, Path: item.OwnerPath, Symbol: item.Display}
			}
		}
		for _, step := range item.EvidencePath {
			for _, endpoint := range []string{step.FromCrossRID, step.ToCrossRID} {
				if node := resolveWikiGraphNode(ctx, snapshot, endpoint); node.ID != "" {
					nodes[node.ID] = node
				} else if endpoint != "" {
					nodes[endpoint] = model.WikiGraphNode{ID: endpoint, Kind: "unknown"}
				}
			}
			if step.EdgeCrossRID == "" || step.FromCrossRID == "" || step.ToCrossRID == "" {
				continue
			}
			edges[step.EdgeCrossRID] = model.WikiGraphEdge{
				ID: step.EdgeCrossRID, From: step.FromCrossRID, Relation: step.Relation,
				To: step.ToCrossRID, Provenance: wikiGraphProvenance(step.Status),
			}
		}
	}
	nodeItems, edgeItems := sortedWikiGraph(nodes, edges)
	return nodeItems, edgeItems, nil
}

func decodeGraphQueryItems(raw any) ([]model.GraphQueryItem, error) {
	if raw == nil {
		return nil, nil
	}
	body, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var items []model.GraphQueryItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func resolveWikiGraphNode(ctx context.Context, snapshot *store.GraphQuerySnapshot, crossRID string) model.WikiGraphNode {
	if strings.TrimSpace(crossRID) == "" {
		return model.WikiGraphNode{}
	}
	nodes, _, err := snapshot.ResolveGraphSelector(ctx, crossRID)
	if err != nil || len(nodes) != 1 {
		return model.WikiGraphNode{}
	}
	node := nodes[0]
	out := model.WikiGraphNode{ID: node.CrossRID, Kind: node.Identity.SymbolKind, Path: node.Identity.OwnerPath}
	if node.Identity.SymbolKind == "document" {
		if node.Identity.SemanticIdentity != node.Identity.OwnerPath {
			out.DocID = node.Identity.SemanticIdentity
		}
	} else if node.Identity.SymbolKind != "file" {
		out.Symbol = node.DisplayName
	}
	return out
}

func wikiGraphNodeFromQueryItem(item model.GraphQueryItem, id string) model.WikiGraphNode {
	return model.WikiGraphNode{
		ID: id, Kind: firstNonEmpty(item.SymbolKind, "unknown"), DocID: item.DocID,
		Path: firstNonEmpty(item.OwnerPath, item.DestinationPath), Symbol: item.Display,
	}
}

func wikiGraphProvenance(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case model.GraphRecordExact:
		return "declared"
	case model.GraphRecordExtracted:
		return "observed"
	case model.GraphRecordInferred:
		return "derived"
	default:
		return "derived"
	}
}

func sortedWikiGraph(nodes map[string]model.WikiGraphNode, edges map[string]model.WikiGraphEdge) ([]model.WikiGraphNode, []model.WikiGraphEdge) {
	nodeItems := make([]model.WikiGraphNode, 0, len(nodes))
	for _, node := range nodes {
		nodeItems = append(nodeItems, node)
	}
	sort.Slice(nodeItems, func(i, j int) bool { return nodeItems[i].ID < nodeItems[j].ID })
	edgeItems := make([]model.WikiGraphEdge, 0, len(edges))
	for _, edge := range edges {
		edgeItems = append(edgeItems, edge)
	}
	sort.Slice(edgeItems, func(i, j int) bool { return edgeItems[i].ID < edgeItems[j].ID })
	return nodeItems, edgeItems
}

func wikiGraphMatrix(ctx context.Context, db *sql.DB, payload map[string]any) ([]model.WikiGraphMatrixRow, []model.GraphQueryItem, string, model.GraphFreshness, bool, []string, error) {
	generation, found, err := store.ActiveGraphGeneration(ctx, db)
	if err != nil {
		return nil, nil, "", model.GraphFreshness{}, false, nil, err
	}
	if !found || generation == (model.GraphDigest{}) {
		return nil, nil, "", model.GraphFreshness{}, false, nil, &model.GraphQueryError{Code: "GPH_QUERY_GRAPH_INVALID", Message: "active graph generation is unavailable"}
	}
	generationID := generation.String()
	freshness, err := store.GraphFreshness(ctx, db, generationID)
	if err != nil {
		return nil, nil, "", model.GraphFreshness{}, false, nil, err
	}
	records, err := store.ListDocRecords(ctx, db)
	if err != nil {
		return nil, nil, "", model.GraphFreshness{}, false, nil, err
	}
	byID := make(map[string]model.DocRecord, len(records))
	for _, record := range records {
		if strings.TrimSpace(record.DocID) != "" {
			byID[record.DocID] = record
		}
	}
	requirements := make([]model.DocRecord, 0)
	for _, record := range records {
		if docgraph.WikiContractKindForPath(record.Path, "") == "requirement" && strings.TrimSpace(record.DocID) != "" {
			requirements = append(requirements, record)
		}
	}
	sort.Slice(requirements, func(i, j int) bool { return requirements[i].DocID < requirements[j].DocID })
	limit := intFromAny(payload["limit"], model.GraphQueryDefaultLimit)
	if limit < 1 || limit > model.GraphQueryMaxLimit {
		limit = model.GraphQueryDefaultLimit
	}
	tokenBudget := intFromAny(payload["token_budget"], model.GraphQueryDefaultToken)
	if tokenBudget < 1 || tokenBudget > model.GraphQueryMaxToken {
		tokenBudget = model.GraphQueryDefaultToken
	}
	rows := make([]model.WikiGraphMatrixRow, 0, len(requirements))
	graphItems := make([]model.GraphQueryItem, 0)
	warnings := make([]string, 0)
	truncated := false
	for _, record := range requirements {
		q := model.GraphQueryRequest{
			Operation: "nav.neighbors", Selector: record.DocID, Generation: generationID,
			Depth: 1, Limit: limit, TokenBudget: tokenBudget, Direction: "in", Relations: []string{"verifies"},
		}
		q, err = q.Normalize()
		if err != nil {
			return nil, nil, "", model.GraphFreshness{}, false, nil, err
		}
		result, queryErr := GraphQuery(ctx, db, q)
		if queryErr != nil {
			return nil, nil, "", model.GraphFreshness{}, false, nil, store.SanitizeGraphQueryError(queryErr)
		}
		if result.GenerationID != generationID {
			return nil, nil, "", model.GraphFreshness{}, false, nil, &model.GraphQueryError{Code: "GPH_QUERY_CURSOR_STALE", Message: "matrix query crossed graph generations"}
		}
		warnings = appendUniqueStrings(warnings, result.Warnings...)
		truncated = truncated || result.Truncated
		items, decodeErr := decodeGraphQueryItems(result.Items)
		if decodeErr != nil {
			return nil, nil, "", model.GraphFreshness{}, false, nil, decodeErr
		}
		graphItems = append(graphItems, items...)
		tpIDs := make([]string, 0)
		seenTP := make(map[string]struct{})
		for _, item := range items {
			if item.Relation != "verifies" {
				continue
			}
			tpID := strings.TrimSpace(item.DocID)
			if tpRecord, ok := byID[tpID]; !ok || docgraph.WikiContractKindForPath(tpRecord.Path, "") != "test" {
				tpID = ""
				if tpRecord, ok := findDocRecordByPath(records, item.OwnerPath); ok && docgraph.WikiContractKindForPath(tpRecord.Path, "") == "test" {
					tpID = tpRecord.DocID
				}
			}
			if tpID == "" {
				continue
			}
			if _, exists := seenTP[tpID]; !exists {
				seenTP[tpID] = struct{}{}
				tpIDs = append(tpIDs, tpID)
			}
		}
		sort.Strings(tpIDs)
		rows = append(rows, model.WikiGraphMatrixRow{RFID: record.DocID, TPIDs: tpIDs})
	}
	return rows, graphItems, generationID, freshness, truncated, warnings, nil
}

func findDocRecordByPath(records []model.DocRecord, path string) (model.DocRecord, bool) {
	for _, record := range records {
		if record.Path == path {
			return record, true
		}
	}
	return model.DocRecord{}, false
}

func applyWikiRoadmapMetadata(root string, envelope *model.Envelope) {
	parsed := loadWikiRoadmapMetadata(root)
	if len(parsed.ByRS) > 0 {
		keys := make([]string, 0, len(parsed.ByRS))
		for key := range parsed.ByRS {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		entries := make([]model.WikiGraphRoadmapEntry, 0, len(keys))
		for _, key := range keys {
			metadata := parsed.ByRS[key]
			entries = append(entries, model.WikiGraphRoadmapEntry{RSID: metadata.RS, Horizon: metadata.Horizon, Status: metadata.State})
		}
		envelope.Roadmap = &model.WikiGraphRoadmap{Canonical: false, Entries: entries}
	}
	for _, warning := range parsed.Warnings {
		envelope.Warnings = appendStringIfMissing(envelope.Warnings, warning.Message)
	}
}

func wikiGraphMermaid(generationID string, nodes []model.WikiGraphNode, edges []model.WikiGraphEdge) *model.WikiGraphMermaid {
	if len(nodes) == 0 || len(edges) == 0 {
		return nil
	}
	nodeIDs := make([]string, 0, len(nodes))
	labelByID := make(map[string]string, len(nodes))
	aliasByID := make(map[string]string, len(nodes))
	for i, node := range nodes {
		alias := fmt.Sprintf("n%d", i+1)
		aliasByID[node.ID] = alias
		nodeIDs = append(nodeIDs, node.ID)
		label := firstNonEmpty(node.DocID, node.Symbol, node.Path, node.ID)
		labelByID[node.ID] = strings.NewReplacer("\"", "'", "\r", " ", "\n", " ").Replace(label)
	}
	var source strings.Builder
	source.WriteString("flowchart LR\n")
	for _, node := range nodes {
		source.WriteString(fmt.Sprintf("  %s[\"%s\"]\n", aliasByID[node.ID], labelByID[node.ID]))
	}
	edgeIDs := make([]string, 0, len(edges))
	for _, edge := range edges {
		from, fromOK := aliasByID[edge.From]
		to, toOK := aliasByID[edge.To]
		if !fromOK || !toOK {
			continue
		}
		edgeIDs = append(edgeIDs, edge.ID)
		relation := strings.NewReplacer("|", " ", "\r", " ", "\n", " ").Replace(edge.Relation)
		source.WriteString(fmt.Sprintf("  %s -->|%s| %s\n", from, relation, to))
	}
	if len(edgeIDs) == 0 {
		return nil
	}
	return &model.WikiGraphMermaid{Format: "mermaid", Source: source.String(), GenerationID: generationID, NodeIDs: nodeIDs, EdgeIDs: edgeIDs}
}

func wikiContractKind(record model.DocRecord, parsed wikisource.ParsedDoc) string {
	kind := strings.ToLower(strings.TrimSpace(parsed.DocumentKind))
	switch kind {
	case "scope", "outcome", "architecture", "decision", "flow", "requirement", "test", "technical", "data_model", "database", "contract":
		return kind
	}
	path := strings.ToLower(filepath.ToSlash(record.Path))
	if strings.HasSuffix(path, "/03_architecture.md") || filepath.Base(path) == "03_architecture.md" {
		return "architecture"
	}
	if kind := docgraph.WikiContractKindForPath(record.Path, ""); kind != "" {
		switch kind {
		case "scope", "outcome", "architecture", "decision", "flow", "requirement", "test", "technical", "data_model", "database", "contract":
			return kind
		}
	}
	if strings.Contains(path, "outcome") {
		return "outcome"
	}
	switch strings.ToUpper(strings.TrimSpace(record.Layer)) {
	case "RS":
		return "scope"
	case "RF":
		return "requirement"
	case "FL":
		return "flow"
	case "TP":
		return "test"
	case "ADR":
		return "decision"
	case "TECH", "TECH-DS":
		return "technical"
	case "DB":
		return "database"
	case "CT":
		return "contract"
	default:
		return ""
	}
}

func appendUniqueStrings(dst []string, values ...string) []string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			dst = appendStringIfMissing(dst, value)
		}
	}
	return dst
}
