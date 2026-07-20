package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	NodeKeyVersion          = byte(1)
	GraphGenerationStaged   = "staged"
	GraphGenerationActive   = "active"
	GraphGenerationRetired  = "retired"
	GraphGenerationInvalid  = "invalid"
	GraphRecordExact        = "exact"
	GraphRecordExtracted    = "extracted"
	GraphRecordInferred     = "inferred"
	maxUnresolvedCandidates = 64
	maxCandidateBytes       = 4096
)

var (
	ErrNodeKeyInvalid         = errors.New("invalid NodeKey")
	ErrNodeKeyCollision       = errors.New("NodeKey canonical collision")
	ErrGraphGenerationInvalid = errors.New("invalid graph generation")
	ErrGraphGenerationCorrupt = errors.New("corrupt graph generation")
	ErrGraphPointerConflict   = errors.New("graph active pointer conflict")
	ErrGraphEdgeInvalid       = errors.New("invalid graph edge")
	ErrGraphEvidenceInvalid   = errors.New("invalid graph evidence")
	ErrGraphUnresolved        = errors.New("invalid graph unresolved")
	ErrGraphRepositoryInvalid = errors.New("invalid repository identity")
	ErrGraphPathInvalid       = errors.New("invalid graph path")
)

type GraphError struct {
	Code, Field, Message string
	Cause                error
}

func (e *GraphError) Error() string {
	return fmt.Sprintf("graph %s %s: %s", e.Code, e.Field, e.Message)
}
func (e *GraphError) Unwrap() error {
	if e.Cause != nil {
		return e.Cause
	}
	switch {
	case strings.HasPrefix(e.Code, "GPH_IDENTITY_REPOSITORY"):
		return ErrGraphRepositoryInvalid
	case strings.HasPrefix(e.Code, "GPH_IDENTITY_PATH"):
		return ErrGraphPathInvalid
	case strings.HasPrefix(e.Code, "GPH_EDGE_"):
		return ErrGraphEdgeInvalid
	case strings.HasPrefix(e.Code, "GPH_EVIDENCE_"):
		return ErrGraphEvidenceInvalid
	case strings.HasPrefix(e.Code, "GPH_UNRESOLVED_"):
		return ErrGraphUnresolved
	case strings.HasPrefix(e.Code, "GPH_GENERATION_"):
		return ErrGraphGenerationInvalid
	default:
		return ErrNodeKeyInvalid
	}
}
func graphErr(code, field, message string) error {
	return &GraphError{Code: code, Field: field, Message: message}
}

type NodeKeyFields struct {
	RepositoryIdentity string `json:"repository_identity"`
	BackendType        string `json:"backend_type"`
	Language           string `json:"language"`
	ProjectOrModule    string `json:"project_or_module"`
	OwnerPath          string `json:"owner_path"`
	SymbolKind         string `json:"symbol_kind"`
	SemanticIdentity   string `json:"semantic_identity"`
}
type NodeKey = NodeKeyFields
type GraphDigest [32]byte

func ParseGraphDigest(s string) (GraphDigest, error) {
	var d GraphDigest
	if len(s) != 64 || strings.ToLower(s) != s {
		return d, errors.New("digest must be lowercase 64-character hex")
	}
	b, e := hex.DecodeString(s)
	if e != nil || len(b) != 32 {
		return d, errors.New("invalid digest hex")
	}
	copy(d[:], b)
	return d, nil
}
func (d GraphDigest) String() string               { return hex.EncodeToString(d[:]) }
func (d GraphDigest) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }
func (d *GraphDigest) UnmarshalJSON(b []byte) error {
	var s string
	if e := json.Unmarshal(b, &s); e != nil {
		return e
	}
	x, e := ParseGraphDigest(s)
	if e != nil {
		return e
	}
	*d = x
	return nil
}
func digestBytes(b []byte) GraphDigest { return GraphDigest(sha256.Sum256(b)) }

func normalizeText(v string) (string, error) {
	if !utf8.ValidString(v) {
		return "", graphErr("GPH_IDENTITY_FIELD_MISSING", "", "invalid UTF-8")
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return "", graphErr("GPH_IDENTITY_FIELD_MISSING", "", "control character is forbidden")
		}
	}
	return norm.NFC.String(strings.TrimSpace(v)), nil
}
func required(v, field string) (string, error) {
	v, e := normalizeText(v)
	if e != nil {
		return "", e
	}
	if v == "" {
		return "", graphErr("GPH_IDENTITY_FIELD_MISSING", field, "field is required")
	}
	return v, nil
}
func enum(v, f string) (string, error) {
	v, e := required(v, f)
	if e != nil {
		return "", e
	}
	v = strings.ToLower(v)
	if v[0] < 'a' || v[0] > 'z' {
		return "", graphErr("GPH_IDENTITY_ENUM_INVALID", f, "identifier must start with lowercase ASCII letter")
	}
	for _, c := range []byte(v) {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '.' && c != '_' && c != '+' && c != '-' {
			return "", graphErr("GPH_IDENTITY_ENUM_INVALID", f, "identifier must use lowercase ASCII registered-name characters")
		}
	}
	return v, nil
}
func slashPath(v, field string) (string, error) {
	v, e := required(v, field)
	if e != nil {
		return "", e
	}
	if strings.Contains(v, "\\") || strings.HasPrefix(v, "/") || strings.HasPrefix(v, "//") || (len(v) > 1 && v[1] == ':') {
		return "", graphErr("GPH_IDENTITY_PATH_INVALID", field, "path must be relative slash form")
	}
	parts := strings.Split(v, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" || p == ".." {
			return "", graphErr("GPH_IDENTITY_PATH_INVALID", field, "invalid path segment")
		}
		if p != "." {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return "", graphErr("GPH_IDENTITY_PATH_INVALID", field, "empty path")
	}
	return strings.Join(out, "/"), nil
}

