package model

import (
	"fmt"
	"testing"
)

func TestGraphImpactRequestNormalizesChangedPathsAndConservativeRelations(t *testing.T) {
	q, err := (GraphImpactRequest{
		ChangedPaths: []string{"./src\\a.go", "src/a.go"},
		Mode:         GraphImpactModeTransitive,
		Depth:        2,
		Relations:    []string{"CALLS", "calls"},
	}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Paths) != 1 || q.Paths[0] != "src/a.go" || len(q.ChangedPaths) != 1 {
		t.Fatalf("paths=%+v changed=%+v", q.Paths, q.ChangedPaths)
	}
	if q.Direction != "in" || len(q.Relations) != 1 || q.Relations[0] != "calls" {
		t.Fatalf("normalized query=%+v", q)
	}
}

func TestGraphImpactRejectsTextualReferenceRelation(t *testing.T) {
	_, err := (GraphImpactRequest{Paths: []string{"src/a.go"}, Relations: []string{"references"}}).Normalize()
	if err == nil {
		t.Fatal("expected unsupported impact relation")
	}
	graphErr, ok := err.(*GraphQueryError)
	if !ok || graphErr.Code != "GPH_IMPACT_RELATION_UNSUPPORTED" {
		t.Fatalf("error=%v", err)
	}
}

func TestGraphImpactRelationSemanticsAreBoundedAndStable(t *testing.T) {
	relation, ok := GraphImpactRelationSemantics(" calls ")
	if !ok || relation.Direction != "in" || !relation.Transitive || relation.Cost != 1 {
		t.Fatalf("relation=%+v ok=%v", relation, ok)
	}
	all := GraphImpactRelations()
	for i := 1; i < len(all); i++ {
		if all[i-1].Relation >= all[i].Relation {
			t.Fatalf("relations are not stable: %+v", all)
		}
	}
}

func TestGraphImpactNormalizeBoundsMassiveSeedsDeterministically(t *testing.T) {
	paths := make([]string, MaxImpactSeeds+17)
	for i := range paths {
		paths[i] = fmt.Sprintf("src/%04d.go", i)
	}
	q, err := (GraphImpactRequest{Paths: paths}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Paths) != MaxImpactSeeds || q.Paths[0] != "src/0000.go" || q.Paths[MaxImpactSeeds-1] != fmt.Sprintf("src/%04d.go", MaxImpactSeeds-1) {
		t.Fatalf("bounded paths=%d first=%q last=%q", len(q.Paths), q.Paths[0], q.Paths[len(q.Paths)-1])
	}
	if len(q.Omissions) != 1 || q.Omissions[0].Code != "GPH_IMPACT_SEED_BUDGET_EXCEEDED" || len(q.Omissions[0].Candidates) > 16 {
		t.Fatalf("omissions=%+v", q.Omissions)
	}
	if len(q.Warnings) != 1 || q.Warnings[0] == "" {
		t.Fatalf("warnings=%+v", q.Warnings)
	}
}
