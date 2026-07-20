package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

type graphCursor struct {
	Generation string   `json:"generation"`
	Operation  string   `json:"operation"`
	Selector   string   `json:"selector,omitempty"`
	From       string   `json:"from,omitempty"`
	To         string   `json:"to,omitempty"`
	Depth      int      `json:"depth"`
	Limit      int      `json:"limit"`
	Token      int      `json:"token"`
	Direction  string   `json:"direction"`
	Relations  []string `json:"relations,omitempty"`
	Offset     int      `json:"offset"`
	Checksum   string   `json:"checksum"`
}

func graphCursorDigest(c graphCursor) string {
	c.Checksum = ""
	b, _ := json.Marshal(c)
	d := sha256.Sum256(b)
	return hex.EncodeToString(d[:])
}
func encodeGraphCursor(c graphCursor) string {
	c.Checksum = graphCursorDigest(c)
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}
func decodeGraphCursor(raw string) (graphCursor, error) {
	var c graphCursor
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || json.Unmarshal(b, &c) != nil || c.Checksum == "" || c.Checksum != graphCursorDigest(c) || c.Offset < 0 {
		return c, &model.GraphQueryError{Code: "GPH_QUERY_CURSOR_STALE", Message: "cursor is invalid or stale"}
	}
	return c, nil
}

func graphRequestFromPayload(request model.CommandRequest) (model.GraphQueryRequest, error) {
	q := model.GraphQueryRequest{
		Operation:   request.Operation,
		Selector:    stringPayload(request.Payload, "selector"),
		From:        stringPayload(request.Payload, "from"),
		To:          stringPayload(request.Payload, "to"),
		Generation:  stringPayload(request.Payload, "generation"),
		Depth:       intFromAny(request.Payload["depth"], 0),
		Limit:       intFromAny(request.Payload["limit"], 0),
		TokenBudget: intFromAny(request.Payload["token_budget"], request.Context.TokenBudget),
		Direction:   stringPayload(request.Payload, "direction"),
		Cursor:      stringPayload(request.Payload, "cursor"),
	}
	if raw, ok := request.Payload["edge"].([]any); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				q.Relations = append(q.Relations, s)
			}
		}
	}
	if raw, ok := request.Payload["edge"].([]string); ok {
		q.Relations = append(q.Relations, raw...)
	}
	q, err := q.Normalize()
	if err != nil {
		return model.GraphQueryRequest{}, err
	}
	// Callers and callees are semantically call-graph queries. Their
	// operation-specific direction and relation cannot be overridden by flags.
	switch q.Operation {
	case "nav.callers":
		q.Direction = "in"
		q.Relations = []string{"calls"}
	case "nav.callees":
		q.Direction = "out"
		q.Relations = []string{"calls"}
	}
	return q, nil
}

