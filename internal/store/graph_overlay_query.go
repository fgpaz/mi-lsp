package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

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
	for batchIndex, batch := range s.batches {
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
		if !s.MasksPath(observation.key.OwnerPath) {
			if lookupErr != nil || s.MasksPath(base.Identity.OwnerPath) || base.SourceDigest != observation.node.SourceDigest || base.ClaimStatus != observation.node.ClaimStatus {
				return nil, nil, fmt.Errorf("graph overlay external node is not an unchanged pinned base anchor")
			}
			ids[hash] = base.NodeID
			continue
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
	if s.MasksPath(node.Identity.OwnerPath) {
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
		if !s.MasksPath(node.Identity.OwnerPath) {
			node.GenerationID = s.generation.GenerationID
			result[id] = node
		}
	}
	return result, nil
}
