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
)

const wikiContractProtocol = docgraph.WikiContractProtocolV1

var wikiContractForbiddenKeys = []string{
	"bounds", "imports", "exports", "planned", "doc_id", "harness_protocol",
	"wiki_source_protocol", "wiki_graph_protocol", "harness_protocol",
	"agent_must_read", "agent_may_edit", "agent_must_not_edit", "verify", "stop_if", "evidence",
	"audience", "business_anchor",
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
		return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
	}

	result := compileWikiContractValidation(registration.Root, records, gov)
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
	return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
}

func compileWikiContractValidation(root string, records []model.DocRecord, gov *docgraph.WikiContractGovernance) model.WikiContractValidationResult {
	result := model.WikiContractValidationResult{
		WikiContract:      wikiContractProtocol,
		ContractReadiness: "ready",
		ContractVerdict:   "PASS",
	}
	for _, record := range records {
		path := filepath.Join(root, filepath.FromSlash(record.Path))
		rawBody, err := os.ReadFile(path)
		body := string(rawBody)
		docLabel := harnessDocLabel(record)
		if err != nil {
			warning := docLabel + ": unreadable markdown"
			result.ContractWarnings = append(result.ContractWarnings, warning)
			result.Documents = append(result.Documents, model.WikiContractDocumentFinding{
				Path: record.Path, Warnings: []string{warning},
			})
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
			if isWikiContractGovernancePath(record.Path) {
				continue
			}
			warning := docLabel + ": missing WIKI-CONTRACT-v1 header"
			finding.Warnings = append(finding.Warnings, warning)
			result.ContractWarnings = append(result.ContractWarnings, warning)
			result.Documents = append(result.Documents, finding)
			continue
		}
		result.ContractsReviewed++
		finding.DocID = header.ID
		finding.Kind = header.Kind
		finding.Status = header.Status
		docWarnings := validateWikiContractHeader(docLabel, header, rawKeys, gov)
		for _, warning := range docWarnings {
			finding.Warnings = append(finding.Warnings, warning)
			result.ContractWarnings = append(result.ContractWarnings, warning)
		}
		result.LinksReviewed += countWikiContractLinks(header.Links)
		if len(finding.Warnings) > 0 {
			result.Documents = append(result.Documents, finding)
		}
	}
	if len(result.ContractWarnings) > 0 {
		result.ContractReadiness = "advisory"
		result.ContractVerdict = "ADVISORY"
	}
	sort.Strings(result.ContractWarnings)
	return result
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

func validateWikiContractHeader(docLabel string, header *wikiContractHeader, rawKeys map[string]struct{}, gov *docgraph.WikiContractGovernance) []string {
	var warnings []string
	if header.ID == "" {
		warnings = append(warnings, docLabel+": missing id")
	}
	if header.Kind == "" {
		warnings = append(warnings, docLabel+": missing kind")
	} else if _, ok := wikiContractKinds[header.Kind]; !ok {
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
	kind := header.Kind
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

func isWikiContractGovernancePath(path string) bool {
	normalized := strings.ToLower(filepath.ToSlash(path))
	return strings.HasSuffix(normalized, "00_governance.md") ||
		strings.HasSuffix(normalized, "00_gobierno_documental.md")
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