func NormalizeRepositoryIdentity(v string) (string, error) {
	v, e := normalizeText(v)
	if e != nil {
		return "", e
	}
	if v == "" {
		return "", graphErr("GPH_IDENTITY_REPOSITORY_MISSING", "repository_identity", "repository identity is required")
	}
	if strings.ContainsRune(v, 0) || strings.HasPrefix(v, "/") || strings.HasPrefix(v, "~") || (len(v) > 1 && v[1] == ':') || strings.ContainsAny(v, "\r\n\t") {
		return "", graphErr("GPH_IDENTITY_REPOSITORY_MISSING", "repository_identity", "local/control identity forbidden")
	}
	if !strings.Contains(v, "://") && !strings.HasPrefix(strings.ToLower(v), "git@") && !strings.Contains(v, ":") && strings.Contains(v, "/") {
		v = "https://" + v
	}
	if strings.HasPrefix(strings.ToLower(v), "git@") || (!strings.Contains(v, "://") && strings.Contains(v, ":")) {
		parts := strings.SplitN(v, ":", 2)
		if len(parts) != 2 || parts[0] == "" {
			return "", graphErr("GPH_IDENTITY_REPOSITORY_MISSING", "repository_identity", "invalid SSH identity")
		}
		v = "ssh://" + parts[0] + "/" + parts[1]
	}
	u, e := url.Parse(v)
	if e != nil || u.Scheme == "" || u.Host == "" {
		return "", graphErr("GPH_IDENTITY_REPOSITORY_MISSING", "repository_identity", "repository must be HTTPS or SSH origin")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", graphErr("GPH_IDENTITY_REPOSITORY_MISSING", "repository_identity", "query/fragment forbidden")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && scheme != "ssh" {
		return "", graphErr("GPH_IDENTITY_REPOSITORY_MISSING", "repository_identity", "unsupported repository scheme")
	}
	if u.User != nil && scheme == "https" {
		return "", graphErr("GPH_IDENTITY_REPOSITORY_MISSING", "repository_identity", "HTTPS credentials forbidden")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || strings.Contains(host, "..") || strings.ContainsAny(host, "/\\") {
		return "", graphErr("GPH_IDENTITY_REPOSITORY_MISSING", "repository_identity", "invalid host")
	}
	path := strings.TrimSuffix(u.EscapedPath(), "/")
	path = strings.TrimSuffix(path, ".git")
	if path == "" || path == "/" || strings.Contains(path, "..") {
		return "", graphErr("GPH_IDENTITY_REPOSITORY_MISSING", "repository_identity", "invalid or folder-only repository path")
	}
	for _, p := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if p == "" || p == "." || p == ".." {
			return "", graphErr("GPH_IDENTITY_REPOSITORY_MISSING", "repository_identity", "invalid repository path")
		}
	}
	return host + path, nil
}
func NewNodeKey(f NodeKeyFields) (NodeKey, error) {
	var e error
	if f.RepositoryIdentity, e = NormalizeRepositoryIdentity(f.RepositoryIdentity); e != nil {
		return NodeKey{}, e
	}
	if f.BackendType, e = registered(f.BackendType, "backend_type"); e != nil {
		return NodeKey{}, e
	}
	if f.Language, e = enum(f.Language, "language"); e != nil {
		return NodeKey{}, e
	}
	if f.ProjectOrModule, e = slashPath(f.ProjectOrModule, "project_or_module"); e != nil {
		return NodeKey{}, e
	}
	if f.OwnerPath, e = slashPath(f.OwnerPath, "owner_path"); e != nil {
		return NodeKey{}, e
	}
	if f.SymbolKind, e = registered(f.SymbolKind, "symbol_kind"); e != nil {
		return NodeKey{}, e
	}
	if f.SemanticIdentity, e = required(f.SemanticIdentity, "semantic_identity"); e != nil {
		return NodeKey{}, e
	}
	return f, nil
}
func (k NodeKey) CanonicalTuple() []string {
	return []string{k.RepositoryIdentity, k.BackendType, k.Language, k.ProjectOrModule, k.OwnerPath, k.SymbolKind, k.SemanticIdentity}
}
func (k NodeKey) Validate() error { _, e := NewNodeKey(k); return e }

type binaryWriter struct{ bytes.Buffer }

