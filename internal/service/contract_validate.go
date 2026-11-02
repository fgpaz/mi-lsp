package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/wikisource"
)

const wikiContractProtocol = docgraph.WikiContractProtocolV1

var wikiContractForbiddenKeys = []string{
	"bounds", "imports", "exports", "planned", "doc_id", "harness_protocol",
	"wiki_source_protocol", "wiki_graph_protocol", "harness_protocol",
	"agent_must_read", "agent_may_edit", "agent_must_not_edit", "verify", "stop_if", "evidence",
	"audience", "business_anchor", "block_id", "source_of_truth",
}

var wikiContractKinds = map[string]struct{}{
	"governance": {}, "scope": {}, "outcome": {}, "architecture": {}, "decision": {},
	"flow": {}, "requirement": {}, "data_model": {}, "test": {}, "technical": {},
	"database": {}, "contract": {}, "journey": {}, "ux_module": {},
}

var wikiContractStatuses = map[string]struct{}{
	"draft": {}, "active": {}, "deprecated": {}, "retired": {},
}

var wikiContractLinkTypes = map[string]struct{}{
	"fulfills": {}, "implements": {}, "verifies": {}, "affects": {}, "supports": {},
	"materializes": {}, "supersedes": {}, "code": {}, "experiences": {}, "stages": {},
	"maps": {}, "grounds_in": {},
}

type wikiContractHeader struct {
	WikiContract string         `yaml:"wiki_contract"`
	ID           string         `yaml:"id"`
	Kind         string         `yaml:"kind"`
	Status       string         `yaml:"status"`
	Title        string         `yaml:"title"`
	Links        map[string]any `yaml:"links"`
}

func (a *App) validateWikiContract(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	// WIKI-CONTRACT-v1 views and checks are advisory: never gated by governance.

	registration, project, err := a.resolveWorkspaceWithProjectForNavigation(request)
	if err != nil {
		return model.Envelope{}, err
	}
	setupSource := docgraph.ResolveSetupWikiSource(registration.Root)
	if setupSource.Configured && setupSource.State != "resolved" {
		reason := setupSource.ReasonCode
		if reason == "" {
			reason = "setup_wiki_source_unavailable"
		}
		hint := "Setup wiki source unavailable: " + reason
		result := model.WikiContractValidationResult{ContractReadiness: "advisory", ContractVerdict: "ADVISORY", ContractWarnings: []string{hint}}
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "wiki.contract", Items: []model.WikiContractValidationResult{result}, Warnings: []string{"setup_wiki_source_pending:" + reason}, Hint: hint}, nil
	}
	wikiRoots, err := harnessWikiRoots(registration, project)
	if err != nil {
		return model.Envelope{}, err
	}
	memory, _ := loadReentryMemory(ctx, registration.Name)

	query := loadDocQueryContext(ctx, registration, wikiContractProtocol)
	defer query.Close()
	if query.dbErr != nil {
		return model.Envelope{}, query.dbErr
	}

	warnings := append([]string{}, query.profileWarnings...)
	warnings = append(warnings, fmt.Sprintf("read_model=%s", query.profileSource))

	gov, govWarnings := docgraph.LoadWikiContractGovernance(registration.Root)
	warnings = append(warnings, govWarnings...)

	corpusRecords := filterGovernedWikiRecordsInRoots(registration.Root, query.docs, wikiRoots)
	records := filterGovernedWikiDocRecordsInRoots(registration.Root, query.docs, request.Payload, wikiRoots)
	if len(records) == 0 {
		hint := harnessNoScopeMatchHint(request.Payload)
		if len(query.docs) == 0 {
			hint = fmt.Sprintf("documentation index is empty; rerun 'mi-lsp index --workspace %s --docs-only' before validate-contract", registration.Name)
		}
		result := model.WikiContractValidationResult{
			WikiContract:      wikiContractProtocol,
			ContractReadiness: "advisory",
			ContractVerdict:   "ADVISORY",
			ContractWarnings:  []string{hint},
		}
		env := model.Envelope{Ok: true, Workspace: registration.Name, Backend: "wiki.contract", Items: []model.WikiContractValidationResult{result}, Warnings: warnings, Hint: hint}
		attachWikiContractV11Details(registration.Root, records, &env)
		return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
	}

	result := compileWikiContractValidationInRoots(registration.Root, records, corpusRecords, gov, wikiRoots)
	env := model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "wiki.contract",
		Items:     []model.WikiContractValidationResult{result},
		Warnings:  warnings,
		Stats:     model.Stats{Files: len(records)},
	}
	if len(result.ContractWarnings) > 0 {
		env.Hint = "repair WIKI-CONTRACT-v1 findings before relying on wiki link navigation"
	}
	attachWikiContractV11Details(registration.Root, records, &env)
	return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
}

