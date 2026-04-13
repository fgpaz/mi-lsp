package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

// resolveCanonicalRoute is the shared routing core used by nav.route, nav.ask, and nav.pack.
//
// It resolves the canonical lane using Tier 1 (governance/profile, filesystem) and optionally
// enriches with Tier 2 (indexed docs). The canonical lane is always authoritative and at least
// 2x stronger than the discovery advisory lane (RF-QRY-014, RF-QRY-015).
//
// Discovery is docs-only by default; code discovery only with includeDiscovery=true and Full mode.
func (a *App) resolveCanonicalRoute(ctx context.Context, registration model.WorkspaceRegistration, task string, opts model.QueryOptions, includeDiscovery bool) (model.RouteResult, string, []string, error) {
	profile, profileSource, profileWarnings := docgraph.LoadProfile(registration.Root)

	// Tier 1: canonical resolution from governance/profile (works without full docs index)
	canonical, tier1Why := docgraph.Tier1CanonicalRoute(task, profile, registration.Root)

	result := model.RouteResult{
		Task:      task,
		Mode:      "preview",
		Canonical: canonical,
		Why:       append([]string{fmt.Sprintf("read_model=%s", profileSource)}, tier1Why...),
	}
	if opts.Full {
		result.Mode = "full"
	}

	// Tier 2: enrichment from index (optional, index-backed)
	db, err := store.Open(registration.Root)
	if err != nil {
		// Tier 1 is sufficient when the index is unavailable
		return result, profileSource, profileWarnings, nil
	}
	defer db.Close()

	docs, err := store.ListDocRecords(ctx, db)
	if err != nil || len(docs) == 0 {
		// Docs index empty; Tier 1 canonical is still valid
		return result, profileSource, profileWarnings, nil
	}

	// Enrich canonical lane from index when available
	family := docgraph.MatchFamily(task, profile)
	_, ftsScores, _ := store.FTSSearchDocs(ctx, db, task, 20)
	ranked := rankDocs(task, family, docs, ftsScores)

	if len(ranked) > 0 {
		primary := ranked[0]
		result.Canonical.AnchorDoc = model.RouteDoc{
			Path:   primary.record.Path,
			Title:  primary.record.Title,
			DocID:  primary.record.DocID,
			Layer:  primary.record.Layer,
			Family: primary.record.Family,
			Why:    strings.Join(primary.reason, ","),
		}
		result.Canonical.Family = family
		result.Why = append(result.Why, "tier2=indexed_docs")

		// Build mini preview pack from top ranked docs (max 2)
		preview := make([]model.RouteDoc, 0, 2)
		seen := map[string]struct{}{primary.record.Path: {}}
		for _, candidate := range ranked[1:] {
			if len(preview) >= 2 {
				break
			}
			if _, exists := seen[candidate.record.Path]; exists {
				continue
			}
			seen[candidate.record.Path] = struct{}{}
			preview = append(preview, model.RouteDoc{
				Path:   candidate.record.Path,
				Title:  candidate.record.Title,
				DocID:  candidate.record.DocID,
				Layer:  candidate.record.Layer,
				Family: candidate.record.Family,
				Why:    strings.Join(candidate.reason, ","),
			})
		}
		result.Canonical.PreviewPack = preview
	}

	// Optional: build docs-only discovery advisory
	// Discovery is never authoritative and never overrides canonical (RF-QRY-014)
	if includeDiscovery && len(ranked) > 0 {
		result.Discovery = buildDiscoveryAdvisory(ranked, 3)
	}

	return result, profileSource, profileWarnings, nil
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

	result, _, warnings, err := a.resolveCanonicalRoute(ctx, registration, task, request.Context, includeDiscovery)
	if err != nil {
		return model.Envelope{}, err
	}

	env := model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "route",
		Items:     []model.RouteResult{result},
		Warnings:  warnings,
		Stats:     model.Stats{Files: 1 + len(result.Canonical.PreviewPack)},
	}
	return applyAXIPreviewHints(env, request.Context, "preview mode: rerun with --full for expanded route and discovery"), nil
}
