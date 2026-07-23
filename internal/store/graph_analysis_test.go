package store

import (
	"context"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestGetGraphAnalysisIgnoresDigestMismatch(t *testing.T) {
	db, _ := seedTestDB(t)
	defer db.Close()
	bundle := testGraphBundle(t)
	if err := StageGraphGeneration(context.Background(), db, &bundle); err != nil {
		t.Fatal(err)
	}
	generation := bundle.Generation.GenerationID
	params, _ := model.ParseGraphDigest(strings.Repeat("b", 64))
	authority, _ := model.ParseGraphDigest(strings.Repeat("c", 64))
	request := model.GraphAnalysisRequest{GenerationID: generation.String(), Algorithm: model.GraphAnalysisAlgorithm, AlgorithmVersion: model.GraphAnalysisVersion, Profile: model.GraphAnalysisProfile, ParametersDigest: params.String(), AuthorityProfile: authority.String(), MaxNodes: 10, MaxEdges: 20, Limit: 5}
	key := GraphAnalysisKey(request)
	analysis := model.GraphAnalysis{GenerationID: generation, AnalysisKey: key, Operation: "rank", ParametersDigest: params, AuthorityProfileDigest: authority, OutputSchema: "graph-rank/v1", ResultJSON: `[{"node_key":"` + strings.Repeat("b", 64) + `"}]`, ProvenanceJSON: `{}`, OmissionsJSON: `[]`, Status: "complete", Algorithm: model.GraphAnalysisAlgorithm, AlgorithmVersion: model.GraphAnalysisVersion, Profile: model.GraphAnalysisProfile}
	if err := PutGraphAnalysis(context.Background(), db, analysis); err != nil {
		t.Fatal(err)
	}
	if _, found, err := GetGraphAnalysis(context.Background(), db, request); err != nil || !found {
		t.Fatalf("valid cache found=%v err=%v", found, err)
	}
	if _, err := db.Exec("UPDATE graph_analysis SET result_digest = zeroblob(32) WHERE analysis_key = ?", digestArg(key)); err != nil {
		t.Fatal(err)
	}
	if _, found, err := GetGraphAnalysis(context.Background(), db, request); err != nil || found {
		t.Fatalf("mismatched cache found=%v err=%v", found, err)
	}
}
