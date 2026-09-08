package wikisource

import (
	"crypto/sha1"
	"encoding/hex"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// Locked field/value vocabulary for artifact_bindings objects.
var canonicalArtifactFields = map[string]struct{}{
	"doc_path": {}, "block_id": {}, "doc_id": {},
	"relation": {}, "role": {},
	"target_path": {}, "target_symbol": {}, "target_kind": {},
	"authoring_origin": {}, "binding_status": {}, "doc_lifecycle": {}, "superseded_by": {},
	"ordinal": {}, "start_line": {}, "end_line": {},
	"source_content_hash": {},
}

// Locked relations.
var validRelations = map[string]struct{}{
	model.RelationImplements: {},
	model.RelationTests:      {},
	model.RelationConfigures: {},
	model.RelationOperates:   {},
}

// Locked target kinds.
var validTargetKinds = map[string]struct{}{
	model.TargetKindFile:   {},
	model.TargetKindSymbol: {},
	model.TargetKindTest:   {},
	model.TargetKindConfig: {},
}

// ParsedBinding is an intermediate representation for binding artifacts.
type ParsedBinding struct {
	DocPath           string
	BlockID           string
	DocID             string
	Relation          string
	Role              string
	TargetPath        string
	TargetSymbol      string
	TargetKind        string
	AuthoringOrigin   string
	BindingStatus     string
	DocLifecycle      string
	SupersededBy      string
	Ordinal           int
	StartLine         int
	EndLine           int
	SourceContentHash string
}

// validateBindingPath rejects unsafe paths at parse time.
func validateBindingPath(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return false
	}
	if strings.Contains(p, "\x00") {
		return false
	}
	if strings.Contains(p, "\n") || strings.Contains(p, "\r") {
		return false
	}
	// Reject absolute paths
	if strings.HasPrefix(p, "/") {
		return false
	}
	// Reject syntactic traversal escaping
	if strings.HasPrefix(p, "../") || p == ".." {
		return false
	}
	// Also check for embedded .. segments that would escape
	if strings.Contains(p, "/../") || strings.HasSuffix(p, "/..") {
		return false
	}
	return true
}

// normalizeRelation maps legacy role/type values to the locked relation vocabulary.
func normalizeRelation(role string, targetKind string) string {
	switch strings.ToLower(role) {
	case "implementation":
		return model.RelationImplements
	case "test":
		return model.RelationTests
	case "config":
		return model.RelationConfigures
	case "compiler", "supervisor", "entrypoint", "adapter":
		return model.RelationOperates
	}
	// If target_kind is test, default to tests
	if targetKind == model.TargetKindTest {
		return model.RelationTests
	}
	// Default to operates for explicit code_links (no relation specified)
	return model.RelationOperates
}

