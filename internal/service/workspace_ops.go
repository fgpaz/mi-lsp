package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func (a *App) workspaceAdd(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	return a.registerWorkspace(ctx, request, "registry")
}

func (a *App) workspaceInit(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	return a.registerWorkspace(ctx, request, "init")
}

func (a *App) registerWorkspace(ctx context.Context, request model.CommandRequest, backend string) (model.Envelope, error) {
	path, _ := request.Payload["path"].(string)
	alias, _ := request.Payload["alias"].(string)
	if path == "" {
		path = "."
	}

	// Canonicalize path: Clean + Abs + check for duplicate final segment
	absPath, err := filepath.Abs(path)
	if err != nil {
		return model.Envelope{}, fmt.Errorf("invalid path %q: %w", path, err)
	}
	absPath = filepath.Clean(absPath)

	// Detect duplicate final segment (e.g., /foo/bar/bar)
	base := filepath.Base(absPath)
	parent := filepath.Dir(absPath)
	parentBase := filepath.Base(parent)
	if base != "" && parentBase != "" && strings.EqualFold(base, parentBase) {
		return model.Envelope{}, fmt.Errorf("path canonicalization error: workspace path ends with duplicate segment %q; did you mean %q?", absPath, parent)
	}

	registration, project, err := workspace.DetectWorkspaceLayout(absPath, alias)
	if err != nil {
		return model.Envelope{}, err
	}
	if alias == "" {
		alias = registration.Name
	}
	registration.Name = alias
	project.Project.Name = alias
	registration = workspace.ApplyProjectTopology(registration, project)
	if _, err := workspace.RegisterWorkspace(alias, registration); err != nil {
		return model.Envelope{}, err
	}
	if err := workspace.SaveProjectFile(registration.Root, project); err != nil {
		return model.Envelope{}, err
	}

	item := workspaceSummaryItem(registration, project)
	warnings := []string{}
	noIndex, _ := request.Payload["no_index"].(bool)
	// Default is a synchronous, bounded auto-index so the init-then-query contract
	// holds (callers can query immediately after add/init). Opt into async with
	// background=true (--background) for very large workspaces. `wait=true` is still
	// honored (forces sync) for backward compatibility; sync is now the default.
	background, _ := request.Payload["background"].(bool)
	forceWait, _ := request.Payload["wait"].(bool) // force full sync, no smart-sync degrade

	if !noIndex {
		// Check if index already exists to use incremental mode
		var mode indexer.IndexMode = indexer.IndexModeFull
		indexPath := filepath.Join(registration.Root, ".mi-lsp", "index.db")
		if _, err := os.Stat(indexPath); err == nil {
			// Index exists and we have git; use incremental
			gitDir := filepath.Join(registration.Root, ".git")
			if _, err := os.Stat(gitDir); err == nil {
				mode = indexer.IndexModeIncremental
			}
		}

		if background {
			// Async index: spawn background job and return immediately. Explicit
			// --background escape for very large workspaces.
			jobID, err := indexer.StartBackgroundIndex(ctx, registration.Root, false, mode)
			if err != nil {
				warnings = append(warnings, "auto-index failed to start: "+err.Error())
			} else {
				item["index_job_id"] = jobID
				item["index_status"] = "background"
			}
		} else {
			// Hybrid smart-sync (FD1): index synchronously within a short window so the
			// init-then-query contract holds for small/incremental repos; if a very large
			// first index exceeds the window (and --wait was not forced), degrade to a
			// background job and return job_id instead of blocking. wait=true forces the
			// full IndexTimeout (no degrade). AUD-01 (no indefinite hang) + D6 (async-first
			// for large repos) reconciled.
			syncTimeout := indexer.SmartSyncTimeout()
			if forceWait {
				syncTimeout = indexer.IndexTimeout()
			}
			ic, cancel := context.WithTimeout(ctx, syncTimeout)
			var indexResult indexer.Result
			// Bound lock acquisition by the sync window too: if another indexer holds the
			// lock, degrade to background instead of burning the window waiting (rather than
			// the unbounded blocking lock). The background job re-acquires the lock itself.
			indexErr := store.AcquireWithTimeout(registration.Root, "workspace.auto-index", syncTimeout, func() error {
				var err error
				indexResult, err = indexer.IndexWorkspace(ic, registration.Root, false)
				return err
			})
			// Compute timed-out BEFORE cancel() (cancel would mask ic.Err with Canceled).
			// Degrade on a deadline (slow index) or a lock-contention timeout, regardless of
			// how the underlying error is wrapped.
			var lockErr *store.IndexLockError
			timedOut := errors.Is(indexErr, context.DeadlineExceeded) || ic.Err() == context.DeadlineExceeded || errors.As(indexErr, &lockErr)
			cancel()
			switch {
			case indexErr == nil:
				item["index_symbols"] = indexResult.Stats.Symbols
				item["index_files"] = indexResult.Stats.Files
				item["index_ms"] = indexResult.Stats.Ms
			case timedOut && !forceWait:
				// Sync window elapsed: finish indexing in the background.
				if jobID, err := indexer.StartBackgroundIndex(ctx, registration.Root, false, mode); err == nil {
					item["index_job_id"] = jobID
					item["index_status"] = "background"
					warnings = append(warnings, "auto-index exceeded the sync window; continuing in background (use --wait to block, or query index status)")
				} else {
					warnings = append(warnings, "auto-index failed to start in background: "+err.Error())
				}
			default:
				warnings = append(warnings, "auto-index failed: "+indexErr.Error())
			}
		}
	}

	if backend == "init" {
		if isAXIMode(request.Context) {
			item["view"] = "preview"
			item["next_steps"] = buildWorkspaceAXINextSteps(alias)
		} else {
			item["next_steps"] = []string{
				"mi-lsp nav governance --workspace " + alias + " --format compact",
				"mi-lsp nav ask \"how is this workspace organized?\" --workspace " + alias + " --format compact",
				"mi-lsp workspace status " + alias + " --format compact",
			}
		}
	}
	return model.Envelope{Ok: true, Workspace: alias, Backend: backend, Items: []map[string]any{item}, Warnings: warnings}, nil
}

