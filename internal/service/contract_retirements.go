package service

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/wikisource"
	"gopkg.in/yaml.v3"
)

// Retirement inventory is machine YAML, not an SDD document contract.
func validateWikiContractRetirements(root, docPath, body string, index wikisource.ContractWikiIDIndex) []string {
	var warnings []string
	warn := func(message string) { warnings = append(warnings, docPath+": "+message) }
	blocks := candidateHarnessYAMLBlocks(body)
	if len(blocks) == 0 {
		blocks = []string{body}
	} else if retirementHasUnparsedStructure(body) {
		warn("retirements unparsed structured content outside YAML inventory blocks")
	}
	var rows []any
	found := false
	explicitEmpty := false
	plain := strings.ToLower(strings.ReplaceAll(body, "\r", ""))
	lines := strings.Split(plain, "\n")
	var prose []string
	proseNone := false
	fence := ""
	frontmatter := len(lines) > 0 && strings.TrimSpace(lines[0]) == "---"
	// Only pure prose declares an empty inventory without machine parsing.
	// A none sentence must never conceal malformed or unrecognized YAML.
	structured := frontmatter || strings.Contains(body, "```") || strings.Contains(body, "~~~")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if frontmatter {
			if i > 0 && trimmed == "---" {
				frontmatter = false
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			marker := trimmed[:3]
			if fence == "" {
				fence = marker
			} else if fence == marker {
				fence = ""
			}
			continue
		}
		if trimmed != "" && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			structured = true
		}
		if fence != "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		line = trimmed
		if line != "" && !strings.HasPrefix(line, "#") {
			prose = append(prose, line)
			if line == "no hay retiros registrados." || line == "no hay retiros registrados. este registro no modifica ni retira identificadores existentes." {
				proseNone = true
			}
		}
	}
	if len(prose) == 1 && proseNone && !structured {
		found = true
		explicitEmpty = true
	}
	for blockIndex, block := range blocks {
		if explicitEmpty && len(prose) == 1 && !structured {
			continue
		}
		var raw any
		decoder := yaml.NewDecoder(strings.NewReader(block))
		if err := decoder.Decode(&raw); err != nil {
			warn("retirements invalid YAML: " + err.Error())
			continue
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			warn("retirements YAML block must contain exactly one complete document")
			continue
		}
		if blockIndex == 0 && strings.HasPrefix(strings.TrimSpace(body), "---") && retirementDocumentMetadata(raw) {
			if proseNone && len(prose) == 1 && !retirementHasUnparsedStructure(body) {
				found = true
				explicitEmpty = true
			}
			continue
		}
		switch value := raw.(type) {
		case []any:
			rows = append(rows, value...)
			found = true
		case map[string]any:
			for _, key := range wikiContractForbiddenKeys {
				if _, exists := value[key]; exists {
					warn("forbidden key " + key)
				}
			}
			inventoryFound := false
			for _, key := range []string{"retirements", "entries"} {
				if inventory, exists := value[key]; exists {
					inventoryFound = true
					if entries, ok := inventory.([]any); ok {
						rows = append(rows, entries...)
						found = true
						if len(entries) == 0 {
							explicitEmpty = true
						}
					} else {
						warn(key + " must be a list")
					}
				}
			}
			if !inventoryFound {
				if _, exists := value["old_path"]; exists {
					rows = append(rows, value)
					found = true
				} else {
					warn("retirements YAML block has no recognized inventory")
				}
			}
		default:
			warn("retirements YAML block must be an inventory mapping or list")
		}
	}
	if !found {
		warn("parser_warning: no retirement rows parsed")
	}
	if (explicitEmpty || proseNone) && len(rows) > 0 {
		warn("contradictory empty retirement inventory and rows")
	}
	allowed := map[string]bool{"id": true, "old_path": true, "archived_path": true, "superseded_by": true, "terminal": true, "reason": true}
	for n, entry := range rows {
		row, ok := entry.(map[string]any)
		if !ok {
			warn(fmt.Sprintf("retirement row %d must be a mapping", n+1))
			continue
		}
		label := fmt.Sprintf("retirement row %d", n+1)
		for key := range allowed {
			// Terminal retirements do not need a successor declaration.
			if key == "superseded_by" {
				continue
			}
			if _, exists := row[key]; !exists {
				warn(label + ": missing " + key)
			}
		}
		for key := range row {
			if !allowed[key] {
				warn(label + ": forbidden key " + key)
			}
		}
		for _, key := range []string{"id", "old_path", "archived_path", "reason"} {
			if strings.TrimSpace(stringField(row, key)) == "" {
				warn(label + ": nonempty string required for " + key)
			}
		}
		terminal, isBool := row["terminal"].(bool)
		if !isBool {
			warn(label + ": terminal must be boolean")
		}
		successor := strings.TrimSpace(stringField(row, "superseded_by"))
		if value := row["superseded_by"]; value != nil {
			if _, ok := value.(string); !ok {
				warn(label + ": superseded_by must be a string or null")
			}
		}
		if isBool && !terminal && successor == "" {
			warn(label + ": nonterminal retirement requires superseded_by")
		}
		if successor != "" && index.LookupKind(successor) == "" {
			warn(label + ": superseded_by target missing: " + successor)
		}
		oldPath := strings.TrimSpace(stringField(row, "old_path"))
		oldAbs, oldRel, oldOK := retirementRepoPath(root, docPath, oldPath)
		if !oldOK || strings.Contains("/"+oldRel, "/_retired/") {
			warn(label + ": old_path must be a legacy repo path outside _retired")
		} else if _, err := os.Stat(oldAbs); err == nil {
			warn(label + ": old_path still present: " + oldPath)
		} else if !os.IsNotExist(err) {
			warn(label + ": old_path unreadable: " + oldPath)
		}
		archived := strings.TrimSpace(stringField(row, "archived_path"))
		abs, rel, valid := retirementRepoPath(root, docPath, archived)
		wikiDir := filepath.ToSlash(filepath.Dir(docPath))
		archiveRel := strings.TrimPrefix(rel, wikiDir+"/")
		parts := strings.Split(archiveRel, "/")
		if !valid || len(parts) < 3 || parts[0] != "_retired" || parts[1] == "" {
			warn(label + ": archived_path must be under _retired/<kind>/")
		} else {
			if _, known := wikiContractKinds[parts[1]]; !known {
				warn(label + ": unknown archived kind " + parts[1])
			}
			if info, err := os.Stat(abs); err != nil || info.IsDir() {
				warn(label + ": archived_path missing: " + archived)
			}
		}
	}
	if strings.Contains(strings.ToLower(body), "```toon") {
		warn("forbidden normative toon fence")
	}
	return warnings
}

