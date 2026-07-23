package model

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func fixtureNode(t *testing.T) NodeKey {
	t.Helper()
	k, e := NewNodeKey(NodeKeyFields{RepositoryIdentity: "HTTPS://GitHub.COM/Org/Repo.git", BackendType: "Roslyn", Language: "csharp", ProjectOrModule: "src/./Core", OwnerPath: "src/Core/Thing.cs", SymbolKind: "Type", SemanticIdentity: "T:Org.Core.Thing"})
	if e != nil {
		t.Fatal(e)
	}
	return k
}
func TestCanonicalNodeVector(t *testing.T) {
	k := fixtureNode(t)
	p, e := k.Serialize()
	if e != nil {
		t.Fatal(e)
	}
	const wantPayload = "4d494c53502d4e4b01000701000000136769746875622e636f6d2f4f72672f5265706f0200000006726f736c796e030000000663736861727004000000087372632f436f726505000000117372632f436f72652f5468696e672e63730600000004747970650700000010543a4f72672e436f72652e5468696e67"
	if got := hex.EncodeToString(p); got != wantPayload {
		t.Fatalf("node payload: %s", got)
	}
	h, e := k.Hash()
	if e != nil {
		t.Fatal(e)
	}
	const wantHash = "16312301d3070c9beacdf83307c1467b8ce1ef4e88771698dd0291f8b59ad765"
	const wantRID = "milsp:gph-node:v1:16312301d3070c9beacdf83307c1467b8ce1ef4e88771698dd0291f8b59ad765"
	if h.String() != wantHash || NodeRID(h) != wantRID {
		t.Fatalf("node hash/RID: %s %s", h, NodeRID(h))
	}
}
func TestRepositoryAndPathCanonicalization(t *testing.T) {
	for _, v := range []string{"https://GitHub.COM/Org/Repo.git", "ssh://git@github.com/Org/Repo.git", "git@github.com:Org/Repo.git", "github.com:Org/Repo.git"} {
		got, e := NormalizeRepositoryIdentity(v)
		if e != nil || got != "github.com/Org/Repo" {
			t.Fatalf("canonicalize %q: %q %v", v, got, e)
		}
	}
	for _, v := range []string{"https://u:p@host/x.git", "/tmp/repo", "../repo", "https://host/repo?token=x", "https://host/", "https://host/repo\n"} {
		if _, e := NormalizeRepositoryIdentity(v); e == nil {
			t.Fatalf("accepted bad repo %q", v)
		}
	}
	for _, v := range []string{"a/../b", "/a", "a\\b", "a/\x00b", "a//b"} {
		if _, e := slashPath(v, "p"); e == nil {
			t.Fatalf("accepted bad path %q", v)
		}
	}
}
func TestNodeKeyEnumGrammarAndSemanticCase(t *testing.T) {
	base := fixtureNode(t)
	for _, field := range []string{"backend type", "roslyn/1", "roslyn!", "roslyné", "1roslyn"} {
		bad := base
		bad.BackendType = field
		if _, err := NewNodeKey(bad); err == nil {
			t.Fatalf("accepted invalid backend enum %q", field)
		} else {
			var ge *GraphError
			if !errors.As(err, &ge) || ge.Code != "GPH_IDENTITY_ENUM_INVALID" {
				t.Fatalf("wrong typed enum error: %v", err)
			}
		}
	}
	k, err := NewNodeKey(NodeKeyFields{RepositoryIdentity: "github.com/Org/Repo", BackendType: "Roslyn", Language: "CSharp", ProjectOrModule: "src/Core", OwnerPath: "src/Core/Thing.cs", SymbolKind: "Type", SemanticIdentity: "T:Org.Core.Café"})
	if err != nil || k.BackendType != "roslyn" || k.Language != "csharp" || k.SymbolKind != "type" || k.SemanticIdentity != "T:Org.Core.Café" {
		t.Fatalf("canonical enum or semantic identity: %#v %v", k, err)
	}
}