func (a *App) workspaceScan() (model.Envelope, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return model.Envelope{}, err
	}
	workspaces, err := workspace.ScanCandidates(cwd)
	if err != nil {
		return model.Envelope{}, err
	}
	items := make([]map[string]any, 0, len(workspaces))
	for _, registration := range workspaces {
		project, loadErr := workspace.LoadProjectTopology(registration.Root, registration)
		if loadErr != nil {
			items = append(items, map[string]any{"name": registration.Name, "root": registration.Root, "kind": registration.Kind, "languages": registration.Languages})
			continue
		}
		registration = workspace.ApplyProjectTopology(registration, project)
		items = append(items, workspaceSummaryItem(registration, project))
	}
	return model.Envelope{Ok: true, Backend: "scanner", Items: items}, nil
}

func (a *App) workspaceList(request model.CommandRequest) (model.Envelope, error) {
	groupByRoot, _ := request.Payload["group_by_root"].(bool)
	if groupByRoot {
		groups, err := workspace.GroupWorkspacesByRoot()
		if err != nil {
			return model.Envelope{}, err
		}
		items := make([]map[string]any, 0, len(groups))
		for _, group := range groups {
			items = append(items, map[string]any{
				"root":             group.Root,
				"alias_count":      group.AliasCount,
				"aliases":          group.Aliases,
				"canonical_alias":  group.CanonicalAlias,
				"selection_reason": group.SelectionReason,
				"kind":             group.Kind,
				"warnings":         group.Warnings,
			})
		}
		return model.Envelope{Ok: true, Backend: "registry", Items: items}, nil
	}
	workspaces, err := workspace.ListWorkspaces()
	if err != nil {
		return model.Envelope{}, err
	}
	items := make([]map[string]any, 0, len(workspaces))
	for _, registration := range workspaces {
		project, loadErr := workspace.LoadProjectSummary(registration.Root, registration)
		if loadErr != nil {
			items = append(items, map[string]any{"name": registration.Name, "root": registration.Root, "kind": registration.Kind, "languages": registration.Languages})
			continue
		}
		registration = workspace.ApplyProjectTopology(registration, project)
		items = append(items, workspaceSummaryItem(registration, project))
	}
	return model.Envelope{Ok: true, Backend: "registry", Items: items}, nil
}

