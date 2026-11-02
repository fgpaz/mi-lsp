package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// GraphWorktreeDocumentProfileContext is an index-state compatibility witness,
// not a filesystem read-set receipt. It is usable only for this pinned graph
// generation. Compatible requires every pinned document's raw source digest to
// match an active canonical parser state with one authority digest, whose
// indexing second ended before the graph's creation second. Missing, mixed,
// newer or malformed state is indeterminate rather than a default profile.
// A caller must still capture/recheck the effective physical profile and close
// its affected document domain before using a changed authority configuration.
type GraphWorktreeDocumentProfileContext struct {
	Generation      model.GraphDigest
	AuthorityDigest string
	DocumentCount   int64
	Compatible      bool
}

// WorktreeDocumentProfileContext reads bounded scalar metadata from the same
// read-only transaction as the graph pin. It does not reconstruct graph rows,
// enumerate a worktree, or treat today's workspace_meta value as historical
// evidence for an older generation.
func (s *GraphQuerySnapshot) WorktreeDocumentProfileContext(ctx context.Context) (GraphWorktreeDocumentProfileContext, error) {
	var result GraphWorktreeDocumentProfileContext
	if s == nil || s.closed || s.tx == nil || ctx == nil {
		return result, model.ErrGraphGenerationInvalid
	}
	result.Generation = s.generation.GenerationID
	var matching, authorities int64
	var authority string
	rows, err := s.query(ctx, `SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN d.path IS NOT NULL
			AND d.lifecycle='active' AND d.parser_version=?
			AND lower(d.content_sha256)=lower(hex(n.source_digest))
			AND d.indexed_at>0 AND d.indexed_at<?
			AND length(d.authority_config_hash)=64
			THEN 1 ELSE 0 END),0),
		COUNT(DISTINCT d.authority_config_hash),
		COALESCE(MIN(d.authority_config_hash),'')
		FROM graph_nodes n LEFT JOIN doc_artifact_states d ON d.path=n.owner_path
		WHERE n.generation_id=? AND n.backend_type='docgraph' AND n.symbol_kind='document'`,
		model.ParserVersion, s.generation.CreatedAt.Unix(), digestArg(s.generation.GenerationID))
	if err != nil {
		return result, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return result, err
		}
		return result, fmt.Errorf("explicit_incomplete: pinned document profile lookup returned no result")
	}
	if err := rows.Scan(&result.DocumentCount, &matching, &authorities, &authority); err != nil {
		return result, err
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if result.DocumentCount == 0 {
		// No old document context exists to certify or invalidate. This does
		// not authorize inventing historical ownership for new documents.
		result.Compatible = true
		return result, nil
	}
	if s.generation.CreatedAt.IsZero() || matching != result.DocumentCount || authorities != 1 {
		return result, nil
	}
	authority = strings.ToLower(authority)
	if _, err := model.ParseGraphDigest(authority); err != nil {
		return result, nil
	}
	result.AuthorityDigest = authority
	result.Compatible = true
	return result, nil
}
