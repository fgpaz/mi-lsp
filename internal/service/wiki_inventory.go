package service

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/nav"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// wikiInventory handles the nav wiki inventory command.
func (a *App) wikiInventory(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	allWorkspaces := true
	if val, ok := request.Payload["all_workspaces"].(bool); ok {
		allWorkspaces = val
	}

	withLayerCounts := false
	if val, ok := request.Payload["with_layer_counts"].(bool); ok {
		withLayerCounts = val
	}

	workspaceFilter := ""
	if val, ok := request.Payload["workspace"].(string); ok && val != "" {
		workspaceFilter = val
	}

	// If not all-workspaces, require workspace filter
	if !allWorkspaces && workspaceFilter == "" {
		return model.Envelope{}, fmt.Errorf("--workspace must be specified when --all-workspaces=false")
	}

	// Fan-out or single-workspace query
	if allWorkspaces {
		return a.wikiInventoryAllWorkspaces(ctx, withLayerCounts)
	}

	return a.wikiInventorySingleWorkspace(ctx, workspaceFilter, withLayerCounts)
}

// wikiInventoryAllWorkspaces fans out across all registered workspaces.
func (a *App) wikiInventoryAllWorkspaces(ctx context.Context, withLayerCounts bool) (model.Envelope, error) {
	fanOutOpts := nav.WikiFanOutOptions{
		Timeout:  0, // Use default (30s)
		Parallel: 0, // Use default (4)
	}

	fanOutResult, err := nav.FanOutWiki(ctx, fanOutOpts, func(subCtx context.Context, ws model.WorkspaceRegistration) ([]any, map[string]any, error) {
		item, err := buildInventoryItem(subCtx, ws, withLayerCounts)
		if err != nil {
			// Don't fail the whole operation; include the item anyway but mark it
			item = &model.WikiInventoryItem{
				Alias:             ws.Name,
				Root:              ws.Root,
				WikiRoot:          "",
				GovernanceBlocked: false,
				DocsReady:         false,
				DocCount:          0,
				LastIndexedAt:     0,
			}
		}
		return []any{item}, map[string]any{}, nil
	})
	if err != nil {
		return model.Envelope{}, fmt.Errorf("FanOutWiki failed: %w", err)
	}

	// Flatten results
	items := make([]any, 0)
	for _, fanOutItem := range fanOutResult.Items {
		items = append(items, fanOutItem.Items...)
	}

	stats := model.Stats{
		WorkspacesQueried: fanOutResult.WorkspacesQueried,
	}

	envelope := model.Envelope{
		Ok:      true,
		Backend: "sqlite",
		Items:   items,
		Stats:   stats,
	}

	// Include failed workspaces in warnings
	if len(fanOutResult.WorkspacesFailed) > 0 {
		failedNames := make([]string, len(fanOutResult.WorkspacesFailed))
		for i, f := range fanOutResult.WorkspacesFailed {
			failedNames[i] = fmt.Sprintf("%s: %s", f.Alias, f.Reason)
		}
		for _, failedMsg := range failedNames {
			envelope.Warnings = appendStringIfMissing(envelope.Warnings, failedMsg)
		}
	}

	return applyCoachPolicy(envelope, model.QueryOptions{}), nil
}

// wikiInventorySingleWorkspace queries a single workspace.
func (a *App) wikiInventorySingleWorkspace(ctx context.Context, workspaceAlias string, withLayerCounts bool) (model.Envelope, error) {
	// Resolve the workspace
	workspaces, err := workspace.ListWorkspaces()
	if err != nil {
		return model.Envelope{}, fmt.Errorf("failed to list workspaces: %w", err)
	}

	var registration model.WorkspaceRegistration
	found := false
	for _, ws := range workspaces {
		if ws.Name == workspaceAlias {
			registration = ws
			found = true
			break
		}
	}

	if !found {
		return model.Envelope{}, fmt.Errorf("workspace %q not found", workspaceAlias)
	}

	item, err := buildInventoryItem(ctx, registration, withLayerCounts)
	if err != nil {
		return model.Envelope{}, err
	}

	envelope := model.Envelope{
		Ok:        true,
		Workspace: workspaceAlias,
		Backend:   "sqlite",
		Items:     []any{item},
		Stats: model.Stats{
			WorkspacesQueried: 1,
		},
	}

	return applyCoachPolicy(envelope, model.QueryOptions{}), nil
}

// buildInventoryItem constructs a single model.WikiInventoryItem from a workspace.
func buildInventoryItem(ctx context.Context, ws model.WorkspaceRegistration, withLayerCounts bool) (*model.WikiInventoryItem, error) {
	item := &model.WikiInventoryItem{
		Alias:         ws.Name,
		Root:          ws.Root,
		WikiRoot:      detectWikiRoot(ws.Root),
		DocCount:      0,
		LastIndexedAt: 0,
	}

	// Check governance
	governance := docgraph.InspectGovernance(ws.Root, true)
	item.GovernanceBlocked = governance.Blocked

	// Open workspace index database
	db, err := openWorkspaceDB(ws, "wiki.inventory")
	if err != nil {
		// No database yet, docs not ready
		item.DocsReady = false
		return item, nil
	}
	defer db.Close()

	// Count docs
	docCount, err := store.CountDocRecords(ctx, db)
	if err != nil {
		item.DocsReady = false
		return item, nil
	}

	item.DocCount = docCount
	item.DocsReady = docCount > 0

	// Get last indexed timestamp
	lastIndexedAt, err := getLastIndexedTimestamp(ctx, db)
	if err == nil && lastIndexedAt > 0 {
		item.LastIndexedAt = lastIndexedAt
	}

	// If requested and not blocked, count docs per layer
	if withLayerCounts && !item.GovernanceBlocked && item.DocsReady {
		layerCounts, err := countDocsByLayer(ctx, db)
		if err == nil && len(layerCounts) > 0 {
			item.Layers = layerCounts
		}
	}

	return item, nil
}

// detectWikiRoot checks if .docs/wiki exists in the workspace root.
func detectWikiRoot(wsRoot string) string {
	wikiPath := filepath.Join(wsRoot, ".docs", "wiki")
	if _, err := os.Stat(wikiPath); err == nil {
		return wikiPath
	}
	return ""
}

// getLastIndexedTimestamp retrieves the most recent indexed_at timestamp from the doc_records table.
func getLastIndexedTimestamp(ctx context.Context, db *sql.DB) (int64, error) {
	query := `SELECT MAX(indexed_at) FROM doc_records`
	var maxTime interface{}
	row := db.QueryRowContext(ctx, query)
	if err := row.Scan(&maxTime); err != nil {
		return 0, err
	}

	if maxTime == nil {
		return 0, nil
	}

	// Handle both int64 and string from SQLite
	switch v := maxTime.(type) {
	case int64:
		return v, nil
	case string:
		var t int64
		if _, err := fmt.Sscanf(v, "%d", &t); err == nil {
			return t, nil
		}
		return 0, nil
	default:
		return 0, nil
	}
}

// countDocsByLayer counts documents by their layer field.
func countDocsByLayer(ctx context.Context, db *sql.DB) (map[string]int, error) {
	const query = `SELECT layer, COUNT(*) as count FROM doc_records GROUP BY layer`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var layer string
		var count int
		if err := rows.Scan(&layer, &count); err != nil {
			continue
		}
		counts[layer] = count
	}

	return counts, rows.Err()
}
