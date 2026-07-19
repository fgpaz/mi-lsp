package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
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
	if g.WorkspaceRoot == "" || g.SchemaVersion < 1 || (g.Status != "" && g.Status != model.GraphGenerationStaged) {
		return fmt.Errorf("%w: generation", model.ErrGraphGenerationInvalid)
	}
	if g.CreatedAt.IsZero() {
		g.CreatedAt = time.Now().UTC()
	}
	if g.GenerationID == "" {
		g.GenerationID = model.GraphGenerationID(g.SchemaVersion, g.WorkspaceRoot, g.SourceFingerprint, g.BackendVersion, g.CompilerVersion, "")
	}
	_, e := d.ExecContext(c, `insert into graph_generations(generation_id,workspace_root,schema_version,created_at,status,owner_id,owner_pid,expected_nodes,expected_edges,expected_evidence,expected_unresolved,source_fingerprint,backend_version,compiler_version) values(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, g.GenerationID, g.WorkspaceRoot, g.SchemaVersion, g.CreatedAt.Format(time.RFC3339Nano), model.GraphGenerationStaged, g.OwnerID, g.OwnerPID, g.ExpectedNodes, g.ExpectedEdges, g.ExpectedEvidence, g.ExpectedUnresolved, g.SourceFingerprint, g.BackendVersion, g.CompilerVersion)
	return e
}

func InsertGraphEvidence(c context.Context, d *sql.DB, x model.GraphEvidence) error {
	if e := staged(c, d, x.GenerationID); e != nil {
		return e
	}
	if x.EvidenceID == "" || x.SourceURI == "" || x.Backend == "" || x.ExtractorVersion == "" || x.Digest == "" || x.ObservedClaim == "" {
		return fmt.Errorf("%w: evidence", model.ErrGraphGenerationInvalid)
	}
	if x.CrossRID == "" {
		x.CrossRID = model.GraphEvidenceCrossRID(x.Digest)
	}
	if !strings.HasPrefix(x.CrossRID, "milsp:gph-evidence:v1:") {
		return fmt.Errorf("%w: evidence RID", model.ErrGraphGenerationInvalid)
	}
	_, e := d.ExecContext(c, `insert into graph_evidence(generation_id,evidence_id,source_uri,source_range,backend,extractor_version,digest,observed_claim,cross_rid,status) values(?,?,?,?,?,?,?,?,?,?)`, x.GenerationID, x.EvidenceID, x.SourceURI, x.SourceRange, x.Backend, x.ExtractorVersion, x.Digest, x.ObservedClaim, x.CrossRID, x.Status)
	return e
}

func InsertGraphNode(c context.Context, d *sql.DB, x model.GraphNodeRecord) error {
	if e := staged(c, d, x.GenerationID); e != nil {
		return e
	}
	k, e := x.NodeKey.Hash()
	if e != nil {
		return e
	}
	payload, _ := x.NodeKey.Serialize()
	if x.Kind == "" || x.DisplayName == "" || x.DeclarationPath == "" || x.Backend == "" || x.Status == "" || x.Provenance == "" {
		return fmt.Errorf("%w: node", model.ErrGraphGenerationInvalid)
	}
	rid := model.GraphNodeCrossRID(hex.EncodeToString(k))
	if x.CrossRID != "" && x.CrossRID != rid {
		return fmt.Errorf("%w: node RID", model.ErrGraphGenerationInvalid)
	}
	_, e = d.ExecContext(c, `insert into graph_nodes(generation_id,node_key,canonical_tuple,kind,display_name,declaration_path,declaration_start,declaration_end,source_fingerprint,backend,compiler_version,confidence,status,cross_rid,provenance) values(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, x.GenerationID, k, payload, x.Kind, x.DisplayName, x.DeclarationPath, x.DeclarationStart, x.DeclarationEnd, x.SourceFingerprint, x.Backend, x.CompilerVersion, x.Confidence, x.Status, rid, x.Provenance)
	if e != nil && strings.Contains(strings.ToLower(e.Error()), "constraint") {
		return fmt.Errorf("%w: %v", model.ErrNodeKeyCollision, e)
	}
	return e
}

func edgeKey(from, to model.NodeKey, relation, scope string) ([]byte, error) {
	a, e := from.Hash()
	if e != nil {
		return nil, e
	}
	b, e := to.Hash()
	if e != nil {
		return nil, e
	}
	h := sha256.New()
	h.Write([]byte("MILSP-EK/v1"))
	for _, v := range [][]byte{a, []byte(relation), b, []byte(scope)} {
		var n [4]byte
		binary.BigEndian.PutUint32(n[:], uint32(len(v)))
		h.Write(n[:])
		h.Write(v)
	}
	return h.Sum(nil), nil
}

