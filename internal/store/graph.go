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
	ErrGraphCompatibilityConflict = errors.New("graph compatibility compare-and-set conflict")
)

const (
	GraphCompatibilityLegacyPreservedNoDualWrite = "legacy-preserved-no-dual-write"
	GraphCompatibilityDualReadWrite              = "dual-read-write"
	GraphCompatibilityGraphAuthoritative         = "graph-authoritative"
)

const (
	graphActiveMeta            = "active_graph_generation_id"
	graphPreviousMeta          = "previous_graph_generation_id"
	GraphCatalogGenerationMeta = "graph_catalog_generation_id"
	GraphRuntimeStateMeta      = "graph_runtime_state"
	GraphRuntimeFresh          = "fresh"
	GraphRuntimeStale          = "stale"
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
	if ctx == nil || db == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
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
func (t *graphImmediateTx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return t.c.QueryContext(ctx, query, args...)
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
		a.WorkspaceIdentity == b.WorkspaceIdentity && a.SourceFingerprint == b.SourceFingerprint &&
		a.ConfigFingerprint == b.ConfigFingerprint &&
		a.BackendManifestDigest == b.BackendManifestDigest && a.ContentDigest == b.ContentDigest &&
		a.Status == b.Status && a.ErrorCode == b.ErrorCode && a.NodeCount == b.NodeCount &&
		a.EdgeCount == b.EdgeCount && a.EvidenceCount == b.EvidenceCount && a.UnresolvedCount == b.UnresolvedCount && previousEqual &&
		a.CreatedAt.Equal(b.CreatedAt)
}

// graphGenerationImmutableMetadataEqual compares only metadata that remains
// fixed across staging and activation. Status, publication timestamps, the
// previous-generation link, and the candidate creation time are publication
// metadata and may differ on an active duplicate restage.
func graphGenerationImmutableMetadataEqual(a, b model.GraphGeneration) bool {
	return a.GenerationID == b.GenerationID && a.SchemaVersion == b.SchemaVersion &&
		a.WorkspaceIdentity == b.WorkspaceIdentity &&
		a.SourceFingerprint == b.SourceFingerprint && a.ConfigFingerprint == b.ConfigFingerprint &&
		a.BackendManifestDigest == b.BackendManifestDigest && a.ContentDigest == b.ContentDigest &&
		a.ErrorCode == b.ErrorCode && a.NodeCount == b.NodeCount && a.EdgeCount == b.EdgeCount &&
		a.EvidenceCount == b.EvidenceCount && a.UnresolvedCount == b.UnresolvedCount
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
	if ctx == nil || db == nil {
		return model.ErrGraphGenerationInvalid
	}
	t, err := beginGraphImmediate(ctx, db)
	if err != nil {
		return err
	}
	defer t.rollback(ctx)
	if err := stageGraphGenerationConn(ctx, t.c, b); err != nil {
		return err
	}
	return t.commit(ctx)
}

// StageGraphGenerationTx stages a sealed graph bundle on an existing transaction.
// Owned index publication uses it to keep graph rows and publication terminality
// inside one commit boundary.
func StageGraphGenerationTx(ctx context.Context, tx *sql.Tx, b *model.GraphBundle) error {
	if tx == nil {
		return model.ErrGraphGenerationInvalid
	}
	return stageGraphGenerationConn(ctx, tx, b)
}

func stageGraphGenerationConn(ctx context.Context, q graphConn, b *model.GraphBundle) error {
	if ctx == nil || q == nil || b == nil || b.Generation.Status != model.GraphGenerationStaged {
		return model.ErrGraphGenerationInvalid
	}
	if err := b.Validate(); err != nil {
		return err
	}
	g := b.Generation
	_, err := q.ExecContext(ctx, `INSERT INTO graph_generations(generation_id,schema_version,workspace_identity,source_fingerprint,config_fingerprint,backend_manifest_digest,content_digest,status,node_count,edge_count,evidence_count,unresolved_count,previous_generation_id,created_at,published_at,error_code) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, digestArg(g.GenerationID), g.SchemaVersion, g.WorkspaceIdentity, digestArg(g.SourceFingerprint), digestArg(g.ConfigFingerprint), digestArg(g.BackendManifestDigest), digestArg(g.ContentDigest), model.GraphGenerationStaged, g.NodeCount, g.EdgeCount, g.EvidenceCount, g.UnresolvedCount, digestOrNil(g.PreviousGenerationID), g.CreatedAt.UTC().Format(time.RFC3339Nano), nil, nil)
	if err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "unique") && !strings.Contains(strings.ToLower(err.Error()), "constraint") {
			return err
		}
		existing, loadErr := loadGeneration(ctx, q, g.GenerationID)
		if loadErr != nil {
			return model.ErrGraphGenerationCorrupt
		}
		if existing.Status == model.GraphGenerationActive {
			if !graphGenerationImmutableMetadataEqual(existing, g) {
				return model.ErrGraphGenerationCorrupt
			}
			if _, validationErr := validateGraphGenerationConn(ctx, q, g.GenerationID); validationErr != nil {
				return model.ErrGraphGenerationCorrupt
			}
			return nil
		}
		if existing.Status == model.GraphGenerationRetired {
			// A canonical generation may be observed again after a different
			// topology/backend generation temporarily became active. Reuse the
			// immutable graph rows only when the retired row is an exact,
			// independently validated match; mismatches remain corruption.
			if !graphGenerationImmutableMetadataEqual(existing, g) {
				return model.ErrGraphGenerationCorrupt
			}
			if _, validationErr := validateGraphGenerationConn(ctx, q, g.GenerationID); validationErr != nil {
				return model.ErrGraphGenerationCorrupt
			}
			return nil
		}
		if !generationMetadataEqual(existing, g) {
			return model.ErrGraphGenerationCorrupt
		}
		if _, loadErr = streamGraph(ctx, q, g.GenerationID, existing); loadErr != nil {
			return model.ErrGraphGenerationCorrupt
		}
		return nil
	}
	for _, n := range b.Nodes {
		if _, err = q.ExecContext(ctx, `INSERT INTO graph_nodes(generation_id,node_id,node_key,identity_schema,repository_identity,backend_type,language,project_or_module,owner_path,symbol_kind,semantic_identity,display_name,source_digest,claim_status,cross_rid,sort_key) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, digestArg(g.GenerationID), n.NodeID, digestArg(n.NodeKey), n.IdentitySchema, n.Identity.RepositoryIdentity, n.Identity.BackendType, n.Identity.Language, n.Identity.ProjectOrModule, n.Identity.OwnerPath, n.Identity.SymbolKind, n.Identity.SemanticIdentity, n.DisplayName, digestArg(n.SourceDigest), n.ClaimStatus, n.CrossRID, n.SortKey); err != nil {
			return err
		}
	}
	for _, x := range b.Edges {
		if _, err = q.ExecContext(ctx, `INSERT INTO graph_edges(generation_id,edge_id,edge_key,from_node_id,to_node_id,relation,claim_scope,claim_status,owner_path,source_backend,cross_rid) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, digestArg(g.GenerationID), x.EdgeID, digestArg(x.EdgeKey), x.FromNodeID, x.ToNodeID, x.Relation, x.ClaimScope, x.ClaimStatus, x.OwnerPath, x.SourceBackend, x.CrossRID); err != nil {
			return err
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
		if _, err = q.ExecContext(ctx, `INSERT INTO graph_evidence(generation_id,evidence_id,evidence_key,subject_kind,node_id,edge_id,source_uri,start_line,start_column,end_line,end_column,backend,extractor_version,source_digest,claim_kind,observed_claim_digest,claim_status,cross_rid) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, digestArg(g.GenerationID), x.EvidenceID, digestArg(x.EvidenceKey), x.SubjectKind, ni, ei, x.SourceURI, x.StartLine, x.StartColumn, x.EndLine, x.EndColumn, x.Backend, x.ExtractorVersion, digestArg(x.SourceDigest), x.ClaimKind, digestArg(x.ObservedClaimDigest), x.ClaimStatus, x.CrossRID); err != nil {
			return err
		}
	}
	for _, x := range b.Unresolved {
		candidates, err := jsonBounded(x.Candidates)
		if err != nil {
			return err
		}
		var sourceDigest any
		if x.SourceDigest != nil {
			sourceDigest = digestArg(*x.SourceDigest)
		}
		if _, err = q.ExecContext(ctx, `INSERT INTO graph_unresolved(generation_id,unresolved_id,unresolved_key,owner_path,subject_kind,selector_digest,reason_code,candidates_json,backend,source_digest,cross_rid,recovery_hint_code,source_document,source_block,target_kind,target_value) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, digestArg(g.GenerationID), x.UnresolvedID, digestArg(x.UnresolvedKey), x.OwnerPath, x.SubjectKind, digestArg(x.SelectorDigest), x.ReasonCode, candidates, x.Backend, sourceDigest, x.CrossRID, x.RecoveryHintCode, nullableGraphText(x.SourceDocument), nullableGraphText(x.SourceBlock), nullableGraphText(x.TargetKind), nullableGraphText(x.TargetValue)); err != nil {
			return err
		}
	}
	return nil
}

func nullableGraphText(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
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

type graphContentValidationMode uint8

const (
	graphContentValidationCurrent graphContentValidationMode = iota
	graphContentValidationLegacy
)

func streamGraph(ctx context.Context, q graphConn, id model.GraphDigest, g model.GraphGeneration) (model.GraphDigest, error) {
	return streamGraphWithMode(ctx, q, id, g, graphContentValidationCurrent)
}

func streamGraphWithMode(ctx context.Context, q graphConn, id model.GraphDigest, g model.GraphGeneration, mode graphContentValidationMode) (model.GraphDigest, error) {
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
	defer rows.Close()
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
	defer rows.Close()
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
	defer rows.Close()
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
	rows, e = q.QueryContext(ctx, `SELECT unresolved_id,unresolved_key,owner_path,subject_kind,selector_digest,reason_code,candidates_json,backend,source_digest,cross_rid,recovery_hint_code,source_document,source_block,target_kind,target_value FROM graph_unresolved WHERE generation_id=? ORDER BY unresolved_id`, digestArg(id))
	if e != nil {
		return model.GraphDigest{}, e
	}
	defer rows.Close()
	for rows.Next() {
		var x model.GraphUnresolved
		var uk, sel, sd []byte
		var cj string
		var hint, sourceDocument, sourceBlock, targetKind, targetValue sql.NullString
		if e = rows.Scan(&x.UnresolvedID, &uk, &x.OwnerPath, &x.SubjectKind, &sel, &x.ReasonCode, &cj, &x.Backend, &sd, &x.CrossRID, &hint, &sourceDocument, &sourceBlock, &targetKind, &targetValue); e != nil {
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
		if sourceDocument.Valid {
			x.SourceDocument = sourceDocument.String
		}
		if sourceBlock.Valid {
			x.SourceBlock = sourceBlock.String
		}
		if targetKind.Valid {
			x.TargetKind = targetKind.String
		}
		if targetValue.Valid {
			x.TargetValue = targetValue.String
		}
		if e = model.NormalizeGraphUnresolvedContext(&x); e != nil {
			return model.GraphDigest{}, e
		}
		if e = json.Unmarshal([]byte(cj), &x.Candidates); e != nil {
			return model.GraphDigest{}, e
		}
		if ve := model.ValidateGraphUnresolved(x); ve != nil {
			return model.GraphDigest{}, ve
		}
		if mode == graphContentValidationLegacy {
			e = h.AddUnresolvedLegacy(x)
		} else {
			e = h.AddUnresolved(x)
		}
		if e != nil {
			return model.GraphDigest{}, e
		}
	}
	if e = rows.Err(); e != nil {
		return model.GraphDigest{}, e
	}
	return h.Sum()
}

func ValidateGraphGeneration(ctx context.Context, db *sql.DB, id model.GraphDigest) (model.GraphGeneration, error) {
	if ctx == nil || db == nil {
		return model.GraphGeneration{}, model.ErrGraphGenerationInvalid
	}
	return validateGraphGenerationConn(ctx, db, id)
}

type GraphReadSnapshot struct {
	tx         *sql.Tx
	generation model.GraphGeneration
	active     bool
	closed     bool
}

// BeginGraphReadSnapshot pins the active graph pointer and its validated
// generation to one read transaction. Callers must Close the snapshot.
func BeginGraphReadSnapshot(ctx context.Context, db *sql.DB) (*GraphReadSnapshot, error) {
	if ctx == nil || db == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	id, ok, err := activeGraphGenerationConn(ctx, tx)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	snapshot := &GraphReadSnapshot{tx: tx, active: ok}
	if ok {
		snapshot.generation, err = validateGraphGenerationConn(ctx, tx, id)
		if err != nil || snapshot.generation.Status != model.GraphGenerationActive {
			_ = tx.Rollback()
			if err == nil {
				err = model.ErrGraphGenerationCorrupt
			}
			return nil, err
		}
	}
	return snapshot, nil
}

// ActiveGraphGeneration returns the validated active generation selected when
// this snapshot was opened. It never consults a second connection.
func (s *GraphReadSnapshot) ActiveGraphGeneration() (model.GraphGeneration, bool, error) {
	if s == nil || s.closed {
		return model.GraphGeneration{}, false, model.ErrGraphGenerationInvalid
	}
	return s.generation, s.active, nil
}

// ValidateGraphGeneration validates a generation using this snapshot's pinned
// read transaction, so all rows come from the same SQLite snapshot.
func (s *GraphReadSnapshot) ValidateGraphGeneration(ctx context.Context, id model.GraphDigest) (model.GraphGeneration, error) {
	if s == nil || s.closed || ctx == nil {
		return model.GraphGeneration{}, model.ErrGraphGenerationInvalid
	}
	return validateGraphGenerationConn(ctx, s.tx, id)
}

func (s *GraphReadSnapshot) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	return s.tx.Rollback()
}

func validateGraphGenerationConn(ctx context.Context, q graphConn, id model.GraphDigest) (model.GraphGeneration, error) {
	return validateGraphGenerationWithMode(ctx, q, id, graphContentValidationCurrent)
}

func validateGraphGenerationWithMode(ctx context.Context, q graphConn, id model.GraphDigest, mode graphContentValidationMode) (model.GraphGeneration, error) {
	g, e := loadGeneration(ctx, q, id)
	if e != nil {
		return g, e
	}
	if g.SourceFingerprint == (model.GraphDigest{}) || g.ConfigFingerprint == (model.GraphDigest{}) || g.BackendManifestDigest == (model.GraphDigest{}) {
		return g, fmt.Errorf("%w: fingerprints", model.ErrGraphGenerationCorrupt)
	}
	d, e := streamGraphWithMode(ctx, q, id, g, mode)
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

const graphGenerationAncestryMaxDepth = 1024

// validateGraphGenerationAncestry checks both the persisted target history and
// the history that activation would create for a staged target. The first
// walk catches corruption already stored on the target; the optional second
// walk substitutes the active predecessor that staged activation will assign.
// Every ancestor is independently validated, but legacy content framing is
// accepted for historical rows just as replacement validation accepts it.
func validateGraphGenerationAncestry(ctx context.Context, q graphConn, id model.GraphDigest, targetWorkspaceIdentity string, replacement *model.GraphDigest, replaceTargetPrevious bool) error {
	if ctx == nil || q == nil {
		return model.ErrGraphGenerationInvalid
	}
	normalized, err := model.NormalizeRepositoryIdentity(targetWorkspaceIdentity)
	if err != nil || normalized != targetWorkspaceIdentity {
		return fmt.Errorf("%w: target workspace identity", model.ErrGraphGenerationCorrupt)
	}
	if err := walkGraphGenerationAncestry(ctx, q, id, targetWorkspaceIdentity, nil, false); err != nil {
		return err
	}
	if !replaceTargetPrevious {
		return nil
	}
	return walkGraphGenerationAncestry(ctx, q, id, targetWorkspaceIdentity, replacement, true)
}

func walkGraphGenerationAncestry(ctx context.Context, q graphConn, id model.GraphDigest, targetWorkspaceIdentity string, replacement *model.GraphDigest, replaceTargetPrevious bool) error {
	seen := make(map[model.GraphDigest]struct{}, graphGenerationAncestryMaxDepth)
	current := id
	for depth := 0; ; depth++ {
		if depth >= graphGenerationAncestryMaxDepth {
			return fmt.Errorf("%w: predecessor chain exceeds %d generations", model.ErrGraphGenerationCorrupt, graphGenerationAncestryMaxDepth)
		}
		if _, ok := seen[current]; ok {
			return fmt.Errorf("%w: predecessor cycle at %s", model.ErrGraphGenerationCorrupt, current)
		}
		seen[current] = struct{}{}
		generation, err := loadGeneration(ctx, q, current)
		if err != nil {
			return fmt.Errorf("%w: predecessor %s: %v", model.ErrGraphGenerationCorrupt, current, err)
		}
		if generation.WorkspaceIdentity != targetWorkspaceIdentity {
			return fmt.Errorf("%w: predecessor %s workspace identity", model.ErrGraphGenerationCorrupt, current)
		}
		if depth > 0 {
			if generation.Status != model.GraphGenerationActive && generation.Status != model.GraphGenerationRetired {
				return fmt.Errorf("%w: predecessor %s has invalid status %q", model.ErrGraphGenerationCorrupt, current, generation.Status)
			}
			if err := validateGraphGenerationAncestryRecord(ctx, q, current, targetWorkspaceIdentity, generation); err != nil {
				return err
			}
		}
		previous := generation.PreviousGenerationID
		if depth == 0 && replaceTargetPrevious {
			previous = replacement
		}
		if previous == nil {
			return nil
		}
		if *previous == (model.GraphDigest{}) {
			return fmt.Errorf("%w: predecessor %s is zero", model.ErrGraphGenerationCorrupt, current)
		}
		current = *previous
	}
}

func validateGraphGenerationAncestryRecord(ctx context.Context, q graphConn, id model.GraphDigest, targetWorkspaceIdentity string, generation model.GraphGeneration) error {
	if generation.WorkspaceIdentity != targetWorkspaceIdentity {
		return fmt.Errorf("%w: predecessor %s workspace identity", model.ErrGraphGenerationCorrupt, id)
	}
	if generation.SchemaVersion != 1 || generation.RepositoryIdentity != generation.WorkspaceIdentity {
		return fmt.Errorf("%w: predecessor %s metadata", model.ErrGraphGenerationCorrupt, id)
	}
	normalized, err := model.NormalizeRepositoryIdentity(generation.WorkspaceIdentity)
	if err != nil || normalized != generation.WorkspaceIdentity {
		return fmt.Errorf("%w: predecessor %s workspace identity", model.ErrGraphGenerationCorrupt, id)
	}
	if _, err := validateGraphGenerationWithMode(ctx, q, id, graphContentValidationCurrent); err == nil {
		return nil
	}
	if _, err := validateGraphGenerationWithMode(ctx, q, id, graphContentValidationLegacy); err != nil {
		return fmt.Errorf("%w: predecessor %s content", model.ErrGraphGenerationCorrupt, id)
	}
	return nil
}

type graphPriorValidation uint8

const (
	graphPriorCurrentValid graphPriorValidation = iota + 1
	graphPriorLegacyValidForReplacement
)

// validateActiveGraphPriorForReplacement is the sole policy for validating an
// active generation before replacement. Legacy acceptance is intentionally
// narrow: the pointer must equal the non-nil expected prior, exactly one
// active row must exist, every persisted row must pass structural validation,
// schema_version must remain 1, and only the exact pre-provenance content
// digest may differ from the current framing. No rows are repaired here.
func validateActiveGraphPriorForReplacement(ctx context.Context, q graphConn, activePointer []byte, expectedPrior *model.GraphDigest, activeRows int) (graphPriorValidation, error) {
	if len(activePointer) != 32 || activeRows != 1 {
		return 0, model.ErrGraphPointerConflict
	}
	oldID, err := scanDigest(activePointer)
	if err != nil {
		return 0, model.ErrGraphPointerConflict
	}
	if expectedPrior != nil && oldID != *expectedPrior {
		return 0, model.ErrGraphPointerConflict
	}
	current, currentErr := validateGraphGenerationConn(ctx, q, oldID)
	currentIdentity, currentIdentityErr := model.NormalizeRepositoryIdentity(current.WorkspaceIdentity)
	if currentErr == nil && current.Status == model.GraphGenerationActive && current.SchemaVersion == 1 && current.RepositoryIdentity == current.WorkspaceIdentity && currentIdentityErr == nil && currentIdentity == current.WorkspaceIdentity {
		return graphPriorCurrentValid, nil
	}
	// A nil expected prior is permitted only for the existing dangling-pointer
	// repair path. Legacy framing is replacement-only and requires an exact CAS
	// against the non-nil active pointer.
	if expectedPrior == nil {
		return 0, model.ErrGraphPointerConflict
	}
	legacy, legacyErr := validateGraphGenerationWithMode(ctx, q, oldID, graphContentValidationLegacy)
	legacyIdentity, legacyIdentityErr := model.NormalizeRepositoryIdentity(legacy.WorkspaceIdentity)
	if legacyErr == nil && legacy.Status == model.GraphGenerationActive && legacy.SchemaVersion == 1 && legacy.RepositoryIdentity == legacy.WorkspaceIdentity && legacyIdentityErr == nil && legacyIdentity == legacy.WorkspaceIdentity {
		return graphPriorLegacyValidForReplacement, nil
	}
	return 0, model.ErrGraphPointerConflict
}

func activeGraphGenerationConn(ctx context.Context, q graphConn) (model.GraphDigest, bool, error) {
	if ctx == nil || q == nil {
		return model.GraphDigest{}, false, model.ErrGraphGenerationInvalid
	}
	var b []byte
	err := q.QueryRowContext(ctx, "SELECT value FROM workspace_meta WHERE key=?", graphActiveMeta).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return model.GraphDigest{}, false, nil
	}
	if err != nil {
		return model.GraphDigest{}, false, err
	}
	if len(b) == 0 {
		return model.GraphDigest{}, false, nil
	}
	d, e := scanDigest(b)
	return d, e == nil, e
}

func SetGraphRuntimeState(ctx context.Context, db *sql.DB, state string, catalogGeneration string) error {
	if ctx == nil || db == nil || (state != GraphRuntimeFresh && state != GraphRuntimeStale) {
		return model.ErrGraphGenerationInvalid
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT INTO workspace_meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", GraphRuntimeStateMeta, state); err != nil {
		return err
	}
	if catalogGeneration == "" {
		if _, err = tx.ExecContext(ctx, "DELETE FROM workspace_meta WHERE key=?", GraphCatalogGenerationMeta); err != nil {
			return err
		}
	} else if _, err = tx.ExecContext(ctx, "INSERT INTO workspace_meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", GraphCatalogGenerationMeta, catalogGeneration); err != nil {
		return err
	}
	return tx.Commit()
}

func GraphRuntimeState(ctx context.Context, q graphConn) (string, error) {
	var state string
	err := q.QueryRowContext(ctx, "SELECT value FROM workspace_meta WHERE key=?", GraphRuntimeStateMeta).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		// Pre-runtime-state databases may contain a valid active graph; preserve compatibility.
		return GraphRuntimeFresh, nil
	}
	return state, err
}

func ActiveGraphGeneration(ctx context.Context, db *sql.DB) (model.GraphDigest, bool, error) {
	if ctx == nil || db == nil {
		return model.GraphDigest{}, false, model.ErrGraphGenerationInvalid
	}
	return activeGraphGenerationConn(ctx, db)
}

func validGraphCompatibilityState(state string) bool {
	return state == GraphCompatibilityLegacyPreservedNoDualWrite || state == GraphCompatibilityDualReadWrite || state == GraphCompatibilityGraphAuthoritative
}

// GetGraphCompatibilityState reads the explicit compatibility mode without
// changing it. In particular, reads never auto-enable dual-read/write.
func GetGraphCompatibilityState(ctx context.Context, db *sql.DB) (string, error) {
	if ctx == nil || db == nil {
		return "", model.ErrGraphGenerationInvalid
	}
	var state string
	if err := db.QueryRowContext(ctx, "SELECT value FROM workspace_meta WHERE key='graph_compatibility_state'").Scan(&state); err != nil {
		return "", err
	}
	if !validGraphCompatibilityState(state) {
		return "", ErrGraphMigrationTransition
	}
	return state, nil
}

// TransitionGraphCompatibilityState performs an explicit CAS under BEGIN
// IMMEDIATE. The only legal transitions are legacy->dual, dual->authoritative,
// and dual->legacy for rollback.
func TransitionGraphCompatibilityState(ctx context.Context, db *sql.DB, expected, next string) error {
	if ctx == nil || db == nil || !validGraphCompatibilityState(expected) || !validGraphCompatibilityState(next) {
		return ErrGraphMigrationTransition
	}
	allowed := map[string]map[string]bool{
		GraphCompatibilityLegacyPreservedNoDualWrite: {GraphCompatibilityDualReadWrite: true},
		GraphCompatibilityDualReadWrite:              {GraphCompatibilityGraphAuthoritative: true, GraphCompatibilityLegacyPreservedNoDualWrite: true},
		GraphCompatibilityGraphAuthoritative:         {},
	}
	if !allowed[expected][next] {
		return ErrGraphMigrationTransition
	}
	t, err := beginGraphImmediate(ctx, db)
	if err != nil {
		return err
	}
	defer t.rollback(ctx)
	var current string
	if err := t.c.QueryRowContext(ctx, "SELECT value FROM workspace_meta WHERE key='graph_compatibility_state'").Scan(&current); err != nil {
		return err
	}
	if current != expected {
		return ErrGraphCompatibilityConflict
	}
	r, err := t.c.ExecContext(ctx, "UPDATE workspace_meta SET value=? WHERE key='graph_compatibility_state' AND value=?", next, expected)
	if err != nil {
		return err
	}
	if n, err := r.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return ErrGraphCompatibilityConflict
	}
	if next == GraphCompatibilityLegacyPreservedNoDualWrite {
		var active []byte
		if err := t.c.QueryRowContext(ctx, "SELECT value FROM workspace_meta WHERE key=?", graphActiveMeta).Scan(&active); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var activeRows int
		if err := t.c.QueryRowContext(ctx, "SELECT COUNT(*) FROM graph_generations WHERE status=?", model.GraphGenerationActive).Scan(&activeRows); err != nil {
			return err
		}
		if (len(active) == 0 && activeRows != 0) || (len(active) != 0 && activeRows != 1) {
			return ErrGraphCompatibilityConflict
		}
		if len(active) != 0 {
			if _, err := scanDigest(active); err != nil {
				return ErrGraphCompatibilityConflict
			}
			updated, err := t.c.ExecContext(ctx, "UPDATE graph_generations SET status=? WHERE status=?", model.GraphGenerationRetired, model.GraphGenerationActive)
			if err != nil {
				return err
			}
			if n, err := updated.RowsAffected(); err != nil || n != 1 {
				return ErrGraphCompatibilityConflict
			}
		}
		if _, err := t.c.ExecContext(ctx, "UPDATE workspace_meta SET value=NULL WHERE key IN (?, ?)", graphActiveMeta, graphPreviousMeta); err != nil {
			return err
		}
	}
	return t.commit(ctx)
}

// SetGraphCompatibilityState is the explicit transition API name used by
// older callers; it retains compare-and-set semantics.
func SetGraphCompatibilityState(ctx context.Context, db *sql.DB, expected, next string) error {
	return TransitionGraphCompatibilityState(ctx, db, expected, next)
}

func ActivateGraphGeneration(ctx context.Context, db *sql.DB, id model.GraphDigest, expectedPrior *model.GraphDigest) error {
	return ActivateGraphGenerationAt(ctx, db, id, expectedPrior, time.Now().UTC())
}

// ActivateGraphGenerationAt is the deterministic publication primitive. The
// supplied timestamp is persisted only after all pointer and generation CAS
// checks pass.
func ActivateGraphGenerationAt(ctx context.Context, db *sql.DB, id model.GraphDigest, expectedPrior *model.GraphDigest, publishedAt time.Time) error {
	if publishedAt.IsZero() {
		return model.ErrGraphGenerationInvalid
	}
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
		if _, err := validateActiveGraphPriorForReplacement(ctx, t.c, old, expectedPrior, activeRows); err != nil {
			return err
		}
		oldID, err := scanDigest(old)
		if err != nil {
			return model.ErrGraphPointerConflict
		}
		if err := validateGraphGenerationAncestry(ctx, t.c, oldID, g.WorkspaceIdentity, nil, false); err != nil {
			return err
		}
	}
	if g.Status == model.GraphGenerationActive {
		if len(old) != len(digestArg(id)) || string(old) != string(digestArg(id)) {
			return model.ErrGraphPointerConflict
		}
		if err := validateGraphGenerationAncestry(ctx, t.c, id, g.WorkspaceIdentity, nil, false); err != nil {
			return err
		}
		return t.commit(ctx)
	}
	if g.Status != model.GraphGenerationStaged && g.Status != model.GraphGenerationRetired {
		return model.ErrGraphGenerationInvalid
	}
	var proposedPrevious *model.GraphDigest
	if len(old) > 0 {
		previous, err := scanDigest(old)
		if err != nil {
			return model.ErrGraphPointerConflict
		}
		proposedPrevious = &previous
	}
	if err := validateGraphGenerationAncestry(ctx, t.c, id, g.WorkspaceIdentity, proposedPrevious, g.Status == model.GraphGenerationStaged); err != nil {
		return err
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
	var r sql.Result
	if g.Status == model.GraphGenerationRetired {
		// A retired canonical snapshot is immutable history. Reactivation may
		// change only publication state; never rewrite its predecessor link,
		// otherwise replaying A after B can create an A<->B history cycle.
		r, e = t.c.ExecContext(ctx, "UPDATE graph_generations SET status=?,published_at=? WHERE generation_id=? AND status=?", model.GraphGenerationActive, publishedAt.UTC().Format(time.RFC3339Nano), digestArg(id), model.GraphGenerationRetired)
	} else {
		// Only a newly staged row receives the currently active generation as
		// its predecessor.
		r, e = t.c.ExecContext(ctx, "UPDATE graph_generations SET status=?,published_at=?,previous_generation_id=? WHERE generation_id=? AND status=?", model.GraphGenerationActive, publishedAt.UTC().Format(time.RFC3339Nano), old, digestArg(id), model.GraphGenerationStaged)
	}
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
	return RollbackGraphGenerationAt(ctx, db, id, expectedCurrent, time.Now().UTC())
}

// RollbackGraphGenerationAt restores a retired generation using an explicit
// publication timestamp and the same immediate-transaction CAS as activation.
func RollbackGraphGenerationAt(ctx context.Context, db *sql.DB, id model.GraphDigest, expectedCurrent *model.GraphDigest, publishedAt time.Time) error {
	if publishedAt.IsZero() {
		return model.ErrGraphGenerationInvalid
	}
	t, e := beginGraphImmediate(ctx, db)
	if e != nil {
		return e
	}
	defer t.rollback(ctx)
	target, e := validateGraphGenerationConn(ctx, t.c, id)
	if e != nil {
		return e
	}
	if target.Status != model.GraphGenerationRetired && target.Status != model.GraphGenerationActive {
		return model.ErrGraphGenerationInvalid
	}
	var active []byte
	metaErr := t.c.QueryRowContext(ctx, "SELECT value FROM workspace_meta WHERE key=?", graphActiveMeta).Scan(&active)
	if metaErr != nil && !errors.Is(metaErr, sql.ErrNoRows) {
		return metaErr
	}
	if (expectedCurrent == nil) || len(active) == 0 || string(digestArg(*expectedCurrent)) != string(active) {
		// A retry after a committed rollback is successful only when both
		// pointers still describe that same completed outcome.
		if target.Status == model.GraphGenerationActive && expectedCurrent != nil && string(active) == string(digestArg(id)) {
			var previous []byte
			if previousErr := t.c.QueryRowContext(ctx, "SELECT value FROM workspace_meta WHERE key=?", graphPreviousMeta).Scan(&previous); previousErr == nil && string(previous) == string(digestArg(*expectedCurrent)) {
				return t.commit(ctx)
			}
		}
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
	var previous []byte
	previousErr := t.c.QueryRowContext(ctx, "SELECT value FROM workspace_meta WHERE key=?", graphPreviousMeta).Scan(&previous)
	if previousErr != nil && !errors.Is(previousErr, sql.ErrNoRows) {
		return previousErr
	}
	if target.Status == model.GraphGenerationActive {
		return model.ErrGraphPointerConflict
	}
	if len(previous) == 0 || string(previous) != string(digestArg(id)) {
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
	r, e = t.c.ExecContext(ctx, "UPDATE graph_generations SET status=?,published_at=?,previous_generation_id=NULL WHERE generation_id=? AND status=?", model.GraphGenerationActive, publishedAt.UTC().Format(time.RFC3339Nano), digestArg(id), model.GraphGenerationRetired)
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

// CleanupGraphGenerations removes only explicitly eligible historical graph
// generations. It refuses to run while recovery evidence (staged generations
// or nonterminal migrations) remains unresolved.
func CleanupGraphGenerations(ctx context.Context, db *sql.DB, workspaceIdentity string, cutoff time.Time) error {
	if ctx == nil || db == nil || strings.TrimSpace(workspaceIdentity) == "" || cutoff.IsZero() {
		return model.ErrGraphGenerationInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	t, err := beginGraphImmediate(ctx, db)
	if err != nil {
		return err
	}
	defer t.rollback(ctx)
	var pending int
	if err = t.c.QueryRowContext(ctx, `SELECT COUNT(*) FROM graph_migrations WHERE status IN ('prepared','applying','validated')`).Scan(&pending); err != nil {
		return err
	}
	var staged int
	if err = t.c.QueryRowContext(ctx, `SELECT COUNT(*) FROM graph_generations WHERE workspace_identity=? AND status='staged'`, workspaceIdentity).Scan(&staged); err != nil {
		return err
	}
	if pending != 0 || staged != 0 {
		return ErrGraphCrashRecoveryRequired
	}
	var active, previous []byte
	if scanErr := t.c.QueryRowContext(ctx, "SELECT value FROM workspace_meta WHERE key=?", graphActiveMeta).Scan(&active); scanErr != nil && !errors.Is(scanErr, sql.ErrNoRows) {
		return scanErr
	}
	if scanErr := t.c.QueryRowContext(ctx, "SELECT value FROM workspace_meta WHERE key=?", graphPreviousMeta).Scan(&previous); scanErr != nil && !errors.Is(scanErr, sql.ErrNoRows) {
		return scanErr
	}
	if len(active) > 0 {
		if _, scanErr := scanDigest(active); scanErr != nil {
			return fmt.Errorf("%w: active pointer: %v", model.ErrGraphGenerationCorrupt, scanErr)
		}
	}
	if len(previous) > 0 {
		if _, scanErr := scanDigest(previous); scanErr != nil {
			return fmt.Errorf("%w: previous pointer: %v", model.ErrGraphGenerationCorrupt, scanErr)
		}
	}
	protected := func(id []byte) bool {
		return len(id) > 0 && (string(id) == string(active) || string(id) == string(previous))
	}
	rows, err := t.c.QueryContext(ctx, `SELECT generation_id FROM graph_generations WHERE workspace_identity=? AND status IN ('invalid','retired') AND created_at < ?`, workspaceIdentity, cutoff.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	var candidates [][]byte
	for rows.Next() {
		var id []byte
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, append([]byte(nil), id...))
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, id := range candidates {
		if protected(id) {
			continue
		}
		var refs int
		if err = t.c.QueryRowContext(ctx, `SELECT COUNT(*) FROM graph_generations WHERE previous_generation_id=?`, id).Scan(&refs); err != nil {
			return err
		}
		if refs != 0 {
			continue
		}
		if err = t.c.QueryRowContext(ctx, `SELECT COUNT(*) FROM graph_migrations WHERE prior_active_generation_id=?`, id).Scan(&refs); err != nil {
			return err
		}
		if refs != 0 {
			continue
		}
		result, execErr := t.c.ExecContext(ctx, "DELETE FROM graph_generations WHERE generation_id=? AND workspace_identity=? AND status IN ('invalid','retired') AND created_at < ?", id, workspaceIdentity, cutoff.UTC().Format(time.RFC3339Nano))
		if execErr != nil {
			return execErr
		}
		if _, execErr = result.RowsAffected(); execErr != nil {
			return execErr
		}
	}
	return t.commit(ctx)
}

// PrepareGraphMigration records an exact, idempotent migration intent.
func PrepareGraphMigration(ctx context.Context, db *sql.DB, m model.GraphMigration) error {
	if ctx == nil || db == nil {
		return ErrGraphMigrationTransition
	}
	canonicalBootstrap := m.FromVersion == 0 && m.ToVersion == 1
	if m.MigrationID == "" || m.FromVersion < 0 || m.ToVersion <= m.FromVersion || (m.FromVersion == 0 && !canonicalBootstrap) || m.Status != "prepared" ||
		m.PreflightDigest == (model.GraphDigest{}) || m.BackupDigest == (model.GraphDigest{}) || m.StartedAt.IsZero() {
		return ErrGraphMigrationTransition
	}
	if m.PriorActiveGenerationID != nil && *m.PriorActiveGenerationID == (model.GraphDigest{}) {
		return ErrGraphMigrationTransition
	}
	if err := graphSchemaPreflight(db); err != nil {
		return err
	}
	t, e := beginGraphImmediate(ctx, db)
	if e != nil {
		return e
	}
	defer t.rollback(ctx)
	var graphTableCount int
	if e = t.c.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name LIKE 'graph_%'").Scan(&graphTableCount); e != nil {
		return e
	}
	if graphTableCount == 0 {
		if !canonicalBootstrap {
			return ErrGraphMigrationTransition
		}
		if e = ensureGraphSchemaTx(ctx, t.c); e != nil {
			return e
		}
	}
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
	if canonicalBootstrap && graphTableCount != 0 {
		return ErrGraphMigrationTransition
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
	if ctx == nil || db == nil || id == "" || now.IsZero() || !allowed[expected][next] || (next == "failed" && errorCode == "") {
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
