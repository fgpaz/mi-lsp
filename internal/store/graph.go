package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"github.com/fgpaz/mi-lsp/internal/model"
	"strings"
	"time"
)

const graphActivePointer = "active_graph_generation_id"

type graphQ interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func staged(ctx context.Context, q graphQ, id string) error {
	var s string
	if e := q.QueryRowContext(ctx, "select status from graph_generations where generation_id=?", id).Scan(&s); e != nil {
		return e
	}
	if s != model.GraphGenerationStaged {
		return fmt.Errorf("%w: state %s", model.ErrGraphGenerationInvalid, s)
	}
	return nil
}
func CreateGraphGeneration(c context.Context, d *sql.DB, g model.GraphGeneration) error {
	if g.GenerationID == "" || g.WorkspaceRoot == "" || g.SchemaVersion < 1 || (g.Status != "" && g.Status != model.GraphGenerationStaged) {
		return fmt.Errorf("%w: generation", model.ErrGraphGenerationInvalid)
	}
	if g.CreatedAt.IsZero() {
		g.CreatedAt = time.Now().UTC()
	}
	_, e := d.ExecContext(c, "insert into graph_generations(generation_id,workspace_root,schema_version,created_at,status,owner_id,owner_pid,expected_nodes,expected_edges,expected_evidence,expected_unresolved)values(?,?,?,?,?,?,?,?,?,?,?)", g.GenerationID, g.WorkspaceRoot, g.SchemaVersion, g.CreatedAt.Format(time.RFC3339Nano), model.GraphGenerationStaged, g.OwnerID, g.OwnerPID, g.ExpectedNodes, g.ExpectedEdges, g.ExpectedEvidence, g.ExpectedUnresolved)
	return e
}
func InsertGraphEvidence(c context.Context, d *sql.DB, x model.GraphEvidence) error {
	if e := staged(c, d, x.GenerationID); e != nil {
		return e
	}
	if x.EvidenceID == "" || x.SourceURI == "" || x.Backend == "" || x.ExtractorVersion == "" || x.Digest == "" || x.ObservedClaim == "" || !strings.HasPrefix(x.CrossRID, "evidence:") {
		return fmt.Errorf("%w: evidence", model.ErrGraphGenerationInvalid)
	}
	_, e := d.ExecContext(c, "insert into graph_evidence(generation_id,evidence_id,source_uri,source_range,backend,extractor_version,digest,observed_claim,cross_rid,status)values(?,?,?,?,?,?,?,?,?,?)", x.GenerationID, x.EvidenceID, x.SourceURI, x.SourceRange, x.Backend, x.ExtractorVersion, x.Digest, x.ObservedClaim, x.CrossRID, x.Status)
	return e
}
func InsertGraphNode(c context.Context, d *sql.DB, x model.GraphNodeRecord) error {
	if e := staged(c, d, x.GenerationID); e != nil {
		return e
	}
	k, e := x.NodeKey.Serialize()
	if e != nil {
		return e
	}
	if x.Kind == "" || x.DisplayName == "" || x.DeclarationPath == "" || x.Backend == "" || x.Status == "" || x.Provenance == "" || !strings.HasPrefix(x.CrossRID, "node:") {
		return fmt.Errorf("%w: node", model.ErrGraphGenerationInvalid)
	}
	_, e = d.ExecContext(c, "insert into graph_nodes(generation_id,node_key,canonical_tuple,kind,display_name,declaration_path,backend,status,cross_rid,provenance)values(?,?,?,?,?,?,?,?,?,?)", x.GenerationID, k, strings.Join(x.NodeKey.CanonicalTuple(), "\x00"), x.Kind, x.DisplayName, x.DeclarationPath, x.Backend, x.Status, x.CrossRID, x.Provenance)
	return e
}
func InsertGraphEdge(c context.Context, d *sql.DB, x model.GraphEdgeRecord) error {
	if e := staged(c, d, x.GenerationID); e != nil {
		return e
	}
	a, e := x.From.Serialize()
	if e != nil {
		return e
	}
	b, e := x.To.Serialize()
	if e != nil {
		return e
	}
	if x.Relation == "" || x.ClaimScope == "" || x.Provenance == "" || x.Status == "" || !strings.HasPrefix(x.CrossRID, "edge:") {
		return fmt.Errorf("%w: edge", model.ErrGraphGenerationInvalid)
	}
	_, e = d.ExecContext(c, "insert into graph_edges(generation_id,from_node_key,to_node_key,relation,claim_scope,evidence_id,provenance,status,cross_rid)values(?,?,?,?,?,?,?,?,?)", x.GenerationID, a, b, x.Relation, x.ClaimScope, x.EvidenceID, x.Provenance, x.Status, x.CrossRID)
	return e
}

