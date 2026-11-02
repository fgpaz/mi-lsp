package store

import (
	"context"
	"fmt"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// WorktreeDocumentOwnerPaths returns only pinned document ownership metadata.
// A document profile change can invalidate that entire document domain without
// reconstructing a graph or borrowing ownership from today's physical files.
// Callers must separately capture current canonical document inputs and close
// dependencies; this list is neither a filesystem snapshot nor a Ready receipt.
func (s *GraphQuerySnapshot) WorktreeDocumentOwnerPaths(ctx context.Context) ([]string, error) {
	if s == nil || s.closed || s.tx == nil || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	rows, err := s.query(ctx, `SELECT DISTINCT owner_path FROM graph_nodes
		WHERE generation_id=? AND backend_type='docgraph' AND symbol_kind='document'
		ORDER BY owner_path LIMIT 4097`, digestArg(s.generation.GenerationID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var owner string
		if err := rows.Scan(&owner); err != nil {
			return nil, err
		}
		if len(owner) > 4096 {
			return nil, fmt.Errorf("explicit_incomplete: pinned document owner path exceeds bound")
		}
		paths = append(paths, owner)
		if len(paths) > 4096 {
			return nil, fmt.Errorf("explicit_incomplete: changed document profile exceeds pinned owner domain bound")
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Reject unsafe or noncanonical persisted ownership rather than silently
	// cleaning it into another physical mask domain.
	normalized, _, err := normalizeGraphOverlayPaths(paths)
	if err != nil {
		return nil, fmt.Errorf("explicit_incomplete: invalid pinned document ownership: %w", err)
	}
	return normalized, nil
}
