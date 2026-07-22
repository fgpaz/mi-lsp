package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

type GraphRankRequest struct {
	GenerationID string
	MaxNodes     int
	MaxEdges     int
	Limit        int
	Utility      []model.UtilitySignal
}

func GraphRank(ctx context.Context, db *sql.DB, request GraphRankRequest) (model.GraphRankEnvelope, error) {
	if ctx == nil || db == nil {
		return model.GraphRankEnvelope{}, &model.GraphQueryError{Code: "GPH_QUERY_BACKEND_UNAVAILABLE", Message: "graph backend is unavailable"}
	}
	freshness, err := store.GraphFreshness(ctx, db, request.GenerationID)
	if err != nil {
		return model.GraphRankEnvelope{GraphFreshness: freshness}, store.SanitizeGraphQueryError(err)
	}
	if !freshness.AllowsExactClaims() {
		return model.GraphRankEnvelope{Ok: false, GenerationID: freshness.GenerationID, GraphFreshness: freshness, Algorithm: model.GraphAnalysisAlgorithm, AlgorithmVersion: model.GraphAnalysisVersion, Profile: model.GraphAnalysisProfile, Items: []model.GraphRank{}, Warnings: []string{"graph rank withheld because graph freshness is " + freshness.State}}, nil
	}
	snapshot, err := store.BeginGraphQuerySnapshot(ctx, db, request.GenerationID)
	if err != nil {
		return model.GraphRankEnvelope{GraphFreshness: freshness}, store.SanitizeGraphQueryError(err)
	}
	defer snapshot.Close()
	generation, err := snapshot.Generation()
	if err != nil {
		return model.GraphRankEnvelope{}, err
	}
	ranks, digest, truncated, err := graphRankSnapshot(ctx, snapshot, request)
	if err != nil {
		return model.GraphRankEnvelope{}, err
	}
	return model.GraphRankEnvelope{Ok: true, GenerationID: generation.GenerationID.String(), GraphFreshness: freshness, Algorithm: model.GraphAnalysisAlgorithm, AlgorithmVersion: model.GraphAnalysisVersion, Profile: model.GraphAnalysisProfile, DeterminismDigest: digest, Items: ranks, Truncated: truncated}, nil
}