func compileWikiContractValidation(root string, records []model.DocRecord, gov *docgraph.WikiContractGovernance) model.WikiContractValidationResult {
	return compileWikiContractValidationWithIndexRecords(root, records, records, gov)
}

func compileWikiContractValidationWithIndexRecords(root string, records, indexRecords []model.DocRecord, gov *docgraph.WikiContractGovernance) model.WikiContractValidationResult {
	roots := []string{root} // Compatibility for fixture callers without a configured wiki.
	if setup := docgraph.ResolveSetupWikiSource(root); setup.Configured {
		roots = nil
		if setup.State == "resolved" {
			roots, _ = docgraph.CanonicalWikiRoots(root)
		}
	}
	return compileWikiContractValidationInRoots(root, records, indexRecords, gov, roots)
}

func compileWikiContractValidationInRoots(root string, records, indexRecords []model.DocRecord, gov *docgraph.WikiContractGovernance, roots []string) model.WikiContractValidationResult {
	language := ResolveWikiContractLanguageAtRoot(root)
	result := model.WikiContractValidationResult{
		WikiContract:      wikiContractProtocol,
		ContractReadiness: "ready",
		ContractVerdict:   "PASS",
	}
	indexBodies := readWikiContractIndexBodiesInRoots(root, indexRecords, roots)
	for _, record := range indexRecords {
		if _, reason := wikiContractContainedPath(root, record.Path, roots); reason == "outside_root" {
			result.ContractWarnings = append(result.ContractWarnings, "index_body_untrusted:"+record.Path)
		}
	}
	wikiIndex := wikisource.BuildContractWikiIDIndex(indexBodies)
	for _, record := range indexRecords {
		if strings.TrimSpace(record.DocID) == "" {
			continue
		}
		kind := docgraph.WikiContractKindForPath(record.Path, "")
		wikisource.RegisterContractWikiID(wikiIndex, record.DocID, kind)
	}
	anchorTargets := buildWikiAnchorTargets(indexRecords, indexBodies, language.Language)
	lifecycleTargets := wikiContractLifecycleTargets(root, indexBodies, indexRecords, roots)

	bodyByPath := make(map[string]string, len(records))
	for _, record := range records {
		path, reason := wikiContractContainedPath(root, record.Path, roots)
		docLabel := harnessDocLabel(record)
		if reason != "" {
			warning := docLabel + ": unreadable or untrusted markdown"
			result.ContractWarnings = append(result.ContractWarnings, warning)
			result.Documents = append(result.Documents, model.WikiContractDocumentFinding{Path: record.Path, Warnings: []string{warning}})
			continue
		}
		rawBody, err := os.ReadFile(path)
		if err != nil {
			warning := docLabel + ": unreadable markdown"
			result.ContractWarnings = append(result.ContractWarnings, warning)
			result.Documents = append(result.Documents, model.WikiContractDocumentFinding{
				Path: record.Path, Warnings: []string{warning},
			})
			continue
		}
		bodyByPath[record.Path] = string(rawBody)
	}

	for _, record := range records {
		body, ok := bodyByPath[record.Path]
		if !ok {
			continue
		}
		docLabel := harnessDocLabel(record)
		if strings.EqualFold(filepath.Base(record.Path), "00_retirements.md") {
			warnings := validateWikiContractRetirements(root, record.Path, body, wikiIndex)
			result.ContractsReviewed++
			if len(warnings) > 0 {
				result.Documents = append(result.Documents, model.WikiContractDocumentFinding{Path: record.Path, Kind: "governance", Warnings: warnings})
				result.ContractWarnings = append(result.ContractWarnings, warnings...)
			}
			continue
		}
		header, rawKeys, parseWarnings := parseWikiContractHeader(body)
		finding := model.WikiContractDocumentFinding{Path: record.Path}
		for _, warning := range parseWarnings {
			warning = docLabel + ": " + warning
			finding.Warnings = append(finding.Warnings, warning)
			result.ContractWarnings = append(result.ContractWarnings, warning)
		}
		if header == nil {
			if isWikiContractGovernancePath(root, record.Path, language.Source) {
				continue
			}
			warning := docLabel + ": missing WIKI-CONTRACT-v1 header"
			finding.Warnings = append(finding.Warnings, warning)
			result.ContractWarnings = append(result.ContractWarnings, warning)
			result.Documents = append(result.Documents, finding)
			continue
		}
		// Machine governance is YAML configuration, not an artifact header.
		if isWikiContractGovernancePath(root, record.Path, language.Source) && header.Kind == "" && header.ID == "" && header.Status == "" {
			result.ContractsReviewed++
			continue
		}
		result.ContractsReviewed++
		finding.DocID = header.ID
		finding.Kind = header.Kind
		finding.Status = header.Status
		docWarnings := validateWikiContractHeader(docLabel, header, rawKeys, gov)
		if strings.Contains(strings.ToLower(body), "```toon") {
			docWarnings = append(docWarnings, docLabel+": forbidden normative toon fence")
		}
		switch strings.ToLower(filepath.Base(record.Path)) {
		case "03_architecture.md":
			if header.Kind != "architecture" || !strings.HasPrefix(header.ID, "ARCH-") {
				docWarnings = append(docWarnings, docLabel+": expected kind architecture and ARCH-prefixed id")
			}
		case "07_technical.md":
			if header.Kind != "technical" || !strings.HasPrefix(header.ID, "TECH-") {
				docWarnings = append(docWarnings, docLabel+": expected kind technical and TECH-prefixed id")
			}
		}
		srcID := header.ID
		if srcID == "" {
			srcID = docLabel
		}
		linkWarnings, linksReviewed, targetsMissing := validateWikiContractLinkDepth(root, srcID, header.Links, wikiIndex)
		docWarnings = append(docWarnings, linkWarnings...)
		if header.Status == "active" {
			docWarnings = append(docWarnings, wikiContractLifecycleWarnings(srcID, header.Links, lifecycleTargets)...)
		}
		for _, finding := range model.ValidateWikiContractSections(header.ID, header.Kind, body, language.Language) {
			if finding.Code == "section_missing" {
				result.SectionsMissing++
			}
			docWarnings = append(docWarnings, finding.Message)
		}
		_, anchorFindings := model.DeclaredWikiAnchors(srcID, body, anchorKindFor(header.Kind, record.Path), language.Language)
		for _, finding := range anchorFindings {
			docWarnings = append(docWarnings, finding.Message)
		}
		refs, refWarnings := resolveWikiAnchorRefs(record.Path, srcID, body, anchorTargets)
		result.AnchorRefs = append(result.AnchorRefs, refs...)
		docWarnings = append(docWarnings, refWarnings...)
		result.LinksReviewed += linksReviewed
		result.TargetsMissing += targetsMissing
		for _, warning := range docWarnings {
			finding.Warnings = append(finding.Warnings, warning)
			result.ContractWarnings = append(result.ContractWarnings, warning)
		}
		if len(finding.Warnings) > 0 {
			result.Documents = append(result.Documents, finding)
		}
	}
	if len(result.ContractWarnings) > 0 {
		result.ContractReadiness = "advisory"
		result.ContractVerdict = "ADVISORY"
	}
	sort.Strings(result.ContractWarnings)
	sort.Slice(result.AnchorRefs, func(i, j int) bool {
		a, b := result.AnchorRefs[i], result.AnchorRefs[j]
		if a.SourcePath != b.SourcePath {
			return a.SourcePath < b.SourcePath
		}
		if a.SourceLine != b.SourceLine {
			return a.SourceLine < b.SourceLine
		}
		return a.Ref < b.Ref
	})
	return result
}

