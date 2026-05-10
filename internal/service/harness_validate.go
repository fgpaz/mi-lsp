package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const harnessProtocolV1 = "SDD-HARNESS-v1"

var (
	fencedYAMLBlockPattern = regexp.MustCompile("(?s)```(?:yaml|yml)\\s*(.*?)\\s*```")
	obsidianLinkPattern    = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
)

type harnessContract struct {
	HarnessProtocol  string   `yaml:"harness_protocol"`
	ID               string   `yaml:"id"`
	Kind             string   `yaml:"kind"`
	Audience         string   `yaml:"audience"`
	Imports          []string `yaml:"imports"`
	Exports          []string `yaml:"exports"`
	AgentMustRead    []string `yaml:"agent_must_read"`
	AgentMayEdit     []string `yaml:"agent_may_edit"`
	AgentMustNotEdit []string `yaml:"agent_must_not_edit"`
	Verify           []string `yaml:"verify"`
	StopIf           []string `yaml:"stop_if"`
	Evidence         []string `yaml:"evidence"`
}

type harnessDoc struct {
	record   model.DocRecord
	content  string
	contract *harnessContract
	links    []string
}

func (a *App) validateHarness(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	if blockedEnv, err := a.governanceGateEnvelope(ctx, request, "nav.wiki.validate-harness"); err != nil {
		return model.Envelope{}, err
	} else if blockedEnv != nil {
		return *blockedEnv, nil
	}

	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}
	memory, _ := loadReentryMemory(ctx, registration.Root)

	query := loadDocQueryContext(ctx, registration, "SDD-HARNESS-v1")
	defer query.Close()
	if query.dbErr != nil {
		return model.Envelope{}, query.dbErr
	}

	warnings := append([]string{}, query.profileWarnings...)
	warnings = append(warnings, fmt.Sprintf("read_model=%s", query.profileSource))
	if len(query.docs) == 0 {
		hint := fmt.Sprintf("documentation index is empty; rerun 'mi-lsp index --workspace %s --docs-only' before validate-harness", registration.Name)
		warnings = appendStringIfMissing(warnings, hint)
		result := model.HarnessValidationResult{
			HarnessProtocol:  harnessProtocolV1,
			HarnessReadiness: "blocked",
			HarnessVerdict:   "BLOCKED",
			HarnessBlockers:  []string{hint},
		}
		env := model.Envelope{Ok: true, Workspace: registration.Name, Backend: "harness", Items: []model.HarnessValidationResult{result}, Warnings: warnings, Hint: hint}
		return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
	}

	records := filterHarnessDocRecords(query.docs, request.Payload)
	if len(records) == 0 {
		hint := harnessNoScopeMatchHint(request.Payload)
		result := model.HarnessValidationResult{
			HarnessProtocol:  harnessProtocolV1,
			HarnessReadiness: "blocked",
			HarnessVerdict:   "BLOCKED",
			HarnessBlockers:  []string{hint},
		}
		env := model.Envelope{Ok: true, Workspace: registration.Name, Backend: "harness", Items: []model.HarnessValidationResult{result}, Warnings: warnings, Hint: hint}
		return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
	}

	docs := loadHarnessDocs(registration.Root, records)
	result := compileHarnessValidation(registration.Root, docs)
	env := model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "harness",
		Items:     []model.HarnessValidationResult{result},
		Warnings:  warnings,
		Stats:     model.Stats{Files: len(docs)},
	}
	if result.HarnessVerdict == "BLOCKED" {
		env.Hint = "repair missing or invalid SDD-HARNESS-v1 contracts before relying on LLM-first wiki execution"
	}
	return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
}

func filterHarnessDocRecords(records []model.DocRecord, payload map[string]any) []model.DocRecord {
	ids := splitHarnessScopeValues(stringPayload(payload, "ids"))
	paths := splitHarnessScopeValues(stringPayload(payload, "paths"))
	if len(ids) == 0 && len(paths) == 0 {
		return records
	}
	filtered := make([]model.DocRecord, 0, len(records))
	filtered = append(filtered, filterHarnessDocRecordsByIDs(records, ids)...)
	for _, record := range records {
		if harnessRecordMatchesPaths(record, paths) {
			filtered = append(filtered, record)
		}
	}
	return uniqueHarnessDocRecords(filtered)
}

