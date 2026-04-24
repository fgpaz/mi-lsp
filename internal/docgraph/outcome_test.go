package docgraph

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestDefaultProfileIndexesOutcomeDocsAsRS(t *testing.T) {
	root := t.TempDir()
	mustWriteDocgraphFile(t, filepath.Join(root, ".docs", "wiki", "02_resultados", "RS-TEDI-HOGAR-01.md"), "# RS-TEDI-HOGAR-01\n\nResultado de hogar.\n")

	docs, _, mentions, warnings, err := IndexWorkspaceDocs(context.Background(), root, nil)
	if err != nil {
		t.Fatalf("IndexWorkspaceDocs returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("IndexWorkspaceDocs warnings = %v, want none", warnings)
	}
	if len(docs) != 1 {
		t.Fatalf("docs = %#v, want one outcome doc", docs)
	}
	if docs[0].DocID != "RS-TEDI-HOGAR-01" || docs[0].Layer != "RS" || docs[0].Family != "functional" {
		t.Fatalf("outcome doc classification = %#v", docs[0])
	}
	foundMention := false
	for _, mention := range mentions {
		if mention.MentionType == "doc_id" && mention.MentionValue == "RS-TEDI-HOGAR-01" {
			foundMention = true
			break
		}
	}
	if !foundMention {
		t.Fatalf("expected RS doc_id mention, got %#v", mentions)
	}
}

func TestGovernanceProjectionDerivesFunctionalStageOrder(t *testing.T) {
	source := model.GovernanceSource{
		Version: 1,
		Profile: "spec_backend",
		Hierarchy: []model.GovernanceHierarchyItem{
			{ID: "governance", Layer: "00", Family: "functional", PackStage: "governance", Paths: []string{".docs/wiki/00_gobierno_documental.md"}},
			{ID: "scope", Layer: "01", Family: "functional", PackStage: "scope", Paths: []string{".docs/wiki/01_alcance_funcional.md"}},
			{ID: "outcome", Layer: "RS", Family: "functional", PackStage: "outcome", Paths: []string{".docs/wiki/02_resultados_soluciones_usuario.md", ".docs/wiki/02_resultados/*.md"}},
			{ID: "architecture", Layer: "02", Family: "functional", PackStage: "architecture", Paths: []string{".docs/wiki/02_arquitectura.md"}},
			{ID: "flow", Layer: "03", Family: "functional", PackStage: "flow", Paths: []string{".docs/wiki/03_FL.md"}},
			{ID: "requirements", Layer: "04", Family: "functional", PackStage: "requirements", Paths: []string{".docs/wiki/04_RF.md"}},
			{ID: "tests", Layer: "06", Family: "functional", PackStage: "tests", Paths: []string{".docs/wiki/06_matriz_pruebas_RF.md"}},
		},
	}
	profile := buildDocsReadProfileFromGovernance(source, resolvedGovernanceProfile{Base: "ordered_wiki"})
	want := []string{"governance", "scope", "outcome", "architecture", "flow", "requirements", "tests"}
	if !reflect.DeepEqual(profile.ReadingPack.FunctionalStageOrder, want) {
		t.Fatalf("functional_stage_order = %v, want %v", profile.ReadingPack.FunctionalStageOrder, want)
	}
	if !strings.Contains(strings.Join(profile.Families[0].Paths, " "), "02_resultados") {
		t.Fatalf("functional paths did not preserve outcome paths: %#v", profile.Families[0].Paths)
	}
}

func TestOutcomeGovernanceStageNormalizesLegacyLayerToRS(t *testing.T) {
	profile := model.DocsReadProfile{
		Governance: model.DocsGovernanceProfile{
			Hierarchy: []model.GovernanceHierarchyItem{
				{
					ID:        "outcome",
					Layer:     "02",
					Family:    "functional",
					PackStage: "outcome",
					Paths:     []string{".docs/wiki/02_resultados_soluciones_usuario.md", ".docs/wiki/02_resultados/*.md"},
				},
			},
		},
	}

	for _, path := range []string{
		".docs/wiki/02_resultados_soluciones_usuario.md",
		".docs/wiki/02_resultados/RS-TEDI-HOGAR-01.md",
	} {
		if got := DetectLayerForPath(profile, path); got != "RS" {
			t.Fatalf("DetectLayerForPath(%q) = %q, want RS", path, got)
		}
	}
}