// parseCanonicalArtifactBinding extracts one artifact_bindings object entry.
// sourceDocPath is the authoritative doc path (from ParsedDoc.SourcePath).
// If the author supplies doc_path, it is accepted only when it does not conflict
// with sourceDocPath; otherwise sourceDocPath is used for the binding's DocPath.
func parseCanonicalArtifactBinding(entry map[string]string, blockID string, docID string, sourceDocPath string, startLine int, endLine int, contentHash string) *ParsedBinding {
	// Author-supplied doc_path is optional provenance. SourceDocPath is authoritative.
	path := sourceDocPath
	if v, ok := entry["doc_path"]; ok {
		if v != "" && v != sourceDocPath {
			// Conflicting author-supplied doc_path: reject the binding entry.
			return nil
		}
		path = v
	}
	if !validateBindingPath(path) {
		return nil
	}

	targetPath, ok := entry["target_path"]
	if !ok || !validateBindingPath(targetPath) {
		return nil
	}

	relation := strings.TrimSpace(entry["relation"])
	if relation == "" {
		// For code_links, default to operates
		relation = model.RelationOperates
	}
	if _, valid := validRelations[relation]; !valid {
		return nil
	}

	targetKind := strings.TrimSpace(entry["target_kind"])
	if targetKind == "" {
		targetKind = model.TargetKindFile
	}
	if _, valid := validTargetKinds[targetKind]; !valid {
		return nil
	}

	role := strings.TrimSpace(entry["role"])
	if role == "" {
		role = entry["type"] // fall back to legacy 'type' field
	}
	if role != "" {
		relation = normalizeRelation(role, targetKind)
	}

	targetSymbol := strings.TrimSpace(entry["target_symbol"])
	ordinal := 1
	if v, ok := entry["ordinal"]; ok {
		if i := parseInt(v); i > 0 {
			ordinal = i
		}
	}

	startLineInt := startLine
	endLineInt := endLine
	if v, ok := entry["start_line"]; ok {
		if i := parseInt(v); i > 0 {
			startLineInt = i
		}
	}
	if v, ok := entry["end_line"]; ok {
		if i := parseInt(v); i > 0 {
			endLineInt = i
		}
	}

	contentHashVal := contentHash
	if v, ok := entry["source_content_hash"]; ok {
		contentHashVal = v
	}

	// Extract lifecycle and redirect fields from canonical artifact_bindings.
	docLifecycle := strings.TrimSpace(entry["doc_lifecycle"])
	if docLifecycle == "" {
		docLifecycle = model.DocLifecycleActive
	}
	supersededBy := strings.TrimSpace(entry["superseded_by"])

	// Extract binding_status and authoring_origin; default when absent.
	bindingStatus := strings.TrimSpace(entry["binding_status"])
	if bindingStatus == "" {
		bindingStatus = model.BindingStatusExact
	}
	authoringOrigin := strings.TrimSpace(entry["authoring_origin"])
	if authoringOrigin == "" {
		authoringOrigin = model.AuthoringOriginCanonical
	}

	return &ParsedBinding{
		DocPath:           strings.ReplaceAll(path, "\\", "/"),
		BlockID:           blockID,
		DocID:             docID,
		Relation:          relation,
		Role:              strings.ToLower(role),
		TargetPath:        strings.ReplaceAll(targetPath, "\\", "/"),
		TargetSymbol:      targetSymbol,
		TargetKind:        targetKind,
		AuthoringOrigin:   authoringOrigin,
		BindingStatus:     bindingStatus,
		DocLifecycle:      docLifecycle,
		SupersededBy:      supersededBy,
		Ordinal:           ordinal,
		StartLine:         startLineInt,
		EndLine:           endLineInt,
		SourceContentHash: contentHashVal,
	}
}

// parseInt is a safe string-to-int converter for field values.
func parseInt(s string) int {
	var out int
	for _, r := range s {
		if r >= '0' && r <= '9' {
			out = out*10 + int(r-'0')
		} else {
			return 0
		}
	}
	return out
}

// extractCanonicalArtifactBindings extracts artifact_bindings objects from TOON block content.
// sourceDocPath is the authoritative path of the source document.
func extractCanonicalArtifactBindings(content string, blockID string, docID string, sourceDocPath string) []ParsedBinding {
	bindings := make([]ParsedBinding, 0)
	contentHash := digest([]byte(content))

	// Find artifact_bindings: [ ... ] or artifact_bindings: [...]
	// Also handle top-level artifact_bindings key
	lines := strings.Split(strings.ReplaceAll(content, "\r", ""), "\n")
	inArtifactBindings := false
	currentObjects := []map[string]string{}
	current := map[string]string{}
	objectBraceDepth := 0

	flushObject := func() {
		if len(current) > 0 {
			currentObjects = append(currentObjects, current)
			current = map[string]string{}
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect start of artifact_bindings
		if !inArtifactBindings {
			if key, value, ok := splitKeyValue(trimmed); ok {
				if key == "artifact_bindings" {
					inArtifactBindings = true
					// Check if inline array follows
					if strings.HasPrefix(value, "[") {
						// parse inline content
						inner := strings.Trim(value, "[]")
						// Simple inline parsing: collect key:value pairs until ]
						partialLines := strings.Split(inner, "}")
						for _, part := range partialLines {
							part = strings.TrimSpace(part)
							if part == "" {
								continue
							}
							if obj, err := parseInlineObject(part + "}"); err == nil {
								currentObjects = append(currentObjects, obj)
							}
						}
					}
					continue
				}
			}
			// Also check for bare [ starting an array
			if trimmed == "[" {
				inArtifactBindings = true
				continue
			}
			continue
		}

		// We're inside artifact_bindings
		if trimmed == "]" {
			// end of array
			inArtifactBindings = false
			flushObject()
			objectBraceDepth = 0
			continue
		}
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "-") {
			// new object entry
			flushObject()
			trimmed = strings.TrimPrefix(trimmed, "-")
			trimmed = strings.TrimSpace(trimmed)
		}
		if strings.Contains(trimmed, "{") {
			objectBraceDepth++
			continue
		}
		if strings.Contains(trimmed, "}") {
			objectBraceDepth--
			if objectBraceDepth <= 0 {
				flushObject()
				objectBraceDepth = 0
				continue
			}
		}
		// key: value inside object
		if key, value, ok := splitKeyValue(trimmed); ok {
			if _, valid := canonicalArtifactFields[key]; valid {
				current[key] = value
			}
			// Also accept legacy fields for convenience in inline objects
			if key == "type" {
				current["type"] = value
			}
		}
	}
	flushObject()

	// Build bindings from collected objects
	for _, obj := range currentObjects {
		if binding := parseCanonicalArtifactBinding(obj, blockID, docID, sourceDocPath, 0, 0, contentHash); binding != nil {
			bindings = append(bindings, *binding)
		}
	}

	return bindings
}