func (w *binaryWriter) field(tag byte, b []byte) {
	w.WriteByte(tag)
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(b)))
	w.Write(n[:])
	w.Write(b)
}
func (w *binaryWriter) text(tag byte, s string)        { w.field(tag, []byte(s)) }
func (w *binaryWriter) digest(tag byte, d GraphDigest) { w.field(tag, d[:]) }
func (w *binaryWriter) integer(tag byte, n int) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(n))
	w.field(tag, b[:])
}
func (k NodeKey) Serialize() ([]byte, error) {
	n, e := NewNodeKey(k)
	if e != nil {
		return nil, e
	}
	var w binaryWriter
	w.WriteString("MILSP-NK")
	w.WriteByte(1)
	var c [2]byte
	binary.BigEndian.PutUint16(c[:], 7)
	w.Write(c[:])
	for i, s := range n.CanonicalTuple() {
		w.text(byte(i+1), s)
	}
	return w.Bytes(), nil
}
func EncodeNodeKey(k NodeKey) ([]byte, error) { return k.Serialize() }
func (k NodeKey) Hash() (GraphDigest, error) {
	b, e := k.Serialize()
	if e != nil {
		return GraphDigest{}, e
	}
	return digestBytes(b), nil
}
func HashNodeKey(k NodeKey) (GraphDigest, error) { return k.Hash() }
func (k NodeKey) HashHex() (string, error)       { d, e := k.Hash(); return d.String(), e }
func NodeRID(d GraphDigest) string               { return "milsp:gph-node:v1:" + d.String() }
func EdgeRID(d GraphDigest) string               { return "milsp:gph-edge:v1:" + d.String() }
func EvidenceRID(d GraphDigest) string           { return "milsp:gph-evidence:v1:" + d.String() }
func UnresolvedRID(d GraphDigest) string         { return "milsp:gph-unresolved:v1:" + d.String() }
func EdgeKey(from, to GraphDigest, relation, scope string) GraphDigest {
	var w binaryWriter
	w.WriteString("MILSP-EK/v1")
	w.digest(1, from)
	w.text(2, relation)
	w.digest(3, to)
	w.text(4, scope)
	return digestBytes(w.Bytes())
}
func EvidenceKey(subject, evidence GraphDigest, ordinal int) GraphDigest {
	var w binaryWriter
	w.WriteString("MILSP-EVIDENCE/v1")
	w.digest(1, subject)
	w.digest(2, evidence)
	w.integer(3, ordinal)
	return digestBytes(w.Bytes())
}
func EvidenceDigest(source GraphDigest, observed GraphDigest, sourceURI, claimKind, backend, extractor string, startLine, startColumn, endLine, endColumn int) GraphDigest {
	var w binaryWriter
	w.WriteString("MILSP-EVIDENCE-DIGEST/v1")
	w.digest(1, source)
	w.digest(2, observed)
	w.text(3, sourceURI)
	w.text(4, claimKind)
	w.text(5, backend)
	w.text(6, extractor)
	w.integer(7, startLine)
	w.integer(8, startColumn)
	w.integer(9, endLine)
	w.integer(10, endColumn)
	return digestBytes(w.Bytes())
}

