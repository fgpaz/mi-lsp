package docgraph

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/fgpaz/mi-lsp/internal/model"
	"gopkg.in/yaml.v3"
)

// Accept machine YAML as well as the fenced representation used by legacy docs.
func parseWikiContractGovernanceDocument(content []byte) (*WikiContractGovernance, []string) {
	block := wikiContractProjectionYAML(content)
	// v1.1 refines the projection, not the graph/runtime protocol.
	var raw map[string]any
	if yaml.Unmarshal([]byte(block), &raw) == nil && stringMapString(raw, "wiki_contract") == "WIKI-CONTRACT-v1.1" {
		raw["wiki_contract"] = WikiContractProtocolV1
		if normalized, err := yaml.Marshal(raw); err == nil {
			block = string(normalized)
		}
	}
	return ParseWikiContractGovernanceYAML(block)
}

func wikiContractProjectionYAML(content []byte) string {
	if block, err := extractGovernanceYAMLBlock(content); err == nil {
		return block
	}
	text := strings.TrimSpace(string(content))
	if start := strings.Index(text, "{"); start >= 0 {
		if end := jsonObjectEnd(text, start); end > start {
			return text[start:end]
		}
	}
	if strings.HasPrefix(text, "---\n") || strings.HasPrefix(text, "---\r\n") {
		text = strings.TrimSpace(text[3:])
		if end := strings.Index("\n"+text, "\n---"); end >= 0 {
			text = text[:end]
		}
	}
	return text
}

