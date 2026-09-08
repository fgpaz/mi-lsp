package service

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

type docQueryContext struct {
	registration      model.WorkspaceRegistration
	task              string
	rankingTask       string
	rankingNormalized bool
	profile           model.DocsReadProfile
	profileSource     string
	profileWarnings   []string
	family            string
	recentChanges     []model.ReentryMemoryChange
	db                *sql.DB
	dbErr             error
	docs              []model.DocRecord
	docByPath         map[string]model.DocRecord
	ftsScores         map[string]float64
	ranked            []scoredDoc
	rankedByPath      map[string]scoredDoc
}

func loadDocQueryContext(ctx context.Context, registration model.WorkspaceRegistration, task string) *docQueryContext {
	profile, profileSource, profileWarnings := docgraph.LoadProfile(registration.Root)
	rankingTask, rankingNormalized := queryRankingTask(task)
	query := &docQueryContext{
		registration:      registration,
		task:              task,
		rankingTask:       rankingTask,
		rankingNormalized: rankingNormalized,
		profile:           profile,
		profileSource:     profileSource,
		profileWarnings:   append([]string{}, profileWarnings...),
		family:            docgraph.MatchFamily(rankingTask, profile),
		docByPath:         map[string]model.DocRecord{},
		rankedByPath:      map[string]scoredDoc{},
	}
	db, err := openWorkspaceDB(registration, "doc.query", true) // readOnly=true for doc queries
	if err != nil {
		query.dbErr = err
		return query
	}
	query.db = db

	// PERF-02/03: read the active docs generation once; doc records and FTS scores are
	// cached per generation and invalidate structurally on reindex. recentChanges and
	// ranking always run fresh (no cache-hit shortcut), so no stale tiebreak data.
	generation := docsGeneration(ctx, db)
	docs, err := loadDocRecordsCached(ctx, db, registration.Root, generation)
	if err != nil {
		query.dbErr = err
		return query
	}
	query.docs = docs
	for _, doc := range docs {
		query.docByPath[doc.Path] = doc
	}
	if snapshot, ok, err := store.LoadReentrySnapshot(ctx, db); err == nil && ok {
		query.recentChanges = append([]model.ReentryMemoryChange(nil), snapshot.RecentCanonicalChanges...)
	}
	if len(docs) == 0 {
		return query
	}

	query.ftsScores = ftsScoresCached(ctx, db, registration.Root, rankingTask, generation)
	query.ranked = rankDocs(rankingTask, query.family, docs, query.ftsScores, query.profile, query.recentChanges)
	for _, item := range query.ranked {
		query.rankedByPath[item.record.Path] = item
	}

	return query
}

func (q *docQueryContext) Close() error {
	if q == nil || q.db == nil {
		return nil
	}
	return q.db.Close()
}

func (q *docQueryContext) routeTask() string {
	if q == nil {
		return ""
	}
	if strings.TrimSpace(q.rankingTask) != "" {
		return q.rankingTask
	}
	return q.task
}

