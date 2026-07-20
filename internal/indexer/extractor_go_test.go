package indexer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestExtractGoSymbols(t *testing.T) {
	repo := model.WorkspaceRepo{ID: "main", Name: "main"}
	content := []byte(`package workspace

// Registry keeps workspace aliases.
type Registry struct{}

// Loader defines registry loading.
type Loader interface {
	Load() error
}

const DefaultName = "mi-lsp"

var currentName = DefaultName

// Add registers a workspace.
func Add(name string) error {
	return nil
}

// Remove unregisters a workspace.
func (r *Registry) Remove(name string) error {
	return nil
}
`)

	items := extractGo(repo, "internal/workspace/registry.go", "hash", content)
	assertGoSymbol(t, items, "Registry", "struct", "", "public")
	assertGoSymbol(t, items, "Loader", "interface", "", "public")
	assertGoSymbol(t, items, "DefaultName", "const", "", "public")
	assertGoSymbol(t, items, "currentName", "var", "", "package")
	assertGoSymbol(t, items, "Add", "function", "", "public")
	assertGoSymbol(t, items, "Remove", "method", "Registry", "public")
}

func TestObserveGoGraphCleanModule(t *testing.T) {
	root := t.TempDir()
	writeGoTestFile(t, root, "go.mod", "module example.com/graph\n\ngo 1.24.4\n")
	writeGoTestFile(t, root, "main.go", `package graph

import (
	"fmt"
	"example.com/graph/sub"
)

type Box[T any] struct { Value T }
func (b *Box[T]) Ptr() T { return b.Value }
func (b Box[T]) Val() T { return b.Value }
func Use() { b := Box[int]{Value: 1}; _ = b.Ptr(); _ = b.Val(); sub.G(); fmt.Println(b.Value) }
`)
	writeGoTestFile(t, root, filepath.Join("sub", "sub.go"), "package sub\nfunc G() {}\n")
	req := GoGraphObservationRequest{Root: root, RepositoryIdentity: "https://example.com/graph", ProjectOrModule: "go.mod"}
	batch, err := ObserveGoGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if err := batch.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := batch.ReadyForStaging(); err != nil {
		t.Fatal(err)
	}
	if batch.Completeness != model.GraphCompletenessComplete {
		t.Fatalf("completeness = %q", batch.Completeness)
	}
	if len(batch.Nodes) == 0 || len(batch.Edges) == 0 {
		t.Fatalf("empty graph: nodes=%d edges=%d", len(batch.Nodes), len(batch.Edges))
	}
	if !hasGoGraphEdge(batch, "imports") || !hasGoGraphEdge(batch, "contains") || !hasGoGraphEdge(batch, "calls") {
		t.Fatalf("missing expected relations: %#v", batch.Coverage)
	}
	for _, n := range batch.Nodes {
		if strings.Contains(n.Key.OwnerPath, string(filepath.Separator)+"tmp") || filepath.IsAbs(n.Key.OwnerPath) {
			t.Fatalf("absolute/raw node path: %q", n.Key.OwnerPath)
		}
	}
}

