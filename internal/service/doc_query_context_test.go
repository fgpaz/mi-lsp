package service

import (
	"reflect"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestRouteDocCandidatePathsStayWithinSelectedMarkdownTargets(t *testing.T) {
	query := &docQueryContext{ranked: []scoredDoc{
		{record: model.DocRecord{Path: "ranked/one.md"}},
		{record: model.DocRecord{Path: "ranked/two.md"}},
		{record: model.DocRecord{Path: "ranked/three.md"}},
	}}
	route := model.RouteResult{
		Canonical: model.RouteCanonicalLane{
			AnchorDoc:   model.RouteDoc{Path: "anchor.md"},
			PreviewPack: []model.RouteDoc{{Path: "preview.md"}},
		},
		Discovery: &model.RouteDiscoveryLane{Docs: []model.RouteDoc{
			{Path: "discovery.md"},
			{Path: "notes.txt"},
		}},
	}

	got := routeDocCandidatePaths(query, route, 5)
	want := []string{"anchor.md", "preview.md", "discovery.md", "ranked/one.md", "ranked/two.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidate paths = %v, want selected Markdown paths %v", got, want)
	}
}
