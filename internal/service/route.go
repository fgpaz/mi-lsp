package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// resolveCanonicalRoute is the shared routing core used by nav.route, nav.ask, and nav.pack.
//
// It resolves the canonical lane using Tier 1 (governance/profile, filesystem) and optionally
// enriches with Tier 2 (indexed docs). The canonical lane is always authoritative and at least
// 2x stronger than the discovery advisory lane (RF-QRY-014, RF-QRY-015).
//
// Discovery is docs-only by default; code discovery only with includeDiscovery=true and Full mode.
func (a *App) resolveCanonicalRoute(ctx context.Context, registration model.WorkspaceRegistration, task string, opts model.QueryOptions, includeDiscovery bool) (model.RouteResult, string, []string, error) {
	query := loadDocQueryContext(ctx, registration, task)
	defer query.Close()
	return query.canonicalRoute(opts, includeDiscovery), query.profileSource, append([]string{}, query.profileWarnings...), nil
}

// buildDiscoveryAdvisory builds a non-authoritative discovery summary.
// It is always advisory-only and must never be used as a canonical answer.
func buildDiscoveryAdvisory(ranked []scoredDoc, maxDocs int) *model.RouteDiscoveryLane {
	docs := make([]model.RouteDoc, 0, maxDocs)
	for i, candidate := range ranked {
		if i >= maxDocs {
			break
		}
		docs = append(docs, model.RouteDoc{
			Path:   candidate.record.Path,
			Title:  candidate.record.Title,
			DocID:  candidate.record.DocID,
			Layer:  candidate.record.Layer,
			Family: candidate.record.Family,
			Why:    strings.Join(candidate.reason, ","),
			Stage:  "discovery",
		})
	}
	return &model.RouteDiscoveryLane{
		Source:   "indexed_docs",
		Docs:     docs,
		Advisory: "non-authoritative: discovery ranking never overrides canonical wiki",
	}
}

// routeCanonicalToPackDocs converts a RouteCanonicalLane into PackDoc slice.
// Used by pack when the docs index is empty or stale but Tier 1 can still produce a canonical route.
func routeCanonicalToPackDocs(canonical model.RouteCanonicalLane) []model.PackDoc {
	docs := make([]model.PackDoc, 0, 1+len(canonical.PreviewPack))

	if canonical.AnchorDoc.Path != "" {
		anchor := canonical.AnchorDoc
		docs = append(docs, model.PackDoc{
			Path:   anchor.Path,
			Title:  anchor.Title,
			DocID:  anchor.DocID,
			Layer:  anchor.Layer,
			Family: anchor.Family,
			Stage:  "anchor",
			Why:    []string{anchor.Why},
		})
	}

	for _, doc := range canonical.PreviewPack {
		docs = append(docs, model.PackDoc{
			Path:   doc.Path,
			Title:  doc.Title,
			DocID:  doc.DocID,
			Layer:  doc.Layer,
			Family: doc.Family,
			Stage:  doc.Stage,
			Why:    []string{doc.Why},
		})
	}

	return docs
}

// route handles the nav.route operation.
// It resolves the canonical low-token document route for a spec-driven task (RF-QRY-014).
func (a *App) route(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	if blockedEnv, err := a.governanceGateEnvelope(ctx, request, "nav.route"); err != nil {
		return model.Envelope{}, err
	} else if blockedEnv != nil {
		return *blockedEnv, nil
	}

	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}
	memory, _ := loadReentryMemory(ctx, registration.Root)

	task, _ := request.Payload["task"].(string)
	task = strings.TrimSpace(task)
	if task == "" {
		task, _ = request.Payload["question"].(string)
		task = strings.TrimSpace(task)
	}
	if task == "" {
		return model.Envelope{}, fmt.Errorf("task is required")
	}

	// Code discovery only with --full or --include-code-discovery flag
	includeCodeDiscovery, _ := request.Payload["include_code_discovery"].(bool)
	includeDiscovery := includeCodeDiscovery || request.Context.Full

	query := loadDocQueryContext(ctx, registration, task)
	defer query.Close()
	result := query.canonicalRoute(request.Context, includeDiscovery)
	warnings := append([]string{}, query.profileWarnings...)

	env := model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "route",
		Items:     []model.RouteResult{result},
		Warnings:  warnings,
		Stats:     model.Stats{Files: 1 + len(result.Canonical.PreviewPack)},
	}
	env = applyWikiRepoCompatHint(env, request, "nav.route", registration.Name, task)
	env = attachMemoryPointer(env, memory)
	env.Continuation = buildRouteContinuation(task, result, request.Context, memory)
	return applyCoachPolicy(applyAXIPreviewHints(env, request.Context, "preview mode: rerun with --full for expanded route and discovery"), request.Context), nil
}
