package docgraph

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const wikiLayersDoctorSchema = "ae-wiki-layers-doctor/v1"
const wikiSkeletonMarker = "<!-- ae-skeleton: pending -->"
const defaultWikiRoot = ".docs/wiki"

var wikiLayerDatePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

type wikiLayerSpec struct {
	id       string
	label    string
	trait    string // empty = core layer
	detect   func(basename string, parentSegments []string) bool
	rootFile string // repo-root file (has_ui)
}

var productWikiLayers = []wikiLayerSpec{
	{id: "scope", label: "Alcance", detect: func(basename string, _ []string) bool {
		return regexp.MustCompile(`(?i)alcance|scope`).MatchString(basename)
	}},
	{id: "outcome", label: "Resultados (RS)", detect: func(basename string, _ []string) bool {
		return regexp.MustCompile(`(?i)^RS-.*\.md$`).MatchString(basename)
	}},
	{id: "architecture", label: "Arquitectura y ADR", detect: func(basename string, _ []string) bool {
		return regexp.MustCompile(`(?i)arquitectura|architecture`).MatchString(basename) ||
			regexp.MustCompile(`(?i)^ADR-.*\.md$`).MatchString(basename)
	}},
	{id: "flow", label: "Flujos (FL)", detect: func(basename string, _ []string) bool {
		return regexp.MustCompile(`(?i)^FL-.*\.md$`).MatchString(basename)
	}},
	{id: "requirement", label: "Requisitos (RF)", detect: func(basename string, _ []string) bool {
		return regexp.MustCompile(`(?i)^RF-.*\.md$`).MatchString(basename)
	}},
	{id: "test_plan", label: "Plan de prueba (TP)", detect: func(basename string, _ []string) bool {
		return regexp.MustCompile(`(?i)^TP-.*\.md$`).MatchString(basename)
	}},
	{id: "technical", label: "Capa técnica", detect: func(basename string, parentSegments []string) bool {
		for _, segment := range parentSegments {
			if regexp.MustCompile(`(?i)^\d*_?tech`).MatchString(segment) {
				return true
			}
		}
		return regexp.MustCompile(`(?i)^TECH-.*\.md$`).MatchString(basename)
	}},
	{id: "data_model", label: "Modelo de datos", trait: "stores_data", detect: func(basename string, parentSegments []string) bool {
		if regexp.MustCompile(`(?i)^data-model\.md$`).MatchString(basename) {
			return true
		}
		return regexp.MustCompile(`(?i)^DM-.*\.md$`).MatchString(basename)
	}},
	{id: "database", label: "Base de datos (DB)", trait: "stores_data", detect: func(basename string, _ []string) bool {
		return regexp.MustCompile(`(?i)^DB-.*\.md$`).MatchString(basename)
	}},
	{id: "contract", label: "Contratos (CT)", trait: "exposes_api", detect: func(basename string, _ []string) bool {
		return regexp.MustCompile(`(?i)^CT-.*\.md$`).MatchString(basename)
	}},
	{id: "journey", label: "Journeys (UJ)", trait: "has_ui", detect: func(basename string, parentSegments []string) bool {
		for _, segment := range parentSegments {
			if regexp.MustCompile(`(?i)^10_?ux`).MatchString(segment) {
				return regexp.MustCompile(`(?i)^UJ-.*\.md$`).MatchString(basename)
			}
		}
		return regexp.MustCompile(`(?i)^UJ-.*\.md$`).MatchString(basename)
	}},
	{id: "ux_module", label: "Módulos UX (UXM)", trait: "has_ui", detect: func(basename string, parentSegments []string) bool {
		for _, segment := range parentSegments {
			if regexp.MustCompile(`(?i)^10_?ux`).MatchString(segment) {
				return regexp.MustCompile(`(?i)^UXM-.*\.md$`).MatchString(basename)
			}
		}
		return regexp.MustCompile(`(?i)^UXM-.*\.md$`).MatchString(basename)
	}},
	{id: "design_md", label: "DESIGN.md", trait: "has_ui", rootFile: "DESIGN.md"},
	{id: "product_md", label: "PRODUCT.md", trait: "has_ui", rootFile: "PRODUCT.md"},
}

var repoKinds = map[string]struct{}{
	"product": {},
	"kernel":  {},
	"docs":    {},
}

var legacyLayerIDs = map[string]string{
	"requirements": "requirement",
	"tests":        "test_plan",
	"data":         "technical",
}