// parseInlineObject parses a single artifact_bindings object from inline text.
func parseInlineObject(raw string) (map[string]string, error) {
	result := map[string]string{}
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "{") {
		raw = strings.TrimPrefix(raw, "{")
	}
	if strings.HasSuffix(raw, "}") {
		raw = strings.TrimSuffix(raw, "}")
	}
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	for _, part := range parts {
		if key, value, ok := splitKeyValue(strings.TrimSpace(part)); ok {
			if _, valid := canonicalArtifactFields[key]; valid {
				result[key] = value
			}
		}
	}
	return result, nil
}

// extractLegacyBindings normalizes legacy alias fields into ParsedBinding.
// sourceDocPath is the authoritative path of the source document (replaces the
// previous practice of using docID as DocPath).
func extractLegacyBindings(content string, blockID string, docID string, kind string, sourceDocPath string) []ParsedBinding {
	bindings := make([]ParsedBinding, 0)
	contentHash := digest([]byte(content))
	startLine := 0

	// Check for implementation_anchors (legacy)
	for i, value := range keyValues(content, "implementation_anchors") {
		if !validateBindingPath(value) {
			continue
		}
		bindings = append(bindings, ParsedBinding{
			DocPath:           sourceDocPath,
			BlockID:           blockID,
			DocID:             docID,
			Relation:          model.RelationImplements,
			TargetPath:        strings.ReplaceAll(value, "\\", "/"),
			TargetKind:        model.TargetKindFile,
			AuthoringOrigin:   model.AuthoringOriginLegacy,
			BindingStatus:     model.BindingStatusExact,
			DocLifecycle:      model.DocLifecycleActive,
			SupersededBy:      "",
			Ordinal:           i + 1,
			StartLine:         startLine,
			EndLine:           startLine,
			SourceContentHash: contentHash,
		})
	}

	// Check for code_links (legacy, defaults to operates)
	for i, value := range keyValues(content, "code_links") {
		if !validateBindingPath(value) {
			continue
		}
		bindings = append(bindings, ParsedBinding{
			DocPath:           sourceDocPath,
			BlockID:           blockID,
			DocID:             docID,
			Relation:          model.RelationOperates,
			TargetPath:        strings.ReplaceAll(value, "\\", "/"),
			TargetKind:        model.TargetKindFile,
			AuthoringOrigin:   model.AuthoringOriginLegacy,
			BindingStatus:     model.BindingStatusExact,
			DocLifecycle:      model.DocLifecycleActive,
			SupersededBy:      "",
			Ordinal:           i + 1,
			StartLine:         startLine,
			EndLine:           startLine,
			SourceContentHash: contentHash,
		})
	}

	// Check for test_links (legacy, maps to tests)
	for i, value := range keyValues(content, "test_links") {
		if !validateBindingPath(value) {
			continue
		}
		bindings = append(bindings, ParsedBinding{
			DocPath:           sourceDocPath,
			BlockID:           blockID,
			DocID:             docID,
			Relation:          model.RelationTests,
			TargetPath:        strings.ReplaceAll(value, "\\", "/"),
			TargetKind:        model.TargetKindTest,
			AuthoringOrigin:   model.AuthoringOriginLegacy,
			BindingStatus:     model.BindingStatusExact,
			DocLifecycle:      model.DocLifecycleActive,
			SupersededBy:      "",
			Ordinal:           i + 1,
			StartLine:         startLine,
			EndLine:           startLine,
			SourceContentHash: contentHash,
		})
	}

	// Check for implements (legacy key)
	for i, value := range keyValues(content, "implements") {
		if !validateBindingPath(value) {
			continue
		}
		bindings = append(bindings, ParsedBinding{
			DocPath:           sourceDocPath,
			BlockID:           blockID,
			DocID:             docID,
			Relation:          model.RelationImplements,
			TargetPath:        strings.ReplaceAll(value, "\\", "/"),
			TargetKind:        model.TargetKindFile,
			AuthoringOrigin:   model.AuthoringOriginLegacy,
			BindingStatus:     model.BindingStatusExact,
			DocLifecycle:      model.DocLifecycleActive,
			SupersededBy:      "",
			Ordinal:           i + 1,
			StartLine:         startLine,
			EndLine:           startLine,
			SourceContentHash: contentHash,
		})
	}

	// Check for tests (legacy key)
	for i, value := range keyValues(content, "tests") {
		if !validateBindingPath(value) {
			continue
		}
		bindings = append(bindings, ParsedBinding{
			DocPath:           sourceDocPath,
			BlockID:           blockID,
			DocID:             docID,
			Relation:          model.RelationTests,
			TargetPath:        strings.ReplaceAll(value, "\\", "/"),
			TargetKind:        model.TargetKindTest,
			AuthoringOrigin:   model.AuthoringOriginLegacy,
			BindingStatus:     model.BindingStatusExact,
			DocLifecycle:      model.DocLifecycleActive,
			SupersededBy:      "",
			Ordinal:           i + 1,
			StartLine:         startLine,
			EndLine:           startLine,
			SourceContentHash: contentHash,
		})
	}

	return bindings
}