func TestRegisteredGraphTaxonomy(t *testing.T) {
	base := fixtureNode(t)
	for _, backend := range []string{"roslyn", "go", "tsserver", "pyright"} {
		k := base
		k.BackendType = backend
		if _, err := NewNodeKey(k); err != nil {
			t.Fatalf("registered backend %q rejected: %v", backend, err)
		}
	}
	for _, kind := range []string{"workspace", "repository", "project", "package", "file", "namespace", "type", "method", "function", "field", "property", "event", "route", "test", "document"} {
		k := base
		k.SymbolKind = kind
		if _, err := NewNodeKey(k); err != nil {
			t.Fatalf("registered kind %q rejected: %v", kind, err)
		}
	}
	for _, field := range []struct{ name, value string }{{"backend_type", "custom"}, {"symbol_kind", "var"}} {
		k := base
		if field.name == "backend_type" {
			k.BackendType = field.value
		} else {
			k.SymbolKind = field.value
		}
		if _, err := NewNodeKey(k); err == nil {
			t.Fatalf("unknown %s accepted", field.name)
		}
	}
	b := makeBundle(t)
	b.Edges[0].Relation = "unknown_relation"
	if err := b.Validate(); !errors.Is(err, ErrGraphEdgeInvalid) {
		t.Fatalf("unknown relation error=%v", err)
	}
}

func TestGraphErrorClasses(t *testing.T) {
	var e *GraphError
	err := graphErr("GPH_EDGE_ENDPOINT_MISSING", "edge", "missing")
	if !errors.As(err, &e) || !errors.Is(err, ErrGraphEdgeInvalid) || errors.Is(err, ErrNodeKeyInvalid) {
		t.Fatal("wrong edge unwrap")
	}
}
func makeBundle(t *testing.T) *GraphBundle {
	t.Helper()
	k := fixtureNode(t)
	d, _ := k.Hash()
	b := &GraphBundle{Generation: GraphGeneration{SchemaVersion: 1, WorkspaceIdentity: k.RepositoryIdentity, RepositoryIdentity: k.RepositoryIdentity, SourceFingerprint: d, ConfigFingerprint: d, BackendManifestDigest: d, Status: GraphGenerationActive, NodeCount: 1, EdgeCount: 1, EvidenceCount: 1, UnresolvedCount: 0}, Nodes: []GraphNodeRecord{{NodeID: 0, NodeKey: d, Identity: k, IdentitySchema: "milsp-node-key/v1", ClaimStatus: GraphRecordExact, CrossRID: NodeRID(d), GenerationID: GraphDigest{}}}, Edges: []GraphEdgeRecord{{EdgeID: 0, FromNodeID: 0, ToNodeID: 0, Relation: "calls", ClaimScope: "symbol", ClaimStatus: GraphRecordExact, OwnerPath: k.OwnerPath, SourceBackend: "roslyn", GenerationID: GraphDigest{}}}, Evidence: []GraphEvidence{{EvidenceID: 0, SubjectKind: "edge", EdgeID: intp(0), SourceURI: "src/Core/Thing.cs", Backend: "roslyn", ExtractorVersion: "v1", SourceDigest: d, ClaimKind: "calls", ObservedClaimDigest: d, ClaimStatus: GraphRecordExact, GenerationID: GraphDigest{}}}}
	b.Edges[0].EdgeKey = EdgeKey(d, d, "calls", "symbol")
	b.Edges[0].CrossRID = EdgeRID(b.Edges[0].EdgeKey)
	b.Evidence[0].EvidenceDigest = EvidenceDigest(d, d, b.Evidence[0].SourceURI, b.Evidence[0].ClaimKind, b.Evidence[0].Backend, b.Evidence[0].ExtractorVersion, 0, 0, 0, 0)
	b.Evidence[0].EvidenceKey = EvidenceKey(b.Edges[0].EdgeKey, b.Evidence[0].EvidenceDigest, 0)
	b.Evidence[0].CrossRID = EvidenceRID(b.Evidence[0].EvidenceKey)
	if e := b.SealIDs(); e != nil {
		t.Fatal(e)
	}
	return b
}
func intp(v int) *int { return &v }
func TestInferredEdgeRequiresEvidence(t *testing.T) {
	b := makeBundle(t)
	b.Edges[0].ClaimStatus = GraphRecordInferred
	b.Evidence = nil
	b.Generation.EvidenceCount = 0
	if err := b.Validate(); !errors.Is(err, ErrGraphEvidenceInvalid) {
		t.Fatalf("inferred edge without evidence: %v", err)
	}
}