func (a *App) workspaceDoctor(ctx context.Context) (model.Envelope, error) {
	report, err := workspace.DoctorWorkspaces()
	if err != nil {
		return model.Envelope{}, err
	}
	report = a.enrichWorkspaceReadiness(ctx, report)
	item := map[string]any{
		"aliases_sharing_root":       nonNilRootGroups(report.AliasesSharingRoot),
		"worktree_families":          nonNilWorktreeFamilies(report.WorktreeFamilies),
		"workspace_readiness_issues": nonNilWorkspaceReadinessIssues(report.WorkspaceReadinessIssues),
		"git_case_collisions":        nonNilGitCaseCollisions(report.GitCaseCollisions),
		"stale_paths":                nonNilStalePaths(report.StalePaths),
		"binary_shadowing":           nonNilBinaryCandidates(report.BinaryShadowing),
		"health":                     workspaceDoctorServiceHealth(report),
		"next_actions":               nonNilDoctorActions(workspaceDoctorServiceActions(report)),
		"suggestions":                nonNilStrings(report.Suggestions),
	}
	warnings := []string{}
	if len(report.AliasesSharingRoot) > 0 {
		warnings = append(warnings, "aliases share exact workspace roots; workspace list remains alias-preserving")
	}
	if len(report.WorktreeFamilies) > 0 {
		warnings = append(warnings, "registered worktrees share git common dirs; keep aliases, indexes, watchers, and runtimes separate per physical root")
	}
	if len(report.StalePaths) > 0 {
		warnings = append(warnings, "registry contains stale workspace roots")
	}
	if len(report.WorkspaceReadinessIssues) > 0 {
		warnings = append(warnings, "registered workspaces have governance or documentation readiness issues; inspect before using those aliases in agent sessions")
	}
	if len(report.GitCaseCollisions) > 0 {
		warnings = append(warnings, "git tree contains case-insensitive path collisions; Windows worktrees may be incomplete")
	}
	if len(report.BinaryShadowing) > 1 {
		warnings = append(warnings, "multiple mi-lsp binaries are visible on PATH")
	}
	if doctorActionsContain(report.NextActions, "review_binary_version_drift") {
		warnings = append(warnings, "visible mi-lsp binaries report different revisions")
	}
	return model.Envelope{Ok: true, Backend: "registry-doctor", Items: []map[string]any{item}, Warnings: warnings}, nil
}

func (a *App) workspaceHygiene(request model.CommandRequest) (model.Envelope, error) {
	report, err := workspace.DoctorWorkspaces()
	if err != nil {
		return model.Envelope{}, err
	}
	report = a.enrichWorkspaceReadiness(context.Background(), report)
	applySafe, _ := request.Payload["apply_safe"].(bool)
	item := map[string]any{
		"health":                     workspaceDoctorServiceHealth(report),
		"aliases_sharing_root":       nonNilRootGroups(report.AliasesSharingRoot),
		"worktree_families":          nonNilWorktreeFamilies(report.WorktreeFamilies),
		"workspace_readiness_issues": nonNilWorkspaceReadinessIssues(report.WorkspaceReadinessIssues),
		"stale_paths":                nonNilStalePaths(report.StalePaths),
		"binary_shadowing":           nonNilBinaryCandidates(report.BinaryShadowing),
		"safe_actions":               hygieneSafeActions(report),
		"manual_actions":             hygieneManualActions(report),
		"applied_actions":            []map[string]any{},
		"apply_safe":                 applySafe,
	}
	warnings := workspaceHygieneWarnings(report)
	if applySafe {
		pruneReport, pruneErr := workspace.PruneStaleWorkspaces(true)
		if pruneErr != nil {
			return model.Envelope{}, pruneErr
		}
		item["prune"] = map[string]any{
			"registry":      pruneReport.Registry,
			"removed":       nonNilStalePaths(pruneReport.Removed),
			"removed_count": pruneReport.RemovedCount,
			"skipped":       nonNilStalePaths(pruneReport.Skipped),
		}
		if pruneReport.RemovedCount > 0 {
			item["applied_actions"] = []map[string]any{{
				"id":      "prune_stale_aliases",
				"summary": fmt.Sprintf("removed %d stale workspace alias(es) from registry", pruneReport.RemovedCount),
			}}
			warnings = append(warnings, "registry aliases removed; no files or git worktrees were deleted")
		}
		if len(pruneReport.Skipped) > 0 {
			warnings = append(warnings, "some aliases were skipped because their roots could not be safely classified as missing")
		}
	}
	return model.Envelope{Ok: true, Backend: "registry-hygiene", Items: []map[string]any{item}, Warnings: warnings}, nil
}

func hygieneSafeActions(report workspace.WorkspaceDoctorReport) []map[string]any {
	actions := []map[string]any{}
	if len(report.StalePaths) > 0 {
		actions = append(actions, map[string]any{
			"id":      "prune_stale_aliases",
			"command": "mi-lsp workspace hygiene --apply-safe",
			"reason":  "remove registry aliases whose roots no longer exist; does not delete files or git worktrees",
			"count":   len(report.StalePaths),
		})
	}
	return actions
}