func TestObserveGoGraphDeterministicAndCancellation(t *testing.T) {
	root := t.TempDir()
	writeGoTestFile(t, root, "go.mod", "module example.com/deterministic\n\ngo 1.24.4\n")
	writeGoTestFile(t, root, "main.go", "package deterministic\nfunc F() {}\n")
	req := GoGraphObservationRequest{Root: root, RepositoryIdentity: "https://example.com/deterministic", ProjectOrModule: "go.mod"}
	first, err := ObserveGoGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ObserveGoGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest || first.SourceFingerprint != second.SourceFingerprint || first.ConfigFingerprint != second.ConfigFingerprint {
		t.Fatal("repeated observation is not deterministic")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	partial, err := ObserveGoGraph(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if partial.Completeness != model.GraphCompletenessPartial || partial.Validate() != nil || partial.ReadyForStaging() == nil {
		t.Fatal("cancelled observation gates are invalid")
	}
	if _, err := ObserveGoGraph(context.Background(), GoGraphObservationRequest{Root: root, RepositoryIdentity: req.RepositoryIdentity, ProjectOrModule: "missing.go.mod"}); err == nil {
		t.Fatal("missing go.mod was accepted")
	}
}

func TestObserveGoGraphSemanticIdentityStability(t *testing.T) {
	root := t.TempDir()
	writeGoTestFile(t, root, "go.mod", `module example.com/stable

go 1.24.4
`)
	firstSource := `package stable

type Box[T any] struct { Value T }
const Default = 1
func (b *Box[T]) Ptr() T { return b.Value }
func Use() int { return Default }
`
	writeGoTestFile(t, root, "main.go", firstSource)
	req := GoGraphObservationRequest{Root: root, RepositoryIdentity: "https://example.com/stable", ProjectOrModule: "go.mod"}
	first, err := ObserveGoGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	writeGoTestFile(t, root, "main.go", `package stable

type Box[T any] struct { Value T; Extra string }
const Default = 2
func (receiver *Box[T]) Ptr() T { return receiver.Value }
func Use() int { return Default + 1 }
`)
	second, err := ObserveGoGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	for _, identity := range []string{"type:example.com/stable:Box", "field:example.com/stable:Default", "method:example.com/stable:Box:pointer:Ptr", "func:example.com/stable:Use"} {
		before, ok := graphNodeByIdentity(first, identity)
		if !ok {
			t.Fatalf("missing first identity %s", identity)
		}
		after, ok := graphNodeByIdentity(second, identity)
		if !ok {
			t.Fatalf("missing second identity %s", identity)
		}
		if before.Ref != after.Ref {
			t.Fatalf("identity %s changed ref", identity)
		}
		if before.SourceDigest == after.SourceDigest {
			t.Fatalf("identity %s did not record source change", identity)
		}
	}
	method, ok := graphNodeByIdentity(first, "method:example.com/stable:Box:pointer:Ptr")
	if !ok || method.Key.SymbolKind != "method" || method.Key.SemanticIdentity == "" {
		t.Fatal("generic pointer method was not a method node")
	}
}

func graphNodeByIdentity(batch model.GraphObservationBatch, identity string) (model.GraphObservationNode, bool) {
	for _, node := range batch.Nodes {
		if node.Key.SemanticIdentity == identity {
			return node, true
		}
	}
	return model.GraphObservationNode{}, false
}

func writeGoTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestObserveGoGraphAcceptanceMatrix(t *testing.T) {
	root := t.TempDir()
	writeGoTestFile(t, root, "go.mod", "module example.com/matrix\n\ngo 1.24.4\n")
	writeGoTestFile(t, root, "main.go", `package matrix

import (
    "example.com/matrix/alpha"
    "example.com/matrix/beta"
)

type Box[T any] struct { Value T }
func (b *Box[T]) Ptr() T { return b.Value }
func (b Box[T]) Val() T { return b.Value }
type Runner interface { Run() error }
const Default = 1
var Current = Default
func Run() error { b := Box[int]{Value: 1}; _ = b.Ptr(); _ = b.Val(); alpha.G(); alpha.G(); beta.G(); return nil }
`)
	writeGoTestFile(t, root, filepath.Join("alpha", "alpha.go"), "package alpha\nfunc G() {}\n")
	writeGoTestFile(t, root, filepath.Join("beta", "beta.go"), "package beta\nfunc G() {}\n")
	req := GoGraphObservationRequest{Root: root, RepositoryIdentity: "https://example.com/matrix", ProjectOrModule: "go.mod"}
	batch, err := ObserveGoGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if err := batch.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := batch.ReadyForStaging(); err != nil {
		t.Fatal(err)
	}
	if batch.Completeness != model.GraphCompletenessComplete {
		t.Fatalf("completeness=%q", batch.Completeness)
	}
	want := []string{"type:example.com/matrix:Box", "method:example.com/matrix:Box:pointer:Ptr", "method:example.com/matrix:Box:value:Val", "method:example.com/matrix:Runner:Run", "field:example.com/matrix:Default", "field:example.com/matrix:Current", "func:example.com/matrix:Run", "func:example.com/matrix/alpha:G", "func:example.com/matrix/beta:G"}
	refs := map[string]string{}
	for _, n := range batch.Nodes {
		refs[n.Key.SemanticIdentity] = n.Ref
	}
	for _, identity := range want {
		if refs[identity] == "" {
			t.Fatalf("missing identity %s", identity)
		}
	}
	if refs["method:example.com/matrix:Box:pointer:Ptr"] == refs["method:example.com/matrix:Box:value:Val"] {
		t.Fatal("pointer/value methods collapsed")
	}
	for _, edge := range batch.Edges {
		if edge.Relation == "contains" {
			from, to := nodeByRef(batch, edge.FromRef), nodeByRef(batch, edge.ToRef)
			if from.Key.SymbolKind != "package" && from.Key.SymbolKind != "type" {
				t.Fatalf("contains source=%q", from.Key.SymbolKind)
			}
			if from.Key.SymbolKind == "type" && to.Key.SymbolKind == "package" {
				t.Fatal("type contains package")
			}
		}
	}
	alphaCalls := 0
	for _, edge := range batch.Edges {
		if edge.Relation == "calls" && edge.ToRef == refs["func:example.com/matrix/alpha:G"] {
			alphaCalls++
		}
	}
	if alphaCalls != 1 {
		t.Fatalf("deduplicated edge count=%d", alphaCalls)
	}
	for _, edge := range batch.Edges {
		if edge.Relation == "calls" && edge.ToRef == refs["func:example.com/matrix/alpha:G"] && evidenceForEdge(batch, edge.Ref) < 2 {
			t.Fatal("repeated call lost evidence")
		}
	}
	seen := map[string]bool{}
	for _, n := range batch.Nodes {
		if seen[n.Ref] {
			t.Fatalf("duplicate node ref %s", n.Ref)
		}
		seen[n.Ref] = true
	}
	for _, e := range batch.Edges {
		if !seen[e.FromRef] || !seen[e.ToRef] {
			t.Fatalf("dangling edge %s", e.Ref)
		}
	}
	for _, ev := range batch.Evidence {
		if (ev.NodeRef == "") == (ev.EdgeRef == "") || ev.ObservedDigest == ev.SourceDigest || ev.SourceURI == "" || ev.Range == nil {
			t.Fatalf("invalid evidence %#v", ev)
		}
	}
}

func TestGoGraphProvenanceAndConfigDomains(t *testing.T) {
	root := t.TempDir()
	writeGoTestFile(t, root, "go.mod", "module example.com/provenance\n\ngo 1.24.4\n")
	writeGoTestFile(t, root, "main.go", "package provenance\nfunc F() {}\n")
	req := GoGraphObservationRequest{Root: root, RepositoryIdentity: "example.com/provenance", ProjectOrModule: "go.mod", BuildTags: []string{"z", "a", "a"}, GOOS: "linux", GOARCH: "amd64"}
	first, err := ObserveGoGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		next, e := ObserveGoGraph(context.Background(), req)
		if e != nil || next.Digest != first.Digest {
			t.Fatalf("nondeterministic run %d: %v", i, e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	partial, err := ObserveGoGraph(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if partial.SourceFingerprint == (model.GraphDigest{}) || partial.ReadyForStaging() == nil {
		t.Fatal("cancelled batch was stageable or unbound")
	}
	before := first.Nodes
	writeGoTestFile(t, root, "main.go", "package provenance\nfunc F() { _ = 1 }\n")
	second, err := ObserveGoGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if second.SourceFingerprint == first.SourceFingerprint || second.Digest == first.Digest {
		t.Fatal("source mutation not reflected")
	}
	for _, n := range before {
		if after, ok := nodeByIdentity(second, n.Key.SemanticIdentity); ok && after.Ref != n.Ref {
			t.Fatalf("ref changed for %s", n.Key.SemanticIdentity)
		}
	}
	if goGraphConfigDigest([]byte("mod"), nil, "example.com/x", "a/go.mod", nil, nil, "linux", "amd64", "", "") == goGraphConfigDigest([]byte("mod"), nil, "example.com/x", "b/go.mod", nil, nil, "linux", "amd64", "", "") {
		t.Fatal("project path omitted from config domain")
	}
}

func TestGoGraphOwnedTypeErrorIsPartial(t *testing.T) {
	root := t.TempDir()
	writeGoTestFile(t, root, "go.mod", "module example.com/broken\n\ngo 1.24.4\n")
	writeGoTestFile(t, root, "main.go", "package broken\nfunc F() { Missing() }\n")
	batch, err := ObserveGoGraph(context.Background(), GoGraphObservationRequest{Root: root, RepositoryIdentity: "example.com/broken", ProjectOrModule: "go.mod"})
	if err != nil {
		t.Fatal(err)
	}
	if batch.Completeness != model.GraphCompletenessPartial || batch.ReadyForStaging() == nil {
		t.Fatal("type error was not sealed partial")
	}
	for _, o := range batch.Omissions {
		if o.ReasonCode == "type_check_error" && o.OwnerPath == "main.go" {
			return
		}
	}
	t.Fatal("missing typed type_check_error omission")
}

func nodeByRef(batch model.GraphObservationBatch, ref string) model.GraphObservationNode {
	for _, n := range batch.Nodes {
		if n.Ref == ref {
			return n
		}
	}
	return model.GraphObservationNode{}
}
func nodeByIdentity(batch model.GraphObservationBatch, identity string) (model.GraphObservationNode, bool) {
	for _, n := range batch.Nodes {
		if n.Key.SemanticIdentity == identity {
			return n, true
		}
	}
	return model.GraphObservationNode{}, false
}
func evidenceForEdge(batch model.GraphObservationBatch, ref string) int {
	count := 0
	for _, ev := range batch.Evidence {
		if ev.EdgeRef == ref {
			count++
		}
	}
	return count
}

func hasGoGraphEdge(batch model.GraphObservationBatch, relation string) bool {
	for _, edge := range batch.Edges {
		if edge.Relation == relation {
			return true
		}
	}
	return false
}

func assertGoSymbol(t *testing.T, items []model.SymbolRecord, name string, kind string, parent string, scope string) {
	t.Helper()
	for _, item := range items {
		if item.Name == name && item.Kind == kind {
			if item.Parent != parent {
				t.Fatalf("%s parent = %q, want %q", name, item.Parent, parent)
			}
			if item.Scope != scope {
				t.Fatalf("%s scope = %q, want %q", name, item.Scope, scope)
			}
			if item.Language != "go" {
				t.Fatalf("%s language = %q, want go", name, item.Language)
			}
			if item.SearchText == "" {
				t.Fatalf("%s search text is empty", name)
			}
			return
		}
	}
	t.Fatalf("symbol %s/%s not found in %#v", name, kind, items)
}