type wikiLifecycleTarget struct {
	path     string
	status   string
	archived bool
}

// Identity and lifecycle are independent of LookupKind's presence-only index.
func wikiContractLifecycleTargets(root string, bodies []wikisource.ContractDocBody, records []model.DocRecord, roots []string) map[string][]wikiLifecycleTarget {
	targets := make(map[string][]wikiLifecycleTarget)
	identity := func(path string) (string, bool) {
		resolved, reason := wikiContractContainedPath(root, path, roots)
		if reason != "" {
			return "", false
		}
		return filepath.ToSlash(resolved), true
	}
	indexedIDs := make(map[string]map[string]bool)
	for _, record := range records {
		path, ok := identity(record.Path)
		id := strings.ToUpper(strings.TrimSpace(record.DocID))
		if !ok || id == "" {
			continue
		}
		if indexedIDs[path] == nil {
			indexedIDs[path] = make(map[string]bool)
		}
		indexedIDs[path][id] = true
	}
	seen := make(map[string]bool)
	for _, body := range bodies {
		path, ok := identity(body.Path)
		if !ok {
			continue
		}
		header, _, warnings := parseWikiContractHeader(body.Content)
		if header == nil || len(warnings) != 0 || header.ID == "" {
			continue
		}
		key := strings.ToUpper(header.ID) + "\x00" + path
		if seen[key] {
			continue
		}
		seen[key] = true
		archived := strings.Contains("/"+path+"/", "/_retired/")
		status := header.Status
		ids := indexedIDs[path]
		if len(ids) > 0 && (!ids[strings.ToUpper(header.ID)] || len(ids) > 1) {
			status = "unknown"
		}
		targets[strings.ToUpper(header.ID)] = append(targets[strings.ToUpper(header.ID)], wikiLifecycleTarget{path: path, status: status, archived: archived})
	}
	// Indexed identity without a matching, readable WIKI header is unknown,
	// not proof of either a missing target or an active one.
	for _, record := range records {
		id := strings.ToUpper(strings.TrimSpace(record.DocID))
		if id == "" {
			continue
		}
		path, ok := identity(record.Path)
		if !ok {
			path = filepath.ToSlash(filepath.Clean(record.Path))
		}
		key := id + "\x00" + path
		if !seen[key] {
			seen[key] = true
			targets[id] = append(targets[id], wikiLifecycleTarget{path: path, status: "unknown", archived: strings.Contains("/"+path+"/", "/_retired/")})
		}
	}
	return targets
}

