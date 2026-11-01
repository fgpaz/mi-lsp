package docgraph

import (
	pathpkg "path"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/fgpaz/mi-lsp/internal/model"
)

// DecodeReadProfile binds the canonical parser/normalizer to caller-owned
// bytes, avoiding a second filesystem read and silent fallback on read errors.
func DecodeReadProfile(content []byte) (model.DocsReadProfile, []string, error) {
	profile := DefaultProfile()
	if _, err := toml.Decode(string(content), &profile); err != nil {
		return model.DocsReadProfile{}, nil, err
	}
	if profile.Version == 0 {
		profile.Version = 1
	}
	profile, warnings := normalizeDocReadProfile(profile, nil)
	return profile, warnings, nil
}

// DocumentEdgeLookupPlan uses the canonical resolver normalization and semantic
// fallback rules to enumerate metadata lookup domains. Basenames are folded
// with the same Go Unicode mapping as newDocResolutionIndex; SQLite lower()
// alone is not an exhaustive replacement. It does not resolve or claim targets.
func DocumentEdgeLookupPlan(edges []model.DocEdge) (basenames, documentIDs []string) {
	for _, edge := range edges {
		if edge.Kind == "code" {
			continue
		}
		if edge.ToDocID != "" {
			documentIDs = append(documentIDs, edge.ToDocID)
		}
		if edge.ToPath == "" {
			continue
		}
		target := ensureMarkdownDocExtension(strings.ReplaceAll(strings.TrimSpace(edge.ToPath), "\\", "/"))
		base := pathpkg.Base(target)
		if base != "" && base != "." && base != ".." {
			basenames = append(basenames, strings.ToLower(base))
		}
		if semanticDocumentRelation(edge.Kind) {
			id := strings.TrimSuffix(pathpkg.Base(normalizeDocCandidate(edge.ToPath)), ".md")
			if id != "" {
				documentIDs = append(documentIDs, id)
			}
		}
	}
	return uniqueSortedDocPaths(basenames), uniqueSortedDocPaths(documentIDs)
}
