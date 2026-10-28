package docgraph

import (
	"path/filepath"
	"strings"
)

// Wiki contract v1 kind names (WIKI-CONTRACT-v1).
const (
	WikiKindGovernance  = "governance"
	WikiKindScope       = "scope"
	WikiKindOutcome     = "outcome"
	WikiKindArchitecture = "architecture"
	WikiKindDecision    = "decision"
	WikiKindFlow        = "flow"
	WikiKindRequirement = "requirement"
	WikiKindDataModel   = "data_model"
	WikiKindTest        = "test"
	WikiKindTechnical   = "technical"
	WikiKindDatabase    = "database"
	WikiKindContract    = "contract"
	WikiKindJourney     = "journey"
	WikiKindUXModule    = "ux_module"
)

// legacyPathRules map legacy wiki path hints to v1 kind and canonical path prefix.
type legacyPathRule struct {
	kind           string
	canonicalHint  string
	match          func(norm string, base string) bool
}

var legacyPathRules = []legacyPathRule{
	{WikiKindGovernance, "00_governance.md", func(n, b string) bool {
		return b == "00_gobierno_documental.md" || b == "00_governance.md" || b == "00_registro_retiros.md" || b == "00_retirements.md"
	}},
	{WikiKindScope, "01_scope.md", func(n, b string) bool {
		return b == "01_alcance_funcional.md" || b == "01_scope.md"
	}},
	{WikiKindOutcome, "02_outcomes/", func(n, b string) bool {
		return strings.HasPrefix(b, "RS-") || strings.Contains(n, "/02_resultados/") || strings.Contains(n, "/02_rs/") || strings.HasPrefix(strings.ToLower(b), "02_resultados")
	}},
	{WikiKindArchitecture, "03_architecture.md", func(n, b string) bool {
		return b == "03_arquitectura.md" || b == "02_arquitectura.md" || b == "03_architecture.md"
	}},
	{WikiKindDecision, "03_architecture/ADR-", func(n, b string) bool {
		return strings.HasPrefix(strings.ToUpper(b), "ADR-")
	}},
	{WikiKindFlow, "04_flows/", func(n, b string) bool {
		return strings.HasPrefix(strings.ToUpper(b), "FL-") ||
			strings.Contains(n, "/03_fl/") || strings.Contains(n, "/04_fl/") ||
			strings.Contains(n, "/04_flujos/") || strings.Contains(n, "/07_fl/")
	}},
	{WikiKindRequirement, "05_requirements/", func(n, b string) bool {
		return strings.HasPrefix(strings.ToUpper(b), "RF-") ||
			strings.Contains(n, "/04_rf/") || strings.Contains(n, "/05_rf/") ||
			strings.Contains(n, "/05_requisitos/")
	}},
	{WikiKindDataModel, "05_requirements/data-model.md", func(n, b string) bool {
		return b == "06_modelo_datos.md" || b == "05_modelo_datos.md" || strings.Contains(n, "/data-model")
	}},
	{WikiKindTest, "06_tests/", func(n, b string) bool {
		return strings.HasPrefix(strings.ToUpper(b), "TP-") ||
			strings.Contains(n, "/06_pruebas/") || strings.Contains(n, "/07_pruebas/") ||
			strings.Contains(n, "matriz_pruebas_rf") || strings.Contains(n, "/tp/")
	}},
	{WikiKindTechnical, "07_technical", func(n, b string) bool {
		return strings.HasPrefix(strings.ToUpper(b), "TECH-") ||
			strings.Contains(n, "/07_tech/") || strings.Contains(n, "/08_") && strings.Contains(strings.ToLower(b), "tech") ||
			b == "07_technical.md" || strings.HasPrefix(strings.ToLower(b), "07_baseline") || strings.HasPrefix(strings.ToLower(b), "08_")
	}},
	{WikiKindDatabase, "07_technical/db/", func(n, b string) bool {
		return strings.HasPrefix(strings.ToUpper(b), "DB-") ||
			strings.Contains(n, "/08_db/") || strings.Contains(n, "/09_db/") ||
			strings.Contains(strings.ToLower(b), "modelo_fisico")
	}},
	{WikiKindContract, "07_technical/contracts/", func(n, b string) bool {
		return strings.HasPrefix(strings.ToUpper(b), "CT-") ||
			strings.Contains(n, "/09_contratos/") || strings.Contains(n, "/10_contratos")
	}},
}

var wikiKindAliases = map[string]string{
	"governance": WikiKindGovernance, "gobierno": WikiKindGovernance,
	"scope": WikiKindScope, "alcance": WikiKindScope,
	"outcome": WikiKindOutcome, "rs": WikiKindOutcome, "resultado": WikiKindOutcome, "resultados": WikiKindOutcome,
	"architecture": WikiKindArchitecture, "arch": WikiKindArchitecture, "arquitectura": WikiKindArchitecture,
	"decision": WikiKindDecision, "adr": WikiKindDecision,
	"flow": WikiKindFlow, "fl": WikiKindFlow, "flujo": WikiKindFlow, "flujos": WikiKindFlow,
	"requirement": WikiKindRequirement, "rf": WikiKindRequirement, "requisito": WikiKindRequirement, "requisitos": WikiKindRequirement,
	"data_model": WikiKindDataModel, "dm": WikiKindDataModel, "datamodel": WikiKindDataModel,
	"test": WikiKindTest, "tp": WikiKindTest, "test_plan": WikiKindTest, "test-plan": WikiKindTest, "prueba": WikiKindTest, "pruebas": WikiKindTest,
	"technical": WikiKindTechnical, "tech": WikiKindTechnical, "tecnico": WikiKindTechnical, "técnico": WikiKindTechnical,
	"database": WikiKindDatabase, "db": WikiKindDatabase,
	"contract": WikiKindContract, "ct": WikiKindContract, "contrato": WikiKindContract, "contratos": WikiKindContract,
	"journey": WikiKindJourney, "uj": WikiKindJourney,
	"ux_module": WikiKindUXModule, "module_map": WikiKindUXModule, "uxm": WikiKindUXModule,
}