func hygieneManualActions(report workspace.WorkspaceDoctorReport) []map[string]any {
	actions := []map[string]any{}
	if len(report.WorkspaceReadinessIssues) > 0 {
		actions = append(actions, map[string]any{
			"id":      "review_workspace_readiness",
			"command": "mi-lsp workspace hygiene --format toon",
			"reason":  "registered aliases have missing governance docs, blocked governance, empty doc indexes, or docs not ready; remove only aliases you explicitly no longer use",
			"count":   len(report.WorkspaceReadinessIssues),
		})
	}
	if len(report.WorktreeFamilies) > 0 {
		actions = append(actions, map[string]any{
			"id":      "verify_worktree_aliases",
			"command": "mi-lsp workspace list --group-by-root",
			"reason":  "registered worktrees share git common dirs; keep one explicit alias per physical worktree",
			"count":   len(report.WorktreeFamilies),
		})
	}
	if len(report.AliasesSharingRoot) > 0 {
		actions = append(actions, map[string]any{
			"id":      "review_duplicate_root_aliases",
			"command": "mi-lsp workspace list --group-by-root",
			"reason":  "multiple aliases point at the same root; allowed but can confuse handoffs",
			"count":   len(report.AliasesSharingRoot),
		})
	}
	if len(report.BinaryShadowing) > 1 || doctorActionsContain(report.NextActions, "review_binary_version_drift") {
		actions = append(actions, map[string]any{
			"id":      "review_binary_shadowing",
			"command": "mi-lsp workspace doctor --format toon",
			"reason":  "visible mi-lsp binaries may shadow each other or report different revisions",
			"count":   len(report.BinaryShadowing),
		})
	}
	return actions
}

func workspaceHygieneWarnings(report workspace.WorkspaceDoctorReport) []string {
	warnings := []string{}
	if len(report.StalePaths) > 0 {
		warnings = append(warnings, "registry contains stale workspace roots; run mi-lsp workspace hygiene --apply-safe to remove only stale aliases")
	}
	if len(report.WorktreeFamilies) > 0 {
		warnings = append(warnings, "registered worktrees share git common dirs; keep aliases, indexes, watchers, and runtimes separate per physical root")
	}
	if len(report.AliasesSharingRoot) > 0 {
		warnings = append(warnings, "aliases share exact workspace roots; workspace list remains alias-preserving")
	}
	if len(report.WorkspaceReadinessIssues) > 0 {
		warnings = append(warnings, "workspace readiness issues require explicit review; --apply-safe will not remove live aliases")
	}
	return warnings
}

func (a *App) enrichWorkspaceReadiness(ctx context.Context, report workspace.WorkspaceDoctorReport) workspace.WorkspaceDoctorReport {
	registrations, err := workspace.ListWorkspaces()
	if err != nil {
		return report
	}
	canonicalByRoot := canonicalAliasesByRoot(report.AliasesSharingRoot)
	for _, registration := range registrations {
		if strings.TrimSpace(registration.Root) == "" {
			continue
		}
		if _, statErr := os.Stat(registration.Root); statErr != nil {
			continue
		}
		governance := docgraph.InspectGovernance(registration.Root, false)
		docCount := 0
		db, dbErr := openWorkspaceDB(registration, "workspace.doctor")
		if dbErr == nil {
			if count, countErr := store.CountDocRecords(ctx, db); countErr == nil {
				docCount = count
			}
			_ = db.Close()
		}
		docsReady := docCount > 0 && !governance.Blocked && !governance.AECanon.Blocking
		reasons := []string{}
		if governance.Blocked {
			reasons = append(reasons, "governance_blocked")
		}
		if governance.HumanDoc == "" || !fileExists(filepath.Join(registration.Root, governance.HumanDoc)) {
			reasons = append(reasons, "governance_doc_missing")
		}
		if docCount == 0 {
			reasons = append(reasons, "doc_count_zero")
		}
		if !docsReady {
			reasons = append(reasons, "docs_not_ready")
		}
		if len(reasons) == 0 {
			continue
		}
		commands := []string{
			fmt.Sprintf("mi-lsp workspace status %s --format toon", registration.Name),
			fmt.Sprintf("mi-lsp workspace remove %s", registration.Name),
		}
		if canonical := canonicalByRoot[workspaceRootKey(registration.Root)]; canonical != "" && canonical != registration.Name {
			commands = append(commands, fmt.Sprintf("mi-lsp workspace status %s --format toon", canonical))
		} else {
			commands = append(commands, "mi-lsp workspace list --group-by-root")
		}
		report.WorkspaceReadinessIssues = append(report.WorkspaceReadinessIssues, workspace.WorkspaceReadinessIssue{
			Alias:             registration.Name,
			Root:              registration.Root,
			GovernanceBlocked: governance.Blocked,
			DocsReady:         docsReady,
			DocCount:          docCount,
			Reasons:           reasons,
			Commands:          commands,
		})
	}
	return report
}

func canonicalAliasesByRoot(groups []workspace.WorkspaceRootGroup) map[string]string {
	result := map[string]string{}
	for _, group := range groups {
		result[workspaceRootKey(group.Root)] = group.CanonicalAlias
	}
	return result
}

