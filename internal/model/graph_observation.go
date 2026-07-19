package model

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
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
	Ref          string      `json:"ref"`
	FromRef      string      `json:"from_ref"`
	ToRef        string      `json:"to_ref"`
	Relation     string      `json:"relation"`
	Scope        string      `json:"scope"`
	Status       string      `json:"status"`
	OwnerPath    string      `json:"owner_path"`
	Backend      string      `json:"backend"`
	Resolution   string      `json:"resolution"`
	SourceDigest GraphDigest `json:"source_digest"`
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
	Capability       string       `json:"capability"`
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
func nfcTrim(s string) string               { return norm.NFC.String(strings.TrimSpace(s)) }
func enumLower(s string) string             { return strings.ToLower(nfcTrim(s)) }
func obsBounded(s string) bool {
	if s == "" || len(s) > 256 {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func stableCode(s string) bool {
	if !obsBounded(s) {
		return false
	}
	for i, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return false
		}
		if i == 0 && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
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
func observationRelation(s string) bool      { return s != "declarations" && observationCapability(s) }
func canonicalPath(s string) (string, error) { return slashPath(nfcTrim(s), "path") }
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
	b.Backend = enumLower(b.Backend)
	b.BackendVersion = nfcTrim(b.BackendVersion)
	b.ExtractorVersion = nfcTrim(b.ExtractorVersion)
	b.ProjectOrModule, e = canonicalPath(b.ProjectOrModule)
	if e != nil {
		return e
	}
	b.RepositoryIdentity, e = NormalizeRepositoryIdentity(nfcTrim(b.RepositoryIdentity))
	if e != nil {
		return e
	}
	b.WorkspaceIdentity, e = NormalizeRepositoryIdentity(nfcTrim(b.WorkspaceIdentity))
	if e != nil {
		return e
	}
	if b.WorkspaceIdentity != b.RepositoryIdentity {
		return observationErr("GPH_OBS_WORKSPACE_MISMATCH", "workspace_identity", "workspace must equal normalized repository")
	}
	for i := range b.Nodes {
		n := &b.Nodes[i]
		n.Ref = nfcTrim(n.Ref)
		n.DisplayName = nfcTrim(n.DisplayName)
		n.Key.RepositoryIdentity, e = NormalizeRepositoryIdentity(nfcTrim(n.Key.RepositoryIdentity))
		if e != nil {
			return e
		}
		n.Key.BackendType = enumLower(n.Key.BackendType)
		n.Key.Language = enumLower(n.Key.Language)
		n.Key.ProjectOrModule, e = canonicalPath(n.Key.ProjectOrModule)
		if e != nil {
			return e
		}
		n.Key.OwnerPath, e = canonicalPath(n.Key.OwnerPath)
		if e != nil {
			return e
		}
		n.Key.SymbolKind = enumLower(n.Key.SymbolKind)
		n.Key.SemanticIdentity = nfcTrim(n.Key.SemanticIdentity)
		n.ClaimStatus = enumLower(n.ClaimStatus)
		n.Resolution = enumLower(n.Resolution)
	}
	for i := range b.Edges {
		v := &b.Edges[i]
		v.Ref = nfcTrim(v.Ref)
		v.FromRef = nfcTrim(v.FromRef)
		v.ToRef = nfcTrim(v.ToRef)
		v.Relation = enumLower(v.Relation)
		v.Scope = enumLower(v.Scope)
		v.Status = enumLower(v.Status)
		v.OwnerPath, e = canonicalPath(v.OwnerPath)
		if e != nil {
			return e
		}
		v.Backend = enumLower(v.Backend)
		v.Resolution = enumLower(v.Resolution)
	}
	for i := range b.Capabilities {
		v := &b.Capabilities[i]
		v.Backend = enumLower(v.Backend)
		v.Capability = enumLower(v.Capability)
		v.State = enumLower(v.State)
	}
	for i := range b.Coverage {
		v := &b.Coverage[i]
		v.Backend = enumLower(v.Backend)
		v.Capability = enumLower(v.Capability)
	}
	for i := range b.Evidence {
		v := &b.Evidence[i]
		v.Ref = nfcTrim(v.Ref)
		v.NodeRef = nfcTrim(v.NodeRef)
		v.EdgeRef = nfcTrim(v.EdgeRef)
		v.SourceURI, e = canonicalPath(v.SourceURI)
		if e != nil {
			return e
		}
		v.Backend = enumLower(v.Backend)
		v.ExtractorVersion = nfcTrim(v.ExtractorVersion)
		v.ClaimKind = enumLower(v.ClaimKind)
		v.Status = enumLower(v.Status)
	}
	for i := range b.Unresolved {
		v := &b.Unresolved[i]
		v.Ref = nfcTrim(v.Ref)
		v.OwnerPath, e = canonicalPath(v.OwnerPath)
		if e != nil {
			return e
		}
		v.SubjectKind = enumLower(v.SubjectKind)
		v.Capability = enumLower(v.Capability)
		v.Backend = enumLower(v.Backend)
		v.ReasonCode = enumLower(v.ReasonCode)
		v.RecoveryHintCode = enumLower(v.RecoveryHintCode)
		for j := range v.Candidates {
			v.Candidates[j] = nfcTrim(v.Candidates[j])
		}
	}
	for i := range b.Omissions {
		v := &b.Omissions[i]
		v.Ref = nfcTrim(v.Ref)
		v.OwnerPath, e = canonicalPath(v.OwnerPath)
		if e != nil {
			return e
		}
		v.SubjectKind = enumLower(v.SubjectKind)
		v.Backend = enumLower(v.Backend)
		v.Capability = enumLower(v.Capability)
		v.ReasonCode = enumLower(v.ReasonCode)
		v.RecoveryHintCode = enumLower(v.RecoveryHintCode)
	}
	sortObservationSlices(b)
	return nil
}

func capabilityState(caps map[string]GraphObservationCapability, cap string) (GraphObservationCapability, bool) {
	c, ok := caps[cap]
	return c, ok
}
func nodeAllowed(b, cap, claim, res string, state string) bool {
	if cap != "declarations" {
		return false
	}
	if b == "roslyn" {
		return state == "stable" && claim == GraphRecordExact && res == "roslyn"
	}
	if b == "go" {
		return state == "stable" && (claim == GraphRecordExtracted && res == "go/ast" || claim == GraphRecordExact && (res == "go/types" || res == "gopls")) || state == GraphObservationStatusExperimental && claim == GraphRecordExtracted && res == "lexical"
	}
	return false
}

func stableCapabilityAllowed(backend, capability string) bool {
	if backend == "roslyn" {
		return capability == "declarations" || capability == "contains" || capability == "references" || capability == "calls" || capability == "implements" || capability == "extends"
	}
	if backend == "go" {
		return capability == "declarations" || capability == "contains" || capability == "imports" || capability == "references" || capability == "calls"
	}
	return false
}
func edgeAllowed(b, rel, claim, res, state string) bool {
	if state != "stable" {
		return false
	}
	if b == "roslyn" {
		return claim == GraphRecordExact && res == "roslyn" && (rel == "contains" || rel == "references" || rel == "calls" || rel == "implements" || rel == "extends")
	}
	if b == "go" {
		if rel == "contains" || rel == "imports" {
			return claim == GraphRecordExtracted && res == "go/ast"
		}
		return (rel == "calls" || rel == "references") && claim == GraphRecordExact && (res == "go/types" || res == "gopls")
	}
	return false
}

func (b *GraphObservationBatch) validateCore() error {
	if b.SchemaVersion != 1 || !observationBackend(b.Backend) || (b.Completeness != GraphCompletenessComplete && b.Completeness != GraphCompletenessPartial) {
		return observationErr("GPH_OBS_CORE_INVALID", "batch", "invalid schema/backend/completeness")
	}
	if !obsBounded(b.BackendVersion) || !obsBounded(b.ExtractorVersion) || b.ProjectOrModule == "" || !observationNonzero(b.SourceFingerprint) || !observationNonzero(b.ConfigFingerprint) {
		return observationErr("GPH_OBS_PROVENANCE_MISSING", "batch", "invalid provenance")
	}
	seen := map[string]bool{}
	nodes := map[string]GraphObservationNode{}
	edges := map[string]GraphObservationEdge{}
	caps := map[string]GraphObservationCapability{}
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
		k, e := NewNodeKey(n.Key)
		if e != nil || !reflect.DeepEqual(k, n.Key) || n.Key.RepositoryIdentity != b.RepositoryIdentity || n.Key.BackendType != b.Backend || n.Key.ProjectOrModule != b.ProjectOrModule || !obsBounded(n.DisplayName) || !observationNonzero(n.SourceDigest) || !observationClaim(n.ClaimStatus) || !observationResolution(n.Resolution) {
			return observationErr("GPH_OBS_NODE_INVALID", "nodes", "node violates canonical batch or matrix")
		}
		nodes[n.Ref] = n
	}
	for _, c := range b.Capabilities {
		if c.Backend != b.Backend || !observationCapability(c.Capability) || !observationState(c.State) || caps[c.Capability].Capability != "" {
			return observationErr("GPH_OBS_CAPABILITY_INVALID", "capabilities", "duplicate or invalid capability")
		}
		if c.State == GraphObservationStatusStable && !stableCapabilityAllowed(b.Backend, c.Capability) {
			return observationErr("GPH_OBS_CAPABILITY_INVALID", "capabilities", "stable capability is outside the closed matrix")
		}
		caps[c.Capability] = c
	}
	for _, n := range b.Nodes {
		c, ok := capabilityState(caps, "declarations")
		if !ok || !nodeAllowed(b.Backend, "declarations", n.ClaimStatus, n.Resolution, c.State) || n.Resolution == "lexical" && n.Key.SymbolKind != "file" && n.Key.SymbolKind != "document" {
			return observationErr("GPH_OBS_NODE_INVALID", "nodes", "node claim is not allowed by declarations capability")
		}
	}
	for _, e := range b.Edges {
		if x := add(e.Ref); x != nil {
			return x
		}
		if !observationRelation(e.Relation) || !observationScope(e.Scope) || !observationClaim(e.Status) || e.Backend != b.Backend || !observationResolution(e.Resolution) || !observationNonzero(e.SourceDigest) {
			return observationErr("GPH_OBS_EDGE_INVALID", "edges", "invalid edge")
		}
		if _, ok := nodes[e.FromRef]; !ok {
			return observationErr("GPH_OBS_ENDPOINT_UNRESOLVED", "edges", "missing endpoint must be unresolved")
		}
		if _, ok := nodes[e.ToRef]; !ok {
			return observationErr("GPH_OBS_ENDPOINT_UNRESOLVED", "edges", "missing endpoint must be unresolved")
		}
		c, ok := caps[e.Relation]
		if !ok || !edgeAllowed(b.Backend, e.Relation, e.Status, e.Resolution, c.State) {
			return observationErr("GPH_OBS_RELATION_GATE", "edges", "edge claim is not allowed by capability matrix")
		}
		edges[e.Ref] = e
	}
	for _, c := range b.Coverage {
		if c.Backend != b.Backend || !observationCapability(c.Capability) || c.Eligible < 0 || c.Observed < 0 || c.Unresolved < 0 || c.Omitted < 0 || cov[c.Capability].Capability != "" || !func() bool { _, ok := caps[c.Capability]; return ok }() {
			return observationErr("GPH_OBS_COVERAGE_INVALID", "coverage", "duplicate, undeclared, or negative coverage")
		}
		cov[c.Capability] = c
	}
	for k := range caps {
		if _, ok := cov[k]; !ok {
			return observationErr("GPH_OBS_COVERAGE_MISSING", "coverage", "capability lacks coverage")
		}
	}
	actual := map[string][3]int{}
	actual["declarations"] = [3]int{len(nodes), 0, 0}
	for _, e := range edges {
		x := actual[e.Relation]
		x[0]++
		actual[e.Relation] = x
	}
	for _, u := range b.Unresolved {
		x := actual[u.Capability]
		x[1]++
		actual[u.Capability] = x
	}
	for _, o := range b.Omissions {
		x := actual[o.Capability]
		x[2]++
		actual[o.Capability] = x
	}
	for k, c := range cov {
		a := actual[k]
		if c.Observed != a[0] || c.Unresolved != a[1] || c.Omitted != a[2] || c.Eligible < a[0]+a[1]+a[2] || (b.Completeness == GraphCompletenessComplete && c.Eligible != a[0]+a[1]+a[2]) {
			return observationErr("GPH_OBS_COVERAGE_INVALID", "coverage", "coverage is not tied to actual content")
		}
	}
	for _, c := range b.Capabilities {
		if c.State != GraphObservationStatusStable {
			found := false
			for _, o := range b.Omissions {
				if o.Capability == c.Capability {
					found = true
				}
			}
			if !found {
				return observationErr("GPH_OBS_OMISSION_MISSING", "capabilities", "non-stable capability requires typed omission")
			}
		}
	}
	for _, v := range b.Evidence {
		if e := add(v.Ref); e != nil {
			return e
		}
		if (v.NodeRef == "") == (v.EdgeRef == "") || v.Backend != b.Backend || v.ExtractorVersion != b.ExtractorVersion || !observationNonzero(v.SourceDigest) || !observationNonzero(v.ObservedDigest) || !observationClaim(v.Status) {
			return observationErr("GPH_OBS_EVIDENCE_INVALID", "evidence", "invalid evidence")
		}
		var owner string
		var digest GraphDigest
		var claim string
		if v.NodeRef != "" {
			n, ok := nodes[v.NodeRef]
			if !ok {
				return observationErr("GPH_OBS_EVIDENCE_INVALID", "evidence", "unknown target")
			}
			owner = n.Key.OwnerPath
			digest = n.SourceDigest
			claim = "declaration"
		} else {
			e, ok := edges[v.EdgeRef]
			if !ok {
				return observationErr("GPH_OBS_EVIDENCE_INVALID", "evidence", "unknown target")
			}
			owner = e.OwnerPath
			digest = e.SourceDigest
			claim = e.Relation
		}
		if v.ClaimKind != claim || v.Status != func() string {
			if v.NodeRef != "" {
				return nodes[v.NodeRef].ClaimStatus
			}
			return edges[v.EdgeRef].Status
		}() || v.SourceURI != owner || v.SourceDigest != digest {
			return observationErr("GPH_OBS_EVIDENCE_INVALID", "evidence", "claim source or status mismatch")
		}
		if v.Range == nil || v.Range.StartLine < 1 || v.Range.StartColumn < 1 || v.Range.EndLine < v.Range.StartLine || v.Range.EndColumn < 1 || (v.Range.EndLine == v.Range.StartLine && v.Range.EndColumn < v.Range.StartColumn) {
			return observationErr("GPH_OBS_RANGE_INVALID", "evidence.range", "positive ordered range required")
		}
	}
	for _, n := range nodes {
		found := false
		for _, v := range b.Evidence {
			if v.NodeRef == n.Ref {
				found = true
			}
		}
		if !found {
			return observationErr("GPH_OBS_EVIDENCE_MISSING", "nodes", "node requires evidence")
		}
	}
	for _, edge := range edges {
		found := false
		for _, v := range b.Evidence {
			if v.EdgeRef == edge.Ref {
				found = true
			}
		}
		if !found {
			return observationErr("GPH_OBS_EVIDENCE_MISSING", "edges", "edge requires evidence")
		}
	}
	for _, u := range b.Unresolved {
		if e := add(u.Ref); e != nil {
			return e
		}
		if u.Backend != b.Backend || u.OwnerPath == "" || !observationCapability(u.Capability) || !observationNonzero(u.SelectorDigest) || !stableCode(u.ReasonCode) || !stableCode(u.RecoveryHintCode) || len(u.Candidates) > 64 {
			return observationErr("GPH_OBS_UNRESOLVED_INVALID", "unresolved", "typed unresolved required")
		}
		if _, ok := caps[u.Capability]; !ok {
			return observationErr("GPH_OBS_UNRESOLVED_INVALID", "unresolved", "capability is not registered")
		}
		if _, ok := registeredGraphValues["symbol_kind"][u.SubjectKind]; !ok {
			return observationErr("GPH_OBS_UNRESOLVED_INVALID", "unresolved", "subject kind is not registered")
		}
		total := 0
		for _, c := range u.Candidates {
			if !obsBounded(c) {
				return observationErr("GPH_OBS_UNRESOLVED_INVALID", "candidates", "candidate is not bounded")
			}
			total += len(c)
		}
		if total > 4096 {
			return observationErr("GPH_OBS_UNRESOLVED_INVALID", "candidates", "candidate payload too large")
		}
		if u.SourceDigest != nil && !observationNonzero(*u.SourceDigest) {
			return observationErr("GPH_OBS_UNRESOLVED_INVALID", "source_digest", "invalid source digest")
		}
	}
	for _, o := range b.Omissions {
		if e := add(o.Ref); e != nil {
			return e
		}
		if o.Backend != b.Backend || o.OwnerPath == "" || !observationCapability(o.Capability) || !stableCode(o.ReasonCode) || !stableCode(o.RecoveryHintCode) {
			return observationErr("GPH_OBS_OMISSION_INVALID", "omissions", "typed omission required")
		}
		if _, ok := caps[o.Capability]; !ok {
			return observationErr("GPH_OBS_OMISSION_INVALID", "omissions", "capability is not registered")
		}
		if _, ok := registeredGraphValues["symbol_kind"][o.SubjectKind]; !ok {
			return observationErr("GPH_OBS_OMISSION_INVALID", "omissions", "subject kind is not registered")
		}
	}
	if b.Completeness == GraphCompletenessPartial && len(b.Unresolved) == 0 && len(b.Omissions) == 0 {
		return observationErr("GPH_OBS_PARTIAL_REASON_MISSING", "completeness", "partial batches require unresolved or omission")
	}
	if b.ResourceStats != nil && (b.ResourceStats.ElapsedMilliseconds < 0 || b.ResourceStats.ElapsedMilliseconds > 24*60*60*1000 || b.ResourceStats.RSSBytes < 0 || b.ResourceStats.RSSBytes > (1<<50)) {
		return observationErr("GPH_OBS_RESOURCE_INVALID", "resource_stats", "resource stats out of bounds")
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
	if b == nil {
		return ErrGraphObservationNotStageable
	}
	if e := b.Validate(); e != nil {
		return e
	}
	if b.Completeness != GraphCompletenessComplete || (b.Backend != "roslyn" && b.Backend != "go") {
		return fmt.Errorf("%w: incomplete or unsupported backend", ErrGraphObservationNotStageable)
	}
	for _, c := range b.Capabilities {
		if c.State != GraphObservationStatusStable {
			return fmt.Errorf("%w: capability not stable", ErrGraphObservationNotStageable)
		}
	}
	if c, ok := func() (GraphObservationCoverage, bool) {
		for _, x := range b.Coverage {
			if x.Capability == "declarations" {
				return x, true
			}
		}
		return GraphObservationCoverage{}, false
	}(); !ok || c.Observed != len(b.Nodes) {
		return fmt.Errorf("%w: declarations coverage required", ErrGraphObservationNotStageable)
	}
	return nil
}
