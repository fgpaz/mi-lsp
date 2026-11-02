package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// baseNodeByKey reads a single identity from the pinned generation, not the
// active generation of a later connection. Missing keys are ordinary misses.
func (s *LayeredGraphSnapshot) baseNodeByKey(ctx context.Context, key model.GraphDigest) (model.GraphNodeRecord, error) {
	return scanGraphNode(s.base.tx.QueryRowContext(ctx, graphNodeSelect+" AND node_key=?", digestArg(s.base.generation.GenerationID), digestArg(key)), s.base.generation.GenerationID)
}

// nodeLayer retains only replacements and the small reference map needed by
// the supplied observations. Unchanged target nodes are verified base anchors,
// not replacement records. Existing identities keep their numeric IDs; new
// identities get deterministic, noncolliding IDs above the pinned base maximum.
func (s *LayeredGraphSnapshot) nodeLayer(ctx context.Context) (map[int]model.GraphNodeRecord, map[string]int, error) {
	if s == nil || s.closed || s.base == nil || s.base.closed || ctx == nil {
		return nil, nil, model.ErrGraphGenerationInvalid
	}
	var maximum sql.NullInt64
	if err := s.base.tx.QueryRowContext(ctx, "SELECT MAX(node_id) FROM graph_nodes WHERE generation_id=?", digestArg(s.base.generation.GenerationID)).Scan(&maximum); err != nil {
		return nil, nil, err
	}
	last := int64(-1)
	if maximum.Valid {
		last = maximum.Int64
	}
	maxInt := int64(^uint(0) >> 1)
	if last < -1 || last > maxInt {
		return nil, nil, model.ErrGraphGenerationInvalid
	}
	type observation struct {
		node model.GraphObservationNode
		key  model.NodeKey
	}
	observations := make(map[string]observation)
	refs := make(map[string]string)
	for batchIndex, batch := range s.groups {
		for _, node := range batch.Nodes {
			key, err := model.NewNodeKey(node.Key)
			if err != nil {
				return nil, nil, err
			}
			digest, err := key.Hash()
			if err != nil {
				return nil, nil, err
			}
			hash := digest.String()
			if prior, ok := observations[hash]; ok && (prior.node.SourceDigest != node.SourceDigest || prior.node.ClaimStatus != node.ClaimStatus || prior.node.Resolution != node.Resolution || prior.node.DisplayName != node.DisplayName) {
				return nil, nil, fmt.Errorf("graph overlay node identity collision")
			}
			observations[hash] = observation{node: node, key: key}
			ref := graphOverlayRef(batchIndex, node.Ref)
			if previous, ok := refs[ref]; ok && previous != hash {
				return nil, nil, fmt.Errorf("graph overlay reference identifies different nodes")
			}
			refs[ref] = hash
		}
	}
	keys := make([]string, 0, len(observations))
	for key := range observations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	delta := make(map[int]model.GraphNodeRecord)
	ids := make(map[string]int, len(keys))
	for _, hash := range keys {
		observation := observations[hash]
		key, err := observation.key.Hash()
		if err != nil {
			return nil, nil, err
		}
		base, lookupErr := s.baseNodeByKey(ctx, key)
		if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
			return nil, nil, lookupErr
		}
		if !s.MasksNode(observation.key) {
			if _, derived := s.derivedSources[key]; derived {
				if lookupErr == nil {
					if base.Identity != observation.key || base.SourceDigest != observation.node.SourceDigest || base.ClaimStatus != observation.node.ClaimStatus || base.DisplayName != observation.node.DisplayName {
						return nil, nil, fmt.Errorf("existing derived file is incompatible with its effective code sources")
					}
					ids[hash] = base.NodeID
					continue
				}
			} else {
				if lookupErr != nil || s.MasksNode(base.Identity) || base.SourceDigest != observation.node.SourceDigest || base.ClaimStatus != observation.node.ClaimStatus {
					return nil, nil, fmt.Errorf("graph overlay external node is not an unchanged pinned base anchor")
				}
				ids[hash] = base.NodeID
				continue
			}
		}
		id := base.NodeID
		if errors.Is(lookupErr, sql.ErrNoRows) {
			if last == maxInt {
				return nil, nil, model.ErrGraphGenerationInvalid
			}
			last++
			id = int(last)
		}
		delta[id] = model.GraphNodeRecord{GenerationID: s.generation.GenerationID, NodeID: id, NodeKey: key, Identity: observation.key, IdentitySchema: "milsp-node-key/v1", DisplayName: observation.node.DisplayName, SourceDigest: observation.node.SourceDigest, ClaimStatus: observation.node.ClaimStatus, CrossRID: model.NodeRID(key), SortKey: hash}
		ids[hash] = id
	}
	resolved := make(map[string]int, len(refs))
	for ref, key := range refs {
		resolved[ref] = ids[key]
	}
	return delta, resolved, nil
}

func (s *LayeredGraphSnapshot) Node(ctx context.Context, id int) (model.GraphNodeRecord, error) {
	if id < 0 {
		return model.GraphNodeRecord{}, model.ErrGraphGenerationInvalid
	}
	nodes, _, err := s.nodeLayer(ctx)
	if err != nil {
		return model.GraphNodeRecord{}, err
	}
	if node, ok := nodes[id]; ok {
		return node, nil
	}
	node, err := s.base.Node(ctx, id)
	if err != nil {
		return model.GraphNodeRecord{}, err
	}
	if s.MasksNode(node.Identity) {
		return model.GraphNodeRecord{}, ErrGraphQueryGenerationMissing
	}
	node.GenerationID = s.generation.GenerationID
	return node, nil
}

func (s *LayeredGraphSnapshot) NodesByIDs(ctx context.Context, requested []int) (map[int]model.GraphNodeRecord, error) {
	delta, _, err := s.nodeLayer(ctx)
	if err != nil {
		return nil, err
	}
	ids := uniqueSortedNonNegative(requested)
	missing := make([]int, 0, len(ids))
	result := make(map[int]model.GraphNodeRecord, len(ids))
	for _, id := range ids {
		if node, ok := delta[id]; ok {
			result[id] = node
		} else {
			missing = append(missing, id)
		}
	}
	base, err := s.base.NodesByIDs(ctx, missing)
	if err != nil {
		return nil, err
	}
	for id, node := range base {
		if !s.MasksNode(node.Identity) {
			node.GenerationID = s.generation.GenerationID
			result[id] = node
		}
	}
	return result, nil
}

