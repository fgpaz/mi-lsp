package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func (a *App) governance(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	registration, project, resolutionWarnings, resolutionHint, err := a.resolvePreflightWorkspaceWithProject(request)
	if err != nil {
		return model.Envelope{}, err
	}
	inspectRoot, autoSync, inspectWarnings := governanceInspectTarget(registration, project)
	status := docgraph.InspectGovernance(inspectRoot, autoSync)
	warnings := append([]string{}, resolutionWarnings...)
	warnings = append(warnings, inspectWarnings...)
	warnings = append(warnings, status.Warnings...)
	if status.Blocked {
		warnings = append(warnings, status.Issues...)
	}
	hint := governanceHint(registration.Name, status)
	if resolutionHint != "" && hint != "" {
		hint = resolutionHint + " " + hint
	} else if resolutionHint != "" {
		hint = resolutionHint
	}
	return model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "governance",
		Items:     []model.GovernanceStatus{status},
		Warnings:  warnings,
		Hint:      hint,
	}, nil
}

func governanceHint(alias string, status model.GovernanceStatus) string {
	if status.Blocked {
		return fmt.Sprintf("governance blocked; rerun mi-lsp nav governance --workspace %s --format toon after repair", alias)
	}
	if status.Sync == "auto_synced" {
		return fmt.Sprintf("read-model auto-synced; rerun mi-lsp index --workspace %s if you need fresh docgraph state", alias)
	}
	return ""
}

func governanceInspectTarget(registration model.WorkspaceRegistration, project model.ProjectFile) (string, bool, []string) {
	inspectRoot := registration.Root
	autoSync := true
	if len(project.Canons) > 0 {
		return inspectRoot, false, nil
	}
	producto := firstCanonLinkByRole(registration.CanonLinks, "producto")
	if producto == nil {
		return inspectRoot, autoSync, nil
	}
	target, err := workspace.ResolveWorkspace(producto.Alias)
	if err != nil {
		return inspectRoot, autoSync, []string{fmt.Sprintf("producto canon link %q could not be resolved: %v", producto.Alias, err)}
	}
	targetProject, loadErr := workspace.LoadProjectFile(target.Root)
	if loadErr != nil {
		return inspectRoot, autoSync, []string{fmt.Sprintf("producto canon link %q could not load project.toml: %v", producto.Alias, loadErr)}
	}
	if readOnly, err := workspace.PathIsReadOnlyCanon(target.Root, targetProject, target.Root); err == nil && readOnly {
		return target.Root, false, nil
	}
	return target.Root, false, nil
}

func firstCanonLinkByRole(links []model.WorkspaceCanonLink, role string) *model.WorkspaceCanonLink {
	role = strings.TrimSpace(role)
	for i := range links {
		if strings.EqualFold(strings.TrimSpace(links[i].Role), role) {
			link := links[i]
			return &link
		}
	}
	return nil
}

func (a *App) governanceGateEnvelope(ctx context.Context, request model.CommandRequest, operation string) (*model.Envelope, error) {
	registration, project, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return nil, err
	}
	inspectRoot, autoSync, _ := governanceInspectTarget(registration, project)
	status := docgraph.InspectGovernance(inspectRoot, autoSync)
	if !status.Blocked {
		return nil, nil
	}
	warnings := append([]string{}, status.Warnings...)
	warnings = append(warnings, status.Issues...)
	env := model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "governance",
		Items:     []model.GovernanceStatus{status},
		Warnings:  warnings,
		Hint:      fmt.Sprintf("%s is blocked by governance; only diagnosis and repair should continue", operation),
	}
	return &env, nil
}