// ExtractBindings parses canonical and legacy binding declarations from a document.
// Returns all parsed bindings in deterministic order.
func ExtractBindings(parsed ParsedDoc) []ParsedBinding {
	allBindings := make([]ParsedBinding, 0)
	sourceDocPath := parsed.SourcePath
	if sourceDocPath == "" {
		sourceDocPath = parsed.DocPath
	}

	// Process canonical artifact_bindings first
	for _, block := range parsed.Blocks {
		canonical := extractCanonicalArtifactBindings(block.Content, block.BlockID, parsed.DocID, sourceDocPath)
		allBindings = append(allBindings, canonical...)
		// Then legacy keys within the block
		legacy := extractLegacyBindings(block.Content, block.BlockID, parsed.DocID, block.Kind, sourceDocPath)
		allBindings = append(allBindings, legacy...)
	}

	// Legacy keys in document header (outside blocks)
	hasBlocks := len(parsed.Blocks) > 0
	var header string
	if hasBlocks {
		header = sourceHeader(parsed.Blocks[0].Content)
		for _, block := range parsed.Blocks {
			header += "\n" + block.Content
		}
	} else {
		header = sourceHeader("")
	}
	headerBindings := extractLegacyBindings(header, "", parsed.DocID, "", sourceDocPath)
	allBindings = append(allBindings, headerBindings...)

	return allBindings
}

// SourceBindings converts parsed bindings into model.DocArtifactBinding rows.
// It assigns stable binding refs and deterministic ordering.
func SourceBindings(parsed ParsedDoc, indexedAt int64) []model.DocArtifactBinding {
	parsedBindings := ExtractBindings(parsed)
	bindings := make([]model.DocArtifactBinding, 0, len(parsedBindings))

	for i, pb := range parsedBindings {
		binding := model.DocArtifactBinding{
			DocPath:           pb.DocPath,
			BlockID:           pb.BlockID,
			DocID:             pb.DocID,
			Relation:          pb.Relation,
			Role:              pb.Role,
			TargetPath:        pb.TargetPath,
			TargetSymbol:      pb.TargetSymbol,
			TargetKind:        pb.TargetKind,
			AuthoringOrigin:   pb.AuthoringOrigin,
			BindingStatus:     pb.BindingStatus,
			DocLifecycle:      pb.DocLifecycle,
			SupersededBy:      pb.SupersededBy,
			Ordinal:           pb.Ordinal,
			StartLine:         pb.StartLine,
			EndLine:           pb.EndLine,
			SourceContentHash: pb.SourceContentHash,
			IndexedAt:         indexedAt,
		}
		// Compute binding ref only from identity fields
		binding.BindingRef = model.WikiCodeBindingRef(
			docPathFromRecord(pb.DocPath, pb.DocID),
			pb.BlockID,
			pb.DocID,
			pb.Relation,
			pb.TargetPath,
			pb.TargetSymbol,
			pb.TargetKind,
		)
		// Override ordinal for ordering
		binding.Ordinal = i + 1
		bindings = append(bindings, binding)
	}

	// Deterministic sort: doc path, block, ordinal, relation, target path, symbol, role
	sort.Slice(bindings, func(i, j int) bool {
		a, b := bindings[i], bindings[j]
		if a.DocPath != b.DocPath {
			return a.DocPath < b.DocPath
		}
		if a.BlockID != b.BlockID {
			return a.BlockID < b.BlockID
		}
		if a.Ordinal != b.Ordinal {
			return a.Ordinal < b.Ordinal
		}
		if a.Relation != b.Relation {
			return a.Relation < b.Relation
		}
		if a.TargetPath != b.TargetPath {
			return a.TargetPath < b.TargetPath
		}
		if a.TargetSymbol != b.TargetSymbol {
			return a.TargetSymbol < b.TargetSymbol
		}
		return a.Role < b.Role
	})

	// Re-assign ordinals after sorting
	for i := range bindings {
		bindings[i].Ordinal = i + 1
	}

	return bindings
}

