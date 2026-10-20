package docgraph

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestWikiContractProjectionTraits(t *testing.T) {
	content := []byte(`wiki_contract: WIKI-CONTRACT-v1
language: es
ae_canon:
  mode: kernel_v2
  source: <kernel_home>/canon
  repo_policy: .docs/ae/repo-policy.yaml
traits:
  stores_data: SQLite
  exposes_api: CLI
  sold_product: Subscription
  has_ui:
    reason: Panel
    ui_roots: [web/, extension/ui/, web/, ../outside/]
`)
	g, warnings := parseWikiContractGovernanceDocument(content)
	if g == nil || len(warnings) != 0 {
		t.Fatalf("parse: %v %v", g, warnings)
	}
	profile := buildWikiContractReadProfile(g, canonGovernanceV1RelPath)
	rendered, err := encodeWikiContractProjection(profile, g, content)
	if err != nil {
		t.Fatal(err)
	}
	again, err := encodeWikiContractProjection(buildWikiContractReadProfile(g, canonGovernanceV1RelPath), g, content)
	if err != nil || string(rendered) != string(again) {
		t.Fatal("projection is not deterministic")
	}
	var metadata map[string]any
	if _, err := toml.Decode(string(rendered), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["language"] != "es" || metadata["read_model_contract"] != "WIKI-CONTRACT-v1.1" || metadata["model_revision"] != "1.1" {
		t.Fatalf("metadata: %+v", metadata)
	}
	governance := metadata["governance"].(map[string]any)
	for _, key := range []string{"profile", "effective_base", "overlays", "effective_overlays", "hierarchy"} {
		if _, exists := governance[key]; exists {
			t.Errorf("legacy authority emitted: %s", key)
		}
	}
	traits := metadata["traits"].(map[string]any)
	ui := traits["has_ui"].(map[string]any)
	if ui["reason"] != "Panel" || !reflect.DeepEqual(ui["ui_roots"], []any{"extension/ui/", "web/"}) {
		t.Fatalf("trait metadata: %+v", ui)
	}
	if governance["ae_canon"].(map[string]any)["mode"] != "kernel_v2" {
		t.Fatal("kernel candidate source lost")
	}
	var decoded model.DocsReadProfile
	if _, err := toml.Decode(string(rendered), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ProjectionSource != "wiki-contract-v1/traits" || decoded.Version != 1 {
		t.Fatalf("unexpected projection: %+v", decoded)
	}
	want := map[string][]string{
		"governance":   {".docs/wiki/00_governance.md", ".docs/wiki/00_retirements.md"},
		"scope":        {".docs/wiki/01_scope.md"},
		"outcome":      {".docs/wiki/02_outcomes/RS-*.md"},
		"architecture": {".docs/wiki/03_architecture.md"},
		"decision":     {".docs/wiki/03_architecture/ADR-*.md"},
		"flow":         {".docs/wiki/04_flows/FL-*.md"},
		"requirement":  {".docs/wiki/05_requirements/RF-*.md"},
		"data_model":   {".docs/wiki/05_requirements/data-model.md", ".docs/wiki/05_requirements/data-model/DM-*.md"},
		"test":         {".docs/wiki/06_tests/TP-*.md"},
		"technical":    {".docs/wiki/07_technical.md", ".docs/wiki/07_technical/TECH-*.md"},
		"database":     {".docs/wiki/07_technical/db/DB-*.md"},
		"contract":     {".docs/wiki/07_technical/contracts/CT-*.md"},
		"journey":      {".docs/wiki/10_ux/UJ-*.md", ".docs/wiki/10_ux/**/UJ-*.md"},
		"ux_module":    {".docs/wiki/10_ux/UXM-*.md", ".docs/wiki/10_ux/**/UXM-*.md"},
		"design_md":    {"DESIGN.md"}, "product_md": {"PRODUCT.md"},
		"ui":       {"extension/ui/**", "web/**"},
		"business": {".docs/wiki/20_business/NEG-*.md"},
	}
	if len(decoded.AgentSurfaces) != len(want) {
		t.Fatalf("surfaces: %+v", decoded.AgentSurfaces)
	}
	for _, surface := range decoded.AgentSurfaces {
		if !reflect.DeepEqual(surface.ReadGlobs, want[surface.Kind]) || !reflect.DeepEqual(surface.EditGlobs, want[surface.Kind]) {
			t.Errorf("surface %s: %+v; want %v", surface.Kind, surface, want[surface.Kind])
		}
	}
	if !reflect.DeepEqual(decoded.Families, profile.Families) {
		t.Fatal("family compatibility lost")
	}
}

func TestWikiContractProjectionExceptionsAndCore(t *testing.T) {
	for _, layer := range []string{"outcome", "architecture", "test", "data_model", "business"} {
		t.Run(layer, func(t *testing.T) {
			g, warnings := ParseWikiContractGovernanceYAML("wiki_contract: WIKI-CONTRACT-v1\ntraits:\n  stores_data: SQLite\n  sold_product: Subscription\nlayer_exceptions:\n  - layer: " + layer + "\n    reason: Not applicable\n    authorized_by: Gabriel\n    date: 2026-10-18\n")
			if len(warnings) != 0 || len(g.LayerExceptions) != 1 {
				t.Fatalf("exceptions: %v %v", g, warnings)
			}
			profile := buildWikiContractReadProfile(g, canonGovernanceV1RelPath)
			for _, surface := range profile.AgentSurfaces {
				if surface.Kind == layer || (layer == "architecture" && surface.Kind == "decision") {
					t.Fatalf("excepted surface: %s", surface.Kind)
				}
			}
		})
	}
	g, _ := ParseWikiContractGovernanceYAML("wiki_contract: WIKI-CONTRACT-v1\n")
	profile := buildWikiContractReadProfile(g, canonGovernanceV1RelPath)
	if len(profile.AgentSurfaces) != 9 {
		t.Fatalf("core surfaces: %+v", profile.AgentSurfaces)
	}
}

func TestWikiContractProjectionAdvisorySync(t *testing.T) {
	root := t.TempDir()
	writeGovernanceYAML(t, root, canonGovernanceV1RelPath, "wiki_contract: WIKI-CONTRACT-v1\nversion: 1\nlanguage: en\n")
	status := InspectGovernance(root, false)
	if status.Blocked || status.Sync != "stale" || len(status.Warnings) == 0 || len(status.Issues) != 0 {
		t.Fatalf("drift blocked: %+v", status)
	}
	status = InspectGovernance(root, true)
	if status.Blocked || status.Sync != "auto_synced" {
		t.Fatalf("sync: %+v", status)
	}
	profile, source, warnings := LoadProfile(root)
	if source != "project" || len(warnings) != 0 || profile.ProjectionSource != "wiki-contract-v1/traits" {
		t.Fatalf("load: %s %v %+v", source, warnings, profile)
	}
	status = InspectGovernance(root, false)
	if status.Blocked || status.Sync != "in_sync" {
		t.Fatalf("in sync: %+v", status)
	}
	output := filepath.Join(root, filepath.FromSlash(defaultProjectionRelPath))
	if err := os.Remove(output); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(output, 0o755); err != nil {
		t.Fatal(err)
	}
	status = InspectGovernance(root, true)
	if status.Blocked || status.Sync != "stale" || len(status.Warnings) == 0 {
		t.Fatalf("write failure blocked: %+v", status)
	}
}

func TestWikiContractProjectionVersionAndExceptions(t *testing.T) {
	content := []byte(`wiki_contract: WIKI-CONTRACT-v1.1
language: en
traits:
  stores_data: SQLite
layer_exceptions:
  - layer: database
    reason: No physical schema
    authorized_by: Gabriel
    date: 2026-10-18
`)
	g, warnings := parseWikiContractGovernanceDocument(content)
	if g == nil || len(warnings) != 0 {
		t.Fatalf("parse: %v %v", g, warnings)
	}
	profile := buildWikiContractReadProfile(g, "wiki/00_governance.md")
	rendered, err := encodeWikiContractProjection(profile, g, content)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Language   string `toml:"language"`
		Exceptions []struct {
			Layer        string `toml:"layer"`
			Reason       string `toml:"reason"`
			AuthorizedBy string `toml:"authorized_by"`
			Date         string `toml:"date"`
		} `toml:"layer_exceptions"`
	}
	if _, err := toml.Decode(string(rendered), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Language != "en" || len(decoded.Exceptions) != 1 {
		t.Fatalf("metadata: %+v", decoded)
	}
	exception := decoded.Exceptions[0]
	if exception.Layer != "database" || exception.Reason != "No physical schema" || exception.AuthorizedBy != "Gabriel" || exception.Date != "2026-10-18" {
		t.Fatalf("exception: %+v", exception)
	}
	for _, surface := range profile.AgentSurfaces {
		if surface.Kind == "database" {
			t.Fatal("excepted database surface emitted")
		}
	}
	if strings.Contains(string(rendered), "10_ux/") || strings.Contains(string(rendered), "20_business/") {
		t.Fatal("disabled trait invented a layer")
	}
	if _, err := encodeWikiContractProjection(profile, g, []byte("language: fr\n")); err == nil {
		t.Fatal("unsupported language accepted")
	}
}

func TestWikiContractProjectionRepositoryParity(t *testing.T) {
	root := filepath.Join("..", "..")
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(canonGovernanceV1RelPath)))
	if err != nil {
		t.Fatal(err)
	}
	g, warnings := parseWikiContractGovernanceDocument(content)
	if g == nil || len(warnings) != 0 {
		t.Fatalf("parse: %v %v", g, warnings)
	}
	rendered, err := encodeWikiContractProjection(buildWikiContractReadProfile(g, canonGovernanceV1RelPath), g, content)
	if err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(defaultProjectionRelPath)))
	if err != nil {
		t.Fatal(err)
	}
	if normalizedText(current) != normalizedText(rendered) {
		t.Fatal("versioned read-model differs from inspection/autoSync rendering")
	}
}

func TestWikiContractProjectionGovernanceRepresentations(t *testing.T) {
	for _, content := range []string{"wiki_contract: WIKI-CONTRACT-v1\nversion: 1\n", "---\r\nwiki_contract: WIKI-CONTRACT-v1\r\nversion: 1\r\n---\r\n# Governance\r\n"} {
		g, warnings := parseWikiContractGovernanceDocument([]byte(content))
		if g == nil || len(warnings) != 0 {
			t.Fatalf("parse %q: %v %v", content, g, warnings)
		}
		profile := buildWikiContractReadProfile(g, "wiki/00_governance.md")
		if profile.Governance.Projection.Output != "wiki/_mi-lsp/read-model.toml" || !strings.HasPrefix(profile.AgentSurfaces[0].ReadGlobs[0], "wiki/") {
			t.Fatalf("custom root: %+v", profile)
		}
	}
}
