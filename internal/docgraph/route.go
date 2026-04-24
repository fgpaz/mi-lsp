package docgraph

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// Tier1CanonicalRoute builds a canonical route from governance/profile alone.
// It does not require a full docs index — it derives the route skeleton from
// the read-model and the root docs that exist on the filesystem.
// This implements the fail-closed canonical semantics for RF-QRY-014.
func Tier1CanonicalRoute(question string, profile model.DocsReadProfile, root string) (model.RouteCanonicalLane, []string) {
	family := MatchFamily(question, profile)
	why := []string{"tier1=governance_profile", "family=" + family}

	anchorPath := canonicalAnchorForFamily(family, profile, root)
	explicitDocID := firstDocID(question)
	explicitAnchorID := ""
	if explicitDocID != "" {
		if explicitPath := containingDocForExplicitID(root, profile, explicitDocID); explicitPath != "" {
			anchorPath = explicitPath
			family = "functional"
			explicitAnchorID = explicitDocID
			why = append(why, "explicit_doc_id="+explicitDocID, "anchor=containing_doc")
		}
	}
	anchorDoc := model.RouteDoc{
		Path:   anchorPath,
		Family: family,
		Why:    "canonical_anchor",
		Stage:  "anchor",
	}
	if explicitAnchorID != "" {
		anchorDoc.DocID = explicitAnchorID
	}
	if title := embeddedDocTitle(filepath.Join(root, filepath.FromSlash(anchorPath)), explicitAnchorID); title != "" {
		anchorDoc.Title = title
	}
	// Fill layer from path prefix
	anchorDoc.Layer = DetectLayerForPath(profile, anchorPath)

	previewPack := buildTier1PreviewPack(family, profile, root, anchorPath)

	return model.RouteCanonicalLane{
		AnchorDoc:     anchorDoc,
		PreviewPack:   previewPack,
		Family:        family,
		Authoritative: true,
	}, why
}

func containingDocForExplicitID(root string, profile model.DocsReadProfile, docID string) string {
	if docID == "" {
		return ""
	}
	searchPaths := explicitDocSearchPaths(profile, docID)
	if len(searchPaths) == 0 {
		searchPaths = fallbackExplicitDocSearchPaths(docID)
	}
	for _, pattern := range searchPaths {
		var found string
		_ = expandPattern(context.Background(), root, pattern, func(absPath string) {
			if found != "" {
				return
			}
			content, err := os.ReadFile(absPath)
			if err != nil {
				return
			}
			if !docContainsExplicitID(string(content), docID) {
				return
			}
			rel, err := filepath.Rel(root, absPath)
			if err != nil {
				return
			}
			found = filepath.ToSlash(rel)
		})
		if found != "" {
			return found
		}
	}
	return ""
}

