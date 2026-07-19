package model

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/text/unicode/norm"
	"reflect"
	"sort"
	"strings"
)

const (
	GraphObservationSchemaVersion      = 1
	GraphCompletenessComplete          = "complete"
	GraphCompletenessPartial           = "partial"
	GraphObservationStatusStable       = "stable"
	GraphObservationStatusExperimental = "experimental"
	GraphObservationStatusGated        = "gated"
	GraphObservationStatusUnsupported  = "unsupported"
	GraphObservationStatusUnavailable  = "unavailable"
)

var (
	ErrGraphObservationInvalid      = errors.New("invalid graph observation batch")
	ErrGraphObservationNotStageable = errors.New("graph observation batch is not stageable")
)

type GraphObservationError struct{ Code, Field, Message string }

func (e *GraphObservationError) Error() string {
	return fmt.Sprintf("graph observation %s %s: %s", e.Code, e.Field, e.Message)
}
func (e *GraphObservationError) Unwrap() error { return ErrGraphObservationInvalid }
func observationErr(c, f, m string) error      { return &GraphObservationError{c, f, m} }

type GraphObservationRange struct {
	StartLine   int `json:"start_line"`
	StartColumn int `json:"start_column"`
	EndLine     int `json:"end_line"`
	EndColumn   int `json:"end_column"`
}
type GraphObservationNode struct {
	Ref          string        `json:"ref"`
	Key          NodeKeyFields `json:"key"`
	DisplayName  string        `json:"display_name"`
	SourceDigest GraphDigest   `json:"source_digest"`
	ClaimStatus  string        `json:"claim_status"`
	Resolution   string        `json:"resolution"`
}
type GraphObservationEdge struct {
	Ref        string `json:"ref"`
	FromRef    string `json:"from_ref"`
	ToRef      string `json:"to_ref"`
	Relation   string `json:"relation"`
	Scope      string `json:"scope"`
	Status     string `json:"status"`
	OwnerPath  string `json:"owner_path"`
	Backend    string `json:"backend"`
	Resolution string `json:"resolution"`
}
type GraphObservationEvidence struct {
	Ref              string                 `json:"ref"`
	NodeRef          string                 `json:"node_ref,omitempty"`
	EdgeRef          string                 `json:"edge_ref,omitempty"`
	SourceURI        string                 `json:"source_uri"`
	Range            *GraphObservationRange `json:"range,omitempty"`
	Backend          string                 `json:"backend"`
	ExtractorVersion string                 `json:"extractor_version"`
	SourceDigest     GraphDigest            `json:"source_digest"`
	ObservedDigest   GraphDigest            `json:"observed_digest"`
	ClaimKind        string                 `json:"claim_kind"`
	Status           string                 `json:"status"`
}
type GraphObservationUnresolved struct {
	Ref              string       `json:"ref"`
	OwnerPath        string       `json:"owner_path"`
	SubjectKind      string       `json:"subject_kind"`
	SelectorDigest   GraphDigest  `json:"selector_digest"`
	ReasonCode       string       `json:"reason_code"`
	Candidates       []string     `json:"candidates,omitempty"`
	Backend          string       `json:"backend"`
	SourceDigest     *GraphDigest `json:"source_digest,omitempty"`
	RecoveryHintCode string       `json:"recovery_hint_code"`
}
type GraphObservationOmission struct {
	Ref              string `json:"ref"`
	OwnerPath        string `json:"owner_path"`
	SubjectKind      string `json:"subject_kind"`
	Backend          string `json:"backend"`
	Capability       string `json:"capability"`
	ReasonCode       string `json:"reason_code"`
	RecoveryHintCode string `json:"recovery_hint_code"`
}
type GraphObservationCapability struct {
	Backend    string `json:"backend"`
	Capability string `json:"capability"`
	State      string `json:"state"`
}
type GraphObservationCoverage struct {
	Backend    string `json:"backend"`
	Capability string `json:"capability"`
	Eligible   int    `json:"eligible"`
	Observed   int    `json:"observed"`
	Unresolved int    `json:"unresolved"`
	Omitted    int    `json:"omitted"`
}
type GraphObservationResourceStats struct {
	ElapsedMilliseconds int64 `json:"elapsed_ms"`
	RSSBytes            int64 `json:"rss_bytes"`
}
type GraphObservationBatch struct {
	SchemaVersion      int                            `json:"schema_version"`
	WorkspaceIdentity  string                         `json:"workspace_identity"`
	RepositoryIdentity string                         `json:"repository_identity"`
	Backend            string                         `json:"backend"`
	BackendVersion     string                         `json:"backend_version"`
	ExtractorVersion   string                         `json:"extractor_version"`
	ProjectOrModule    string                         `json:"project_or_module"`
	SourceFingerprint  GraphDigest                    `json:"source_fingerprint"`
	ConfigFingerprint  GraphDigest                    `json:"config_fingerprint"`
	Completeness       string                         `json:"completeness"`
	Capabilities       []GraphObservationCapability   `json:"capabilities"`
	Coverage           []GraphObservationCoverage     `json:"coverage"`
	Nodes              []GraphObservationNode         `json:"nodes"`
	Edges              []GraphObservationEdge         `json:"edges"`
	Evidence           []GraphObservationEvidence     `json:"evidence"`
	Unresolved         []GraphObservationUnresolved   `json:"unresolved"`
	Omissions          []GraphObservationOmission     `json:"omissions"`
	ResourceStats      *GraphObservationResourceStats `json:"resource_stats,omitempty"`
	Digest             GraphDigest                    `json:"digest"`
}

