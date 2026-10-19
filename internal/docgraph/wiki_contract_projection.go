package docgraph

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// Accept machine YAML as well as the fenced representation used by legacy docs.
func parseWikiContractGovernanceDocument(content []byte) (*WikiContractGovernance, []string) {
	if block, err := extractGovernanceYAMLBlock(content); err == nil {
		return ParseWikiContractGovernanceYAML(block)
	}
	text := strings.TrimSpace(string(content))
	if strings.HasPrefix(text, "---\n") || strings.HasPrefix(text, "---\r\n") {
		text = strings.TrimSpace(text[3:])
		if end := strings.Index("\n"+text, "\n---"); end >= 0 {
			text = text[:end]
		}
	}
	return ParseWikiContractGovernanceYAML(text)
}

func buildWikiContractReadProfile(g *WikiContractGovernance, sourceDoc string) model.DocsReadProfile {
	wikiRoot := path.Dir(filepath.ToSlash(sourceDoc))
	type surface struct {
		kind, family, trait string
		paths               []string
	}
	specs := []surface{
		{"governance", "functional", "", []string{"00_governance.md", "00_retirements.md"}},
		{"scope", "functional", "", []string{"01_scope.md"}},
		{"outcome", "functional", "", []string{"02_outcomes/RS-*.md"}},
		{"architecture", "functional", "", []string{"03_architecture.md"}},
		{"decision", "functional", "", []string{"03_architecture/ADR-*.md"}},
		{"flow", "functional", "", []string{"04_flows/FL-*.md"}},
		{"requirement", "functional", "", []string{"05_requirements/RF-*.md"}},
		{"data_model", "functional", "stores_data", []string{"05_requirements/data-model.md", "05_requirements/data-model/DM-*.md"}},
		{"test", "functional", "", []string{"06_tests/TP-*.md"}},
		{"technical", "technical", "", []string{"07_technical.md", "07_technical/TECH-*.md"}},
		{"database", "technical", "stores_data", []string{"07_technical/db/DB-*.md"}},
		{"contract", "technical", "exposes_api", []string{"07_technical/contracts/CT-*.md"}},
		{"journey", "ux", "has_ui", []string{"10_ux/UJ-*.md", "10_ux/**/UJ-*.md"}},
		{"ux_module", "ux", "has_ui", []string{"10_ux/UXM-*.md", "10_ux/**/UXM-*.md"}},
		{"design_md", "ux", "has_ui", nil},
		{"product_md", "ux", "has_ui", nil},
		{"ui", "ux", "has_ui", nil},
		{"business", "functional", "sold_product", []string{"20_business/NEG-*.md"}},
	}
	excepted := map[string]bool{}
	for _, exception := range g.LayerExceptions {
		excepted[exceptionLayerID(exception)] = true
	}
	source := model.GovernanceSource{Version: 1, Profile: "product", Projection: model.GovernanceProjection{
		Output: path.Join(wikiRoot, "_mi-lsp/read-model.toml"), Format: "toml", AutoSync: true, Versioned: true,
	}}
	var surfaces []model.DocsAgentSurface
	for _, spec := range specs {
		if (spec.trait != "" && !g.TraitEnabled(spec.trait)) || excepted[spec.kind] ||
			(spec.kind == "test" && excepted["test_plan"]) ||
			(spec.kind == "decision" && excepted["architecture"]) {
			continue
		}
		var globs []string
		for _, rel := range spec.paths {
			globs = append(globs, path.Join(wikiRoot, rel))
		}
		switch spec.kind {
		case "design_md":
			globs = []string{"DESIGN.md"}
		case "product_md":
			globs = []string{"PRODUCT.md"}
		case "ui":
			roots, _ := model.SafeRoots(g.AggregatedUIRoots())
			for _, root := range roots {
				globs = append(globs, strings.TrimSuffix(root, "/")+"/**")
			}
			sort.Strings(globs)
			globs = dedupeStrings(globs)
		}
		if len(globs) == 0 {
			continue
		}
		surfaces = append(surfaces, model.DocsAgentSurface{Kind: spec.kind, ReadGlobs: globs, EditGlobs: append([]string{}, globs...)})
		source.Hierarchy = append(source.Hierarchy, model.GovernanceHierarchyItem{
			ID: spec.kind, Layer: spec.kind, Family: spec.family, PackStage: spec.kind, Paths: globs,
		})
	}
	profile := buildDocsReadProfileFromGovernance(source, resolvedGovernanceProfile{Base: "product"}, sourceDoc)
	// Broad fallback roots would reintroduce layers removed by traits/exceptions.
	profile.GenericDocs.Paths = []string{"README.md", "README*.md"}
	profile.ProjectionSource = "wiki-contract-v1/traits"
	profile.AgentSurfaces = surfaces
	return profile
}

func inspectWikiContractProjection(root string, status model.GovernanceStatus, g *WikiContractGovernance, autoSync bool) model.GovernanceStatus {
	profile := buildWikiContractReadProfile(g, status.HumanDoc)
	status.ProjectionDoc = profile.Governance.Projection.Output
	status.Profile = profile.Governance.Profile
	status.EffectiveBase = profile.Governance.EffectiveBase
	status.WikiContract = g.WikiContract
	status.Sync = "stale"
	status.IndexSync, status.IndexSyncDetails = inspectIndexSync(root)
	if status.IndexSync == "stale" {
		status.Issues = append(status.Issues, "workspace index is stale relative to governance sources; rerun mi-lsp index")
		status.Blocked = true
	}
	rendered, err := encodeDocsReadProfile(profile)
	if err != nil {
		status.Warnings = append(status.Warnings, fmt.Sprintf("read-model projection render failed: %v", err))
	} else if safe, ok := safeProjectionOutput(root, status.ProjectionDoc, tryResolvedCanons(root)); !ok {
		status.Warnings = append(status.Warnings, "read-model projection output is unsafe or inside a read-only canon root")
	} else {
		projectionAbs := absFromDeclared(root, safe)
		current, readErr := os.ReadFile(projectionAbs)
		switch {
		case readErr == nil && normalizedText(current) == normalizedText(rendered):
			status.Sync = "in_sync"
		case autoSync:
			err = os.MkdirAll(filepath.Dir(projectionAbs), 0o755)
			if err == nil {
				err = os.WriteFile(projectionAbs, rendered, 0o644)
			}
			if err != nil {
				status.Warnings = append(status.Warnings, fmt.Sprintf("read-model projection write failed: %v", err))
			} else {
				status.Sync = "auto_synced"
				status.Warnings = append(status.Warnings, "read-model.toml auto-synced from "+status.HumanDoc)
			}
		default:
			status.Warnings = append(status.Warnings, "read-model.toml is out of sync with "+status.HumanDoc)
		}
	}
	status.Summary = "WIKI-CONTRACT-v1 read-model projection is advisory (" + status.Sync + ")."
	return status
}
