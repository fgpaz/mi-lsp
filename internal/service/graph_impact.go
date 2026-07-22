package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

type graphImpactPath struct {
	Steps []model.GraphImpactPathStep
	Key   string
}

type graphImpactCursor struct {
	Generation    string   `json:"generation"`
	Operation     string   `json:"operation"`
	Paths         []string `json:"paths"`
	Mode          string   `json:"mode"`
	Depth         int      `json:"depth"`
	Limit         int      `json:"limit"`
	Token         int      `json:"token"`
	Direction     string   `json:"direction"`
	Relations     []string `json:"relations"`
	IncludeTests  bool     `json:"include_tests"`
	IncludeDocs   bool     `json:"include_docs"`
	Offset        int      `json:"offset"`
	RequestDigest string   `json:"request_digest"`
	Checksum      string   `json:"checksum"`
}

func graphImpactCursorDigest(cursor graphImpactCursor) string {
	cursor.Checksum = ""
	payload, _ := json.Marshal(cursor)
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func encodeGraphImpactCursor(cursor graphImpactCursor) string {
	cursor.Checksum = graphImpactCursorDigest(cursor)
	payload, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeGraphImpactCursor(raw string) (graphImpactCursor, error) {
	var cursor graphImpactCursor
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || json.Unmarshal(payload, &cursor) != nil || cursor.Checksum == "" || cursor.Checksum != graphImpactCursorDigest(cursor) || cursor.Offset < 0 {
		return cursor, &model.GraphQueryError{Code: "GPH_QUERY_CURSOR_STALE", Message: "impact cursor is invalid or stale"}
	}
	if cursor.Operation != "nav.graph-impact" || len(cursor.Paths) > model.MaxImpactSeeds {
		return cursor, &model.GraphQueryError{Code: "GPH_QUERY_CURSOR_STALE", Message: "impact cursor is invalid or stale"}
	}
	return cursor, nil
}

func GraphImpact(ctx context.Context, db *sql.DB, request model.GraphImpactRequest) (model.GraphImpactEnvelope, error) {
	if ctx == nil || db == nil {
		return model.GraphImpactEnvelope{}, &model.GraphQueryError{Code: "GPH_QUERY_BACKEND_UNAVAILABLE", Message: "graph backend is unavailable"}
	}
	var cursor graphImpactCursor
	offset := 0
	if strings.TrimSpace(request.Cursor) != "" {
		var err error
		cursor, err = decodeGraphImpactCursor(strings.TrimSpace(request.Cursor))
		if err != nil {
			return model.GraphImpactEnvelope{}, err
		}
		request.Cursor = strings.TrimSpace(request.Cursor)
		if request.Generation != "" && cursor.Generation != strings.TrimSpace(request.Generation) {
			return model.GraphImpactEnvelope{}, &model.GraphQueryError{Code: "GPH_QUERY_CURSOR_STALE", Message: "impact cursor generation does not match request"}
		}
		if len(request.Paths) == 0 && len(request.ChangedPaths) == 0 {
			request.Paths = append([]string(nil), cursor.Paths...)
			request.ChangedPaths = append([]string(nil), cursor.Paths...)
		}
		if request.Generation == "" {
			request.Generation = cursor.Generation
		}
		if request.Mode == "" {
			request.Mode = cursor.Mode
		}
		if request.Depth == 0 {
			request.Depth = cursor.Depth
		}
		if request.Limit == 0 {
			request.Limit = cursor.Limit
		}
		if request.TokenBudget == 0 {
			request.TokenBudget = cursor.Token
		}
		if request.Direction == "" {
			request.Direction = cursor.Direction
		}
		if len(request.Relations) == 0 {
			request.Relations = append([]string(nil), cursor.Relations...)
		}
		request.IncludeTests = cursor.IncludeTests
		request.IncludeDocs = cursor.IncludeDocs
		offset = cursor.Offset
	}
	q, err := request.Normalize()
	if err != nil {
		return model.GraphImpactEnvelope{}, err
	}
	snapshot, err := store.BeginGraphQuerySnapshot(ctx, db, q.Generation)
	if err != nil {
		var graphErr *model.GraphQueryError
		if errors.As(err, &graphErr) && graphErr.Code == "GPH_QUERY_GRAPH_INVALID" {
			return model.GraphImpactEnvelope{}, &model.GraphQueryError{Code: "GPH_IMPACT_GRAPH_STALE", Message: "graph catalog is stale"}
		}
		return model.GraphImpactEnvelope{}, store.SanitizeGraphQueryError(err)
	}
	defer snapshot.Close()
	generation, err := snapshot.Generation()
	if err != nil {
		return model.GraphImpactEnvelope{}, store.SanitizeGraphQueryError(err)
	}
	q.Generation = generation.GenerationID.String()
	requestDigest := graphImpactRequestDigest(q)
	if q.Cursor != "" && (cursor.Generation != q.Generation || cursor.RequestDigest != requestDigest) {
		return model.GraphImpactEnvelope{}, &model.GraphQueryError{Code: "GPH_QUERY_CURSOR_STALE", Message: "impact cursor parameters do not match request"}
	}
	seeds, err := snapshot.ResolveImpactSeeds(ctx, q.Paths, model.MaxImpactSeeds)
	if err != nil {
		return model.GraphImpactEnvelope{}, store.SanitizeGraphQueryError(err)
	}
	env := model.GraphImpactEnvelope{
		Ok:                 true,
		Backend:            "sqlite-direct",
		GenerationID:       generation.GenerationID.String(),
		GraphSchemaVersion: generation.SchemaVersion,
		Mode:               q.Mode,
		PrecisionScope:     "exact-and-extracted graph edges only; inferred edges are reported separately and are not traversed",
		Relations:          append([]string(nil), q.Relations...),
		Items:              []model.GraphImpactItem{},
		Inferred:           []model.GraphImpactItem{},
		Omissions:          append(append([]model.GraphImpactOmission(nil), q.Omissions...), seeds.Omissions...),
		Warnings:           append([]string(nil), q.Warnings...),
	}
	seedTruncated := impactSeedBudgetOmitted(q.Omissions) || impactSeedBudgetOmitted(seeds.Omissions)
	if seedTruncated {
		env.Truncated = true
		env.Warnings = appendStringIfMissing(env.Warnings, "impact seed resolution was bounded; omitted seeds were not traversed")
	}
	if len(q.Paths) == 0 {
		env.Omissions = append(env.Omissions, model.GraphImpactOmission{Code: "GPH_IMPACT_SEED_UNRESOLVED", Reason: "at least one changed owner path is required"})
	}
	if len(seeds.Nodes) == 0 {
		env.Stats = model.GraphImpactStats{Unresolved: generation.UnresolvedCount}
		env.DeterminismDigest = model.GraphImpactDeterminismDigest(q, generation.GenerationID, env.Items, env.Inferred, env.Stats)
		return env, nil
	}

	frontier := make([]int, 0, len(seeds.Nodes))
	visited := map[int]bool{}
	for _, seed := range seeds.Nodes {
		if visited[seed.NodeID] {
			continue
		}
		visited[seed.NodeID] = true
		frontier = append(frontier, seed.NodeID)
	}
	paths := map[int]graphImpactPath{}
	for _, seed := range seeds.Nodes {
		paths[seed.NodeID] = graphImpactPath{}
	}
	primary := make([]model.GraphImpactItem, 0)
	primaryIndex := map[string]int{}
	inferred := make([]model.GraphImpactItem, 0)
	maxDepth := 1
	if q.Mode == model.GraphImpactModeTransitive {
		maxDepth = q.Depth
	}
	stats := model.GraphImpactStats{Seeds: len(frontier), Visited: len(frontier), Unresolved: generation.UnresolvedCount}
	depthTruncated := false
	for depth := 1; depth <= maxDepth && len(frontier) > 0; depth++ {
		if err := contextErr(ctx); err != nil {
			return model.GraphImpactEnvelope{}, err
		}
		primaryEdges, queryErr := snapshot.ImpactEdgesFromFrontier(ctx, frontier, q.Direction, q.Relations, []string{model.GraphRecordExact, model.GraphRecordExtracted}, q.Limit)
		if queryErr != nil {
			return model.GraphImpactEnvelope{}, store.SanitizeGraphQueryError(queryErr)
		}
		inferredEdges, queryErr := snapshot.ImpactEdgesFromFrontier(ctx, frontier, q.Direction, q.Relations, []string{model.GraphRecordInferred}, q.Limit)
		if queryErr != nil {
			return model.GraphImpactEnvelope{}, store.SanitizeGraphQueryError(queryErr)
		}
		next := make([]int, 0, len(primaryEdges))
		for _, edge := range primaryEdges {
			target := edge.FromNodeID
			step, stepErr := graphImpactStep(ctx, snapshot, edge)
			if stepErr != nil {
				return model.GraphImpactEnvelope{}, store.SanitizeGraphQueryError(stepErr)
			}
			parentPath := paths[edge.ToNodeID]
			candidatePath := graphImpactPath{Steps: appendImpactStep(parentPath.Steps, step)}
			candidatePath.Key = impactPathKey(candidatePath.Steps)
			if old, exists := paths[target]; exists && old.Key != "" && old.Key <= candidatePath.Key {
				continue
			}
			paths[target] = candidatePath
			if impactRelationCanTraverse(edge.Relation) && !visited[target] {
				visited[target] = true
				next = append(next, target)
			}
			node, nodeErr := snapshot.Node(ctx, target)
			if nodeErr != nil {
				return model.GraphImpactEnvelope{}, store.SanitizeGraphQueryError(nodeErr)
			}
			trigger := ""
			if parent, parentErr := snapshot.Node(ctx, edge.ToNodeID); parentErr == nil {
				trigger = parent.Identity.OwnerPath
			}
			item := impactItemFromNode(generation, node, depth, edge, trigger, candidatePath.Steps, step.EvidenceRefs)
			if index, exists := primaryIndex[item.CrossRID]; exists {
				primary[index] = item
			} else {
				primaryIndex[item.CrossRID] = len(primary)
				primary = append(primary, item)
			}
		}
		for _, edge := range inferredEdges {
			// Inferred edges are reported for explainability but never become traversal
			// frontier, even when their relation itself is transitive.
			target := edge.FromNodeID
			step, stepErr := graphImpactStep(ctx, snapshot, edge)
			if stepErr != nil {
				return model.GraphImpactEnvelope{}, store.SanitizeGraphQueryError(stepErr)
			}
			parentPath := paths[edge.ToNodeID]
			candidatePath := appendImpactStep(parentPath.Steps, step)
			node, nodeErr := snapshot.Node(ctx, target)
			if nodeErr != nil {
				return model.GraphImpactEnvelope{}, store.SanitizeGraphQueryError(nodeErr)
			}
			trigger := ""
			if parent, parentErr := snapshot.Node(ctx, edge.ToNodeID); parentErr == nil {
				trigger = parent.Identity.OwnerPath
			}
			item := impactItemFromNode(generation, node, depth, edge, trigger, candidatePath, step.EvidenceRefs)
			if !containsImpactItem(inferred, item.CrossRID) {
				inferred = append(inferred, item)
			}
		}
		stats.DepthReached = depth
		stats.Visited = len(visited)
		if q.Mode == model.GraphImpactModeDirect {
			break
		}
		frontier = uniqueImpactInts(next)
		if depth == maxDepth && len(frontier) > 0 {
			depthTruncated = true
		}
	}
	stats.Frontier = len(frontier)
	sortImpactItems(primary)
	sortImpactItems(inferred)
	primary, inferred, units, outputTruncated, nextOffset := applyImpactBudget(q, primary, inferred, offset)
	truncated := outputTruncated || depthTruncated || seedTruncated
	stats.Returned = len(primary)
	stats.Inferred = len(inferred)
	stats.TokenUnits = units
	env.Items = primary
	env.Inferred = inferred
	env.Stats = stats
	env.Truncated = truncated
	if truncated {
		env.Warnings = appendStringIfMissing(env.Warnings, "impact result truncated by limit or token budget; coverage is not complete")
	}
	if outputTruncated {
		next := encodeGraphImpactCursor(graphImpactCursor{
			Generation:    q.Generation,
			Operation:     "nav.graph-impact",
			Paths:         append([]string(nil), q.Paths...),
			Mode:          q.Mode,
			Depth:         q.Depth,
			Limit:         q.Limit,
			Token:         q.TokenBudget,
			Direction:     q.Direction,
			Relations:     append([]string(nil), q.Relations...),
			IncludeTests:  q.IncludeTests,
			IncludeDocs:   q.IncludeDocs,
			Offset:        nextOffset,
			RequestDigest: requestDigest,
		})
		env.Continuation = &model.Continuation{Reason: "graph impact result truncated", Next: model.ContinuationTarget{Op: "nav.graph-impact", Query: next}}
	}
	env.DeterminismDigest = model.GraphImpactDeterminismDigest(q, generation.GenerationID, primary, inferred, stats)
	return env, nil
}

func (a *App) graphImpact(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	q, err := graphImpactRequestFromPayload(request)
	if err != nil {
		return model.Envelope{}, err
	}
	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, &model.GraphQueryError{Code: "GPH_QUERY_BACKEND_UNAVAILABLE", Message: "graph backend is unavailable"}
	}
	db, err := openWorkspaceDB(registration, request.Operation, true)
	if err != nil {
		return model.Envelope{}, store.SanitizeGraphQueryError(err)
	}
	defer db.Close()
	impact, err := GraphImpact(ctx, db, q)
	if err != nil {
		return model.Envelope{}, store.SanitizeGraphQueryError(err)
	}
	items := make([]model.GraphImpactItem, 0, len(impact.Items)+len(impact.Inferred))
	items = append(items, impact.Items...)
	items = append(items, impact.Inferred...)
	omissions := make([]model.EnvelopeOmission, 0, len(impact.Omissions))
	for _, omission := range impact.Omissions {
		omissions = append(omissions, model.EnvelopeOmission{Path: omission.Path, Reason: omission.Reason, ErrorCode: omission.Code})
	}
	return model.Envelope{
		Ok:                 impact.Ok,
		Workspace:          registration.Name,
		Backend:            impact.Backend,
		Mode:               impact.Mode,
		Items:              items,
		Omissions:          omissions,
		Truncated:          impact.Truncated,
		Warnings:           append([]string(nil), impact.Warnings...),
		Continuation:       impact.Continuation,
		Operation:          "nav.graph-impact",
		GenerationID:       impact.GenerationID,
		GraphSchemaVersion: impact.GraphSchemaVersion,
		DeterminismDigest:  impact.DeterminismDigest,
		Stats:              model.Stats{Symbols: len(items), TokensEstimate: impact.Stats.TokenUnits},
	}, nil
}

func graphImpactRequestFromPayload(request model.CommandRequest) (model.GraphImpactRequest, error) {
	payload := request.Payload
	cursor := strings.TrimSpace(stringPayload(payload, "cursor"))
	query := strings.TrimSpace(stringPayload(payload, "query"))
	if cursor != "" && query != "" && cursor != query {
		return model.GraphImpactRequest{}, &model.GraphQueryError{Code: "GPH_QUERY_CURSOR_STALE", Message: "impact cursor and continuation query do not match"}
	}
	if cursor == "" {
		cursor = query
	}
	tokenBudget := intFromAny(payload["token_budget"], 0)
	if tokenBudget == 0 && cursor == "" {
		tokenBudget = request.Context.TokenBudget
	}
	q := model.GraphImpactRequest{
		Generation:   stringPayload(payload, "generation"),
		Mode:         stringPayload(payload, "mode"),
		Depth:        intFromAny(payload["depth"], 0),
		Limit:        intFromAny(payload["limit"], 0),
		TokenBudget:  tokenBudget,
		Direction:    stringPayload(payload, "direction"),
		IncludeTests: boolPayload(payload, "include_tests"),
		IncludeDocs:  boolPayload(payload, "include_docs"),
		Cursor:       cursor,
		Paths:        graphImpactPathsFromPayload(payload["paths"]),
		ChangedPaths: graphImpactPathsFromPayload(payload["changed_paths"]),
	}
	for _, key := range []string{"edge", "edges", "relations"} {
		switch value := payload[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				q.Relations = append(q.Relations, value)
			}
		case []string:
			q.Relations = append(q.Relations, value...)
		case []any:
			for _, item := range value {
				if relation, ok := item.(string); ok {
					q.Relations = append(q.Relations, relation)
				}
			}
		}
	}
	return q, nil
}