func splitHarnessScopeValues(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	values := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		value := strings.TrimSpace(part)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func filterHarnessDocRecordsByIDs(records []model.DocRecord, ids []string) []model.DocRecord {
	if len(ids) == 0 {
		return nil
	}
	filtered := make([]model.DocRecord, 0, len(records))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		matches := make([]model.DocRecord, 0)
		canonical := make([]model.DocRecord, 0)
		for _, record := range records {
			if !harnessRecordMatchesID(record, id) {
				continue
			}
			matches = append(matches, record)
			if harnessRecordHasCanonicalIDPath(record, id) {
				canonical = append(canonical, record)
			}
		}
		if len(canonical) > 0 {
			filtered = append(filtered, canonical...)
			continue
		}
		filtered = append(filtered, matches...)
	}
	return uniqueHarnessDocRecords(filtered)
}

func harnessRecordMatchesID(record model.DocRecord, id string) bool {
	candidates := []string{
		record.DocID,
		record.Title,
		strings.TrimSuffix(filepath.Base(filepath.ToSlash(record.Path)), ".md"),
	}
	for _, candidate := range candidates {
		if strings.EqualFold(strings.TrimSpace(id), strings.TrimSpace(candidate)) {
			return true
		}
	}
	return false
}

func harnessRecordHasCanonicalIDPath(record model.DocRecord, id string) bool {
	base := strings.TrimSuffix(filepath.Base(filepath.ToSlash(record.Path)), ".md")
	return strings.EqualFold(strings.TrimSpace(id), strings.TrimSpace(base))
}

func harnessRecordMatchesPaths(record model.DocRecord, paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	recordPath := normalizeHarnessScopePath(record.Path)
	recordBase := strings.ToLower(filepath.Base(recordPath))
	recordBaseNoExt := strings.TrimSuffix(recordBase, ".md")
	for _, path := range paths {
		normalized := normalizeHarnessScopePath(path)
		if normalized == "" {
			continue
		}
		if normalized == recordPath || normalized == recordBase || normalized == recordBaseNoExt {
			return true
		}
	}
	return false
}

func normalizeHarnessScopePath(path string) string {
	normalized := strings.TrimSpace(filepath.ToSlash(path))
	for strings.HasPrefix(normalized, "./") {
		normalized = strings.TrimPrefix(normalized, "./")
	}
	return strings.ToLower(normalized)
}

func uniqueHarnessDocRecords(records []model.DocRecord) []model.DocRecord {
	filtered := make([]model.DocRecord, 0, len(records))
	seen := map[string]struct{}{}
	for _, record := range records {
		key := strings.ToLower(filepath.ToSlash(record.Path)) + "\x00" + strings.ToLower(record.DocID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, record)
	}
	return filtered
}

func harnessNoScopeMatchHint(payload map[string]any) string {
	parts := []string{}
	if ids := strings.TrimSpace(stringPayload(payload, "ids")); ids != "" {
		parts = append(parts, "ids="+ids)
	}
	if paths := strings.TrimSpace(stringPayload(payload, "paths")); paths != "" {
		parts = append(parts, "paths="+paths)
	}
	if len(parts) == 0 {
		return "scoped validate-harness filters matched no indexed wiki docs"
	}
	return "scoped validate-harness filters matched no indexed wiki docs (" + strings.Join(parts, ", ") + ")"
}

func loadHarnessDocs(root string, records []model.DocRecord) []harnessDoc {
	docs := make([]harnessDoc, 0, len(records))
	seen := map[string]struct{}{}
	for _, record := range records {
		if strings.TrimSpace(record.Path) == "" {
			continue
		}
		key := strings.ToLower(filepath.ToSlash(record.Path))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		path := filepath.Join(root, filepath.FromSlash(filepath.ToSlash(record.Path)))
		body, err := os.ReadFile(path)
		if err != nil {
			docs = append(docs, harnessDoc{record: record})
			continue
		}
		content := string(body)
		docs = append(docs, harnessDoc{
			record:   record,
			content:  content,
			contract: extractHarnessContract(content),
			links:    extractObsidianLinks(content),
		})
	}
	return docs
}

