package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/wikisource"
)

const wikiSourceProtocolV1 = wikisource.ProtocolV1

type sourceDoc struct {
	record  model.DocRecord
	content string
	parsed  wikisource.ParsedDoc
}

func (a *App) validateSource(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	if blockedEnv, err := a.governanceGateEnvelope(ctx, request, "nav.wiki.validate-source"); err != nil {
		return model.Envelope{}, err
	} else if blockedEnv != nil {
		return *blockedEnv, nil
	}

	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}
	memory, _ := loadReentryMemory(ctx, registration.Root)

	query := loadDocQueryContext(ctx, registration, wikiSourceProtocolV1)
	defer query.Close()
	if query.dbErr != nil {
		return model.Envelope{}, query.dbErr
	}

	warnings := append([]string{}, query.profileWarnings...)
	warnings = append(warnings, fmt.Sprintf("read_model=%s", query.profileSource))
	if explicitEmptyWikiSourceScope(request.Payload) {
		hint := wikiSourceScopeHint(request.Payload)
		result := model.WikiSourceValidationResult{
			WikiSourceProtocol:  wikiSourceProtocolV1,
			IndexFreshness:      "scope_empty",
			GovernanceSync:      "in_sync",
			WikiSourceReadiness: "blocked",
			WikiSourceVerdict:   "BLOCKED",
			WikiSourceBlockers:  []string{hint},
			NavigationReadiness: "blocked",
			NavigationBlockers:  []string{hint},
		}
		env := model.Envelope{Ok: true, Workspace: registration.Name, Backend: "wiki.source", Items: []model.WikiSourceValidationResult{result}, Warnings: warnings, Hint: hint}
		return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
	}
	if len(query.docs) == 0 {
		hint := fmt.Sprintf("documentation index is empty; rerun 'mi-lsp index --workspace %s --docs-only' before validate-source", registration.Name)
		result := model.WikiSourceValidationResult{
			WikiSourceProtocol:  wikiSourceProtocolV1,
			IndexFreshness:      "empty_index",
			GovernanceSync:      "in_sync",
			WikiSourceReadiness: "blocked",
			WikiSourceVerdict:   "BLOCKED",
			WikiSourceBlockers:  []string{hint},
			NavigationReadiness: "blocked",
			NavigationBlockers:  []string{hint},
		}
		env := model.Envelope{Ok: true, Workspace: registration.Name, Backend: "wiki.source", Items: []model.WikiSourceValidationResult{result}, Warnings: warnings, Hint: hint}
		return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
	}

	allWikiDocs := filterGovernedWikiDocRecords(query.docs, map[string]any{})
	scopedDocs := filterGovernedWikiDocRecords(query.docs, request.Payload)
	if wikiSourceScopeRequested(request.Payload) && len(scopedDocs) == 0 {
		hint := wikiSourceScopeHint(request.Payload)
		result := model.WikiSourceValidationResult{
			WikiSourceProtocol:  wikiSourceProtocolV1,
			IndexFreshness:      "scope_empty",
			GovernanceSync:      "in_sync",
			WikiSourceReadiness: "blocked",
			WikiSourceVerdict:   "BLOCKED",
			WikiSourceBlockers:  []string{hint},
			NavigationReadiness: "blocked",
			NavigationBlockers:  []string{hint},
		}
		env := model.Envelope{Ok: true, Workspace: registration.Name, Backend: "wiki.source", Items: []model.WikiSourceValidationResult{result}, Warnings: warnings, Hint: hint}
		return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
	}
	blocks, err := store.ListDocSourceBlocks(ctx, query.db)
	if err != nil {
		return model.Envelope{}, err
	}
	records, err := store.ListDocSourceRecords(ctx, query.db)
	if err != nil {
		return model.Envelope{}, err
	}
	docs := loadSourceDocs(registration.Root, scopedDocs)
	reportedBlocks := filterSourceBlocksByDocs(blocks, docs)
	reportedRecords := filterSourceRecordsByDocs(records, docs)
	corpusBlocks := filterSourceBlocksByDocRecords(blocks, allWikiDocs)
	corpusRecords := filterSourceRecordsByDocRecords(records, allWikiDocs)
	result := compileSourceValidationWithCorpus(docs, allWikiDocs, reportedBlocks, reportedRecords, corpusBlocks, corpusRecords)
	env := model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "wiki.source",
		Items:     []model.WikiSourceValidationResult{result},
		Warnings:  warnings,
		Stats:     model.Stats{Files: len(docs)},
	}
	if result.WikiSourceVerdict == "BLOCKED" {
		env.Hint = "repair SDD-WIKI-SOURCE-v1 doc_id, block_id, fenced toon, table exceptions, or index navigation rows"
	}
	return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
}

