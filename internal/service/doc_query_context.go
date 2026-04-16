package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

type docQueryContext struct {
	registration    model.WorkspaceRegistration
	task            string
	profile         model.DocsReadProfile
	profileSource   string
	profileWarnings []string
	family          string
	recentChanges   []model.ReentryMemoryChange
	db              *sql.DB
	dbErr           error
	docs            []model.DocRecord
	docByPath       map[string]model.DocRecord
	ftsScores       map[string]float64
	ranked          []scoredDoc
	rankedByPath    map[string]scoredDoc
}

func loadDocQueryContext(ctx context.Context, registration model.WorkspaceRegistration, task string) *docQueryContext {
	profile, profileSource, profileWarnings := docgraph.LoadProfile(registration.Root)
	query := &docQueryContext{
		registration:    registration,
		task:            task,
		profile:         profile,
		profileSource:   profileSource,
		profileWarnings: append([]string{}, profileWarnings...),
		family:          docgraph.MatchFamily(task, profile),
		docByPath:       map[string]model.DocRecord{},
		rankedByPath:    map[string]scoredDoc{},
	}
	db, err := store.Open(registration.Root)
	if err != nil {
		query.dbErr = err
		return query
	}
	query.db = db
	docs, err := store.ListDocRecords(ctx, db)
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
	_, query.ftsScores, _ = store.FTSSearchDocs(ctx, db, task, 20)
	query.ranked = rankDocs(task, query.family, docs, query.ftsScores, query.profile, query.recentChanges)
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

func (q *docQueryContext) canonicalRoute(opts model.QueryOptions, includeDiscovery bool) model.RouteResult {
	canonical, tier1Why := docgraph.Tier1CanonicalRoute(q.task, q.profile, q.registration.Root)
	result := model.RouteResult{
		Task:      q.task,
		Mode:      "preview",
		Canonical: canonical,
		Why:       append([]string{fmt.Sprintf("read_model=%s", q.profileSource)}, tier1Why...),
	}
	if opts.Full {
		result.Mode = "full"
	}
	if q.dbErr != nil || len(q.ranked) == 0 {
		return result
	}
	primary := q.ranked[0]
	result.Canonical.AnchorDoc = model.RouteDoc{
		Path:   primary.record.Path,
		Title:  primary.record.Title,
		DocID:  primary.record.DocID,
		Layer:  primary.record.Layer,
		Family: primary.record.Family,
		Why:    strings.Join(primary.reason, ","),
		Stage:  "anchor",
	}
	result.Canonical.Family = q.family
	result.Why = append(result.Why, "tier2=indexed_docs")

	preview := make([]model.RouteDoc, 0, 2)
	seen := map[string]struct{}{primary.record.Path: {}}
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
	}
	if len(q.ranked) > 0 {
		return q.ranked[0], true
	}
	return scoredDoc{}, false
}
