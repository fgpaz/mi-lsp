package store

import (
	"context"
	"database/sql"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// GraphImpactSeedResolution is the bounded, read-only result of mapping changed
// owner paths to graph nodes. Exact owner matches are required; textual or
// directory proximity is intentionally not treated as a seed.
type GraphImpactSeedResolution struct {
	Nodes     []model.GraphNodeRecord
	Omissions []model.GraphImpactOmission
}

func normalizeImpactPath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	return strings.TrimPrefix(path, "./")
}

func (s *GraphQuerySnapshot) ResolveImpactSeeds(ctx context.Context, paths []string, max int) (GraphImpactSeedResolution, error) {
	if s == nil || s.closed || ctx == nil {
		return GraphImpactSeedResolution{}, model.ErrGraphGenerationInvalid
	}
	if max < 1 {
		return GraphImpactSeedResolution{}, nil
	}
	canonical := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for _, raw := range paths {
		path := normalizeImpactPath(raw)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; !ok {
			seen[path] = struct{}{}
			canonical = append(canonical, path)
		}
	}
	sort.Strings(canonical)
	result := GraphImpactSeedResolution{}
	for _, path := range canonical {
		if len(result.Nodes) >= max {
			result.Omissions = append(result.Omissions, model.GraphImpactOmission{Path: path, Code: "GPH_IMPACT_BUDGET_EXCEEDED", Reason: "seed budget exceeded"})
			continue
		}
		rows, err := s.query(ctx, `SELECT node_id,node_key,identity_schema,repository_identity,backend_type,language,project_or_module,owner_path,symbol_kind,semantic_identity,display_name,source_digest,claim_status,cross_rid,sort_key FROM graph_nodes WHERE generation_id=? AND owner_path=? ORDER BY symbol_kind,sort_key,node_key LIMIT ?`, digestArg(s.generation.GenerationID), path, max-len(result.Nodes)+1)
		if err != nil {
			return GraphImpactSeedResolution{}, err
		}
		var nodes []model.GraphNodeRecord
		for rows.Next() {
			n, scanErr := scanGraphNode(rows, s.generation.GenerationID)
			if scanErr != nil {
				rows.Close()
				return GraphImpactSeedResolution{}, scanErr
			}
			nodes = append(nodes, n)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return GraphImpactSeedResolution{}, err
		}
		rows.Close()
		if len(nodes) == 0 {
			// Preserve unresolved graph evidence when it names the same owner.
			unresolved, queryErr := s.query(ctx, `SELECT reason_code,candidates_json,recovery_hint_code FROM graph_unresolved WHERE generation_id=? AND owner_path=? ORDER BY unresolved_id LIMIT 4`, digestArg(s.generation.GenerationID), path)
			if queryErr != nil {
				return GraphImpactSeedResolution{}, queryErr
			}
			var candidates []string
			var reason, hint sql.NullString
			for unresolved.Next() {
				var rawCandidates string
				if scanErr := unresolved.Scan(&reason, &rawCandidates, &hint); scanErr != nil {
					unresolved.Close()
					return GraphImpactSeedResolution{}, scanErr
				}
				candidates = append(candidates, DecodeGraphCandidates(rawCandidates)...)
			}
			if scanErr := unresolved.Err(); scanErr != nil {
				unresolved.Close()
				return GraphImpactSeedResolution{}, scanErr
			}
			unresolved.Close()
			code := "GPH_IMPACT_SEED_UNRESOLVED"
			why := "owner path has no resolvable graph node"
			if reason.Valid && reason.String != "" {
				why = reason.String
			}
			if hint.Valid && hint.String != "" {
				code = hint.String
			}
			result.Omissions = append(result.Omissions, model.GraphImpactOmission{Path: path, Code: code, Reason: why, Candidates: boundedStrings(candidates, 16)})
			continue
		}
		if len(result.Nodes)+len(nodes) > max {
			result.Omissions = append(result.Omissions, model.GraphImpactOmission{Path: path, Code: "GPH_IMPACT_BUDGET_EXCEEDED", Reason: "seed budget exceeded"})
			nodes = nodes[:max-len(result.Nodes)]
		}
		result.Nodes = append(result.Nodes, nodes...)
	}
	return result, nil
}

func boundedStrings(values []string, max int) []string {
	if len(values) <= max {
		return values
	}
	return values[:max]
}

// ImpactEdgesFromFrontier performs one indexed SQL frontier expansion. It
// never materializes the graph and only returns the registered relation and
// claim-status classes requested by the caller.
func (s *GraphQuerySnapshot) ImpactEdgesFromFrontier(ctx context.Context, frontier []int, direction string, relations, statuses []string, maxRows int) ([]model.GraphEdgeRecord, error) {
	if s == nil || s.closed || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	if direction != "in" && direction != "out" {
		return nil, &model.GraphQueryError{Code: "GPH_IMPACT_RELATION_UNSUPPORTED", Field: "direction", Message: "impact direction must be in or out"}
	}
	if maxRows < 1 || len(frontier) == 0 {
		return []model.GraphEdgeRecord{}, nil
	}
	ids := uniqueSortedNonNegative(frontier)
	if len(ids) > maxRows {
		ids = ids[:maxRows]
	}
	whereColumn := "from_node_id"
	if direction == "in" {
		whereColumn = "to_node_id"
	}
	args := []any{digestArg(s.generation.GenerationID)}
	placeholders := make([]string, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	where := " AND " + whereColumn + " IN (" + strings.Join(placeholders, ",") + ")"
	if len(relations) > 0 {
		placeholders = make([]string, len(relations))
		for i, relation := range relations {
			placeholders[i] = "?"
			args = append(args, strings.ToLower(strings.TrimSpace(relation)))
		}
		where += " AND relation IN (" + strings.Join(placeholders, ",") + ")"
	}
	if len(statuses) > 0 {
		placeholders = make([]string, len(statuses))
		for i, status := range statuses {
			placeholders[i] = "?"
			args = append(args, status)
		}
		where += " AND claim_status IN (" + strings.Join(placeholders, ",") + ")"
	}
	args = append(args, maxRows)
	query := `SELECT edge_id,edge_key,from_node_id,to_node_id,relation,claim_scope,claim_status,owner_path,source_backend,cross_rid FROM graph_edges WHERE generation_id=?` + where + ` ORDER BY relation,claim_status,edge_key LIMIT ?`
	rows, err := s.query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.GraphEdgeRecord, 0, minGraphQueryInt(maxRows, 32))
	for rows.Next() {
		e, scanErr := graphEdgeFromRow(rows, s.generation.GenerationID)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func uniqueSortedNonNegative(values []int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value < 0 {
			continue
		}
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	sort.Ints(out)
	return out
}

// ImpactEdges is a short alias used by query-core callers.
func (s *GraphQuerySnapshot) ImpactEdges(ctx context.Context, frontier []int, direction string, relations, statuses []string, maxRows int) ([]model.GraphEdgeRecord, error) {
	return s.ImpactEdgesFromFrontier(ctx, frontier, direction, relations, statuses, maxRows)
}
