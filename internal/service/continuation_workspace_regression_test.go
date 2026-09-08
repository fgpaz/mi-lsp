package service

import (
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestPackContinuationPreservesWorkspaceAndDocID(t *testing.T) {
	result := model.PackResult{
		PrimaryDoc: "canon/AE-POLICY-PROJECTION.md",
		Docs: []model.PackDoc{{
			Path:  "canon/AE-POLICY-PROJECTION.md",
			Title: "AE-POLICY-PROJECTION",
			DocID: "AE-POLICY-PROJECTION-V2",
		}},
	}
	cont := buildPackContinuation("nav.pack", "AE-POLICY-PROJECTION", result, model.QueryOptions{Workspace: "ae-kernel"}, nil)
	if cont == nil {
		t.Fatal("expected preview continuation")
	}
	if cont.Next.Workspace != "ae-kernel" {
		t.Fatalf("preview continuation lost workspace: %#v", cont.Next)
	}
	if cont.Next.DocID != "AE-POLICY-PROJECTION-V2" {
		t.Fatalf("preview continuation lost doc id: %#v", cont.Next)
	}

	full := buildPackContinuation("nav.pack", "AE-POLICY-PROJECTION", result, model.QueryOptions{Workspace: "ae-kernel", Full: true}, nil)
	if full == nil {
		t.Fatal("expected full continuation")
	}
	if full.Next.Workspace != "ae-kernel" {
		t.Fatalf("full continuation lost workspace: %#v", full.Next)
	}
}

func TestRouteContinuationPreservesWorkspaceAndDocID(t *testing.T) {
	result := model.RouteResult{
		Canonical: model.RouteCanonicalLane{
			AnchorDoc: model.RouteDoc{Path: "canon/AE-POLICY-PROJECTION.md", DocID: "AE-POLICY-PROJECTION-V2"},
		},
	}
	cont := buildRouteContinuation("AE-POLICY-PROJECTION", result, model.QueryOptions{Workspace: "ae-kernel"}, nil)
	if cont == nil {
		t.Fatal("expected preview continuation")
	}
	if cont.Next.Workspace != "ae-kernel" || cont.Next.DocID != "AE-POLICY-PROJECTION-V2" {
		t.Fatalf("route preview continuation lost workspace/doc id: %#v", cont.Next)
	}

	full := buildRouteContinuation("AE-POLICY-PROJECTION", result, model.QueryOptions{Workspace: "ae-kernel", Full: true}, nil)
	if full == nil {
		t.Fatal("expected full continuation")
	}
	if full.Next.Workspace != "ae-kernel" {
		t.Fatalf("route full continuation lost workspace: %#v", full.Next)
	}
}

func TestContinuationOmitsWorkspaceWhenSelectorEmpty(t *testing.T) {
	result := model.RouteResult{
		Canonical: model.RouteCanonicalLane{
			AnchorDoc: model.RouteDoc{Path: ".docs/wiki/00_gobierno_documental.md"},
		},
	}
	cont := buildRouteContinuation("governance", result, model.QueryOptions{}, nil)
	if cont == nil {
		t.Fatal("expected continuation")
	}
	if cont.Next.Workspace != "" {
		t.Fatalf("empty selector must stay omitted, got %#v", cont.Next)
	}
}

// TestRankingDeclaredOwnerBeatsMentionArtifact reproduces the source-proven
// observation: a query naming the full canonical ID must rank the declared
// owner (the canon doc carrying that DocID) above a support artifact whose
// body merely mentions the ID. Mentions/assertions must not impersonate
// document identity.
func TestRankingDeclaredOwnerBeatsMentionArtifact(t *testing.T) {
	owner := model.DocRecord{
		Path:       "canon/AE-POLICY-PROJECTION.md",
		Title:      "AE-POLICY-PROJECTION.md — Deterministic Policy Kernel Rendering",
		DocID:      "AE-POLICY-PROJECTION-V2",
		Layer:      "canon",
		Family:     "technical",
		SearchText: "deterministic policy kernel rendering projection contract",
	}
	artifact := model.DocRecord{
		Path:       "scripts/kernel-contract-docs.test.mjs",
		Title:      "kernel contract docs test",
		Layer:      "07",
		Family:     "technical",
		SearchText: "asserts AE-POLICY-PROJECTION-V2 projection stays in sync with AE-POLICY-PROJECTION-V2",
	}

	legacy := legacyRankDocs("AE-POLICY-PROJECTION-V2", "technical", []model.DocRecord{artifact, owner}, nil)
	if len(legacy) == 0 || legacy[0].record.Path != owner.Path {
		t.Fatalf("legacy ranking: expected declared owner first, got %v", rankedPathsForTest(legacy))
	}

	ownerAware := ownerAwareRankDocs("AE-POLICY-PROJECTION-V2", "technical", []model.DocRecord{artifact, owner}, nil, model.DocsReadProfile{}, nil)
	if len(ownerAware) == 0 || ownerAware[0].record.Path != owner.Path {
		t.Fatalf("owner-aware ranking: expected declared owner first, got %v", rankedPathsForTest(ownerAware))
	}
}

// TestRankingAmbiguousSiblingIDsDoNotPickOwner guards the ambiguous-sibling
// boundary: a suffix-less query that matches two sibling DocIDs must not
// silently elevate either sibling via ID matching. Without an explicit doc
// ID in the question, no doc_id reason may appear for either sibling.
func TestRankingAmbiguousSiblingIDsDoNotPickOwner(t *testing.T) {
	v2 := model.DocRecord{
		Path:       "canon/AE-POLICY-PROJECTION-V2.md",
		Title:      "AE-POLICY-PROJECTION-V2",
		DocID:      "AE-POLICY-PROJECTION-V2",
		Layer:      "canon",
		Family:     "technical",
		SearchText: "deterministic policy kernel rendering version 2",
	}
	v3 := model.DocRecord{
		Path:       "canon/AE-POLICY-PROJECTION-V3.md",
		Title:      "AE-POLICY-PROJECTION-V3",
		DocID:      "AE-POLICY-PROJECTION-V3",
		Layer:      "canon",
		Family:     "technical",
		SearchText: "deterministic policy kernel rendering version 3",
	}

	legacy := legacyRankDocs("AE-POLICY-PROJECTION", "technical", []model.DocRecord{v2, v3}, nil)
	assertNoDocIDReason(t, "legacy", legacy)

	ownerAware := ownerAwareRankDocs("AE-POLICY-PROJECTION", "technical", []model.DocRecord{v2, v3}, nil, model.DocsReadProfile{}, nil)
	assertNoDocIDReason(t, "owner-aware", ownerAware)
}

func assertNoDocIDReason(t *testing.T, label string, ranked []scoredDoc) {
	t.Helper()
	for _, item := range ranked {
		for _, reason := range item.reason {
			if strings.HasPrefix(reason, "doc_id=") {
				t.Fatalf("%s ranking attached doc_id reason %q to %q from a suffix-less query", label, reason, item.record.Path)
			}
		}
	}
}

func rankedPathsForTest(items []scoredDoc) []string {
	paths := make([]string, 0, len(items))
	for _, item := range items {
		paths = append(paths, item.record.Path)
	}
	return paths
}
