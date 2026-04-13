package docgraph

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/fgpaz/mi-lsp/internal/model"
)

var governanceYAMLBlockPattern = regexp.MustCompile("(?s)```(?:yaml|yml)\\s*(.*?)\\s*```")

type resolvedGovernanceProfile struct {
	Base     string
	Overlays []string
}

func GovernanceDocPath(root string) string {
	return filepath.Join(root, ".docs", "wiki", "00_gobierno_documental.md")
}

func InspectGovernance(root string, autoSync bool) model.GovernanceStatus {
	status := model.GovernanceStatus{
		HumanDoc:      filepath.ToSlash(filepath.Join(".docs", "wiki", "00_gobierno_documental.md")),
		ProjectionDoc: filepath.ToSlash(filepath.Join(".docs", "wiki", "_mi-lsp", "read-model.toml")),
		AllowedActions: []string{
			"mi-lsp nav governance --workspace <alias> --format toon",
			"mi-lsp index --workspace <alias>",
		},
	}

	docPath := GovernanceDocPath(root)
	content, err := os.ReadFile(docPath)
	if err != nil {
		status.Sync = "missing"
		status.IndexSync = indexSyncState(root)
		status.Issues = []string{"missing .docs/wiki/00_gobierno_documental.md"}
		status.Blocked = true
		status.NextSteps = governanceRepairSteps(status.ProjectionDoc)
		status.Summary = "Governance is blocked because the human governance document is missing."
		return status
	}

	block, err := extractGovernanceYAMLBlock(content)
	if err != nil {
		status.Sync = "invalid"
		status.IndexSync = indexSyncState(root)
		status.Issues = []string{"missing fenced YAML governance block in 00_gobierno_documental.md"}
		status.Blocked = true
		status.NextSteps = governanceRepairSteps(status.ProjectionDoc)
		status.Summary = "Governance is blocked because 00_gobierno_documental.md has no machine-readable YAML source."
		return status
	}

	var source model.GovernanceSource
	if err := yaml.Unmarshal([]byte(block), &source); err != nil {
		status.Sync = "invalid"
		status.IndexSync = indexSyncState(root)
		status.Issues = []string{fmt.Sprintf("invalid governance YAML: %v", err)}
		status.Blocked = true
		status.NextSteps = governanceRepairSteps(status.ProjectionDoc)
		status.Summary = "Governance is blocked because the YAML source inside 00_gobierno_documental.md is invalid."
		return status
	}
	if source.Version == 0 {
		source.Version = 1
	}
	if strings.TrimSpace(source.Projection.Output) == "" {
		source.Projection.Output = status.ProjectionDoc
	}
	if strings.TrimSpace(source.Projection.Format) == "" {
		source.Projection.Format = "toml"
	}

	resolved, issues := validateAndResolveGovernance(source)
	status.Profile = source.Profile
	status.Extends = source.Extends
	status.EffectiveBase = resolved.Base
	status.EffectiveOverlays = append([]string{}, resolved.Overlays...)
	status.ContextChain = append([]string{}, source.ContextChain...)
	status.ClosureChain = append([]string{}, source.ClosureChain...)
	status.AuditChain = append([]string{}, source.AuditChain...)
	status.BlockingRules = append([]string{}, source.BlockingRules...)
	status.NumberingRecommended = source.NumberingRecommended

	if len(issues) > 0 {
		status.Sync = "invalid"
		status.IndexSync = indexSyncState(root)
		status.Issues = issues
		status.Blocked = true
		status.NextSteps = governanceRepairSteps(status.ProjectionDoc)
		status.Summary = "Governance is blocked because the YAML source is incomplete or contradictory."
		return status
	}

	profile := buildDocsReadProfileFromGovernance(source, resolved)
	rendered, err := encodeDocsReadProfile(profile)
	if err != nil {
		status.Sync = "invalid"
		status.IndexSync = indexSyncState(root)
		status.Issues = []string{fmt.Sprintf("failed to render read-model projection: %v", err)}
		status.Blocked = true
		status.NextSteps = governanceRepairSteps(status.ProjectionDoc)
		status.Summary = "Governance is blocked because read-model projection could not be rendered."
		return status
	}

	projectionAbs := ProfilePath(root)
	projectionBytes, readErr := os.ReadFile(projectionAbs)
	switch {
	case readErr == nil && normalizedText(projectionBytes) == normalizedText(rendered):
		status.Sync = "in_sync"
	case autoSync:
		if err := os.MkdirAll(filepath.Dir(projectionAbs), 0o755); err != nil {
			status.Sync = "invalid"
			status.IndexSync = indexSyncState(root)
			status.Issues = []string{fmt.Sprintf("failed to create read-model directory: %v", err)}
			status.Blocked = true
			status.NextSteps = governanceRepairSteps(status.ProjectionDoc)
			status.Summary = "Governance is blocked because read-model projection could not be written."
			return status
		}
		if err := os.WriteFile(projectionAbs, rendered, 0o644); err != nil {
			status.Sync = "invalid"
			status.IndexSync = indexSyncState(root)
			status.Issues = []string{fmt.Sprintf("failed to write read-model projection: %v", err)}
			status.Blocked = true
			status.NextSteps = governanceRepairSteps(status.ProjectionDoc)
			status.Summary = "Governance is blocked because read-model projection could not be written."
			return status
		}
		status.Sync = "auto_synced"
		status.Warnings = append(status.Warnings, "read-model.toml auto-synced from 00_gobierno_documental.md")
	default:
		status.Sync = "stale"
		status.Issues = append(status.Issues, "read-model.toml is out of sync with 00_gobierno_documental.md")
	}

	status.IndexSync = indexSyncState(root)
	if status.IndexSync == "stale" {
		status.Issues = append(status.Issues, "workspace index is stale relative to governance sources; rerun mi-lsp index")
	}

	status.Blocked = len(status.Issues) > 0
	if status.Blocked {
		status.NextSteps = governanceRepairSteps(status.ProjectionDoc)
		status.Summary = "Governance is blocked until the projection and the workspace index are both consistent."
		return status
	}

	status.NextSteps = []string{
		"mi-lsp nav route \"how is this workspace organized?\" --workspace <alias> --format toon",
		"mi-lsp nav ask \"how is this workspace organized?\" --workspace <alias> --format toon",
		"mi-lsp nav pack \"understand this task\" --workspace <alias> --format toon",
	}
	status.Summary = fmt.Sprintf("Governance is valid for profile %s and the projection is ready.", status.Profile)
	return status
}

