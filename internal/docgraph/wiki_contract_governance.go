package docgraph

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const (
	WikiContractProtocolV1           = "WIKI-CONTRACT-v1"
	wikiContractProtocolV1           = WikiContractProtocolV1
	canonGovernanceV1RelPath         = ".docs/wiki/00_governance.md"
	legacyGovernanceRelPath          = ".docs/wiki/00_gobierno_documental.md"
	canonRetirementsRelPath          = ".docs/wiki/00_retirements.md"
	legacyRetirementsRelPath         = ".docs/wiki/00_registro_retiros.md"
	legacyGovernanceFilenameWarning  = "governance_legacy_filename: prefer 00_governance.md over 00_gobierno_documental.md"
	legacyRetirementsFilenameWarning = "retirements_legacy_filename: prefer 00_retirements.md over 00_registro_retiros.md"
	wikiExceptionsLegacyLocation     = "wiki_exceptions_legacy_location: move exceptions to 00_governance.md layer_exceptions"
)

var wikiContractTraitNames = map[string]struct{}{
	"has_ui":       {},
	"stores_data":  {},
	"exposes_api":  {},
	"sold_product": {},
}

// WikiContractGovernance holds parsed WIKI-CONTRACT-v1 machine YAML from governance.
type WikiContractGovernance struct {
	WikiContract    string
	ModelRevision   string
	Version         int
	AECanon         model.GovernanceAECanon
	AECanonPresent  bool
	Traits          map[string]model.WikiContractTraitView
	LayerExceptions []model.WikiLayerExceptionView
}

func (g *WikiContractGovernance) TraitEnabled(name string) bool {
	if g == nil {
		return false
	}
	_, ok := g.Traits[name]
	return ok
}

func (g *WikiContractGovernance) AggregatedUIRoots() []string {
	if g == nil {
		return nil
	}
	trait, ok := g.Traits["has_ui"]
	if !ok {
		return nil
	}
	return append([]string{}, trait.UIRoots...)
}

func discoverGovernanceRelPath(root string) (rel string, warnings []string) {
	v1 := filepath.Join(root, filepath.FromSlash(canonGovernanceV1RelPath))
	legacy := filepath.Join(root, filepath.FromSlash(legacyGovernanceRelPath))
	if pathExists(v1) {
		return canonGovernanceV1RelPath, nil
	}
	if pathExists(legacy) {
		return legacyGovernanceRelPath, []string{legacyGovernanceFilenameWarning}
	}
	return "", nil
}

func discoverRetirementsRelPath(root string) (rel string, warnings []string) {
	v1 := filepath.Join(root, filepath.FromSlash(canonRetirementsRelPath))
	legacy := filepath.Join(root, filepath.FromSlash(legacyRetirementsRelPath))
	if pathExists(v1) {
		return canonRetirementsRelPath, nil
	}
	if pathExists(legacy) {
		return legacyRetirementsRelPath, []string{legacyRetirementsFilenameWarning}
	}
	return "", nil
}

func LoadWikiContractGovernance(root string) (*WikiContractGovernance, []string) {
	setupSource := ResolveSetupWikiSource(root)
	if setupSource.Configured {
		if setupSource.State != "resolved" {
			reason := firstNonEmptySetupReason(setupSource.ReasonCode, "setup_wiki_source_unavailable")
			return nil, []string{"setup_wiki_source_pending:" + reason}
		}
		path, _, sourceKind, reason := setupWikiGovernanceDocument(root, setupSource)
		if reason != "" {
			return nil, []string{reason}
		}
		warnings := []string{}
		if sourceKind == "setup_legacy" {
			warnings = append(warnings, legacyGovernanceFilenameWarning)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, warnings
		}
		parsed, parseWarnings := parseWikiContractGovernanceDocument(content)
		return parsed, append(warnings, parseWarnings...)
	}
	rel, err := ResolvedGovernanceDocument(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []string{"wiki_contract_governance: " + err.Error()}
	}
	var pathWarnings []string
	if filepath.Base(rel) == "00_gobierno_documental.md" {
		pathWarnings = append(pathWarnings, legacyGovernanceFilenameWarning)
	}
	docPath := ResolveDocumentPath(root, rel)
	content, err := os.ReadFile(docPath)
	if err != nil {
		return nil, pathWarnings
	}
	parsed, parseWarnings := parseWikiContractGovernanceDocument(content)
	warnings := append(pathWarnings, parseWarnings...)
	if parsed == nil {
		return nil, warnings
	}
	return parsed, warnings
}

