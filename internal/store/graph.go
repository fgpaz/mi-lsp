package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

var (
	ErrGraphCrashRecoveryRequired = errors.New("GPH_CRASH_RECOVERY_REQUIRED")
	ErrGraphMigrationTransition   = errors.New("illegal graph migration transition")
)

const (
	graphActiveMeta   = "active_graph_generation_id"
	graphPreviousMeta = "previous_graph_generation_id"
)

type graphConn interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type graphImmediateTx struct {
	c    *sql.Conn
	done bool
}

func beginGraphImmediate(ctx context.Context, db *sql.DB) (*graphImmediateTx, error) {
	c, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if _, err = c.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		_ = c.Close()
		return nil, err
	}
	return &graphImmediateTx{c: c}, nil
}
func (t *graphImmediateTx) commit(ctx context.Context) error {
	if t.done {
		return nil
	}
	_, e := t.c.ExecContext(ctx, "COMMIT")
	t.done = true
	_ = t.c.Close()
	return e
}
func (t *graphImmediateTx) rollback(ctx context.Context) error {
	if t.done {
		return nil
	}
	_, e := t.c.ExecContext(ctx, "ROLLBACK")
	t.done = true
	_ = t.c.Close()
	return e
}

func digestArg(d model.GraphDigest) []byte { return append([]byte(nil), d[:]...) }

func generationMetadataEqual(a, b model.GraphGeneration) bool {
	previousEqual := (a.PreviousGenerationID == nil) == (b.PreviousGenerationID == nil)
	if previousEqual && a.PreviousGenerationID != nil {
		previousEqual = *a.PreviousGenerationID == *b.PreviousGenerationID
	}
	return a.GenerationID == b.GenerationID && a.SchemaVersion == b.SchemaVersion &&
		a.WorkspaceIdentity == b.WorkspaceIdentity && a.RepositoryIdentity == b.RepositoryIdentity &&
		a.SourceFingerprint == b.SourceFingerprint && a.ConfigFingerprint == b.ConfigFingerprint &&
		a.BackendManifestDigest == b.BackendManifestDigest && a.ContentDigest == b.ContentDigest &&
		a.Status == b.Status && a.ErrorCode == b.ErrorCode && a.NodeCount == b.NodeCount &&
		a.EdgeCount == b.EdgeCount && a.EvidenceCount == b.EvidenceCount && a.UnresolvedCount == b.UnresolvedCount && previousEqual &&
		a.CreatedAt.Equal(b.CreatedAt)
}
func scanDigest(v []byte) (model.GraphDigest, error) {
	var d model.GraphDigest
	if len(v) != 32 {
		return d, fmt.Errorf("digest length %d", len(v))
	}
	copy(d[:], v)
	return d, nil
}
func jsonBounded(v []string) (string, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return "", e
	}
	if len(b) > 4096 {
		return "", model.ErrGraphUnresolved
	}
	return string(b), nil
}

