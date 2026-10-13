package wikisource

import (
	"fmt"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"gopkg.in/yaml.v3"
)

// Graph source vocabulary/ownership is defined by shared SDD-WIKI-GRAPH-v1.
// This producer only derives associations; it does not compile SDD edges.
func flatGraphValue(content, key string) string {
	value := ""
	seen := false
	for _, line := range strings.Split(content, "\n") {
		if bindingLineIndent(line) != 0 {
			continue
		}
		k, v, ok := splitKeyValue(line)
		if ok && k == key {
			if seen {
				return "!conflict"
			}
			value, seen = v, true
		}
	}
	return value
}

func boundedGraphRoot(content string) string {
	lines := strings.Split(content, "\n")
	started := false
	end := 0
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			end = i + 1
			continue
		}
		if !started && strings.HasPrefix(t, "# ") {
			end = i + 1
			continue
		}
		if strings.HasPrefix(t, "#") || strings.HasPrefix(t, "```") {
			break
		}
		if bindingLineIndent(line) == 0 && !strings.HasPrefix(t, "- ") && t != "]" {
			if _, _, ok := splitKeyValue(t); !ok {
				break
			}
		}
		started = true
		end = i + 1
	}
	return strings.Join(lines[:end], "\n")
}

func parseGraphDocument(parsed *ParsedDoc, content string) {
	content = strings.ReplaceAll(content, "\r", "")
	envelopes := leadingIdentityBlocks(content)
	// Adoption is only from a bounded definition envelope, never body examples.
	if len(envelopes) == 0 {
		return
	}
	// Preserve indented lists in an unfenced source header; identity discovery
	// deliberately retains only its flat metadata and is not a list extractor.
	root := boundedGraphRoot(content)
	for i, envelope := range envelopes {
		if flatGraphValue(envelope, "wiki_graph_protocol") != "" && !strings.Contains(content, "---\n"+envelope) && !strings.Contains(content, "```toon\n"+envelope) && !strings.Contains(content, "```yaml\n"+envelope) && !strings.Contains(content, "```yml\n"+envelope) {
			envelopes[i] = root
		}
	}
	for _, envelope := range envelopes {
		if protocol := flatGraphValue(envelope, "wiki_graph_protocol"); protocol != "" {
			if parsed.GraphProtocol != "" && parsed.GraphProtocol != protocol {
				parsed.GraphProtocol = "!conflict"
			} else {
				parsed.GraphProtocol = protocol
			}
		}
	}
	if parsed.GraphProtocol == "" {
		return
	}
	parsed.DocID, _ = documentIdentityFromEnvelopes(envelopes)
	parsed.graphBindingIDs = map[string]int{}
	for _, envelope := range envelopes {
		// Only definition metadata owns the document. A materialization block's
		// kind is never promoted to document ownership.
		if flatGraphValue(envelope, "wiki_graph_protocol") == "" && flatGraphValue(envelope, "block_id") != "" {
			continue
		}
		for key, dest := range map[string]*string{"kind": &parsed.DocumentKind, "document_lifecycle": &parsed.DocumentLifecycle} {
			value := flatGraphValue(envelope, key)
			if value == "" {
				continue
			}
			if *dest != "" && *dest != value {
				*dest = "!conflict"
			} else {
				*dest = value
			}
		}
	}
	parsed.Header = ""
	if parsed.GraphProtocol != "SDD-WIKI-GRAPH-v1" || parsed.DocID == "" || parsed.DocumentKind == "" || !member(parsed.DocumentLifecycle, "active|deprecated|retired") {
		parsed.BindingOmissions = append(parsed.BindingOmissions, "invalid_graph_document_metadata")
		return
	}
	// Leading fenced envelopes are visited through the TOON pass, never twice.
	for _, envelope := range envelopes {
		isToon := false
		for _, match := range toonFencePattern.FindAllStringSubmatch(content, -1) {
			if strings.TrimSpace(match[1]) == strings.TrimSpace(envelope) {
				isToon = true
			}
		}
		if !isToon {
			parsed.Header += envelope + "\n"
			extractStrictSections(parsed, envelope, flatGraphValue(envelope, "block_id"))
		}
	}
	for _, match := range toonFencePattern.FindAllStringSubmatch(content, -1) {
		extractStrictSections(parsed, match[1], flatGraphValue(match[1], "block_id"))
	}
	// IDs are document-wide. Reject every ambiguous declaration, not just the last.
	counts := parsed.graphBindingIDs
	kept := parsed.CanonicalBindings[:0]
	for _, binding := range parsed.CanonicalBindings {
		if counts[binding.BindingID] > 1 {
			parsed.BindingOmissions = append(parsed.BindingOmissions, "duplicate_binding_id:"+binding.BindingID)
			continue
		}
		kept = append(kept, binding)
	}
	parsed.CanonicalBindings = kept
	parsed.BindingOmissions = append(parsed.BindingOmissions, "explicit_incomplete:canonical_source_collection_not_persisted")
}

