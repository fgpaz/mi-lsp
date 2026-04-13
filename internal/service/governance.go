package service

import (
	"context"
	"fmt"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
)

func (a *App) governance(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}
	status := docgraph.InspectGovernance(registration.Root, true)
	warnings := append([]string{}, status.Warnings...)
	if status.Blocked {
		warnings = append(warnings, status.Issues...)
	}
	return model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "governance",
		Items:     []model.GovernanceStatus{status},
		Warnings:  warnings,
		Hint:      governanceHint(registration.Name, status),
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

func (a *App) governanceGateEnvelope(ctx context.Context, request model.CommandRequest, operation string) (*model.Envelope, error) {
	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return nil, err
	}
	status := docgraph.InspectGovernance(registration.Root, true)
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