func graphRankSnapshot(ctx context.Context, snapshot *store.GraphQuerySnapshot, request GraphRankRequest) ([]model.GraphRank, string, bool, error) {
	data, err := snapshot.BoundedData(ctx, request.MaxNodes, request.MaxEdges)
	if err != nil {
		return nil, "", false, err
	}
	if request.Limit <= 0 || request.Limit > model.GraphQueryMaxLimit {
		request.Limit = model.GraphQueryMaxLimit
	}
	nodes := make(map[int]model.GraphNodeRecord, len(data.Nodes))
	for _, n := range data.Nodes {
		nodes[n.NodeID] = n
	}
	adj := make(map[int]map[int]struct{}, len(nodes))
	inbound := make(map[int]int, len(nodes))
	outbound := make(map[int]int, len(nodes))
	boundary := make(map[int]int, len(nodes))
	for id := range nodes {
		adj[id] = map[int]struct{}{}
	}
	for _, e := range data.Edges {
		from, fok := nodes[e.FromNodeID]
		to, tok := nodes[e.ToNodeID]
		if !fok || !tok {
			continue
		}
		// BoundedData already filters these statuses; retain the explicit gate so
		// future callers cannot accidentally grant inferred edges authority.
		if e.ClaimStatus != model.GraphRecordExact && e.ClaimStatus != model.GraphRecordExtracted {
			continue
		}
		adj[e.FromNodeID][e.ToNodeID] = struct{}{}
		adj[e.ToNodeID][e.FromNodeID] = struct{}{}
		inbound[e.ToNodeID]++
		outbound[e.FromNodeID]++
		if from.Identity.ProjectOrModule != to.Identity.ProjectOrModule || from.Identity.RepositoryIdentity != to.Identity.RepositoryIdentity {
			boundary[e.FromNodeID]++
			boundary[e.ToNodeID]++
		}
	}
	components := deterministicCommunities(nodes, adj)
	maxImpact, maxCentrality, maxBoundary := 1, 1, 1
	for id := range nodes {
		if inbound[id]+outbound[id] > maxImpact {
			maxImpact = inbound[id] + outbound[id]
		}
		if len(adj[id]) > maxCentrality {
			maxCentrality = len(adj[id])
		}
		if boundary[id] > maxBoundary {
			maxBoundary = boundary[id]
		}
	}
	utilityByNode := map[string]float64{}
	for _, signal := range request.Utility {
		utilityByNode[signal.Operation+"\x00"+signal.Intent] = signal.Score
	}
	ranks := make([]model.GraphRank, 0, len(nodes))
	for id, node := range nodes {
		authority := 0.0
		if node.ClaimStatus == model.GraphRecordExact {
			authority = 1
		} else if node.ClaimStatus == model.GraphRecordExtracted {
			authority = .7
		}
		impact := float64(inbound[id]) / float64(maxImpact)
		centrality := float64(len(adj[id])) / float64(maxCentrality)
		boundaryScore := float64(boundary[id]) / float64(maxBoundary)
		score := .45*authority + .25*impact + .20*centrality + .10*boundaryScore
		heuristicExcluded := node.ClaimStatus == model.GraphRecordInferred
		reason := "bounded exact/extracted graph rank"
		if heuristicExcluded {
			reason = "inferred node excluded from authority graph"
		}
		ranks = append(ranks, model.GraphRank{NodeKey: node.NodeKey.String(), CrossRID: node.CrossRID, Display: node.DisplayName, OwnerPath: node.Identity.OwnerPath, CommunityID: components[id], RankReason: reason, AlgorithmVersion: model.GraphAnalysisVersion, Score: score, Authority: authority, Impact: impact, Centrality: centrality, Boundary: boundaryScore, HeuristicExcluded: heuristicExcluded})
	}
	sort.SliceStable(ranks, func(i, j int) bool {
		a, b := ranks[i], ranks[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Authority != b.Authority {
			return a.Authority > b.Authority
		}
		if a.Impact != b.Impact {
			return a.Impact > b.Impact
		}
		if a.Centrality != b.Centrality {
			return a.Centrality > b.Centrality
		}
		if a.Boundary != b.Boundary {
			return a.Boundary > b.Boundary
		}
		// Utility is deliberately the last tie-break and cannot change a score.
		if utilityByNode[a.NodeKey] != utilityByNode[b.NodeKey] {
			return utilityByNode[a.NodeKey] > utilityByNode[b.NodeKey]
		}
		return a.NodeKey < b.NodeKey
	})
	truncated := data.Truncated || len(ranks) > request.Limit
	if len(ranks) > request.Limit {
		ranks = ranks[:request.Limit]
	}
	digest := model.GraphRankDeterminismDigest(generationString(snapshot), model.GraphAnalysisAlgorithm, model.GraphAnalysisVersion, model.GraphAnalysisProfile, ranks)
	return ranks, digest, truncated, nil
}

func generationString(snapshot *store.GraphQuerySnapshot) string {
	g, _ := snapshot.Generation()
	return g.GenerationID.String()
}

func deterministicCommunities(nodes map[int]model.GraphNodeRecord, adj map[int]map[int]struct{}) map[int]string {
	visited := map[int]bool{}
	result := map[int]string{}
	ids := make([]int, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return nodes[ids[i]].NodeKey.String() < nodes[ids[j]].NodeKey.String() })
	for _, start := range ids {
		if visited[start] {
			continue
		}
		queue := []int{start}
		visited[start] = true
		keys := []string{}
		members := []int{}
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			members = append(members, id)
			keys = append(keys, nodes[id].NodeKey.String())
			neighbors := make([]int, 0, len(adj[id]))
			for n := range adj[id] {
				neighbors = append(neighbors, n)
			}
			sort.Slice(neighbors, func(i, j int) bool {
				return nodes[neighbors[i]].NodeKey.String() < nodes[neighbors[j]].NodeKey.String()
			})
			for _, n := range neighbors {
				if !visited[n] {
					visited[n] = true
					queue = append(queue, n)
				}
			}
		}
		community := model.CommunityID(keys)
		for _, id := range members {
			result[id] = community
		}
	}
	return result
}

func graphRankCacheJSON(ranks []model.GraphRank) string {
	b, _ := json.Marshal(ranks)
	return string(b)
}

func cacheGraphRanks(ctx context.Context, db *sql.DB, generation model.GraphGeneration, ranks []model.GraphRank, digest string) error {
	result := graphRankCacheJSON(ranks)
	if len(result) > model.GraphAnalysisMaxBytes {
		return fmt.Errorf("graph rank result exceeds bounded cache")
	}
	return store.PutGraphAnalysis(ctx, db, model.GraphAnalysis{GenerationID: generation.GenerationID, ExtensionID: "mi-lsp", ExtensionVersion: model.GraphAnalysisVersion, Operation: "rank", OutputSchema: "graph-rank/v1", ResultJSON: result, ProvenanceJSON: `{"source":"graph_nodes+graph_edges"}`, OmissionsJSON: `[]`, Status: "complete", Algorithm: model.GraphAnalysisAlgorithm, AlgorithmVersion: model.GraphAnalysisVersion, Profile: model.GraphAnalysisProfile, DeterminismDigest: digest})
}

func rankIntentFromOperation(operation string) string {
	operation = strings.TrimPrefix(strings.TrimSpace(operation), "nav.")
	return model.SanitizeUtilityIntent(operation)
}