func graphDigest(c context.Context, d *sql.DB, id string) (string, error) {
	h := sha256.New()
	for _, q := range []string{"select hex(node_key)||canonical_tuple||kind||display_name||declaration_path||backend||status||cross_rid||provenance from graph_nodes where generation_id=? order by node_key", "select hex(from_node_key)||hex(to_node_key)||relation||claim_scope||ifnull(evidence_id,'')||provenance||status||cross_rid from graph_edges where generation_id=? order by from_node_key,to_node_key,relation,claim_scope", "select evidence_id||source_uri||digest||observed_claim||cross_rid from graph_evidence where generation_id=? order by evidence_id"} {
		r, e := d.QueryContext(c, q, id)
		if e != nil {
			return "", e
		}
		for r.Next() {
			var v string
			if e = r.Scan(&v); e != nil {
				r.Close()
				return "", e
			}
			h.Write([]byte(v))
			h.Write([]byte{0})
		}
		if e = r.Err(); e != nil {
			r.Close()
			return "", e
		}
		r.Close()
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func SealGraphGeneration(c context.Context, d *sql.DB, id string) error {
	if e := staged(c, d, id); e != nil {
		return e
	}
	x, e := graphDigest(c, d, id)
	if e != nil {
		return e
	}
	_, e = d.ExecContext(c, "update graph_generations set sealed_at=?,digest=? where generation_id=?", time.Now().UTC().Format(time.RFC3339Nano), x, id)
	return e
}
func ValidateGraphGeneration(c context.Context, d *sql.DB, id string) (model.GraphGenerationValidation, error) {
	var x string
	if e := d.QueryRowContext(c, "select digest from graph_generations where generation_id=?", id).Scan(&x); e != nil {
		return model.GraphGenerationValidation{}, e
	}
	y, e := graphDigest(c, d, id)
	r := model.GraphGenerationValidation{GenerationID: id, Digest: y, Valid: e == nil && x != "" && x == y}
	if !r.Valid {
		return r, fmt.Errorf("%w: digest", model.ErrGraphGenerationInvalid)
	}
	return r, nil
}
func ActivateGraphGeneration(c context.Context, d *sql.DB, id string) error {
	if _, e := ValidateGraphGeneration(c, d, id); e != nil {
		return e
	}
	t, e := d.BeginTx(c, nil)
	if e != nil {
		return e
	}
	defer t.Rollback()
	var old string
	_ = t.QueryRowContext(c, "select value from workspace_meta where key=?", graphActivePointer).Scan(&old)
	if old != "" && old != id {
		if _, e = t.ExecContext(c, "update graph_generations set status=?,retired_at=? where generation_id=? and status=?", model.GraphGenerationRetired, time.Now().UTC().Format(time.RFC3339Nano), old, model.GraphGenerationActive); e != nil {
			return e
		}
	}
	if _, e = t.ExecContext(c, "update graph_generations set status=?,activated_at=? where generation_id=? and status=?", model.GraphGenerationActive, time.Now().UTC().Format(time.RFC3339Nano), id, model.GraphGenerationStaged); e != nil {
		return e
	}
	if _, e = t.ExecContext(c, "insert into workspace_meta(key,value)values(?,?) on conflict(key) do update set value=excluded.value", graphActivePointer, id); e != nil {
		return e
	}
	return t.Commit()
}
func ActiveGraphGeneration(c context.Context, d *sql.DB) (string, bool, error) {
	var x string
	e := d.QueryRowContext(c, "select value from workspace_meta where key=?", graphActivePointer).Scan(&x)
	if e == sql.ErrNoRows {
		return "", false, nil
	}
	return x, e == nil, e
}
func RollbackGraphGeneration(c context.Context, d *sql.DB, id string) error {
	var s string
	if e := d.QueryRowContext(c, "select status from graph_generations where generation_id=?", id).Scan(&s); e != nil {
		return e
	}
	if s != model.GraphGenerationRetired {
		return fmt.Errorf("%w: rollback target", model.ErrGraphGenerationInvalid)
	}
	_, e := d.ExecContext(c, "update graph_generations set status=?,activated_at=? where generation_id=?", model.GraphGenerationActive, time.Now().UTC().Format(time.RFC3339Nano), id)
	return e
}