type GraphGeneration struct {
	GenerationID          GraphDigest  `json:"generation_id"`
	SchemaVersion         int          `json:"schema_version"`
	WorkspaceIdentity     string       `json:"workspace_identity"`
	RepositoryIdentity    string       `json:"repository_identity"`
	SourceFingerprint     GraphDigest  `json:"source_fingerprint"`
	ConfigFingerprint     GraphDigest  `json:"config_fingerprint"`
	BackendManifestDigest GraphDigest  `json:"backend_manifest_digest"`
	ContentDigest         GraphDigest  `json:"content_digest"`
	Status                string       `json:"status"`
	ErrorCode             string       `json:"error_code,omitempty"`
	NodeCount             int          `json:"node_count"`
	EdgeCount             int          `json:"edge_count"`
	EvidenceCount         int          `json:"evidence_count"`
	UnresolvedCount       int          `json:"unresolved_count"`
	PreviousGenerationID  *GraphDigest `json:"previous_generation_id,omitempty"`
	CreatedAt             time.Time    `json:"created_at,omitempty"`
	PublishedAt           time.Time    `json:"published_at,omitempty"`
}
type GraphNodeRecord struct {
	GenerationID   GraphDigest   `json:"generation_id"`
	NodeID         int           `json:"node_id"`
	NodeKey        GraphDigest   `json:"node_key"`
	Identity       NodeKeyFields `json:"identity_fields"`
	IdentitySchema string        `json:"identity_schema"`
	DisplayName    string        `json:"display_name"`
	SourceDigest   GraphDigest   `json:"source_digest"`
	ClaimStatus    string        `json:"claim_status"`
	CrossRID       string        `json:"cross_rid"`
	SortKey        string        `json:"sort_key"`
}
type GraphEdgeRecord struct {
	GenerationID  GraphDigest `json:"generation_id"`
	EdgeID        int         `json:"edge_id"`
	EdgeKey       GraphDigest `json:"edge_key"`
	FromNodeID    int         `json:"from_node_id"`
	ToNodeID      int         `json:"to_node_id"`
	Relation      string      `json:"relation"`
	ClaimScope    string      `json:"claim_scope"`
	ClaimStatus   string      `json:"claim_status"`
	OwnerPath     string      `json:"owner_path"`
	SourceBackend string      `json:"source_backend"`
	CrossRID      string      `json:"cross_rid"`
}
type GraphEvidence struct {
	GenerationID        GraphDigest `json:"generation_id"`
	EvidenceID          int         `json:"evidence_id"`
	EvidenceKey         GraphDigest `json:"evidence_key"`
	EvidenceDigest      GraphDigest `json:"evidence_digest"`
	SubjectKind         string      `json:"subject_kind"`
	NodeID              *int        `json:"node_id,omitempty"`
	EdgeID              *int        `json:"edge_id,omitempty"`
	SourceURI           string      `json:"source_uri"`
	StartLine           *int        `json:"start_line,omitempty"`
	StartColumn         *int        `json:"start_column,omitempty"`
	EndLine             *int        `json:"end_line,omitempty"`
	EndColumn           *int        `json:"end_column,omitempty"`
	Backend             string      `json:"backend"`
	ExtractorVersion    string      `json:"extractor_version"`
	SourceDigest        GraphDigest `json:"source_digest"`
	ClaimKind           string      `json:"claim_kind"`
	ObservedClaimDigest GraphDigest `json:"observed_claim_digest"`
	ClaimStatus         string      `json:"claim_status"`
	CrossRID            string      `json:"cross_rid"`
}
type GraphUnresolved struct {
	GenerationID     GraphDigest  `json:"generation_id"`
	UnresolvedID     int          `json:"unresolved_id"`
	UnresolvedKey    GraphDigest  `json:"unresolved_key"`
	OwnerPath        string       `json:"owner_path"`
	SubjectKind      string       `json:"subject_kind"`
	SelectorDigest   GraphDigest  `json:"selector_digest"`
	ReasonCode       string       `json:"reason_code"`
	Candidates       []string     `json:"candidates_json"`
	Backend          string       `json:"backend"`
	SourceDigest     *GraphDigest `json:"source_digest,omitempty"`
	CrossRID         string       `json:"cross_rid"`
	RecoveryHintCode string       `json:"recovery_hint_code,omitempty"`
}
type GraphMigration struct {
	MigrationID             string       `json:"migration_id"`
	FromVersion             int          `json:"from_version"`
	ToVersion               int          `json:"to_version"`
	Status                  string       `json:"status"`
	PreflightDigest         GraphDigest  `json:"preflight_digest"`
	BackupDigest            GraphDigest  `json:"backup_digest"`
	PriorActiveGenerationID *GraphDigest `json:"prior_active_generation_id,omitempty"`
	StartedAt               time.Time    `json:"started_at,omitempty"`
	CompletedAt             time.Time    `json:"completed_at,omitempty"`
	ErrorCode               string       `json:"error_code,omitempty"`
}
type GraphAnalysis struct {
	GenerationID           GraphDigest `json:"generation_id"`
	AnalysisKey            GraphDigest `json:"analysis_key"`
	ExtensionID            string      `json:"extension_id"`
	ExtensionVersion       string      `json:"extension_version"`
	ExecutableDigest       GraphDigest `json:"executable_digest"`
	Operation              string      `json:"operation"`
	ParametersDigest       GraphDigest `json:"parameters_digest"`
	AuthorityProfileDigest GraphDigest `json:"authority_profile_digest"`
	OutputSchema           string      `json:"output_schema"`
	ResultJSON             string      `json:"result_json"`
	ResultDigest           GraphDigest `json:"result_digest"`
	ProvenanceJSON         string      `json:"provenance_json"`
	OmissionsJSON          string      `json:"omissions_json"`
	Status                 string      `json:"status"`
	CreatedAt              time.Time   `json:"created_at"`
}
type GraphBundle struct {
	Generation GraphGeneration
	Nodes      []GraphNodeRecord
	Edges      []GraphEdgeRecord
	Evidence   []GraphEvidence
	Unresolved []GraphUnresolved
}

func validStatus(s string) bool {
	return s == GraphGenerationStaged || s == GraphGenerationActive || s == GraphGenerationRetired || s == GraphGenerationInvalid
}

var registeredGraphValues = map[string]map[string]struct{}{
	"backend_type": {"roslyn": {}, "go": {}, "tsserver": {}, "pyright": {}},
	"symbol_kind":  {"workspace": {}, "repository": {}, "project": {}, "package": {}, "file": {}, "namespace": {}, "type": {}, "method": {}, "function": {}, "field": {}, "property": {}, "event": {}, "route": {}, "test": {}, "document": {}},
	"relation":     {"contains": {}, "imports": {}, "references": {}, "calls": {}, "implements": {}, "extends": {}, "tests": {}, "route_to_handler": {}, "publishes": {}, "consumes": {}, "reads": {}, "writes": {}, "doc_mentions": {}},
}

func registered(v, field string) (string, error) {
	v, err := enum(v, field)
	if err != nil {
		return "", err
	}
	if _, ok := registeredGraphValues[field][v]; !ok {
		return "", graphErr("GPH_IDENTITY_REGISTERED_INVALID", field, "value is not registered for schema v1")
	}
	return v, nil
}
func validClaim(s string) bool {
	return s == GraphRecordExact || s == GraphRecordExtracted || s == GraphRecordInferred
}
func (b *GraphBundle) Validate() error { return b.validate(true) }

// ValidateGraphNodeRecord validates the canonical, self-contained node fields.
func ValidateGraphNodeRecord(n GraphNodeRecord) error {
	if n.NodeID < 0 || n.IdentitySchema != "milsp-node-key/v1" || !validClaim(n.ClaimStatus) {
		return ErrNodeKeyInvalid
	}
	identity, err := NewNodeKey(n.Identity)
	if err != nil {
		return err
	}
	key, err := identity.Hash()
	if err != nil || key != n.NodeKey || n.CrossRID != NodeRID(key) {
		return ErrNodeKeyInvalid
	}
	return nil
}