func extractGovernanceYAMLBlock(content []byte) (string, error) {
	matches := governanceYAMLBlockPattern.FindSubmatch(content)
	if len(matches) < 2 {
		return "", fmt.Errorf("yaml block not found")
	}
	return strings.TrimSpace(string(matches[1])), nil
}

func validateAndResolveGovernance(source model.GovernanceSource) (resolvedGovernanceProfile, []string) {
	issues := []string{}
	profile := strings.TrimSpace(source.Profile)
	if profile == "" {
		issues = append(issues, "profile is required")
	}

	if strings.TrimSpace(source.Projection.Output) != filepath.ToSlash(filepath.Join(".docs", "wiki", "_mi-lsp", "read-model.toml")) {
		issues = append(issues, "projection.output must be .docs/wiki/_mi-lsp/read-model.toml")
	}
	if strings.TrimSpace(strings.ToLower(source.Projection.Format)) != "toml" {
		issues = append(issues, "projection.format must be toml")
	}
	if !source.Projection.AutoSync {
		issues = append(issues, "projection.auto_sync must be true")
	}
	if !source.Projection.Versioned {
		issues = append(issues, "projection.versioned must be true")
	}

	itemByID := make(map[string]model.GovernanceHierarchyItem, len(source.Hierarchy))
	for _, item := range source.Hierarchy {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			issues = append(issues, "hierarchy items require id")
			continue
		}
		if _, exists := itemByID[id]; exists {
			issues = append(issues, "duplicate hierarchy id: "+id)
		}
		if strings.TrimSpace(item.Layer) == "" {
			issues = append(issues, "hierarchy."+id+".layer is required")
		}
		if strings.TrimSpace(item.Family) == "" {
			issues = append(issues, "hierarchy."+id+".family is required")
		}
		if len(item.Paths) == 0 {
			issues = append(issues, "hierarchy."+id+".paths must include at least one path")
		}
		itemByID[id] = item
	}
	if len(source.Hierarchy) == 0 {
		issues = append(issues, "hierarchy must include at least one declared layer")
	}
	if len(source.ContextChain) == 0 {
		issues = append(issues, "context_chain is required")
	}
	if len(source.ClosureChain) == 0 {
		issues = append(issues, "closure_chain is required")
	}
	if len(source.AuditChain) == 0 {
		issues = append(issues, "audit_chain is required")
	}
	if len(source.BlockingRules) == 0 {
		issues = append(issues, "blocking_rules is required")
	}

	for _, chain := range [][]string{source.ContextChain, source.ClosureChain, source.AuditChain} {
		for _, id := range chain {
			if _, ok := itemByID[id]; !ok {
				issues = append(issues, "chain references unknown hierarchy id: "+id)
			}
		}
	}

	resolved, resolveIssues := resolveGovernanceProfile(profile, strings.TrimSpace(source.Extends), source.Overlays)
	issues = append(issues, resolveIssues...)
	return resolved, dedupeStrings(issues)
}

