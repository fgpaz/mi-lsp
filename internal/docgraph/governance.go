package docgraph

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/fgpaz/mi-lsp/internal/model"
)

var governanceYAMLBlockPattern = regexp.MustCompile("(?s)```(?:yaml|yml)\\s*(.*?)\\s*```")

type resolvedGovernanceProfile struct {
	Base     string
	Overlays []string
}

var requiredAECanonModules = []string{
	"README.md",
	"AE-PHASES.md",
	"AE-HARNESS-MANIFEST.md",
	"AE-HARNESS-ORCHESTRATION.md",
	"AE-WORK-MODES.md",
	"AE-SESSION-CONTRACT.md",
	"AE-PROJECTION-POLICY.md",
	"AE-EVIDENCE-POLICY.md",
	"AE-RELEASE-DISTRIBUTION.md",
}

func GovernanceDocPath(root string) string {
	return filepath.Join(root, ".docs", "wiki", "00_gobierno_documental.md")
}

func resolveGovernanceDoc(root string) (string, string, model.DocsReadProfile, string) {
	profile, source, _ := LoadProfile(root)
	displayPath := filepath.ToSlash(filepath.Join(".docs", "wiki", "00_gobierno_documental.md"))
	docPath := filepath.Join(root, filepath.FromSlash(displayPath))
	if source == "project" && strings.TrimSpace(profile.Governance.SourceDoc) != "" {
		if safeDisplayPath, ok := safeGovernanceSourceDoc(root, profile.Governance.SourceDoc); ok {
			displayPath = safeDisplayPath
			docPath = filepath.Join(root, filepath.FromSlash(displayPath))
		} else {
			displayPath = "INVALID:" + filepath.ToSlash(strings.TrimSpace(profile.Governance.SourceDoc))
			docPath = ""
		}
	}
	return docPath, displayPath, profile, source
}

func safeGovernanceSourceDoc(root string, sourceDoc string) (string, bool) {
	candidate := filepath.Clean(filepath.FromSlash(strings.TrimSpace(sourceDoc)))
	if candidate == "." || candidate == "" || filepath.IsAbs(candidate) {
		return "", false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	absCandidate, err := filepath.Abs(filepath.Join(absRoot, candidate))
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(absRoot, absCandidate)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", false
	}
	normalizedRel := filepath.ToSlash(rel)
	if !strings.HasPrefix(normalizedRel, ".docs/wiki/") {
		return "", false
	}
	return normalizedRel, true
}

func isKnowledgeWikiProjection(profile model.DocsReadProfile, source string) bool {
	if source != "project" {
		return false
	}
	sourceFormat := strings.ToLower(strings.TrimSpace(profile.Governance.SourceFormat))
	profileName := strings.ToLower(strings.TrimSpace(profile.Governance.Profile))
	effectiveBase := strings.ToLower(strings.TrimSpace(profile.Governance.EffectiveBase))
	return sourceFormat == "markdown" && (profileName == "knowledge-wiki" || effectiveBase == "knowledge-wiki")
}

