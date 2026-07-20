package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

type graphImpactPath struct {
	Steps []model.GraphImpactPathStep
	Key   string
}

// GraphImpact executes the G6 core directly against one fixed, published
// graph snapshot. It is intentionally independent of affected/diff/CLI
// surfaces so those callers can adopt it without changing their contracts.
func GraphImpact(ctx context.Context, db *sql.DB, request model.GraphImpactRequest) (model.GraphImpactEnvelope, error) {
	if ctx == nil || db == nil {
		return model.GraphImpactEnvelope{}, &model.GraphQueryError{Code: "GPH_QUERY_BACKEND_UNAVAILABLE", Message: "graph backend is unavailable"}
	}
	q, err := request.Normalize()
	if err != nil {
		return model.GraphImpactEnvelope{}, err
	}
	snapshot, err := store.BeginGraphQuerySnapshot(ctx, db, q.Generation)
	if err != nil {
		return model.GraphImpactEnvelope{}, store.SanitizeGraphQueryError(err)
	}
	defer snapshot.Close()
	generation, err := snapshot.Generation()
	if err != nil {
		return model.GraphImpactEnvelope{}, store.SanitizeGraphQueryError(err)
	}
	seeds, err := snapshot.ResolveImpactSeeds(ctx, q.Paths, q.Limit)
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
		Omissions:          append([]model.GraphImpactOmission(nil), seeds.Omissions...),
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
			if !visited[target] {
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
	primary, inferred, units, truncated := applyImpactBudget(q, primary, inferred)
	truncated = truncated || depthTruncated
	stats.Returned = len(primary)
	stats.Inferred = len(inferred)
	stats.TokenUnits = units
	env.Items = primary
	env.Inferred = inferred
	env.Stats = stats
	env.Truncated = truncated
	if truncated {
		env.Warnings = append(env.Warnings, "impact result truncated by limit or token budget; coverage is not complete")
	}
	env.DeterminismDigest = model.GraphImpactDeterminismDigest(q, generation.GenerationID, primary, inferred, stats)
	return env, nil
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

func applyImpactBudget(q model.GraphImpactRequest, primary, inferred []model.GraphImpactItem) ([]model.GraphImpactItem, []model.GraphImpactItem, int, bool) {
	allCount := len(primary) + len(inferred)
	truncated := allCount > q.Limit
	if len(primary) > q.Limit {
		primary = primary[:q.Limit]
		inferred = nil
	} else if len(primary)+len(inferred) > q.Limit {
		inferred = inferred[:q.Limit-len(primary)]
	}
	units := 0
	selectedPrimary := primary[:0]
	for _, item := range primary {
		b, _ := json.Marshal(item)
		n := (len(b) + 3) / 4
		if units+n > q.TokenBudget && len(selectedPrimary) > 0 {
			truncated = true
			break
		}
		units += n
		selectedPrimary = append(selectedPrimary, item)
	}
	selectedInferred := inferred[:0]
	for _, item := range inferred {
		b, _ := json.Marshal(item)
		n := (len(b) + 3) / 4
		if units+n > q.TokenBudget && (len(selectedPrimary) > 0 || len(selectedInferred) > 0) {
			truncated = true
			break
		}
		units += n
		selectedInferred = append(selectedInferred, item)
	}
	if len(selectedPrimary) != len(primary) || len(selectedInferred) != len(inferred) {
		truncated = true
	}
	return selectedPrimary, selectedInferred, units, truncated
}