func extractHarnessContract(content string) *harnessContract {
	blocks := candidateHarnessYAMLBlocks(content)
	for _, block := range blocks {
		if !strings.Contains(block, "harness_protocol") || !strings.Contains(block, harnessProtocolV1) {
			continue
		}
		var contract harnessContract
		if err := yaml.Unmarshal([]byte(block), &contract); err != nil {
			return &harnessContract{HarnessProtocol: harnessProtocolV1}
		}
		if strings.TrimSpace(contract.HarnessProtocol) == harnessProtocolV1 {
			return &contract
		}
	}
	return nil
}

func candidateHarnessYAMLBlocks(content string) []string {
	blocks := []string{}
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "---") {
		rest := trimmed[3:]
		if end := strings.Index(rest, "\n---"); end >= 0 {
			blocks = append(blocks, rest[:end])
		}
	}
	for _, match := range fencedYAMLBlockPattern.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			blocks = append(blocks, match[1])
		}
	}
	return blocks
}

func compileHarnessValidation(root string, docs []harnessDoc) model.HarnessValidationResult {
	result := model.HarnessValidationResult{
		HarnessProtocol: harnessProtocolV1,
		HarnessVerdict:  "PASS",
	}
	docIndex := buildHarnessDocIndex(docs)
	contractCoverage := buildHarnessContractCoverage(docs)
	seenRequired := map[string]struct{}{}
	seenFound := map[string]struct{}{}

	for _, doc := range docs {
		docLabel := harnessDocLabel(doc.record)
		if doc.content == "" {
			result.HarnessBlockers = append(result.HarnessBlockers, docLabel+": unreadable markdown")
			continue
		}
		if doc.contract == nil {
			if _, covered := contractCoverage[normalizeHarnessRef(docLabel)]; covered {
				continue
			}
			result.HarnessDocsMissingContract = append(result.HarnessDocsMissingContract, docLabel)
			result.HarnessBlockers = append(result.HarnessBlockers, docLabel+": missing SDD-HARNESS-v1 contract")
			continue
		}

		result.HarnessContractsReviewed++
		contract := doc.contract
		audience := normalizeHarnessAudience(contract.Audience)
		if audience == "unknown" {
			result.HarnessDocsUnknownAudience = append(result.HarnessDocsUnknownAudience, docLabel)
			result.HarnessBlockers = append(result.HarnessBlockers, docLabel+": unknown harness audience")
		}

		for _, field := range missingHarnessFields(contract) {
			result.HarnessBlockers = append(result.HarnessBlockers, docLabel+": missing "+field)
		}
		if audience == "llm-first" || audience == "unknown" {
			if len(trimmedNonEmpty(contract.Verify)) == 0 {
				result.HarnessBlockers = append(result.HarnessBlockers, docLabel+": llm-first contract missing verify")
			}
			if len(trimmedNonEmpty(contract.StopIf)) == 0 {
				result.HarnessBlockers = append(result.HarnessBlockers, docLabel+": llm-first contract missing stop_if")
			}
			if len(trimmedNonEmpty(contract.Evidence)) == 0 {
				result.HarnessBlockers = append(result.HarnessBlockers, docLabel+": llm-first contract missing evidence")
			}
		} else if audience == "human" || audience == "dual" {
			if len(trimmedNonEmpty(contract.Verify)) == 0 {
				result.HarnessWarnings = append(result.HarnessWarnings, docLabel+": "+audience+" contract has empty verify")
			}
			if len(trimmedNonEmpty(contract.StopIf)) == 0 {
				result.HarnessWarnings = append(result.HarnessWarnings, docLabel+": "+audience+" contract has empty stop_if")
			}
			if len(trimmedNonEmpty(contract.Evidence)) == 0 {
				result.HarnessWarnings = append(result.HarnessWarnings, docLabel+": "+audience+" contract has empty evidence")
			}
		}

		for _, conflict := range intersectStrings(contract.AgentMayEdit, contract.AgentMustNotEdit) {
			result.HarnessBlockers = append(result.HarnessBlockers, docLabel+": edit allow/deny conflict for "+conflict)
		}

		for _, ref := range append(trimmedNonEmpty(contract.Imports), doc.links...) {
			result.HarnessLinksReviewed++
			if !harnessRefExists(root, docIndex, doc.record.Path, ref) {
				result.HarnessBlockers = append(result.HarnessBlockers, docLabel+": broken import/link "+ref)
			}
		}

		for _, evidence := range trimmedNonEmpty(contract.Evidence) {
			if _, ok := seenRequired[evidence]; !ok {
				seenRequired[evidence] = struct{}{}
				result.HarnessEvidenceRequired = append(result.HarnessEvidenceRequired, evidence)
			}
			if harnessRefExists(root, docIndex, doc.record.Path, evidence) {
				if _, ok := seenFound[evidence]; !ok {
					seenFound[evidence] = struct{}{}
					result.HarnessEvidenceFound = append(result.HarnessEvidenceFound, evidence)
				}
			} else if audience == "llm-first" || audience == "unknown" {
				result.HarnessBlockers = append(result.HarnessBlockers, docLabel+": evidence not found "+evidence)
			}
		}
	}

	result.HarnessBlockers = uniqueSortedStrings(result.HarnessBlockers)
	result.HarnessWarnings = uniqueSortedStrings(result.HarnessWarnings)
	result.HarnessDocsMissingContract = uniqueSortedStrings(result.HarnessDocsMissingContract)
	result.HarnessDocsUnknownAudience = uniqueSortedStrings(result.HarnessDocsUnknownAudience)
	result.HarnessEvidenceRequired = uniqueSortedStrings(result.HarnessEvidenceRequired)
	result.HarnessEvidenceFound = uniqueSortedStrings(result.HarnessEvidenceFound)

	if len(result.HarnessBlockers) > 0 {
		result.HarnessVerdict = "BLOCKED"
		result.HarnessReadiness = "blocked"
	} else if len(result.HarnessWarnings) > 0 {
		result.HarnessVerdict = "WARN"
		result.HarnessReadiness = "warning"
	} else {
		result.HarnessReadiness = "ready"
	}
	return result
}

