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

const ProtocolV1 = "SDD-WIKI-SOURCE-v1"

var (
	toonFencePattern      = regexp.MustCompile("(?ms)^```toon\\s*$\\n(.*?)^```\\s*$")
	wikiSourceDeclPattern = regexp.MustCompile(`(?m)^\s*(?:wiki_source_protocol|source_protocol):\s*SDD-WIKI-SOURCE-v1\s*$`)
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
	parsed := ParsedDoc{DocPath: filepath.ToSlash(docPath)}
	if !DeclaresSource(content) {
		return parsed
	}
	header := sourceHeader(content)
	parsed.DeclaresSource = true
	parsed.SourceProtocol = firstNonEmpty(firstKeyValue(header, "wiki_source_protocol"), firstKeyValue(header, "source_protocol"))
	parsed.DocID = firstNonEmpty(firstKeyValue(header, "doc_id"), firstKeyValue(header, "id"))
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
	return wikiSourceDeclPattern.MatchString(content)
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