func workspaceRootKey(root string) string {
	clean := filepath.Clean(strings.TrimSpace(root))
	return strings.ToLower(clean)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func workspaceDoctorServiceHealth(report workspace.WorkspaceDoctorReport) string {
	if len(report.StalePaths) > 0 || len(report.GitCaseCollisions) > 0 || len(report.WorkspaceReadinessIssues) > 0 {
		return "action_required"
	}
	return report.Health
}

func workspaceDoctorServiceActions(report workspace.WorkspaceDoctorReport) []workspace.WorkspaceDoctorAction {
	actions := append([]workspace.WorkspaceDoctorAction{}, report.NextActions...)
	if len(report.WorkspaceReadinessIssues) > 0 && !doctorActionsContain(actions, "review_workspace_readiness") {
		actions = append(actions, workspace.WorkspaceDoctorAction{
			ID:       "review_workspace_readiness",
			Severity: "high",
			Command:  "mi-lsp workspace hygiene --format toon",
			Reason:   "registered aliases have governance or documentation readiness issues; inspect and remove only aliases no longer in use",
		})
	}
	return actions
}

func doctorActionsContain(actions []workspace.WorkspaceDoctorAction, id string) bool {
	for _, action := range actions {
		if action.ID == id {
			return true
		}
	}
	return false
}

func (a *App) workspacePrune(request model.CommandRequest) (model.Envelope, error) {
	staleOnly, _ := request.Payload["stale"].(bool)
	if !staleOnly {
		return model.Envelope{}, errors.New("workspace prune requires --stale")
	}
	apply, _ := request.Payload["apply"].(bool)
	report, err := workspace.PruneStaleWorkspaces(apply)
	if err != nil {
		return model.Envelope{}, err
	}
	item := map[string]any{
		"dry_run":       report.DryRun,
		"registry":      report.Registry,
		"candidates":    nonNilStalePaths(report.Candidates),
		"removed":       nonNilStalePaths(report.Removed),
		"removed_count": report.RemovedCount,
		"skipped":       nonNilStalePaths(report.Skipped),
	}
	warnings := []string{}
	if report.DryRun && len(report.Candidates) > 0 {
		warnings = append(warnings, "dry-run only; rerun with --apply to remove stale aliases from registry")
	}
	if !report.DryRun && len(report.Removed) > 0 {
		warnings = append(warnings, "registry aliases removed; no files or git worktrees were deleted")
	}
	if len(report.Skipped) > 0 {
		warnings = append(warnings, "some aliases were skipped because their roots could not be safely classified as missing")
	}
	return model.Envelope{Ok: true, Backend: "registry-prune", Items: []map[string]any{item}, Warnings: warnings}, nil
}

func nonNilRootGroups(items []workspace.WorkspaceRootGroup) []workspace.WorkspaceRootGroup {
	if items == nil {
		return []workspace.WorkspaceRootGroup{}
	}
	return items
}

func nonNilWorktreeFamilies(items []workspace.WorkspaceWorktreeFamily) []workspace.WorkspaceWorktreeFamily {
	if items == nil {
		return []workspace.WorkspaceWorktreeFamily{}
	}
	return items
}

func nonNilWorkspaceReadinessIssues(items []workspace.WorkspaceReadinessIssue) []workspace.WorkspaceReadinessIssue {
	if items == nil {
		return []workspace.WorkspaceReadinessIssue{}
	}
	return items
}

func nonNilGitCaseCollisions(items []workspace.GitCaseCollision) []workspace.GitCaseCollision {
	if items == nil {
		return []workspace.GitCaseCollision{}
	}
	return items
}

func nonNilStalePaths(items []workspace.WorkspaceStalePath) []workspace.WorkspaceStalePath {
	if items == nil {
		return []workspace.WorkspaceStalePath{}
	}
	return items
}

func nonNilBinaryCandidates(items []workspace.BinaryCandidate) []workspace.BinaryCandidate {
	if items == nil {
		return []workspace.BinaryCandidate{}
	}
	return items
}

func nonNilDoctorActions(items []workspace.WorkspaceDoctorAction) []workspace.WorkspaceDoctorAction {
	if items == nil {
		return []workspace.WorkspaceDoctorAction{}
	}
	return items
}

func nonNilStrings(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}

func (a *App) workspaceStatus(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	opts := request.Context
	registration, project, selector, source, resolutionWarnings, resolutionHint, err := a.resolveWorkspaceStatusTarget(request)
	if err != nil {
		return model.Envelope{}, err
	}
	item := workspaceSummaryItem(registration, project)
	item["workspace_root"] = registration.Root
	item["workspace_source"] = source
	item["repos"] = project.Repos
	item["entrypoints"] = project.Entrypoints
	item["docs_read_model"] = workspaceProfileHint(registration.Root)
	autoSync := true
	if value, ok := request.Payload["auto_sync"].(bool); ok {
		autoSync = value
	}
	governance := docgraph.InspectGovernance(registration.Root, autoSync)
	item["governance_doc"] = governance.HumanDoc
	item["governance_projection"] = governance.ProjectionDoc
	item["governance_profile"] = governance.Profile
	item["governance_extends"] = governance.Extends
	item["governance_base"] = governance.EffectiveBase
	item["governance_overlays"] = governance.EffectiveOverlays
	item["governance_sync"] = governance.Sync
	item["governance_index_sync"] = governance.IndexSync
	item["governance_index_sync_details"] = governance.IndexSyncDetails
	item["governance_blocked"] = governance.Blocked
	item["governance_summary"] = governance.Summary
	item["ae_canon"] = governance.AECanon

	// Embeddings status
	embeddingsEnabled := project.Embeddings.Active()
	item["embeddings_enabled"] = embeddingsEnabled
	rerankExtensionEnabled := project.Recall != nil && project.Recall.RerankExtension.Active()
	item["rerank_extension_enabled"] = rerankExtensionEnabled
	if rerankExtensionEnabled {
		item["rerank_extension_mode"] = "local-command"
	}
	recallProfile := "knowledge-wiki"
	if governance.Profile != "" {
		recallProfile = "spec-driven"
	}
	if project.Embeddings != nil && project.Embeddings.Profile != "" {
		recallProfile = project.Embeddings.Profile
	}
	item["recall_profile"] = recallProfile

	memory, memoryWarnings := a.statusMemory(ctx, registration, opts, autoSync, governance.Blocked)
	db, err := openWorkspaceDB(registration, "workspace.status")
	if err != nil {
		item["index_ready"] = false
		item["docs_ready"] = false
		item["docs_index_ready"] = false
		item["doc_count"] = 0
		warnings := append([]string{}, resolutionWarnings...)
		warnings = append(warnings, memoryWarnings...)
		warnings = append(warnings, "workspace has no index yet")
		warnings = append(warnings, governance.Warnings...)
		if governance.Blocked {
			warnings = append(warnings, governance.Issues...)
		}
		if memory != nil && (!isAXIMode(opts) || opts.Full) {
			item["memory"] = buildWorkspaceStatusMemory(memory.Snapshot, memory.Stale)
		}
		envelope := model.Envelope{Ok: true, Workspace: registration.Name, Backend: "sqlite", Items: []any{applyWorkspaceStatusAXIView(item, selector, opts)}, Warnings: warnings, Hint: resolutionHint}
		envelope = attachMemoryPointer(envelope, memory)
		envelope.Continuation = buildStatusContinuation(opts, memory)
		return applyCoachPolicy(envelope, opts), nil
	}
	defer db.Close()
	stats, err := store.WorkspaceStats(ctx, db)
	if err != nil {
		return model.Envelope{}, err
	}
	docCount, err := store.CountDocRecords(ctx, db)
	if err != nil {
		return model.Envelope{}, err
	}
	docsIndexReady := docCount > 0
	item["index_ready"] = stats.Files > 0 || stats.Symbols > 0
	item["docs_ready"] = docsIndexReady && !governance.AECanon.Blocking
	item["docs_index_ready"] = docsIndexReady
	item["doc_count"] = docCount
	item["index_files"] = stats.Files
	item["index_symbols"] = stats.Symbols
	warnings := append([]string{}, resolutionWarnings...)
	warnings = append(warnings, memoryWarnings...)
	warnings = append(warnings, governance.Warnings...)
	if governance.Blocked {
		warnings = append(warnings, governance.Issues...)
	}
	if w := entrypointLanguageMismatchWarning(ctx, db, project); w != "" {
		warnings = append(warnings, w)
	}
	if memory != nil && (!isAXIMode(opts) || opts.Full) {
		item["memory"] = buildWorkspaceStatusMemory(memory.Snapshot, memory.Stale)
	}
	if memory == nil {
		warnings = append(warnings, fmt.Sprintf("reentry memory snapshot absent; rerun 'mi-lsp index --workspace %s' to rebuild memory and memory_pointer", registration.Name))
	}
	if canonicalWikiExists(registration.Root) && !docsIndexReady {
		warnings = appendStringIfMissing(warnings, fmt.Sprintf("documentation index is empty; rerun 'mi-lsp index --workspace %s --docs-only' to rebuild docgraph and memory_pointer", registration.Name))
	}
	if docsIndexReady && !item["index_ready"].(bool) {
		warnings = appendStringIfMissing(warnings, fmt.Sprintf("code catalog is empty while documentation is ready; docs-only recovery rebuilt governed docs and memory_pointer, but nav.find/nav.symbols/semantic code features still need 'mi-lsp index --workspace %s'", registration.Name))
	}
	envelope := model.Envelope{Ok: true, Workspace: registration.Name, Backend: "sqlite", Items: []any{applyWorkspaceStatusAXIView(item, selector, opts)}, Stats: stats, Warnings: warnings, Hint: resolutionHint}
	envelope = attachMemoryPointer(envelope, memory)
	envelope.Continuation = buildStatusContinuation(opts, memory)
	return applyCoachPolicy(envelope, opts), nil
}

func (a *App) resolveWorkspaceStatusTarget(request model.CommandRequest) (model.WorkspaceRegistration, model.ProjectFile, string, string, []string, string, error) {
	resolution, err := workspace.ResolveWorkspaceSelection(request.Context.Workspace, request.Context.CallerCWD)
	if err != nil {
		return model.WorkspaceRegistration{}, model.ProjectFile{}, "", "", nil, "", err
	}
	registration := resolution.Registration
	source := firstNonEmpty(request.Context.WorkspaceSource, string(resolution.Source))
	selector := registration.Name
	warnings := []string{}
	hint := ""
	var project model.ProjectFile

	if source == string(workspace.ResolutionSourceLastWorkspace) {
		if root, ok := callerWorkspaceRoot(request.Context.CallerCWD); ok && !sameWorkspaceStatusPath(root, registration.Root) {
			detected, detectedProject, detectErr := workspace.DetectWorkspaceLayout(root, "")
			if detectErr == nil {
				detected = workspace.ApplyProjectTopology(detected, detectedProject)
				warnings = append(warnings, fmt.Sprintf("workspace omitted; caller cwd contains unregistered workspace %q; ignored unrelated last_workspace=%q", root, registration.Name))
				registration = detected
				project = detectedProject
				source = string(workspace.ResolutionSourceCallerCWD)
				selector = "."
				hint = workspacePathHint(registration.Name)
			}
		}
	}

	if project.Project.Name == "" && len(project.Repos) == 0 {
		project, err = workspace.LoadProjectTopology(registration.Root, registration)
		if err != nil {
			return model.WorkspaceRegistration{}, model.ProjectFile{}, "", "", nil, "", err
		}
		registration = workspace.ApplyProjectTopology(registration, project)
	}

	if source == string(workspace.ResolutionSourcePath) {
		selector = strings.TrimSpace(request.Context.Workspace)
		if selector == "" {
			selector = "."
		}
		if !workspaceAliasRegistered(registration.Name) {
			warnings = append(warnings, fmt.Sprintf("workspace resolved from path %q; generated alias %q is not registered", selector, registration.Name))
			hint = workspacePathHint(registration.Name)
		}
	}

	return registration, project, selector, source, warnings, hint, nil
}

func (a *App) resolvePreflightWorkspaceWithProject(request model.CommandRequest) (model.WorkspaceRegistration, model.ProjectFile, []string, string, error) {
	resolution, err := workspace.ResolveWorkspaceSelection(request.Context.Workspace, request.Context.CallerCWD)
	if err != nil {
		return model.WorkspaceRegistration{}, model.ProjectFile{}, nil, "", err
	}
	registration := resolution.Registration
	project, err := workspace.LoadProjectTopology(registration.Root, registration)
	if err != nil {
		return model.WorkspaceRegistration{}, model.ProjectFile{}, nil, "", err
	}
	registration = workspace.ApplyProjectTopology(registration, project)
	return registration, project, resolution.Warnings, "", nil
}

func (a *App) statusMemory(ctx context.Context, registration model.WorkspaceRegistration, opts model.QueryOptions, autoSync bool, governanceBlocked bool) (*loadedReentryMemory, []string) {
	memory, _ := loadReentryMemory(ctx, registration.Root)
	if memory == nil || !memory.Stale || !autoSync || !opts.Full || governanceBlocked {
		return memory, nil
	}
	warnings := []string{}
	err := store.WithWorkspaceIndexLock(registration.Root, "workspace.status.memory-refresh", func() error {
		_, err := indexer.IndexWorkspaceDocsOnly(ctx, registration.Root)
		return err
	})
	if err != nil {
		return memory, []string{"stale reentry memory refresh failed: " + err.Error()}
	}
	refreshed, _ := loadReentryMemory(ctx, registration.Root)
	if refreshed != nil {
		memory = refreshed
	}
	warnings = append(warnings, "refreshed stale reentry memory snapshot from docs index")
	return memory, warnings
}

func workspacePathHint(alias string) string {
	return fmt.Sprintf("Use --workspace . from this repo, or register a stable alias with `mi-lsp workspace add . --name %s`.", alias)
}

func callerWorkspaceRoot(callerCWD string) (string, bool) {
	current := strings.TrimSpace(callerCWD)
	if current == "" {
		return "", false
	}
	abs, err := filepath.Abs(current)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", false
	}
	if !info.IsDir() {
		abs = filepath.Dir(abs)
	}

	for {
		if workspaceStatusRootMarker(abs) {
			return abs, true
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", false
		}
		abs = parent
	}
}