// ValidateGraphEdgeRecord validates an edge against its canonical endpoint keys.
func ValidateGraphEdgeRecord(e GraphEdgeRecord, from, to GraphDigest) error {
	if e.EdgeID < 0 || e.ClaimScope == "" || !validClaim(e.ClaimStatus) {
		return ErrGraphEdgeInvalid
	}
	if _, err := registered(e.Relation, "relation"); err != nil {
		return ErrGraphEdgeInvalid
	}
	if _, err := registered(e.SourceBackend, "backend_type"); err != nil {
		return ErrGraphEdgeInvalid
	}
	if _, err := slashPath(e.OwnerPath, "owner_path"); err != nil {
		return err
	}
	key := EdgeKey(from, to, e.Relation, e.ClaimScope)
	if e.EdgeKey != key || e.CrossRID != EdgeRID(key) {
		return ErrGraphEdgeInvalid
	}
	return nil
}

// ValidateGraphEvidence validates evidence against the exact node or edge key it supports.
func ValidateGraphEvidence(e GraphEvidence, subject GraphDigest) error {
	if e.EvidenceID < 0 || (e.NodeID == nil) == (e.EdgeID == nil) || (e.SubjectKind != "node" && e.SubjectKind != "edge") || e.SourceURI == "" || e.ExtractorVersion == "" || e.ClaimKind == "" || !validClaim(e.ClaimStatus) {
		return ErrGraphEvidenceInvalid
	}
	if _, err := registered(e.Backend, "backend_type"); err != nil {
		return ErrGraphEvidenceInvalid
	}
	if _, err := slashPath(e.SourceURI, "source_uri"); err != nil {
		return ErrGraphEvidenceInvalid
	}
	values := []*int{e.StartLine, e.StartColumn, e.EndLine, e.EndColumn}
	set := 0
	for _, v := range values {
		if v != nil {
			set++
			if *v < 0 {
				return ErrGraphEvidenceInvalid
			}
		}
	}
	if set != 0 && set != len(values) {
		return ErrGraphEvidenceInvalid
	}
	if set == len(values) && (*e.StartLine > *e.EndLine || (*e.StartLine == *e.EndLine && *e.StartColumn > *e.EndColumn)) {
		return ErrGraphEvidenceInvalid
	}
	sl, sc, el, ec := 0, 0, 0, 0
	if e.StartLine != nil {
		sl, sc, el, ec = *e.StartLine, *e.StartColumn, *e.EndLine, *e.EndColumn
	}
	digest := EvidenceDigest(e.SourceDigest, e.ObservedClaimDigest, e.SourceURI, e.ClaimKind, e.Backend, e.ExtractorVersion, sl, sc, el, ec)
	if e.EvidenceDigest != digest || e.EvidenceKey != EvidenceKey(subject, digest, e.EvidenceID) || e.CrossRID != EvidenceRID(e.EvidenceKey) {
		return ErrGraphEvidenceInvalid
	}
	return nil
}

// GraphUnresolvedKey derives the canonical key for an unresolved claim.
// Its field framing is part of the v1 graph identity contract.
func GraphUnresolvedKey(u GraphUnresolved) GraphDigest {
	var w binaryWriter
	w.WriteString("MILSP-UNRESOLVED/v1")
	w.text(1, u.OwnerPath)
	w.text(2, u.SubjectKind)
	w.digest(3, u.SelectorDigest)
	w.text(4, u.ReasonCode)
	for _, c := range u.Candidates {
		w.text(5, c)
	}
	w.text(6, u.Backend)
	if u.SourceDigest != nil {
		w.digest(7, *u.SourceDigest)
	}
	w.text(8, u.RecoveryHintCode)
	return digestBytes(w.Bytes())
}

// ValidateGraphUnresolved validates an unresolved claim and its canonical key.
func ValidateGraphUnresolved(u GraphUnresolved) error {
	if u.UnresolvedID < 0 || u.OwnerPath == "" || u.SubjectKind == "" || u.ReasonCode == "" || u.Backend == "" || len(u.Candidates) > maxUnresolvedCandidates {
		return ErrGraphUnresolved
	}
	if _, err := slashPath(u.OwnerPath, "owner_path"); err != nil {
		return ErrGraphUnresolved
	}
	total := 0
	for _, c := range u.Candidates {
		if _, err := required(c, "candidate"); err != nil {
			return ErrGraphUnresolved
		}
		total += len(c)
		if total > maxCandidateBytes {
			return ErrGraphUnresolved
		}
	}
	key := GraphUnresolvedKey(u)
	if u.UnresolvedKey != key || u.CrossRID != UnresolvedRID(key) {
		return ErrGraphUnresolved
	}
	return nil
}