func cursorMatches(c graphCursor, q model.GraphQueryRequest, generation string) bool {
	return c.Generation == generation && c.Operation == q.Operation && c.Selector == q.Selector && c.From == q.From && c.To == q.To && c.Depth == q.Depth && c.Limit == q.Limit && c.Token == q.TokenBudget && c.Direction == q.Direction && equalStrings(c.Relations, q.Relations)
}
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func GraphQuery(ctx context.Context, db *sql.DB, q model.GraphQueryRequest) (model.Envelope, error) {
	if ctx == nil || db == nil {
		return model.Envelope{}, &model.GraphQueryError{Code: "GPH_QUERY_GENERATION_INVALID", Message: "graph database is unavailable"}
	}
	q, err := q.Normalize()
	if err != nil {
		return model.Envelope{}, err
	}
	var cursor graphCursor
	if q.Cursor != "" {
		cursor, err = decodeGraphCursor(q.Cursor)
		if err != nil {
			return model.Envelope{}, err
		}
		if cursor.Operation != q.Operation || cursor.Selector != q.Selector || cursor.From != q.From || cursor.To != q.To || cursor.Depth != q.Depth || cursor.Limit != q.Limit || cursor.Token != q.TokenBudget || cursor.Direction != q.Direction || !equalStrings(cursor.Relations, q.Relations) {
			return model.Envelope{}, &model.GraphQueryError{Code: "GPH_QUERY_CURSOR_STALE", Message: "cursor parameters do not match request"}
		}
		if q.Generation != "" && cursor.Generation != q.Generation {
			return model.Envelope{}, &model.GraphQueryError{Code: "GPH_QUERY_CURSOR_STALE", Message: "cursor generation does not match request"}
		}
		if q.Generation == "" {
			q.Generation = cursor.Generation
		}
	}
	snapshot, err := store.BeginGraphQuerySnapshot(ctx, db, q.Generation)
	if err != nil {
		if q.Cursor != "" {
			return model.Envelope{}, &model.GraphQueryError{Code: "GPH_QUERY_CURSOR_STALE", Message: "cursor generation is no longer available", Hint: "restart the graph query"}
		}
		return model.Envelope{}, store.SanitizeGraphQueryError(err)
	}
	defer snapshot.Close()
	generation, err := snapshot.Generation()
	if err != nil {
		return model.Envelope{}, store.SanitizeGraphQueryError(err)
	}
	if q.Cursor != "" && cursor.Generation != generation.GenerationID.String() {
		return model.Envelope{}, &model.GraphQueryError{Code: "GPH_QUERY_CURSOR_STALE", Message: "cursor generation is no longer available"}
	}
	var envelope model.Envelope
	switch q.Operation {
	case "nav.graph.stats":
		envelope, err = graphStats(ctx, snapshot, q, generation)
	case "nav.graph.validate":
		envelope, err = graphValidate(ctx, snapshot, q, generation)
	case "nav.path":
		envelope, err = graphPath(ctx, snapshot, q, generation, cursor.Offset)
	default:
		envelope, err = graphNeighborhood(ctx, snapshot, q, generation, cursor.Offset)
	}
	if err != nil {
		return model.Envelope{}, store.SanitizeGraphQueryError(err)
	}
	if envelope.Omissions == nil {
		envelope.Omissions, err = snapshot.UnresolvedOmissions(ctx, q.Limit)
		if err != nil {
			return model.Envelope{}, store.SanitizeGraphQueryError(err)
		}
	}
	return envelope, nil
}

func graphStats(ctx context.Context, s *store.GraphQuerySnapshot, q model.GraphQueryRequest, g model.GraphGeneration) (model.Envelope, error) {
	counts, err := s.Stats(ctx)
	if err != nil {
		return model.Envelope{}, store.SanitizeGraphQueryError(err)
	}
	facets, err := s.FacetStats(ctx)
	if err != nil {
		return model.Envelope{}, store.SanitizeGraphQueryError(err)
	}
	item := map[string]any{"kind": "graph_stats", "generation_id": g.GenerationID.String(), "schema": g.SchemaVersion, "nodes": counts["nodes"], "edges": counts["edges"], "evidence": counts["evidence"], "unresolved": counts["unresolved"], "by_kind": facets["kind"], "by_relation": facets["relation"], "by_status": facets["status"], "by_backend": facets["backend"]}
	stats := model.GraphQueryStats{Returned: 1, Depth: q.Depth, Unresolved: counts["unresolved"]}
	items := []model.GraphQueryItem{{Kind: "graph_stats", CrossRID: g.GenerationID.String(), Display: "graph stats", Status: g.Status, Distance: 0}}
	env := graphEnvelope(q, g, []any{item}, items, stats, "")
	env.Omissions, err = s.UnresolvedOmissions(ctx, q.Limit)
	if err != nil {
		return model.Envelope{}, store.SanitizeGraphQueryError(err)
	}
	return env, nil
}
func graphValidate(ctx context.Context, s *store.GraphQuerySnapshot, q model.GraphQueryRequest, g model.GraphGeneration) (model.Envelope, error) {
	validated, err := s.Validate(ctx)
	if err != nil {
		return model.Envelope{}, store.SanitizeGraphQueryError(err)
	}
	item := map[string]any{"kind": "graph_validate", "generation_id": validated.GenerationID.String(), "schema": validated.SchemaVersion, "status": "valid", "nodes": validated.NodeCount, "edges": validated.EdgeCount, "evidence": validated.EvidenceCount, "unresolved": validated.UnresolvedCount}
	stats := model.GraphQueryStats{Returned: 1, Depth: q.Depth, Unresolved: validated.UnresolvedCount}
	items := []model.GraphQueryItem{{Kind: "graph_validate", CrossRID: validated.GenerationID.String(), Display: "graph validate", Status: "valid", Distance: 0}}
	env := graphEnvelope(q, g, []any{item}, items, stats, "")
	env.Omissions, err = s.UnresolvedOmissions(ctx, q.Limit)
	if err != nil {
		return model.Envelope{}, store.SanitizeGraphQueryError(err)
	}
	return env, nil
}