func workspaceStatusRootMarker(root string) bool {
	markers := []string{
		workspace.ProjectConfigPath(root),
		filepath.Join(root, ".docs", "wiki", "00_gobierno_documental.md"),
		filepath.Join(root, ".git"),
	}
	for _, marker := range markers {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	return false
}

func workspaceAliasRegistered(alias string) bool {
	registrations, err := workspace.ListWorkspaces()
	if err != nil {
		return false
	}
	for _, registration := range registrations {
		if registration.Name == alias {
			return true
		}
	}
	return false
}

func sameWorkspaceStatusPath(left string, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr == nil {
		left = leftAbs
	}
	if rightErr == nil {
		right = rightAbs
	}
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

// entrypointLanguageMismatchWarning returns a warning when the default entrypoint is C#-only
// but TypeScript files significantly outnumber C# files in the index (ratio > 2x).
func entrypointLanguageMismatchWarning(ctx context.Context, db *sql.DB, project model.ProjectFile) string {
	ep := project.Project.DefaultEntrypoint
	if ep == "" {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(ep))
	if ext != ".sln" && ext != ".csproj" {
		return ""
	}
	counts, err := store.FilesCountByLanguage(ctx, db)
	if err != nil || len(counts) == 0 {
		return ""
	}
	tsCount := counts["typescript"]
	csCount := counts["csharp"]
	if tsCount == 0 || csCount*2 >= tsCount {
		return ""
	}
	return fmt.Sprintf(
		"typescript files (%d) outnumber csharp files (%d) 2x: default_entrypoint is C#-only — nav.ask and nav.pack may be biased toward backend docs; consider `mi-lsp workspace add <ts-root> --name <alias>-ts`",
		tsCount, csCount,
	)
}

func applyWorkspaceStatusAXIView(item map[string]any, alias string, opts model.QueryOptions) map[string]any {
	if !isAXIMode(opts) {
		return item
	}
	item["view"] = "full"
	item["next_steps"] = buildWorkspaceAXINextSteps(alias)
	if isAXIPreview(opts) {
		item["view"] = "preview"
		delete(item, "repos")
		delete(item, "entrypoints")
	}
	return item
}

func (a *App) workspaceRemove(request model.CommandRequest) (model.Envelope, error) {
	name, _ := request.Payload["name"].(string)
	if name == "" {
		return model.Envelope{}, errors.New("workspace name is required")
	}
	// Capture the root before removing so its cached doc/FTS entries can be dropped.
	var root string
	if reg, err := workspace.ResolveWorkspace(name); err == nil {
		root = reg.Root
	}
	if err := workspace.RemoveWorkspace(name); err != nil {
		return model.Envelope{}, err
	}
	if root != "" {
		PurgeWorkspaceCaches(root)
	}
	return model.Envelope{Ok: true, Backend: "registry", Items: []map[string]any{{"removed": name}}}, nil
}

func (a *App) resolveWorkspaceWithProject(name string) (model.WorkspaceRegistration, model.ProjectFile, error) {
	registration, err := a.ResolveWorkspace(name)
	if err != nil {
		return model.WorkspaceRegistration{}, model.ProjectFile{}, err
	}
	project, err := workspace.LoadProjectTopology(registration.Root, registration)
	if err != nil {
		return model.WorkspaceRegistration{}, model.ProjectFile{}, err
	}
	registration = workspace.ApplyProjectTopology(registration, project)
	return registration, project, nil
}

func workspaceSummaryItem(registration model.WorkspaceRegistration, project model.ProjectFile) map[string]any {
	return map[string]any{
		"name":               registration.Name,
		"root":               registration.Root,
		"languages":          registration.Languages,
		"kind":               registration.Kind,
		"solution":           registration.Solution,
		"repo_count":         len(project.Repos),
		"entrypoint_count":   len(project.Entrypoints),
		"default_repo":       project.Project.DefaultRepo,
		"default_entrypoint": project.Project.DefaultEntrypoint,
	}
}

func workspaceProfileHint(root string) string {
	if _, err := os.Stat(filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml")); err == nil {
		return ".docs/wiki/_mi-lsp/read-model.toml"
	}
	return "builtin-default"
}
