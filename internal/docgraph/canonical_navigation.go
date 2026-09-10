package docgraph

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/wikisource"
)

// CanonicalWikiRoots returns workspace-relative roots without introducing a
// new authority or discovering arbitrary sibling directories.
func CanonicalWikiRoots(root string) ([]string, error) {
	canons, err := loadProjectCanons(root)
	if err != nil {
		return nil, err
	}
	roots := []string{}
	if info, err := os.Lstat(filepath.Join(root, ".docs", "wiki")); err == nil && info.IsDir() {
		roots = append(roots, ".docs/wiki")
	}
	for _, canon := range canons {
		rel, err := filepath.Rel(root, canon.AbsRoot)
		if err != nil {
			return nil, err
		}
		roots = append(roots, filepath.ToSlash(rel))
	}
	return roots, nil
}

// DeclaresDocumentID distinguishes a declared owner from legacy index rows
// whose ID may have been inferred from a body mention. Legacy aggregates keep
// their existing route precedence when there is no declared owner.
func DeclaresDocumentID(root, path, id string) bool {
	roots, err := CanonicalWikiRoots(root)
	if err != nil || !safeRouteDocument(root, path, roots) {
		return false
	}
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		return false
	}
	owner, declared := wikisource.DocumentIdentity(string(content))
	return declared && owner != "" && strings.EqualFold(owner, id)
}

// routeProfilePath rebases a canon-owned read model, not a workspace-owned
// projection. Already workspace-qualified paths retain their meaning.
func routeProfilePath(root, path string) string {
	return declaredProfilePath(root, DiscoverProfilePath(root), path)
}

func declaredProfilePath(root, profilePath, path string) string {
	if path == "" || isAbsoluteDeclaredPath(path) {
		return path
	}
	if filepath.Clean(profilePath) == filepath.Clean(ProfilePath(root)) {
		return path
	}
	base := filepath.Dir(filepath.Dir(profilePath))
	relBase, err := filepath.Rel(root, base)
	if err != nil {
		return path
	}
	basePath := filepath.ToSlash(relBase)
	normalized := filepath.ToSlash(path)
	if normalized == basePath || strings.HasPrefix(normalized, basePath+"/") || strings.HasPrefix(normalized, ".docs/wiki/") {
		return normalized
	}
	rel, err := filepath.Rel(root, filepath.Join(base, filepath.FromSlash(path)))
	if err != nil {
		return path
	}
	rebased := filepath.ToSlash(rel)
	if strings.HasSuffix(normalized, "/") {
		rebased += "/"
	}
	return rebased
}

// RouteReadProfile resolves canon-relative authority paths consistently for
// Tier1 routing and service-level canonical/support selection.
func RouteReadProfile(root string, profile model.DocsReadProfile) model.DocsReadProfile {
	profile.Governance.SourceDoc = routeProfilePath(root, profile.Governance.SourceDoc)
	profile.Families = append([]model.DocsReadFamily(nil), profile.Families...)
	for i := range profile.Families {
		paths := append([]string(nil), profile.Families[i].Paths...)
		for j := range paths {
			paths[j] = routeProfilePath(root, paths[j])
		}
		profile.Families[i].Paths = paths
	}
	profile.Governance.Hierarchy = append([]model.GovernanceHierarchyItem(nil), profile.Governance.Hierarchy...)
	for i := range profile.Governance.Hierarchy {
		paths := append([]string(nil), profile.Governance.Hierarchy[i].Paths...)
		for j := range paths {
			paths[j] = routeProfilePath(root, paths[j])
		}
		profile.Governance.Hierarchy[i].Paths = paths
	}
	return profile
}