func missingHarnessFields(contract *harnessContract) []string {
	missing := []string{}
	if strings.TrimSpace(contract.ID) == "" {
		missing = append(missing, "id")
	}
	if strings.TrimSpace(contract.Kind) == "" {
		missing = append(missing, "kind")
	}
	if strings.TrimSpace(contract.Audience) == "" {
		missing = append(missing, "audience")
	}
	if len(trimmedNonEmpty(contract.Imports)) == 0 {
		missing = append(missing, "imports")
	}
	if len(trimmedNonEmpty(contract.Exports)) == 0 {
		missing = append(missing, "exports")
	}
	if len(trimmedNonEmpty(contract.AgentMustRead)) == 0 {
		missing = append(missing, "agent_must_read")
	}
	if len(trimmedNonEmpty(contract.AgentMayEdit)) == 0 {
		missing = append(missing, "agent_may_edit")
	}
	if len(trimmedNonEmpty(contract.AgentMustNotEdit)) == 0 {
		missing = append(missing, "agent_must_not_edit")
	}
	return missing
}

func normalizeHarnessAudience(audience string) string {
	switch strings.ToLower(strings.TrimSpace(audience)) {
	case "human", "dual", "llm-first":
		return strings.ToLower(strings.TrimSpace(audience))
	default:
		return "unknown"
	}
}