// A WIKI-CONTRACT-v1 governance frontmatter describes this registry document;
// it is not an additional inventory. Unknown shapes still go through validation.
func retirementDocumentMetadata(raw any) bool {
	header, ok := raw.(map[string]any)
	if !ok || header["wiki_contract"] != "WIKI-CONTRACT-v1" || header["kind"] != "governance" {
		return false
	}
	for key := range header {
		switch key {
		case "wiki_contract", "id", "kind", "status", "title", "version", "language", "model_revision":
		default:
			return false
		}
	}
	return true
}

// Every machine-looking block must be accounted for, even if another block
// already supplied a valid inventory. Prose/headings are not parsed as rows.
func retirementHasUnparsedStructure(body string) bool {
	remaining := body
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, "---") {
		rest := trimmed[3:]
		if end := strings.Index(rest, "\n---"); end >= 0 {
			remaining = rest[end+4:]
		}
	}
	remaining = fencedYAMLBlockPattern.ReplaceAllString(remaining, "")
	for _, line := range strings.Split(strings.ReplaceAll(remaining, "\r", ""), "\n") {
		text := strings.TrimSpace(line)
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") ||
			strings.HasPrefix(text, "```") || strings.HasPrefix(text, "~~~") ||
			text == "---" || text == "..." ||
			strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
			return true
		}
		// Markdown bullets are prose unless they actually contain a YAML key.
		candidate := strings.TrimSpace(strings.TrimPrefix(text, "- "))
		if key, _, ok := strings.Cut(candidate, ":"); ok {
			key = strings.TrimSpace(key)
			mapping := key != ""
			for _, char := range key {
				if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
					(char >= '0' && char <= '9') || char == '_' || char == '-') {
					mapping = false
					break
				}
			}
			if mapping {
				return true
			}
		}
	}
	return false
}

// Accept repo-relative paths and wiki-relative inventory paths, never outside root.
func retirementRepoPath(root, docPath, path string) (abs, rel string, ok bool) {
	path = strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	if path == "" || filepath.IsAbs(path) || strings.Contains(path, ":") {
		return "", "", false
	}
	// Only wiki-layout paths are relative to the inventory; other paths are repo-relative.
	if strings.HasPrefix(path, "_retired/") || len(path) > 2 && path[0] >= '0' && path[0] <= '9' && path[1] >= '0' && path[1] <= '9' && path[2] == '_' {
		path = filepath.ToSlash(filepath.Join(filepath.Dir(docPath), path))
	}
	abs = filepath.Join(root, filepath.FromSlash(path))
	relative, err := filepath.Rel(root, abs)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", false
	}
	return abs, filepath.ToSlash(relative), true
}