func graphImpactPathsFromPayload(value any) []string {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{typed}
	case []string:
		return append([]string(nil), typed...)
	case []any:
		paths := make([]string, 0, len(typed))
		for _, item := range typed {
			if path, ok := item.(string); ok {
				paths = append(paths, path)
			}
		}
		return paths
	default:
		return nil
	}
}

func graphImpactRequestDigest(q model.GraphImpactRequest) string {
	q.Cursor = ""
	q.Omissions = nil
	q.Warnings = nil
	b, _ := json.Marshal(q)
	d := sha256.Sum256(b)
	return hex.EncodeToString(d[:])
}

func impactSeedBudgetOmitted(omissions []model.GraphImpactOmission) bool {
	for _, omission := range omissions {
		if omission.Code == "GPH_IMPACT_SEED_BUDGET_EXCEEDED" {
			return true
		}
	}
	return false
}

func contextErr(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func graphImpactStep(ctx context.Context, snapshot *store.GraphQuerySnapshot, edge model.GraphEdgeRecord) (model.GraphImpactPathStep, error) {
	from, err := snapshot.Node(ctx, edge.FromNodeID)
	if err != nil {
		return model.GraphImpactPathStep{}, err
	}
	to, err := snapshot.Node(ctx, edge.ToNodeID)
	if err != nil {
		return model.GraphImpactPathStep{}, err
	}
	refs, err := snapshot.EvidenceRefs(ctx, nil, &edge.EdgeID, 32)
	if err != nil {
		return model.GraphImpactPathStep{}, err
	}
	return model.GraphImpactPathStep{EdgeCrossRID: edge.CrossRID, Relation: edge.Relation, Status: edge.ClaimStatus, FromCrossRID: from.CrossRID, ToCrossRID: to.CrossRID, EvidenceRefs: refs}, nil
}

func appendImpactStep(previous []model.GraphImpactPathStep, step model.GraphImpactPathStep) []model.GraphImpactPathStep {
	out := make([]model.GraphImpactPathStep, len(previous), len(previous)+1)
	copy(out, previous)
	return append(out, step)
}

func impactPathKey(steps []model.GraphImpactPathStep) string {
	var b strings.Builder
	for _, step := range steps {
		b.WriteString(step.Relation)
		b.WriteByte(0)
		b.WriteString(step.FromCrossRID)
		b.WriteByte(0)
		b.WriteString(step.ToCrossRID)
		b.WriteByte(0)
		b.WriteString(step.EdgeCrossRID)
		b.WriteByte(0)
	}
	return b.String()
}

func impactItemFromNode(generation model.GraphGeneration, node model.GraphNodeRecord, distance int, edge model.GraphEdgeRecord, trigger string, path []model.GraphImpactPathStep, refs []string) model.GraphImpactItem {
	return model.GraphImpactItem{
		Kind:            "node",
		Path:            node.Identity.OwnerPath,
		GenerationID:    generation.GenerationID.String(),
		CrossRID:        node.CrossRID,
		NodeKey:         node.NodeKey.String(),
		Display:         node.DisplayName,
		OwnerPath:       node.Identity.OwnerPath,
		SymbolKind:      node.Identity.SymbolKind,
		Status:          node.ClaimStatus,
		ConfidenceClass: impactConfidence(edge.ClaimStatus),
		Distance:        distance,
		Relation:        edge.Relation,
		Reason:          "impacted by " + edge.Relation + " relation",
		TriggerPath:     trigger,
		NodeID:          node.NodeID,
		EvidencePath:    path,
		EvidenceRefs:    append([]string(nil), refs...),
	}
}

func impactRelationCanTraverse(relation string) bool {
	semantics, ok := model.GraphImpactRelationSemantics(relation)
	return ok && semantics.Transitive
}

func impactConfidence(status string) string {
	switch status {
	case model.GraphRecordExact:
		return "exact"
	case model.GraphRecordExtracted:
		return "extracted"
	case model.GraphRecordInferred:
		return "inferred"
	default:
		return "unknown"
	}
}

func containsImpactItem(items []model.GraphImpactItem, crossRID string) bool {
	for _, item := range items {
		if item.CrossRID == crossRID {
			return true
		}
	}
	return false
}

func uniqueImpactInts(values []int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}

func sortImpactItems(items []model.GraphImpactItem) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Distance != b.Distance {
			return a.Distance < b.Distance
		}
		if impactClaimRank(a.ConfidenceClass) != impactClaimRank(b.ConfidenceClass) {
			return impactClaimRank(a.ConfidenceClass) < impactClaimRank(b.ConfidenceClass)
		}
		if a.Relation != b.Relation {
			return a.Relation < b.Relation
		}
		if strings.ToLower(a.Display) != strings.ToLower(b.Display) {
			return strings.ToLower(a.Display) < strings.ToLower(b.Display)
		}
		if a.Display != b.Display {
			return a.Display < b.Display
		}
		return a.NodeKey < b.NodeKey
	})
}