func wikiContractLifecycleWarnings(src string, links map[string]any, targets map[string][]wikiLifecycleTarget) []string {
	var warnings []string
	for linkType, raw := range links {
		if linkType == "code" {
			continue
		}
		if _, ok := wikiContractLinkTypes[linkType]; !ok {
			continue
		}
		for _, target := range wikiContractStringLinkTargets(raw) {
			candidates := targets[strings.ToUpper(strings.TrimSpace(target))]
			var current []wikiLifecycleTarget
			unknown := false
			for _, candidate := range candidates {
				if candidate.status == "unknown" {
					unknown = true
					continue
				}
				if !candidate.archived && candidate.status != "retired" {
					if candidate.status != "active" && candidate.status != "draft" && candidate.status != "deprecated" {
						unknown = true
					} else {
						current = append(current, candidate)
					}
				}
			}
			if unknown || len(current) > 1 || len(current) == 0 && len(candidates) > 1 {
				warnings = append(warnings, fmt.Sprintf("link_target_ambiguous:%s->%s->%s", src, linkType, target))
			} else if len(current) == 0 && len(candidates) == 1 && (candidates[0].archived || candidates[0].status == "retired") {
				warnings = append(warnings, fmt.Sprintf("link_target_retired:%s->%s->%s", src, linkType, target))
			}
		}
	}
	return warnings
}