func TestBundleValidationAndTamper(t *testing.T) {
	b := makeBundle(t)
	if e := b.Validate(); e != nil {
		t.Fatal(e)
	}
	b.Nodes[0].DisplayName = "tampered"
	if e := b.Validate(); e == nil {
		t.Fatal("accepted content tamper")
	}
}
func TestOrderAndTimestampIndependence(t *testing.T) {
	b := makeBundle(t)
	a, _ := b.ContentDigest()
	before := append([]GraphNodeRecord(nil), b.Nodes...)
	b.Generation.CreatedAt = b.Generation.CreatedAt.AddDate(3, 0, 0)
	c, _ := b.ContentDigest()
	if a != c || !reflect.DeepEqual(before, b.Nodes) {
		t.Fatal("content depends on timestamp or mutates input")
	}
	b.Nodes = append([]GraphNodeRecord(nil), b.Nodes...)
	b.Nodes[0].DisplayName = strings.TrimSpace(b.Nodes[0].DisplayName)
	if e := b.Validate(); e != nil {
		t.Fatal(e)
	}
}
func TestGraphSerializationGoldens(t *testing.T) {
	b := makeBundle(t)
	content, e := b.ContentDigest()
	if e != nil || content.String() != "a56a90a65f5076a5e95dcb0895688f20967831d52d6d819900508defc446c9e2" {
		t.Fatalf("content golden: %v %v", content, e)
	}
	raw, e := generationBytes(b.Generation.SchemaVersion, b.Generation.RepositoryIdentity, b.Generation.SourceFingerprint, b.Generation.ConfigFingerprint, b.Generation.BackendManifestDigest, content)
	const wantGenerationBytes = "4d494c53502d47454e45524154494f4e2f76320100000008000000000000000102000000136769746875622e636f6d2f4f72672f5265706f030000002016312301d3070c9beacdf83307c1467b8ce1ef4e88771698dd0291f8b59ad765040000002016312301d3070c9beacdf83307c1467b8ce1ef4e88771698dd0291f8b59ad765050000002016312301d3070c9beacdf83307c1467b8ce1ef4e88771698dd0291f8b59ad7650600000020a56a90a65f5076a5e95dcb0895688f20967831d52d6d819900508defc446c9e2"
	const wantGenerationHash = "7d99f92733cba36abdd2c248a1a36d3160273f36866683939eee1c285e159107"
	if e != nil || hex.EncodeToString(raw) != wantGenerationBytes || digestBytes(raw).String() != wantGenerationHash {
		t.Fatal("generation golden")
	}
}
func TestGraphValidationRejectsMissingEdgeEndpoints(t *testing.T) {
	b := makeBundle(t)
	b.Nodes = nil
	b.Generation.NodeCount = 0
	if err := b.Validate(); !errors.Is(err, ErrGraphEdgeInvalid) {
		t.Fatalf("zero-value missing endpoint error=%v", err)
	}

	b = makeBundle(t)
	b.Edges[0].ToNodeID = 1
	if err := b.Validate(); !errors.Is(err, ErrGraphEdgeInvalid) {
		t.Fatalf("out-of-range endpoint error=%v", err)
	}
}

