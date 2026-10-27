package service

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
)

const (
	wikiContractGovernancePath       = ".docs/wiki/00_governance.md"
	wikiContractGovernanceLegacyPath = ".docs/wiki/00_gobierno_documental.md"
)

// ResolveWikiContractLanguageAtRoot selects canonical governance first and only reads the
// legacy filename when the canonical source is absent. Parsing remains pure in model.
func ResolveWikiContractLanguageAtRoot(root string) model.WikiContractLanguageResult {
	if source, err := docgraph.ResolvedGovernanceDocument(root); err == nil {
		content, err := os.ReadFile(docgraph.ResolveDocumentPath(root, source))
		if err != nil {
			return wikiGovernanceReadFailure(source, err)
		}
		result := model.ResolveWikiContractLanguage(source, string(content))
		if filepath.Base(source) == "00_gobierno_documental.md" {
			result.Warnings = append(result.Warnings, model.WikiContractFinding{Code: "governance_legacy_filename", Expected: wikiContractGovernancePath, Message: "governance_legacy_filename: prefer 00_governance.md over 00_gobierno_documental.md"})
		}
		return result
	} else if os.IsNotExist(err) {
		return model.ResolveWikiContractLanguage("", "")
	} else {
		return wikiGovernanceReadFailure("", err)
	}
}

func wikiGovernanceReadFailure(source string, err error) model.WikiContractLanguageResult {
	return model.WikiContractLanguageResult{
		Source: source,
		Valid:  false,
		Warnings: []model.WikiContractFinding{{
			Code: "governance_unreadable", Source: source,
			Message:  fmt.Sprintf("governance_unreadable:%s:%v", source, err),
			Blocking: false,
		}},
	}
}
