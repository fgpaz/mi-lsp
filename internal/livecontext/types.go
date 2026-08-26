package livecontext

import (
	"context"
	"database/sql"
	"errors"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// ArtifactStateReader is the read-only T4 artifact-state contract consumed by
// reconciliation. Implementations must not create a database or mutate it.
type ArtifactStateReader interface {
	ListDocArtifactStates(context.Context) ([]model.DocArtifactState, error)
}

// DocArtifactStateReader is the name used by the T4 skeleton and remains an
// alias so callers can use either spelling without changing the contract.
type DocArtifactStateReader = ArtifactStateReader
type Reader = ArtifactStateReader

// SQLArtifactStateReader adapts an already-open read-only SQLite handle to the
// reconciliation contract. It intentionally has no write methods.
type SQLArtifactStateReader struct {
	DB *sql.DB
}

func NewSQLArtifactStateReader(db *sql.DB) SQLArtifactStateReader {
	return SQLArtifactStateReader{DB: db}
}

func (r SQLArtifactStateReader) ListDocArtifactStates(ctx context.Context) ([]model.DocArtifactState, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if r.DB == nil {
		return nil, errors.New("artifact state reader has no database")
	}
	rows, err := r.DB.QueryContext(ctx, `
		SELECT path, size, mtime_nsec, content_sha256, parser_version,
		       authority_config_hash, indexed_at, last_docs_gen_at, lifecycle
		FROM doc_artifact_states ORDER BY path ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := make([]model.DocArtifactState, 0)
	for rows.Next() {
		var state model.DocArtifactState
		if err := rows.Scan(&state.Path, &state.Size, &state.MtimeNsec, &state.ContentSHA256,
			&state.ParserVersion, &state.AuthorityConfigHash, &state.IndexedAt,
			&state.LastDocsGenAt, &state.Lifecycle); err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

// CanonicalManifestEntry is the metadata-only result of canonical discovery.
// Absolute paths are retained privately by the implementation and never
// serialized into the overlay.
type CanonicalManifestEntry struct {
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	MtimeNsec  int64  `json:"-"`
}

// IncrementalDocChange is the in-memory classification used by T5. Content is
// present only for a new/changed/racy document that was read safely. The
// absolute filesystem path is intentionally not part of this public type.
type IncrementalDocChange struct {
	Path           string
	Classification string
	Action         string
	Proof          string
	Content        []byte
	ContentHash    string
	ReadStatus     ReadStatus
	State          model.DocArtifactState
}

const (
	DocChangeNew            = "new"
	DocChangeChanged        = "changed"
	DocChangeUnchanged      = "unchanged"
	DocChangeDeleted        = "deleted"
	DocChangeExcludedAlive  = "excluded_alive"
	DocChangeConcurrent     = "concurrent_change"
	DocChangeReadFailed     = "read_failed"
	DocChangeScopeLimited   = "scope_limited"
)

// ReadStatus is the result of one stat-before/read/stat-after sequence.
type ReadStatus string

const (
	ReadStatusStable          ReadStatus = "stable"
	ReadStatusRacy            ReadStatus = "racy"
	ReadStatusConcurrent      ReadStatus = "concurrent_change"
	ReadStatusConcurrentChange           = ReadStatusConcurrent
)

// WikiCodeScope is re-exported from the model package for convenient package
// local construction while preserving one wire/model definition.
type WikiCodeScope = model.WikiCodeScope
type WikiCodeScopeKind = model.WikiCodeScopeKind
type WikiCodeBlockScope = model.WikiCodeBlockScope

const (
	ScopeExactWiki   = model.ScopeExactWiki
	ScopeReverseCode = model.ScopeReverseCode
)

// OverlayRequest bounds one request-scoped reconciliation. DB and DBPath are
// optional for source-only exact scopes; when a database is available it is
// opened/used read-only and the published generation is pinned before rows are
// read.
type OverlayRequest struct {
	WorkspaceRoot string
	// Root is a compatibility alias for WorkspaceRoot. WorkspaceRoot wins.
	Root string
	DB   *sql.DB
	DBPath string
	Scope model.WikiCodeScope

	// CanonicalRoots, when set, are additional canonical directory roots. A
	// relative root is relative to WorkspaceRoot; absolute roots are accepted
	// only when they are already declared/safe canonical roots.
	CanonicalRoots []string
	Profile        *model.DocsReadProfile
	Matcher        *workspace.IgnoreMatcher
	StateReader    ArtifactStateReader

	// MaxDocuments and MaxBytes are request-local cost bounds. Zero selects
	// conservative defaults; negative values are rejected.
	MaxDocuments int
	MaxBytes     int64

	// BaseGeneration is a test/embedding seam. Empty means pin the published
	// generation with store.ReadWorkspaceGenerationSnapshot.
	BaseGeneration string
}

// WikiCodeOverlayRequest is an additive alias used by callers that name the
// operation rather than the underlying object.
type WikiCodeOverlayRequest = OverlayRequest

const (
	DefaultExactMaxDocuments   = 16
	DefaultReverseMaxDocuments = 256
	DefaultOverlayMaxBytes     = 4 << 20
	MaxExactDocuments          = DefaultExactMaxDocuments
	MaxReverseDocuments        = DefaultReverseMaxDocuments
	MaxOverlayBytes            = DefaultOverlayMaxBytes
)

var (
	ErrInvalidScope       = errors.New("invalid overlay scope")
	ErrUnsafeTarget       = errors.New("unsafe target")
	ErrCanonicalDiscovery = errors.New("canonical discovery failed")
	ErrOverlayBound       = errors.New("overlay cost bound exceeded")
)

// BuildOverlay creates one ephemeral request view. It is the canonical entry
// point; BuildWikiCodeOverlay is an equivalent descriptive alias.
func BuildOverlay(ctx context.Context, req OverlayRequest) (model.WikiCodeOverlay, error) {
	return buildOverlay(ctx, req)
}

func BuildWikiCodeOverlay(ctx context.Context, req OverlayRequest) (model.WikiCodeOverlay, error) {
	return buildOverlay(ctx, req)
}

func BuildEphemeralOverlay(ctx context.Context, req OverlayRequest) (model.WikiCodeOverlay, error) {
	return buildOverlay(ctx, req)
}
