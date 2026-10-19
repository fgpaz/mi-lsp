package service

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const (
	wikiContractGovernancePath       = ".docs/wiki/00_governance.md"
	wikiContractGovernanceLegacyPath = ".docs/wiki/00_gobierno_documental.md"
)

// ResolveWikiContractLanguageAtRoot selects canonical governance first and only reads the
// legacy filename when the canonical source is absent. Parsing remains pure in model.
func ResolveWikiContractLanguageAtRoot(root string) model.WikiContractLanguageResult {
	canonical := filepath.Join(root, filepath.FromSlash(wikiContractGovernancePath))
	content, err := os.ReadFile(canonical)
	if err == nil {
		return model.ResolveWikiContractLanguage(wikiContractGovernancePath, string(content))
	}
	if !os.IsNotExist(err) {
		return wikiGovernanceReadFailure(wikiContractGovernancePath, err)
	}

	legacy := filepath.Join(root, filepath.FromSlash(wikiContractGovernanceLegacyPath))
	content, err = os.ReadFile(legacy)
	if err == nil {
		result := model.ResolveWikiContractLanguage(wikiContractGovernanceLegacyPath, string(content))
		result.Warnings = append(result.Warnings, model.WikiContractFinding{
			Code: "governance_legacy_filename", Expected: wikiContractGovernancePath,
			Message:  "governance_legacy_filename: prefer 00_governance.md over 00_gobierno_documental.md",
			Blocking: false,
		})
		return result
	}
	if !os.IsNotExist(err) {
		return wikiGovernanceReadFailure(wikiContractGovernanceLegacyPath, err)
	}
	return model.ResolveWikiContractLanguage("", "")
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