// NormalizeWikiKind maps CLI or legacy kind tokens to a v1 kind name.
func NormalizeWikiKind(input string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(input))
	if key == "" {
		return "", false
	}
	if kind, ok := wikiKindAliases[key]; ok {
		return kind, true
	}
	return "", false
}

// WikiContractKindForPath classifies a relative wiki path into a v1 kind using aliases and ID prefixes.
func WikiContractKindForPath(relPath string, declaredKind string) string {
	if kind, ok := NormalizeWikiKind(declaredKind); ok {
		return kind
	}
	decl := strings.ToLower(strings.TrimSpace(declaredKind))
	switch decl {
	case "requirement", "test-plan", "test_plan", "flow", "outcome", "decision":
		if k, ok := NormalizeWikiKind(decl); ok {
			return k
		}
	}
	norm := normalizeWikiRelPath(relPath)
	base := strings.ToLower(filepath.Base(norm))
	upper := strings.ToUpper(base)
	switch {
	case strings.HasPrefix(upper, "RF-"):
		return WikiKindRequirement
	case strings.HasPrefix(upper, "TP-"):
		return WikiKindTest
	case strings.HasPrefix(upper, "FL-"):
		return WikiKindFlow
	case strings.HasPrefix(upper, "RS-"):
		return WikiKindOutcome
	case strings.HasPrefix(upper, "ADR-"):
		return WikiKindDecision
	case strings.HasPrefix(upper, "TECH-"):
		return WikiKindTechnical
	case strings.HasPrefix(upper, "DB-"):
		return WikiKindDatabase
	case strings.HasPrefix(upper, "CT-"):
		return WikiKindContract
	}
	for _, rule := range legacyPathRules {
		if rule.match(norm, strings.ToLower(base)) {
			return rule.kind
		}
	}
	return ""
}

func normalizeWikiRelPath(relPath string) string {
	return strings.ToLower(filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(relPath), "./")))
}

// LegacyPathRenameWarning suggests a v1 path when the document still uses a legacy layout.
func LegacyPathRenameWarning(relPath string) string {
	norm := normalizeWikiRelPath(relPath)
	base := strings.ToLower(filepath.Base(norm))
	origBase := filepath.Base(filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(relPath), "./")))
	for _, rule := range legacyPathRules {
		if !rule.match(norm, base) {
			continue
		}
		hint := rule.canonicalHint
		switch rule.kind {
		case WikiKindGovernance:
			if base == "00_gobierno_documental.md" {
				return "legacy_path: rename 00_gobierno_documental.md -> 00_governance.md"
			}
			if base == "00_registro_retiros.md" {
				return "legacy_path: rename 00_registro_retiros.md -> 00_retirements.md"
			}
		case WikiKindScope:
			if base == "01_alcance_funcional.md" {
				return "legacy_path: rename 01_alcance_funcional.md -> 01_scope.md"
			}
		case WikiKindDataModel:
			if base == "06_modelo_datos.md" || base == "05_modelo_datos.md" {
				return "legacy_path: rename " + origBase + " -> 05_requirements/data-model.md"
			}
		case WikiKindTest:
			if strings.Contains(norm, "matriz_pruebas") {
				return "legacy_path: hand-written matrix file is deprecated; use mi-lsp nav wiki matrix"
			}
			if strings.Contains(norm, "/06_pruebas/") || strings.Contains(norm, "/07_pruebas/") {
				return "legacy_path: move " + filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(relPath), "./")) + " -> 06_tests/ (WIKI-CONTRACT-v1)"
			}
		case WikiKindFlow:
			if strings.Contains(norm, "/03_fl/") || strings.Contains(norm, "/04_fl/") || strings.Contains(norm, "/04_flujos/") {
				return "legacy_path: move " + filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(relPath), "./")) + " -> 04_flows/ (WIKI-CONTRACT-v1)"
			}
		case WikiKindRequirement:
			if strings.Contains(norm, "/04_rf/") || strings.Contains(norm, "/05_rf/") || strings.Contains(norm, "/05_requisitos/") {
				return "legacy_path: move " + filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(relPath), "./")) + " -> 05_requirements/ (WIKI-CONTRACT-v1)"
			}
		default:
			if hint != "" && !strings.Contains(norm, strings.TrimSuffix(hint, "/")) {
				return "legacy_path: align " + norm + " with " + hint + " (WIKI-CONTRACT-v1)"
			}
		}
	}
	return ""
}

// DocMatchesWikiKind reports whether a document belongs to the requested v1 kind.
func DocMatchesWikiKind(relPath, docID, declaredKind, requestedKind string) bool {
	want, ok := NormalizeWikiKind(requestedKind)
	if !ok {
		return false
	}
	got := WikiContractKindForPath(relPath, declaredKind)
	if got == want {
		return true
	}
	if got == "" {
		got = inferKindFromDocID(docID)
	}
	return got == want
}

func inferKindFromDocID(docID string) string {
	id := strings.ToUpper(strings.TrimSpace(docID))
	if idx := strings.Index(id, "-"); idx > 0 {
		prefix := id[:idx]
		if kind, ok := NormalizeWikiKind(prefix); ok {
			return kind
		}
	}
	return ""
}
