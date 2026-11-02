package store

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// SnapshotFreshness reports the freshness of the pinned base generation while
// identifying the effective, worktree-local overlay generation to callers.
func (s *LayeredGraphSnapshot) SnapshotFreshness(requested string) model.GraphFreshness {
	if s == nil || s.closed || s.base == nil {
		return model.GraphFreshness{State: model.GraphFreshnessInvalid, ReasonCode: "snapshot_closed"}
	}
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = s.generation.GenerationID.String()
	}
	baseFreshness := s.base.SnapshotFreshness(s.base.generation.GenerationID.String())
	freshness := model.GraphFreshness{
		State:              baseFreshness.State,
		GenerationID:       s.generation.GenerationID.String(),
		ObservedGeneration: requested,
		ReasonCode:         baseFreshness.ReasonCode,
	}
	if requested != s.generation.GenerationID.String() {
		freshness.State = model.GraphFreshnessLagging
		freshness.ReasonCode = "generation_mismatch"
	}
	return freshness
}

// BoundedData returns a deterministic bounded view of the effective layered
// graph. Base rows owned by affected paths are omitted before overlay rows are
// merged, so deleted and replaced records cannot leak through.
func (s *LayeredGraphSnapshot) BoundedData(ctx context.Context, maxNodes, maxEdges int) (GraphBoundedData, error) {
	if s == nil || s.closed || s.base == nil || ctx == nil {
		return GraphBoundedData{}, model.ErrGraphGenerationInvalid
	}
	if maxNodes <= 0 || maxNodes > model.GraphAnalysisMaxNodes {
		maxNodes = model.GraphAnalysisMaxNodes
	}
	if maxEdges <= 0 || maxEdges > model.GraphAnalysisMaxEdges {
		maxEdges = model.GraphAnalysisMaxEdges
	}
	data := GraphBoundedData{Generation: s.generation}
	nodeMask, nodeArgs := s.baseUnmaskedPredicate("owner_path")
	nodeArgs = append([]any{digestArg(s.base.generation.GenerationID)}, nodeArgs...)
	deltaNodes, _, err := s.nodeLayer(ctx)
	if err != nil {
		return data, err
	}
	nodeArgs = append(nodeArgs, maxNodes+len(deltaNodes)+1)
	rows, err := s.base.query(ctx, graphNodeSelect+nodeMask+` ORDER BY node_key LIMIT ?`, nodeArgs...)
	if err != nil {
		return data, err
	}
	baseNodes := make(map[int]model.GraphNodeRecord)
	for rows.Next() {
		n, scanErr := scanGraphNode(rows, s.base.generation.GenerationID)
		if scanErr != nil {
			rows.Close()
			return data, scanErr
		}
		baseNodes[n.NodeID] = n
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return data, err
	}
	for id, node := range deltaNodes {
		baseNodes[id] = node
	}
	nodes := make([]model.GraphNodeRecord, 0, len(baseNodes))
	for _, node := range baseNodes {
		node.GenerationID = s.generation.GenerationID
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].NodeKey.String() < nodes[j].NodeKey.String() })
	if len(nodes) > maxNodes {
		data.Truncated = true
		nodes = nodes[:maxNodes]
	}
	data.Nodes = nodes

	edgeMask, edgeArgs := s.baseUnmaskedPredicate("owner_path")
	edgeArgs = append([]any{digestArg(s.base.generation.GenerationID), model.GraphRecordExact, model.GraphRecordExtracted}, edgeArgs...)
	deltaEdges, _, err := s.edgeLayer(ctx)
	if err != nil {
		return data, err
	}
	// An unmodified edge can still point at a removed/rekeyed node. Filter
	// invisible endpoints before LIMIT, while retaining replacements that
	// preserve the pinned base node's identity and ID.
	if len(s.affected) > 0 {
		visibleIDs := make([]int, 0, len(deltaNodes))
		for id := range deltaNodes {
			visibleIDs = append(visibleIDs, id)
		}
		maskedJSON, err := json.Marshal(s.affected)
		if err != nil {
			return data, err
		}
		visibleJSON, err := json.Marshal(visibleIDs)
		if err != nil {
			return data, err
		}
		edgeMask += ` AND NOT EXISTS (SELECT 1 FROM graph_nodes AS endpoint
			WHERE endpoint.generation_id=graph_edges.generation_id
			AND endpoint.node_id IN (graph_edges.from_node_id,graph_edges.to_node_id)
			AND endpoint.owner_path IN (SELECT value FROM json_each(?))
			AND endpoint.node_id NOT IN (SELECT value FROM json_each(?)))`
		edgeArgs = append(edgeArgs, string(maskedJSON), string(visibleJSON))
	}
	edgeArgs = append(edgeArgs, maxEdges+len(deltaEdges)+1)
	rows, err = s.base.query(ctx, `SELECT edge_id,edge_key,from_node_id,to_node_id,relation,claim_scope,claim_status,owner_path,source_backend,cross_rid FROM graph_edges WHERE generation_id=? AND claim_status IN (?,?)`+edgeMask+` ORDER BY edge_key LIMIT ?`, edgeArgs...)
	if err != nil {
		return data, err
	}
	baseEdges := make(map[int]model.GraphEdgeRecord)
	for rows.Next() {
		e, scanErr := graphEdgeFromRow(rows, s.base.generation.GenerationID)
		if scanErr != nil {
			rows.Close()
			return data, scanErr
		}
		baseEdges[e.EdgeID] = e
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return data, err
	}
	for id, edge := range deltaEdges {
		if edge.ClaimStatus == model.GraphRecordExact || edge.ClaimStatus == model.GraphRecordExtracted {
			baseEdges[id] = edge
		} else {
			delete(baseEdges, id)
		}
	}
	edges := make([]model.GraphEdgeRecord, 0, len(baseEdges))
	for _, edge := range baseEdges {
		edge.GenerationID = s.generation.GenerationID
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].EdgeKey.String() < edges[j].EdgeKey.String() })
	if len(edges) > maxEdges {
		data.Truncated = true
		edges = edges[:maxEdges]
	}
	data.Edges = edges
	return data, nil
}