func (s *LayeredGraphSnapshot) edgeLayer(ctx context.Context) (map[int]model.GraphEdgeRecord, map[string]int, error) {
	nodes, refs, err := s.nodeLayer(ctx)
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[int]model.GraphNodeRecord, len(nodes))
	for id, n := range nodes {
		byID[id] = n
	}
	var max sql.NullInt64
	if err := s.base.tx.QueryRowContext(ctx, "SELECT MAX(edge_id) FROM graph_edges WHERE generation_id=?", digestArg(s.base.generation.GenerationID)).Scan(&max); err != nil {
		return nil, nil, err
	}
	last := int64(-1)
	if max.Valid {
		last = max.Int64
	}
	maxInt := int64(^uint(0) >> 1)
	if last < -1 || last > maxInt {
		return nil, nil, model.ErrGraphGenerationInvalid
	}
	type candidate struct {
		batch    int
		obs      model.GraphObservationEdge
		from, to int
		key      model.GraphDigest
	}
	candidates := map[string]candidate{}
	localRefs := map[string]string{}
	for bi, batch := range s.groups {
		for _, edge := range batch.Edges {
			from, fok := refs[graphOverlayRef(bi, edge.FromRef)]
			to, tok := refs[graphOverlayRef(bi, edge.ToRef)]
			if !fok || !tok {
				return nil, nil, fmt.Errorf("graph overlay edge endpoint is unresolved")
			}
			fn, fok := byID[from]
			if !fok {
				fn, err = s.base.Node(ctx, from)
				if err != nil || s.MasksNode(fn.Identity) {
					return nil, nil, fmt.Errorf("graph overlay from anchor is not visible")
				}
			}
			tn, tok := byID[to]
			if !tok {
				tn, err = s.base.Node(ctx, to)
				if err != nil || s.MasksNode(tn.Identity) {
					return nil, nil, fmt.Errorf("graph overlay to anchor is not visible")
				}
			}
			key := model.EdgeKey(fn.NodeKey, tn.NodeKey, edge.Relation, edge.Scope)
			h := key.String()
			if prior, ok := candidates[h]; ok && (prior.obs.OwnerPath != edge.OwnerPath || prior.obs.SourceDigest != edge.SourceDigest || prior.obs.Status != edge.Status || prior.obs.Backend != edge.Backend || prior.obs.Resolution != edge.Resolution) {
				return nil, nil, fmt.Errorf("graph overlay edge identity collision")
			}
			ref := graphOverlayRef(bi, edge.Ref)
			if prior, ok := localRefs[ref]; ok && prior != h {
				return nil, nil, fmt.Errorf("graph overlay edge reference identifies different edges")
			}
			localRefs[ref] = h
			candidates[h] = candidate{batch: bi, obs: edge, from: from, to: to, key: key}
		}
	}
	keys := make([]string, 0, len(candidates))
	for k := range candidates {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	delta := map[int]model.GraphEdgeRecord{}
	edgeRefs := map[string]int{}
	ids := map[string]int{}
	for _, h := range keys {
		c := candidates[h]
		var id int
		err := s.base.tx.QueryRowContext(ctx, `SELECT edge_id FROM graph_edges WHERE generation_id=? AND edge_key=?`, digestArg(s.base.generation.GenerationID), digestArg(c.key)).Scan(&id)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, nil, err
		}
		if errors.Is(err, sql.ErrNoRows) {
			if last == maxInt {
				return nil, nil, model.ErrGraphGenerationInvalid
			}
			last++
			id = int(last)
		}
		delta[id] = model.GraphEdgeRecord{GenerationID: s.generation.GenerationID, EdgeID: id, EdgeKey: c.key, FromNodeID: c.from, ToNodeID: c.to, Relation: c.obs.Relation, ClaimScope: c.obs.Scope, ClaimStatus: c.obs.Status, OwnerPath: c.obs.OwnerPath, SourceBackend: c.obs.Backend, CrossRID: model.EdgeRID(c.key)}
		ids[h] = id
	}
	for ref, hash := range localRefs {
		edgeRefs[ref] = ids[hash]
	}
	return delta, edgeRefs, nil
}

func overlaySortEdges(edges []model.GraphEdgeRecord) {
	sort.Slice(edges, func(i, j int) bool {
		a, b := edges[i], edges[j]
		if a.Relation != b.Relation {
			return a.Relation < b.Relation
		}
		if a.ClaimStatus != b.ClaimStatus {
			return a.ClaimStatus < b.ClaimStatus
		}
		return a.EdgeKey.String() < b.EdgeKey.String()
	})
}

