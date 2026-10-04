package service

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func qRelativeFile(root, file string) (string, bool) {
	file = strings.ReplaceAll(file, "\\", "/")
	if file == "" {
		return "", true
	}
	if filepath.IsAbs(file) {
		if root == "" {
			return "", false
		}
		rel, err := filepath.Rel(root, file)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", false
		}
		file = rel
	}
	file = filepath.ToSlash(filepath.Clean(file))
	if file == "." || strings.HasPrefix(file, "../") {
		return "", false
	}
	return file, true
}

func validQRevision(value string) bool {
	if len(value) != model.IDRevLen {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func qItemsFromEnvelope(env model.Envelope, workspaceID, root string) []model.QItem {
	data, err := json.Marshal(env.Items)
	if err != nil {
		return nil
	}
	var rows []map[string]any
	if json.Unmarshal(data, &rows) != nil {
		return nil
	}
	counts := map[string]int{}
	for _, row := range rows {
		file := stringValue(row, "file_path", "file", "path")
		file, ok := qRelativeFile(root, file)
		if !ok {
			continue
		}
		name := stringValue(row, "qualified_name", "name", "symbol", "doc_id", "title")
		if file != "" && name != "" && stringValue(row, "doc_id") == "" && env.Backend != "text" {
			counts[file+"#"+name]++
		}
	}
	out := make([]model.QItem, 0, len(rows))
	for _, row := range rows {
		file := stringValue(row, "file_path", "file", "path")
		file, ok := qRelativeFile(root, file)
		if !ok {
			continue
		}
		docID := stringValue(row, "doc_id")
		name := stringValue(row, "qualified_name", "name", "symbol", "doc_id", "title")
		kind := stringValue(row, "kind")
		line, end := intValue(row, "line", "start_line"), intValue(row, "end_line")
		if file == "" && docID != "" {
			file = "_wiki/" + docID
		}
		if name == "" {
			name = filepath.Base(file)
		}
		if docID!=""{if title:=stringValue(row,"title");title!=""{name=title}}
		if qname := stringValue(row, "qualified_name"); qname != "" {
			name = model.SymbolQName(model.SymbolRecord{FilePath: file, QualifiedName: qname, Name: name, Parent: stringValue(row, "parent")})
		}
		origin := stringValue(row, "origin")
		if origin == "" {
			origin = env.Backend
		}
		it := model.QItem{Kind: kind, Name: name, File: file, Line: line, EndLine: end, Origin: origin, Lang: stringValue(row, "language", "lang"), Parent: stringValue(row, "parent"), Signature: stringValue(row, "signature"), Title: stringValue(row, "title"), Layer: stringValue(row, "layer"), Score: intValue(row, "score")}
		text := stringValue(row, "text", "content", "snippet", "evidence")
		it.Text = text
		switch {
		case docID != "":
			rev := strings.TrimPrefix(strings.ToLower(stringValue(row, "content_hash")), "rev:")
			if !validQRevision(rev) {
				if text != "" {
					rev = model.RevOf(text)
				} else {
					rev = ""
				}
			}
			it.ID = model.StableDocumentID(workspaceID, file, docID, rev).String()
			it.Revision = strings.TrimPrefix(rev, "rev:")
			it.Kind = "doc"
		case file != "" && line > 0 && (env.Backend == "text" || kind == "text" || origin == "text"):
			rev := fileRevision(root, file, line, max(line, end))
			it.ID = model.StableRangeID(workspaceID, file, line, max(line, end), rev).String()
			it.Revision = rev
			it.Kind = "text"
		case file != "" && line > 0:
			rev := fileRevision(root, file, line, max(line, end))
			signatureHash := strings.ToLower(stringValue(row, "signature_hash"))
			if !validQRevision(signatureHash) {
				signature := stringValue(row, "signature")
				if signature != "" {
					signatureHash = model.RevOf(signature)
				}
			}
			if counts[file+"#"+stringValue(row, "qualified_name", "name", "symbol", "doc_id", "title")] > 1 && !validQRevision(signatureHash) {
				continue
			}
			record := model.SymbolRecord{FilePath: file, Name: name, Kind: kind, QualifiedName: stringValue(row, "qualified_name"), SignatureHash: signatureHash}
			if record.QualifiedName == "" {
				record.QualifiedName = name
			}
			it.ID = model.StableSymbolID(workspaceID, record, rev, counts[file+"#"+stringValue(row, "qualified_name", "name", "symbol", "doc_id", "title")] > 1).String()
			it.Revision = rev
			if old, ok := row["stale"].(bool); ok {
				it.Stale = old
			}
		}
		if missing, ok := row["missing"].(bool); ok {
			it.Missing = missing
		}
		if stale, ok := row["stale"].(bool); ok {
			it.Stale = it.Stale || stale
		}
		out = append(out, it)
	}
	return out
}