func graphNeighborhood(ctx context.Context, s *store.GraphQuerySnapshot, q model.GraphQueryRequest, g model.GraphGeneration, offset int) (model.Envelope, error) {
	nodes, _, err := s.ResolveGraphSelector(ctx, q.Selector)
	if err != nil {
		return model.Envelope{}, store.SanitizeGraphQueryError(err)
	}
	if len(nodes) == 0 {
		return graphEnvelope(q, g, []any{}, nil, model.GraphQueryStats{Depth: q.Depth}, "selector not found"), nil
	}
	if len(nodes) > 1 {
		candidates := make([]model.GraphQueryItem, 0, minInt(len(nodes), 50))
		for _, n := range nodes[:minInt(len(nodes), 50)] {
			item, itemErr := graphNodeItem(s, ctx, n, 0, nil)
			if itemErr != nil {
				return model.Envelope{}, store.SanitizeGraphQueryError(itemErr)
			}
			candidates = append(candidates, item)
		}
		return model.Envelope{}, &model.GraphQueryError{Code: "GPH_QUERY_SELECTOR_AMBIGUOUS", Message: "selector matched multiple graph nodes", Hint: "refine the selector with a NodeKey, cross-RID, or scoped name", Candidates: candidates}
	}
	root := nodes[0]
	frontier := []int{root.NodeID}
	visited := map[int]int{root.NodeID: 0}
	result := []model.GraphQueryItem{}
	seenEdges := map[int]bool{}
	frontierCount := 1
	for distance := 1; distance <= q.Depth; distance++ {
		edges, e := s.Edges(ctx, frontier, effectiveNeighborhoodDirection(q), q.Relations, q.Limit+offset+1)
		if e != nil {
			return model.Envelope{}, store.SanitizeGraphQueryError(e)
		}
		next := []int{}
		for _, edge := range edges {
			if seenEdges[edge.EdgeID] {
				continue
			}
			seenEdges[edge.EdgeID] = true
			toID := edge.ToNodeID
			if effectiveNeighborhoodDirection(q) == "in" {
				toID = edge.FromNodeID
			}
			if effectiveNeighborhoodDirection(q) == "both" && !containsInt(frontier, edge.FromNodeID) {
				toID = edge.FromNodeID
			}
			n, e := sNode(s, ctx, toID)
			if e != nil {
				return model.Envelope{}, store.SanitizeGraphQueryError(e)
			}
			item, e := graphNodeItem(s, ctx, n, distance, &edge)
			if e != nil {
				return model.Envelope{}, store.SanitizeGraphQueryError(e)
			}
			result = append(result, item)
			if _, ok := visited[toID]; !ok {
				visited[toID] = distance
				next = append(next, toID)
			}
		}
		frontier = uniqueInts(next)
		frontierCount = len(frontier)
		if len(frontier) == 0 {
			break
		}
	}
	canonical := append([]model.GraphQueryItem(nil), result...)
	sortGraphItems(canonical)
	return finalizeGraphItems(q, g, s, canonical, len(visited), frontierCount, offset), nil
}