func (q *docQueryContext) canonicalRoute(opts model.QueryOptions, includeDiscovery bool) model.RouteResult {
	canonical, tier1Why := docgraph.Tier1CanonicalRoute(q.routeTask(), q.profile, q.registration.Root)
	result := model.RouteResult{
		Task:      q.task,
		Mode:      "preview",
		Canonical: canonical,
		Why:       append([]string{fmt.Sprintf("read_model=%s", q.profileSource)}, tier1Why...),
	}
	if q.rankingNormalized {
		result.Why = append(result.Why, "ranking_query=meta_terms_normalized")
	}
	if opts.Full {
		result.Mode = "full"
	}
	if q.dbErr != nil || len(q.ranked) == 0 {
		return result
	}
	primary := q.ranked[0]
	rankedAnchor := model.RouteDoc{
		Path:   primary.record.Path,
		Title:  primary.record.Title,
		DocID:  primary.record.DocID,
		Layer:  primary.record.Layer,
		Family: primary.record.Family,
		Why:    strings.Join(primary.reason, ","),
		Stage:  "anchor",
	}
	anchorDoc := rankedAnchor
	if canonical.AnchorDoc.Path != "" && tier1AnchorIsGovernanceOwned(q.registration.Root, q.profile, canonical.AnchorDoc.Path) && !rankedDocIsDeclaredOwner(q, primary, canonical) {
		if canonical.AnchorDoc.DocID != "" {
			if doc, ok := q.docByPath[canonical.AnchorDoc.Path]; ok {
				primary = scoredDoc{
					record: doc,
					score:  primary.score,
					reason: []string{"explicit_doc_id=" + canonical.AnchorDoc.DocID, "tier1_anchor_preserved"},
				}
				anchorDoc = canonical.AnchorDoc
				if anchorDoc.Title == "" {
					anchorDoc.Title = doc.Title
				}
				if anchorDoc.Layer == "" {
					anchorDoc.Layer = doc.Layer
				}
				if anchorDoc.Family == "" {
					anchorDoc.Family = doc.Family
				}
				anchorDoc.Stage = "anchor"
				result.Why = append(result.Why, "tier2=explicit_anchor_preserved")
			} else {
				// The Tier1 governance anchor exists on disk even though the
				// docs index has not indexed it. Keep the canonical anchor: a
				// document that only mentions the ID in body text must not
				// impersonate document identity (RF-QRY-014). Indexed docs
				// stay as tier2 preview only.
				anchorDoc = canonical.AnchorDoc
				anchorDoc.Stage = "anchor"
				result.Why = append(result.Why, "tier2=anchor_not_indexed")
			}
		} else {
			// Tier1 declared a governance-owned anchor without an explicit
			// doc ID. Preserve it over ranked discovery so a mention-bearing
			// artifact cannot take the anchor seat (RF-QRY-014).
			anchorDoc = canonical.AnchorDoc
			anchorDoc.Stage = "anchor"
			if doc, ok := q.docByPath[canonical.AnchorDoc.Path]; ok {
				primary = scoredDoc{
					record: doc,
					score:  primary.score,
					reason: []string{"tier1_anchor_preserved"},
				}
				if anchorDoc.Title == "" {
					anchorDoc.Title = doc.Title
				}
				if anchorDoc.Layer == "" {
					anchorDoc.Layer = doc.Layer
				}
				if anchorDoc.Family == "" {
					anchorDoc.Family = doc.Family
				}
				result.Why = append(result.Why, "tier2=anchor_preserved")
			} else {
				result.Why = append(result.Why, "tier2=anchor_not_indexed")
			}
		}
	}
	result.Canonical.AnchorDoc = anchorDoc
	result.Canonical.Family = q.family
	result.Why = append(result.Why, "tier2=indexed_docs")

	preview := make([]model.RouteDoc, 0, 2)
	seen := map[string]struct{}{primary.record.Path: {}}
	if anchorDoc.Path != "" {
		seen[anchorDoc.Path] = struct{}{}
	}
	for _, candidate := range q.ranked[1:] {
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
			Stage:  "preview",
		})
	}
	result.Canonical.PreviewPack = preview
	if includeDiscovery {
		result.Discovery = buildDiscoveryAdvisory(q.ranked, 3)
	}
	return result
}

func rankedDocIsDeclaredOwner(q *docQueryContext, candidate scoredDoc, canonical model.RouteCanonicalLane) bool {
	if q == nil || candidate.record.Path == "" || candidate.record.Path == canonical.AnchorDoc.Path || canonical.AnchorDoc.DocID != "" {
		return false
	}
	for _, reason := range candidate.reason {
		if strings.HasPrefix(reason, "owner_hint=") {
			return true
		}
	}
	if !governanceHierarchyDeclaresPath(q.profile, candidate.record.Path) {
		return false
	}
	// A suffix-less query must not silently bind one of several versioned
	// siblings. An explicit full DocID or an owner hint is required in that
	// ambiguous case.
	ownerKey := versionedDocOwnerKey(candidate.record.DocID)
	if ownerKey == "" {
		return true
	}
	matches := 0
	for _, doc := range q.docs {
		if versionedDocOwnerKey(doc.DocID) == ownerKey {
			matches++
		}
	}
	if matches > 1 && !strings.Contains(normalizeRankingText(q.rankingTask), normalizeRankingText(candidate.record.DocID)) {
		return false
	}
	return true
}

func versionedDocOwnerKey(docID string) string {
	parts := strings.Split(strings.TrimSpace(docID), "-")
	if len(parts) < 2 {
		return ""
	}
	suffix := strings.ToUpper(parts[len(parts)-1])
	if suffix == "" {
		return ""
	}
	if !strings.HasPrefix(suffix, "V") {
		return ""
	}
	suffix = strings.TrimPrefix(suffix, "V")
	if suffix == "" {
		return ""
	}
	for _, char := range suffix {
		if char < '0' || char > '9' {
			return ""
		}
	}
	return strings.Join(parts[:len(parts)-1], "-")
}