func observationNonzero(d GraphDigest) bool { return d != (GraphDigest{}) }
func obsText(s string) string               { return norm.NFC.String(strings.ToLower(strings.TrimSpace(s))) }
func obsBounded(s string) bool {
	return s != "" && len(s) <= 256 && !strings.ContainsAny(s, "\r\n\x00")
}
func observationBackend(s string) bool {
	return s == "roslyn" || s == "go" || s == "tsserver" || s == "pyright"
}
func observationState(s string) bool {
	return s == GraphObservationStatusStable || s == GraphObservationStatusExperimental || s == GraphObservationStatusGated || s == GraphObservationStatusUnsupported || s == GraphObservationStatusUnavailable
}
func observationResolution(s string) bool {
	return s == "roslyn" || s == "go/ast" || s == "go/types" || s == "gopls" || s == "tsserver" || s == "pyright" || s == "lexical" || s == "unresolved"
}
func observationClaim(s string) bool {
	return s == GraphRecordExact || s == GraphRecordExtracted || s == GraphRecordInferred
}
func observationScope(s string) bool {
	return s == "symbol" || s == "file" || s == "project" || s == "package"
}
func observationRelation(s string) bool {
	return s != "declarations" && observationCapability(s)
}
func observationCapability(s string) bool {
	if s == "declarations" {
		return true
	}
	for _, v := range []string{"contains", "imports", "references", "calls", "implements", "extends", "tests", "route_to_handler", "publishes", "consumes", "reads", "writes", "doc_mentions"} {
		if s == v {
			return true
		}
	}
	return false
}
func canonicalPath(s string) (string, error) { return slashPath(obsText(s), "path") }
func sortObservationSlices(b *GraphObservationBatch) {
	sort.Slice(b.Capabilities, func(i, j int) bool { return b.Capabilities[i].Capability < b.Capabilities[j].Capability })
	sort.Slice(b.Coverage, func(i, j int) bool { return b.Coverage[i].Capability < b.Coverage[j].Capability })
	sort.Slice(b.Nodes, func(i, j int) bool { return b.Nodes[i].Ref < b.Nodes[j].Ref })
	sort.Slice(b.Edges, func(i, j int) bool { return b.Edges[i].Ref < b.Edges[j].Ref })
	sort.Slice(b.Evidence, func(i, j int) bool { return b.Evidence[i].Ref < b.Evidence[j].Ref })
	sort.Slice(b.Unresolved, func(i, j int) bool { return b.Unresolved[i].Ref < b.Unresolved[j].Ref })
	sort.Slice(b.Omissions, func(i, j int) bool { return b.Omissions[i].Ref < b.Omissions[j].Ref })
	for i := range b.Unresolved {
		sort.Strings(b.Unresolved[i].Candidates)
	}
}
func cloneObservation(b *GraphObservationBatch) GraphObservationBatch {
	x := *b
	x.Capabilities = append([]GraphObservationCapability(nil), b.Capabilities...)
	x.Coverage = append([]GraphObservationCoverage(nil), b.Coverage...)
	x.Nodes = append([]GraphObservationNode(nil), b.Nodes...)
	x.Edges = append([]GraphObservationEdge(nil), b.Edges...)
	x.Evidence = append([]GraphObservationEvidence(nil), b.Evidence...)
	x.Unresolved = append([]GraphObservationUnresolved(nil), b.Unresolved...)
	x.Omissions = append([]GraphObservationOmission(nil), b.Omissions...)
	for i := range x.Evidence {
		if b.Evidence[i].Range != nil {
			r := *b.Evidence[i].Range
			x.Evidence[i].Range = &r
		}
	}
	for i := range x.Unresolved {
		x.Unresolved[i].Candidates = append([]string(nil), b.Unresolved[i].Candidates...)
	}
	return x
}
func observationSemanticBytes(b *GraphObservationBatch) ([]byte, error) {
	x := *b
	x.ResourceStats = nil
	x.Digest = GraphDigest{}
	return json.Marshal(x)
}
func observationDigest(b *GraphObservationBatch) (GraphDigest, error) {
	p, e := observationSemanticBytes(b)
	if e != nil {
		return GraphDigest{}, e
	}
	return GraphDigest(sha256.Sum256(p)), nil
}