func effectiveNeighborhoodDirection(q model.GraphQueryRequest) string {
	if q.Operation == "nav.callers" {
		return "in"
	}
	if q.Operation == "nav.callees" {
		return "out"
	}
	return q.Direction
}
func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
func uniqueInts(xs []int) []int {
	out := []int{}
	seen := map[int]bool{}
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func graphPath(ctx context.Context, s *store.GraphQuerySnapshot, q model.GraphQueryRequest, g model.GraphGeneration, offset int) (model.Envelope, error) {
	from, _, err := s.ResolveGraphSelector(ctx, q.From)
	if err != nil {
		return model.Envelope{}, store.SanitizeGraphQueryError(err)
	}
	to, _, err := s.ResolveGraphSelector(ctx, q.To)
	if err != nil {
		return model.Envelope{}, store.SanitizeGraphQueryError(err)
	}
	if len(from) == 0 || len(to) == 0 {
		return graphEnvelope(q, g, []any{}, nil, model.GraphQueryStats{Depth: q.Depth}, "path endpoint not found"), nil
	}
	if len(from) > 1 || len(to) > 1 {
		return model.Envelope{}, &model.GraphQueryError{Code: "GPH_QUERY_SELECTOR_AMBIGUOUS", Message: "path endpoint selector is ambiguous"}
	}
	start, target := from[0], to[0]
	if start.NodeID == target.NodeID {
		item, _ := graphNodeItem(s, ctx, start, 0, nil)
		return graphEnvelope(q, g, []any{item}, []model.GraphQueryItem{item}, model.GraphQueryStats{Visited: 1, Frontier: 1, Returned: 1, Depth: 0}, ""), nil
	}
	type parent struct {
		node     int
		edge     model.GraphEdgeRecord
		distance int
	}
	parents := map[int]parent{}
	visited := map[int]bool{start.NodeID: true}
	frontier := []int{start.NodeID}
	found := false
	depth := 0
	for depth < q.Depth && len(frontier) > 0 && !found {
		depth++
		edges, e := s.Edges(ctx, frontier, "out", q.Relations, q.Limit+offset+1)
		if e != nil {
			return model.Envelope{}, store.SanitizeGraphQueryError(e)
		}
		sortPathEdges(ctx, s, edges)
		next := []int{}
		for _, edge := range edges {
			if visited[edge.ToNodeID] {
				continue
			}
			visited[edge.ToNodeID] = true
			parents[edge.ToNodeID] = parent{node: edge.FromNodeID, edge: edge, distance: depth}
			next = append(next, edge.ToNodeID)
			if edge.ToNodeID == target.NodeID {
				found = true
				break
			}
		}
		frontier = uniqueInts(next)
	}
	if !found {
		return graphEnvelope(q, g, []any{}, nil, model.GraphQueryStats{Visited: len(visited), Frontier: len(frontier), Depth: depth}, "no path within depth"), nil
	}
	edges := []model.GraphEdgeRecord{}
	cur := target.NodeID
	for cur != start.NodeID {
		p := parents[cur]
		edges = append(edges, p.edge)
		cur = p.node
	}
	for i, j := 0, len(edges)-1; i < j; i, j = i+1, j-1 {
		edges[i], edges[j] = edges[j], edges[i]
	}
	items := []model.GraphQueryItem{}
	for i, edge := range edges {
		n, e := sNode(s, ctx, edge.ToNodeID)
		if e != nil {
			return model.Envelope{}, store.SanitizeGraphQueryError(e)
		}
		item, e := graphNodeItem(s, ctx, n, i+1, &edge)
		if e != nil {
			return model.Envelope{}, store.SanitizeGraphQueryError(e)
		}
		items = append(items, item)
	}
	return finalizeGraphItems(q, g, s, items, len(visited), len(frontier), offset), nil
}

func sNode(s *store.GraphQuerySnapshot, ctx context.Context, id int) (model.GraphNodeRecord, error) {
	return s.Node(ctx, id)
}

func graphNodeItem(s *store.GraphQuerySnapshot, ctx context.Context, n model.GraphNodeRecord, distance int, edge *model.GraphEdgeRecord) (model.GraphQueryItem, error) {
	item := model.GraphQueryItem{Kind: "node", CrossRID: n.CrossRID, Display: n.DisplayName, Status: n.ClaimStatus, Distance: distance, NodeKey: n.NodeKey.String(), NodeID: n.NodeID, SymbolKind: n.Identity.SymbolKind, OwnerPath: n.Identity.OwnerPath}
	var evidenceNodeID = &n.NodeID
	var evidenceEdgeID *int
	if edge != nil {
		item.Kind = "edge"
		item.EdgeKey = edge.EdgeKey.String()
		item.EdgeID = edge.EdgeID
		item.Relation = edge.Relation
		item.Status = edge.ClaimStatus
		item.CrossRID = edge.CrossRID
		evidenceEdgeID = &edge.EdgeID
		from, err := s.Node(ctx, edge.FromNodeID)
		if err != nil {
			return item, err
		}
		to, err := s.Node(ctx, edge.ToNodeID)
		if err != nil {
			return item, err
		}
		item.FromNodeKey = from.NodeKey.String()
		item.ToNodeKey = to.NodeKey.String()
		item.FromCrossRID = from.CrossRID
		item.ToCrossRID = to.CrossRID
	}
	refs, err := s.EvidenceRefs(ctx, evidenceNodeID, evidenceEdgeID, 32)
	item.EvidenceRefs = refs
	return item, err
}

func sortPathEdges(ctx context.Context, s *store.GraphQuerySnapshot, edges []model.GraphEdgeRecord) {
	if len(edges) < 2 {
		return
	}
	nodeKeys := make(map[int]string, len(edges)*2)
	for _, edge := range edges {
		if _, ok := nodeKeys[edge.FromNodeID]; !ok {
			if node, err := s.Node(ctx, edge.FromNodeID); err == nil {
				nodeKeys[edge.FromNodeID] = node.NodeKey.String()
			}
		}
		if _, ok := nodeKeys[edge.ToNodeID]; !ok {
			if node, err := s.Node(ctx, edge.ToNodeID); err == nil {
				nodeKeys[edge.ToNodeID] = node.NodeKey.String()
			}
		}
	}
	sort.SliceStable(edges, func(i, j int) bool {
		a, b := edges[i], edges[j]
		if a.Relation != b.Relation {
			return a.Relation < b.Relation
		}
		if nodeKeys[a.FromNodeID] != nodeKeys[b.FromNodeID] {
			return nodeKeys[a.FromNodeID] < nodeKeys[b.FromNodeID]
		}
		if nodeKeys[a.ToNodeID] != nodeKeys[b.ToNodeID] {
			return nodeKeys[a.ToNodeID] < nodeKeys[b.ToNodeID]
		}
		return a.EdgeKey.String() < b.EdgeKey.String()
	})
}
func sortGraphItems(items []model.GraphQueryItem) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Distance != b.Distance {
			return a.Distance < b.Distance
		}
		if claimRank(a.Status) != claimRank(b.Status) {
			return claimRank(a.Status) < claimRank(b.Status)
		}
		if a.Relation != b.Relation {
			return a.Relation < b.Relation
		}
		al, bl := strings.ToLower(a.Display), strings.ToLower(b.Display)
		if al != bl {
			return al < bl
		}
		if a.Display != b.Display {
			return a.Display < b.Display
		}
		if a.NodeKey != b.NodeKey {
			return a.NodeKey < b.NodeKey
		}
		return a.EdgeKey < b.EdgeKey
	})
}
func claimRank(s string) int {
	switch s {
	case model.GraphRecordExact:
		return 0
	case model.GraphRecordExtracted:
		return 1
	case model.GraphRecordInferred:
		return 2
	}
	return 3
}