func explicitDocSearchPaths(profile model.DocsReadProfile, docID string) []string {
	prefix := docIDPrefix(docID)
	if prefix == "" {
		return nil
	}
	seen := map[string]struct{}{}
	paths := make([]string, 0)
	add := func(path string) {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path == "" {
			return
		}
		if _, exists := seen[path]; exists {
			return
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}

	for _, item := range profile.Governance.Hierarchy {
		if strings.TrimSpace(item.Family) != "functional" {
			continue
		}
		if !governanceItemMatchesDocPrefix(item, prefix) {
			continue
		}
		for _, path := range item.Paths {
			add(path)
		}
	}
	for _, family := range profile.Families {
		if family.Name != "functional" {
			continue
		}
		for _, path := range family.Paths {
			if pathMatchesDocPrefix(path, prefix) {
				add(path)
			}
		}
	}
	for _, path := range fallbackExplicitDocSearchPaths(docID) {
		add(path)
	}
	sort.SliceStable(paths, func(i, j int) bool {
		leftSpecific := isSpecificDocPatternForPrefix(paths[i], prefix)
		rightSpecific := isSpecificDocPatternForPrefix(paths[j], prefix)
		if leftSpecific != rightSpecific {
			return leftSpecific
		}
		return false
	})
	return paths
}

func governanceItemMatchesDocPrefix(item model.GovernanceHierarchyItem, prefix string) bool {
	layer := strings.ToUpper(strings.TrimSpace(item.Layer))
	stage := strings.ToLower(strings.TrimSpace(item.PackStage))
	id := strings.ToLower(strings.TrimSpace(item.ID))
	switch prefix {
	case "RS":
		return layer == "RS" || stage == "outcome" || id == "outcome" || id == "resultados"
	case "RF":
		return layer == "RF" || layer == "04" || stage == "requirements" || id == "requirements"
	case "TP":
		return layer == "TP" || layer == "06" || stage == "tests" || id == "tests"
	case "FL":
		return layer == "FL" || layer == "03" || stage == "flow" || id == "flow"
	default:
		return layer == prefix
	}
}

func pathMatchesDocPrefix(path string, prefix string) bool {
	normalized := filepath.ToSlash(strings.ToLower(strings.TrimSpace(path)))
	switch prefix {
	case "RS":
		return strings.Contains(normalized, "02_resultados") || strings.Contains(normalized, "/rs")
	case "RF":
		return strings.Contains(normalized, "04_rf") || strings.Contains(normalized, "/rf")
	case "TP":
		return strings.Contains(normalized, "06_pruebas") || strings.Contains(normalized, "/tp")
	case "FL":
		return strings.Contains(normalized, "03_fl") || strings.Contains(normalized, "/fl")
	default:
		return strings.Contains(normalized, strings.ToLower(prefix)+"-")
	}
}

func isSpecificDocPatternForPrefix(path string, prefix string) bool {
	normalized := filepath.ToSlash(strings.ToLower(strings.TrimSpace(path)))
	switch prefix {
	case "RS":
		return strings.Contains(normalized, "/02_resultados/")
	case "RF":
		return strings.Contains(normalized, "/04_rf/") || strings.Contains(normalized, "/rf/")
	case "TP":
		return strings.Contains(normalized, "/06_pruebas/") || strings.Contains(normalized, "/tp/")
	case "FL":
		return strings.Contains(normalized, "/03_fl/") || strings.Contains(normalized, "/fl/")
	default:
		return false
	}
}

func fallbackExplicitDocSearchPaths(docID string) []string {
	switch docIDPrefix(docID) {
	case "RS":
		return []string{".docs/wiki/02_resultados/*.md", ".docs/wiki/02_resultados_soluciones_usuario.md"}
	case "RF":
		return []string{".docs/wiki/04_RF/*.md", ".docs/wiki/04_RF.md"}
	case "TP":
		return []string{".docs/wiki/06_pruebas/*.md", ".docs/wiki/06_matriz_pruebas_RF.md"}
	case "FL":
		return []string{".docs/wiki/03_FL/*.md", ".docs/wiki/03_FL.md"}
	default:
		return nil
	}
}

func docIDPrefix(docID string) string {
	docID = strings.ToUpper(strings.TrimSpace(docID))
	if idx := strings.Index(docID, "-"); idx > 0 {
		return docID[:idx]
	}
	return ""
}

func docContainsExplicitID(content string, docID string) bool {
	for _, match := range docIDPattern.FindAllString(content, -1) {
		if strings.EqualFold(match, docID) {
			return true
		}
	}
	return false
}

// canonicalAnchorForFamily returns the best canonical anchor path for a family.
// It checks the filesystem so Tier 1 stays honest about what actually exists.
func canonicalAnchorForFamily(family string, profile model.DocsReadProfile, root string) string {
	for _, f := range profile.Families {
		if f.Name != family {
			continue
		}
		for _, path := range f.Paths {
			// Skip directory globs; only consider concrete file paths
			if strings.HasSuffix(path, "/") || strings.ContainsAny(path, "*?") {
				continue
			}
			absPath := filepath.Join(root, filepath.FromSlash(path))
			if _, err := os.Stat(absPath); err == nil {
				return path
			}
		}
	}

	// Fallback: governance doc is always the safe canonical anchor
	govPath := ".docs/wiki/00_gobierno_documental.md"
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(govPath))); err == nil {
		return govPath
	}

	// Last resort: README.md (only when no wiki exists at all)
	return "README.md"
}

