package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

var (
	ErrGraphQueryGenerationMissing = errors.New("GPH_QUERY_GENERATION_NOT_FOUND")
	ErrGraphQueryGenerationStatus  = errors.New("GPH_QUERY_GENERATION_NOT_FOUND")
	ErrGraphQuerySelectorAmbiguous = errors.New("GPH_QUERY_SELECTOR_AMBIGUOUS")
)

type GraphQuerySnapshot struct {
	tx         *sql.Tx
	generation model.GraphGeneration
	closed     bool
}

func BeginGraphQuerySnapshot(ctx context.Context, db *sql.DB, generation string) (*GraphQuerySnapshot, error) {
	if ctx == nil || db == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	fail := func(e error) (*GraphQuerySnapshot, error) { _ = tx.Rollback(); return nil, e }
	state, err := GraphRuntimeState(ctx, tx)
	if err != nil {
		return fail(err)
	}
	if state != GraphRuntimeFresh {
		return fail(&model.GraphQueryError{Code: "GPH_QUERY_GRAPH_INVALID", Message: "graph catalog is stale"})
	}
	graphCatalogGeneration, graphBound, err := workspaceMetaValueConn(ctx, tx, GraphCatalogGenerationMeta)
	if err != nil {
		return fail(err)
	}
	activeCatalogGeneration, catalogBound, err := workspaceMetaValueConn(ctx, tx, WorkspaceMetaActiveCatalogGeneration)
	if err != nil {
		return fail(err)
	}
	if graphBound && catalogBound && graphCatalogGeneration != activeCatalogGeneration {
		return fail(&model.GraphQueryError{Code: "GPH_QUERY_GRAPH_INVALID", Message: "graph catalog is stale"})
	}
	var id model.GraphDigest
	if strings.TrimSpace(generation) == "" {
		var ok bool
		var e error
		id, ok, e = activeGraphGenerationConn(ctx, tx)
		if e != nil {
			return fail(e)
		}
		if !ok {
			return fail(ErrGraphQueryGenerationMissing)
		}
	} else {
		id, err = model.ParseGraphDigest(strings.TrimSpace(generation))
		if err != nil {
			return fail(ErrGraphQueryGenerationMissing)
		}
	}
	g, err := loadGeneration(ctx, tx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return fail(ErrGraphQueryGenerationMissing)
	}
	if err != nil {
		return fail(ErrGraphQueryGenerationStatus)
	}
	if g.Status != model.GraphGenerationActive && g.Status != model.GraphGenerationRetired {
		return fail(ErrGraphQueryGenerationStatus)
	}
	return &GraphQuerySnapshot{tx: tx, generation: g}, nil
}

func (s *GraphQuerySnapshot) Generation() (model.GraphGeneration, error) {
	if s == nil || s.closed {
		return model.GraphGeneration{}, model.ErrGraphGenerationInvalid
	}
	return s.generation, nil
}
func (s *GraphQuerySnapshot) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	return s.tx.Rollback()
}

func (s *GraphQuerySnapshot) query(ctx context.Context, sqlText string, args ...any) (*sql.Rows, error) {
	if s == nil || s.closed || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	return s.tx.QueryContext(ctx, sqlText, args...)
}

func scanGraphNode(scanner interface{ Scan(...any) error }, generation model.GraphDigest) (model.GraphNodeRecord, error) {
	var n model.GraphNodeRecord
	var key, source []byte
	err := scanner.Scan(&n.NodeID, &key, &n.IdentitySchema, &n.Identity.RepositoryIdentity, &n.Identity.BackendType, &n.Identity.Language, &n.Identity.ProjectOrModule, &n.Identity.OwnerPath, &n.Identity.SymbolKind, &n.Identity.SemanticIdentity, &n.DisplayName, &source, &n.ClaimStatus, &n.CrossRID, &n.SortKey)
	if err != nil {
		return n, err
	}
	n.GenerationID = generation
	n.NodeKey, err = scanDigest(key)
	if err != nil {
		return n, err
	}
	n.SourceDigest, err = scanDigest(source)
	if err != nil {
		return n, err
	}
	return n, nil
}

const graphNodeSelect = `SELECT node_id,node_key,identity_schema,repository_identity,backend_type,language,project_or_module,owner_path,symbol_kind,semantic_identity,display_name,source_digest,claim_status,cross_rid,sort_key FROM graph_nodes WHERE generation_id=?`