func jsonObjectEnd(text string, start int) int {
	depth, quoted, escaped := 0, false, false
	for i := start; i < len(text); i++ {
		c := text[i]
		if quoted {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				quoted = false
			}
			continue
		}
		if c == '"' {
			quoted = true
			continue
		}
		if c == '{' {
			depth++
		}
		if c == '}' {
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}

func buildWikiContractReadProfile(g *WikiContractGovernance, sourceDoc string) model.DocsReadProfile {
	wikiRoot := path.Dir(filepath.ToSlash(sourceDoc))
	type surface struct {
		kind, family, trait string
		paths               []string
	}
	specs := []surface{
		{"governance", "functional", "", []string{"00_governance.md", "00_retirements.md", "ae/**"}},
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
	// Edit scope is explicit when it differs from read visibility. AE compatibility
	// history remains in the governance read/family surface but is not an edit target.
	editPathsByKind := map[string][]string{
		"governance": {"00_governance.md", "00_retirements.md"},
	}
	excepted := map[string]bool{}
	for _, exception := range g.LayerExceptions {
		excepted[exceptionLayerID(exception)] = true
	}
	source := model.GovernanceSource{Version: 1, Projection: model.GovernanceProjection{
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
		editGlobs := append([]string{}, globs...)
		if rels, ok := editPathsByKind[spec.kind]; ok {
			editGlobs = make([]string, 0, len(rels))
			for _, rel := range rels {
				editGlobs = append(editGlobs, path.Join(wikiRoot, rel))
			}
		}
		surfaces = append(surfaces, model.DocsAgentSurface{Kind: spec.kind, ReadGlobs: globs, EditGlobs: editGlobs})
		source.Hierarchy = append(source.Hierarchy, model.GovernanceHierarchyItem{
			ID: spec.kind, Layer: spec.kind, Family: spec.family, PackStage: spec.kind, Paths: globs,
		})
	}
	profile := buildDocsReadProfileFromGovernance(source, resolvedGovernanceProfile{}, sourceDoc)
	// Hierarchy is only an internal compatibility input to family construction.
	// Contract authority is traits + kinds, never the legacy profile hierarchy.
	profile.Governance.Hierarchy = nil
	profile.ReadingPack.TechnicalStageOrder = []string{"governance", "scope", "architecture"}
	profile.ReadingPack.UXStageOrder = []string{"governance", "scope", "architecture"}
	for _, spec := range specs {
		for _, surface := range surfaces {
			if surface.Kind != spec.kind {
				continue
			}
			if spec.family == "technical" {
				profile.ReadingPack.TechnicalStageOrder = append(profile.ReadingPack.TechnicalStageOrder, spec.kind)
			}
			if spec.family == "ux" {
				profile.ReadingPack.UXStageOrder = append(profile.ReadingPack.UXStageOrder, spec.kind)
			}
		}
	}
	// Broad fallback roots would reintroduce layers removed by traits/exceptions.
	profile.GenericDocs.Paths = []string{"README.md", "README*.md"}
	profile.ProjectionSource = "wiki-contract-v1/traits"
	profile.AgentSurfaces = surfaces
	return profile
}

// encodeWikiContractProjection is deliberately local to the contract projection:
// the legacy runtime profile remains a compatibility read view, not authority.
func encodeWikiContractProjection(profile model.DocsReadProfile, g *WikiContractGovernance, content []byte) ([]byte, error) {
	var metadata struct {
		Language      string                  `yaml:"language"`
		ModelRevision string                  `yaml:"model_revision"`
		AECanon       model.GovernanceAECanon `yaml:"ae_canon"`
	}
	if err := yaml.Unmarshal([]byte(wikiContractProjectionYAML(content)), &metadata); err != nil {
		return nil, err
	}
	language := strings.TrimSpace(metadata.Language)
	if language != "es" && language != "en" {
		return nil, fmt.Errorf("wiki contract projection requires language es|en, got %q", language)
	}
	modelRevision := strings.TrimSpace(metadata.ModelRevision)
	if modelRevision == "" {
		modelRevision = "1.1"
	}
	profile.Governance.Profile = ""
	profile.Governance.EffectiveBase = ""
	profile.Governance.EffectiveOverlays = nil
	profile.Governance.Hierarchy = nil
	profile.Governance.AECanon = metadata.AECanon
	base, err := encodeDocsReadProfile(profile)
	if err != nil {
		return nil, err
	}
	type trait struct {
		Reason  string   `toml:"reason"`
		UIRoots []string `toml:"ui_roots,omitempty"`
	}
	type exception struct {
		Layer        string `toml:"layer"`
		Reason       string `toml:"reason"`
		AuthorizedBy string `toml:"authorized_by"`
		Date         string `toml:"date"`
	}
	tail := struct {
		Traits     map[string]trait `toml:"traits"`
		Exceptions []exception      `toml:"layer_exceptions,omitempty"`
	}{Traits: make(map[string]trait)}
	for name, value := range g.Traits {
		roots, _ := model.SafeRoots(value.UIRoots)
		sort.Strings(roots)
		tail.Traits[name] = trait{Reason: value.Reason, UIRoots: dedupeStrings(roots)}
	}
	for _, value := range g.LayerExceptions {
		tail.Exceptions = append(tail.Exceptions, exception{
			Layer: exceptionLayerID(value), Reason: value.Reason,
			AuthorizedBy: value.AuthorizedBy, Date: value.Date,
		})
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "read_model_contract = %q\nmodel_revision = %q\nlanguage = %q\n", "WIKI-CONTRACT-v1.1", modelRevision, language)
	if len(tail.Exceptions) == 0 {
		buf.WriteString("layer_exceptions = []\n")
	}
	buf.Write(base)
	if err := toml.NewEncoder(&buf).Encode(tail); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func inspectWikiContractProjection(root string, status model.GovernanceStatus, g *WikiContractGovernance, autoSync bool) model.GovernanceStatus {
	profile := buildWikiContractReadProfile(g, status.HumanDoc)
	status.ProjectionDoc = profile.Governance.Projection.Output
	status.Profile = ""
	status.EffectiveBase = ""
	status.WikiContract = g.WikiContract
	status.AECanon = inspectWikiContractAECanon(root, g)
	if status.AECanon.Blocking && (status.AECanon.Status == "missing" || status.AECanon.Status == "mismatch") {
		status.Issues = append(status.Issues, "AE canon is blocked: "+status.AECanon.Reason)
		status.Blocked = true
	}
	status.Sync = "stale"
	status.IndexSync, status.IndexSyncDetails = inspectIndexSync(root)
	if status.IndexSync == "stale" {
		status.Issues = append(status.Issues, "workspace index is stale relative to governance sources; rerun mi-lsp index")
		status.Blocked = true
	}
	content, err := os.ReadFile(absFromDeclared(root, status.HumanDoc))
	var rendered []byte
	if err == nil {
		rendered, err = encodeWikiContractProjection(profile, g, content)
	}
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
	if status.AECanon.Blocking {
		status.Summary = "WIKI-CONTRACT-v1 governance is blocked by AE canon validation (" + status.AECanon.Reason + "); read-model projection is advisory (" + status.Sync + ")."
	}
	return status
}

func inspectWikiContractAECanon(root string, g *WikiContractGovernance) model.AECanonStatus {
	config := model.GovernanceAECanon{}
	present := false
	if g != nil {
		config = g.AECanon
		present = g.AECanonPresent
	}
	if present && strings.TrimSpace(config.Mode) == "" {
		// A declared but incomplete block must not collapse to the optional zero-value mode.
		config.Mode = "invalid"
	}
	return inspectConfiguredAECanon(root, config, nil, "governance")
}