func resolveGovernanceProfile(profile string, extends string, overlays []string) (resolvedGovernanceProfile, []string) {
	switch profile {
	case "ordered_wiki":
		return resolvedGovernanceProfile{
			Base:     "ordered_wiki",
			Overlays: dedupeStrings(append([]string{}, overlays...)),
		}, nil
	case "spec_backend":
		return resolvedGovernanceProfile{
			Base:     "ordered_wiki",
			Overlays: dedupeStrings(append([]string{"spec_core", "technical"}, overlays...)),
		}, nil
	case "spec_full":
		return resolvedGovernanceProfile{
			Base:     "ordered_wiki",
			Overlays: dedupeStrings(append([]string{"spec_core", "technical", "uxui"}, overlays...)),
		}, nil
	case "custom":
		if extends == "" {
			return resolvedGovernanceProfile{}, []string{"custom profile requires extends"}
		}
		parent, issues := resolveGovernanceProfile(extends, "", nil)
		if len(issues) > 0 {
			return resolvedGovernanceProfile{}, append([]string{"custom extends invalid profile: " + extends}, issues...)
		}
		return resolvedGovernanceProfile{
			Base:     parent.Base,
			Overlays: dedupeStrings(append(parent.Overlays, overlays...)),
		}, nil
	default:
		return resolvedGovernanceProfile{}, []string{"unsupported governance profile: " + profile}
	}
}