func LinkGraphEvidence(c context.Context, d *sql.DB, generation string, edge []byte, evidence string, ordinal int) error {
	_, e := d.ExecContext(c, `insert into graph_edge_evidence(generation_id,edge_key,evidence_id,ordinal) values(?,?,?,?)`, generation, edge, evidence, ordinal)
	return e
}

func InsertGraphEdge(c context.Context, d *sql.DB, x model.GraphEdgeRecord) error {
	if e := staged(c, d, x.GenerationID); e != nil {
		return e
	}
	a, e := x.From.Hash()
	if e != nil {
		return e
	}
	b, e := x.To.Hash()
	if e != nil {
		return e
	}
	ek, e := edgeKey(x.From, x.To, x.Relation, x.ClaimScope)
	if e != nil {
		return e
	}
	if x.Relation == "" || x.ClaimScope == "" || x.Provenance == "" || x.Status == "" {
		return fmt.Errorf("%w: edge", model.ErrGraphGenerationInvalid)
	}
	rid := model.GraphEdgeCrossRID(hex.EncodeToString(ek))
	if x.CrossRID != "" && x.CrossRID != rid {
		return fmt.Errorf("%w: edge RID", model.ErrGraphGenerationInvalid)
	}
	_, e = d.ExecContext(c, `insert into graph_edges(generation_id,edge_key,from_node_key,to_node_key,relation,claim_scope,evidence_id,source_path,source_start,source_end,provenance,confidence,status,cross_rid) values(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, x.GenerationID, ek, a, b, x.Relation, x.ClaimScope, x.EvidenceID, x.SourcePath, x.SourceStart, x.SourceEnd, x.Provenance, x.Confidence, x.Status, rid)
	if e == nil && x.EvidenceID != "" {
		e = LinkGraphEvidence(c, d, x.GenerationID, ek, x.EvidenceID, 0)
	}
	return e
}

func putLP(h interface{ Write([]byte) (int, error) }, v string) {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(v)))
	h.Write(n[:])
	h.Write([]byte(v))
}
func graphDigest(c context.Context, d *sql.DB, id string) (string, error) {
	h := sha256.New()
	putLP(h, "MILSP-GRAPH/v1")
	var v string
	var schema int
	if e := d.QueryRowContext(c, "select schema_version from graph_generations where generation_id=?", id).Scan(&schema); e != nil {
		return "", e
	}
	putLP(h, fmt.Sprint(schema))
	for _, q := range []string{`select hex(node_key),hex(canonical_tuple),kind,display_name,declaration_path,backend,status,cross_rid,provenance from graph_nodes where generation_id=? order by node_key`, `select hex(edge_key),hex(from_node_key),hex(to_node_key),relation,claim_scope,provenance,status,cross_rid from graph_edges where generation_id=? order by edge_key`, `select evidence_id,source_uri,digest,observed_claim,cross_rid from graph_evidence where generation_id=? order by evidence_id`, `select unresolved_id,reason_code,selector,cross_rid,recovery_hint from graph_unresolved where generation_id=? order by unresolved_id`} {
		r, e := d.QueryContext(c, q, id)
		if e != nil {
			return "", e
		}
		for r.Next() {
			cols, _ := r.Columns()
			vals := make([]any, len(cols))
			ptr := make([]any, len(cols))
			for i := range vals {
				ptr[i] = &vals[i]
			}
			if e = r.Scan(ptr...); e != nil {
				r.Close()
				return "", e
			}
			for _, x := range vals {
				if x != nil {
					v = fmt.Sprint(x)
				} else {
					v = ""
				}
				putLP(h, v)
			}
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
	_, e = d.ExecContext(c, `update graph_generations set sealed_at=?,digest=? where generation_id=? and status=?`, time.Now().UTC().Format(time.RFC3339Nano), x, id, model.GraphGenerationStaged)
	return e
}
func ValidateGraphGeneration(c context.Context, d *sql.DB, id string) (model.GraphGenerationValidation, error) {
	var stored, status string
	var n, evi, ed, u int
	if e := d.QueryRowContext(c, `select digest,status,expected_nodes,expected_edges,expected_evidence,expected_unresolved from graph_generations where generation_id=?`, id).Scan(&stored, &status, &n, &ed, &evi, &u); e != nil {
		return model.GraphGenerationValidation{}, e
	}
	digest, e := graphDigest(c, d, id)
	r := model.GraphGenerationValidation{GenerationID: id, Digest: digest, Valid: e == nil && stored != "" && stored == digest}
	if r.Valid {
		var cn, ce, cv, cu int
		d.QueryRowContext(c, "select count(*) from graph_nodes where generation_id=?", id).Scan(&cn)
		d.QueryRowContext(c, "select count(*) from graph_edges where generation_id=?", id).Scan(&ce)
		d.QueryRowContext(c, "select count(*) from graph_evidence where generation_id=?", id).Scan(&cv)
		d.QueryRowContext(c, "select count(*) from graph_unresolved where generation_id=?", id).Scan(&cu)
		r.Nodes, r.Edges, r.Evidence, r.Unresolved = cn, ce, cv, cu
		if (n > 0 && n != cn) || (ed > 0 && ed != ce) || (evi > 0 && evi != cv) || (u > 0 && u != cu) {
			r.Valid = false
		}
	}
	if !r.Valid {
		return r, fmt.Errorf("%w: digest", model.ErrGraphGenerationInvalid)
	}
	if status == model.GraphGenerationActive || status == model.GraphGenerationStaged {
		var count int
		d.QueryRowContext(c, "select count(*) from graph_edges where generation_id=? and status=?", id, model.GraphRecordAccepted).Scan(&count)
		if status == model.GraphGenerationActive && count > 0 {
			var links int
			d.QueryRowContext(c, "select count(*) from graph_edge_evidence where generation_id=?", id).Scan(&links)
			if links < count {
				return r, fmt.Errorf("%w: evidence required", model.ErrGraphGenerationInvalid)
			}
		}
	}
	return r, nil
}

func ActivateGraphGeneration(c context.Context, d *sql.DB, id string) error {
	if _, e := ValidateGraphGeneration(c, d, id); e != nil {
		return e
	}
	t, e := d.BeginTx(c, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if e != nil {
		return e
	}
	defer t.Rollback()
	var status, digest string
	if e = t.QueryRowContext(c, "select status,digest from graph_generations where generation_id=?", id).Scan(&status, &digest); e != nil {
		return e
	}
	if status == model.GraphGenerationActive && digest != "" {
		return t.Commit()
	}
	if status != model.GraphGenerationStaged || digest == "" {
		return fmt.Errorf("%w: only sealed staged generation", model.ErrGraphGenerationInvalid)
	}
	var old string
	_ = t.QueryRowContext(c, "select value from workspace_meta where key=?", graphActivePointer).Scan(&old)
	if old == id {
		return t.Commit()
	}
	if old != "" {
		r, e := t.ExecContext(c, "update graph_generations set status=?,retired_at=? where generation_id=? and status=?", model.GraphGenerationRetired, time.Now().UTC().Format(time.RFC3339Nano), old, model.GraphGenerationActive)
		if e != nil {
			return e
		}
		if n, _ := r.RowsAffected(); n != 1 {
			return model.ErrGraphPointerConflict
		}
	}
	r, e := t.ExecContext(c, "update graph_generations set status=?,activated_at=? where generation_id=? and status=?", model.GraphGenerationActive, time.Now().UTC().Format(time.RFC3339Nano), id, model.GraphGenerationStaged)
	if e != nil {
		return e
	}
	if n, _ := r.RowsAffected(); n != 1 {
		return model.ErrGraphPointerConflict
	}
	if _, e = t.ExecContext(c, `insert into workspace_meta(key,value) values(?,?) on conflict(key) do update set value=excluded.value`, graphActivePointer, id); e != nil {
		return e
	}
	return t.Commit()
}
func ActiveGraphGeneration(c context.Context, d *sql.DB) (string, bool, error) {
	var x string
	e := d.QueryRowContext(c, "select value from workspace_meta where key=?", graphActivePointer).Scan(&x)
	if errors.Is(e, sql.ErrNoRows) {
		return "", false, nil
	}
	return x, e == nil, e
}
func RollbackGraphGeneration(c context.Context, d *sql.DB, id string) error {
	t, e := d.BeginTx(c, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if e != nil {
		return e
	}
	defer t.Rollback()
	var s, digest string
	if e = t.QueryRowContext(c, "select status,digest from graph_generations where generation_id=?", id).Scan(&s, &digest); e != nil {
		return e
	}
	if digest == "" {
		return fmt.Errorf("%w: rollback digest", model.ErrGraphGenerationInvalid)
	}
	if s != model.GraphGenerationRetired {
		return fmt.Errorf("%w: rollback target", model.ErrGraphGenerationInvalid)
	}
	var old string
	_ = t.QueryRowContext(c, "select value from workspace_meta where key=?", graphActivePointer).Scan(&old)
	if old != "" {
		if _, e = t.ExecContext(c, "update graph_generations set status=? where generation_id=? and status=?", model.GraphGenerationRetired, old, model.GraphGenerationActive); e != nil {
			return e
		}
	}
	if _, e = t.ExecContext(c, "update graph_generations set status=?,activated_at=? where generation_id=? and status=?", model.GraphGenerationActive, time.Now().UTC().Format(time.RFC3339Nano), id, model.GraphGenerationRetired); e != nil {
		return e
	}
	if _, e = t.ExecContext(c, `insert into workspace_meta(key,value) values(?,?) on conflict(key) do update set value=excluded.value`, graphActivePointer, id); e != nil {
		return e
	}
	return t.Commit()
}
