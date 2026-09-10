package service

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

const (
	defaultWikiRootPath          = ".docs/wiki"
	defaultGovernanceDocPath     = ".docs/wiki/00_gobierno_documental.md"
	canonGovernanceFile          = "00_gobierno_documental.md"
	wikiRootResolvedDefault      = "default"
	wikiRootResolvedCanon        = "canon."
	wikiRootResolvedRegistryLink = "registry.link"
)

func (a *App) wikiRoot(_ context.Context, request model.CommandRequest) (model.Envelope, error) {
	registration, err := a.ResolveWorkspace(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}
	project, err := workspace.LoadProjectFile(registration.Root)
	if err != nil {
		return model.Envelope{}, err
	}

	items, err := wikiRootItems(registration, project, strings.TrimSpace(stringPayload(request.Payload, "role")))
	if err != nil {
		return model.Envelope{}, err
	}
	warnings := []string{}
	for _, item := range items {
		if item.GovernanceDoc == "" {
			warnings = append(warnings, fmt.Sprintf("governance source unresolved for canon %q; no document path asserted", item.ID))
		}
	}
	return model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "wiki-root",
		Items:     items,
		Warnings:  warnings,
	}, nil
}

func wikiRootItems(registration model.WorkspaceRegistration, project model.ProjectFile, role string) ([]model.WikiRootResolution, error) {
	if len(project.Canons) > 0 {
		return wikiRootItemsFromCanons(registration, project, role)
	}
	if len(registration.CanonLinks) > 0 {
		return wikiRootItemsFromCanonLinks(registration, role)
	}
	return []model.WikiRootResolution{defaultWikiRootItem(registration.Name)}, nil
}

func wikiRootItemsFromCanons(registration model.WorkspaceRegistration, project model.ProjectFile, role string) ([]model.WikiRootResolution, error) {
	resolved, err := workspace.ResolveCanonsByRole(registration.Root, project, role)
	if err != nil {
		return nil, err
	}
	items := make([]model.WikiRootResolution, 0, len(resolved))
	for _, canon := range resolved {
		declared := portableRelativePath(canon.DeclaredRoot)
		canonGovernance, _ := docgraph.ResolvedCanonGovernanceDocument(registration.Root, canon.AbsRoot)
		items = append(items, model.WikiRootResolution{
			WikiRoot:      declared,
			Role:          canon.Role,
			Workspace:     registration.Name,
			GovernanceDoc: canonGovernance,
			ResolvedFrom:  wikiRootResolvedCanon + canon.ID,
			ID:            canon.ID,
		})
	}
	sortWikiRootItems(items)
	return items, nil
}

func wikiRootItemsFromCanonLinks(registration model.WorkspaceRegistration, role string) ([]model.WikiRootResolution, error) {
	filtered := workspace.FilterCanonLinksByRole(registration.CanonLinks, role)
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no registry canon link matched role %q; available: %s; pass a declared role or omit --role", role, workspace.FormatCanonLinkAvailability(registration.CanonLinks))
	}
	registry, err := workspace.LoadRegistry()
	if err != nil {
		return nil, err
	}
	items := make([]model.WikiRootResolution, 0, len(filtered))
	for _, link := range filtered {
		canonicalAlias, target, ok := workspace.FindRegisteredWorkspace(registry, link.Alias)
		if !ok {
			return nil, fmt.Errorf("registry canon link alias %q is not registered; register the canon workspace or remove the link", strings.TrimSpace(link.Alias))
		}
		targetProject, err := workspace.LoadProjectFile(target.Root)
		if err != nil {
			return nil, err
		}
		linkItems, err := wikiRootItemsForLinkedTarget(registration, canonicalAlias, strings.TrimSpace(link.Role), target, targetProject, role)
		if err != nil {
			return nil, err
		}
		items = append(items, linkItems...)
	}
	sortWikiRootItems(items)
	return items, nil
}