// buildTier1PreviewPack builds a minimal 2-doc preview pack from the canonical
// stage order without requiring the docs index.
func buildTier1PreviewPack(family string, profile model.DocsReadProfile, root string, anchorPath string) []model.RouteDoc {
	stageOrder := profile.ReadingPack.FunctionalStageOrder
	switch family {
	case "technical":
		stageOrder = profile.ReadingPack.TechnicalStageOrder
	case "ux":
		stageOrder = profile.ReadingPack.UXStageOrder
	}

	preview := make([]model.RouteDoc, 0, 2)
	seen := map[string]struct{}{anchorPath: {}}

	for _, stage := range stageOrder {
		if len(preview) >= 2 {
			break
		}
		for _, path := range canonicalPathsForStage(stage) {
			if _, exists := seen[path]; exists {
				continue
			}
			absPath := filepath.Join(root, filepath.FromSlash(path))
			if _, err := os.Stat(absPath); err == nil {
				doc := model.RouteDoc{
					Path:   path,
					Stage:  stage,
					Layer:  DetectLayerForPath(profile, path),
					Family: family,
					Why:    "canonical_preview",
				}
				if title := readDocTitle(absPath); title != "" {
					doc.Title = title
				}
				preview = append(preview, doc)
				seen[path] = struct{}{}
				break
			}
		}
	}

	return preview
}

// canonicalPathsForStage maps a pack stage name to expected canonical doc paths.
// These are the well-known paths for spec-driven wiki workspaces.
func canonicalPathsForStage(stage string) []string {
	switch stage {
	case "governance":
		return []string{".docs/wiki/00_gobierno_documental.md"}
	case "scope":
		return []string{".docs/wiki/01_alcance_funcional.md"}
	case "outcome":
		return []string{".docs/wiki/02_resultados_soluciones_usuario.md", ".docs/wiki/02_resultados/"}
	case "architecture":
		return []string{".docs/wiki/02_arquitectura.md"}
	case "flow":
		return []string{".docs/wiki/03_FL.md"}
	case "requirements":
		return []string{".docs/wiki/04_RF.md"}
	case "data":
		return []string{".docs/wiki/05_modelo_datos.md"}
	case "tests":
		return []string{".docs/wiki/06_matriz_pruebas_RF.md"}
	case "technical_baseline":
		return []string{".docs/wiki/07_baseline_tecnica.md"}
	case "physical_data":
		return []string{".docs/wiki/08_modelo_fisico_datos.md"}
	case "contracts":
		return []string{".docs/wiki/09_contratos_tecnicos.md"}
	}
	return nil
}

// readDocTitle reads the first H1 heading from a markdown file.
// Returns empty string on any error.
func readDocTitle(absPath string) string {
	return embeddedDocTitle(absPath, "")
}

func embeddedDocTitle(absPath string, docID string) string {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(content), "\n")
	if docID != "" {
		for _, line := range lines {
			if !strings.Contains(strings.ToUpper(line), strings.ToUpper(docID)) {
				continue
			}
			if title := markdownTableTitleForDocID(line, docID); title != "" {
				return title
			}
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") {
				return strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			}
		}
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}

func markdownTableTitleForDocID(line string, docID string) string {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") {
		return ""
	}
	cells := strings.Split(trimmed, "|")
	values := make([]string, 0, len(cells))
	for _, cell := range cells {
		value := strings.TrimSpace(cell)
		if value != "" {
			values = append(values, value)
		}
	}
	for i, value := range values {
		if strings.EqualFold(value, docID) && i+1 < len(values) {
			return values[i+1]
		}
	}
	return ""
}