func (s *LayeredGraphSnapshot) ResolveGraphSelector(ctx context.Context, selector string) ([]model.GraphNodeRecord, string, error) {
	if s == nil || s.closed || ctx == nil {
		return nil, "", model.ErrGraphGenerationInvalid
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, "", &model.GraphQueryError{Code: "GPH_QUERY_SELECTOR_INVALID", Message: "selector is required"}
	}
	delta, _, err := s.nodeLayer(ctx)
	if err != nil {
		return nil, "", err
	}
	for _, q := range []struct{ kind, col string }{{"node_key", "node_key"}, {"doc_path", "owner_path"}, {"doc_id", "semantic_identity"}, {"cross_rid", "cross_rid"}, {"semantic_identity", "semantic_identity"}, {"scoped_name", "sort_key"}, {"display_name", "display_name"}} {
		if q.kind == "node_key" && (len(selector) != 64 || strings.ToLower(selector) != selector) {
			continue
		}
		var arg any = selector
		if q.kind == "node_key" {
			d, e := model.ParseGraphDigest(selector)
			if e != nil {
				continue
			}
			arg = digestArg(d)
		}
		where := ""
		if q.kind == "doc_path" || q.kind == "doc_id" {
			where = " AND symbol_kind='document'"
		}
		mask, maskArgs := s.baseNodeUnmaskedPredicate()
		qargs := []any{digestArg(s.base.generation.GenerationID), arg}
		qargs = append(qargs, maskArgs...)
		rows, e := s.base.query(ctx, graphNodeSelect+where+" AND "+q.col+"=?"+mask+" ORDER BY owner_path,node_key LIMIT 51", qargs...)
		if e != nil {
			return nil, "", e
		}
		found := []model.GraphNodeRecord{}
		for rows.Next() {
			n, se := scanGraphNode(rows, s.generation.GenerationID)
			if se != nil {
				rows.Close()
				return nil, "", se
			}
			if !s.MasksNode(n.Identity) {
				n.GenerationID = s.generation.GenerationID
				found = append(found, n)
				if len(found) >= 51 {
					break
				}
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, "", e
		}
		for _, n := range delta {
			match := false
			switch q.kind {
			case "node_key":
				d, _ := model.ParseGraphDigest(selector)
				match = n.NodeKey == d
			case "doc_path":
				match = n.Identity.SymbolKind == "document" && n.Identity.OwnerPath == selector
			case "doc_id":
				match = n.Identity.SymbolKind == "document" && n.Identity.SemanticIdentity == selector
			case "cross_rid":
				match = n.CrossRID == selector
			case "semantic_identity":
				match = n.Identity.SemanticIdentity == selector
			case "scoped_name":
				match = n.SortKey == selector
			case "display_name":
				match = n.DisplayName == selector
			}
			if match {
				found = append(found, n)
			}
		}
		if len(found) > 0 {
			sort.Slice(found, func(i, j int) bool {
				if found[i].Identity.OwnerPath != found[j].Identity.OwnerPath {
					return found[i].Identity.OwnerPath < found[j].Identity.OwnerPath
				}
				return found[i].NodeKey.String() < found[j].NodeKey.String()
			})
			if len(found) > 51 {
				found = found[:51]
			}
			return found, q.kind, nil
		}
	}
	return []model.GraphNodeRecord{}, "", nil
}

func (s *LayeredGraphSnapshot) ResolveGraphEdgeSelector(ctx context.Context, selector string) ([]model.GraphEdgeRecord, error) {
	if s == nil || s.closed || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, &model.GraphQueryError{Code: "GPH_QUERY_SELECTOR_INVALID", Message: "selector is required"}
	}
	delta, _, err := s.edgeLayer(ctx)
	if err != nil {
		return nil, err
	}
	mask, maskArgs := s.baseEdgeUnmaskedPredicate("graph_edges")
	args := []any{digestArg(s.base.generation.GenerationID), selector}
	args = append(args, maskArgs...)
	rows, err := s.base.query(ctx, `SELECT edge_id,edge_key,from_node_id,to_node_id,relation,claim_scope,claim_status,owner_path,source_backend,cross_rid FROM graph_edges WHERE generation_id=? AND cross_rid=?`+mask+` ORDER BY edge_key`, args...)
	if err != nil {
		return nil, err
	}
	baseEdges := []model.GraphEdgeRecord{}
	for rows.Next() {
		e, se := graphEdgeFromRow(rows, s.generation.GenerationID)
		if se != nil {
			rows.Close()
			return nil, se
		}
		baseEdges = append(baseEdges, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	endpointIDs := make([]int, 0, len(baseEdges)*2)
	for _, e := range baseEdges {
		endpointIDs = append(endpointIDs, e.FromNodeID, e.ToNodeID)
	}
	visibleNodes, err := s.NodesByIDs(ctx, endpointIDs)
	if err != nil {
		return nil, err
	}
	out := []model.GraphEdgeRecord{}
	for _, e := range baseEdges {
		if _, replaced := delta[e.EdgeID]; replaced {
			continue
		}
		if !s.MasksGraphEdge(ctx, e) {
			if _, ok := visibleNodes[e.FromNodeID]; ok {
				if _, ok := visibleNodes[e.ToNodeID]; ok {
					e.GenerationID = s.generation.GenerationID
					out = append(out, e)
				}
			}
		}
	}
	for _, e := range delta {
		if e.CrossRID == selector {
			out = append(out, e)
		}
	}
	overlaySortEdges(out)
	if len(out) > 2 {
		out = out[:2]
	}
	return out, nil
}

func (s *LayeredGraphSnapshot) EdgesFromFrontier(ctx context.Context, frontier []int, direction string, relations []string, maxRows int) ([]model.GraphEdgeRecord, error) {
	if s == nil || s.closed || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	if direction != "in" && direction != "out" {
		return nil, &model.GraphQueryError{Code: "GPH_QUERY_BUDGET_INVALID", Field: "direction", Message: "direction must be in or out"}
	}
	if maxRows < 1 || len(frontier) == 0 {
		return []model.GraphEdgeRecord{}, nil
	}
	for _, id := range frontier {
		if id < 0 {
			return nil, model.ErrGraphGenerationInvalid
		}
	}
	ids := uniqueSortedNonNegative(frontier)
	if len(ids) > maxRows {
		ids = ids[:maxRows]
	}
	wanted := map[int]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	delta, _, err := s.edgeLayer(ctx)
	if err != nil {
		return nil, err
	}
	nodes, _, err := s.nodeLayer(ctx)
	if err != nil {
		return nil, err
	}
	visible := map[int]bool{}
	for id := range nodes {
		visible[id] = true
	}
	pivot := "from_node_id"
	if direction == "in" {
		pivot = "to_node_id"
	}
	placeholders := make([]string, len(ids))
	args := []any{digestArg(s.base.generation.GenerationID)}
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	rows, err := s.base.query(ctx, `SELECT edge_id,edge_key,from_node_id,to_node_id,relation,claim_scope,claim_status,owner_path,source_backend,cross_rid FROM graph_edges WHERE generation_id=? AND `+pivot+` IN (`+strings.Join(placeholders, ",")+`) ORDER BY relation,claim_status,edge_key`, args...)
	if err != nil {
		return nil, err
	}
	out := []model.GraphEdgeRecord{}
	appendIf := func(e model.GraphEdgeRecord, base bool) {
		pivot := e.FromNodeID
		if direction == "in" {
			pivot = e.ToNodeID
		}
		if !wanted[pivot] || (base && s.MasksGraphEdge(ctx, e)) {
			return
		}
		if !visible[e.FromNodeID] {
			n, ne := s.base.Node(ctx, e.FromNodeID)
			if ne != nil || s.MasksNode(n.Identity) {
				return
			}
		}
		if !visible[e.ToNodeID] {
			n, ne := s.base.Node(ctx, e.ToNodeID)
			if ne != nil || s.MasksNode(n.Identity) {
				return
			}
		}
		if len(relations) > 0 {
			ok := false
			for _, r := range relations {
				if e.Relation == r {
					ok = true
					break
				}
			}
			if !ok {
				return
			}
		}
		out = append(out, e)
	}
	for rows.Next() {
		e, se := graphEdgeFromRow(rows, s.generation.GenerationID)
		if se != nil {
			rows.Close()
			return nil, se
		}
		if _, replaced := delta[e.EdgeID]; replaced {
			continue
		}
		appendIf(e, true)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for _, e := range delta {
		appendIf(e, false)
	}
	overlaySortEdges(out)
	if len(out) > maxRows {
		out = out[:maxRows]
	}
	return out, nil
}

func (s *LayeredGraphSnapshot) Edges(ctx context.Context, frontier []int, direction string, relations []string, maxRows int) ([]model.GraphEdgeRecord, error) {
	if direction != "both" {
		return s.EdgesFromFrontier(ctx, frontier, direction, relations, maxRows)
	}
	if maxRows < 1 {
		return []model.GraphEdgeRecord{}, nil
	}
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

func nonGoOverlayPaths(paths []string) []string {
	var result []string
	for _, path := range paths {
		if !strings.HasPrefix(path, "@package/") && !strings.HasSuffix(strings.ToLower(path), ".go") {
			result = append(result, path)
		}
	}
	return result
}

func (s *LayeredGraphSnapshot) baseNodeMaskPredicate() (string, []any) {
	var clauses []string
	var args []any
	paths := nonGoOverlayPaths(s.affected)
	if len(paths) > 0 {
		marks := make([]string, len(paths))
		for i, p := range paths {
			marks[i], args = "?", append(args, p)
		}
		clauses = append(clauses, "(backend_type<>'go' AND owner_path IN ("+strings.Join(marks, ",")+"))")
	}
	for _, domain := range s.worktreeDomains {
		owners := make([]string, 0, len(domain.OwnerMasks))
		for _, mask := range domain.OwnerMasks {
			owners = append(owners, mask.OwnerPath)
		}
		owners = graphUniqueSorted(owners)
		if len(owners) == 0 {
			continue
		}
		marks := make([]string, len(owners))
		clauses = append(clauses, "(backend_type='go' AND repository_identity=? AND project_or_module=? AND owner_path IN ("+strings.Join(marks, ",")+"))")
		args = append(args, domain.RepositoryIdentity, domain.ProjectOrModule)
		for i, owner := range owners {
			marks[i], args = "?", append(args, owner)
		}
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return "(" + strings.Join(clauses, " OR ") + ")", args
}

func (s *LayeredGraphSnapshot) baseNodeUnmaskedPredicate() (string, []any) {
	mask, args := s.baseNodeMaskPredicate()
	if mask == "" {
		return "", nil
	}
	return " AND NOT " + mask, args
}

func (s *LayeredGraphSnapshot) baseEdgeUnmaskedPredicate(table string) (string, []any) {
	if table != "graph_edges" && table != "e" {
		return "", nil
	}
	var clauses []string
	var args []any
	paths := nonGoOverlayPaths(s.affected)
	if len(paths) > 0 {
		marks := make([]string, len(paths))
		for i, p := range paths {
			marks[i], args = "?", append(args, p)
		}
		clauses = append(clauses, "("+table+".source_backend<>'go' AND "+table+".owner_path IN ("+strings.Join(marks, ",")+"))")
	}
	for _, domain := range s.worktreeDomains {
		owners := make([]string, 0, len(domain.OwnerMasks))
		for _, mask := range domain.OwnerMasks {
			owners = append(owners, mask.OwnerPath)
		}
		owners = graphUniqueSorted(owners)
		if len(owners) == 0 {
			continue
		}
		for _, owner := range owners {
			clauses = append(clauses, "("+table+".source_backend='go' AND "+table+".owner_path=? AND EXISTS (SELECT 1 FROM graph_nodes AS go_owner WHERE go_owner.generation_id="+table+".generation_id AND go_owner.owner_path="+table+".owner_path AND go_owner.node_id IN ("+table+".from_node_id,"+table+".to_node_id) AND go_owner.backend_type='go' AND go_owner.repository_identity=? AND go_owner.project_or_module=?))")
			args = append(args, owner, domain.RepositoryIdentity, domain.ProjectOrModule)
		}
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " AND NOT (" + strings.Join(clauses, " OR ") + ")", args
}

func (s *LayeredGraphSnapshot) baseUnresolvedUnmaskedPredicate() (string, []any) {
	var clauses []string
	var args []any
	paths := nonGoOverlayPaths(s.affected)
	if len(paths) > 0 {
		marks := make([]string, len(paths))
		for i, path := range paths {
			marks[i], args = "?", append(args, path)
		}
		clauses = append(clauses, "(backend<>'go' AND owner_path IN ("+strings.Join(marks, ",")+"))")
	}
	for _, domain := range s.worktreeDomains {
		for _, owner := range graphWorktreeOwnerPaths(domain) {
			clauses = append(clauses, "(backend='go' AND owner_path=? AND EXISTS (SELECT 1 FROM graph_nodes AS go_owner WHERE go_owner.generation_id=graph_unresolved.generation_id AND go_owner.owner_path=graph_unresolved.owner_path AND go_owner.backend_type='go' AND go_owner.repository_identity=? AND go_owner.project_or_module=?))")
			args = append(args, owner, domain.RepositoryIdentity, domain.ProjectOrModule)
		}
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " AND NOT (" + strings.Join(clauses, " OR ") + ")", args
}

func (s *LayeredGraphSnapshot) baseEvidenceUnmaskedPredicate() (string, []any) {
	var clauses []string
	var args []any
	paths := nonGoOverlayPaths(s.affected)
	if len(paths) > 0 {
		marks := make([]string, len(paths))
		for i, path := range paths {
			marks[i], args = "?", append(args, path)
		}
		clauses = append(clauses, "(ge.backend<>'go' AND ge.source_uri IN ("+strings.Join(marks, ",")+"))")
	}
	for _, domain := range s.worktreeDomains {
		for _, owner := range graphWorktreeOwnerPaths(domain) {
			clauses = append(clauses, "(ge.backend='go' AND ((ge.node_id IS NOT NULL AND gn.backend_type='go' AND gn.repository_identity=? AND gn.project_or_module=? AND gn.owner_path=?) OR (ge.edge_id IS NOT NULL AND be.source_backend='go' AND be.owner_path=? AND EXISTS (SELECT 1 FROM graph_nodes AS go_owner WHERE go_owner.generation_id=be.generation_id AND go_owner.owner_path=be.owner_path AND go_owner.node_id IN (be.from_node_id,be.to_node_id) AND go_owner.backend_type='go' AND go_owner.repository_identity=? AND go_owner.project_or_module=?))))")
			args = append(args, domain.RepositoryIdentity, domain.ProjectOrModule, owner, owner, domain.RepositoryIdentity, domain.ProjectOrModule)
		}
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " AND NOT (" + strings.Join(clauses, " OR ") + ")", args
}

func graphWorktreeOwnerPaths(domain model.GraphWorktreeDomain) []string {
	owners := make([]string, 0, len(domain.OwnerMasks))
	for _, mask := range domain.OwnerMasks {
		owners = append(owners, mask.OwnerPath)
	}
	return graphUniqueSorted(owners)
}

func graphUniqueSorted(values []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func (s *LayeredGraphSnapshot) EvidenceRefs(ctx context.Context, nodeID, edgeID *int, max int) ([]string, error) {
	if s == nil || s.closed || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	if max < 1 {
		return []string{}, nil
	}
	if (nodeID == nil) == (edgeID == nil) {
		return []string{}, nil
	}
	deltaEdges, _, err := s.edgeLayer(ctx)
	if err != nil {
		return nil, err
	}
	deltaNodes, _, err := s.nodeLayer(ctx)
	if err != nil {
		return nil, err
	}
	refs := []string{}
	column := "node_id"
	var target int
	if nodeID != nil {
		target = *nodeID
	} else {
		column = "edge_id"
		target = *edgeID
	}
	if target < 0 {
		return nil, model.ErrGraphGenerationInvalid
	}
	// Canonical overlay refs hash the subject and evidence identity, so a
	// persisted evidence_key prefix is NOT a correct top-max candidate set.
	// Bound work independently of max and fail explicitly on overflow rather
	// than silently returning the wrong canonical ordering or alias subset.
	const maximumBaseCandidates = 4096
	rows, err := s.base.query(ctx, `SELECT source_digest,observed_claim_digest,source_uri,claim_kind,backend,extractor_version,COALESCE(start_line,0),COALESCE(start_column,0),COALESCE(end_line,0),COALESCE(end_column,0),node_id,edge_id,subject_kind FROM graph_evidence WHERE generation_id=? AND `+column+`=? ORDER BY evidence_key LIMIT ?`, digestArg(s.base.generation.GenerationID), target, maximumBaseCandidates+1)
	if err != nil {
		return nil, err
	}
	baseCandidates := 0
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		if baseCandidates == maximumBaseCandidates {
			rows.Close()
			return nil, &model.GraphQueryError{Code: "GPH_QUERY_BUDGET_EXCEEDED", Field: "base_evidence_candidates", Message: "explicit_incomplete: base evidence candidates exceed the 4096-candidate query budget"}
		}
		baseCandidates++
		var sourceRaw, observedRaw []byte
		var owner, claimKind, backend, extractorVersion, subjectKind string
		var sl, sc, el, ec int
		var subjectNode, subjectEdge sql.NullInt64
		if err = rows.Scan(&sourceRaw, &observedRaw, &owner, &claimKind, &backend, &extractorVersion, &sl, &sc, &el, &ec, &subjectNode, &subjectEdge, &subjectKind); err != nil {
			rows.Close()
			return nil, err
		}
		if s.MasksSemanticOwner(backend, owner) {
			continue
		}
		var subjectKey model.GraphDigest
		if subjectNode.Valid == subjectEdge.Valid {
			rows.Close()
			return nil, model.ErrGraphEvidenceInvalid
		}
		if subjectNode.Valid {
			if _, shadowed := deltaNodes[int(subjectNode.Int64)]; shadowed {
				continue
			}
			n, e := s.base.Node(ctx, int(subjectNode.Int64))
			if e != nil || s.MasksNode(n.Identity) {
				continue
			}
			subjectKey = n.NodeKey
		}
		if subjectEdge.Valid {
			if _, shadowed := deltaEdges[int(subjectEdge.Int64)]; shadowed {
				continue
			}
			var edge model.GraphEdgeRecord
			var edgeKeyRaw []byte
			e := s.base.tx.QueryRowContext(ctx, `SELECT edge_id,edge_key,from_node_id,to_node_id,relation,claim_scope,claim_status,owner_path,source_backend,cross_rid FROM graph_edges WHERE generation_id=? AND edge_id=?`, digestArg(s.base.generation.GenerationID), int(subjectEdge.Int64)).Scan(&edge.EdgeID, &edgeKeyRaw, &edge.FromNodeID, &edge.ToNodeID, &edge.Relation, &edge.ClaimScope, &edge.ClaimStatus, &edge.OwnerPath, &edge.SourceBackend, &edge.CrossRID)
			if e != nil || s.MasksGraphEdge(ctx, edge) {
				continue
			}
			visible, e := s.NodesByIDs(ctx, []int{edge.FromNodeID, edge.ToNodeID})
			if e != nil {
				rows.Close()
				return nil, e
			}
			if _, ok := visible[edge.FromNodeID]; !ok {
				continue
			}
			if _, ok := visible[edge.ToNodeID]; !ok {
				continue
			}
			subjectKey, e = scanDigest(edgeKeyRaw)
			if e != nil {
				rows.Close()
				return nil, e
			}
		}
		source, e := scanDigest(sourceRaw)
		if e != nil {
			rows.Close()
			return nil, e
		}
		observed, e := scanDigest(observedRaw)
		if e != nil {
			rows.Close()
			return nil, e
		}
		d := model.EvidenceDigest(source, observed, owner, claimKind, backend, extractorVersion, sl, sc, el, ec)
		identity := graphOverlayEvidenceIdentity(subjectKind, subjectKey, d)
		refs = append(refs, graphOverlayEvidenceRef(identity))
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	overlayEvidence, err := s.effectiveGraphOverlayEvidenceLayer(ctx)
	if err != nil {
		return nil, err
	}
	for identity, entry := range overlayEvidence {
		if (nodeID != nil && entry.kind == "node" && entry.nodeID == target) || (edgeID != nil && entry.kind == "edge" && entry.edgeID == target) {
			refs = append(refs, graphOverlayEvidenceRef(identity))
		}
	}
	sort.Strings(refs)
	refs = uniqueGraphOverlayStrings(refs)
	if len(refs) > max {
		refs = refs[:max]
	}
	return refs, nil
}

func uniqueGraphOverlayStrings(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := in[:1]
	for _, v := range in[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}

func (s *LayeredGraphSnapshot) EvidenceRefsByEdges(ctx context.Context, edgeIDs []int, maxEach int) (map[int][]string, error) {
	if s == nil || s.closed || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	if maxEach < 1 {
		return map[int][]string{}, nil
	}
	if maxEach > 32 {
		maxEach = 32
	}
	out := map[int][]string{}
	for _, id := range uniqueSortedNonNegative(edgeIDs) {
		refs, err := s.EvidenceRefs(ctx, nil, &id, maxEach)
		if err != nil {
			return nil, err
		}
		if len(refs) > 0 {
			out[id] = refs
		}
	}
	return out, nil
}

type graphOverlayEvidence struct {
	module         string
	kind           string
	subjectKey     model.GraphDigest
	evidenceDigest model.GraphDigest
	nodeID         int
	edgeID         int
	payload        model.GraphObservationEvidence
}

func graphOverlayEvidenceIdentity(kind string, subjectKey, evidenceDigest model.GraphDigest) string {
	return kind + ":" + subjectKey.String() + ":" + evidenceDigest.String()
}

func graphOverlayEvidenceRef(identity string) string {
	digest := model.GraphDigest(sha256.Sum256([]byte("MILSP-OVERLAY-EVIDENCE/v1:" + identity)))
	return "milsp:gph-evidence-overlay:v1:" + digest.String()
}

func graphOverlayObservationEvidenceDigest(e model.GraphObservationEvidence) model.GraphDigest {
	sl, sc, el, ec := 0, 0, 0, 0
	if e.Range != nil {
		sl, sc, el, ec = e.Range.StartLine, e.Range.StartColumn, e.Range.EndLine, e.Range.EndColumn
	}
	return model.EvidenceDigest(e.SourceDigest, e.ObservedDigest, e.SourceURI, e.ClaimKind, e.Backend, e.ExtractorVersion, sl, sc, el, ec)
}

func (s *LayeredGraphSnapshot) graphOverlayEvidenceLayer(ctx context.Context) (map[string]graphOverlayEvidence, error) {
	_, nodeRefs, err := s.nodeLayer(ctx)
	if err != nil {
		return nil, err
	}
	_, edgeRefs, err := s.edgeLayer(ctx)
	if err != nil {
		return nil, err
	}
	entries := map[string]graphOverlayEvidence{}
	for bi, batch := range s.groups {
		nodeKeys := map[string]model.GraphDigest{}
		for _, node := range batch.Nodes {
			key, err := model.NewNodeKey(node.Key)
			if err != nil {
				return nil, err
			}
			digest, err := key.Hash()
			if err != nil {
				return nil, err
			}
			nodeKeys[node.Ref] = digest
		}
		for _, e := range batch.Evidence {
			kind, subjectKey := "", model.GraphDigest{}
			nodeID, edgeID := -1, -1
			if e.NodeRef != "" {
				var ok bool
				subjectKey, ok = nodeKeys[e.NodeRef]
				if !ok {
					return nil, fmt.Errorf("graph overlay evidence node target is unresolved")
				}
				nodeID, ok = nodeRefs[graphOverlayRef(bi, e.NodeRef)]
				if !ok {
					return nil, fmt.Errorf("graph overlay evidence node target is unresolved")
				}
				kind = "node"
			} else {
				var ok bool
				edgeID, ok = edgeRefs[graphOverlayRef(bi, e.EdgeRef)]
				if !ok {
					return nil, fmt.Errorf("graph overlay evidence edge target is unresolved")
				}
				var edge *model.GraphObservationEdge
				for i := range batch.Edges {
					if batch.Edges[i].Ref == e.EdgeRef {
						edge = &batch.Edges[i]
						break
					}
				}
				if edge == nil {
					return nil, fmt.Errorf("graph overlay evidence edge target is unresolved")
				}
				from, fromOK := nodeKeys[edge.FromRef]
				to, toOK := nodeKeys[edge.ToRef]
				if !fromOK || !toOK {
					return nil, fmt.Errorf("graph overlay evidence edge endpoint is unresolved")
				}
				subjectKey = model.EdgeKey(from, to, edge.Relation, edge.Scope)
				kind = "edge"
			}
			digest := graphOverlayObservationEvidenceDigest(e)
			identity := graphOverlayEvidenceIdentity(kind, subjectKey, digest)
			entry := graphOverlayEvidence{module: batch.ProjectOrModule, kind: kind, subjectKey: subjectKey, evidenceDigest: digest, nodeID: nodeID, edgeID: edgeID, payload: e}
			if prior, exists := entries[identity]; exists && !reflect.DeepEqual(graphOverlayEvidencePayload(prior.payload), graphOverlayEvidencePayload(e)) {
				return nil, fmt.Errorf("graph overlay evidence identity collision")
			}
			entries[identity] = entry
		}
	}
	return entries, nil
}

func (s *LayeredGraphSnapshot) effectiveGraphOverlayEvidenceLayer(ctx context.Context) (map[string]graphOverlayEvidence, error) {
	entries, err := s.graphOverlayEvidenceLayer(ctx)
	if err != nil {
		return nil, err
	}
	for identity, entry := range entries {
		if !s.MasksSemanticOwnerInDomain(entry.payload.Backend, entry.module, entry.payload.SourceURI) {
			delete(entries, identity)
		}
	}
	return entries, nil
}

func graphOverlayEvidencePayload(e model.GraphObservationEvidence) model.GraphObservationEvidence {
	e.Ref, e.NodeRef, e.EdgeRef = "", "", ""
	return e
}

func graphOverlayUnresolvedPayload(u model.GraphObservationUnresolved) model.GraphObservationUnresolved {
	u.Ref = ""
	return u
}

func graphOverlayUnresolvedKey(u model.GraphObservationUnresolved) model.GraphDigest {
	var source *model.GraphDigest
	if u.SourceDigest != nil {
		d := *u.SourceDigest
		source = &d
	}
	return model.GraphUnresolvedKey(model.GraphUnresolved{OwnerPath: u.OwnerPath, SubjectKind: u.SubjectKind, SelectorDigest: u.SelectorDigest, ReasonCode: u.ReasonCode, Candidates: u.Candidates, Backend: u.Backend, SourceDigest: source, RecoveryHintCode: u.RecoveryHintCode})
}

func graphOverlayOmissionKey(o model.GraphObservationOmission) string {
	return strings.Join([]string{o.OwnerPath, o.SubjectKind, o.Backend, o.Capability, o.ReasonCode, o.RecoveryHintCode}, "\x00")
}

func (s *LayeredGraphSnapshot) UnresolvedCount(ctx context.Context) (int, error) {
	if s == nil || s.closed || ctx == nil {
		return 0, model.ErrGraphGenerationInvalid
	}
	mask, args := s.baseUnresolvedUnmaskedPredicate()
	var count int
	if err := s.base.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM graph_unresolved WHERE generation_id=?`+mask, append([]any{digestArg(s.base.generation.GenerationID)}, args...)...).Scan(&count); err != nil {
		return 0, err
	}
	seenUnresolved := map[model.GraphDigest]model.GraphObservationUnresolved{}
	seenOmissions := map[string]bool{}
	for _, b := range s.groups {
		for _, row := range b.Unresolved {
			u := row.Observation
			if !s.MasksSemanticOwnerInDomain(u.Backend, b.ProjectOrModule, u.OwnerPath) {
				continue
			}
			key := graphOverlayUnresolvedKey(u)
			if prior, ok := seenUnresolved[key]; ok && !reflect.DeepEqual(graphOverlayUnresolvedPayload(prior), graphOverlayUnresolvedPayload(u)) {
				return 0, fmt.Errorf("graph overlay unresolved identity collision")
			}
			seenUnresolved[key] = u
		}
		for _, o := range b.Omissions {
			if s.MasksSemanticOwnerInDomain(o.Backend, b.ProjectOrModule, o.OwnerPath) {
				seenOmissions[graphOverlayOmissionKey(o)] = true
			}
		}
	}
	return count + len(seenUnresolved) + len(seenOmissions), nil
}

func (s *LayeredGraphSnapshot) hiddenBaseNodeIDs(ctx context.Context) (map[int]bool, error) {
	ids := map[int]bool{}
	mask, maskArgs := s.baseNodeMaskPredicate()
	if mask == "" {
		return ids, nil
	}
	args := append([]any{digestArg(s.base.generation.GenerationID)}, maskArgs...)
	rows, err := s.base.query(ctx, `SELECT node_id FROM graph_nodes WHERE generation_id=? AND `+mask, args...)
	if err != nil {
		return nil, err
	}
	baseIDs := []int{}
	for rows.Next() {
		var id int
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		baseIDs = append(baseIDs, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	delta, _, err := s.nodeLayer(ctx)
	if err != nil {
		return nil, err
	}
	for _, id := range baseIDs {
		if _, replaced := delta[id]; !replaced {
			ids[id] = true
		}
	}
	return ids, nil
}

func graphOverlayEndpointFilter(ids map[int]bool) (string, []any) {
	if len(ids) == 0 {
		return "", nil
	}
	ordered := make([]int, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Ints(ordered)
	marks := make([]string, len(ordered))
	args := make([]any, 0, len(ordered)*2)
	for i, id := range ordered {
		marks[i] = "?"
		args = append(args, id)
	}
	return " AND from_node_id NOT IN (" + strings.Join(marks, ",") + ") AND to_node_id NOT IN (" + strings.Join(marks, ",") + ")", append(args, args...)
}

func graphOverlayNodeIDFilter(column string, ids map[int]model.GraphNodeRecord) (string, []any) {
	if len(ids) == 0 {
		return "", nil
	}
	ordered := make([]int, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Ints(ordered)
	marks := make([]string, len(ordered))
	args := make([]any, len(ordered))
	for i, id := range ordered {
		marks[i] = "?"
		args[i] = id
	}
	return " AND " + column + " NOT IN (" + strings.Join(marks, ",") + ")", args
}

func graphOverlaySingleIDFilter(column string, ids map[int]model.GraphEdgeRecord) (string, []any) {
	if len(ids) == 0 {
		return "", nil
	}
	ordered := make([]int, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Ints(ordered)
	marks := make([]string, len(ordered))
	args := make([]any, len(ordered))
	for i, id := range ordered {
		marks[i] = "?"
		args[i] = id
	}
	return " AND " + column + " NOT IN (" + strings.Join(marks, ",") + ")", args
}

func (s *LayeredGraphSnapshot) Stats(ctx context.Context) (map[string]int, error) {
	if s == nil || s.closed || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	nm, na := s.baseNodeUnmaskedPredicate()
	em, ea := s.baseEdgeUnmaskedPredicate("graph_edges")
	um, ua := s.baseUnresolvedUnmaskedPredicate()
	vm, va := s.baseEvidenceUnmaskedPredicate()
	hidden, err := s.hiddenBaseNodeIDs(ctx)
	if err != nil {
		return nil, err
	}
	deltaNodes, _, err := s.nodeLayer(ctx)
	if err != nil {
		return nil, err
	}
	endpoint, endpointArgs := graphOverlayEndpointFilter(hidden)
	deltaEdges, _, err := s.edgeLayer(ctx)
	if err != nil {
		return nil, err
	}
	shadowEdges, shadowEdgeArgs := graphOverlaySingleIDFilter("edge_id", deltaEdges)
	hiddenIDs := make([]int, 0, len(hidden))
	for id := range hidden {
		hiddenIDs = append(hiddenIDs, id)
	}
	sort.Ints(hiddenIDs)
	shadowNodes := make(map[int]model.GraphNodeRecord, len(deltaNodes))
	for id, node := range deltaNodes {
		shadowNodes[id] = node
	}
	if len(hiddenIDs) > 0 {
		for _, id := range hiddenIDs {
			shadowNodes[id] = model.GraphNodeRecord{}
		}
	}
	nodeShadow, nodeShadowArgs := graphOverlayNodeIDFilter("ge.node_id", shadowNodes)
	if nodeShadow != "" {
		vm += " AND (ge.node_id IS NULL" + strings.Replace(nodeShadow, " AND ", " OR ", 1) + ")"
		va = append(va, nodeShadowArgs...)
	}
	edgeOwnerMask, edgeOwnerArgs := s.baseEdgeUnmaskedPredicate("e")
	edgeEndpoint, edgeEndpointArgs := graphOverlayEndpointFilter(hidden)
	edgeShadow, edgeShadowArgs := graphOverlaySingleIDFilter("e.edge_id", deltaEdges)
	edgeSub := ` AND (ge.edge_id IS NULL OR ge.edge_id IN (SELECT e.edge_id FROM graph_edges e WHERE e.generation_id=?` + edgeOwnerMask + edgeEndpoint + edgeShadow + `))`
	evidenceArgs := append([]any{digestArg(s.base.generation.GenerationID)}, va...)
	evidenceArgs = append(evidenceArgs, digestArg(s.base.generation.GenerationID))
	evidenceArgs = append(evidenceArgs, edgeOwnerArgs...)
	evidenceArgs = append(evidenceArgs, edgeEndpointArgs...)
	evidenceArgs = append(evidenceArgs, edgeShadowArgs...)
	queries := []struct {
		sql  string
		args []any
		key  string
	}{
		{`SELECT COUNT(*) FROM graph_nodes WHERE generation_id=?` + nm, append([]any{digestArg(s.base.generation.GenerationID)}, na...), "nodes"},
		{`SELECT COUNT(*) FROM graph_edges WHERE generation_id=?` + em + endpoint + shadowEdges, append(append(append([]any{digestArg(s.base.generation.GenerationID)}, ea...), endpointArgs...), shadowEdgeArgs...), "edges"},
		{`SELECT COUNT(*) FROM (SELECT DISTINCT ge.subject_kind,gn.node_key,be.edge_key,ge.source_digest,ge.observed_claim_digest,ge.source_uri,ge.claim_kind,ge.backend,ge.extractor_version,COALESCE(ge.start_line,0),COALESCE(ge.start_column,0),COALESCE(ge.end_line,0),COALESCE(ge.end_column,0) FROM graph_evidence ge LEFT JOIN graph_nodes gn ON gn.generation_id=ge.generation_id AND gn.node_id=ge.node_id LEFT JOIN graph_edges be ON be.generation_id=ge.generation_id AND be.edge_id=ge.edge_id WHERE ge.generation_id=?` + vm + edgeSub + `)`, evidenceArgs, "evidence"},
		{`SELECT COUNT(*) FROM graph_unresolved WHERE generation_id=?` + um, append([]any{digestArg(s.base.generation.GenerationID)}, ua...), "unresolved"},
	}
	result := map[string]int{}
	for _, q := range queries {
		var count int
		if err := s.base.tx.QueryRowContext(ctx, q.sql, q.args...).Scan(&count); err != nil {
			return nil, err
		}
		result[q.key] = count
	}
	result["nodes"] += len(deltaNodes)
	result["edges"] += len(deltaEdges)
	overlayEvidence, err := s.effectiveGraphOverlayEvidenceLayer(ctx)
	if err != nil {
		return nil, err
	}
	seenUnresolved := map[model.GraphDigest]model.GraphObservationUnresolved{}
	seenOmissions := map[string]bool{}
	for _, b := range s.groups {
		for _, row := range b.Unresolved {
			u := row.Observation
			if !s.MasksSemanticOwnerInDomain(u.Backend, b.ProjectOrModule, u.OwnerPath) {
				continue
			}
			key := graphOverlayUnresolvedKey(u)
			if prior, exists := seenUnresolved[key]; exists && !reflect.DeepEqual(graphOverlayUnresolvedPayload(prior), graphOverlayUnresolvedPayload(u)) {
				return nil, fmt.Errorf("graph overlay unresolved identity collision")
			}
			seenUnresolved[key] = u
		}
		for _, o := range b.Omissions {
			if s.MasksSemanticOwnerInDomain(o.Backend, b.ProjectOrModule, o.OwnerPath) {
				seenOmissions[graphOverlayOmissionKey(o)] = true
			}
		}
	}
	result["evidence"] += len(overlayEvidence)
	result["unresolved"] += len(seenUnresolved) + len(seenOmissions)
	return result, nil
}

func (s *LayeredGraphSnapshot) FacetStats(ctx context.Context) (map[string]map[string]int, error) {
	if s == nil || s.closed || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	facets := map[string]map[string]int{"kind": {}, "relation": {}, "status": {}, "backend": {}}
	hidden, err := s.hiddenBaseNodeIDs(ctx)
	if err != nil {
		return nil, err
	}
	deltaEdges, _, err := s.edgeLayer(ctx)
	if err != nil {
		return nil, err
	}
	for facet, spec := range map[string]struct{ col, table string }{"kind": {"symbol_kind", "graph_nodes"}, "status": {"claim_status", "graph_nodes"}, "backend": {"backend_type", "graph_nodes"}, "relation": {"relation", "graph_edges"}} {
		mask, args := s.baseNodeUnmaskedPredicate()
		if spec.table == "graph_edges" {
			mask, args = s.baseEdgeUnmaskedPredicate("graph_edges")
		}
		endpoint, endpointArgs, shadow, shadowArgs := "", []any(nil), "", []any(nil)
		if facet == "relation" {
			endpoint, endpointArgs = graphOverlayEndpointFilter(hidden)
			shadow, shadowArgs = graphOverlaySingleIDFilter("edge_id", deltaEdges)
		}
		queryArgs := append([]any{digestArg(s.base.generation.GenerationID)}, args...)
		queryArgs = append(queryArgs, endpointArgs...)
		queryArgs = append(queryArgs, shadowArgs...)
		rows, err := s.base.query(ctx, `SELECT `+spec.col+`,COUNT(*) FROM `+spec.table+` WHERE generation_id=?`+mask+endpoint+shadow+` GROUP BY `+spec.col+` ORDER BY `+spec.col, queryArgs...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var name string
			var n int
			if err = rows.Scan(&name, &n); err != nil {
				rows.Close()
				return nil, err
			}
			facets[facet][name] = n
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	deltaNodes, _, err := s.nodeLayer(ctx)
	if err != nil {
		return nil, err
	}
	for _, n := range deltaNodes {
		facets["kind"][n.Identity.SymbolKind]++
		facets["status"][n.ClaimStatus]++
		facets["backend"][n.Identity.BackendType]++
	}
	for _, e := range deltaEdges {
		facets["relation"][e.Relation]++
	}
	return facets, nil
}

func (s *LayeredGraphSnapshot) UnresolvedOmissions(ctx context.Context, max int) ([]model.EnvelopeOmission, error) {
	if s == nil || s.closed || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	if max < 1 {
		return []model.EnvelopeOmission{}, nil
	}
	if max > 50 {
		max = 50
	}
	mask, args := s.baseUnresolvedUnmaskedPredicate()
	// Do not limit to max before the final sanitized deterministic ordering.
	// An independent candidate ceiling bounds both modern and legacy cursors;
	// overflow is an explicit failure, never a silently incomplete result.
	const maximumBaseCandidates = 4096
	queryArgs := append([]any{digestArg(s.base.generation.GenerationID)}, args...)
	queryArgs = append(queryArgs, maximumBaseCandidates+1)
	rows, err := s.base.query(ctx, `SELECT cross_rid,owner_path,subject_kind,reason_code,recovery_hint_code,source_document,source_block,target_kind,target_value FROM graph_unresolved WHERE generation_id=?`+mask+` ORDER BY unresolved_id LIMIT ?`, queryArgs...)
	legacy := false
	if err != nil && graphUnresolvedOmissionCapabilityError(err) {
		legacy = true
		rows, err = s.base.query(ctx, `SELECT cross_rid,owner_path,subject_kind,reason_code,recovery_hint_code FROM graph_unresolved WHERE generation_id=?`+mask+` ORDER BY unresolved_id LIMIT ?`, queryArgs...)
	}
	if err != nil {
		return nil, err
	}
	out := []model.EnvelopeOmission{}
	baseCandidates := 0
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		if baseCandidates == maximumBaseCandidates {
			rows.Close()
			return nil, &model.GraphQueryError{Code: "GPH_QUERY_BUDGET_EXCEEDED", Field: "base_unresolved_candidates", Message: "explicit_incomplete: base unresolved candidates exceed the 4096-candidate query budget"}
		}
		baseCandidates++
		o, e := scanGraphUnresolvedOmission(rows, legacy)
		if e != nil {
			rows.Close()
			return nil, e
		}
		out = append(out, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	seenUnresolved := map[model.GraphDigest]graphOverlayUnresolvedRow{}
	seenUnresolvedRefs := map[model.GraphDigest]map[string]bool{}
	seenOmissions := map[string]map[string]bool{}
	for _, b := range s.groups {
		for _, row := range b.Unresolved {
			u := row.Observation
			key := graphOverlayUnresolvedKey(u)
			if !s.MasksSemanticOwnerInDomain(u.Backend, b.ProjectOrModule, u.OwnerPath) {
				continue
			}
			if prior, ok := seenUnresolved[key]; ok {
				if !reflect.DeepEqual(graphOverlayUnresolvedPayload(prior.Observation), graphOverlayUnresolvedPayload(u)) || prior.Record.SourceDocument != row.Record.SourceDocument || prior.Record.SourceBlock != row.Record.SourceBlock || prior.Record.TargetKind != row.Record.TargetKind || prior.Record.TargetValue != row.Record.TargetValue {
					rows.Close()
					return nil, fmt.Errorf("graph overlay unresolved identity collision")
				}
			} else {
				seenUnresolved[key] = row
				seenUnresolvedRefs[key] = map[string]bool{}
			}
			if !seenUnresolvedRefs[key][u.Ref] {
				seenUnresolvedRefs[key][u.Ref] = true
				out = append(out, model.EnvelopeOmission{Input: u.Ref, Reason: sanitizeGraphOmissionText(row.Record.ReasonCode), ErrorCode: sanitizeGraphOmissionText(row.Record.RecoveryHintCode), OwnerPath: sanitizeGraphOmissionPath(row.Record.OwnerPath), SourceDocument: sanitizeGraphOmissionPath(row.Record.SourceDocument), SourceBlock: sanitizeGraphOmissionText(row.Record.SourceBlock), TargetKind: sanitizeGraphOmissionText(row.Record.TargetKind), TargetValue: sanitizeGraphOmissionText(row.Record.TargetValue)})
			}
		}
		for _, o := range b.Omissions {
			key := graphOverlayOmissionKey(o)
			if !s.MasksSemanticOwnerInDomain(o.Backend, b.ProjectOrModule, o.OwnerPath) {
				continue
			}
			if seenOmissions[key] == nil {
				seenOmissions[key] = map[string]bool{}
			}
			if !seenOmissions[key][o.Ref] {
				seenOmissions[key][o.Ref] = true
				out = append(out, model.EnvelopeOmission{Input: o.Ref, Reason: o.ReasonCode, ErrorCode: o.RecoveryHintCode, OwnerPath: o.OwnerPath})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OwnerPath != out[j].OwnerPath {
			return out[i].OwnerPath < out[j].OwnerPath
		}
		if out[i].Input != out[j].Input {
			return out[i].Input < out[j].Input
		}
		return out[i].Reason < out[j].Reason
	})
	if len(out) > max {
		out = out[:max]
	}
	return out, nil
}

// Validate checks effective overlay references and returns computed counts; the overlay is not a published canonical generation.
func (s *LayeredGraphSnapshot) Validate(ctx context.Context) (model.GraphGeneration, error) {
	if s == nil || s.closed || ctx == nil {
		return model.GraphGeneration{}, model.ErrGraphGenerationInvalid
	}
	if _, _, err := s.nodeLayer(ctx); err != nil {
		return model.GraphGeneration{}, err
	}
	if _, _, err := s.edgeLayer(ctx); err != nil {
		return model.GraphGeneration{}, err
	}
	g := s.generation
	stats, err := s.Stats(ctx)
	if err != nil {
		return model.GraphGeneration{}, err
	}
	g.NodeCount = stats["nodes"]
	g.EdgeCount = stats["edges"]
	g.EvidenceCount = stats["evidence"]
	g.UnresolvedCount = stats["unresolved"]
	g.ContentDigest = model.GraphDigest{}
	g.BackendManifestDigest = model.GraphDigest{}
	g.Status = "overlay"
	return g, nil
}