func (b *GraphObservationBatch) canonicalize() error {
	var e error
	b.Backend = obsText(b.Backend)
	b.BackendVersion = obsText(b.BackendVersion)
	b.ExtractorVersion = obsText(b.ExtractorVersion)
	b.ProjectOrModule = obsText(b.ProjectOrModule)
	b.RepositoryIdentity, e = NormalizeRepositoryIdentity(b.RepositoryIdentity)
	if e != nil {
		return e
	}
	b.WorkspaceIdentity = obsText(b.WorkspaceIdentity)
	if b.WorkspaceIdentity != b.RepositoryIdentity {
		return observationErr("GPH_OBS_WORKSPACE_MISMATCH", "workspace_identity", "workspace must equal normalized repository")
	}
	b.ProjectOrModule, e = canonicalPath(b.ProjectOrModule)
	if e != nil {
		return e
	}
	for i := range b.Nodes {
		n := &b.Nodes[i]
		n.Ref = obsText(n.Ref)
		n.DisplayName = norm.NFC.String(strings.TrimSpace(n.DisplayName))
		n.Key.RepositoryIdentity, e = NormalizeRepositoryIdentity(n.Key.RepositoryIdentity)
		if e != nil {
			return e
		}
		n.Key.BackendType = obsText(n.Key.BackendType)
		n.Key.Language = obsText(n.Key.Language)
		n.Key.ProjectOrModule, e = canonicalPath(n.Key.ProjectOrModule)
		if e != nil {
			return e
		}
		n.Key.OwnerPath, e = canonicalPath(n.Key.OwnerPath)
		if e != nil {
			return e
		}
		n.Key.SymbolKind = obsText(n.Key.SymbolKind)
		n.Key.SemanticIdentity = norm.NFC.String(strings.TrimSpace(n.Key.SemanticIdentity))
		n.ClaimStatus = obsText(n.ClaimStatus)
		n.Resolution = obsText(n.Resolution)
	}
	for i := range b.Edges {
		e1 := &b.Edges[i]
		e1.Ref = obsText(e1.Ref)
		e1.FromRef = obsText(e1.FromRef)
		e1.ToRef = obsText(e1.ToRef)
		e1.Relation = obsText(e1.Relation)
		e1.Scope = obsText(e1.Scope)
		e1.Status = obsText(e1.Status)
		e1.Backend = obsText(e1.Backend)
		e1.Resolution = obsText(e1.Resolution)
		e1.OwnerPath, e = canonicalPath(e1.OwnerPath)
		if e != nil {
			return e
		}
	}
	for i := range b.Capabilities {
		c := &b.Capabilities[i]
		c.Backend = obsText(c.Backend)
		c.Capability = obsText(c.Capability)
		c.State = obsText(c.State)
	}
	for i := range b.Coverage {
		c := &b.Coverage[i]
		c.Backend = obsText(c.Backend)
		c.Capability = obsText(c.Capability)
	}
	for i := range b.Evidence {
		v := &b.Evidence[i]
		v.Ref = obsText(v.Ref)
		v.NodeRef = obsText(v.NodeRef)
		v.EdgeRef = obsText(v.EdgeRef)
		v.SourceURI, e = canonicalPath(v.SourceURI)
		if e != nil {
			return e
		}
		v.Backend = obsText(v.Backend)
		v.ExtractorVersion = obsText(v.ExtractorVersion)
		v.ClaimKind = obsText(v.ClaimKind)
		v.Status = obsText(v.Status)
		if v.Range != nil {
			*v.Range = GraphObservationRange{v.Range.StartLine, v.Range.StartColumn, v.Range.EndLine, v.Range.EndColumn}
		}
	}
	for i := range b.Unresolved {
		u := &b.Unresolved[i]
		u.Ref = obsText(u.Ref)
		u.OwnerPath, e = canonicalPath(u.OwnerPath)
		if e != nil {
			return e
		}
		u.SubjectKind = obsText(u.SubjectKind)
		u.ReasonCode = obsText(u.ReasonCode)
		u.Backend = obsText(u.Backend)
		u.RecoveryHintCode = obsText(u.RecoveryHintCode)
		for j := range u.Candidates {
			u.Candidates[j] = norm.NFC.String(strings.TrimSpace(u.Candidates[j]))
		}
	}
	for i := range b.Omissions {
		o := &b.Omissions[i]
		o.Ref = obsText(o.Ref)
		o.OwnerPath, e = canonicalPath(o.OwnerPath)
		if e != nil {
			return e
		}
		o.SubjectKind = obsText(o.SubjectKind)
		o.Backend = obsText(o.Backend)
		o.Capability = obsText(o.Capability)
		o.ReasonCode = obsText(o.ReasonCode)
		o.RecoveryHintCode = obsText(o.RecoveryHintCode)
	}
	sortObservationSlices(b)
	return nil
}
func (b *GraphObservationBatch) validateCore() error {
	if b.SchemaVersion != 1 || !observationBackend(b.Backend) || b.Completeness != GraphCompletenessComplete && b.Completeness != GraphCompletenessPartial {
		return observationErr("GPH_OBS_CORE_INVALID", "batch", "invalid schema/backend/completeness")
	}
	if b.WorkspaceIdentity != b.RepositoryIdentity || b.BackendVersion == "" || b.ExtractorVersion == "" || b.ProjectOrModule == "" || !observationNonzero(b.SourceFingerprint) || !observationNonzero(b.ConfigFingerprint) {
		return observationErr("GPH_OBS_PROVENANCE_MISSING", "batch", "invalid provenance")
	}
	seen := map[string]bool{}
	nodes := map[string]bool{}
	edges := map[string]GraphObservationEdge{}
	caps := map[string]bool{}
	cov := map[string]GraphObservationCoverage{}
	add := func(r string) error {
		if !obsBounded(r) || seen[r] {
			return observationErr("GPH_OBS_REF_DUPLICATE", "ref", "refs must be globally unique")
		}
		seen[r] = true
		return nil
	}
	for _, n := range b.Nodes {
		if e := add(n.Ref); e != nil {
			return e
		}
		nodes[n.Ref] = true
		k, e := NewNodeKey(n.Key)
		if e != nil || !reflect.DeepEqual(k, n.Key) || n.Key.RepositoryIdentity != b.RepositoryIdentity || n.Key.BackendType != b.Backend || n.Key.ProjectOrModule != b.ProjectOrModule || !obsBounded(n.DisplayName) || !observationNonzero(n.SourceDigest) || !observationClaim(n.ClaimStatus) || !observationResolution(n.Resolution) {
			return observationErr("GPH_OBS_NODE_INVALID", "nodes", "node violates canonical batch or matrix")
		}
	}
	for _, e := range b.Edges {
		if x := add(e.Ref); x != nil {
			return x
		}
		edges[e.Ref] = e
		if !observationRelation(e.Relation) || !observationScope(e.Scope) || !observationClaim(e.Status) || e.Backend != b.Backend || !observationResolution(e.Resolution) {
			return observationErr("GPH_OBS_EDGE_INVALID", "edges", "invalid edge")
		}
		if !nodes[e.FromRef] || !nodes[e.ToRef] {
			return observationErr("GPH_OBS_ENDPOINT_UNRESOLVED", "edges", "missing endpoint must be unresolved")
		}
		if e.Backend == "roslyn" && (e.Status != GraphRecordExact || e.Resolution != "roslyn" || !(e.Relation == "contains" || e.Relation == "references" || e.Relation == "calls" || e.Relation == "implements" || e.Relation == "extends")) {
			return observationErr("GPH_OBS_RELATION_GATE", "edges", "Roslyn edge not permitted")
		}
		if e.Backend == "go" && e.Relation != "contains" && e.Relation != "imports" && (e.Status != GraphRecordExact || !(e.Resolution == "go/types" || e.Resolution == "gopls")) {
			return observationErr("GPH_OBS_RELATION_GATE", "edges", "Go semantic edge not typed")
		}
		if e.Backend == "go" && (e.Relation == "contains" || e.Relation == "imports") && (e.Status != GraphRecordExtracted || e.Resolution != "go/ast") {
			return observationErr("GPH_OBS_RELATION_GATE", "edges", "Go AST edge provenance required")
		}
		if (b.Backend == "tsserver" || b.Backend == "pyright") || e.Resolution == "lexical" {
			return observationErr("GPH_OBS_RELATION_GATE", "edges", "backend has no stable semantic edge")
		}
	}
	for _, c := range b.Capabilities {
		if c.Backend != b.Backend || !observationCapability(c.Capability) || !observationState(c.State) || caps[c.Capability] {
			return observationErr("GPH_OBS_CAPABILITY_INVALID", "capabilities", "duplicate or invalid capability")
		}
		caps[c.Capability] = true
	}
	for _, c := range b.Coverage {
		if c.Backend != b.Backend || !observationCapability(c.Capability) || c.Eligible < 0 || c.Observed < 0 || c.Unresolved < 0 || c.Omitted < 0 || c.Observed+c.Unresolved+c.Omitted != c.Eligible || cov[c.Capability].Capability != "" {
			return observationErr("GPH_OBS_COVERAGE_INVALID", "coverage", "coverage mismatch or duplicate")
		}
		cov[c.Capability] = c
		if !caps[c.Capability] {
			return observationErr("GPH_OBS_COVERAGE_INVALID", "coverage", "coverage capability undeclared")
		}
	}
	for k := range caps {
		if _, ok := cov[k]; !ok {
			return observationErr("GPH_OBS_COVERAGE_MISSING", "coverage", "capability lacks coverage")
		}
	}
	for _, v := range b.Evidence {
		if e := add(v.Ref); e != nil {
			return e
		}
		if (v.NodeRef == "") == (v.EdgeRef == "") || v.Backend != b.Backend || v.ExtractorVersion != b.ExtractorVersion || !observationNonzero(v.SourceDigest) || !observationNonzero(v.ObservedDigest) || !observationClaim(v.Status) {
			return observationErr("GPH_OBS_EVIDENCE_INVALID", "evidence", "invalid evidence")
		}
		if v.NodeRef != "" && !nodes[v.NodeRef] || v.EdgeRef != "" && !nodes[edges[v.EdgeRef].FromRef] {
			return observationErr("GPH_OBS_EVIDENCE_INVALID", "evidence", "unknown target")
		}
		if v.NodeRef != "" && v.ClaimKind != "declaration" {
			return observationErr("GPH_OBS_EVIDENCE_INVALID", "evidence", "node evidence must be declaration")
		}
		if v.EdgeRef != "" && v.ClaimKind != edges[v.EdgeRef].Relation {
			return observationErr("GPH_OBS_EVIDENCE_INVALID", "evidence", "edge claim mismatch")
		}
		if v.Range != nil && (v.Range.StartLine < 1 || v.Range.StartColumn < 1 || v.Range.EndLine < v.Range.StartLine || v.Range.EndColumn < v.Range.StartColumn) {
			return observationErr("GPH_OBS_RANGE_INVALID", "evidence.range", "range must be positive ordered")
		}
	}
	for _, n := range b.Nodes {
		ok := false
		for _, v := range b.Evidence {
			if v.NodeRef == n.Ref {
				ok = true
			}
		}
		if !ok {
			return observationErr("GPH_OBS_EVIDENCE_MISSING", "nodes", "node requires evidence")
		}
	}
	for _, u := range b.Unresolved {
		if e := add(u.Ref); e != nil {
			return e
		}
		if u.Backend != b.Backend || u.OwnerPath == "" || u.SubjectKind == "" || !observationNonzero(u.SelectorDigest) || u.ReasonCode == "" || u.RecoveryHintCode == "" || len(u.Candidates) > 64 {
			return observationErr("GPH_OBS_UNRESOLVED_INVALID", "unresolved", "typed unresolved required")
		}
	}
	for _, o := range b.Omissions {
		if e := add(o.Ref); e != nil {
			return e
		}
		if o.Backend != b.Backend || o.OwnerPath == "" || o.SubjectKind == "" || !observationCapability(o.Capability) || o.ReasonCode == "" || o.RecoveryHintCode == "" {
			return observationErr("GPH_OBS_OMISSION_INVALID", "omissions", "typed omission required")
		}
	}
	return nil
}
func (b *GraphObservationBatch) Validate() error {
	if b == nil {
		return ErrGraphObservationInvalid
	}
	c := cloneObservation(b)
	if e := c.canonicalize(); e != nil {
		return e
	}
	if e := c.validateCore(); e != nil {
		return e
	}
	if !reflect.DeepEqual(*b, c) {
		return observationErr("GPH_OBS_NONCANONICAL", "batch", "batch is not canonical")
	}
	d, e := observationDigest(b)
	if e != nil {
		return e
	}
	if b.Digest != d {
		return observationErr("GPH_OBS_DIGEST_MISMATCH", "digest", "semantic digest mismatch")
	}
	return nil
}
func SealGraphObservationBatch(b *GraphObservationBatch) error {
	if b == nil {
		return ErrGraphObservationInvalid
	}
	if e := b.canonicalize(); e != nil {
		return e
	}
	if e := b.validateCore(); e != nil {
		return e
	}
	d, e := observationDigest(b)
	if e != nil {
		return e
	}
	b.Digest = d
	return b.Validate()
}
func (b *GraphObservationBatch) ReadyForStaging() error {
	if e := b.Validate(); e != nil {
		return e
	}
	if b.Completeness != GraphCompletenessComplete || b.Backend != "roslyn" && b.Backend != "go" {
		return fmt.Errorf("%w: incomplete or unsupported backend", ErrGraphObservationNotStageable)
	}
	decl := false
	for _, c := range b.Capabilities {
		if c.State != GraphObservationStatusStable {
			return fmt.Errorf("%w: capability not stable", ErrGraphObservationNotStageable)
		}
		if c.Capability == "declarations" {
			decl = true
		}
	}
	if !decl {
		return fmt.Errorf("%w: declarations capability required", ErrGraphObservationNotStageable)
	}
	return nil
}