func ParseWikiContractGovernanceYAML(block string) (*WikiContractGovernance, []string) {
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(block), &raw); err != nil {
		return nil, []string{fmt.Sprintf("wiki_contract_governance: invalid YAML: %v", err)}
	}
	protocol := strings.TrimSpace(stringMapString(raw, "wiki_contract"))
	if protocol == "" || !strings.EqualFold(protocol, wikiContractProtocolV1) {
		return nil, nil
	}
	version := 1
	switch v := raw["version"].(type) {
	case int:
		if v > 0 {
			version = v
		}
	case int64:
		if v > 0 {
			version = int(v)
		}
	case float64:
		if v > 0 {
			version = int(v)
		}
	}
	modelRevision := strings.TrimSpace(stringMapString(raw, "model_revision"))
	if modelRevision == "" {
		modelRevision = "1.1"
	}
	aeCanon, aeCanonPresent, aeCanonWarnings := parseWikiContractAECanon(raw)
	traits, traitWarnings := parseWikiContractTraits(raw["traits"])
	exceptions, exceptionWarnings := parseWikiContractLayerExceptions(raw["layer_exceptions"])
	warnings := append(traitWarnings, exceptionWarnings...)
	warnings = append(warnings, aeCanonWarnings...)
	return &WikiContractGovernance{
		WikiContract:    wikiContractProtocolV1,
		ModelRevision:   modelRevision,
		Version:         version,
		AECanon:         aeCanon,
		AECanonPresent:  aeCanonPresent,
		Traits:          traits,
		LayerExceptions: exceptions,
	}, warnings
}

func parseWikiContractAECanon(raw map[string]any) (model.GovernanceAECanon, bool, []string) {
	value, present := raw["ae_canon"]
	if !present {
		return model.GovernanceAECanon{}, false, nil
	}
	mapping, ok := value.(map[string]any)
	if !ok || mapping == nil {
		return model.GovernanceAECanon{}, true, []string{"wiki_contract_governance: ae_canon must be a mapping"}
	}
	encoded, err := yaml.Marshal(mapping)
	if err != nil {
		return model.GovernanceAECanon{}, true, []string{"wiki_contract_governance: ae_canon could not be parsed"}
	}
	var config model.GovernanceAECanon
	if err := yaml.Unmarshal(encoded, &config); err != nil {
		return config, true, []string{"wiki_contract_governance: ae_canon has invalid field types"}
	}
	if strings.TrimSpace(config.Mode) == "" {
		return config, true, []string{"wiki_contract_governance: ae_canon.mode is required when ae_canon is declared"}
	}
	return config, true, nil
}

func parseWikiContractTraits(raw any) (map[string]model.WikiContractTraitView, []string) {
	if raw == nil {
		return nil, nil
	}
	mapping, ok := raw.(map[string]any)
	if !ok {
		return nil, []string{"wiki_contract_governance: traits must be a mapping"}
	}
	out := make(map[string]model.WikiContractTraitView, len(mapping))
	var warnings []string
	for name, value := range mapping {
		name = strings.TrimSpace(name)
		if _, known := wikiContractTraitNames[name]; !known {
			warnings = append(warnings, fmt.Sprintf("wiki_contract_governance: unknown trait %q", name))
			continue
		}
		view := model.WikiContractTraitView{Name: name}
		switch typed := value.(type) {
		case string:
			view.Reason = strings.TrimSpace(typed)
		case map[string]any:
			view.Reason = strings.TrimSpace(stringMapString(typed, "reason"))
			for _, root := range stringSliceAny(typed["ui_roots"]) {
				if root != "" {
					view.UIRoots = append(view.UIRoots, root)
				}
			}
		default:
			warnings = append(warnings, fmt.Sprintf("wiki_contract_governance: trait %q has invalid shape", name))
			continue
		}
		if view.Reason == "" {
			warnings = append(warnings, fmt.Sprintf("wiki_contract_governance: trait %q missing reason", name))
		}
		if len(view.UIRoots) > 0 && name != "has_ui" {
			warnings = append(warnings, fmt.Sprintf("wiki_contract_governance: ui_roots only allowed on has_ui (trait %q)", name))
			view.UIRoots = nil
		}
		out[name] = view
	}
	return out, warnings
}