// Separate from DocumentIdentity to avoid Parse/identity recursion.
func documentIdentityFromEnvelopes(envelopes []string) (string, bool) {
	id := ""
	for _, envelope := range envelopes {
		value, declared, ambiguous := metadataIdentity(envelope)
		if ambiguous || (declared && value == "") || (value != "" && id != "" && value != id) {
			return "", true
		}
		if value != "" {
			id = value
		}
	}
	return id, id != ""
}

// Read only root declarations or descendants of surfaces. Planned payload and
// response lists outside that source namespace are not declarations. Each list
// is decoded independently: repeated TOON sections remain separate declarations.
func extractStrictSections(parsed *ParsedDoc, content, blockID string) {
	lines := strings.Split(strings.ReplaceAll(content, "\r", ""), "\n")
	type parent struct {
		indent int
		key    string
	}
	stack := []parent{}
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		indent := bindingLineIndent(raw)
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		key, value, ok := splitKeyValue(trimmed)
		if !ok {
			continue
		}
		if key != "artifact_bindings" {
			if value == "" {
				stack = append(stack, parent{indent, key})
			}
			continue
		}
		allowed := len(stack) == 0 || stack[0].key == "surfaces"
		for _, ancestor := range stack {
			if member(ancestor.key, "response|responses|planned|planned_payload|wiki_code_context|candidates|new_binding_candidates|examples") {
				allowed = false
			}
		}
		if flatGraphValue(content, "projection") != "" {
			allowed = false
		}
		end := i + 1
		for end < len(lines) {
			t := strings.TrimSpace(lines[end])
			if t == "]" && value == "[" {
				end++
				break
			}
			// An explicit bracket list is bounded by its closing delimiter,
			// not indentation: TOON brace rows and their fields may be flush
			// left. Decode the whole slice below to reject malformed objects
			// or a missing closing bracket without relaxing field validation.
			if value != "[" && t != "" && bindingLineIndent(lines[end]) <= indent && !strings.HasPrefix(t, "- ") {
				break
			}
			end++
		}
		if !allowed {
			i = end - 1
			continue
		}
		snippet := value
		if value == "" || value == "[" {
			snippet = value + "\n" + strings.Join(lines[i+1:end], "\n")
		}
		var node yaml.Node
		err := yaml.Unmarshal([]byte(snippet), &node)
		if err != nil {
			// TOON also allows brace-delimited multiline objects without
			// YAML flow commas. Normalize only that bounded list form.
			if normalized, ok := toonBindingListYAML(snippet); ok {
				err = yaml.Unmarshal([]byte(normalized), &node)
			}
		}
		if err != nil || len(node.Content) != 1 || node.Content[0].Kind != yaml.SequenceNode {
			parsed.BindingOmissions = append(parsed.BindingOmissions, "invalid_artifact_bindings_list:"+blockID)
			i = end - 1
			continue
		}
		for _, object := range node.Content[0].Content {
			entry := map[string]string{}
			valid := object.Kind == yaml.MappingNode
			if valid {
				for j := 0; j+1 < len(object.Content); j += 2 {
					k, v := object.Content[j], object.Content[j+1]
					if _, exists := entry[k.Value]; exists || k.Kind != yaml.ScalarNode || v.Kind != yaml.ScalarNode || v.Tag != "!!str" {
						valid = false
					}
					entry[k.Value] = v.Value
				}
			}
			if id := entry["binding_id"]; id != "" {
				parsed.graphBindingIDs[id]++
			}
			binding, reason := strictGraphBinding(entry, parsed, blockID, digest([]byte(content)))
			if !valid {
				reason = "invalid_binding_object"
			}
			if reason != "" {
				parsed.BindingOmissions = append(parsed.BindingOmissions, fmt.Sprintf("%s:%s:%s", reason, blockID, entry["binding_id"]))
				continue
			}
			parsed.CanonicalBindings = append(parsed.CanonicalBindings, binding)
		}
		i = end - 1
	}
}

// toonBindingListYAML accepts only scalar fields inside standalone brace rows.
// It retains all keys (including forbidden ones) for strict object validation.
func toonBindingListYAML(snippet string) (string, bool) {
	lines := strings.Split(strings.TrimSpace(snippet), "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "[" || strings.TrimSpace(lines[len(lines)-1]) != "]" {
		return "", false
	}
	var output strings.Builder
	inObject, first := false, false
	for _, raw := range lines[1 : len(lines)-1] {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		switch line {
		case "{":
			if inObject {
				return "", false
			}
			inObject, first = true, true
		case "}", "},":
			if !inObject || first {
				return "", false
			}
			inObject = false
		default:
			if !inObject {
				return "", false
			}
			line = strings.TrimSuffix(line, ",")
			if _, _, ok := splitKeyValue(line); !ok {
				return "", false
			}
			if first {
				output.WriteString("- ")
				first = false
			} else {
				output.WriteString("  ")
			}
			output.WriteString(line + "\n")
		}
	}
	return output.String(), !inObject && output.Len() > 0
}

