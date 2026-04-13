package docgraph

import (
	"os"
	"path/filepath"
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
	anchorDoc := model.RouteDoc{
		Path:   anchorPath,
		Family: family,
		Why:    "canonical_anchor",
		Stage:  "anchor",
	}
	if title := readDocTitle(filepath.Join(root, filepath.FromSlash(anchorPath))); title != "" {
		anchorDoc.Title = title
	}
	// Fill layer from path prefix
	anchorDoc.Layer = detectLayer(anchorPath)

	previewPack := buildTier1PreviewPack(family, profile, root, anchorPath)

	return model.RouteCanonicalLane{
		AnchorDoc:     anchorDoc,
		PreviewPack:   previewPack,
		Family:        family,
		Authoritative: true,
	}, why
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
					Layer:  detectLayer(path),
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
	content, err := os.ReadFile(absPath)
	if err != nil {
		return ""
	}
	for _, line := range strings.SplitN(string(content), "\n", 20) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}