// wikiAnchorTarget is an indexed document with its declared H3 anchors, so a citation can name file:line.
// readWikiContractIndexBodies reads each indexed document once, so IDs and anchors are declared from the
// same bytes. An unreadable document is skipped; trace and validate-contract share this rule.
// wikiContractContainedPath resolves symlinks before reading any indexed body.
func wikiContractContainedPath(root, path string, roots []string) (string, string) {
	logical, err := filepath.Abs(docgraph.ResolveDocumentPath(root, path))
	if err != nil {
		return "", "unavailable"
	}
	resolved, err := filepath.EvalSymlinks(logical)
	if err != nil {
		return "", "unavailable"
	}
	for _, declared := range roots {
		logicalBase, err := filepath.Abs(docgraph.ResolveDocumentPath(root, declared))
		if err != nil {
			continue
		}
		logicalRel, err := filepath.Rel(logicalBase, logical)
		if err != nil || filepath.IsAbs(logicalRel) || logicalRel == "." || logicalRel == ".." || strings.HasPrefix(logicalRel, ".."+string(filepath.Separator)) {
			continue
		}
		base, err := filepath.EvalSymlinks(logicalBase)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(base, resolved)
		if err == nil && !filepath.IsAbs(rel) && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return resolved, ""
		}
	}
	return "", "outside_root"
}

func readWikiContractIndexBodies(root string, indexRecords []model.DocRecord) []wikisource.ContractDocBody {
	roots, err := docgraph.CanonicalWikiRoots(root)
	if err != nil {
		return nil
	}
	return readWikiContractIndexBodiesInRoots(root, indexRecords, roots)
}

func readWikiContractIndexBodiesInRoots(root string, indexRecords []model.DocRecord, roots []string) []wikisource.ContractDocBody {
	indexBodies := make([]wikisource.ContractDocBody, 0, len(indexRecords))
	for _, record := range indexRecords {
		path, reason := wikiContractContainedPath(root, record.Path, roots)
		if reason != "" {
			continue
		}
		rawBody, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		indexBodies = append(indexBodies, wikisource.ContractDocBody{Path: record.Path, Content: string(rawBody)})
	}
	return indexBodies
}

type wikiAnchorTarget struct {
	path    string
	anchors map[string]int
}

// buildWikiAnchorTargets declares anchors for every indexed document, not only the selected ones, so a
// selected source can cite a target outside the selection. When two documents claim one ID, the first
// in index order wins, which keeps the result deterministic.
func buildWikiAnchorTargets(indexRecords []model.DocRecord, indexBodies []wikisource.ContractDocBody, language model.WikiContractLanguage) map[string]wikiAnchorTarget {
	contents := make(map[string]string, len(indexBodies))
	for _, body := range indexBodies {
		contents[body.Path] = body.Content
	}
	targets := make(map[string]wikiAnchorTarget)
	for _, record := range indexRecords {
		content, ok := contents[record.Path]
		if !ok {
			continue
		}
		meta := wikisource.ParseContractMetadata(content)
		id := strings.ToUpper(strings.TrimSpace(firstNonEmpty(meta.ID, record.DocID)))
		if id == "" {
			continue
		}
		if _, exists := targets[id]; exists {
			continue
		}
		anchors, _ := model.DeclaredWikiAnchors(id, content, anchorKindFor(meta.Kind, record.Path), language)
		targets[id] = wikiAnchorTarget{path: record.Path, anchors: anchors}
	}
	return targets
}

// resolveWikiAnchorRefs checks each `<DOC-ID>#<ANCLA>` citation in a reviewed source against the anchors
// of its target. A citation to a document that is not indexed stays silent, the same treatment broken
// prose IDs have always had; only an existing document without that anchor is drift.
func resolveWikiAnchorRefs(sourcePath, srcID, body string, targets map[string]wikiAnchorTarget) ([]model.WikiAnchorRef, []string) {
	refs := make([]model.WikiAnchorRef, 0)
	warnings := make([]string, 0)
	seen := map[string]struct{}{}
	for _, citation := range model.ParseWikiAnchorCitations(body) {
		target, ok := targets[citation.DocID]
		if !ok {
			continue
		}
		ref := model.WikiAnchorRef{
			Ref:        citation.DocID + "#" + citation.Anchor,
			SourcePath: sourcePath,
			SourceLine: citation.Line,
			TargetPath: target.path,
		}
		if line, declared := target.anchors[citation.Anchor]; declared {
			ref.TargetLine = line
			ref.Status = model.WikiAnchorStatusResolved
		} else {
			ref.Status = model.WikiAnchorStatusMissing
			warning := fmt.Sprintf("anchor_missing:%s->%s", srcID, ref.Ref)
			if _, dup := seen[warning]; !dup {
				seen[warning] = struct{}{}
				warnings = append(warnings, warning)
			}
		}
		refs = append(refs, ref)
	}
	return refs, warnings
}