func governanceHierarchyDeclaresPath(profile model.DocsReadProfile, docPath string) bool {
	normalized := strings.ToLower(filepath.ToSlash(strings.TrimSpace(docPath)))
	if normalized == "" {
		return false
	}
	for _, item := range profile.Governance.Hierarchy {
		for _, pattern := range item.Paths {
			pattern = strings.ToLower(filepath.ToSlash(strings.TrimSpace(pattern)))
			if pattern == "" {
				continue
			}
			if strings.HasSuffix(pattern, "/**") && strings.HasPrefix(normalized, strings.TrimSuffix(pattern, "**")) {
				return true
			}
			if strings.ContainsAny(pattern, "*?[") {
				matched, err := path.Match(pattern, normalized)
				if err == nil && matched {
					return true
				}
				continue
			}
			if normalized == pattern {
				return true
			}
		}
	}
	return false
}

func (q *docQueryContext) primaryDoc(routeResult model.RouteResult) (scoredDoc, bool) {
	if anchorPath := strings.TrimSpace(routeResult.Canonical.AnchorDoc.Path); anchorPath != "" {
		if candidate, ok := q.rankedByPath[anchorPath]; ok {
			return candidate, true
		}
		if doc, ok := q.docByPath[anchorPath]; ok {
			return scoredDoc{
				record: doc,
				score:  1,
				reason: []string{"route_anchor=" + anchorPath},
			}, true
		}
		// Keep a governance route anchor distinct from an unrelated ranked
		// artifact when the canonical document is not indexed yet.
		return scoredDoc{
			record: model.DocRecord{
				Path:    anchorPath,
				Title:   routeResult.Canonical.AnchorDoc.Title,
				DocID:   routeResult.Canonical.AnchorDoc.DocID,
				Layer:   routeResult.Canonical.AnchorDoc.Layer,
				Family:  routeResult.Canonical.AnchorDoc.Family,
			},
			score:  1,
			reason: []string{"route_anchor=" + anchorPath},
		}, true
	}
	if len(q.ranked) > 0 {
		return q.ranked[0], true
	}
	return scoredDoc{}, false
}

// tier1AnchorIsGovernanceOwned reports whether the Tier1 anchor path is
// declared by the workspace read model (family paths, governance hierarchy,
// governance source doc, or Tier1's documented fallback anchors) and exists
// on disk. Mere file existence is not canonical authority: an arbitrary file
// that only mentions an ID is never preserved as the anchor.
func tier1AnchorIsGovernanceOwned(root string, profile model.DocsReadProfile, anchorPath string) bool {
	trimmed := strings.TrimSpace(anchorPath)
	if trimmed == "" || root == "" {
		return false
	}
	normalized := strings.ToLower(filepath.ToSlash(trimmed))
	declared := false
	isDeclaredPattern := func(pattern string) bool {
		pattern = strings.ToLower(filepath.ToSlash(strings.TrimSpace(pattern)))
		if pattern == "" {
			return false
		}
		if strings.HasSuffix(pattern, "/**") {
			return strings.HasPrefix(normalized, strings.TrimSuffix(pattern, "**"))
		}
		if strings.ContainsAny(pattern, "*?[") {
			matched, err := path.Match(pattern, normalized)
			return err == nil && matched
		}
		return normalized == pattern
	}
	for _, family := range profile.Families {
		for _, pattern := range family.Paths {
			if isDeclaredPattern(pattern) {
				declared = true
			}
		}
	}
	for _, item := range profile.Governance.Hierarchy {
		for _, pattern := range item.Paths {
			if isDeclaredPattern(pattern) {
				declared = true
			}
		}
	}
	if strings.EqualFold(trimmed, strings.TrimSpace(profile.Governance.SourceDoc)) {
		declared = true
	}
	// Tier1's documented fallback anchors.
	if strings.EqualFold(normalized, ".docs/wiki/00_gobierno_documental.md") || strings.EqualFold(normalized, "readme.md") {
		declared = true
	}
	if !declared {
		return false
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(trimmed)))
	return err == nil && !info.IsDir()
}