func loadSourceDocs(root string, records []model.DocRecord) []sourceDoc {
	docs := make([]sourceDoc, 0)
	for _, record := range records {
		if record.Path == "" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(filepath.ToSlash(record.Path))))
		if err != nil {
			continue
		}
		content := string(body)
		parsed := wikisource.Parse(record.Path, content, 0)
		if !parsed.DeclaresSource {
			continue
		}
		docs = append(docs, sourceDoc{
			record:  record,
			content: content,
			parsed:  parsed,
		})
	}
	return docs
}

func wikiSourceScopeRequested(payload map[string]any) bool {
	_, idsPresent := payload["ids"]
	_, pathsPresent := payload["paths"]
	return idsPresent || pathsPresent
}

func explicitEmptyWikiSourceScope(payload map[string]any) bool {
	if !wikiSourceScopeRequested(payload) {
		return false
	}
	return len(splitHarnessScopeValues(stringPayload(payload, "ids"))) == 0 && len(splitHarnessScopeValues(stringPayload(payload, "paths"))) == 0
}

func wikiSourceScopeHint(payload map[string]any) string {
	if explicitEmptyWikiSourceScope(payload) {
		return "scope=explicit_empty; required=non-empty ids or paths; protocol=" + wikiSourceProtocolV1
	}
	parts := []string{}
	if ids := strings.TrimSpace(stringPayload(payload, "ids")); ids != "" {
		parts = append(parts, "ids="+ids)
	}
	if paths := strings.TrimSpace(stringPayload(payload, "paths")); paths != "" {
		parts = append(parts, "paths="+paths)
	}
	if len(parts) == 0 {
		return "scope=no_match; required=governed wiki docs declaring " + wikiSourceProtocolV1
	}
	return "scope=no_match; " + strings.Join(parts, ", ") + "; required=governed wiki docs declaring " + wikiSourceProtocolV1
}

func filterSourceBlocksByDocs(blocks []model.DocSourceBlock, docs []sourceDoc) []model.DocSourceBlock {
	allowed := sourceDocPathSet(docs)
	filtered := make([]model.DocSourceBlock, 0, len(blocks))
	for _, block := range blocks {
		if _, ok := allowed[normalizeHarnessScopePath(block.DocPath)]; ok {
			filtered = append(filtered, block)
		}
	}
	return filtered
}

