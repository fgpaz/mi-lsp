package service

import (
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// TestCanonicalRoutePreservesGovernanceAnchorWhenNotIndexed reproduces the
// source-proven inconsistency: a docs index that contains a mention-bearing
// artifact (a test file asserting an ID) but not the canonical governance
// anchor. The Tier1 anchor must keep the anchor seat; indexed docs stay as
// tier2 preview only.
func TestCanonicalRoutePreservesGovernanceAnchorWhenNotIndexed(t *testing.T) {
	root := makeRouteFixtureRoot(t)

	artifact := model.DocRecord{
		Path:       "scripts/kernel-contract-docs.test.mjs",
		Title:      "kernel contract docs test",
		Layer:      "07",
		Family:     "technical",
		SearchText: "asserts AE-POLICY-PROJECTION projection stays in sync with AE-POLICY-PROJECTION-V2",
		IndexedAt:  1,
	}
	ranked := []scoredDoc{
		{record: artifact, score: 120, reason: []string{"fts5=match", "search_overlap"}},
	}

	query := &docQueryContext{
		registration:  model.WorkspaceRegistration{Name: "fixture", Root: root},
		task:          "AE-POLICY-PROJECTION",
		rankingTask:   "AE-POLICY-PROJECTION",
		profile:       routeFixtureProfile(),
		profileSource: "fixture",
		family:        "technical",
		docs:          []model.DocRecord{artifact},
		docByPath:     map[string]model.DocRecord{artifact.Path: artifact},
		ranked:        ranked,
		rankedByPath:  map[string]scoredDoc{artifact.Path: ranked[0]},
	}

	result := query.canonicalRoute(model.QueryOptions{}, false)
	anchor := result.Canonical.AnchorDoc
	if anchor.Path != "canon/AE-POLICY-PROJECTION.md" {
		t.Fatalf("anchor path = %q, want governance canon doc", anchor.Path)
	}
	if anchor.Path == artifact.Path {
		t.Fatal("mention-bearing artifact took the anchor seat")
	}
	foundWhy := false
	for _, why := range result.Why {
		if why == "tier2=anchor_not_indexed" {
			foundWhy = true
		}
	}
	if !foundWhy {
		t.Fatalf("expected tier2=anchor_not_indexed reason, got %v", result.Why)
	}
	// The artifact must not impersonate document identity.
	for _, preview := range result.Canonical.PreviewPack {
		if preview.Stage == "anchor" {
			t.Fatalf("preview doc carried anchor stage: %#v", preview)
		}
	}
}

// TestCanonicalRouteAmbiguousSiblingIDsDoNotPickOwner guards against a
// suffix-less query silently binding to one of two sibling documents. With
// no explicit doc ID in the query, neither sibling may be elevated by ID
// matching: the governance-owned anchor keeps the anchor seat and the
// siblings stay in ranked discovery.
func TestCanonicalRouteAmbiguousSiblingIDsDoNotPickOwner(t *testing.T) {
	root := makeRouteFixtureRoot(t)

	siblingV2 := model.DocRecord{
		Path:       "canon/AE-POLICY-PROJECTION-V2.md",
		Title:      "AE-POLICY-PROJECTION-V2",
		DocID:      "AE-POLICY-PROJECTION-V2",
		Layer:      "canon",
		Family:     "technical",
		SearchText: "deterministic policy kernel rendering version 2",
		IndexedAt:  1,
	}
	siblingV3 := model.DocRecord{
		Path:       "canon/AE-POLICY-PROJECTION-V3.md",
		Title:      "AE-POLICY-PROJECTION-V3",
		DocID:      "AE-POLICY-PROJECTION-V3",
		Layer:      "canon",
		Family:     "technical",
		SearchText: "deterministic policy kernel rendering version 3",
		IndexedAt:  1,
	}
	artifact := model.DocRecord{
		Path:       "scripts/kernel-contract-docs.test.mjs",
		Title:      "kernel contract docs test",
		Layer:      "07",
		Family:     "technical",
		SearchText: "asserts AE-POLICY-PROJECTION projection stays in sync",
		IndexedAt:  1,
	}

	docs := []model.DocRecord{siblingV2, siblingV3, artifact}
	docByPath := map[string]model.DocRecord{}
	for _, doc := range docs {
		docByPath[doc.Path] = doc
	}

	query := &docQueryContext{
		registration:  model.WorkspaceRegistration{Name: "fixture", Root: root},
		task:          "AE-POLICY-PROJECTION",
		rankingTask:   "AE-POLICY-PROJECTION",
		profile:       routeFixtureProfile(),
		profileSource: "fixture",
		family:        "technical",
		docs:          docs,
		docByPath:     docByPath,
		ranked: []scoredDoc{
			{record: artifact, score: 60, reason: []string{"search_overlap"}},
			{record: siblingV2, score: 55, reason: []string{"path_overlap"}},
			{record: siblingV3, score: 50, reason: []string{"search_overlap"}},
		},
		rankedByPath: map[string]scoredDoc{},
	}

	result := query.canonicalRoute(model.QueryOptions{}, false)
	anchor := result.Canonical.AnchorDoc
	// No sibling may be silently picked as the owner of the suffix-less ID.
	if anchor.Path == siblingV2.Path || anchor.Path == siblingV3.Path {
		t.Fatalf("ambiguous sibling silently picked as owner: %q", anchor.Path)
	}
	if anchor.DocID == "AE-POLICY-PROJECTION-V2" || anchor.DocID == "AE-POLICY-PROJECTION-V3" {
		t.Fatalf("ambiguous sibling ID attached to anchor: %#v", anchor)
	}
}

func TestCanonicalRouteExplicitDocIDStillWinsOverRankedArtifact(t *testing.T) {
	root := makeRouteFixtureRoot(t)

	artifact := model.DocRecord{
		Path:       "scripts/kernel-contract-docs.test.mjs",
		Title:      "kernel contract docs test",
		Layer:      "07",
		Family:     "technical",
		SearchText: "mentions AE-POLICY-PROJECTION-V2 many times",
		IndexedAt:  1,
	}
	canon := model.DocRecord{
		Path:       "canon/AE-POLICY-PROJECTION.md",
		Title:      "AE-POLICY-PROJECTION.md — Deterministic Policy Kernel Rendering",
		DocID:      "AE-POLICY-PROJECTION-V2",
		Layer:      "canon",
		Family:     "technical",
		SearchText: "deterministic policy kernel rendering",
		IndexedAt:  1,
	}

	query := &docQueryContext{
		registration:  model.WorkspaceRegistration{Name: "fixture", Root: root},
		task:          "AE-POLICY-PROJECTION-V2",
		rankingTask:   "AE-POLICY-PROJECTION-V2",
		profile:       routeFixtureProfile(),
		profileSource: "fixture",
		family:        "technical",
		docs:          []model.DocRecord{artifact, canon},
		docByPath:     map[string]model.DocRecord{artifact.Path: artifact, canon.Path: canon},
		ranked: []scoredDoc{
			{record: artifact, score: 200, reason: []string{"fts5=match"}},
			{record: canon, score: 90, reason: []string{"doc_id=AE-POLICY-PROJECTION-V2"}},
		},
		rankedByPath: map[string]scoredDoc{},
	}

	result := query.canonicalRoute(model.QueryOptions{}, false)
	anchor := result.Canonical.AnchorDoc
	if anchor.Path != canon.Path {
		t.Fatalf("anchor path = %q, want indexed governance canon doc", anchor.Path)
	}
	if anchor.DocID != "AE-POLICY-PROJECTION-V2" {
		t.Fatalf("anchor doc id = %q, want AE-POLICY-PROJECTION-V2", anchor.DocID)
	}
}