// docPathFromRecord returns the canonical path key used in binding identity.
func docPathFromRecord(docPath string, docID string) string {
	if docPath != "" {
		return docPath
	}
	return docID
}

const ProtocolV1 = "SDD-WIKI-SOURCE-v1"

var (
	toonFencePattern = regexp.MustCompile("(?ms)^```toon\\s*$\\n(.*?)^```\\s*$")
)

type ParsedDoc struct {
	DocPath         string
	DeclaresSource  bool
	SourceProtocol  string
	DocID           string
	HarnessProtocol string
	Audience        string
	Imports         []string
	Exports         []string
	Blocks          []ParsedBlock
	Records         []ParsedRecord
	Mentions        []model.DocMention
	// SourcePath is the original path supplied to Parse(), used as the
	// authoritative DocPath for bindings when the author does not supply one.
	SourcePath string
}

type ParsedBlock struct {
	BlockID       string
	Kind          string
	SourceOfTruth string
	Imports       []string
	Exports       []string
	Verify        []string
	Evidence      []string
	Records       []ParsedRecord
	StartLine     int
	EndLine       int
	ContentHash   string
	Content       string
}

type ParsedRecord struct {
	ID        string
	Type      string
	Ordinal   int
	StartLine int
	EndLine   int
}

func Parse(docPath string, content string, indexedAt int64) ParsedDoc {
	canonicalPath := filepath.ToSlash(docPath)
	parsed := ParsedDoc{DocPath: canonicalPath, SourcePath: canonicalPath}
	if !DeclaresSource(content) {
		return parsed
	}
	header := sourceHeader(content)
	// Some canonical documents put the source envelope in the leading TOON
	// block rather than frontmatter. Include that bounded block in the header
	// view so its owner and imports remain source metadata, not body examples.
	for _, block := range leadingIdentityBlocks(content) {
		if hasSourceProtocol(block) {
			header += "\n" + block
		}
	}
	parsed.DeclaresSource = true
	parsed.SourceProtocol = firstNonEmpty(firstKeyValue(header, "wiki_source_protocol"), firstKeyValue(header, "source_protocol"))
	parsed.DocID = firstNonEmpty(firstKeyValue(header, "doc_id"), firstKeyValue(header, "id"))
	if _, declared, ambiguous := metadataIdentity(header); declared && ambiguous {
		parsed.DocID = ""
	}
	parsed.HarnessProtocol = firstKeyValue(header, "harness_protocol")
	parsed.Audience = firstKeyValue(header, "audience")
	parsed.Imports = uniqueValues(append(keyValues(header, "imports"), keyValues(header, "links.imports")...))
	parsed.Exports = uniqueValues(append(keyValues(header, "exports"), keyValues(header, "links.exports")...))

	seenMention := map[string]struct{}{}
	addMention := func(kind string, value string) {
		value = cleanScalar(value)
		if value == "" {
			return
		}
		key := kind + "::" + value
		if _, ok := seenMention[key]; ok {
			return
		}
		seenMention[key] = struct{}{}
		parsed.Mentions = append(parsed.Mentions, model.DocMention{DocPath: parsed.DocPath, MentionType: kind, MentionValue: value})
	}
	addMention("source_protocol", ProtocolV1)
	addMention("doc_id", parsed.DocID)
	addMention("source_audience", parsed.Audience)
	for _, value := range parsed.Imports {
		addMention("source_import", value)
	}
	for _, value := range parsed.Exports {
		addMention("source_export", value)
	}
	for _, value := range keyValues(header, "implements") {
		addMention("implements", value)
	}
	for _, value := range keyValues(header, "tests") {
		addMention("test_file", value)
	}
	for _, value := range keyValues(header, "code_links") {
		addMention("implements", value)
	}
	for _, value := range keyValues(header, "test_links") {
		addMention("test_file", value)
	}

	matches := toonFencePattern.FindAllStringSubmatchIndex(content, -1)
	for idx, match := range matches {
		if len(match) < 4 {
			continue
		}
		blockContent := content[match[2]:match[3]]
		block := ParsedBlock{
			BlockID:       firstKeyValue(blockContent, "block_id"),
			Kind:          firstKeyValue(blockContent, "kind"),
			SourceOfTruth: firstKeyValue(blockContent, "source_of_truth"),
			Imports:       uniqueValues(append(keyValues(blockContent, "imports"), keyValues(blockContent, "links.imports")...)),
			Exports:       uniqueValues(append(keyValues(blockContent, "exports"), keyValues(blockContent, "links.exports")...)),
			Verify:        keyValues(blockContent, "verify"),
			Evidence:      keyValues(blockContent, "evidence"),
			StartLine:     lineNumberAt(content, match[0]),
			EndLine:       lineNumberAt(content, match[1]),
			ContentHash:   digest([]byte(blockContent)),
			Content:       blockContent,
		}
		if block.BlockID != "" {
			addMention("block_id", block.BlockID)
		}
		for _, value := range keyValues(blockContent, "implements") {
			addMention("implements", value)
		}
		for _, value := range keyValues(blockContent, "tests") {
			addMention("test_file", value)
		}
		for _, value := range keyValues(blockContent, "code_links") {
			addMention("implements", value)
		}
		for _, value := range keyValues(blockContent, "test_links") {
			addMention("test_file", value)
		}
		block.Records = extractRecords(blockContent, parsed.DocID, block.BlockID, block.StartLine, block.EndLine)
		for _, record := range block.Records {
			if record.ID != "" {
				addMention("record_id", record.ID)
				parsed.Records = append(parsed.Records, record)
			} else if record.Type != "" {
				parsed.Records = append(parsed.Records, record)
			}
		}
		parsed.Blocks = append(parsed.Blocks, block)
		_ = indexedAt
		_ = idx
	}
	return parsed
}