// ResolvedGovernanceDocument reports an existing validated source, never a
// guessed filename. It is read-only and does not regenerate a projection.
func ResolvedGovernanceDocument(root string) (string, error) {
	abs, display, profile, source := resolveGovernanceDoc(root)
	if source == "project" && strings.TrimSpace(profile.Governance.SourceDoc) != "" {
		canons, err := loadProjectCanons(root)
		if err != nil {
			return "", err
		}
		var ok bool
		display, ok = safeGovernanceSourceDoc(root, routeProfilePath(root, profile.Governance.SourceDoc), canons)
		if !ok {
			return "", fmt.Errorf("invalid governance source_doc")
		}
		abs = absFromDeclared(root, display)
	}
	if abs == "" || strings.HasPrefix(display, "INVALID:") {
		return "", fmt.Errorf("governance source is invalid or ambiguous")
	}
	roots, err := CanonicalWikiRoots(root)
	if err != nil || !safeRouteDocument(root, display, roots) {
		return "", fmt.Errorf("governance source is unavailable or unsafe")
	}
	return display, nil
}

// ResolvedCanonGovernanceDocument keeps each root's binding separate. The
// workspace projection retains precedence for its own source; another canon
// may have its own read model without inheriting that source by accident.
func ResolvedCanonGovernanceDocument(root, canonRoot string) (string, error) {
	resolved, resolveErr := ResolvedGovernanceDocument(root)
	canons, err := loadProjectCanons(root)
	if err != nil {
		return "", err
	}
	profile, source, _ := LoadProfile(root)
	if source == "project" && strings.TrimSpace(profile.Governance.SourceDoc) != "" {
		declared, ok := safeGovernanceSourceDoc(root, routeProfilePath(root, profile.Governance.SourceDoc), canons)
		if !ok {
			return "", fmt.Errorf("invalid governance source_doc")
		}
		if pathHasDirPrefix(canonRoot, absFromDeclared(root, declared)) && resolveErr != nil {
			return "", resolveErr
		}
	}
	if resolveErr == nil && pathHasDirPrefix(canonRoot, absFromDeclared(root, resolved)) {
		return resolved, nil
	}
	roots, err := CanonicalWikiRoots(root)
	if err != nil {
		return "", err
	}
	profilePath := filepath.Join(canonRoot, "_mi-lsp", "read-model.toml")
	profileRel, err := filepath.Rel(root, profilePath)
	if err != nil {
		return "", err
	}
	candidate, err := filepath.Rel(root, filepath.Join(canonRoot, canonGovernanceFileName))
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(profilePath); err == nil {
		if !safeRouteDocument(root, filepath.ToSlash(profileRel), roots) {
			return "", fmt.Errorf("canon read model is unsafe")
		}
		var profile model.DocsReadProfile
		if _, err := toml.DecodeFile(profilePath, &profile); err != nil {
			return "", fmt.Errorf("canon read model is invalid")
		}
		if strings.TrimSpace(profile.Governance.SourceDoc) != "" {
			candidate = declaredProfilePath(root, profilePath, profile.Governance.SourceDoc)
		}
	}
	display, ok := safeGovernanceSourceDoc(root, filepath.ToSlash(candidate), canons)
	if !ok || !safeRouteDocument(root, display, roots) {
		return "", fmt.Errorf("canon governance source is unavailable or unsafe")
	}
	return display, nil
}

// Check each component beneath an authorized root; a symlink must not turn a
// declared read-only document path into access to another authority.
func safeRouteDocument(root, relative string, canonRoots []string) bool {
	if isAbsoluteDeclaredPath(relative) || strings.TrimSpace(relative) == "" {
		return false
	}
	target := filepath.Join(root, filepath.FromSlash(relative))
	bases := []string{root}
	for _, canonRoot := range canonRoots {
		bases = append(bases, filepath.Join(root, filepath.FromSlash(canonRoot)))
	}
	for _, base := range bases {
		if !pathHasDirPrefix(base, target) {
			continue
		}
		rel, err := filepath.Rel(base, target)
		if err != nil || rel == "." {
			continue
		}
		current := base
		safe := true
		for _, component := range strings.Split(rel, string(filepath.Separator)) {
			current = filepath.Join(current, component)
			info, err := os.Lstat(current)
			if err != nil || info.Mode()&os.ModeSymlink != 0 {
				safe = false
				break
			}
		}
		if safe {
			info, err := os.Stat(target)
			return err == nil && info.Mode().IsRegular()
		}
		return false
	}
	return false
}