// StageGraphGeneration validates a sealed bundle, then inserts the complete bundle in one transaction.
func StageGraphGeneration(ctx context.Context, db *sql.DB, b *model.GraphBundle) error {
	if b == nil || b.Generation.Status != model.GraphGenerationStaged {
		return model.ErrGraphGenerationInvalid
	}
	if err := b.Validate(); err != nil {
		return err
	}
	t, e := beginGraphImmediate(ctx, db)
	if e != nil {
		return e
	}
	defer t.rollback(ctx)
	g := b.Generation
	_, e = t.c.ExecContext(ctx, `INSERT INTO graph_generations(generation_id,schema_version,workspace_identity,source_fingerprint,config_fingerprint,backend_manifest_digest,content_digest,status,node_count,edge_count,evidence_count,unresolved_count,previous_generation_id,created_at,published_at,error_code) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, digestArg(g.GenerationID), g.SchemaVersion, g.WorkspaceIdentity, digestArg(g.SourceFingerprint), digestArg(g.ConfigFingerprint), digestArg(g.BackendManifestDigest), digestArg(g.ContentDigest), model.GraphGenerationStaged, g.NodeCount, g.EdgeCount, g.EvidenceCount, g.UnresolvedCount, nil, g.CreatedAt.UTC().Format(time.RFC3339Nano), nil, nil)
	if e != nil {
		if !strings.Contains(strings.ToLower(e.Error()), "unique") && !strings.Contains(strings.ToLower(e.Error()), "constraint") {
			return e
		}
		existing, le := loadGeneration(ctx, t.c, g.GenerationID)
		if le != nil {
			return model.ErrGraphGenerationCorrupt
		}
		if !generationMetadataEqual(existing, g) {
			return model.ErrGraphGenerationCorrupt
		}
		if _, le = streamGraph(ctx, t.c, g.GenerationID, existing); le != nil {
			return model.ErrGraphGenerationCorrupt
		}
		return t.commit(ctx)
	}
	for _, n := range b.Nodes {
		if _, e = t.c.ExecContext(ctx, `INSERT INTO graph_nodes(generation_id,node_id,node_key,identity_schema,repository_identity,backend_type,language,project_or_module,owner_path,symbol_kind,semantic_identity,display_name,source_digest,claim_status,cross_rid,sort_key) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, digestArg(g.GenerationID), n.NodeID, digestArg(n.NodeKey), n.IdentitySchema, n.Identity.RepositoryIdentity, n.Identity.BackendType, n.Identity.Language, n.Identity.ProjectOrModule, n.Identity.OwnerPath, n.Identity.SymbolKind, n.Identity.SemanticIdentity, n.DisplayName, digestArg(n.SourceDigest), n.ClaimStatus, n.CrossRID, n.SortKey); e != nil {
			return e
		}
	}
	for _, x := range b.Edges {
		if _, e = t.c.ExecContext(ctx, `INSERT INTO graph_edges(generation_id,edge_id,edge_key,from_node_id,to_node_id,relation,claim_scope,claim_status,owner_path,source_backend,cross_rid) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, digestArg(g.GenerationID), x.EdgeID, digestArg(x.EdgeKey), x.FromNodeID, x.ToNodeID, x.Relation, x.ClaimScope, x.ClaimStatus, x.OwnerPath, x.SourceBackend, x.CrossRID); e != nil {
			return e
		}
	}
	for _, x := range b.Evidence {
		var ni, ei any
		if x.NodeID != nil {
			ni = *x.NodeID
		}
		if x.EdgeID != nil {
			ei = *x.EdgeID
		}
		if _, e = t.c.ExecContext(ctx, `INSERT INTO graph_evidence(generation_id,evidence_id,evidence_key,subject_kind,node_id,edge_id,source_uri,start_line,start_column,end_line,end_column,backend,extractor_version,source_digest,claim_kind,observed_claim_digest,claim_status,cross_rid) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, digestArg(g.GenerationID), x.EvidenceID, digestArg(x.EvidenceKey), x.SubjectKind, ni, ei, x.SourceURI, x.StartLine, x.StartColumn, x.EndLine, x.EndColumn, x.Backend, x.ExtractorVersion, digestArg(x.SourceDigest), x.ClaimKind, digestArg(x.ObservedClaimDigest), x.ClaimStatus, x.CrossRID); e != nil {
			return e
		}
	}
	for _, x := range b.Unresolved {
		cj, e := jsonBounded(x.Candidates)
		if e != nil {
			return e
		}
		var sd any
		if x.SourceDigest != nil {
			sd = digestArg(*x.SourceDigest)
		}
		if _, e = t.c.ExecContext(ctx, `INSERT INTO graph_unresolved(generation_id,unresolved_id,unresolved_key,owner_path,subject_kind,selector_digest,reason_code,candidates_json,backend,source_digest,cross_rid,recovery_hint_code) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, digestArg(g.GenerationID), x.UnresolvedID, digestArg(x.UnresolvedKey), x.OwnerPath, x.SubjectKind, digestArg(x.SelectorDigest), x.ReasonCode, cj, x.Backend, sd, x.CrossRID, x.RecoveryHintCode); e != nil {
			return e
		}
	}
	return t.commit(ctx)
}

func loadGeneration(ctx context.Context, q graphConn, id model.GraphDigest) (model.GraphGeneration, error) {
	var g model.GraphGeneration
	var raw, src, cfg, bak, content, prev []byte
	var created, published sql.NullString
	var status, errorCode sql.NullString
	err := q.QueryRowContext(ctx, `SELECT generation_id,schema_version,workspace_identity,source_fingerprint,config_fingerprint,backend_manifest_digest,content_digest,status,node_count,edge_count,evidence_count,unresolved_count,previous_generation_id,created_at,published_at,error_code FROM graph_generations WHERE generation_id=?`, digestArg(id)).Scan(&raw, &g.SchemaVersion, &g.WorkspaceIdentity, &src, &cfg, &bak, &content, &status, &g.NodeCount, &g.EdgeCount, &g.EvidenceCount, &g.UnresolvedCount, &prev, &created, &published, &errorCode)
	if err != nil {
		return g, err
	}
	var e error
	if g.GenerationID, e = scanDigest(raw); e != nil {
		return g, fmt.Errorf("%w: generation_id: %w", model.ErrGraphGenerationCorrupt, e)
	}
	g.RepositoryIdentity = g.WorkspaceIdentity
	if g.SourceFingerprint, e = scanDigest(src); e != nil {
		return g, fmt.Errorf("%w: source_fingerprint: %w", model.ErrGraphGenerationCorrupt, e)
	}
	if g.ConfigFingerprint, e = scanDigest(cfg); e != nil {
		return g, fmt.Errorf("%w: config_fingerprint: %w", model.ErrGraphGenerationCorrupt, e)
	}
	if g.BackendManifestDigest, e = scanDigest(bak); e != nil {
		return g, fmt.Errorf("%w: backend_manifest_digest: %w", model.ErrGraphGenerationCorrupt, e)
	}
	if g.ContentDigest, e = scanDigest(content); e != nil {
		return g, fmt.Errorf("%w: content_digest: %w", model.ErrGraphGenerationCorrupt, e)
	}
	g.Status = status.String
	g.ErrorCode = errorCode.String
	if !created.Valid || created.String == "" {
		return g, fmt.Errorf("%w: created_at missing", model.ErrGraphGenerationCorrupt)
	}
	if g.CreatedAt, e = time.Parse(time.RFC3339Nano, created.String); e != nil {
		return g, fmt.Errorf("%w: created_at: %w", model.ErrGraphGenerationCorrupt, e)
	}
	if published.Valid {
		if published.String == "" {
			return g, fmt.Errorf("%w: published_at empty", model.ErrGraphGenerationCorrupt)
		}
		if g.PublishedAt, e = time.Parse(time.RFC3339Nano, published.String); e != nil {
			return g, fmt.Errorf("%w: published_at: %w", model.ErrGraphGenerationCorrupt, e)
		}
	}
	if len(prev) > 0 {
		d, e := scanDigest(prev)
		if e != nil {
			return g, fmt.Errorf("%w: previous_generation_id: %w", model.ErrGraphGenerationCorrupt, e)
		}
		g.PreviousGenerationID = &d
	}
	return g, nil
}

func streamGraph(ctx context.Context, q graphConn, id model.GraphDigest, g model.GraphGeneration) (model.GraphDigest, error) {
	h, e := model.NewGraphContentHasher(g.NodeCount, g.EdgeCount, g.EvidenceCount, g.UnresolvedCount)
	if e != nil {
		return model.GraphDigest{}, e
	}
	nodeKeys := map[int]model.GraphDigest{}
	edgeKeys := map[int]model.GraphDigest{}
	edgeStatus := map[int]string{}
	rows, e := q.QueryContext(ctx, `SELECT node_id,node_key,identity_schema,repository_identity,backend_type,language,project_or_module,owner_path,symbol_kind,semantic_identity,display_name,source_digest,claim_status,cross_rid,sort_key FROM graph_nodes WHERE generation_id=? ORDER BY node_id`, digestArg(id))
	if e != nil {
		return model.GraphDigest{}, e
	}
	for rows.Next() {
		var n model.GraphNodeRecord
		var nk, sd []byte
		if e = rows.Scan(&n.NodeID, &nk, &n.IdentitySchema, &n.Identity.RepositoryIdentity, &n.Identity.BackendType, &n.Identity.Language, &n.Identity.ProjectOrModule, &n.Identity.OwnerPath, &n.Identity.SymbolKind, &n.Identity.SemanticIdentity, &n.DisplayName, &sd, &n.ClaimStatus, &n.CrossRID, &n.SortKey); e != nil {
			rows.Close()
			return model.GraphDigest{}, e
		}
		n.GenerationID = id
		n.NodeKey, e = scanDigest(nk)
		if e != nil {
			rows.Close()
			return model.GraphDigest{}, e
		}
		n.SourceDigest, e = scanDigest(sd)
		if e != nil {
			rows.Close()
			return model.GraphDigest{}, e
		}
		if ve := model.ValidateGraphNodeRecord(n); ve != nil {
			rows.Close()
			return model.GraphDigest{}, ve
		}
		computed := n.NodeKey
		nodeKeys[n.NodeID] = computed
		if e = h.AddNode(n); e != nil {
			rows.Close()
			return model.GraphDigest{}, e
		}
	}
	if e = rows.Err(); e != nil {
		return model.GraphDigest{}, e
	}
	rows.Close()
	rows, e = q.QueryContext(ctx, `SELECT edge_id,edge_key,from_node_id,to_node_id,relation,claim_scope,claim_status,owner_path,source_backend,cross_rid FROM graph_edges WHERE generation_id=? ORDER BY edge_id`, digestArg(id))
	if e != nil {
		return model.GraphDigest{}, e
	}
	for rows.Next() {
		var x model.GraphEdgeRecord
		var ek []byte
		if e = rows.Scan(&x.EdgeID, &ek, &x.FromNodeID, &x.ToNodeID, &x.Relation, &x.ClaimScope, &x.ClaimStatus, &x.OwnerPath, &x.SourceBackend, &x.CrossRID); e != nil {
			rows.Close()
			return model.GraphDigest{}, e
		}
		x.GenerationID = id
		x.EdgeKey, e = scanDigest(ek)
		if e != nil {
			return model.GraphDigest{}, e
		}
		from, fok := nodeKeys[x.FromNodeID]
		to, tok := nodeKeys[x.ToNodeID]
		computed := model.EdgeKey(from, to, x.Relation, x.ClaimScope)
		if !fok || !tok || model.ValidateGraphEdgeRecord(x, from, to) != nil {
			return model.GraphDigest{}, model.ErrGraphEdgeInvalid
		}
		edgeKeys[x.EdgeID] = computed
		edgeStatus[x.EdgeID] = x.ClaimStatus
		if e = h.AddEdge(x); e != nil {
			return model.GraphDigest{}, e
		}
	}
	if e = rows.Err(); e != nil {
		return model.GraphDigest{}, e
	}
	rows.Close()
	rows, e = q.QueryContext(ctx, `SELECT evidence_id,evidence_key,subject_kind,node_id,edge_id,source_uri,start_line,start_column,end_line,end_column,backend,extractor_version,source_digest,claim_kind,observed_claim_digest,claim_status,cross_rid FROM graph_evidence WHERE generation_id=? ORDER BY evidence_id`, digestArg(id))
	if e != nil {
		return model.GraphDigest{}, e
	}
	for rows.Next() {
		var x model.GraphEvidence
		var ek, sd, oc []byte
		var ni, ei sql.NullInt64
		if e = rows.Scan(&x.EvidenceID, &ek, &x.SubjectKind, &ni, &ei, &x.SourceURI, &x.StartLine, &x.StartColumn, &x.EndLine, &x.EndColumn, &x.Backend, &x.ExtractorVersion, &sd, &x.ClaimKind, &oc, &x.ClaimStatus, &x.CrossRID); e != nil {
			return model.GraphDigest{}, e
		}
		x.GenerationID = id
		x.EvidenceKey, e = scanDigest(ek)
		if e != nil {
			return model.GraphDigest{}, e
		}
		x.SourceDigest, e = scanDigest(sd)
		if e != nil {
			return model.GraphDigest{}, e
		}
		x.ObservedClaimDigest, e = scanDigest(oc)
		if e != nil {
			return model.GraphDigest{}, e
		}
		if ni.Valid == ei.Valid {
			return model.GraphDigest{}, model.ErrGraphEvidenceInvalid
		}
		var subject model.GraphDigest
		if ni.Valid {
			var ok bool
			subject, ok = nodeKeys[int(ni.Int64)]
			if !ok || x.SubjectKind != "node" {
				return model.GraphDigest{}, model.ErrGraphEvidenceInvalid
			}
		} else {
			var ok bool
			subject, ok = edgeKeys[int(ei.Int64)]
			if !ok || x.SubjectKind != "edge" {
				return model.GraphDigest{}, model.ErrGraphEvidenceInvalid
			}
		}
		sl, sc, el, ec := 0, 0, 0, 0
		if x.StartLine != nil {
			sl = *x.StartLine
		}
		if x.StartColumn != nil {
			sc = *x.StartColumn
		}
		if x.EndLine != nil {
			el = *x.EndLine
		}
		if x.EndColumn != nil {
			ec = *x.EndColumn
		}
		x.EvidenceDigest = model.EvidenceDigest(x.SourceDigest, x.ObservedClaimDigest, x.SourceURI, x.ClaimKind, x.Backend, x.ExtractorVersion, sl, sc, el, ec)
		if ni.Valid {
			v := int(ni.Int64)
			x.NodeID = &v
		}
		if ei.Valid {
			v := int(ei.Int64)
			x.EdgeID = &v
		}
		if ve := model.ValidateGraphEvidence(x, subject); ve != nil {
			return model.GraphDigest{}, ve
		}
		if e = h.AddEvidence(x); e != nil {
			return model.GraphDigest{}, e
		}
	}
	if e = rows.Err(); e != nil {
		return model.GraphDigest{}, e
	}
	rows.Close()
	rows, e = q.QueryContext(ctx, `SELECT unresolved_id,unresolved_key,owner_path,subject_kind,selector_digest,reason_code,candidates_json,backend,source_digest,cross_rid,recovery_hint_code FROM graph_unresolved WHERE generation_id=? ORDER BY unresolved_id`, digestArg(id))
	if e != nil {
		return model.GraphDigest{}, e
	}
	for rows.Next() {
		var x model.GraphUnresolved
		var uk, sel, sd []byte
		var cj string
		var hint sql.NullString
		if e = rows.Scan(&x.UnresolvedID, &uk, &x.OwnerPath, &x.SubjectKind, &sel, &x.ReasonCode, &cj, &x.Backend, &sd, &x.CrossRID, &hint); e != nil {
			return model.GraphDigest{}, e
		}
		x.GenerationID = id
		x.UnresolvedKey, e = scanDigest(uk)
		if e != nil {
			return model.GraphDigest{}, e
		}
		x.SelectorDigest, e = scanDigest(sel)
		if e != nil {
			return model.GraphDigest{}, e
		}
		if len(sd) > 0 {
			d, e := scanDigest(sd)
			if e != nil {
				return model.GraphDigest{}, e
			}
			x.SourceDigest = &d
		}
		if hint.Valid {
			x.RecoveryHintCode = hint.String
		}
		if e = json.Unmarshal([]byte(cj), &x.Candidates); e != nil {
			return model.GraphDigest{}, e
		}
		if ve := model.ValidateGraphUnresolved(x); ve != nil {
			return model.GraphDigest{}, ve
		}
		if e = h.AddUnresolved(x); e != nil {
			return model.GraphDigest{}, e
		}
	}
	if e = rows.Err(); e != nil {
		return model.GraphDigest{}, e
	}
	return h.Sum()
}

func ValidateGraphGeneration(ctx context.Context, db *sql.DB, id model.GraphDigest) (model.GraphGeneration, error) {
	return validateGraphGenerationConn(ctx, db, id)
}

func validateGraphGenerationConn(ctx context.Context, q graphConn, id model.GraphDigest) (model.GraphGeneration, error) {
	g, e := loadGeneration(ctx, q, id)
	if e != nil {
		return g, e
	}
	if g.SourceFingerprint == (model.GraphDigest{}) || g.ConfigFingerprint == (model.GraphDigest{}) || g.BackendManifestDigest == (model.GraphDigest{}) {
		return g, fmt.Errorf("%w: fingerprints", model.ErrGraphGenerationCorrupt)
	}
	d, e := streamGraph(ctx, q, id, g)
	if e != nil || d != g.ContentDigest {
		return g, fmt.Errorf("%w: content", model.ErrGraphGenerationCorrupt)
	}
	expectedID := model.DeriveGenerationID(g.SchemaVersion, g.WorkspaceIdentity, g.SourceFingerprint, g.ConfigFingerprint, g.BackendManifestDigest, d)
	if expectedID != g.GenerationID {
		return g, fmt.Errorf("%w: generation_id", model.ErrGraphGenerationCorrupt)
	}
	var n, e1, ev, u int
	for _, x := range []struct {
		q string
		p *int
	}{{"SELECT COUNT(*) FROM graph_nodes WHERE generation_id=?", &n}, {"SELECT COUNT(*) FROM graph_edges WHERE generation_id=?", &e1}, {"SELECT COUNT(*) FROM graph_evidence WHERE generation_id=?", &ev}, {"SELECT COUNT(*) FROM graph_unresolved WHERE generation_id=?", &u}} {
		if e = q.QueryRowContext(ctx, x.q, digestArg(id)).Scan(x.p); e != nil {
			return g, e
		}
	}
	if n != g.NodeCount || e1 != g.EdgeCount || ev != g.EvidenceCount || u != g.UnresolvedCount {
		return g, fmt.Errorf("%w: counts", model.ErrGraphGenerationInvalid)
	}
	{
		var missing int
		if e = q.QueryRowContext(ctx, `SELECT COUNT(*) FROM graph_edges x WHERE x.generation_id=? AND NOT EXISTS (SELECT 1 FROM graph_evidence e WHERE e.generation_id=x.generation_id AND e.edge_id=x.edge_id)`, digestArg(id)).Scan(&missing); e != nil {
			return g, e
		}
		if missing != 0 {
			return g, fmt.Errorf("%w: evidence", model.ErrGraphGenerationCorrupt)
		}
	}
	return g, nil
}

func ActiveGraphGeneration(ctx context.Context, db *sql.DB) (model.GraphDigest, bool, error) {
	var b []byte
	err := db.QueryRowContext(ctx, "SELECT value FROM workspace_meta WHERE key=?", graphActiveMeta).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) || len(b) == 0 {
		return model.GraphDigest{}, false, nil
	}
	if err != nil {
		return model.GraphDigest{}, false, err
	}
	d, e := scanDigest(b)
	return d, e == nil, e
}

func ActivateGraphGeneration(ctx context.Context, db *sql.DB, id model.GraphDigest, expectedPrior *model.GraphDigest) error {
	t, e := beginGraphImmediate(ctx, db)
	if e != nil {
		return e
	}
	defer t.rollback(ctx)
	g, e := validateGraphGenerationConn(ctx, t.c, id)
	if e != nil {
		return e
	}
	var old []byte
	metaErr := t.c.QueryRowContext(ctx, "SELECT value FROM workspace_meta WHERE key=?", graphActiveMeta).Scan(&old)
	if metaErr != nil && !errors.Is(metaErr, sql.ErrNoRows) {
		return metaErr
	}
	if (expectedPrior == nil) != (len(old) == 0) || (expectedPrior != nil && string(digestArg(*expectedPrior)) != string(old)) {
		return model.ErrGraphPointerConflict
	}
	var activeRows int
	if e = t.c.QueryRowContext(ctx, "SELECT COUNT(*) FROM graph_generations WHERE status=?", model.GraphGenerationActive).Scan(&activeRows); e != nil {
		return e
	}
	if (len(old) == 0 && activeRows != 0) || (len(old) != 0 && activeRows != 1) {
		return model.ErrGraphPointerConflict
	}
	if len(old) > 0 {
		oldID, err := scanDigest(old)
		if err != nil {
			return model.ErrGraphPointerConflict
		}
		oldGeneration, err := validateGraphGenerationConn(ctx, t.c, oldID)
		if err != nil || oldGeneration.Status != model.GraphGenerationActive {
			return model.ErrGraphPointerConflict
		}
	}
	if g.Status == model.GraphGenerationActive {
		if len(old) != len(digestArg(id)) || string(old) != string(digestArg(id)) {
			return model.ErrGraphPointerConflict
		}
		return t.commit(ctx)
	}
	if g.Status != model.GraphGenerationStaged {
		return model.ErrGraphGenerationInvalid
	}
	if len(old) > 0 {
		r, e := t.c.ExecContext(ctx, "UPDATE graph_generations SET status=? WHERE generation_id=? AND status=?", model.GraphGenerationRetired, old, model.GraphGenerationActive)
		if e != nil {
			return e
		}
		n, e := r.RowsAffected()
		if e != nil {
			return e
		}
		if n != 1 {
			return model.ErrGraphPointerConflict
		}
	}
	r, e := t.c.ExecContext(ctx, "UPDATE graph_generations SET status=?,published_at=? WHERE generation_id=? AND status=?", model.GraphGenerationActive, time.Now().UTC().Format(time.RFC3339Nano), digestArg(id), model.GraphGenerationStaged)
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return model.ErrGraphPointerConflict
	}
	if _, e = t.c.ExecContext(ctx, "INSERT INTO workspace_meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", graphPreviousMeta, old); e != nil {
		return e
	}
	if _, e = t.c.ExecContext(ctx, "INSERT INTO workspace_meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", graphActiveMeta, digestArg(id)); e != nil {
		return e
	}
	return t.commit(ctx)
}

func RollbackGraphGeneration(ctx context.Context, db *sql.DB, id model.GraphDigest, expectedCurrent *model.GraphDigest) error {
	t, e := beginGraphImmediate(ctx, db)
	if e != nil {
		return e
	}
	defer t.rollback(ctx)
	target, e := validateGraphGenerationConn(ctx, t.c, id)
	if e != nil {
		return e
	}
	if target.Status != model.GraphGenerationRetired {
		return model.ErrGraphGenerationInvalid
	}
	var active []byte
	metaErr := t.c.QueryRowContext(ctx, "SELECT value FROM workspace_meta WHERE key=?", graphActiveMeta).Scan(&active)
	if metaErr != nil && !errors.Is(metaErr, sql.ErrNoRows) {
		return metaErr
	}
	if (expectedCurrent == nil) || len(active) == 0 || string(digestArg(*expectedCurrent)) != string(active) {
		return model.ErrGraphPointerConflict
	}
	currentID, se := scanDigest(active)
	if se != nil {
		return model.ErrGraphPointerConflict
	}
	current, se := validateGraphGenerationConn(ctx, t.c, currentID)
	if se != nil || current.Status != model.GraphGenerationActive {
		return model.ErrGraphPointerConflict
	}
	r, e := t.c.ExecContext(ctx, "UPDATE graph_generations SET status=? WHERE generation_id=? AND status=?", model.GraphGenerationRetired, active, model.GraphGenerationActive)
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return model.ErrGraphPointerConflict
	}
	r, e = t.c.ExecContext(ctx, "UPDATE graph_generations SET status=? WHERE generation_id=? AND status=?", model.GraphGenerationActive, digestArg(id), model.GraphGenerationRetired)
	if e != nil {
		return e
	}
	n, e = r.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return model.ErrGraphPointerConflict
	}
	r, e = t.c.ExecContext(ctx, "UPDATE workspace_meta SET value=? WHERE key=?", digestArg(id), graphActiveMeta)
	if e != nil {
		return e
	}
	if n, e = r.RowsAffected(); e != nil {
		return e
	} else if n != 1 {
		return model.ErrGraphPointerConflict
	}
	r, e = t.c.ExecContext(ctx, "INSERT INTO workspace_meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", graphPreviousMeta, active)
	if e != nil {
		return e
	}
	if n, e = r.RowsAffected(); e != nil {
		return e
	} else if n != 1 {
		return model.ErrGraphPointerConflict
	}
	var activeCount int
	if e = t.c.QueryRowContext(ctx, "SELECT COUNT(*) FROM graph_generations WHERE status=?", model.GraphGenerationActive).Scan(&activeCount); e != nil || activeCount != 1 {
		return model.ErrGraphPointerConflict
	}
	return t.commit(ctx)
}

// PrepareGraphMigration records an exact, idempotent migration intent.
func PrepareGraphMigration(ctx context.Context, db *sql.DB, m model.GraphMigration) error {
	if m.MigrationID == "" || m.FromVersion <= 0 || m.ToVersion <= m.FromVersion || m.Status != "prepared" ||
		m.PreflightDigest == (model.GraphDigest{}) || m.BackupDigest == (model.GraphDigest{}) || m.StartedAt.IsZero() {
		return ErrGraphMigrationTransition
	}
	if m.PriorActiveGenerationID != nil && *m.PriorActiveGenerationID == (model.GraphDigest{}) {
		return ErrGraphMigrationTransition
	}
	t, e := beginGraphImmediate(ctx, db)
	if e != nil {
		return e
	}
	defer t.rollback(ctx)
	var existing model.GraphMigration
	var pre, bak, prior []byte
	var started string
	e = t.c.QueryRowContext(ctx, `SELECT migration_id,from_version,to_version,status,preflight_digest,backup_digest,prior_active_generation_id,started_at FROM graph_migrations WHERE migration_id=?`, m.MigrationID).Scan(&existing.MigrationID, &existing.FromVersion, &existing.ToVersion, &existing.Status, &pre, &bak, &prior, &started)
	if e == nil {
		if existing.FromVersion != m.FromVersion || existing.ToVersion != m.ToVersion || existing.Status != m.Status || string(pre) != string(m.PreflightDigest[:]) || string(bak) != string(m.BackupDigest[:]) || string(prior) != string(digestOrNil(m.PriorActiveGenerationID)) || started != m.StartedAt.UTC().Format(time.RFC3339Nano) {
			return ErrGraphMigrationTransition
		}
		return t.commit(ctx)
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	var priorValue any
	if m.PriorActiveGenerationID != nil {
		priorValue = digestArg(*m.PriorActiveGenerationID)
	}
	_, e = t.c.ExecContext(ctx, `INSERT INTO graph_migrations(migration_id,from_version,to_version,status,preflight_digest,backup_digest,prior_active_generation_id,started_at) VALUES(?,?,?,?,?,?,?,?)`, m.MigrationID, m.FromVersion, m.ToVersion, m.Status, digestArg(m.PreflightDigest), digestArg(m.BackupDigest), priorValue, m.StartedAt.UTC().Format(time.RFC3339Nano))
	if e != nil {
		return e
	}
	return t.commit(ctx)
}

func digestOrNil(d *model.GraphDigest) []byte {
	if d == nil {
		return nil
	}
	return digestArg(*d)
}

// TransitionGraphMigration performs a compare-and-set lifecycle transition.
func TransitionGraphMigration(ctx context.Context, db *sql.DB, id, expected, next, errorCode string, now time.Time) error {
	allowed := map[string]map[string]bool{"prepared": {"applying": true, "rolled_back": true, "failed": true}, "applying": {"validated": true, "rolled_back": true, "failed": true}, "validated": {"committed": true, "rolled_back": true, "failed": true}}
	if id == "" || !allowed[expected][next] || (next == "failed" && errorCode == "") {
		return ErrGraphMigrationTransition
	}
	t, e := beginGraphImmediate(ctx, db)
	if e != nil {
		return e
	}
	defer t.rollback(ctx)
	completed := any(nil)
	if next == "committed" || next == "rolled_back" || next == "failed" {
		completed = now.UTC().Format(time.RFC3339Nano)
	}
	r, e := t.c.ExecContext(ctx, `UPDATE graph_migrations SET status=?,error_code=?,completed_at=? WHERE migration_id=? AND status=?`, next, errorCode, completed, id, expected)
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return ErrGraphMigrationTransition
	}
	return t.commit(ctx)
}

func RollbackGraphMigration(ctx context.Context, db *sql.DB, id, expected, errorCode string, now time.Time) error {
	return TransitionGraphMigration(ctx, db, id, expected, "rolled_back", errorCode, now)
}