func enrichWikiProductLayers(root string, status *model.GovernanceStatus) {
	if status == nil {
		return
	}
	attachWikiContractGovernance(root, status)
	report, warnings := wikiLayersDoctor(root)
	if report != nil {
		status.RepoKind = report.RepoKind
		status.WikiLayers = report
	}
	if len(warnings) > 0 {
		status.Warnings = append(status.Warnings, warnings...)
	}
}

func wikiLayersDoctor(root string) (*model.WikiLayersDoctor, []string) {
	policyPath := filepath.Join(root, kernelV2CanonicalRepoPolicyPath)
	content, err := os.ReadFile(policyPath)
	if err != nil {
		return nil, nil
	}
	var policy map[string]any
	if err := yaml.Unmarshal(content, &policy); err != nil {
		return nil, []string{"wiki_layers: repo-policy.yaml is not valid YAML"}
	}
	kind := validateWikiRepoKind(policy)
	policyExceptions, policyExceptionWarnings := validateWikiExceptions(policy["exceptions"])
	warnings := append([]string{}, policyExceptionWarnings...)
	govParsed, govWarnings := LoadWikiContractGovernance(root)
	warnings = append(warnings, govWarnings...)
	exceptions := govParsedLayerExceptions(govParsed)
	if len(exceptions) == 0 {
		exceptions = policyExceptions
	} else if len(policyExceptions) > 0 {
		warnings = append(warnings, wikiExceptionsLegacyLocation)
	}
	if kind == "" {
		warnings = append(warnings, "repo_kind_undeclared")
	}
	report := &model.WikiLayersDoctor{
		Schema:     wikiLayersDoctorSchema,
		RepoKind:   firstNonEmpty(kind, "undeclared"),
		WikiRoot:   resolveWikiRootFromPolicy(policy),
		Exceptions: exceptions,
	}
	if kind != "product" {
		report.LayersRequired = false
		return report, warnings
	}
	report.LayersRequired = true
	excepted := map[string]struct{}{}
	for _, item := range exceptions {
		excepted[exceptionLayerID(item)] = struct{}{}
	}
	layers, missing := detectProductWikiLayers(root, report.WikiRoot, excepted, govParsed)
	report.Layers = layers
	report.Missing = missing
	for _, layerID := range missing {
		warnings = append(warnings, fmt.Sprintf("wiki_layer_gap:%s", layerID))
	}
	return report, warnings
}

func validateWikiRepoKind(policy map[string]any) string {
	repo, ok := policy["repo"].(map[string]any)
	if !ok {
		return ""
	}
	kind, ok := repo["kind"].(string)
	if !ok {
		return ""
	}
	kind = strings.TrimSpace(kind)
	if _, valid := repoKinds[kind]; !valid {
		return ""
	}
	return kind
}

func validateWikiExceptions(raw any) ([]model.WikiLayerExceptionView, []string) {
	if raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, []string{"wiki_layers: exceptions must be a list"}
	}
	validLayerIDs := productLayerIDSet()
	out := make([]model.WikiLayerExceptionView, 0, len(items))
	var warnings []string
	for _, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			warnings = append(warnings, "wiki_layers: invalid exception entry")
			continue
		}
		capa := wikiExceptionString(record, "capa", "layer")
		motivo := wikiExceptionString(record, "motivo", "reason")
		autorizo := wikiExceptionString(record, "autorizo", "authorized_by")
		fecha := wikiExceptionString(record, "fecha", "date")
		if capa == "" {
			warnings = append(warnings, "wiki_layers: exception missing capa/layer")
			continue
		}
		if mapped, ok := legacyLayerIDs[capa]; ok {
			capa = mapped
		}
		if _, ok := validLayerIDs[capa]; !ok {
			warnings = append(warnings, fmt.Sprintf("wiki_layers: unknown exception capa %q", capa))
			continue
		}
		if motivo == "" || autorizo == "" || fecha == "" || !wikiLayerDatePattern.MatchString(fecha) {
			warnings = append(warnings, fmt.Sprintf("wiki_layers: incomplete exception for capa %q", capa))
			continue
		}
		if _, err := time.Parse("2006-01-02", fecha); err != nil {
			warnings = append(warnings, fmt.Sprintf("wiki_layers: invalid exception fecha for capa %q", capa))
			continue
		}
		out = append(out, model.WikiLayerExceptionView{
			Layer:        capa,
			Capa:         capa,
			Reason:       motivo,
			Motivo:       motivo,
			AuthorizedBy: autorizo,
			Autorizo:     autorizo,
			Date:         fecha,
			Fecha:        fecha,
		})
	}
	return out, warnings
}

func wikiExceptionString(record map[string]any, primary, alternate string) string {
	for _, key := range []string{primary, alternate} {
		if value := yamlScalarString(record[key]); value != "" {
			return value
		}
	}
	return ""
}

