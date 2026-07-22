package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const (
	GraphAnalysisCacheFresh = "fresh"
	GraphAnalysisCacheMiss  = "miss"
)

func GraphFreshness(ctx context.Context, db *sql.DB, requestedGeneration string) (model.GraphFreshness, error) {
	if ctx == nil || db == nil {
		return model.GraphFreshness{State: model.GraphFreshnessInvalid, ReasonCode: "backend_unavailable"}, model.ErrGraphGenerationInvalid
	}
	runtimeState, err := GraphRuntimeState(ctx, db)
	if err != nil {
		return model.GraphFreshness{State: model.GraphFreshnessUnknown, ReasonCode: "runtime_state_unavailable"}, err
	}
	active, ok, err := ActiveGraphGeneration(ctx, db)
	if err != nil {
		return model.GraphFreshness{State: model.GraphFreshnessInvalid, ReasonCode: "active_pointer_invalid"}, err
	}
	if !ok {
		return model.ClassifyGraphFreshness(runtimeState, "", requestedGeneration, true), nil
	}
	generation, err := ValidateGraphGeneration(ctx, db, active)
	if err != nil || generation.Status != model.GraphGenerationActive {
		freshness := model.ClassifyGraphFreshness(runtimeState, active.String(), requestedGeneration, false)
		if err == nil {
			freshness.ReasonCode = "active_generation_not_active"
		}
		return freshness, nil
	}
	return model.ClassifyGraphFreshness(runtimeState, active.String(), requestedGeneration, true), nil
}

func (s *GraphQuerySnapshot) Freshness(ctx context.Context, requested string) (model.GraphFreshness, error) {
	if s == nil || s.closed {
		return model.GraphFreshness{State: model.GraphFreshnessInvalid, ReasonCode: "snapshot_closed"}, model.ErrGraphGenerationInvalid
	}
	return model.ClassifyGraphFreshness(GraphRuntimeFresh, s.generation.GenerationID.String(), requested, true), nil
}

func GraphAnalysisKey(request model.GraphAnalysisRequest) model.GraphDigest {
	request = request.Normalize()
	b, _ := json.Marshal(request)
	return model.GraphDigest(sha256.Sum256(append([]byte("milsp-graph-analysis/v1\x00"), b...)))
}

func GraphAnalysisDigest(resultJSON string) model.GraphDigest {
	return model.GraphDigest(sha256.Sum256([]byte(resultJSON)))
}

func PutGraphAnalysis(ctx context.Context, db *sql.DB, analysis model.GraphAnalysis) error {
	if ctx == nil || db == nil || analysis.GenerationID == (model.GraphDigest{}) {
		return model.ErrGraphGenerationInvalid
	}
	if len(analysis.ResultJSON) > model.GraphAnalysisMaxBytes || len(analysis.ProvenanceJSON) > model.GraphAnalysisMaxBytes || len(analysis.OmissionsJSON) > model.GraphAnalysisMaxBytes {
		return errors.New("graph analysis bounded output exceeds limit")
	}
	if analysis.Algorithm == "" {
		analysis.Algorithm = model.GraphAnalysisAlgorithm
	}
	if analysis.AlgorithmVersion == "" {
		analysis.AlgorithmVersion = model.GraphAnalysisVersion
	}
	if analysis.Profile == "" {
		analysis.Profile = model.GraphAnalysisProfile
	}
	if analysis.ResultDigest == (model.GraphDigest{}) {
		analysis.ResultDigest = GraphAnalysisDigest(analysis.ResultJSON)
	}
	if analysis.AnalysisKey == (model.GraphDigest{}) {
		analysis.AnalysisKey = GraphAnalysisKey(model.GraphAnalysisRequest{GenerationID: analysis.GenerationID.String(), Algorithm: analysis.Algorithm, AlgorithmVersion: analysis.AlgorithmVersion, Profile: analysis.Profile, ParametersDigest: analysis.ParametersDigest.String(), AuthorityProfile: analysis.AuthorityProfileDigest.String()})
	}
	if analysis.CreatedAt.IsZero() {
		analysis.CreatedAt = time.Now().UTC()
	}
	_, err := db.ExecContext(ctx, `INSERT INTO graph_analysis(analysis_key,generation_id,extension_id,extension_version,executable_digest,operation,parameters_digest,authority_profile_digest,output_schema,result_json_bounded,result_digest,provenance_json_sanitized,omissions_json_sanitized,status,created_at,algorithm,algorithm_version,profile,determinism_digest) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(analysis_key) DO UPDATE SET result_json_bounded=excluded.result_json_bounded,result_digest=excluded.result_digest,provenance_json_sanitized=excluded.provenance_json_sanitized,omissions_json_sanitized=excluded.omissions_json_sanitized,status=excluded.status,created_at=excluded.created_at,algorithm=excluded.algorithm,algorithm_version=excluded.algorithm_version,profile=excluded.profile,determinism_digest=excluded.determinism_digest`,
		digestArg(analysis.AnalysisKey), digestArg(analysis.GenerationID), analysis.ExtensionID, analysis.ExtensionVersion, digestArg(analysis.ExecutableDigest), analysis.Operation, digestArg(analysis.ParametersDigest), digestArg(analysis.AuthorityProfileDigest), analysis.OutputSchema, analysis.ResultJSON, digestArg(analysis.ResultDigest), analysis.ProvenanceJSON, analysis.OmissionsJSON, analysis.Status, analysis.CreatedAt.UTC().Format(time.RFC3339Nano), analysis.Algorithm, analysis.AlgorithmVersion, analysis.Profile, analysis.DeterminismDigest)
	return err
}