func validateWorkspaceIdentity(v string) error {
	v, e := required(v, "workspace_identity")
	if e != nil {
		return e
	}
	if strings.HasPrefix(v, "/") || strings.HasPrefix(v, "~") || strings.HasPrefix(v, "\\\\") || (len(v) > 1 && v[1] == ':') {
		return graphErr("GPH_GENERATION_WORKSPACE_INVALID", "workspace_identity", "workspace metadata must not be an absolute path")
	}
	return nil
}
func (b *GraphBundle) validate(requireSealed bool) error {
	g := b.Generation
	if g.SchemaVersion < 1 || g.RepositoryIdentity == "" || !validStatus(g.Status) || g.NodeCount != len(b.Nodes) || g.EdgeCount != len(b.Edges) || g.EvidenceCount != len(b.Evidence) || g.UnresolvedCount != len(b.Unresolved) {
		return graphErr("GPH_GENERATION_INVALID", "generation", "invalid generation metadata or collection counts")
	}
	if e := validateWorkspaceIdentity(g.WorkspaceIdentity); e != nil {
		return e
	}
	normalRepo, e := NormalizeRepositoryIdentity(g.RepositoryIdentity)
	if e != nil {
		return e
	}
	if g.WorkspaceIdentity != normalRepo {
		return graphErr("GPH_GENERATION_WORKSPACE_INVALID", "workspace_identity", "workspace identity must equal normalized repository identity")
	}
	if requireSealed && (g.GenerationID == (GraphDigest{}) || g.ContentDigest == (GraphDigest{})) {
		return ErrGraphGenerationInvalid
	}
	nodes := map[int]GraphNodeRecord{}
	nodeKeys := map[GraphDigest]NodeKeyFields{}
	for i, n := range b.Nodes {
		if n.NodeID != i || n.GenerationID != g.GenerationID || n.IdentitySchema != "milsp-node-key/v1" || !validClaim(n.ClaimStatus) {
			return ErrGraphGenerationInvalid
		}
		if e := ValidateGraphNodeRecord(n); e != nil {
			return e
		}
		nk, e := NewNodeKey(n.Identity)
		if e != nil {
			return e
		}
		d, e := nk.Hash()
		if e != nil {
			return e
		}
		if old, ok := nodeKeys[d]; ok {
			if old != nk {
				return graphErr("GPH_IDENTITY_COLLISION", "node_key", "same node key has different canonical tuple")
			}
			return graphErr("GPH_IDENTITY_DUPLICATE", "node_key", "duplicate node key in generation")
		}
		nodeKeys[d] = nk
		if n.CrossRID != NodeRID(d) {
			return ErrNodeKeyInvalid
		}
		nodes[n.NodeID] = n
	}
	edges := map[int]GraphEdgeRecord{}
	edgeKeys := map[GraphDigest]bool{}
	for i, e := range b.Edges {
		if e.EdgeID != i || e.GenerationID != g.GenerationID || nodes[e.FromNodeID].NodeID != e.FromNodeID || nodes[e.ToNodeID].NodeID != e.ToNodeID || e.Relation == "" || e.SourceBackend == "" || !validClaim(e.ClaimStatus) {
			return ErrGraphEdgeInvalid
		}
		d := EdgeKey(nodes[e.FromNodeID].NodeKey, nodes[e.ToNodeID].NodeKey, e.Relation, e.ClaimScope)
		if x := ValidateGraphEdgeRecord(e, nodes[e.FromNodeID].NodeKey, nodes[e.ToNodeID].NodeKey); x != nil || edgeKeys[d] {
			return ErrGraphEdgeInvalid
		}
		edgeKeys[d] = true
		edges[e.EdgeID] = e
	}
	evidenceEdges := map[int]bool{}
	for i, e := range b.Evidence {
		if e.EvidenceID != i || e.GenerationID != g.GenerationID || e.Backend == "" || e.ExtractorVersion == "" || e.SourceURI == "" || strings.HasPrefix(e.SourceURI, "/") || strings.Contains(e.SourceURI, "..") || !validClaim(e.ClaimStatus) {
			return ErrGraphEvidenceInvalid
		}
		if (e.NodeID == nil) == (e.EdgeID == nil) {
			return ErrGraphEvidenceInvalid
		}
		var subject GraphDigest
		var kind string
		if e.NodeID != nil {
			n, ok := nodes[*e.NodeID]
			if !ok {
				return ErrGraphEvidenceInvalid
			}
			subject = n.NodeKey
			kind = "node"
		} else {
			ed, ok := edges[*e.EdgeID]
			if !ok {
				return ErrGraphEvidenceInvalid
			}
			subject = ed.EdgeKey
			kind = "edge"
			evidenceEdges[*e.EdgeID] = true
		}
		if e.SubjectKind != kind {
			return ErrGraphEvidenceInvalid
		}
		sl, sc, el, ec := 0, 0, 0, 0
		if e.StartLine != nil {
			sl = *e.StartLine
		}
		if e.StartColumn != nil {
			sc = *e.StartColumn
		}
		if e.EndLine != nil {
			el = *e.EndLine
		}
		if e.EndColumn != nil {
			ec = *e.EndColumn
		}
		_ = sl
		_ = sc
		_ = el
		_ = ec
		if x := ValidateGraphEvidence(e, subject); x != nil {
			return x
		}
	}
	for _, e := range b.Edges {
		if !evidenceEdges[e.EdgeID] {
			return ErrGraphEvidenceInvalid
		}
	}
	for i, u := range b.Unresolved {
		if u.UnresolvedID != i || u.GenerationID != g.GenerationID || ValidateGraphUnresolved(u) != nil {
			return ErrGraphUnresolved
		}
		if _, e := slashPath(u.OwnerPath, "owner_path"); e != nil {
			return e
		}
		total := 0
		for _, c := range u.Candidates {
			if _, e := required(c, "candidate"); e != nil {
				return e
			}
			total += len(c)
			if total > maxCandidateBytes {
				return ErrGraphUnresolved
			}
		}
		d := GraphUnresolvedKey(u)
		if d != u.UnresolvedKey || u.CrossRID != UnresolvedRID(d) {
			return ErrGraphUnresolved
		}
	}
	if requireSealed {
		cd, e := b.ContentDigest()
		if e != nil || cd != g.ContentDigest {
			return ErrGraphGenerationInvalid
		}
		expected := DeriveGenerationID(g.SchemaVersion, g.WorkspaceIdentity, g.SourceFingerprint, g.ConfigFingerprint, g.BackendManifestDigest, cd)
		if expected != g.GenerationID {
			return ErrGraphGenerationInvalid
		}
	}
	return nil
}

