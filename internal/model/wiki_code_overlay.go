package model

// WikiCodeOverlayVersion identifies the in-memory overlay contract. It is
// intentionally independent from the persisted schema version: overlays are
// request-scoped and are never written back to SQLite.
const WikiCodeOverlayVersion = "wiki-code-overlay-v1"

type WikiCodeFreshnessDomain string
type WikiCodeFreshnessStatus string
type WikiCodeOverlayStatus string
type WikiCodeOverlayMode string

const (
	OverlayModeRAMOnly = "ram_only"
	OverlayModeRequest = "request_scoped"

	OverlayStatusCurrent           = "current"
	OverlayStatusOverlay           = "overlay"
	OverlayStatusStale             = "stale"
	OverlayStatusUnknown           = "unknown"
	OverlayStatusConcurrentChange  = "concurrent_change"
	WikiCodeOverlayStatusCurrent          = OverlayStatusCurrent
	WikiCodeOverlayStatusOverlay          = OverlayStatusOverlay
	WikiCodeOverlayStatusStale            = OverlayStatusStale
	WikiCodeOverlayStatusUnknown          = OverlayStatusUnknown
	WikiCodeOverlayStatusConcurrentChange = OverlayStatusConcurrentChange
	WikiCodeOverlayModeRAMOnly            = OverlayModeRAMOnly

	FreshnessDomainDocsManifest = "docs_manifest"
	FreshnessDomainBindings     = "bindings"
	FreshnessDomainCatalog      = "catalog"
	FreshnessDomainGraph        = "graph"
	FreshnessDomainAuthority    = "authority"
	FreshnessDocsManifest       = FreshnessDomainDocsManifest
	FreshnessBindings           = FreshnessDomainBindings
	FreshnessCatalog            = FreshnessDomainCatalog
	FreshnessGraph              = FreshnessDomainGraph
	FreshnessAuthority          = FreshnessDomainAuthority

	FreshnessCurrent          = "current"
	FreshnessOverlay          = "overlay"
	FreshnessStale            = "stale"
	FreshnessUnknown          = "unknown"
	FreshnessConcurrentChange = "concurrent_change"

	FreshnessStatusCurrent          WikiCodeFreshnessStatus = FreshnessCurrent
	FreshnessStatusOverlay          WikiCodeFreshnessStatus = FreshnessOverlay
	FreshnessStatusStale            WikiCodeFreshnessStatus = FreshnessStale
	FreshnessStatusUnknown          WikiCodeFreshnessStatus = FreshnessUnknown
	FreshnessStatusConcurrentChange WikiCodeFreshnessStatus = FreshnessConcurrentChange
)

// Freshness is a short alias for the wire-compatible freshness projection.
type Freshness = WikiCodeFreshness

// WikiCodeFreshness carries independent claim status for every domain touched
// by a live wiki↔code request. A domain is never inferred from another domain;
// in particular, an overlay does not make catalog or graph claims current.
type WikiCodeFreshness struct {
	DocsManifest string            `json:"docs_manifest"`
	Bindings     string            `json:"bindings"`
	Catalog      string            `json:"catalog"`
	Graph        string            `json:"graph"`
	Authority    string            `json:"authority"`
	Domains      map[string]string `json:"domains,omitempty"`
}

// Status returns the status for one freshness domain. Unknown is returned for
// an unrecognized or unset domain so callers cannot accidentally claim current.
func (f WikiCodeFreshness) Status(domain string) string {
	var status string
	switch domain {
	case FreshnessDomainDocsManifest:
		status = f.DocsManifest
	case FreshnessDomainBindings:
		status = f.Bindings
	case FreshnessDomainCatalog:
		status = f.Catalog
	case FreshnessDomainGraph:
		status = f.Graph
	case FreshnessDomainAuthority:
		status = f.Authority
	}
	if status == "" && f.Domains != nil {
		if value, ok := f.Domains[domain]; ok {
			status = value
		}
	}
	if status != "" {
		return status
	}
	return FreshnessUnknown
}

// WikiCodeOverlayCost contains diagnostic counters for one overlay request.
// Counters are deliberately excluded from the deterministic overlay digest.
type WikiCodeOverlayCost struct {
	MetadataChecked int   `json:"metadata_checked,omitempty"`
	FilesHashed     int   `json:"files_hashed,omitempty"`
	FilesParsed     int   `json:"files_parsed,omitempty"`
	UnchangedReused int   `json:"unchanged_reused,omitempty"`
	BytesRead       int64 `json:"bytes_read,omitempty"`
}

// WikiCodeOverlayCosts is a compatibility alias for callers that prefer a
// plural field/type name while the canonical type remains singular.
type WikiCodeOverlayCosts = WikiCodeOverlayCost
type OverlayCost = WikiCodeOverlayCost