func buildHarnessDocIndex(docs []harnessDoc) map[string]struct{} {
	index := map[string]struct{}{}
	for _, doc := range docs {
		path := filepath.ToSlash(doc.record.Path)
		if path != "" {
			addHarnessIndexKey(index, path)
			addHarnessIndexKey(index, strings.TrimSuffix(path, ".md"))
			addHarnessIndexKey(index, filepath.Base(path))
			addHarnessIndexKey(index, strings.TrimSuffix(filepath.Base(path), ".md"))
		}
		addHarnessIndexKey(index, doc.record.DocID)
		addHarnessIndexKey(index, doc.record.Title)
		if doc.contract != nil {
			addHarnessIndexKey(index, doc.contract.ID)
			for _, exported := range doc.contract.Exports {
				addHarnessIndexKey(index, exported)
			}
		}
	}
	return index
}

func buildHarnessContractCoverage(docs []harnessDoc) map[string]struct{} {
	covered := map[string]struct{}{}
	for _, doc := range docs {
		if doc.contract == nil {
			continue
		}
		addHarnessIndexKey(covered, doc.record.DocID)
		addHarnessIndexKey(covered, doc.contract.ID)
		for _, exported := range doc.contract.Exports {
			addHarnessIndexKey(covered, exported)
		}
	}
	return covered
}

func addHarnessIndexKey(index map[string]struct{}, value string) {
	if key := normalizeHarnessRef(value); key != "" {
		index[key] = struct{}{}
	}
}

func harnessRefExists(root string, index map[string]struct{}, fromPath string, ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" || ref == "." || strings.EqualFold(ref, "none") || strings.EqualFold(ref, "n/a") {
		return true
	}
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return true
	}
	if _, ok := index[normalizeHarnessRef(ref)]; ok {
		return true
	}
	if strings.HasPrefix(ref, "[[") && strings.HasSuffix(ref, "]]") {
		ref = strings.TrimSuffix(strings.TrimPrefix(ref, "[["), "]]")
	}
	ref = strings.Split(ref, "|")[0]
	ref = strings.Split(ref, "#")[0]
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return true
	}
	path := filepath.ToSlash(ref)
	if strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") {
		path = filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(filepath.ToSlash(fromPath)), filepath.FromSlash(path))))
	}
	if !strings.HasSuffix(strings.ToLower(path), ".md") && strings.Contains(path, "/") {
		path += ".md"
	}
	if _, ok := index[normalizeHarnessRef(path)]; ok {
		return true
	}
	if strings.Contains(path, "/") {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err == nil {
			return true
		}
	}
	return false
}

func normalizeHarnessRef(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[[")
	value = strings.TrimSuffix(value, "]]")
	value = strings.Split(value, "|")[0]
	value = strings.Split(value, "#")[0]
	value = filepath.ToSlash(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "./")
	return strings.ToLower(value)
}

func extractObsidianLinks(content string) []string {
	links := []string{}
	seen := map[string]struct{}{}
	for _, match := range obsidianLinkPattern.FindAllStringSubmatch(content, -1) {
		if len(match) < 2 {
			continue
		}
		link := strings.TrimSpace(match[1])
		if link == "" {
			continue
		}
		if _, ok := seen[link]; ok {
			continue
		}
		seen[link] = struct{}{}
		links = append(links, link)
	}
	return links
}

func trimmedNonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func intersectStrings(left []string, right []string) []string {
	rightSet := map[string]string{}
	for _, value := range right {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			rightSet[strings.ToLower(trimmed)] = trimmed
		}
	}
	seen := map[string]struct{}{}
	conflicts := []string{}
	for _, value := range left {
		trimmed := strings.TrimSpace(value)
		key := strings.ToLower(trimmed)
		if trimmed == "" {
			continue
		}
		if original, ok := rightSet[key]; ok {
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			conflicts = append(conflicts, original)
		}
	}
	sort.Strings(conflicts)
	return conflicts
}

func harnessDocLabel(record model.DocRecord) string {
	if docID := strings.TrimSpace(record.DocID); docID != "" {
		base := strings.TrimSuffix(filepath.Base(filepath.ToSlash(record.Path)), ".md")
		if base == "" || strings.EqualFold(docID, base) {
			return docID
		}
		return docID + " (" + record.Path + ")"
	}
	return record.Path
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