func sourceHeader(content string) string {
	idx := strings.Index(content, "\n```toon")
	if idx < 0 {
		return content
	}
	return content[:idx]
}

func SourceBlocks(parsed ParsedDoc, indexedAt int64) []model.DocSourceBlock {
	blocks := make([]model.DocSourceBlock, 0, len(parsed.Blocks))
	for idx, block := range parsed.Blocks {
		if block.BlockID == "" {
			continue
		}
		blocks = append(blocks, model.DocSourceBlock{
			DocPath:      parsed.DocPath,
			BlockID:      block.BlockID,
			DocID:        parsed.DocID,
			Kind:         block.Kind,
			SourceFormat: ProtocolV1,
			Ordinal:      idx + 1,
			StartLine:    block.StartLine,
			EndLine:      block.EndLine,
			ContentHash:  block.ContentHash,
			IndexedAt:    indexedAt,
		})
	}
	return blocks
}

func SourceRecords(parsed ParsedDoc, indexedAt int64) []model.DocSourceRecord {
	records := make([]model.DocSourceRecord, 0, len(parsed.Records))
	ordinal := 0
	for _, block := range parsed.Blocks {
		for _, record := range block.Records {
			if record.ID == "" || block.BlockID == "" {
				continue
			}
			ordinal++
			records = append(records, model.DocSourceRecord{
				DocPath:     parsed.DocPath,
				BlockID:     block.BlockID,
				RecordID:    record.ID,
				RecordType:  firstNonEmpty(record.Type, RecordType(record.ID)),
				Ordinal:     ordinal,
				StartLine:   record.StartLine,
				EndLine:     record.EndLine,
				ContentHash: digest([]byte(block.Content + record.ID)),
				IndexedAt:   indexedAt,
			})
		}
	}
	return records
}

func DeclaresSource(content string) bool {
	return leadingSourceDeclaration(content)
}

func leadingSourceDeclaration(content string) bool {
	for _, block := range leadingIdentityBlocks(content) {
		if hasSourceProtocol(block) {
			return true
		}
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r", ""), "\n")
	seenHeading := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			if !seenHeading && strings.HasPrefix(trimmed, "# ") {
				seenHeading = true
				continue
			}
			return false
		}
		key, value, ok := splitKeyValue(trimmed)
		if !ok {
			return false
		}
		if (key == "source_protocol" || key == "wiki_source_protocol") && value == ProtocolV1 {
			return true
		}
	}
	return false
}