// WikiCodeChangedInput records the public, path-relative outcome for one
// manifest input. Absolute filesystem paths and timestamps are intentionally
// absent from this type.
type WikiCodeChangedInput struct {
	Path           string `json:"path"`
	Classification string `json:"classification"`
	Status         string `json:"status,omitempty"`
	ContentHash    string `json:"content_hash,omitempty"`
}

// WikiCodeOverlayInput is an additive alias used by integrations that call
// changed manifest entries inputs.
type WikiCodeOverlayInput = WikiCodeChangedInput

// BindingTombstone masks a persisted binding in the request view. A tombstone
// may identify one binding_ref, one doc/block owner, or both. Empty identity
// fields are not wildcarded except where the owner path is explicitly set.
type BindingTombstone struct {
	BindingRef string `json:"binding_ref,omitempty"`
	DocPath    string `json:"doc_path,omitempty"`
	BlockID    string `json:"block_id,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Proof      string `json:"proof,omitempty"`
}

// WikiCodeOmission is a typed fail-closed result. Path-bearing values must be
// repository-relative; callers must not copy filesystem diagnostics here.
type WikiCodeOmission struct {
	Code       string   `json:"code"`
	Path       string   `json:"path,omitempty"`
	DocPath    string   `json:"doc_path,omitempty"`
	TargetPath string   `json:"target_path,omitempty"`
	Reason     string   `json:"reason,omitempty"`
	Candidates []string `json:"candidates,omitempty"`
}

// WikiCodeOverlayOmission names the same typed omission for overlay-specific
// APIs without duplicating the model contract.
type WikiCodeOverlayOmission = WikiCodeOmission

const (
	OmissionUnsafeTarget       = "unsafe_target"
	OmissionConcurrentChange  = "concurrent_change"
	OmissionReadFailed         = "read_failed"
	OmissionExcludedAlive      = "excluded_alive"
	OmissionUnknownDocument    = "unknown_document"
	OmissionDatabaseUnavailable = "database_unavailable"
	OmissionAuthorityUnavailable = "authority_unavailable"
	OmissionParseFailed        = "parse_failed"
	OmissionInvalidScope       = "invalid_scope"
)

// WikiCodeScopeKind identifies the bounded reconciliation direction.
type WikiCodeScopeKind string

const (
	ScopeExactWiki   WikiCodeScopeKind = "exact_wiki"
	ScopeReverseCode WikiCodeScopeKind = "reverse_code"

	// Longer names make call sites self-documenting without changing the wire
	// value used in the deterministic digest.
	WikiCodeScopeExactWiki   = ScopeExactWiki
	WikiCodeScopeReverseCode = ScopeReverseCode
	ScopeKindExactWiki       = ScopeExactWiki
	ScopeKindReverseCode     = ScopeReverseCode
	WikiCodeScopeKindExact   = ScopeExactWiki
	WikiCodeScopeKindReverse = ScopeReverseCode
)

// WikiCodeBlockScope selects one source block within a canonical document.
type WikiCodeBlockScope struct {
	DocPath string `json:"doc_path,omitempty"`
	DocID   string `json:"doc_id,omitempty"`
	BlockID string `json:"block_id"`
}

// WikiCodeScope is the complete normalized scope included in an overlay
// digest. Exact wiki scopes may select docs by path/id/block; reverse scopes
// select a repo-relative target path and optional symbol.
type WikiCodeScope struct {
	Kind         WikiCodeScopeKind   `json:"kind"`
	DocIDs       []string             `json:"doc_ids,omitempty"`
	DocPaths     []string             `json:"doc_paths,omitempty"`
	Blocks       []WikiCodeBlockScope `json:"blocks,omitempty"`
	TargetPath   string               `json:"target_path,omitempty"`
	TargetSymbol string               `json:"target_symbol,omitempty"`
}

// WikiCodeOverlay is an ephemeral request view over a pinned published
// generation. Additions and tombstones live only in RAM; no field represents a
// write intent or a durable publication operation.
type WikiCodeOverlay struct {
	BaseGeneration string                `json:"base_generation"`
	Mode           string                `json:"mode"`
	Status         string                `json:"status"`
	Scope          WikiCodeScope         `json:"scope"`
	Freshness      WikiCodeFreshness     `json:"freshness"`
	ChangedInputs  []WikiCodeChangedInput `json:"changed_inputs,omitempty"`
	Additions      []DocArtifactBinding  `json:"additions,omitempty"`
	Tombstones     []BindingTombstone    `json:"tombstones,omitempty"`
	Omissions      []WikiCodeOmission    `json:"omissions,omitempty"`
	Digest         string                `json:"digest"`
	Cost           WikiCodeOverlayCost   `json:"cost,omitempty"`
}

// WikiCodeOverlayCostCounters is retained as a readable alias for adapters
// that expose diagnostics under that name.
type WikiCodeOverlayCostCounters = WikiCodeOverlayCost