func parseWikiContractLayerExceptions(raw any) ([]model.WikiLayerExceptionView, []string) {
	if raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, []string{"wiki_contract_governance: layer_exceptions must be a list"}
	}
	validLayerIDs := wikiProductLayerIDSet()
	// Contract kinds also name exception layers; legacy doctor IDs remain valid.
	for _, kind := range []string{"governance", "decision", "test", "business", "ui"} {
		validLayerIDs[kind] = struct{}{}
	}
	out := make([]model.WikiLayerExceptionView, 0, len(items))
	var warnings []string
	for _, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			warnings = append(warnings, "wiki_contract_governance: invalid layer_exceptions entry")
			continue
		}
		layer := wikiExceptionString(record, "layer", "capa")
		reason := wikiExceptionString(record, "reason", "motivo")
		authorized := wikiExceptionString(record, "authorized_by", "autorizo")
		date := wikiExceptionString(record, "date", "fecha")
		if layer == "" {
			warnings = append(warnings, "wiki_contract_governance: layer_exceptions entry missing layer")
			continue
		}
		if mapped, ok := legacyLayerIDs[layer]; ok {
			layer = mapped
		}
		if _, ok := validLayerIDs[layer]; !ok {
			warnings = append(warnings, fmt.Sprintf("wiki_contract_governance: unknown layer_exceptions layer %q", layer))
			continue
		}
		if reason == "" || authorized == "" || date == "" || !wikiLayerDatePattern.MatchString(date) {
			warnings = append(warnings, fmt.Sprintf("wiki_contract_governance: incomplete layer_exceptions for layer %q", layer))
			continue
		}
		out = append(out, model.WikiLayerExceptionView{
			Layer:        layer,
			Capa:         layer,
			Reason:       reason,
			Motivo:       reason,
			AuthorizedBy: authorized,
			Autorizo:     authorized,
			Date:         date,
			Fecha:        date,
		})
	}
	return out, warnings
}

func attachWikiContractGovernance(root string, status *model.GovernanceStatus) {
	if status == nil {
		return
	}
	setupSource := ResolveSetupWikiSource(root)
	if !setupSource.Configured {
		if rel, warnings := discoverGovernanceRelPath(root); rel == legacyGovernanceRelPath {
			status.Warnings = append(status.Warnings, warnings...)
		}
		if _, warnings := discoverRetirementsRelPath(root); len(warnings) > 0 {
			status.Warnings = append(status.Warnings, warnings...)
		}
	}
	parsed, warnings := LoadWikiContractGovernance(root)
	if len(warnings) > 0 {
		status.Warnings = append(status.Warnings, warnings...)
	}
	if parsed == nil {
		return
	}
	status.WikiContract = parsed.WikiContract
	traits := make([]model.WikiContractTraitView, 0, len(parsed.Traits))
	for _, name := range []string{"has_ui", "stores_data", "exposes_api", "sold_product"} {
		if trait, ok := parsed.Traits[name]; ok {
			traits = append(traits, trait)
		}
	}
	for name, trait := range parsed.Traits {
		found := false
		for _, existing := range traits {
			if existing.Name == name {
				found = true
				break
			}
		}
		if !found {
			traits = append(traits, trait)
		}
	}
	status.Traits = traits
	status.UIRoots = parsed.AggregatedUIRoots()
}

func stringMapString(record map[string]any, key string) string {
	if record == nil {
		return ""
	}
	value, ok := record[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func stringSliceAny(raw any) []string {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			out = append(out, strings.TrimSpace(text))
		}
	}
	return out
}

func exceptionLayerID(item model.WikiLayerExceptionView) string {
	if layer := strings.TrimSpace(item.Layer); layer != "" {
		return layer
	}
	return strings.TrimSpace(item.Capa)
}