func member(value, values string) bool {
	for _, candidate := range strings.Split(values, "|") {
		if value == candidate {
			return true
		}
	}
	return false
}

func strictGraphBinding(entry map[string]string, parsed *ParsedDoc, blockID, hash string) (ParsedBinding, string) {
	for key := range entry {
		if !member(key, "binding_id|kind|role|path|symbol|surface|binding_lifecycle") {
			return ParsedBinding{}, "invalid_binding_field"
		}
	}
	for _, key := range []string{"binding_id", "kind", "role", "path", "surface", "binding_lifecycle"} {
		if strings.TrimSpace(entry[key]) == "" {
			return ParsedBinding{}, "missing_binding_field_" + key
		}
	}
	kind, role := entry["kind"], entry["role"]
	if !member(kind, "code|test|schema|migration|contract|config|ui") || !member(role, "behavior_implementation|orchestration_entrypoint|executable_test|handler|adapter|contract_schema|db_schema|db_migration|shared_component|infrastructure|runtime_config") || !member(entry["surface"], "frontend|backend|shared|integration|data|operations") || !member(entry["binding_lifecycle"], "active|deprecated") {
		return ParsedBinding{}, "invalid_binding_enum"
	}
	ownerRoles := map[string]string{
		"RF": "behavior_implementation", "TP": "executable_test", "CT": "handler|adapter|contract_schema", "DB": "db_schema|db_migration", "TECH": "shared_component|infrastructure|runtime_config", "TECH-DS": "shared_component|infrastructure|runtime_config", "FL": "orchestration_entrypoint",
	}
	if roles, ok := ownerRoles[parsed.DocumentKind]; !ok || !member(role, roles) {
		return ParsedBinding{}, "invalid_binding_document_owner"
	}
	if !validateBindingPath(parsed.SourcePath) || !validateBindingPath(entry["path"]) || strings.ContainsAny(entry["symbol"], "\x00\r\n") {
		return ParsedBinding{}, "unsafe_binding_target"
	}
	relation, targetKind := model.RelationOperates, model.TargetKindFile
	targetSymbol := entry["symbol"]
	if member(kind, "schema|migration|contract") {
		targetSymbol = ""
	}
	switch kind {
	case "test":
		relation, targetKind = model.RelationTests, model.TargetKindTest
	case "config":
		relation, targetKind = model.RelationConfigures, model.TargetKindConfig
	case "code", "ui":
		relation = normalizeRelation(role, targetKind)
		if entry["symbol"] != "" {
			targetKind = model.TargetKindSymbol
		}
	}
	// Source lifecycle axes stay independent. The legacy bridge has one column;
	// conservatively exclude either non-active axis without inventing identity.
	lifecycle := parsed.DocumentLifecycle
	if lifecycle == model.DocLifecycleActive && entry["binding_lifecycle"] != model.DocLifecycleActive {
		lifecycle = model.DocLifecycleDeprecated
	}
	return ParsedBinding{
		Path: entry["path"], Symbol: entry["symbol"], BindingID: entry["binding_id"], Kind: kind, Surface: entry["surface"], BindingLifecycle: entry["binding_lifecycle"], DocumentLifecycle: parsed.DocumentLifecycle,
		DocPath: parsed.SourcePath, DocID: parsed.DocID, BlockID: blockID, Relation: relation, Role: role,
		TargetPath: strings.ReplaceAll(entry["path"], "\\", "/"), TargetSymbol: targetSymbol, TargetKind: targetKind,
		AuthoringOrigin: model.AuthoringOriginCanonical, BindingStatus: model.BindingStatusExact, DocLifecycle: lifecycle, SourceContentHash: hash,
	}, ""
}

func extractGraphDocumentBindings(parsed ParsedDoc) []ParsedBinding {
	bindings := append([]ParsedBinding(nil), parsed.CanonicalBindings...)
	// Read-only legacy aliases remain advisory even inside an adopted document.
	for _, block := range parsed.Blocks {
		bindings = append(bindings, extractLegacyBindings(block.Content, block.BlockID, parsed.DocID, block.Kind, parsed.SourcePath)...)
	}
	bindings = append(bindings, extractLegacyBindings(parsed.Header, "", parsed.DocID, "", parsed.SourcePath)...)
	if parsed.DocumentLifecycle != "" && parsed.DocumentLifecycle != model.DocLifecycleActive {
		for i := range bindings {
			bindings[i].DocLifecycle = parsed.DocumentLifecycle
		}
	}
	return bindings
}