func wikiRootItemsForLinkedTarget(current model.WorkspaceRegistration, targetAlias, linkRole string, target model.WorkspaceRegistration, targetProject model.ProjectFile, requestedRole string) ([]model.WikiRootResolution, error) {
	if len(targetProject.Canons) == 0 {
		item, err := registryLinkItem(current, targetAlias, linkRole, filepath.Join(target.Root, filepath.FromSlash(defaultWikiRootPath)))
		if err != nil {
			return nil, err
		}
		return []model.WikiRootResolution{item}, nil
	}
	filterRole := strings.TrimSpace(requestedRole)
	if filterRole == "" {
		filterRole = linkRole
	}
	resolved, err := workspace.ResolveCanons(target.Root, targetProject)
	if err != nil {
		return nil, err
	}
	matched := make([]workspace.ResolvedCanon, 0, len(resolved))
	for _, canon := range resolved {
		if strings.EqualFold(canon.Role, filterRole) {
			matched = append(matched, canon)
		}
	}
	if len(matched) == 0 {
		item, err := registryLinkItem(current, targetAlias, linkRole, filepath.Join(target.Root, filepath.FromSlash(defaultWikiRootPath)))
		if err != nil {
			return nil, err
		}
		return []model.WikiRootResolution{item}, nil
	}
	items := make([]model.WikiRootResolution, 0, len(matched))
	for _, canon := range matched {
		item, err := registryLinkItem(current, targetAlias, linkRole, canon.AbsRoot)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func registryLinkItem(current model.WorkspaceRegistration, targetAlias, linkRole, absWikiRoot string) (model.WikiRootResolution, error) {
	wikiRoot, err := portableRelPath(current.Root, absWikiRoot)
	if err != nil {
		return model.WikiRootResolution{}, err
	}
	governanceDoc, err := portableRelPath(current.Root, filepath.Join(absWikiRoot, canonGovernanceFile))
	if err != nil {
		return model.WikiRootResolution{}, err
	}
	return model.WikiRootResolution{
		WikiRoot:      wikiRoot,
		Role:          linkRole,
		Workspace:     current.Name,
		GovernanceDoc: governanceDoc,
		ResolvedFrom:  wikiRootResolvedRegistryLink,
		ID:            targetAlias,
	}, nil
}

func portableRelPath(fromRoot, absTarget string) (string, error) {
	fromAbs, err := filepath.Abs(fromRoot)
	if err != nil {
		return "", err
	}
	toAbs, err := filepath.Abs(absTarget)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(fromAbs, toAbs)
	if err != nil {
		return "", fmt.Errorf("cannot express linked wiki %q relative to workspace %q: %w; declare [[canon]] in project.toml instead of a registry link", absTarget, fromRoot, err)
	}
	if filepath.IsAbs(rel) || (len(rel) >= 2 && rel[1] == ':') {
		return "", fmt.Errorf("cannot express linked wiki %q relative to workspace (different volume); declare [[canon]] in project.toml instead of a registry link", absTarget)
	}
	return filepath.ToSlash(rel), nil
}

func defaultWikiRootItem(alias string) model.WikiRootResolution {
	return model.WikiRootResolution{
		WikiRoot:      defaultWikiRootPath,
		Workspace:     alias,
		GovernanceDoc: defaultGovernanceDocPath,
		ResolvedFrom:  wikiRootResolvedDefault,
	}
}

func canonGovernanceDoc(declaredRoot string) string {
	declared := strings.TrimSuffix(portableRelativePath(declaredRoot), "/")
	if declared == "" || declared == "." {
		return canonGovernanceFile
	}
	return declared + "/" + canonGovernanceFile
}

func portableRelativePath(path string) string {
	return filepath.ToSlash(strings.TrimSpace(path))
}

func sortWikiRootItems(items []model.WikiRootResolution) {
	sort.SliceStable(items, func(i, j int) bool {
		leftID := strings.ToLower(items[i].ID)
		rightID := strings.ToLower(items[j].ID)
		if leftID != rightID {
			return leftID < rightID
		}
		return strings.ToLower(items[i].WikiRoot) < strings.ToLower(items[j].WikiRoot)
	})
}