func impactClaimRank(class string) int {
	switch class {
	case "exact":
		return 0
	case "extracted":
		return 1
	case "inferred":
		return 2
	default:
		return 3
	}
}

func applyImpactBudget(q model.GraphImpactRequest, primary, inferred []model.GraphImpactItem, offset int) ([]model.GraphImpactItem, []model.GraphImpactItem, int, bool, int) {
	all := make([]model.GraphImpactItem, 0, len(primary)+len(inferred))
	all = append(all, primary...)
	all = append(all, inferred...)
	if offset < 0 {
		offset = 0
	}
	if offset > len(all) {
		offset = len(all)
	}
	selected := make([]model.GraphImpactItem, 0, minInt(q.Limit, len(all)-offset))
	units := 0
	for _, item := range all[offset:] {
		if len(selected) >= q.Limit {
			break
		}
		b, _ := json.Marshal(item)
		n := (len(b) + 3) / 4
		if units+n > q.TokenBudget && len(selected) > 0 {
			break
		}
		units += n
		selected = append(selected, item)
	}
	truncated := offset+len(selected) < len(all)
	primaryCount := 0
	for i := offset; i < offset+len(selected) && i < len(primary); i++ {
		primaryCount++
	}
	selectedPrimary := append([]model.GraphImpactItem(nil), selected[:primaryCount]...)
	selectedInferred := append([]model.GraphImpactItem(nil), selected[primaryCount:]...)
	return selectedPrimary, selectedInferred, units, truncated, offset + len(selected)
}
