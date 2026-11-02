package store

import (
	"context"
	"encoding/json"
	"fmt"
	pathpkg "path"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// GraphWorktreeDeclaredOwner is provenance from actual declarations in the
// pinned generation, not a guess from a filename or directory prefix.
type GraphWorktreeDeclaredOwner struct {
	Backend, ProjectOrModule string
}

// WorktreeDeclaredCodeOwners reads only distinct declaration contexts for one
// owner. An empty result is NOT proof that the path was an owned source file;
// callers must not manufacture a tombstone certificate from that absence.
func (s *GraphQuerySnapshot) WorktreeDeclaredCodeOwners(ctx context.Context, path string) ([]GraphWorktreeDeclaredOwner, error) {
	if s == nil || s.closed || s.tx == nil || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	paths, _, err := normalizeGraphOverlayPaths([]string{path})
	if err != nil || len(paths) != 1 {
		return nil, model.ErrGraphGenerationInvalid
	}
	rows, err := s.query(ctx, `SELECT backend_type,project_or_module FROM graph_nodes WHERE generation_id=? AND owner_path=?
		UNION
		SELECT 'go',package.project_or_module FROM graph_edges AS edge
		JOIN graph_nodes AS package ON package.generation_id=edge.generation_id AND package.node_id=edge.from_node_id
		WHERE edge.generation_id=? AND edge.owner_path=? AND edge.source_backend='go' AND edge.relation='imports'
		AND package.backend_type='go' AND package.symbol_kind='package'
		ORDER BY 1,2`, digestArg(s.generation.GenerationID), paths[0], digestArg(s.generation.GenerationID), paths[0])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []GraphWorktreeDeclaredOwner
	for rows.Next() {
		var owner GraphWorktreeDeclaredOwner
		if err := rows.Scan(&owner.Backend, &owner.ProjectOrModule); err != nil {
			return nil, err
		}
		result = append(result, owner)
	}
	return result, rows.Err()
}

// WorktreeDocumentBasenamePaths streams only pinned document owner metadata.
// Unicode folding matches the canonical Go resolver rather than SQLite's ASCII
// lower(). Only matching paths are retained; full graph records are not loaded.
// The separate exact-node lookup then loads the minimal candidate context.
func (s *GraphQuerySnapshot) WorktreeDocumentBasenamePaths(ctx context.Context, foldedNames []string, limit int) ([]string, error) {
	if s == nil || s.closed || s.tx == nil || ctx == nil || limit < 1 || limit > 4096 {
		return nil, model.ErrGraphGenerationInvalid
	}
	wanted := map[string]bool{}
	for _, name := range foldedNames {
		wanted[name] = true
	}
	if len(wanted) == 0 {
		return nil, nil
	}
	rows, err := s.query(ctx, `SELECT DISTINCT owner_path FROM graph_nodes WHERE generation_id=? AND backend_type='docgraph' AND symbol_kind='document' ORDER BY owner_path`, digestArg(s.generation.GenerationID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		if wanted[strings.ToLower(pathpkg.Base(path))] {
			paths = append(paths, path)
			if len(paths) > limit {
				return nil, fmt.Errorf("explicit_incomplete: document basename context exceeded its bound")
			}
		}
	}
	return paths, rows.Err()
}

// WorktreeDocumentContextNodes resolves exact path/DocID selectors against the
// pinned graph, preserving all ambiguous candidates. This is minimal lookup
// context, not a document scan. A bounded prefix is never reported as complete.
// The caller still has to merge delta documents and exclude masked base owners.
func (s *GraphQuerySnapshot) WorktreeDocumentContextNodes(ctx context.Context, selectedPaths, docIDs []string, limit int) ([]model.GraphNodeRecord, error) {
	if s == nil || s.closed || s.tx == nil || ctx == nil || limit < 1 || limit > 4096 {
		return nil, model.ErrGraphGenerationInvalid
	}
	paths, _, err := normalizeGraphOverlayPaths(selectedPaths)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 && len(docIDs) == 0 {
		return nil, nil
	}
	// Encode empty selectors as [] rather than SQL NULL. Values are parameters,
	// never interpolated identifiers or SQL fragments.
	if paths == nil {
		paths = []string{}
	}
	if docIDs == nil {
		docIDs = []string{}
	}
	pathJSON, err := json.Marshal(paths)
	if err != nil {
		return nil, err
	}
	idJSON, err := json.Marshal(docIDs)
	if err != nil {
		return nil, err
	}
	rows, err := s.query(ctx, graphNodeSelect+` AND backend_type='docgraph' AND symbol_kind='document'
		AND (owner_path IN (SELECT value FROM json_each(?)) OR semantic_identity IN (SELECT value FROM json_each(?)))
		ORDER BY node_key LIMIT ?`, digestArg(s.generation.GenerationID), string(pathJSON), string(idJSON), limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodes []model.GraphNodeRecord
	for rows.Next() {
		node, err := scanGraphNode(rows, s.generation.GenerationID)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
		if len(nodes) > limit {
			return nil, fmt.Errorf("explicit_incomplete: pinned document lookup exceeded its context bound")
		}
	}
	return nodes, rows.Err()
}

// WorktreeCodeContextNodes supplies exact owner/semantic-identity candidates
// for document code links. It does not resolve ambiguous bindings or claim that
// a masked base node is still an effective anchor; callers must merge native
// delta rows and remove masked rows before claiming complete lookup context.
func (s *GraphQuerySnapshot) WorktreeCodeContextNodes(ctx context.Context, ownerPaths, semanticIDs []string, limit int) ([]model.GraphNodeRecord, error) {
	if s == nil || s.closed || s.tx == nil || ctx == nil || limit < 1 || limit > 4096 {
		return nil, model.ErrGraphGenerationInvalid
	}
	paths, _, err := normalizeGraphOverlayPaths(ownerPaths)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 && len(semanticIDs) == 0 {
		return nil, nil
	}
	if paths == nil {
		paths = []string{}
	}
	if semanticIDs == nil {
		semanticIDs = []string{}
	}
	pathJSON, err := json.Marshal(paths)
	if err != nil {
		return nil, err
	}
	idJSON, err := json.Marshal(semanticIDs)
	if err != nil {
		return nil, err
	}
	rows, err := s.query(ctx, graphNodeSelect+` AND backend_type IN ('go','roslyn','kotlin-language-server')
		AND (owner_path IN (SELECT value FROM json_each(?)) OR semantic_identity IN (SELECT value FROM json_each(?)))
		ORDER BY node_key LIMIT ?`, digestArg(s.generation.GenerationID), string(pathJSON), string(idJSON), limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodes []model.GraphNodeRecord
	for rows.Next() {
		node, err := scanGraphNode(rows, s.generation.GenerationID)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
		if len(nodes) > limit {
			return nil, fmt.Errorf("explicit_incomplete: pinned code lookup exceeded its context bound")
		}
	}
	return nodes, rows.Err()
}

// GraphWorktreePackageOwner is a package owner selected from pinned native
// context metadata. Contains edges select known declaration membership; packages
// lacking any such edge are conservative candidates for Go changes below their
// canonical module directory, including nested modules in the same request root. This does not assert physical file membership or WT presence:
// every candidate still requires a replacement or causal scoped absence receipt.
type GraphWorktreePackageOwner struct {
	ProjectOrModule, OwnerPath string
}

func (s *GraphQuerySnapshot) WorktreeGoPackageOwners(ctx context.Context, selected []string) ([]GraphWorktreePackageOwner, error) {
	if s == nil || s.closed || s.tx == nil || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	paths, _, err := normalizeGraphOverlayPaths(selected)
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	encoded, err := json.Marshal(paths)
	if err != nil {
		return nil, err
	}
	rows, err := s.query(ctx, `WITH packages AS (
		SELECT generation_id,node_id,project_or_module,owner_path FROM graph_nodes
		WHERE generation_id=? AND backend_type='go' AND symbol_kind='package'
	), selected AS (SELECT value AS path FROM json_each(?))
		SELECT DISTINCT package.project_or_module,package.owner_path FROM packages AS package
		WHERE EXISTS (SELECT 1 FROM graph_edges AS edge
			WHERE edge.generation_id=package.generation_id AND edge.from_node_id=package.node_id
			AND edge.relation='contains' AND edge.owner_path IN (SELECT path FROM selected))
		OR ((package.project_or_module='go.mod' OR substr(package.project_or_module,-7)='/go.mod')
			AND EXISTS (SELECT 1 FROM selected WHERE
				(path GLOB '*.go' AND substr(path,1,length(package.project_or_module)-6)=substr(package.project_or_module,1,length(package.project_or_module)-6))
				OR path=package.project_or_module
				OR path=substr(package.project_or_module,1,length(package.project_or_module)-6)||'go.sum')
			AND NOT EXISTS (SELECT 1 FROM graph_edges AS edge
				WHERE edge.generation_id=package.generation_id AND edge.from_node_id=package.node_id AND edge.relation='contains'))
		ORDER BY package.project_or_module,package.owner_path LIMIT 4097`, digestArg(s.generation.GenerationID), string(encoded))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var owners []GraphWorktreePackageOwner
	for rows.Next() {
		var owner GraphWorktreePackageOwner
		if err := rows.Scan(&owner.ProjectOrModule, &owner.OwnerPath); err != nil {
			return nil, err
		}
		owners = append(owners, owner)
		if len(owners) > 4096 {
			return nil, fmt.Errorf("explicit_incomplete: pinned Go package candidate lookup exceeded its bound")
		}
	}
	return owners, rows.Err()
}

// WorktreeGoPackageOwnerContexts resolves every pinned Go module using one of
// the exact semantic package-owner paths. Callers use it to reject ambiguous
// physical masks before collapsing module-scoped package owners to paths.
func (s *GraphQuerySnapshot) WorktreeGoPackageOwnerContexts(ctx context.Context, ownerPaths []string) ([]GraphWorktreePackageOwner, error) {
	if s == nil || s.closed || s.tx == nil || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	paths, _, err := normalizeGraphOverlayPaths(ownerPaths)
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	encoded, err := json.Marshal(paths)
	if err != nil {
		return nil, err
	}
	rows, err := s.query(ctx, `SELECT DISTINCT project_or_module,owner_path FROM graph_nodes
		WHERE generation_id=? AND backend_type='go' AND symbol_kind='package'
		AND owner_path IN (SELECT value FROM json_each(?))
		ORDER BY owner_path,project_or_module LIMIT 4097`, digestArg(s.generation.GenerationID), string(encoded))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []GraphWorktreePackageOwner
	for rows.Next() {
		var owner GraphWorktreePackageOwner
		if err := rows.Scan(&owner.ProjectOrModule, &owner.OwnerPath); err != nil {
			return nil, err
		}
		result = append(result, owner)
		if len(result) > 4096 {
			return nil, fmt.Errorf("explicit_incomplete: pinned package-owner collision lookup exceeded its bound")
		}
	}
	return result, rows.Err()
}

// WorktreeGoPackageNodeKeys returns bounded NodeKey witnesses for one exact
// pinned Go module domain. These keys let a worktree bridge prove that its raw
// request-root identity tuple is the same domain already present in the pin.
func (s *GraphQuerySnapshot) WorktreeGoPackageNodeKeys(ctx context.Context, projectOrModule string) ([]model.GraphDigest, error) {
	if s == nil || s.closed || s.tx == nil || ctx == nil || projectOrModule == "" {
		return nil, model.ErrGraphGenerationInvalid
	}
	rows, err := s.query(ctx, `SELECT node_key FROM graph_nodes WHERE generation_id=? AND backend_type='go' AND symbol_kind='package' AND project_or_module=? ORDER BY node_key LIMIT 4097`, digestArg(s.generation.GenerationID), projectOrModule)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []model.GraphDigest
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		key, err := scanDigest(raw)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
		if len(keys) > 4096 {
			return nil, fmt.Errorf("explicit_incomplete: pinned Go package-key context exceeded its bound")
		}
	}
	return keys, rows.Err()
}

// WorktreeGoPackageNodeKeysInDomain binds pinned witnesses to one repository
// identity as well as the raw module key.
func (s *GraphQuerySnapshot) WorktreeGoPackageNodeKeysInDomain(ctx context.Context, repositoryIdentity, projectOrModule string) ([]model.GraphDigest, error) {
	if s == nil || s.closed || s.tx == nil || ctx == nil || repositoryIdentity == "" || projectOrModule == "" {
		return nil, model.ErrGraphGenerationInvalid
	}
	rows, err := s.query(ctx, "SELECT node_key FROM graph_nodes WHERE generation_id=? AND repository_identity=? AND backend_type='go' AND symbol_kind='package' AND project_or_module=? ORDER BY node_key LIMIT 4097", digestArg(s.generation.GenerationID), repositoryIdentity, projectOrModule)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []model.GraphDigest
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		key, err := scanDigest(raw)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
		if len(keys) > 4096 {
			return nil, fmt.Errorf("explicit_incomplete: pinned Go package-key context exceeded its bound")
		}
	}
	return keys, rows.Err()
}

// WorktreeGraphPathHasRecords checks the exhaustive pinned graph footprint for
// one canonical physical path. An empty result is only a no-record no-op proof;
// it does not establish historical source membership or authorize a tombstone.
func (s *GraphQuerySnapshot) WorktreeGraphPathHasRecords(ctx context.Context, path string) (bool, error) {
	if s == nil || s.closed || s.tx == nil || ctx == nil {
		return false, model.ErrGraphGenerationInvalid
	}
	paths, _, err := normalizeGraphOverlayPaths([]string{path})
	if err != nil || len(paths) != 1 {
		return false, model.ErrGraphGenerationInvalid
	}
	var found bool
	rows, err := s.query(ctx, `SELECT
		EXISTS(SELECT 1 FROM graph_nodes WHERE generation_id=? AND owner_path=?) OR
		EXISTS(SELECT 1 FROM graph_edges WHERE generation_id=? AND owner_path=?) OR
		EXISTS(SELECT 1 FROM graph_unresolved WHERE generation_id=? AND (owner_path=? OR source_document=?)) OR
		EXISTS(SELECT 1 FROM graph_evidence WHERE generation_id=? AND source_uri=?)`,
		digestArg(s.generation.GenerationID), paths[0],
		digestArg(s.generation.GenerationID), paths[0],
		digestArg(s.generation.GenerationID), paths[0], paths[0],
		digestArg(s.generation.GenerationID), paths[0])
	if err != nil {
		return false, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, fmt.Errorf("explicit_incomplete: pinned graph footprint query returned no result")
	}
	if err := rows.Scan(&found); err != nil {
		return false, err
	}
	return found, rows.Err()
}

// WorktreeContextOwnerPaths is bounded in domain by an exact canonical backend
// and project/module. It returns only owner metadata for invalidating a changed
// compiler context, never materialized graph nodes/edges or a copied index.
func (s *GraphQuerySnapshot) WorktreeContextOwnerPaths(ctx context.Context, backend, projectOrModule string) ([]string, error) {
	if s == nil || s.closed || s.tx == nil || ctx == nil || (backend != "go" && backend != "roslyn") || projectOrModule == "" {
		return nil, model.ErrGraphGenerationInvalid
	}
	rows, err := s.query(ctx, `SELECT DISTINCT owner_path FROM graph_nodes WHERE generation_id=? AND backend_type=? AND project_or_module=? ORDER BY owner_path`, digestArg(s.generation.GenerationID), backend, projectOrModule)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	paths := make([]string, 0)
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	paths, _, err = normalizeGraphOverlayPaths(paths)
	return paths, err
}

// GraphWorktreeDocumentTarget is unresolved lookup metadata, not a binding.
// Canonical staging folds doc_* kinds into document; legacy doc_path remains
// accepted. Values may be paths OR IDs and must be resolved conservatively.
type GraphWorktreeDocumentTarget struct{ OwnerPath, SourceDocument, TargetValue string }

func (s *GraphQuerySnapshot) WorktreeDocumentPathTargets(ctx context.Context, limit int) ([]GraphWorktreeDocumentTarget, error) {
	if s == nil || s.closed || s.tx == nil || ctx == nil || limit < 1 || limit > 4096 {
		return nil, model.ErrGraphGenerationInvalid
	}
	rows, err := s.query(ctx, `SELECT owner_path,COALESCE(source_document,''),COALESCE(target_value,'') FROM graph_unresolved WHERE generation_id=? AND backend='docgraph' AND target_kind IN ('document','doc_path') ORDER BY unresolved_id LIMIT ?`, digestArg(s.generation.GenerationID), limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []GraphWorktreeDocumentTarget
	for rows.Next() {
		var target GraphWorktreeDocumentTarget
		if err := rows.Scan(&target.OwnerPath, &target.SourceDocument, &target.TargetValue); err != nil {
			return nil, err
		}
		targets = append(targets, target)
		if len(targets) > limit {
			return nil, fmt.Errorf("explicit_incomplete: document unresolved path lookup exceeded its bound")
		}
	}
	return targets, rows.Err()
}

// WorktreeDocumentDependencyPaths closes incoming document owners, including
// previously unresolved exact targets that may become resolvable in the delta.
// Only pinned owner metadata is read; no graph or document body is copied.
func (s *GraphQuerySnapshot) WorktreeDocumentDependencyPaths(ctx context.Context, changed, targetValues []string) ([]string, error) {
	if s == nil || s.closed || s.tx == nil || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	paths, _, err := normalizeGraphOverlayPaths(changed)
	if err != nil {
		return nil, err
	}
	if paths == nil {
		paths = []string{}
	}
	if targetValues == nil {
		targetValues = []string{}
	}
	pathJSON, err := json.Marshal(paths)
	if err != nil {
		return nil, err
	}
	targetJSON, err := json.Marshal(targetValues)
	if err != nil {
		return nil, err
	}
	rows, err := s.query(ctx, `WITH RECURSIVE affected(path) AS (
		SELECT value FROM json_each(?)
		UNION
		SELECT owner_path FROM graph_unresolved WHERE generation_id=? AND backend='docgraph'
		AND target_value IN (SELECT value FROM json_each(?))
		UNION
		SELECT edge.owner_path FROM affected
		JOIN graph_nodes AS target ON target.generation_id=? AND target.owner_path=affected.path
		JOIN graph_edges AS edge ON edge.generation_id=target.generation_id AND edge.to_node_id=target.node_id
		JOIN graph_nodes AS source ON source.generation_id=edge.generation_id AND source.node_id=edge.from_node_id
		WHERE source.backend_type='docgraph' AND source.symbol_kind='document'
	) SELECT path FROM affected ORDER BY path`, string(pathJSON), digestArg(s.generation.GenerationID), string(targetJSON), digestArg(s.generation.GenerationID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var owners []string
	for rows.Next() {
		var owner string
		if err := rows.Scan(&owner); err != nil {
			return nil, err
		}
		owners = append(owners, owner)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	owners, _, err = normalizeGraphOverlayPaths(owners)
	return owners, err
}

// WorktreeCodeDependencyPaths expands changed file owners to their transitive
// typed code dependents in this pinned generation. These unchanged caller files
// must also be observed and masked when a changed declaration alters its key or
// binding. Only owner paths are retained, never a copy of base nodes or edges.
//
// This is not a configuration-input resolver: callers must separately expand
// module/build-context changes and non-code backend dependencies. Containment
// is deliberately not a dependency; following it backwards would invalidate
// an entire package for every private declaration edit.
func (s *GraphQuerySnapshot) WorktreeCodeDependencyPaths(ctx context.Context, changed []string) ([]string, error) {
	if s == nil || s.closed || s.tx == nil || ctx == nil {
		return nil, model.ErrGraphGenerationInvalid
	}
	paths, _, err := normalizeGraphOverlayPaths(changed)
	if err != nil || len(paths) == 0 {
		return paths, err
	}
	encoded, err := json.Marshal(paths)
	if err != nil {
		return nil, err
	}
	rows, err := s.query(ctx, `WITH RECURSIVE affected(path) AS (
		SELECT value FROM json_each(?)
		UNION
		SELECT edge.owner_path
		FROM affected
		JOIN graph_nodes AS target ON target.generation_id=? AND target.owner_path=affected.path
		JOIN graph_edges AS edge ON edge.generation_id=target.generation_id AND edge.to_node_id=target.node_id
		WHERE edge.relation IN ('imports','references','calls','implements','extends','tests','route_to_handler','reads','writes')
	)
	SELECT path FROM affected ORDER BY path`, string(encoded), digestArg(s.generation.GenerationID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]string, 0, len(paths))
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		result = append(result, path)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Do not silently drop malformed owners from a corrupt generation or
	// report a bounded prefix as a complete semantic invalidation frontier.
	result, _, err = normalizeGraphOverlayPaths(result)
	return result, err
}