// anchorKindFor returns the kind that governs anchor rules. A declared kind wins even when the path
// classifier has no alias for it (ux_module); the path is only consulted when nothing is declared.
func anchorKindFor(declared, path string) string {
	if kind := strings.ToLower(strings.TrimSpace(declared)); kind != "" {
		if normalized, ok := docgraph.NormalizeWikiKind(kind); ok {
			return normalized
		}
		return kind
	}
	return docgraph.WikiContractKindForPath(path, "")
}

func validateWikiContractLinkDepth(root, srcID string, links map[string]any, index wikisource.ContractWikiIDIndex) (warnings []string, linksReviewed, targetsMissing int) {
	if links == nil {
		return nil, 0, 0
	}
	srcID = strings.TrimSpace(srcID)
	for linkType, raw := range links {
		switch linkType {
		case "code":
			for _, path := range wikiContractCodePaths(raw) {
				linksReviewed++
				if wikisource.InferContractKindFromDocID(path) != "" && !strings.ContainsAny(path, "/\\.") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(path)), "criterio ") {
					warnings = append(warnings, fmt.Sprintf("link_code_invalid_path:%s:%s", srcID, path))
					continue
				}
				rel := filepath.FromSlash(strings.TrimSpace(path))
				if rel == "" {
					continue
				}
				abs := filepath.Join(root, rel)
				if _, err := os.Stat(abs); err != nil {
					warnings = append(warnings, fmt.Sprintf("link_code_path_missing:%s:%s", srcID, strings.TrimSpace(path)))
				}
			}
			continue
		}
		if _, ok := wikiContractLinkTypes[linkType]; !ok {
			continue
		}
		for _, target := range wikiContractStringLinkTargets(raw) {
			linksReviewed++
			target = strings.TrimSpace(target)
			if target == "" {
				continue
			}
			targetKind := index.LookupKind(target)
			if targetKind == "" {
				warnings = append(warnings, fmt.Sprintf("link_target_missing:%s->%s->%s", srcID, linkType, target))
				targetsMissing++
				continue
			}
			if wikisource.ContractLinkTargetKindMismatch(linkType, target, targetKind) {
				warnings = append(warnings, fmt.Sprintf("link_target_kind_mismatch:%s->%s->%s", srcID, linkType, target))
			}
		}
	}
	return warnings, linksReviewed, targetsMissing
}

func wikiContractStringLinkTargets(raw any) []string {
	switch typed := raw.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if v, ok := item.(string); ok {
				out = append(out, v)
			}
		}
		return out
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{typed}
	default:
		return nil
	}
}

func wikiContractCodePaths(raw any) []string {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if path, ok := m["path"].(string); ok {
			out = append(out, path)
		}
	}
	return out
}

func parseWikiContractHeader(content string) (*wikiContractHeader, map[string]struct{}, []string) {
	block := firstYAMLHeaderBlock(content)
	if block == "" {
		return nil, nil, nil
	}
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(block), &raw); err != nil {
		return nil, nil, []string{fmt.Sprintf("invalid header YAML: %v", err)}
	}
	protocol := strings.TrimSpace(stringField(raw, "wiki_contract"))
	if protocol == "" {
		return nil, keySet(raw), nil
	}
	if !strings.EqualFold(protocol, wikiContractProtocol) {
		return nil, keySet(raw), []string{fmt.Sprintf("wiki_contract must be %s", wikiContractProtocol)}
	}
	header := &wikiContractHeader{
		WikiContract: protocol,
		ID:           strings.TrimSpace(stringField(raw, "id")),
		Kind:         strings.TrimSpace(strings.ToLower(stringField(raw, "kind"))),
		Status:       strings.TrimSpace(strings.ToLower(stringField(raw, "status"))),
		Title:        strings.TrimSpace(stringField(raw, "title")),
	}
	if links, ok := raw["links"].(map[string]any); ok {
		header.Links = links
	}
	return header, keySet(raw), nil
}

func firstYAMLHeaderBlock(content string) string {
	blocks := candidateHarnessYAMLBlocks(content)
	if len(blocks) == 0 {
		return ""
	}
	return blocks[0]
}