func GetGraphAnalysis(ctx context.Context, db *sql.DB, request model.GraphAnalysisRequest) (model.GraphAnalysis, bool, error) {
	if ctx == nil || db == nil {
		return model.GraphAnalysis{}, false, model.ErrGraphGenerationInvalid
	}
	request = request.Normalize()
	key := GraphAnalysisKey(request)
	var a model.GraphAnalysis
	var generation, analysisKey, executable, params, authority, resultDigest []byte
	var created string
	err := db.QueryRowContext(ctx, `SELECT generation_id,analysis_key,extension_id,extension_version,executable_digest,operation,parameters_digest,authority_profile_digest,output_schema,result_json_bounded,result_digest,provenance_json_sanitized,omissions_json_sanitized,status,created_at,COALESCE(algorithm,''),COALESCE(algorithm_version,''),COALESCE(profile,''),COALESCE(determinism_digest,'') FROM graph_analysis WHERE analysis_key=?`, digestArg(key)).Scan(&generation, &analysisKey, &a.ExtensionID, &a.ExtensionVersion, &executable, &a.Operation, &params, &authority, &a.OutputSchema, &a.ResultJSON, &resultDigest, &a.ProvenanceJSON, &a.OmissionsJSON, &a.Status, &created, &a.Algorithm, &a.AlgorithmVersion, &a.Profile, &a.DeterminismDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return model.GraphAnalysis{}, false, nil
	}
	if err != nil {
		return model.GraphAnalysis{}, false, err
	}
	var scanErr error
	if a.GenerationID, scanErr = scanDigest(generation); scanErr != nil {
		return model.GraphAnalysis{}, false, scanErr
	}
	if a.AnalysisKey, scanErr = scanDigest(analysisKey); scanErr != nil {
		return model.GraphAnalysis{}, false, scanErr
	}
	if a.ExecutableDigest, scanErr = scanDigest(executable); scanErr != nil {
		return model.GraphAnalysis{}, false, scanErr
	}
	if a.ParametersDigest, scanErr = scanDigest(params); scanErr != nil {
		return model.GraphAnalysis{}, false, scanErr
	}
	if a.AuthorityProfileDigest, scanErr = scanDigest(authority); scanErr != nil {
		return model.GraphAnalysis{}, false, scanErr
	}
	if a.ResultDigest, scanErr = scanDigest(resultDigest); scanErr != nil {
		return model.GraphAnalysis{}, false, scanErr
	}
	a.CreatedAt, scanErr = time.Parse(time.RFC3339Nano, created)
	if scanErr != nil {
		return model.GraphAnalysis{}, false, scanErr
	}
	if a.GenerationID.String() != request.GenerationID || a.Algorithm != request.Algorithm || a.AlgorithmVersion != request.AlgorithmVersion || a.Profile != request.Profile || len(a.ResultJSON) > model.GraphAnalysisMaxBytes {
		return model.GraphAnalysis{}, false, nil
	}
	if GraphAnalysisDigest(a.ResultJSON) != a.ResultDigest {
		return model.GraphAnalysis{}, false, nil
	}
	return a, true, nil
}

// GraphBoundedData is intentionally bounded and never represents a transitive
// closure or a durable full-graph cache.
type GraphBoundedData struct {
	Generation model.GraphGeneration
	Nodes      []model.GraphNodeRecord
	Edges      []model.GraphEdgeRecord
	Truncated  bool
}

func (s *GraphQuerySnapshot) BoundedData(ctx context.Context, maxNodes, maxEdges int) (GraphBoundedData, error) {
	if s == nil || s.closed || ctx == nil {
		return GraphBoundedData{}, model.ErrGraphGenerationInvalid
	}
	if maxNodes <= 0 || maxNodes > model.GraphAnalysisMaxNodes {
		maxNodes = model.GraphAnalysisMaxNodes
	}
	if maxEdges <= 0 || maxEdges > model.GraphAnalysisMaxEdges {
		maxEdges = model.GraphAnalysisMaxEdges
	}
	data := GraphBoundedData{Generation: s.generation}
	rows, err := s.query(ctx, graphNodeSelect+` ORDER BY node_key LIMIT ?`, digestArg(s.generation.GenerationID), maxNodes+1)
	if err != nil {
		return data, err
	}
	for rows.Next() {
		n, e := scanGraphNode(rows, s.generation.GenerationID)
		if e != nil {
			rows.Close()
			return data, e
		}
		if len(data.Nodes) == maxNodes {
			data.Truncated = true
			break
		}
		data.Nodes = append(data.Nodes, n)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return data, err
	}
	rows.Close()
	rows, err = s.query(ctx, `SELECT edge_id,edge_key,from_node_id,to_node_id,relation,claim_scope,claim_status,owner_path,source_backend,cross_rid FROM graph_edges WHERE generation_id=? AND claim_status IN (?,?) ORDER BY edge_key LIMIT ?`, digestArg(s.generation.GenerationID), model.GraphRecordExact, model.GraphRecordExtracted, maxEdges+1)
	if err != nil {
		return data, err
	}
	for rows.Next() {
		e, scanErr := graphEdgeFromRow(rows, s.generation.GenerationID)
		if scanErr != nil {
			rows.Close()
			return data, scanErr
		}
		if len(data.Edges) == maxEdges {
			data.Truncated = true
			break
		}
		data.Edges = append(data.Edges, e)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return data, err
	}
	rows.Close()
	return data, nil
}

func ParseAnalysisDigest(raw string) (model.GraphDigest, error) {
	return model.ParseGraphDigest(strings.ToLower(strings.TrimSpace(raw)))
}
func AnalysisDigestHex(d model.GraphDigest) string { return hex.EncodeToString(d[:]) }