func TestGraphValidationRejectsDuplicateAndTypedFailures(t *testing.T) {
	b := makeBundle(t)
	dup := b.Nodes[0]
	dup.NodeID = 1
	b.Nodes = append(b.Nodes, dup)
	b.Generation.NodeCount++
	if e := b.Validate(); !errors.Is(e, ErrNodeKeyInvalid) {
		t.Fatalf("duplicate node: %v", e)
	}
	b = makeBundle(t)
	b.Generation.WorkspaceIdentity = "C:/absolute/root"
	if e := b.Validate(); !errors.Is(e, ErrGraphGenerationInvalid) {
		t.Fatalf("workspace metadata: %v", e)
	}
	b = makeBundle(t)
	b.Evidence[0].Backend = ""
	if e := b.Validate(); !errors.Is(e, ErrGraphEvidenceInvalid) {
		t.Fatalf("evidence error: %v", e)
	}
	b = makeBundle(t)
	b.Unresolved = append(b.Unresolved, GraphUnresolved{UnresolvedID: 0})
	b.Generation.UnresolvedCount++
	if e := b.Validate(); !errors.Is(e, ErrGraphUnresolved) {
		t.Fatalf("unresolved error: %v", e)
	}
}
func TestGenerationExcludesWorkspaceMetadata(t *testing.T) {
	b := makeBundle(t)
	got := DeriveGenerationID(b.Generation.SchemaVersion, b.Generation.RepositoryIdentity, b.Generation.SourceFingerprint, b.Generation.ConfigFingerprint, b.Generation.BackendManifestDigest, b.Generation.ContentDigest)
	if got != b.Generation.GenerationID {
		t.Fatal("generation ID changed with metadata")
	}
}
func TestGraphUnresolvedKeyGoldenAndBounds(t *testing.T) {
	u := GraphUnresolved{OwnerPath: "src/main.go", SubjectKind: "file", SelectorDigest: digestBytes([]byte("selector")), ReasonCode: "missing_symbol", Candidates: []string{"a", "b"}, Backend: "go", RecoveryHintCode: "retry"}
	u.UnresolvedKey = GraphUnresolvedKey(u)
	u.CrossRID = UnresolvedRID(u.UnresolvedKey)
	if err := ValidateGraphUnresolved(u); err != nil {
		t.Fatal(err)
	}
	if GraphUnresolvedKey(u) != u.UnresolvedKey {
		t.Fatal("unresolved key is not stable")
	}
	tooMany := u
	tooMany.Candidates = make([]string, maxUnresolvedCandidates+1)
	tooMany.UnresolvedKey = GraphUnresolvedKey(tooMany)
	tooMany.CrossRID = UnresolvedRID(tooMany.UnresolvedKey)
	if err := ValidateGraphUnresolved(tooMany); !errors.Is(err, ErrGraphUnresolved) {
		t.Fatalf("accepted too many candidates: %v", err)
	}
}

func TestJSONTagsRoundTrip(t *testing.T) {
	g := GraphGeneration{NodeCount: 1, EdgeCount: 2, EvidenceCount: 3, UnresolvedCount: 4, ErrorCode: "x"}
	raw, e := json.Marshal(g)
	if e != nil {
		t.Fatal(e)
	}
	var got GraphGeneration
	if e = json.Unmarshal(raw, &got); e != nil || got.ErrorCode != "x" || got.EdgeCount != 2 {
		t.Fatal("json roundtrip")
	}
}

func TestGraphContentHasherMatchesBundle(t *testing.T) {
	b := makeBundle(t)
	want, err := b.ContentDigest()
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewGraphContentHasher(len(b.Nodes), len(b.Edges), len(b.Evidence), len(b.Unresolved))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range b.Nodes {
		if err = h.AddNode(n); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range b.Edges {
		if err = h.AddEdge(e); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range b.Evidence {
		if err = h.AddEvidence(e); err != nil {
			t.Fatal(err)
		}
	}
	got, err := h.Sum()
	if err != nil || got != want {
		t.Fatalf("stream digest: %v %v", got, err)
	}
}
func TestGraphContentHasherZeroSections(t *testing.T) {
	h, err := NewGraphContentHasher(0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.Sum(); err != nil {
		t.Fatal(err)
	}
}
func TestGraphContentHasherRejectsOrderAndCounts(t *testing.T) {
	b := makeBundle(t)
	h, err := NewGraphContentHasher(1, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = h.AddEdge(b.Edges[0]); err == nil {
		t.Fatal("accepted edge before node")
	}
	if err = h.AddNode(b.Nodes[0]); err != nil {
		t.Fatal(err)
	}
	if err = h.AddNode(b.Nodes[0]); err == nil {
		t.Fatal("accepted excess node")
	}
	if _, err = h.Sum(); err == nil {
		t.Fatal("accepted missing edge")
	}
	h, err = NewGraphContentHasher(0, 0, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.Sum(); err == nil {
		t.Fatal("accepted missing evidence")
	}
}