// wikiContractCanonicalKind maps the kind spelling used by WIKI-CONTRACT-v1 onto the name this
// package validates. UXM documents declare module_map, which is the ux_module kind.
func wikiContractCanonicalKind(kind string) string {
	if kind == "module_map" {
		return docgraph.WikiKindUXModule
	}
	return kind
}

func validateWikiContractHeader(docLabel string, header *wikiContractHeader, rawKeys map[string]struct{}, gov *docgraph.WikiContractGovernance) []string {
	var warnings []string
	if header.ID == "" && header.Kind != "governance" && header.Kind != "scope" {
		warnings = append(warnings, docLabel+": missing id")
	}
	if header.Kind == "" {
		warnings = append(warnings, docLabel+": missing kind")
	} else if _, ok := wikiContractKinds[wikiContractCanonicalKind(header.Kind)]; !ok {
		warnings = append(warnings, docLabel+": unknown kind "+header.Kind)
	}
	if header.Status == "" {
		warnings = append(warnings, docLabel+": missing status")
	} else if _, ok := wikiContractStatuses[header.Status]; !ok {
		warnings = append(warnings, docLabel+": invalid status "+header.Status)
	}
	for _, forbidden := range wikiContractForbiddenKeys {
		if _, ok := rawKeys[forbidden]; ok {
			warnings = append(warnings, docLabel+": forbidden key "+forbidden)
		}
	}
	if header.Kind == "governance" && header.Links != nil && len(header.Links) > 0 {
		warnings = append(warnings, docLabel+": governance documents must not declare links")
	}
	for linkType := range header.Links {
		if _, ok := wikiContractLinkTypes[linkType]; !ok {
			warnings = append(warnings, docLabel+": unknown links."+linkType)
		}
	}
	warnings = append(warnings, validateWikiContractRequiredLinks(docLabel, header, gov)...)
	return warnings
}

func validateWikiContractRequiredLinks(docLabel string, header *wikiContractHeader, gov *docgraph.WikiContractGovernance) []string {
	var warnings []string
	kind := wikiContractCanonicalKind(header.Kind)
	links := header.Links
	requireLink := func(linkType string) {
		if !wikiContractLinkNonEmpty(links, linkType) {
			warnings = append(warnings, fmt.Sprintf("%s: missing required links.%s", docLabel, linkType))
		}
	}
	switch kind {
	case "flow":
		requireLink("fulfills")
	case "requirement":
		requireLink("implements")
	case "test":
		requireLink("verifies")
	case "database":
		requireLink("materializes")
	case "journey":
		if gov != nil && gov.TraitEnabled("has_ui") {
			requireLink("experiences")
			requireLink("stages")
		}
	case "ux_module":
		if gov != nil && gov.TraitEnabled("has_ui") {
			requireLink("maps")
		}
	case "outcome":
		if gov != nil && gov.TraitEnabled("sold_product") {
			requireLink("grounds_in")
		}
	}
	return warnings
}

func wikiContractLinkNonEmpty(links map[string]any, key string) bool {
	if links == nil {
		return false
	}
	value := links[key]
	switch typed := value.(type) {
	case []any:
		return len(typed) > 0
	case string:
		return strings.TrimSpace(typed) != ""
	default:
		return value != nil
	}
}

func countWikiContractLinks(links map[string]any) int {
	if links == nil {
		return 0
	}
	count := 0
	for _, value := range links {
		switch typed := value.(type) {
		case []any:
			count += len(typed)
		default:
			count++
		}
	}
	return count
}

func isWikiContractGovernancePath(root, path, selected string) bool {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(selected) == "" {
		return false
	}
	actual, err := filepath.Abs(docgraph.ResolveDocumentPath(root, path))
	if err != nil {
		return false
	}
	expected, err := filepath.Abs(docgraph.ResolveDocumentPath(root, selected))
	return err == nil && filepath.Clean(actual) == filepath.Clean(expected)
}

func stringField(raw map[string]any, key string) string {
	value, ok := raw[key].(string)
	if !ok {
		return ""
	}
	return value
}

func keySet(raw map[string]any) map[string]struct{} {
	set := make(map[string]struct{}, len(raw))
	for key := range raw {
		set[key] = struct{}{}
	}
	return set
}