// DocumentIdentity returns the explicitly declared owner of a document. It
// only considers the bounded metadata envelopes that the repository supports:
// YAML frontmatter and a leading fenced Harness YAML block. Source documents
// retain the existing SDD parser as their source of identity. Body references,
// imports, and records are deliberately not candidates for document ownership.
// The declared result is true when an owner field was present, including an
// empty or ambiguous declaration; callers must not fall back to body scanning
// in that case.
func DocumentIdentity(content string) (id string, declared bool) {
	identities := make([]string, 0, 2)
	declarations := false
	for _, block := range leadingIdentityBlocks(content) {
		blockID, blockDeclared, ambiguous := metadataIdentity(block)
		if !blockDeclared {
			continue
		}
		declarations = true
		if ambiguous {
			return "", true
		}
		identities = append(identities, blockID)
	}
	if declarations {
		for _, candidate := range identities {
			if candidate == "" {
				continue
			}
			if id != "" && id != candidate {
				return "", true
			}
			id = candidate
		}
		return id, true
	}

	if DeclaresSource(content) {
		if _, declared, ambiguous := metadataIdentity(sourceHeader(content)); declared && ambiguous {
			return "", true
		}
		parsed := Parse("", content, 0)
		if parsed.DocID != "" {
			return parsed.DocID, true
		}
	}
	return "", false
}

// MetadataReferences returns links declared by a leading supported metadata
// envelope. These values are references only; callers must not use them as a
// document owner.
func MetadataReferences(content string) []string {
	values := make([]string, 0)
	for _, block := range leadingIdentityBlocks(content) {
		values = append(values, keyValues(block, "imports")...)
		values = append(values, keyValues(block, "links.imports")...)
		values = append(values, keyValues(block, "exports")...)
		values = append(values, keyValues(block, "links.exports")...)
	}
	return uniqueValues(values)
}

func leadingIdentityBlocks(content string) []string {
	lines := strings.Split(strings.ReplaceAll(content, "\r", ""), "\n")
	blocks := make([]string, 0, 2)
	seenHeading := false
	for i := 0; i < len(lines); {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			i++
			continue
		}
		if trimmed == "---" && len(blocks) == 0 {
			start := i + 1
			closed := false
			for j := start; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) == "---" {
					blocks = append(blocks, strings.Join(lines[start:j], "\n"))
					i = j + 1
					closed = true
					break
				}
			}
			if !closed {
				break
			}
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			if !seenHeading && strings.HasPrefix(trimmed, "# ") {
				seenHeading = true
				i++
				continue
			}
			break
		}
		if strings.EqualFold(trimmed, "```yaml") || strings.EqualFold(trimmed, "```yml") || strings.EqualFold(trimmed, "```toon") {
			start := i + 1
			closed := false
			for j := start; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) == "```" {
					block := strings.Join(lines[start:j], "\n")
					if !supportedMetadataProtocol(block) {
						return blocks
					}
					blocks = append(blocks, block)
					i = j + 1
					closed = true
					break
				}
			}
			if !closed {
				break
			}
			continue
		}
		break
	}
	return blocks
}

func supportedMetadataProtocol(content string) bool {
	return hasHarnessProtocol(content) || hasSourceProtocol(content)
}

func hasHarnessProtocol(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		key, value, ok := splitKeyValue(strings.TrimSpace(line))
		if ok && key == "harness_protocol" && value == "SDD-HARNESS-v1" {
			return true
		}
	}
	return false
}

func hasSourceProtocol(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		key, value, ok := splitKeyValue(strings.TrimSpace(line))
		if ok && (key == "source_protocol" || key == "wiki_source_protocol") && value == ProtocolV1 {
			return true
		}
	}
	return false
}

func metadataIdentity(content string) (id string, declared bool, ambiguous bool) {
	var docIDs, ids []string
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		key, value, ok := splitKeyValue(strings.TrimSpace(line))
		if !ok {
			continue
		}
		value = ownerScalar(value)
		switch key {
		case "doc_id":
			declared = true
			docIDs = append(docIDs, value)
		case "id":
			declared = true
			ids = append(ids, value)
		}
	}
	if !sameDeclaredValue(docIDs) {
		return "", declared, true
	}
	// doc_id is the explicit owner field; id is a compatibility fallback and
	// may also occur in source metadata for a record. Do not let that fallback
	// shadow an explicit doc_id in the same envelope.
	if len(docIDs) > 0 {
		return docIDs[0], declared, false
	}
	if !sameDeclaredValue(ids) {
		return "", declared, true
	}
	if len(ids) > 0 {
		return ids[0], declared, false
	}
	return "", declared, false
}

func ownerScalar(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "#") {
		return ""
	}
	if idx := strings.Index(value, " #"); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	if value == "|" || value == ">" {
		return ""
	}
	if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') {
		quote := value[0]
		if end := strings.IndexByte(value[1:], quote); end >= 0 {
			return value[1 : end+1]
		}
	}
	return strings.Trim(strings.TrimSpace(value), `"'`)
}