type GraphContentHasher struct {
	h               hash.Hash
	counts, written [4]int
	phase           int
}

func NewGraphContentHasher(nodes, edges, evidence, unresolved int) (*GraphContentHasher, error) {
	c := [4]int{nodes, edges, evidence, unresolved}
	for _, n := range c {
		if n < 0 {
			return nil, graphErr("GPH_GENERATION_INVALID", "content_counts", "content record counts must be non-negative")
		}
	}
	h := &GraphContentHasher{h: sha256.New(), counts: c}
	h.ws("MILSP-GRAPH-CONTENT/v2")
	h.section(0)
	return h, nil
}
func (h *GraphContentHasher) w(b []byte)  { _, _ = h.h.Write(b) }
func (h *GraphContentHasher) ws(s string) { h.w([]byte(s)) }
func (h *GraphContentHasher) f(t byte, b []byte) {
	h.w([]byte{t})
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(b)))
	h.w(n[:])
	h.w(b)
}
func (h *GraphContentHasher) text(t byte, s string)        { h.f(t, []byte(s)) }
func (h *GraphContentHasher) digest(t byte, d GraphDigest) { h.f(t, d[:]) }
func (h *GraphContentHasher) integer(t byte, n int) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(n))
	h.f(t, b[:])
}
func (h *GraphContentHasher) section(p int) {
	m := [...]string{"MILSP-GRAPH-NODES/v2", "MILSP-GRAPH-EDGES/v2", "MILSP-GRAPH-EVIDENCE/v2", "MILSP-GRAPH-UNRESOLVED/v2"}
	h.text(240, m[p])
	h.integer(241, h.counts[p])
}
func (h *GraphContentHasher) advance(to int) error {
	if to < h.phase || to > 3 {
		return graphErr("GPH_GENERATION_INVALID", "content_order", "graph content section is out of order")
	}
	for h.phase < to {
		if h.written[h.phase] != h.counts[h.phase] {
			return graphErr("GPH_GENERATION_INVALID", "content_counts", "graph content section is incomplete")
		}
		h.phase++
		h.section(h.phase)
	}
	return nil
}
func (h *GraphContentHasher) begin(p int) error {
	if h == nil || h.h == nil {
		return graphErr("GPH_GENERATION_INVALID", "content_hasher", "graph content hasher is nil")
	}
	if e := h.advance(p); e != nil {
		return e
	}
	if h.written[p] >= h.counts[p] {
		return graphErr("GPH_GENERATION_INVALID", "content_counts", "graph content has too many records")
	}
	h.f(242, nil)
	h.written[p]++
	return nil
}
func (h *GraphContentHasher) AddNode(n GraphNodeRecord) error {
	if e := h.begin(0); e != nil {
		return e
	}
	h.integer(1, n.NodeID)
	h.digest(2, n.NodeKey)
	h.text(3, n.IdentitySchema)
	h.text(4, n.Identity.RepositoryIdentity)
	h.text(5, n.Identity.BackendType)
	h.text(6, n.Identity.Language)
	h.text(7, n.Identity.ProjectOrModule)
	h.text(8, n.Identity.OwnerPath)
	h.text(9, n.Identity.SymbolKind)
	h.text(10, n.Identity.SemanticIdentity)
	h.text(11, n.DisplayName)
	h.digest(12, n.SourceDigest)
	h.text(13, n.ClaimStatus)
	h.text(14, n.CrossRID)
	h.text(15, n.SortKey)
	return nil
}
func (h *GraphContentHasher) AddEdge(e GraphEdgeRecord) error {
	if x := h.begin(1); x != nil {
		return x
	}
	h.integer(1, e.EdgeID)
	h.digest(2, e.EdgeKey)
	h.integer(3, e.FromNodeID)
	h.integer(4, e.ToNodeID)
	h.text(5, e.Relation)
	h.text(6, e.ClaimScope)
	h.text(7, e.ClaimStatus)
	h.text(8, e.OwnerPath)
	h.text(9, e.SourceBackend)
	h.text(10, e.CrossRID)
	return nil
}
func (h *GraphContentHasher) AddEvidence(e GraphEvidence) error {
	if x := h.begin(2); x != nil {
		return x
	}
	h.integer(1, e.EvidenceID)
	h.digest(2, e.EvidenceKey)
	h.digest(3, e.EvidenceDigest)
	h.text(4, e.SubjectKind)
	if e.NodeID != nil {
		h.integer(5, *e.NodeID)
	}
	if e.EdgeID != nil {
		h.integer(6, *e.EdgeID)
	}
	h.text(7, e.SourceURI)
	h.text(8, e.Backend)
	h.text(9, e.ExtractorVersion)
	h.digest(10, e.SourceDigest)
	h.text(11, e.ClaimKind)
	h.digest(12, e.ObservedClaimDigest)
	h.text(13, e.ClaimStatus)
	h.text(14, e.CrossRID)
	return nil
}
func (h *GraphContentHasher) AddUnresolved(u GraphUnresolved) error {
	if e := h.begin(3); e != nil {
		return e
	}
	h.integer(1, u.UnresolvedID)
	h.digest(2, u.UnresolvedKey)
	h.text(3, u.OwnerPath)
	h.text(4, u.SubjectKind)
	h.digest(5, u.SelectorDigest)
	h.text(6, u.ReasonCode)
	for _, c := range u.Candidates {
		h.text(7, c)
	}
	h.text(8, u.Backend)
	if u.SourceDigest != nil {
		h.digest(9, *u.SourceDigest)
	}
	h.text(10, u.CrossRID)
	h.text(11, u.RecoveryHintCode)
	return nil
}
func (h *GraphContentHasher) Sum() (GraphDigest, error) {
	if h == nil || h.h == nil {
		return GraphDigest{}, graphErr("GPH_GENERATION_INVALID", "content_hasher", "graph content hasher is nil")
	}
	if e := h.advance(3); e != nil {
		return GraphDigest{}, e
	}
	if h.written[3] != h.counts[3] {
		return GraphDigest{}, graphErr("GPH_GENERATION_INVALID", "content_counts", "graph content section is incomplete")
	}
	var d GraphDigest
	copy(d[:], h.h.Sum(nil))
	return d, nil
}
func (b *GraphBundle) ContentDigest() (GraphDigest, error) {
	if e := b.validate(false); e != nil {
		return GraphDigest{}, e
	}
	nodes := append([]GraphNodeRecord(nil), b.Nodes...)
	edges := append([]GraphEdgeRecord(nil), b.Edges...)
	ev := append([]GraphEvidence(nil), b.Evidence...)
	un := append([]GraphUnresolved(nil), b.Unresolved...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].NodeID < nodes[j].NodeID })
	sort.Slice(edges, func(i, j int) bool { return edges[i].EdgeID < edges[j].EdgeID })
	sort.Slice(ev, func(i, j int) bool { return ev[i].EvidenceID < ev[j].EvidenceID })
	sort.Slice(un, func(i, j int) bool { return un[i].UnresolvedID < un[j].UnresolvedID })
	h, e := NewGraphContentHasher(len(nodes), len(edges), len(ev), len(un))
	if e != nil {
		return GraphDigest{}, e
	}
	for _, n := range nodes {
		if e = h.AddNode(n); e != nil {
			return GraphDigest{}, e
		}
	}
	for _, x := range edges {
		if e = h.AddEdge(x); e != nil {
			return GraphDigest{}, e
		}
	}
	for _, x := range ev {
		if e = h.AddEvidence(x); e != nil {
			return GraphDigest{}, e
		}
	}
	for _, x := range un {
		if e = h.AddUnresolved(x); e != nil {
			return GraphDigest{}, e
		}
	}
	return h.Sum()
}