func buildDocsReadProfileFromGovernance(source model.GovernanceSource, resolved resolvedGovernanceProfile) model.DocsReadProfile {
	families := make([]model.DocsReadFamily, 0, 3)
	for _, familyName := range []string{"functional", "technical", "ux"} {
		paths := hierarchyPathsForFamily(source.Hierarchy, familyName)
		if len(paths) == 0 {
			continue
		}
		families = append(families, model.DocsReadFamily{
			Name:           familyName,
			IntentKeywords: defaultIntentKeywords(familyName, resolved),
			Paths:          paths,
		})
	}

	return model.DocsReadProfile{
		Version:  1,
		Families: families,
		GenericDocs: model.DocsGenericFallback{
			Paths: []string{"README.md", "README*.md", "docs/", ".docs/"},
		},
		ReadingPack: model.DocsReadingPackProfile{
			MaxDocs:              7,
			FunctionalStageOrder: []string{"governance", "scope", "architecture", "flow", "requirements", "data", "tests"},
			TechnicalStageOrder:  []string{"governance", "scope", "architecture", "technical_baseline", "technical_detail", "physical_data", "contracts"},
			UXStageOrder:         []string{"governance", "scope", "architecture", "ux_global", "ux_research", "ux_spec", "ux_handoff"},
		},
		Governance: model.DocsGovernanceProfile{
			SourceDoc:            filepath.ToSlash(filepath.Join(".docs", "wiki", "00_gobierno_documental.md")),
			SourceFormat:         "markdown+yaml",
			Profile:              source.Profile,
			Extends:              source.Extends,
			EffectiveBase:        resolved.Base,
			EffectiveOverlays:    append([]string{}, resolved.Overlays...),
			ContextChain:         append([]string{}, source.ContextChain...),
			ClosureChain:         append([]string{}, source.ClosureChain...),
			AuditChain:           append([]string{}, source.AuditChain...),
			BlockingRules:        append([]string{}, source.BlockingRules...),
			NumberingRecommended: source.NumberingRecommended,
			Projection:           source.Projection,
			Hierarchy:            append([]model.GovernanceHierarchyItem(nil), source.Hierarchy...),
		},
	}
}

func hierarchyPathsForFamily(items []model.GovernanceHierarchyItem, family string) []string {
	seen := map[string]struct{}{}
	paths := make([]string, 0)
	for _, item := range items {
		if item.Family != family {
			continue
		}
		for _, path := range item.Paths {
			path = filepath.ToSlash(strings.TrimSpace(path))
			if path == "" {
				continue
			}
			if _, exists := seen[path]; exists {
				continue
			}
			seen[path] = struct{}{}
			paths = append(paths, path)
		}
	}
	return paths
}

func defaultIntentKeywords(family string, resolved resolvedGovernanceProfile) []string {
	switch family {
	case "technical":
		keywords := []string{"technical", "governance", "runtime", "backend", "contract", "protocol", "search", "context", "refs", "service", "index", "routing", resolved.Base}
		return dedupeStrings(append(keywords, resolved.Overlays...))
	case "ux":
		return []string{"ux", "ui", "frontend", "visual", "design", "journey", "experience", "pattern", "interface", "governance"}
	default:
		keywords := []string{"scope", "flow", "feature", "behavior", "rf", "fl", "test", "workflow", "governance", resolved.Base}
		return dedupeStrings(append(keywords, resolved.Overlays...))
	}
}

func encodeDocsReadProfile(profile model.DocsReadProfile) ([]byte, error) {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(profile); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func normalizedText(content []byte) string {
	return strings.ReplaceAll(string(content), "\r\n", "\n")
}

func dedupeStrings(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func governanceRepairSteps(projectionPath string) []string {
	return []string{
		"repair .docs/wiki/00_gobierno_documental.md",
		"verify the fenced YAML governance block is complete",
		"rerun mi-lsp nav governance --workspace <alias> --format toon",
		"rerun mi-lsp index --workspace <alias> once the projection is stable",
		fmt.Sprintf("confirm %s stays versioned and in sync", projectionPath),
	}
}

func indexSyncState(root string) string {
	indexPath := filepath.Join(root, ".mi-lsp", "index.db")
	indexInfo, err := os.Stat(indexPath)
	if err != nil {
		return "missing"
	}
	latest := indexInfo.ModTime()
	for _, path := range []string{GovernanceDocPath(root), ProfilePath(root)} {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.ModTime().After(latest) {
			return "stale"
		}
	}
	return "current"
}

func GovernanceReadinessSummary(status model.GovernanceStatus) string {
	if status.Blocked {
		return status.Summary
	}
	return fmt.Sprintf("profile=%s base=%s sync=%s index=%s", status.Profile, status.EffectiveBase, status.Sync, status.IndexSync)
}

func sortedGovernanceIssues(status model.GovernanceStatus) []string {
	items := append([]string{}, status.Issues...)
	sort.Strings(items)
	return items
}