func yamlScalarString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case time.Time:
		return typed.Format("2006-01-02")
	default:
		return ""
	}
}

func wikiProductLayerIDSet() map[string]struct{} {
	set := map[string]struct{}{}
	for _, layer := range productWikiLayers {
		set[layer.id] = struct{}{}
	}
	return set
}

func productLayerIDSet() map[string]struct{} {
	return wikiProductLayerIDSet()
}

func govParsedLayerExceptions(parsed *WikiContractGovernance) []model.WikiLayerExceptionView {
	if parsed == nil || len(parsed.LayerExceptions) == 0 {
		return nil
	}
	return append([]model.WikiLayerExceptionView{}, parsed.LayerExceptions...)
}

func activeWikiLayersForTraits(parsed *WikiContractGovernance) []wikiLayerSpec {
	layers := make([]wikiLayerSpec, 0, len(productWikiLayers))
	for _, layer := range productWikiLayers {
		if layer.trait == "" {
			layers = append(layers, layer)
			continue
		}
		if parsed != nil && parsed.TraitEnabled(layer.trait) {
			layers = append(layers, layer)
		}
	}
	return layers
}

func resolveWikiRootFromPolicy(policy map[string]any) string {
	wiki, ok := policy["wiki"].(map[string]any)
	if !ok {
		return defaultWikiRoot
	}
	if root, ok := wiki["root"].(string); ok {
		root = strings.TrimSpace(root)
		if root != "" {
			return filepath.ToSlash(root)
		}
	}
	if layersMap, ok := wiki["layers_map"].(map[string]any); ok {
		if path, ok := layersMap["02"].(string); ok {
			path = strings.TrimSpace(path)
			if strings.HasSuffix(path, ".md") {
				return filepath.ToSlash(filepath.Dir(path))
			}
		}
	}
	return defaultWikiRoot
}

func detectProductWikiLayers(root, wikiRoot string, excepted map[string]struct{}, gov *WikiContractGovernance) ([]model.WikiLayerEntry, []string) {
	wikiRootAbs := filepath.Join(root, filepath.FromSlash(wikiRoot))
	files := listWikiMarkdownFiles(wikiRootAbs)
	layerSpecs := activeWikiLayersForTraits(gov)
	layers := make([]model.WikiLayerEntry, 0, len(layerSpecs))
	var missing []string
	for _, layer := range layerSpecs {
		var matches []string
		if layer.rootFile != "" {
			rel := layer.rootFile
			if pathExists(filepath.Join(root, rel)) {
				matches = []string{rel}
			}
		} else if layer.detect != nil {
			for _, rel := range files {
				basename := filepath.Base(rel)
				parentSegments := strings.Split(filepath.ToSlash(filepath.Dir(rel)), "/")
				if layer.detect(basename, parentSegments) {
					matches = append(matches, filepath.ToSlash(filepath.Join(wikiRoot, rel)))
				}
			}
		}
		sort.Strings(matches)
		status := "missing"
		if _, ok := excepted[layer.id]; ok {
			status = "excepted"
		} else if hasRealWikiLayerContent(root, matches) {
			status = "present"
		} else if len(matches) > 0 {
			status = "skeleton"
		}
		layers = append(layers, model.WikiLayerEntry{
			ID:     layer.id,
			Label:  layer.label,
			Status: status,
			Paths:  matches,
		})
		if status == "missing" || status == "skeleton" {
			missing = append(missing, layer.id)
		}
	}
	return layers, missing
}

func listWikiMarkdownFiles(wikiRootAbs string) []string {
	if _, err := os.Stat(wikiRootAbs); err != nil {
		return nil
	}
	var files []string
	var walk func(dir string, prefix string)
	walk = func(dir string, prefix string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			name := entry.Name()
			if name == "_mi-lsp" || strings.HasPrefix(name, ".") {
				continue
			}
			rel := name
			if prefix != "" {
				rel = prefix + "/" + name
			}
			full := filepath.Join(dir, name)
			if entry.IsDir() {
				walk(full, rel)
				continue
			}
			if strings.HasSuffix(strings.ToLower(name), ".md") {
				files = append(files, rel)
			}
		}
	}
	walk(wikiRootAbs, "")
	sort.Strings(files)
	return files
}

func hasRealWikiLayerContent(repoRoot string, matches []string) bool {
	for _, match := range matches {
		abs := filepath.Join(repoRoot, filepath.FromSlash(match))
		content, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(string(content)), wikiSkeletonMarker) {
			return true
		}
	}
	return false
}