func filterSourceRecordsByDocs(records []model.DocSourceRecord, docs []sourceDoc) []model.DocSourceRecord {
	allowed := sourceDocPathSet(docs)
	filtered := make([]model.DocSourceRecord, 0, len(records))
	for _, record := range records {
		if _, ok := allowed[normalizeHarnessScopePath(record.DocPath)]; ok {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func filterSourceBlocksByDocRecords(blocks []model.DocSourceBlock, docs []model.DocRecord) []model.DocSourceBlock {
	allowed := docRecordPathSet(docs)
	filtered := make([]model.DocSourceBlock, 0, len(blocks))
	for _, block := range blocks {
		if _, ok := allowed[normalizeHarnessScopePath(block.DocPath)]; ok {
			filtered = append(filtered, block)
		}
	}
	return filtered
}

func filterSourceRecordsByDocRecords(records []model.DocSourceRecord, docs []model.DocRecord) []model.DocSourceRecord {
	allowed := docRecordPathSet(docs)
	filtered := make([]model.DocSourceRecord, 0, len(records))
	for _, record := range records {
		if _, ok := allowed[normalizeHarnessScopePath(record.DocPath)]; ok {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func sourceDocPathSet(docs []sourceDoc) map[string]struct{} {
	allowed := make(map[string]struct{}, len(docs))
	for _, doc := range docs {
		allowed[normalizeHarnessScopePath(doc.record.Path)] = struct{}{}
	}
	return allowed
}

func docRecordPathSet(docs []model.DocRecord) map[string]struct{} {
	allowed := make(map[string]struct{}, len(docs))
	for _, doc := range docs {
		allowed[normalizeHarnessScopePath(doc.Path)] = struct{}{}
	}
	return allowed
}

func compileSourceValidation(docs []sourceDoc, allDocs []model.DocRecord, indexedBlocks []model.DocSourceBlock, indexedRecords []model.DocSourceRecord) model.WikiSourceValidationResult {
	return compileSourceValidationWithCorpus(docs, allDocs, indexedBlocks, indexedRecords, indexedBlocks, indexedRecords)
}

func compileSourceValidationWithCorpus(docs []sourceDoc, allDocs []model.DocRecord, indexedBlocks []model.DocSourceBlock, indexedRecords []model.DocSourceRecord, corpusBlocks []model.DocSourceBlock, corpusRecords []model.DocSourceRecord) model.WikiSourceValidationResult {
	result := model.WikiSourceValidationResult{
		WikiSourceProtocol:          wikiSourceProtocolV1,
		IndexFreshness:              "current",
		GovernanceSync:              "in_sync",
		WikiSourceVerdict:           "PASS",
		WikiSourceArtifactsReviewed: len(docs),
		WikiSourceBlocksReviewed:    len(indexedBlocks),
		WikiSourceRecordsReviewed:   len(indexedRecords),
		NavigationReadiness:         "ready",
	}
	if len(docs) == 0 {
		result.WikiSourceReadiness = "not_declared"
		result.WikiSourceVerdict = "PASS"
		return result
	}

	docIDToPath := map[string]string{}
	indexedBlockKeys := sourceBlockKeySet(indexedBlocks)
	indexedRecordKeys := sourceRecordKeySet(indexedRecords)
	indexedSourceIDs := sourceResolverIDSet(corpusBlocks, corpusRecords)
	corpusRecordIDs := sourceRecordIDSet(corpusRecords)
	docIndex := buildSourceDocIndex(docs, allDocs, indexedSourceIDs)
	for _, doc := range docs {
		parsed := doc.parsed
		label := sourceDocLabel(doc)
		detail := model.WikiSourceDocumentValidation{
			DocID:           parsed.DocID,
			Path:            doc.record.Path,
			SourceProtocol:  parsed.SourceProtocol,
			HarnessProtocol: parsed.HarnessProtocol,
			Audience:        parsed.Audience,
			Imports:         parsed.Imports,
			Exports:         parsed.Exports,
			Verdict:         "PASS",
		}
		addDocBlocker := func(message string) {
			result.WikiSourceBlockers = append(result.WikiSourceBlockers, label+": "+message)
			detail.Blockers = append(detail.Blockers, message)
		}
		addDocNavigationBlocker := func(message string) {
			result.NavigationBlockers = append(result.NavigationBlockers, label+": "+message)
			detail.NavigationBlockers = append(detail.NavigationBlockers, message)
		}
		if strings.TrimSpace(parsed.DocID) == "" {
			result.WikiSourceBlockers = append(result.WikiSourceBlockers, label+": missing doc_id")
			detail.Blockers = append(detail.Blockers, "missing doc_id")
		} else if previous := docIDToPath[strings.ToUpper(parsed.DocID)]; previous != "" && previous != doc.record.Path {
			addDocBlocker("duplicate doc_id " + parsed.DocID + " also in " + previous)
		} else {
			docIDToPath[strings.ToUpper(parsed.DocID)] = doc.record.Path
		}
		if parsed.SourceProtocol != wikiSourceProtocolV1 {
			addDocBlocker("missing source_protocol=" + wikiSourceProtocolV1)
		}
		if strings.TrimSpace(parsed.HarnessProtocol) != harnessProtocolV1 {
			addDocBlocker("missing harness_protocol=" + harnessProtocolV1)
		}
		if strings.TrimSpace(parsed.Audience) == "" {
			addDocBlocker("missing audience")
		}
		if len(parsed.Imports) == 0 {
			addDocBlocker("missing imports")
		}
		if len(parsed.Exports) == 0 {
			addDocBlocker("missing exports")
		}
		for _, ref := range parsed.Imports {
			if !harnessRefExists("", docIndex, doc.record.Path, ref) {
				addDocBlocker("broken import " + ref)
			}
		}
		for _, ref := range parsed.Exports {
			if !sourceExportExists(parsed, indexedSourceIDs, ref) {
				addDocBlocker("export not indexed " + ref)
			}
		}
		if len(parsed.Blocks) == 0 {
			addDocBlocker("missing fenced toon normative block")
		}
		for _, block := range parsed.Blocks {
			blockDetail := model.WikiSourceBlockValidation{
				BlockID:       block.BlockID,
				Kind:          block.Kind,
				SourceOfTruth: block.SourceOfTruth,
				Verify:        block.Verify,
				Evidence:      block.Evidence,
				Verdict:       "PASS",
				StartLine:     block.StartLine,
				EndLine:       block.EndLine,
			}
			blockBlockers := []string{}
			if strings.TrimSpace(block.BlockID) == "" {
				blockBlockers = append(blockBlockers, "fenced toon block missing block_id")
			}
			if strings.TrimSpace(block.Kind) == "" {
				blockBlockers = append(blockBlockers, "block missing kind")
			}
			if strings.TrimSpace(block.SourceOfTruth) == "" {
				blockBlockers = append(blockBlockers, "block missing source_of_truth")
			}
			if len(block.Verify) == 0 {
				blockBlockers = append(blockBlockers, "block missing verify")
			}
			if len(block.Evidence) == 0 {
				blockBlockers = append(blockBlockers, "block missing evidence")
			}
			for _, ref := range block.Imports {
				if !harnessRefExists("", docIndex, doc.record.Path, ref) {
					blockBlockers = append(blockBlockers, "broken block import "+ref)
				}
			}
			for _, blocker := range blockBlockers {
				addDocBlocker(blocker)
			}
			if len(blockBlockers) > 0 {
				blockDetail.Verdict = "BLOCKED"
				blockDetail.Severity = "error"
			}
			if strings.TrimSpace(block.BlockID) != "" {
				if _, ok := indexedBlockKeys[sourceBlockKey(doc.record.Path, block.BlockID)]; !ok {
					addDocNavigationBlocker("block_id " + block.BlockID + " missing from doc_source_blocks")
				}
			}
			for _, record := range block.Records {
				recordDetail := model.WikiSourceRecordValidation{
					ID:        record.ID,
					Type:      record.Type,
					BlockID:   block.BlockID,
					Verdict:   "PASS",
					StartLine: record.StartLine,
					EndLine:   record.EndLine,
				}
				if isReferencableRecord(record) && strings.TrimSpace(record.ID) == "" {
					recordDetail.Verdict = "BLOCKED"
					recordDetail.Severity = "error"
					addDocBlocker("referencable record missing id")
				} else if strings.TrimSpace(record.ID) != "" {
					key := sourceRecordKey(doc.record.Path, block.BlockID, record.ID)
					if _, ok := indexedRecordKeys[key]; !ok {
						if _, external := corpusRecordIDs[normalizeHarnessRef(record.ID)]; !external {
							recordDetail.Verdict = "BLOCKED"
							recordDetail.Severity = "error"
							addDocNavigationBlocker("record_id " + record.ID + " missing from doc_source_records")
						}
					}
				}
				detail.Records = append(detail.Records, recordDetail)
			}
			detail.Blocks = append(detail.Blocks, blockDetail)
		}
		if count := sourceNormativeTableCount(doc.content); count > 0 {
			result.WikiSourceTablesReviewed += count
			detail.TablesReviewed = count
			audience := strings.ToLower(strings.TrimSpace(parsed.Audience))
			switch {
			case audience == "human" || audience == "dual":
				detail.Exceptions = append(detail.Exceptions, "audience="+audience)
			case doc.record.IsSnapshot:
				detail.Exceptions = append(detail.Exceptions, "snapshot")
			case sourceHasTableException(doc.content) && len(parsed.Blocks) > 0:
				detail.Exceptions = append(detail.Exceptions, "wiki_source_table_exception")
			default:
				addDocBlocker("normative Markdown table without allowed table exception")
			}
		}
		detail.Blockers = uniqueSortedStrings(detail.Blockers)
		detail.Warnings = uniqueSortedStrings(detail.Warnings)
		detail.NavigationBlockers = uniqueSortedStrings(detail.NavigationBlockers)
		detail.Evidence = sourceDetailEvidence(parsed)
		if len(detail.Blockers) > 0 || len(detail.NavigationBlockers) > 0 {
			detail.Verdict = "BLOCKED"
			detail.Severity = "error"
		}
		result.Documents = append(result.Documents, detail)
	}

	result.WikiSourceBlockers = uniqueSortedStrings(result.WikiSourceBlockers)
	result.WikiSourceWarnings = uniqueSortedStrings(result.WikiSourceWarnings)
	result.NavigationBlockers = uniqueSortedStrings(result.NavigationBlockers)
	if len(result.NavigationBlockers) > 0 {
		result.NavigationReadiness = "blocked"
	}
	if len(result.WikiSourceBlockers) > 0 || len(result.NavigationBlockers) > 0 {
		result.WikiSourceVerdict = "BLOCKED"
		result.WikiSourceReadiness = "blocked"
	} else if len(result.WikiSourceWarnings) > 0 {
		result.WikiSourceVerdict = "WARN"
		result.WikiSourceReadiness = "warning"
	} else {
		result.WikiSourceReadiness = "ready"
	}
	return result
}

func buildSourceDocIndex(docs []sourceDoc, allDocs []model.DocRecord, indexedSourceIDs map[string]struct{}) map[string]struct{} {
	index := map[string]struct{}{}
	for _, doc := range allDocs {
		path := filepath.ToSlash(doc.Path)
		addHarnessIndexKey(index, path)
		addHarnessIndexKey(index, strings.TrimSuffix(path, ".md"))
		addHarnessIndexKey(index, filepath.Base(path))
		addHarnessIndexKey(index, strings.TrimSuffix(filepath.Base(path), ".md"))
		addHarnessIndexKey(index, doc.DocID)
		addHarnessIndexKey(index, doc.Title)
	}
	for _, doc := range docs {
		path := filepath.ToSlash(doc.record.Path)
		addHarnessIndexKey(index, path)
		addHarnessIndexKey(index, strings.TrimSuffix(path, ".md"))
		addHarnessIndexKey(index, filepath.Base(path))
		addHarnessIndexKey(index, strings.TrimSuffix(filepath.Base(path), ".md"))
		addHarnessIndexKey(index, doc.record.DocID)
		addHarnessIndexKey(index, doc.parsed.DocID)
		for _, value := range doc.parsed.Exports {
			addHarnessIndexKey(index, value)
		}
	}
	for value := range indexedSourceIDs {
		addHarnessIndexKey(index, value)
	}
	return index
}

func sourceResolverIDSet(blocks []model.DocSourceBlock, records []model.DocSourceRecord) map[string]struct{} {
	ids := map[string]struct{}{}
	for _, block := range blocks {
		addSourceResolverKey(ids, block.DocID)
		addSourceResolverKey(ids, block.BlockID)
	}
	for _, record := range records {
		addSourceResolverKey(ids, record.RecordID)
	}
	return ids
}

func sourceRecordIDSet(records []model.DocSourceRecord) map[string]struct{} {
	ids := map[string]struct{}{}
	for _, record := range records {
		addSourceResolverKey(ids, record.RecordID)
	}
	return ids
}

func sourceBlockKeySet(blocks []model.DocSourceBlock) map[string]struct{} {
	keys := map[string]struct{}{}
	for _, block := range blocks {
		keys[sourceBlockKey(block.DocPath, block.BlockID)] = struct{}{}
	}
	return keys
}

func sourceRecordKeySet(records []model.DocSourceRecord) map[string]struct{} {
	keys := map[string]struct{}{}
	for _, record := range records {
		keys[sourceRecordKey(record.DocPath, record.BlockID, record.RecordID)] = struct{}{}
	}
	return keys
}

func sourceBlockKey(docPath string, blockID string) string {
	return normalizeHarnessScopePath(docPath) + "::" + normalizeHarnessRef(blockID)
}

func sourceRecordKey(docPath string, blockID string, recordID string) string {
	return normalizeHarnessScopePath(docPath) + "::" + normalizeHarnessRef(blockID) + "::" + normalizeHarnessRef(recordID)
}

func addSourceResolverKey(index map[string]struct{}, value string) {
	if normalized := normalizeHarnessRef(value); normalized != "" {
		index[normalized] = struct{}{}
	}
}

func sourceNormativeTableCount(content string) int {
	count := 0
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r", ""), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") && !isSourceTableSeparator(trimmed) {
			count++
		}
	}
	return count
}

func isSourceTableSeparator(line string) bool {
	line = strings.Trim(line, "| ")
	if line == "" {
		return false
	}
	for _, part := range strings.Split(line, "|") {
		part = strings.TrimSpace(part)
		if part == "" {
			return false
		}
		for _, r := range part {
			if r != '-' && r != ':' {
				return false
			}
		}
	}
	return true
}

func sourceHasTableException(content string) bool {
	normalized := strings.ToLower(content)
	return strings.Contains(normalized, "wiki_source_table_exception: true") ||
		strings.Contains(normalized, "table_exception") ||
		strings.Contains(normalized, "table-exception") ||
		strings.Contains(normalized, "table exception") ||
		strings.Contains(normalized, "markdown table exception")
}

func sourceDocLabel(doc sourceDoc) string {
	if strings.TrimSpace(doc.parsed.DocID) != "" {
		return doc.parsed.DocID
	}
	if strings.TrimSpace(doc.record.DocID) != "" {
		return doc.record.DocID
	}
	return doc.record.Path
}

func sourceExportExists(parsed wikisource.ParsedDoc, indexedSourceIDs map[string]struct{}, ref string) bool {
	if normalizeHarnessRef(ref) == normalizeHarnessRef(parsed.DocID) {
		return true
	}
	for _, block := range parsed.Blocks {
		if normalizeHarnessRef(ref) == normalizeHarnessRef(block.BlockID) {
			return true
		}
		for _, record := range block.Records {
			if normalizeHarnessRef(ref) == normalizeHarnessRef(record.ID) {
				return true
			}
		}
	}
	_, ok := indexedSourceIDs[normalizeHarnessRef(ref)]
	return ok
}

func isReferencableRecord(record wikisource.ParsedRecord) bool {
	typ := strings.ToUpper(strings.TrimSpace(record.Type))
	if typ == "" {
		typ = wikisource.RecordType(record.ID)
	}
	switch typ {
	case "RS", "FL", "RF", "TP", "TECH", "DB", "CT", "UXR", "UXI", "UJ", "VOICE", "UXS", "PROTOTYPE", "UX-VALIDATION", "UI-RFC", "HANDOFF", "STATE", "EVENT", "ERROR", "EVIDENCE", "POLICY", "PROMPT":
		return true
	default:
		return strings.TrimSpace(record.ID) != ""
	}
}

func sourceDetailEvidence(parsed wikisource.ParsedDoc) []string {
	evidence := []string{}
	for _, block := range parsed.Blocks {
		evidence = append(evidence, block.Evidence...)
	}
	return uniqueSortedStrings(evidence)
}