func InspectGovernance(root string, autoSync bool) model.GovernanceStatus {
	docPath, humanDoc, projectionProfile, projectionSource := resolveGovernanceDoc(root)
	status := model.GovernanceStatus{
		HumanDoc:      humanDoc,
		ProjectionDoc: filepath.ToSlash(filepath.Join(".docs", "wiki", "_mi-lsp", "read-model.toml")),
		AllowedActions: []string{
			"mi-lsp nav governance --workspace <alias> --format toon",
			"mi-lsp index --workspace <alias>",
		},
	}
	if strings.HasPrefix(humanDoc, "INVALID:") {
		status.Sync = "invalid"
		status.IndexSync, status.IndexSyncDetails = inspectIndexSync(root)
		status.AECanon = inspectAECanonFromProjection(root, "governance_invalid")
		status.Issues = []string{"invalid governance source_doc; it must be a relative path under .docs/wiki/ and stay inside the workspace"}
		status.Blocked = true
		status.NextSteps = governanceRepairStepsFor(filepath.ToSlash(filepath.Join(".docs", "wiki", "00_gobierno_documental.md")), status.ProjectionDoc)
		status.Summary = "Governance is blocked because the read-model source_doc points outside the governed wiki boundary."
		return status
	}

	content, err := os.ReadFile(docPath)
	if err != nil {
		status.Sync = "missing"
		status.IndexSync, status.IndexSyncDetails = inspectIndexSync(root)
		status.AECanon = inspectAECanonFromProjection(root, "governance_missing")
		status.Issues = []string{"missing " + status.HumanDoc}
		status.Blocked = true
		status.NextSteps = governanceRepairStepsFor(status.HumanDoc, status.ProjectionDoc)
		status.Summary = "Governance is blocked because the human governance document is missing."
		return status
	}

	if isKnowledgeWikiProjection(projectionProfile, projectionSource) {
		return inspectKnowledgeWikiGovernance(root, status, projectionProfile)
	}

	block, err := extractGovernanceYAMLBlock(content)
	if err != nil {
		status.Sync = "invalid"
		status.IndexSync, status.IndexSyncDetails = inspectIndexSync(root)
		status.AECanon = inspectAECanonFromProjection(root, "governance_invalid")
		status.Issues = []string{"missing fenced YAML governance block in " + status.HumanDoc}
		status.Blocked = true
		status.NextSteps = governanceRepairStepsFor(status.HumanDoc, status.ProjectionDoc)
		status.Summary = "Governance is blocked because the human governance document has no machine-readable YAML source."
		return status
	}

	var source model.GovernanceSource
	if err := yaml.Unmarshal([]byte(block), &source); err != nil {
		status.Sync = "invalid"
		status.IndexSync, status.IndexSyncDetails = inspectIndexSync(root)
		status.AECanon = inspectAECanonFromProjection(root, "governance_invalid")
		status.Issues = []string{fmt.Sprintf("invalid governance YAML: %v", err)}
		status.Blocked = true
		status.NextSteps = governanceRepairStepsFor(status.HumanDoc, status.ProjectionDoc)
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
	status.AECanon = inspectAECanonFromHierarchy(root, source.Hierarchy, "governance")

	if len(issues) > 0 {
		status.Sync = "invalid"
		status.IndexSync, status.IndexSyncDetails = inspectIndexSync(root)
		status.Issues = issues
		status.Blocked = true
		status.NextSteps = governanceRepairStepsFor(status.HumanDoc, status.ProjectionDoc)
		status.Summary = "Governance is blocked because the YAML source is incomplete or contradictory."
		return status
	}

	profile := buildDocsReadProfileFromGovernance(source, resolved)
	rendered, err := encodeDocsReadProfile(profile)
	if err != nil {
		status.Sync = "invalid"
		status.IndexSync, status.IndexSyncDetails = inspectIndexSync(root)
		status.Issues = []string{fmt.Sprintf("failed to render read-model projection: %v", err)}
		status.Blocked = true
		status.NextSteps = governanceRepairStepsFor(status.HumanDoc, status.ProjectionDoc)
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
			status.IndexSync, status.IndexSyncDetails = inspectIndexSync(root)
			status.Issues = []string{fmt.Sprintf("failed to create read-model directory: %v", err)}
			status.Blocked = true
			status.NextSteps = governanceRepairStepsFor(status.HumanDoc, status.ProjectionDoc)
			status.Summary = "Governance is blocked because read-model projection could not be written."
			return status
		}
		if err := os.WriteFile(projectionAbs, rendered, 0o644); err != nil {
			status.Sync = "invalid"
			status.IndexSync, status.IndexSyncDetails = inspectIndexSync(root)
			status.Issues = []string{fmt.Sprintf("failed to write read-model projection: %v", err)}
			status.Blocked = true
			status.NextSteps = governanceRepairStepsFor(status.HumanDoc, status.ProjectionDoc)
			status.Summary = "Governance is blocked because read-model projection could not be written."
			return status
		}
		status.Sync = "auto_synced"
		status.Warnings = append(status.Warnings, "read-model.toml auto-synced from 00_gobierno_documental.md")
	default:
		status.Sync = "stale"
		status.Issues = append(status.Issues, "read-model.toml is out of sync with 00_gobierno_documental.md")
	}

	status.IndexSync, status.IndexSyncDetails = inspectIndexSync(root)
	if status.IndexSync == "stale" {
		status.Issues = append(status.Issues, "workspace index is stale relative to governance sources; rerun mi-lsp index")
	}
	if status.AECanon.Blocking && (status.AECanon.Status == "missing" || status.AECanon.Status == "mismatch") {
		status.Issues = append(status.Issues, "AE canon is blocked: "+status.AECanon.Reason)
	}

	status.Blocked = len(status.Issues) > 0
	if status.Blocked {
		status.NextSteps = governanceRepairStepsFor(status.HumanDoc, status.ProjectionDoc)
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

func inspectKnowledgeWikiGovernance(root string, status model.GovernanceStatus, profile model.DocsReadProfile) model.GovernanceStatus {
	status.Profile = firstNonEmpty(profile.Governance.Profile, "knowledge-wiki")
	status.Extends = profile.Governance.Extends
	status.EffectiveBase = firstNonEmpty(profile.Governance.EffectiveBase, status.Profile)
	status.EffectiveOverlays = append([]string{}, profile.Governance.EffectiveOverlays...)
	status.ContextChain = append([]string{}, profile.Governance.ContextChain...)
	status.ClosureChain = append([]string{}, profile.Governance.ClosureChain...)
	status.AuditChain = append([]string{}, profile.Governance.AuditChain...)
	status.BlockingRules = append([]string{}, profile.Governance.BlockingRules...)
	status.NumberingRecommended = profile.Governance.NumberingRecommended
	status.Sync = "in_sync"
	status.AECanon = inspectAECanonFromHierarchy(root, profile.Governance.Hierarchy, "read_model")
	if len(profile.Governance.Hierarchy) == 0 && status.AECanon.Status == "not_applicable" {
		status.AECanon = inspectAECanonFromProjection(root, "knowledge_wiki")
	}

	status.IndexSync, status.IndexSyncDetails = inspectIndexSync(root)
	if status.IndexSync == "stale" {
		status.Issues = append(status.Issues, "workspace index is stale relative to governance sources; rerun mi-lsp index")
	}
	if status.AECanon.Blocking && (status.AECanon.Status == "missing" || status.AECanon.Status == "mismatch") {
		status.Issues = append(status.Issues, "AE canon is blocked: "+status.AECanon.Reason)
	}

	status.Blocked = len(status.Issues) > 0
	if status.Blocked {
		status.NextSteps = governanceRepairStepsFor(status.HumanDoc, status.ProjectionDoc)
		status.Summary = "Governance is blocked until the knowledge-wiki source, projection and index are consistent."
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

func inspectAECanonFromProjection(root string, reason string) model.AECanonStatus {
	profile, source, _ := LoadProfile(root)
	if source == "project" {
		roots := declaredAECanonRoots(profile.Governance.Hierarchy)
		if len(roots) > 0 {
			status := inspectAECanonRoots(root, roots, "read_model", reason+"_read_model_projection")
			status.Status = "projection_only"
			// Only block if the workspace is actually declaring AE in its governance
			// (i.e., the governance is valid and declares it). When governance itself is invalid,
			// don't block on AE to allow repair of governance first.
			status.Blocking = false
			status.Reason = reason + "_read_model_projection_only"
			return status
		}
	}
	if directoryExists(filepath.Join(root, ".docs", "wiki", "ae")) {
		return inspectAECanonRoots(root, []string{filepath.ToSlash(filepath.Join(".docs", "wiki", "ae"))}, "fallback", reason+"_fallback")
	}
	return model.AECanonStatus{
		Status:          "missing",
		Source:          "fallback",
		RequiredModules: append([]string{}, requiredAECanonModules...),
		MissingModules:  append([]string{}, requiredAECanonModules...),
		Blocking:        false,
		Reason:          reason,
	}
}

func inspectAECanonFromHierarchy(root string, hierarchy []model.GovernanceHierarchyItem, source string) model.AECanonStatus {
	roots := declaredAECanonRoots(hierarchy)
	if len(roots) > 0 {
		return inspectAECanonRoots(root, roots, source, source+"_declared")
	}
	fallbackRoot := filepath.ToSlash(filepath.Join(".docs", "wiki", "ae"))
	if directoryExists(filepath.Join(root, filepath.FromSlash(fallbackRoot))) {
		return inspectAECanonRoots(root, []string{fallbackRoot}, "fallback", "fallback_docs_wiki_ae")
	}
	return model.AECanonStatus{
		Status:          "not_applicable",
		Source:          "fallback",
		RequiredModules: append([]string{}, requiredAECanonModules...),
		Blocking:        false,
		Reason:          "no_ae_canon_declared",
	}
}

func inspectAECanonRoots(root string, roots []string, source string, reason string) model.AECanonStatus {
	roots = dedupeStrings(normalizeAECanonRoots(roots))
	for _, declaredRoot := range roots {
		if redirectedRoot := aeCanonRedirectRoot(root, declaredRoot); redirectedRoot != "" {
			status := inspectAECanonRoot(root, redirectedRoot, "redirect", "readme_redirect")
			if status.Status == "valid" {
				return status
			}
			if status.Status == "missing" {
				status.Status = "mismatch"
				status.Reason = "readme_redirect_missing_modules"
			}
			return status
		}
	}
	best := model.AECanonStatus{
		Status:          "missing",
		Roots:           append([]string{}, roots...),
		Source:          source,
		RequiredModules: append([]string{}, requiredAECanonModules...),
		Blocking:        true,
		Reason:          reason + "_missing_modules",
	}
	for _, declaredRoot := range roots {
		status := inspectAECanonRoot(root, declaredRoot, source, reason)
		if status.Status == "valid" {
			return status
		}
		if len(best.MissingModules) == 0 || len(status.MissingModules) < len(best.MissingModules) {
			best = status
		}
	}
	return best
}

func inspectAECanonRoot(root string, aeRoot string, source string, reason string) model.AECanonStatus {
	aeRoot = filepath.ToSlash(strings.Trim(strings.TrimSpace(aeRoot), "/"))
	missing := make([]string, 0)
	for _, module := range requiredAECanonModules {
		rel := filepath.ToSlash(filepath.Join(aeRoot, module))
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			missing = append(missing, rel)
		}
	}
	status := model.AECanonStatus{
		Status:          "valid",
		Roots:           []string{aeRoot},
		Source:          source,
		RequiredModules: append([]string{}, requiredAECanonModules...),
		MissingModules:  missing,
		Blocking:        false,
		Reason:          reason + "_valid",
	}
	if len(missing) > 0 {
		status.Status = "missing"
		status.Blocking = true
		status.Reason = reason + "_missing_modules"
	}
	return status
}

func declaredAECanonRoots(hierarchy []model.GovernanceHierarchyItem) []string {
	roots := make([]string, 0)
	for _, item := range hierarchy {
		itemDeclaresAE := governanceItemDeclaresAE(item)
		for _, path := range item.Paths {
			if !itemDeclaresAE && !pathLooksLikeAERoot(path) {
				continue
			}
			if root := aeRootFromPattern(path); root != "" {
				roots = append(roots, root)
			}
		}
	}
	return dedupeStrings(normalizeAECanonRoots(roots))
}

func governanceItemDeclaresAE(item model.GovernanceHierarchyItem) bool {
	layer := strings.ToUpper(strings.TrimSpace(item.Layer))
	id := strings.ToLower(strings.TrimSpace(item.ID))
	label := strings.ToLower(strings.TrimSpace(item.Label))
	return layer == "AE" ||
		strings.Contains(id, "agent_engineering") ||
		strings.Contains(id, "agent-engineering") ||
		strings.Contains(label, "agent engineering")
}

func pathLooksLikeAERoot(path string) bool {
	normalized := "/" + strings.ToLower(filepath.ToSlash(strings.TrimSpace(path))) + "/"
	return strings.Contains(normalized, "/ae/") ||
		strings.Contains(normalized, "/wiki/ae/") ||
		strings.Contains(normalized, "/.docs/ae/")
}

func aeRootFromPattern(path string) string {
	normalized := filepath.ToSlash(strings.TrimSpace(path))
	if normalized == "" {
		return ""
	}
	normalized = strings.TrimSuffix(normalized, "/")
	if strings.ContainsAny(normalized, "*?[") {
		parts := strings.Split(normalized, "/")
		kept := make([]string, 0, len(parts))
		for _, part := range parts {
			if strings.ContainsAny(part, "*?[") {
				break
			}
			kept = append(kept, part)
		}
		return strings.Trim(strings.Join(kept, "/"), "/")
	}
	if strings.HasSuffix(strings.ToLower(normalized), ".md") {
		return strings.Trim(filepath.ToSlash(filepath.Dir(normalized)), "/")
	}
	return strings.Trim(normalized, "/")
}

func normalizeAECanonRoots(roots []string) []string {
	out := make([]string, 0, len(roots))
	for _, root := range roots {
		normalized := filepath.ToSlash(strings.Trim(strings.TrimSpace(root), "/"))
		if normalized == "." || normalized == "" {
			continue
		}
		out = append(out, normalized)
	}
	return out
}

func aeCanonRedirectRoot(root string, declaredRoot string) string {
	readmePath := filepath.Join(root, filepath.FromSlash(declaredRoot), "README.md")
	content, err := os.ReadFile(readmePath)
	if err != nil {
		return ""
	}
	text := strings.ToLower(filepath.ToSlash(string(content)))
	candidates := []string{
		".docs/ae/README.md",
		".docs/ae",
		".docs/wiki/ae/README.md",
		".docs/wiki/ae",
		"wiki/ae/README.md",
		"wiki/ae",
	}
	for _, candidate := range candidates {
		if !containsAEPathMention(text, strings.ToLower(candidate)) {
			continue
		}
		target := aeRootFromPattern(candidate)
		if target != "" && target != declaredRoot {
			return target
		}
	}
	return ""
}

func containsAEPathMention(text string, candidate string) bool {
	if !strings.HasPrefix(candidate, "wiki/ae") {
		return strings.Contains(text, candidate)
	}
	for _, prefix := range []string{"`", " ", "\n", "\r", "\t", "(", "[", ":"} {
		if strings.Contains(text, prefix+candidate) {
			return true
		}
	}
	return strings.HasPrefix(text, candidate)
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
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
	for idx, hint := range source.OwnerHints {
		if len(dedupeStrings(hint.Terms)) == 0 {
			issues = append(issues, fmt.Sprintf("owner_hints[%d].terms must include at least one term", idx))
		}
		if len(dedupeStrings(hint.PreferDocIDs)) == 0 &&
			len(dedupeStrings(hint.PreferPaths)) == 0 &&
			len(dedupeStrings(hint.PreferFamilies)) == 0 &&
			len(dedupeStrings(hint.PreferLayers)) == 0 {
			issues = append(issues, fmt.Sprintf("owner_hints[%d] must include at least one prefer_* target", idx))
		}
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
			FunctionalStageOrder: stageOrderForFamily(source.Hierarchy, "functional", []string{"governance", "scope", "outcome", "architecture", "flow", "requirements", "data", "tests"}),
			TechnicalStageOrder:  []string{"governance", "scope", "architecture", "technical_baseline", "technical_detail", "physical_data", "contracts"},
			UXStageOrder:         []string{"governance", "scope", "architecture", "ux_global", "ux_research", "ux_spec", "ux_handoff"},
		},
		OwnerHints: normalizeOwnerHints(source.OwnerHints),
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

func stageOrderForFamily(items []model.GovernanceHierarchyItem, family string, fallback []string) []string {
	stages := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		if strings.TrimSpace(item.Family) != family {
			continue
		}
		stage := strings.TrimSpace(item.PackStage)
		if stage == "" {
			stage = strings.TrimSpace(item.ID)
		}
		if stage == "" {
			continue
		}
		if _, exists := seen[stage]; exists {
			continue
		}
		seen[stage] = struct{}{}
		stages = append(stages, stage)
	}
	if len(stages) == 0 {
		return append([]string{}, fallback...)
	}
	return stages
}

func normalizeOwnerHints(hints []model.DocsOwnerHint) []model.DocsOwnerHint {
	if len(hints) == 0 {
		return nil
	}
	normalized := make([]model.DocsOwnerHint, 0, len(hints))
	for _, hint := range hints {
		entry := model.DocsOwnerHint{
			Terms:          dedupeStrings(hint.Terms),
			PreferDocIDs:   dedupeStrings(hint.PreferDocIDs),
			PreferPaths:    dedupeStrings(hint.PreferPaths),
			PreferFamilies: dedupeStrings(hint.PreferFamilies),
			PreferLayers:   dedupeStrings(hint.PreferLayers),
		}
		if len(entry.Terms) == 0 {
			continue
		}
		if len(entry.PreferDocIDs) == 0 &&
			len(entry.PreferPaths) == 0 &&
			len(entry.PreferFamilies) == 0 &&
			len(entry.PreferLayers) == 0 {
			continue
		}
		normalized = append(normalized, entry)
	}
	return normalized
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
		keywords := []string{"technical", "governance", "runtime", "backend", "contract", "protocol", "search", "context", "refs", "service", "index", "routing", "ae", "agent engineering", "release", "distribution", "binary", "install", resolved.Base}
		return dedupeStrings(append(keywords, resolved.Overlays...))
	case "ux":
		return []string{"ux", "ui", "frontend", "visual", "design", "journey", "experience", "pattern", "interface", "governance"}
	default:
		keywords := []string{"scope", "outcome", "result", "resultado", "solucion", "solution", "flow", "feature", "behavior", "rs", "rf", "fl", "test", "workflow", "governance", resolved.Base}
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
	return governanceRepairStepsFor(filepath.ToSlash(filepath.Join(".docs", "wiki", "00_gobierno_documental.md")), projectionPath)
}

func governanceRepairStepsFor(humanDoc string, projectionPath string) []string {
	return []string{
		"repair " + humanDoc,
		"verify the governance source is complete for its declared source_format",
		"rerun mi-lsp nav governance --workspace <alias> --format toon",
		"rerun mi-lsp index --workspace <alias> once the projection is stable",
		fmt.Sprintf("confirm %s stays versioned and in sync", projectionPath),
	}
}

func indexSyncState(root string) string {
	state, _ := inspectIndexSync(root)
	return state
}

func inspectIndexSync(root string) (string, *model.GovernanceIndexSyncDetails) {
	indexPath := filepath.Join(root, ".mi-lsp", "index.db")
	details := &model.GovernanceIndexSyncDetails{
		IndexPath: displayPath(root, indexPath),
	}
	indexInfo, err := os.Stat(indexPath)
	if err != nil {
		details.Reason = "index database is missing"
		governanceDocPath, _, _, _ := resolveGovernanceDoc(root)
		for _, path := range []string{governanceDocPath, ProfilePath(root)} {
			details.ComparedPaths = append(details.ComparedPaths, governanceComparedPath(root, path, time.Time{}))
		}
		return "missing", details
	}
	latest := indexInfo.ModTime()
	details.IndexModTime = latest.UTC().Format(time.RFC3339Nano)
	state := "current"
	governanceDocPath, _, _, _ := resolveGovernanceDoc(root)
	for _, path := range []string{governanceDocPath, ProfilePath(root)} {
		compared := governanceComparedPath(root, path, latest)
		details.ComparedPaths = append(details.ComparedPaths, compared)
		if compared.NewerThanIndex {
			state = "stale"
		}
	}
	if state == "stale" {
		details.Reason = "governance source is newer than workspace index"
	} else {
		details.Reason = "workspace index is current relative to governance sources"
	}
	return state, details
}

func governanceComparedPath(root string, path string, indexModTime time.Time) model.GovernanceIndexComparedPath {
	item := model.GovernanceIndexComparedPath{Path: displayPath(root, path)}
	info, err := os.Stat(path)
	if err != nil {
		item.Missing = true
		return item
	}
	item.ModTime = info.ModTime().UTC().Format(time.RFC3339Nano)
	if !indexModTime.IsZero() && info.ModTime().After(indexModTime) {
		item.NewerThanIndex = true
	}
	return item
}

func displayPath(root string, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.Clean(path)
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
