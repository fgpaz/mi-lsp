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

func TestGoGraphListDisablesVCSStamping(t *testing.T) {
	args := goGraphListArgs(nil)
	if !strings.Contains(" "+strings.Join(args, " ")+" ", " -buildvcs=false ") {
		t.Fatalf("go list args must disable VCS stamping in worktrees: %v", args)
	}
}

func TestGoGraphListEnvSupportsAutoToolchains(t *testing.T) {
	env := goGraphListEnv("linux", "amd64")
	joined := "\n" + strings.Join(env, "\n") + "\n"
	for _, want := range []string{"GOPROXY=off", "GOSUMDB=sum.golang.org", "GOTOOLCHAIN=auto", "GOOS=linux", "GOARCH=amd64"} {
		if !strings.Contains(joined, "\n"+want+"\n") {
			t.Fatalf("go list env must contain %q: %v", want, env)
		}
	}
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

func TestObserveGoGraphSharesExportImporterIdentity(t *testing.T) {
	root := t.TempDir()
	writeGoTestFile(t, root, "go.mod", "module example.com/sharedimporter\n\ngo 1.24.4\n")
	writeGoTestFile(t, root, "main.go", `package sharedimporter

import (
	"context"
	"time"
)

func F() (context.Context, time.Duration) {
	return context.Background(), time.Second
}
`)
	batch, err := ObserveGoGraph(context.Background(), GoGraphObservationRequest{
		Root:               root,
		RepositoryIdentity: "https://example.com/sharedimporter",
		ProjectOrModule:    "go.mod",
	})
	if err != nil {
		t.Fatal(err)
	}
	if batch.Completeness != model.GraphCompletenessComplete {
		t.Fatalf("completeness = %q, omissions = %#v, unresolved = %#v", batch.Completeness, batch.Omissions, batch.Unresolved)
	}
	if err := batch.ReadyForStaging(); err != nil {
		t.Fatalf("shared standard-library imports made graph unstageable: %v", err)
	}
}

func TestObserveGoGraphLocalTargetsAreUnsupported(t *testing.T) {
	root := t.TempDir()
	writeGoTestFile(t, root, "go.mod", "module example.com/localtargets\n\ngo 1.24.4\n")
	writeGoTestFile(t, root, "main.go", `package localtargets

type Top struct{ Value int }

const TopConst = 2
var TopVar = 3

func Use() int {
	type Local struct{ Value int }
	var local Local
	const localConst = 4
	return local.Value + localConst + TopConst + TopVar
}
`)
	batch, err := ObserveGoGraph(context.Background(), GoGraphObservationRequest{
		Root:               root,
		RepositoryIdentity: "https://example.com/localtargets",
		ProjectOrModule:    "go.mod",
	})
	if err != nil {
		t.Fatal(err)
	}
	if batch.Completeness != model.GraphCompletenessComplete {
		t.Fatalf("local declarations made graph partial: omissions=%#v unresolved=%#v", batch.Omissions, batch.Unresolved)
	}
	if err := batch.ReadyForStaging(); err != nil {
		t.Fatal(err)
	}
	for _, unresolved := range batch.Unresolved {
		if unresolved.ReasonCode == "local_target_missing_ref" {
			t.Fatalf("local declaration was treated as an eligible missing target: %#v", unresolved)
		}
	}
	if !hasOmission(batch, "references", "unsupported_symbol_kind") {
		t.Fatalf("expected unsupported local target omission: %#v", batch.Omissions)
	}
}

func TestObserveGoGraphTopLevelEmbeddedFieldRemainsUnresolved(t *testing.T) {
	root := t.TempDir()
	writeGoTestFile(t, root, "go.mod", "module example.com/topleveltarget\n\ngo 1.24.4\n")
	writeGoTestFile(t, root, "main.go", `package topleveltarget

type Embedded struct{ Value int }
type Top struct{ Embedded }

func Use() int {
	var top Top
	return top.Embedded.Value
}
`)
	batch, err := ObserveGoGraph(context.Background(), GoGraphObservationRequest{
		Root:               root,
		RepositoryIdentity: "https://example.com/topleveltarget",
		ProjectOrModule:    "go.mod",
	})
	if err != nil {
		t.Fatal(err)
	}
	if batch.Completeness != model.GraphCompletenessPartial || batch.ReadyForStaging() == nil {
		t.Fatalf("missing top-level field endpoint was not preserved: completeness=%q omissions=%#v unresolved=%#v", batch.Completeness, batch.Omissions, batch.Unresolved)
	}
	if !hasOmission(batch, "declarations", "embedded_field_unsupported") || !hasUnresolved(batch, "references", "local_target_missing_ref") {
		t.Fatalf("missing top-level field diagnostics: omissions=%#v unresolved=%#v", batch.Omissions, batch.Unresolved)
	}
}

func hasUnresolved(batch model.GraphObservationBatch, capability, reason string) bool {
	for _, unresolved := range batch.Unresolved {
		if unresolved.Capability == capability && unresolved.ReasonCode == reason {
			return true
		}
	}
	return false
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

func TestGoGraphContractInvariants(t *testing.T) {
	root := t.TempDir()
	writeGoTestFile(t, root, "go.mod", "module example.com/contract\n\ngo 1.24.4\n")
	writeGoTestFile(t, root, "main.go", `package contract
import "example.com/contract/sub"
type Box struct{ Value int }
func (b Box) M() { sub.G() }
`)
	writeGoTestFile(t, root, "sub/sub.go", "package sub\nfunc G() {}\n")
	batch, err := ObserveGoGraph(context.Background(), GoGraphObservationRequest{Root: root, RepositoryIdentity: "example.com/contract", ProjectOrModule: "go.mod"})
	if err != nil {
		t.Fatal(err)
	}
	assertGoGraphContract(t, batch)
	if batch.ReadyForStaging() != nil {
		t.Fatal("clean batch is not ready")
	}
	if !hasEvidenceAtLine(batch, "main.go", 2) {
		t.Fatal("ImportSpec range missing")
	}
	for _, edge := range batch.Edges {
		from, to := nodeByRef(batch, edge.FromRef), nodeByRef(batch, edge.ToRef)
		if edge.Relation == "contains" && (from.Key.SymbolKind == "package" && to.Key.SymbolKind == "method" || from.Key.SymbolKind == "package" && to.Key.SymbolKind == "field") {
			t.Fatalf("non-immediate contains %#v", edge)
		}
	}
}

func TestGoGraphRelocationAndConfigSelection(t *testing.T) {
	one, two := t.TempDir(), t.TempDir()
	for _, root := range []string{one, two} {
		writeGoTestFile(t, root, "go.mod", "module example.com/relocate\n\ngo 1.24.4\n")
		writeGoTestFile(t, root, "main.go", "package relocate\nfunc F() {}\n")
		writeGoTestFile(t, root, "tagged.go", "//go:build special\npackage relocate\nfunc Tagged() {}\n")
	}
	req := GoGraphObservationRequest{RepositoryIdentity: "example.com/relocate", ProjectOrModule: "go.mod", BuildTags: []string{"special", "special"}}
	req.Root = one
	a, err := ObserveGoGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	req.Root = two
	b, err := ObserveGoGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if a.SourceFingerprint != b.SourceFingerprint || a.ConfigFingerprint != b.ConfigFingerprint || a.Digest != b.Digest {
		t.Fatal("relocation changed semantic batch")
	}
	req.Root = one
	req.BuildTags = nil
	plain, err := ObserveGoGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if plain.ConfigFingerprint == a.ConfigFingerprint || plain.Digest == a.Digest {
		t.Fatal("build tags did not affect config")
	}
	writeGoTestFile(t, one, "go.mod", "module example.com/relocate\n\ngo 1.24.4\n// config mutation\n")
	changed, err := ObserveGoGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if changed.ConfigFingerprint == plain.ConfigFingerprint {
		t.Fatal("go.mod mutation did not affect config")
	}
}

func TestGoGraphCgoIsSealedPartial(t *testing.T) {
	root := t.TempDir()
	writeGoTestFile(t, root, "go.mod", "module example.com/cgo\n\ngo 1.24.4\n")
	writeGoTestFile(t, root, "main.go", "package cgo\n/*\n#include <stdlib.h>\n*/\nimport \"C\"\nfunc F() { C.free(nil) }\n")
	batch, err := ObserveGoGraph(context.Background(), GoGraphObservationRequest{Root: root, RepositoryIdentity: "example.com/cgo", ProjectOrModule: "go.mod", GOOS: "js", GOARCH: "wasm"})
	if err != nil {
		t.Fatal(err)
	}
	if batch.Validate() != nil || batch.ReadyForStaging() == nil || batch.Completeness != model.GraphCompletenessPartial {
		t.Fatal("cgo was not sealed partial")
	}
	if !hasOmission(batch, "calls", "type_check_error") && !hasOmission(batch, "calls", "cgo_list_error") {
		t.Fatalf("missing typed cgo omission: %#v", batch.Omissions)
	}
}

func TestGoGraphExternalReplaceDoesNotLeakDependency(t *testing.T) {
	root := t.TempDir()
	writeGoTestFile(t, root, "go.mod", "module example.com/main\n\ngo 1.24.4\n\nrequire example.com/dep v0.0.0\nreplace example.com/dep => ./dep\n")
	writeGoTestFile(t, root, "main.go", "package main\nimport \"example.com/dep\"\nfunc F() { dep.G() }\n")
	writeGoTestFile(t, root, "dep/go.mod", "module example.com/dep\n\ngo 1.24.4\n")
	writeGoTestFile(t, root, "dep/dep.go", "package dep\nfunc G() {}\n")
	batch, err := ObserveGoGraph(context.Background(), GoGraphObservationRequest{Root: root, RepositoryIdentity: "example.com/main", ProjectOrModule: "go.mod"})
	if err != nil {
		t.Fatal(err)
	}
	assertGoGraphContract(t, batch)
	if batch.ReadyForStaging() != nil || batch.Completeness != model.GraphCompletenessComplete {
		t.Fatal("external replace main module was not complete")
	}
	for _, n := range batch.Nodes {
		if strings.Contains(n.Key.OwnerPath, "dep/") || strings.Contains(n.Key.SemanticIdentity, "example.com/dep") {
			t.Fatalf("external dependency leaked: %#v", n)
		}
	}
	if !hasOmission(batch, "calls", "external_target") && !hasOmission(batch, "references", "external_target") {
		t.Fatal("external target omission missing")
	}
}

func TestGoGraphCancelledAndTypedRequestErrors(t *testing.T) {
	root := t.TempDir()
	writeGoTestFile(t, root, "go.mod", "module example.com/cancel\n\ngo 1.24.4\n")
	writeGoTestFile(t, root, "main.go", "package cancel\nfunc F() {}\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	batch, err := ObserveGoGraph(ctx, GoGraphObservationRequest{Root: root, RepositoryIdentity: "example.com/cancel", ProjectOrModule: "go.mod"})
	if err != nil {
		t.Fatal(err)
	}
	if batch.Validate() != nil || batch.ReadyForStaging() == nil || batch.SourceFingerprint == (model.GraphDigest{}) || batch.ConfigFingerprint == (model.GraphDigest{}) {
		t.Fatal("cancelled batch contract invalid")
	}
	for _, cap := range []string{"declarations", "contains", "imports", "references", "calls"} {
		if !hasOmission(batch, cap, "cancelled") {
			t.Fatalf("missing cancelled %s", cap)
		}
	}
	for _, project := range []string{"", "missing.go.mod", "../go.mod", filepath.Join(root, "go.mod")} {
		_, err := ObserveGoGraph(context.Background(), GoGraphObservationRequest{Root: root, RepositoryIdentity: "example.com/cancel", ProjectOrModule: project})
		if err == nil {
			t.Fatalf("accepted invalid project %q", project)
		}
		if _, ok := err.(*model.GraphObservationError); !ok {
			t.Fatalf("untyped error %T", err)
		}
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
	for _, capability := range []string{"calls", "references"} {
		if !hasOmission(batch, capability, "type_check_error") {
			t.Fatalf("missing type_check_error %s", capability)
		}
	}
	for _, o := range batch.Omissions {
		if strings.Contains(o.OwnerPath, root) || strings.Contains(o.ReasonCode, "Missing") {
			t.Fatalf("raw diagnostic leaked: %#v", o)
		}
	}
}

func assertGoGraphContract(t *testing.T, batch model.GraphObservationBatch) {
	t.Helper()
	if err := batch.Validate(); err != nil {
		t.Fatal(err)
	}
	caps := map[string]bool{}
	for _, c := range batch.Capabilities {
		if c.Backend != "go" || c.State != model.GraphObservationStatusStable {
			t.Fatalf("bad capability %#v", c)
		}
		caps[c.Capability] = true
	}
	for _, cap := range []string{"declarations", "contains", "imports", "references", "calls"} {
		if !caps[cap] {
			t.Fatalf("missing capability %s", cap)
		}
	}
	nodes, edges := map[string]model.GraphObservationNode{}, map[string]model.GraphObservationEdge{}
	for _, n := range batch.Nodes {
		if nodes[n.Ref].Ref != "" {
			t.Fatalf("duplicate node %s", n.Ref)
		}
		nodes[n.Ref] = n
		if filepath.IsAbs(n.Key.OwnerPath) {
			t.Fatalf("absolute node path %q", n.Key.OwnerPath)
		}
	}
	for _, e := range batch.Edges {
		if _, ok := edges[e.Ref]; ok {
			t.Fatalf("duplicate edge %s", e.Ref)
		}
		edges[e.Ref] = e
		if filepath.IsAbs(e.OwnerPath) {
			t.Fatalf("absolute edge path %q", e.OwnerPath)
		}
	}
	has := map[string]int{}
	for _, ev := range batch.Evidence {
		if ev.Range == nil || ev.Range.StartLine < 1 || ev.Range.EndLine < ev.Range.StartLine || ev.Range.StartColumn < 1 || ev.Range.EndColumn < 1 || filepath.IsAbs(ev.SourceURI) || ev.ObservedDigest == (model.GraphDigest{}) || ev.ObservedDigest == ev.SourceDigest {
			t.Fatalf("invalid evidence %#v", ev)
		}
		if n, ok := nodes[ev.NodeRef]; ok {
			if ev.SourceDigest != n.SourceDigest || ev.Status != n.ClaimStatus {
				t.Fatalf("node evidence mismatch %#v", ev)
			}
			has[ev.NodeRef]++
		} else if e, ok := edges[ev.EdgeRef]; ok {
			if ev.SourceDigest != e.SourceDigest || ev.Status != e.Status {
				t.Fatalf("edge evidence mismatch %#v", ev)
			}
			has[ev.EdgeRef]++
		} else {
			t.Fatalf("dangling evidence %#v", ev)
		}
	}
	for ref := range nodes {
		if has[ref] == 0 {
			t.Fatalf("node lacks evidence %s", ref)
		}
	}
	for ref := range edges {
		if has[ref] == 0 {
			t.Fatalf("edge lacks evidence %s", ref)
		}
	}
	for _, c := range batch.Coverage {
		observed, unresolved, omitted := 0, 0, 0
		if c.Capability == "declarations" {
			observed = len(batch.Nodes)
		} else {
			for _, e := range batch.Edges {
				if e.Relation == c.Capability {
					observed++
				}
			}
		}
		for _, u := range batch.Unresolved {
			if u.Capability == c.Capability {
				unresolved++
			}
			if filepath.IsAbs(u.OwnerPath) {
				t.Fatal("absolute unresolved path")
			}
		}
		for _, o := range batch.Omissions {
			if o.Capability == c.Capability {
				omitted++
			}
			if filepath.IsAbs(o.OwnerPath) {
				t.Fatal("absolute omission path")
			}
		}
		if c.Observed != observed || c.Unresolved != unresolved || c.Omitted != omitted || (batch.Completeness == model.GraphCompletenessComplete && c.Eligible != observed+unresolved+omitted) {
			t.Fatalf("bad coverage %#v", c)
		}
	}
}
func hasOmission(batch model.GraphObservationBatch, capability, reason string) bool {
	for _, o := range batch.Omissions {
		if o.Capability == capability && o.ReasonCode == reason {
			return true
		}
	}
	return false
}
func hasEvidenceAtLine(batch model.GraphObservationBatch, uri string, line int) bool {
	for _, e := range batch.Evidence {
		if e.SourceURI == uri && e.Range != nil && e.Range.StartLine == line {
			return true
		}
	}
	return false
}

func nodeByRef(batch model.GraphObservationBatch, ref string) model.GraphObservationNode {
	for _, n := range batch.Nodes {
		if n.Ref == ref {
			return n
		}
	}
	return model.GraphObservationNode{}
}
func graphNodeByIdentityOwner(batch model.GraphObservationBatch, identity, owner string) (model.GraphObservationNode, bool) {
	for _, node := range batch.Nodes {
		if node.Key.SemanticIdentity == identity && node.Key.OwnerPath == owner {
			return node, true
		}
	}
	return model.GraphObservationNode{}, false
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

func TestGoGraphStableSyntheticPackageAndFileLocalInit(t *testing.T) {
	root := t.TempDir()
	writeGoTestFile(t, root, "go.mod", "module example.com/stablepkg\n\ngo 1.24.4\n")
	writeGoTestFile(t, root, "z.go", "package stablepkg\nfunc init() {}\nfunc F() {}\n")
	req := GoGraphObservationRequest{Root: root, RepositoryIdentity: "example.com/stablepkg", ProjectOrModule: "go.mod"}
	before, err := ObserveGoGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	pkgBefore, ok := graphNodeByIdentity(before, "pkg:example.com/stablepkg")
	if !ok || pkgBefore.Key.OwnerPath != "@package/example.com/stablepkg" {
		t.Fatalf("package owner = %#v", pkgBefore.Key)
	}
	initBefore, ok := graphNodeByIdentity(before, "func:example.com/stablepkg:init:0")
	if !ok {
		t.Fatal("missing init")
	}
	writeGoTestFile(t, root, "a.go", "package stablepkg\nfunc init() {}\n")
	after, err := ObserveGoGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	pkgAfter, ok := graphNodeByIdentity(after, "pkg:example.com/stablepkg")
	if !ok || pkgAfter.Ref != pkgBefore.Ref || pkgAfter.SourceDigest == pkgBefore.SourceDigest {
		t.Fatal("package identity/digest not stable/changed")
	}
	initAfter, ok := graphNodeByIdentityOwner(after, "func:example.com/stablepkg:init:0", "z.go")
	if !ok || initAfter.Ref != initBefore.Ref {
		t.Fatal("file-local init identity shifted")
	}
}

func TestGoGraphUnknownEndpointIsUnresolved(t *testing.T) {
	root := t.TempDir()
	writeGoTestFile(t, root, "go.mod", "module example.com/missing\n\ngo 1.24.4\n")
	writeGoTestFile(t, root, "main.go", "package missing\nfunc F() { Missing() }\n")
	batch, err := ObserveGoGraph(context.Background(), GoGraphObservationRequest{Root: root, RepositoryIdentity: "example.com/missing", ProjectOrModule: "go.mod"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range batch.Unresolved {
		if u.ReasonCode == "target_endpoint_missing" && (u.Capability == "calls" || u.Capability == "references") {
			found = true
		}
	}
	if !found || batch.ReadyForStaging() == nil {
		t.Fatalf("missing unresolved endpoint: %#v", batch.Unresolved)
	}
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