func generationBytes(schema int, repo string, source, config, backend, content GraphDigest) ([]byte, error) {
	canonical, e := NormalizeRepositoryIdentity(repo)
	if e != nil {
		return nil, e
	}
	if schema < 1 {
		return nil, graphErr("GPH_GENERATION_INVALID", "schema_version", "schema version is required")
	}
	var w binaryWriter
	w.WriteString("MILSP-GENERATION/v2")
	w.integer(1, schema)
	w.text(2, canonical)
	w.digest(3, source)
	w.digest(4, config)
	w.digest(5, backend)
	w.digest(6, content)
	return w.Bytes(), nil
}
func DeriveGenerationID(schema int, repo string, source, config, backend, content GraphDigest) GraphDigest {
	raw, e := generationBytes(schema, repo, source, config, backend, content)
	if e != nil {
		return GraphDigest{}
	}
	return digestBytes(raw)
}
func (b *GraphBundle) SealIDs() error {
	cd, e := b.ContentDigest()
	if e != nil {
		return e
	}
	b.Generation.ContentDigest = cd
	b.Generation.GenerationID = DeriveGenerationID(b.Generation.SchemaVersion, b.Generation.WorkspaceIdentity, b.Generation.SourceFingerprint, b.Generation.ConfigFingerprint, b.Generation.BackendManifestDigest, cd)
	for i := range b.Nodes {
		b.Nodes[i].GenerationID = b.Generation.GenerationID
	}
	for i := range b.Edges {
		b.Edges[i].GenerationID = b.Generation.GenerationID
	}
	for i := range b.Evidence {
		b.Evidence[i].GenerationID = b.Generation.GenerationID
	}
	for i := range b.Unresolved {
		b.Unresolved[i].GenerationID = b.Generation.GenerationID
	}
	return nil
}