func (s *GraphQuerySnapshot) Node(ctx context.Context, id int) (model.GraphNodeRecord, error) {
	if s == nil || s.closed || ctx == nil || id < 0 {
		return model.GraphNodeRecord{}, model.ErrGraphGenerationInvalid
	}
	row := s.tx.QueryRowContext(ctx, graphNodeSelect+" AND node_id=?", digestArg(s.generation.GenerationID), id)
	n, err := scanGraphNode(row, s.generation.GenerationID)
	if errors.Is(err, sql.ErrNoRows) {
		return n, ErrGraphQueryGenerationMissing
	}
	return n, err
}

func (s *GraphQuerySnapshot) nodeByID(ctx context.Context, id int) (model.GraphNodeRecord, error) {
	return s.Node(ctx, id)
}

func (s *GraphQuerySnapshot) NodesByIDs(ctx context.Context, ids []int) (map[int]model.GraphNodeRecord, error) {
	if s == nil || s.closed || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	canonical := uniqueSortedNonNegative(ids)
	result := make(map[int]model.GraphNodeRecord, len(canonical))
	for start := 0; start < len(canonical); start += 500 {
		end := start + 500
		if end > len(canonical) {
			end = len(canonical)
		}
		placeholders := make([]string, end-start)
		args := []any{digestArg(s.generation.GenerationID)}
		for i, id := range canonical[start:end] {
			placeholders[i] = "?"
			args = append(args, id)
		}
		rows, err := s.query(ctx, graphNodeSelect+" AND node_id IN ("+strings.Join(placeholders, ",")+") ORDER BY node_id", args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			n, scanErr := scanGraphNode(rows, s.generation.GenerationID)
			if scanErr != nil {
				rows.Close()
				return nil, scanErr
			}
			result[n.NodeID] = n
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return result, nil
}

func (s *GraphQuerySnapshot) EvidenceRefsByEdges(ctx context.Context, edgeIDs []int, maxEach int) (map[int][]string, error) {
	if s == nil || s.closed || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	if maxEach < 1 {
		return map[int][]string{}, nil
	}
	if maxEach > 32 {
		maxEach = 32
	}
	canonical := uniqueSortedNonNegative(edgeIDs)
	result := make(map[int][]string, len(canonical))
	for start := 0; start < len(canonical); start += 500 {
		end := start + 500
		if end > len(canonical) {
			end = len(canonical)
		}
		placeholders := make([]string, end-start)
		args := []any{digestArg(s.generation.GenerationID)}
		for i, id := range canonical[start:end] {
			placeholders[i] = "?"
			args = append(args, id)
		}
		args = append(args, maxEach)
		query := `SELECT edge_id,evidence_key FROM (SELECT edge_id,evidence_key,ROW_NUMBER() OVER (PARTITION BY edge_id ORDER BY evidence_key) AS rn FROM graph_evidence WHERE generation_id=? AND edge_id IN (` + strings.Join(placeholders, ",") + `)) WHERE rn<=? ORDER BY edge_id,evidence_key`
		rows, err := s.query(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var edgeID int
			var raw []byte
			if err := rows.Scan(&edgeID, &raw); err != nil {
				rows.Close()
				return nil, err
			}
			digest, err := scanDigest(raw)
			if err != nil {
				rows.Close()
				return nil, err
			}
			result[edgeID] = append(result[edgeID], digest.String())
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return result, nil
}

func (s *GraphQuerySnapshot) ResolveGraphSelector(ctx context.Context, selector string) ([]model.GraphNodeRecord, string, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, "", &model.GraphQueryError{Code: "GPH_QUERY_SELECTOR_INVALID", Message: "selector is required"}
	}
	queries := []struct {
		kind string
		sql  string
		arg  any
	}{}
	if len(selector) == 64 && strings.ToLower(selector) == selector {
		if key, err := model.ParseGraphDigest(selector); err == nil {
			queries = append(queries, struct {
				kind string
				sql  string
				arg  any
			}{"node_key", graphNodeSelect + " AND node_key=? ORDER BY node_key LIMIT 2", digestArg(key)})
		}
	}
	queries = append(queries,
		struct {
			kind string
			sql  string
			arg  any
		}{"cross_rid", graphNodeSelect + " AND cross_rid=? ORDER BY node_key LIMIT 51", selector},
		struct {
			kind string
			sql  string
			arg  any
		}{"semantic_identity", graphNodeSelect + " AND semantic_identity=? ORDER BY node_key LIMIT 51", selector},
		struct {
			kind string
			sql  string
			arg  any
		}{"scoped_name", graphNodeSelect + " AND sort_key=? ORDER BY owner_path, node_key LIMIT 51", selector},
		struct {
			kind string
			sql  string
			arg  any
		}{"display_name", graphNodeSelect + " AND display_name=? ORDER BY owner_path, node_key LIMIT 51", selector},
	)
	for _, query := range queries {
		rows, err := s.query(ctx, query.sql, append([]any{digestArg(s.generation.GenerationID)}, query.arg)...)
		if err != nil {
			return nil, "", err
		}
		var nodes []model.GraphNodeRecord
		for rows.Next() {
			n, e := scanGraphNode(rows, s.generation.GenerationID)
			if e != nil {
				rows.Close()
				return nil, "", e
			}
			nodes = append(nodes, n)
			if len(nodes) >= 51 {
				break
			}
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return nil, "", err
		}
		rows.Close()
		if len(nodes) > 0 {
			return nodes, query.kind, nil
		}
	}
	return []model.GraphNodeRecord{}, "", nil
}

func graphEdgeFromRow(scanner interface{ Scan(...any) error }, generation model.GraphDigest) (model.GraphEdgeRecord, error) {
	var e model.GraphEdgeRecord
	var key []byte
	err := scanner.Scan(&e.EdgeID, &key, &e.FromNodeID, &e.ToNodeID, &e.Relation, &e.ClaimScope, &e.ClaimStatus, &e.OwnerPath, &e.SourceBackend, &e.CrossRID)
	if err != nil {
		return e, err
	}
	e.GenerationID = generation
	e.EdgeKey, err = scanDigest(key)
	return e, err
}

func (s *GraphQuerySnapshot) ResolveGraphEdgeSelector(ctx context.Context, selector string) ([]model.GraphEdgeRecord, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, &model.GraphQueryError{Code: "GPH_QUERY_SELECTOR_INVALID", Message: "selector is required"}
	}
	rows, err := s.query(ctx, `SELECT edge_id,edge_key,from_node_id,to_node_id,relation,claim_scope,claim_status,owner_path,source_backend,cross_rid FROM graph_edges WHERE generation_id=? AND cross_rid=? ORDER BY edge_key LIMIT 2`, digestArg(s.generation.GenerationID), selector)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var edges []model.GraphEdgeRecord
	for rows.Next() {
		e, scanErr := graphEdgeFromRow(rows, s.generation.GenerationID)
		if scanErr != nil {
			return nil, scanErr
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

func (s *GraphQuerySnapshot) EdgesFromFrontier(ctx context.Context, frontier []int, direction string, relations []string, maxRows int) ([]model.GraphEdgeRecord, error) {
	if s == nil || s.closed || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	if direction != "in" && direction != "out" {
		return nil, &model.GraphQueryError{Code: "GPH_QUERY_BUDGET_INVALID", Field: "direction", Message: "direction must be in or out"}
	}
	if maxRows < 1 || len(frontier) == 0 {
		return []model.GraphEdgeRecord{}, nil
	}
	seen := make(map[int]struct{}, len(frontier))
	canonical := make([]int, 0, minGraphQueryInt(len(frontier), maxRows))
	for _, nodeID := range frontier {
		if nodeID < 0 {
			return nil, model.ErrGraphGenerationInvalid
		}
		if _, ok := seen[nodeID]; ok {
			continue
		}
		seen[nodeID] = struct{}{}
		canonical = append(canonical, nodeID)
	}
	sort.Ints(canonical)
	if len(canonical) > maxRows {
		canonical = canonical[:maxRows]
	}
	var result []model.GraphEdgeRecord
	for _, nodeID := range canonical {
		if len(result) >= maxRows {
			break
		}
		column := "from_node_id"
		if direction == "in" {
			column = "to_node_id"
		}
		args := []any{digestArg(s.generation.GenerationID), nodeID}
		where := " AND " + column + "=?"
		if len(relations) > 0 {
			placeholders := make([]string, len(relations))
			for i, relation := range relations {
				placeholders[i] = "?"
				args = append(args, relation)
			}
			where += " AND relation IN (" + strings.Join(placeholders, ",") + ")"
		}
		query := `SELECT edge_id,edge_key,from_node_id,to_node_id,relation,claim_scope,claim_status,owner_path,source_backend,cross_rid FROM graph_edges WHERE generation_id=?` + where + ` ORDER BY relation,claim_status,edge_key LIMIT ?`
		args = append(args, maxRows-len(result))
		rows, err := s.query(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			e, scanErr := graphEdgeFromRow(rows, s.generation.GenerationID)
			if scanErr != nil {
				rows.Close()
				return nil, scanErr
			}
			result = append(result, e)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return result, nil
}

func minGraphQueryInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *GraphQuerySnapshot) Edges(ctx context.Context, frontier []int, direction string, relations []string, maxRows int) ([]model.GraphEdgeRecord, error) {
	if direction == "both" {
		if maxRows < 1 {
			return []model.GraphEdgeRecord{}, nil
		}
		// Read each direction under the same bounded budget, then interleave the
		// results so one side cannot consume the entire both-direction budget.
		out, err := s.EdgesFromFrontier(ctx, frontier, "out", relations, maxRows)
		if err != nil {
			return nil, err
		}
		in, err := s.EdgesFromFrontier(ctx, frontier, "in", relations, maxRows)
		if err != nil {
			return nil, err
		}
		return interleaveGraphEdges(out, in, maxRows), nil
	}
	return s.EdgesFromFrontier(ctx, frontier, direction, relations, maxRows)
}

func interleaveGraphEdges(out, in []model.GraphEdgeRecord, maxRows int) []model.GraphEdgeRecord {
	if maxRows < 1 {
		return []model.GraphEdgeRecord{}
	}
	result := make([]model.GraphEdgeRecord, 0, minGraphQueryInt(maxRows, len(out)+len(in)))
	seen := make(map[int]struct{}, len(out)+len(in))
	next := func(edges []model.GraphEdgeRecord, index *int) (model.GraphEdgeRecord, bool) {
		for *index < len(edges) {
			edge := edges[*index]
			(*index)++
			if _, ok := seen[edge.EdgeID]; ok {
				continue
			}
			return edge, true
		}
		return model.GraphEdgeRecord{}, false
	}
	for i, j := 0, 0; len(result) < maxRows && (i < len(out) || j < len(in)); {
		if edge, ok := next(out, &i); ok {
			seen[edge.EdgeID] = struct{}{}
			result = append(result, edge)
		}
		if len(result) >= maxRows {
			break
		}
		if edge, ok := next(in, &j); ok {
			seen[edge.EdgeID] = struct{}{}
			result = append(result, edge)
		}
	}
	return result
}

func (s *GraphQuerySnapshot) EvidenceRefs(ctx context.Context, nodeID, edgeID *int, max int) ([]string, error) {
	if max < 1 {
		return []string{}, nil
	}
	var rows *sql.Rows
	var err error
	if edgeID != nil {
		rows, err = s.query(ctx, `SELECT evidence_key FROM graph_evidence WHERE generation_id=? AND edge_id=? ORDER BY evidence_key LIMIT ?`, digestArg(s.generation.GenerationID), *edgeID, max)
	} else if nodeID != nil {
		rows, err = s.query(ctx, `SELECT evidence_key FROM graph_evidence WHERE generation_id=? AND node_id=? ORDER BY evidence_key LIMIT ?`, digestArg(s.generation.GenerationID), *nodeID, max)
	} else {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	refs := []string{}
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		d, err := scanDigest(b)
		if err != nil {
			return nil, err
		}
		refs = append(refs, d.String())
	}
	return refs, rows.Err()
}

func (s *GraphQuerySnapshot) UnresolvedCount(ctx context.Context) (int, error) {
	var n int
	err := s.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM graph_unresolved WHERE generation_id=?`, digestArg(s.generation.GenerationID)).Scan(&n)
	return n, err
}

func (s *GraphQuerySnapshot) Stats(ctx context.Context) (map[string]int, error) {
	if s == nil || s.closed || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	var n, e, ev, u int
	for _, q := range []struct {
		sql string
		out *int
	}{
		{`SELECT node_count FROM graph_generations WHERE generation_id=?`, &n},
		{`SELECT edge_count FROM graph_generations WHERE generation_id=?`, &e},
		{`SELECT evidence_count FROM graph_generations WHERE generation_id=?`, &ev},
		{`SELECT unresolved_count FROM graph_generations WHERE generation_id=?`, &u},
	} {
		if err := s.tx.QueryRowContext(ctx, q.sql, digestArg(s.generation.GenerationID)).Scan(q.out); err != nil {
			return nil, err
		}
	}
	return map[string]int{"nodes": n, "edges": e, "evidence": ev, "unresolved": u}, nil
}

func (s *GraphQuerySnapshot) FacetStats(ctx context.Context) (map[string]map[string]int, error) {
	if s == nil || s.closed || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	facets := map[string]map[string]int{"kind": {}, "relation": {}, "status": {}, "backend": {}}
	queries := []struct {
		facet string
		sql   string
	}{
		{"kind", `SELECT symbol_kind, COUNT(*) FROM graph_nodes WHERE generation_id=? GROUP BY symbol_kind ORDER BY symbol_kind`},
		{"status", `SELECT claim_status, COUNT(*) FROM graph_nodes WHERE generation_id=? GROUP BY claim_status ORDER BY claim_status`},
		{"relation", `SELECT relation, COUNT(*) FROM graph_edges WHERE generation_id=? GROUP BY relation ORDER BY relation`},
		{"backend", `SELECT backend_type, COUNT(*) FROM graph_nodes WHERE generation_id=? GROUP BY backend_type ORDER BY backend_type`},
	}
	for _, query := range queries {
		rows, err := s.query(ctx, query.sql, digestArg(s.generation.GenerationID))
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var name string
			var count int
			if err := rows.Scan(&name, &count); err != nil {
				rows.Close()
				return nil, err
			}
			facets[query.facet][name] = count
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return facets, nil
}

func (s *GraphQuerySnapshot) UnresolvedOmissions(ctx context.Context, max int) ([]model.EnvelopeOmission, error) {
	if s == nil || s.closed || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	if max < 1 {
		return []model.EnvelopeOmission{}, nil
	}
	if max > 50 {
		max = 50
	}
	rows, err := s.query(ctx, `SELECT cross_rid,reason_code,recovery_hint_code FROM graph_unresolved WHERE generation_id=? ORDER BY unresolved_id LIMIT ?`, digestArg(s.generation.GenerationID), max)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	omissions := []model.EnvelopeOmission{}
	for rows.Next() {
		var crossRID, reason, hint sql.NullString
		if err := rows.Scan(&crossRID, &reason, &hint); err != nil {
			return nil, err
		}
		omissions = append(omissions, model.EnvelopeOmission{Input: crossRID.String, Reason: reason.String, ErrorCode: hint.String})
	}
	return omissions, rows.Err()
}

func (s *GraphQuerySnapshot) Validate(ctx context.Context) (model.GraphGeneration, error) {
	if s == nil || s.closed {
		return model.GraphGeneration{}, model.ErrGraphGenerationInvalid
	}
	return validateGraphGenerationConn(ctx, s.tx, s.generation.GenerationID)
}

func DecodeGraphCandidates(raw string) []string {
	var candidates []string
	_ = json.Unmarshal([]byte(raw), &candidates)
	return candidates
}

func SanitizeGraphQueryError(err error) error {
	if err == nil {
		return nil
	}
	var qe *model.GraphQueryError
	if errors.As(err, &qe) {
		return qe
	}
	if errors.Is(err, ErrGraphQueryGenerationMissing) || errors.Is(err, ErrGraphQueryGenerationStatus) {
		return &model.GraphQueryError{Code: "GPH_QUERY_GENERATION_NOT_FOUND", Message: "requested graph generation is not available"}
	}
	if errors.Is(err, ErrGraphQuerySelectorAmbiguous) {
		return &model.GraphQueryError{Code: "GPH_QUERY_SELECTOR_AMBIGUOUS", Message: "selector matched multiple graph nodes"}
	}
	if errors.Is(err, model.ErrGraphGenerationInvalid) || errors.Is(err, model.ErrGraphGenerationCorrupt) || errors.Is(err, model.ErrGraphEdgeInvalid) || errors.Is(err, model.ErrGraphEvidenceInvalid) || errors.Is(err, model.ErrGraphUnresolved) {
		return &model.GraphQueryError{Code: "GPH_QUERY_GRAPH_INVALID", Message: "graph generation is invalid"}
	}
	return &model.GraphQueryError{Code: "GPH_QUERY_BACKEND_UNAVAILABLE", Message: "graph backend is unavailable"}
}