func sameDeclaredValue(values []string) bool {
	if len(values) < 2 {
		return true
	}
	for _, value := range values[1:] {
		if value != values[0] {
			return false
		}
	}
	return true
}

func RecordType(id string) string {
	if idx := strings.Index(id, "-"); idx > 0 {
		return strings.ToUpper(id[:idx])
	}
	return ""
}

func extractRecords(content string, docID string, blockID string, startLine int, endLine int) []ParsedRecord {
	records := make([]ParsedRecord, 0)
	for ordinal, value := range keyValues(content, "id") {
		if value == "" || strings.EqualFold(value, docID) || strings.EqualFold(value, blockID) {
			continue
		}
		records = append(records, ParsedRecord{
			ID:        value,
			Type:      firstNonEmpty(firstKeyValueNearRecord(content, value, "type"), RecordType(value)),
			Ordinal:   ordinal + 1,
			StartLine: startLine,
			EndLine:   endLine,
		})
	}
	for _, record := range recordsFromList(content, startLine, endLine) {
		if record.ID != "" && (strings.EqualFold(record.ID, docID) || strings.EqualFold(record.ID, blockID)) {
			continue
		}
		duplicate := false
		for _, existing := range records {
			if existing.ID != "" && strings.EqualFold(existing.ID, record.ID) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			records = append(records, record)
		}
	}
	return records
}

func recordsFromList(content string, startLine int, endLine int) []ParsedRecord {
	lines := strings.Split(strings.ReplaceAll(content, "\r", ""), "\n")
	records := []ParsedRecord{}
	inRecords := false
	current := map[string]string{}
	flush := func() {
		if len(current) == 0 {
			return
		}
		id := cleanScalar(current["id"])
		typ := firstNonEmpty(current["type"], current["kind"], RecordType(id))
		if id != "" || typ != "" {
			records = append(records, ParsedRecord{ID: id, Type: cleanScalar(typ), Ordinal: len(records) + 1, StartLine: startLine, EndLine: endLine})
		}
		current = map[string]string{}
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "records:" {
			inRecords = true
			continue
		}
		if !inRecords {
			continue
		}
		if trimmed != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(trimmed, "- ") {
			flush()
			inRecords = false
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			flush()
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		}
		if key, value, ok := splitKeyValue(trimmed); ok {
			current[key] = value
		}
	}
	flush()
	return records
}

func keyValues(content string, key string) []string {
	lines := strings.Split(strings.ReplaceAll(content, "\r", ""), "\n")
	values := make([]string, 0)
	inList := false
	keyPrefix := key + ":"
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, keyPrefix) {
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, keyPrefix))
			if raw != "" && raw != "[]" {
				values = append(values, splitInlineValues(raw)...)
			}
			inList = raw == "" || raw == "[]"
			continue
		}
		if inList {
			if strings.HasPrefix(trimmed, "- ") {
				item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
				if key, value, ok := splitKeyValue(item); ok && key == "id" {
					values = append(values, value)
				} else {
					values = append(values, item)
				}
				continue
			}
			if trimmed != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				inList = false
			}
		}
	}
	return uniqueValues(values)
}

func firstKeyValue(content string, key string) string {
	values := keyValues(content, key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func firstKeyValueNearRecord(content string, id string, key string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r", ""), "\n")
	for i, line := range lines {
		if !strings.Contains(line, id) {
			continue
		}
		for j := i - 2; j <= i+2; j++ {
			if j < 0 || j >= len(lines) {
				continue
			}
			if k, value, ok := splitKeyValue(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[j]), "- "))); ok && k == key {
				return value
			}
		}
	}
	return ""
}

func splitInlineValues(raw string) []string {
	raw = strings.Trim(strings.TrimSpace(raw), "[]")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		values = append(values, cleanScalar(part))
	}
	return values
}

func splitKeyValue(line string) (string, string, bool) {
	idx := strings.Index(line, ":")
	if idx <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:idx])
	value := strings.TrimSpace(line[idx+1:])
	if key == "" {
		return "", "", false
	}
	return key, cleanScalar(value), true
}

func uniqueValues(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = cleanScalar(value)
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

func cleanScalar(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"'`)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func lineNumberAt(content string, offset int) int {
	if offset <= 0 {
		return 1
	}
	if offset > len(content) {
		offset = len(content)
	}
	return strings.Count(content[:offset], "\n") + 1
}

func digest(content []byte) string {
	sum := sha1.Sum(content)
	return hex.EncodeToString(sum[:])
}