func finalizeGraphItems(q model.GraphQueryRequest, g model.GraphGeneration, s *store.GraphQuerySnapshot, all []model.GraphQueryItem, visited, frontier, offset int) model.Envelope {
	if offset > len(all) {
		offset = len(all)
	}
	selected := all[offset:]
	if len(selected) > q.Limit {
		selected = selected[:q.Limit]
	}
	units := 0
	returned := []model.GraphQueryItem{}
	for _, item := range selected {
		b, _ := json.Marshal(item)
		n := (len(b) + 3) / 4
		// Always advance a truncated continuation. Returning zero items here
		// would emit a cursor with the same offset and make clients loop forever.
		if units+n > q.TokenBudget && len(returned) > 0 {
			break
		}
		units += n
		returned = append(returned, item)
	}
	truncated := offset+len(returned) < len(all)
	next := ""
	if truncated {
		next = encodeGraphCursor(graphCursor{Generation: g.GenerationID.String(), Operation: q.Operation, Selector: q.Selector, From: q.From, To: q.To, Depth: q.Depth, Limit: q.Limit, Token: q.TokenBudget, Direction: q.Direction, Relations: q.Relations, Offset: offset + len(returned)})
	}
	stats := model.GraphQueryStats{Visited: visited, Frontier: frontier, Returned: len(returned), Depth: q.Depth, TokenUnits: units, Unresolved: g.UnresolvedCount}
	env := graphEnvelope(q, g, nil, returned, stats, "")
	env.Items = returned
	env.Truncated = truncated
	env.Graph.NextCursor = next
	return env
}
func graphEnvelope(q model.GraphQueryRequest, g model.GraphGeneration, raw []any, canonical []model.GraphQueryItem, stats model.GraphQueryStats, hint string) model.Envelope {
	if raw == nil {
		raw = make([]any, len(canonical))
		for i := range canonical {
			raw[i] = canonical[i]
		}
	}
	meta := &model.GraphQueryMetadata{Operation: q.Operation, GenerationID: g.GenerationID.String(), Schema: g.SchemaVersion, Stats: stats}
	meta.DeterminismDigest = model.DeterminismDigest(q.Operation, g.GenerationID, canonical, stats)
	return model.Envelope{Ok: true, Backend: "sqlite-direct", Mode: "query_only", Items: raw, Hint: hint, Graph: meta, Operation: q.Operation, GenerationID: g.GenerationID.String(), GraphSchemaVersion: g.SchemaVersion, DeterminismDigest: meta.DeterminismDigest}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
