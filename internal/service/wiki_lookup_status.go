package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

type wikiSourceIdentity struct {
	docID    string
	blockID  string
	recordID string
	path     string
	kind     string
}

func baseWikiLookupStatus(workspaceName string, queryText string, docs []model.DocRecord) model.WikiLookupStatus {
	freshness := "current"
	reason := ""
	if docs != nil && len(docs) == 0 {
		freshness = "empty_index"
		reason = "empty_index"
	}
	return model.WikiLookupStatus{
		Query:          strings.TrimSpace(queryText),
		Workspace:      workspaceName,
		IndexFreshness: freshness,
		GovernanceSync: "in_sync",
		Reason:         reason,
	}
}

func wikiLookupStatusForDoc(workspaceName string, queryText string, doc model.DocRecord, rankReasons []string, totalMatches int, shownMatches int, nextHint string) model.WikiLookupStatus {
	status := baseWikiLookupStatus(workspaceName, queryText, nil)
	status.DocID = doc.DocID
	status.Path = doc.Path
	status.Layer = wikiLayerForDoc(doc)
	status.Stage = wikiStageForDoc(doc)
	status.MatchKind = wikiMatchKind(queryText, doc, rankReasons)
	status.RankReason = strings.Join(rankReasons, ",")
	status.TotalMatches = totalMatches
	status.ShownMatches = shownMatches
	status.NextHint = nextHint
	if status.MatchKind == "" {
		status.MatchKind = "content_fallback"
	}
	return status
}

func wikiMatchKind(queryText string, doc model.DocRecord, rankReasons []string) string {
	normalizedQuery := strings.ToUpper(strings.TrimSpace(queryText))
	for _, reason := range rankReasons {
		switch {
		case strings.Contains(reason, "source_id_exact"):
			return "canonical_indexed_id"
		case strings.HasPrefix(reason, "doc_id="):
			return "canonical_indexed_id"
		case strings.HasPrefix(reason, "explicit_doc_id="):
			return "canonical_indexed_id"
		case strings.Contains(reason, "owner_hint="):
			return "alias_read_model_routing"
		case strings.Contains(reason, "tier1_anchor_preserved"):
			return "alias_read_model_routing"
		}
	}
	if doc.DocID != "" && strings.EqualFold(doc.DocID, normalizedQuery) {
		return "canonical_indexed_id"
	}
	if strings.Contains(strings.ToUpper(doc.SearchText), normalizedQuery) || strings.Contains(strings.ToUpper(doc.Title), normalizedQuery) {
		return "mentions_content_fallback"
	}
	return "content_fallback"
}

func sourceIdentityForQuery(ctx context.Context, db *sql.DB, queryText string) (wikiSourceIdentity, bool, error) {
	queryText = strings.TrimSpace(queryText)
	if queryText == "" || db == nil {
		return wikiSourceIdentity{}, false, nil
	}
	blocks, err := store.ListDocSourceBlocks(ctx, db)
	if err != nil {
		return wikiSourceIdentity{}, false, err
	}
	for _, block := range blocks {
		switch {
		case strings.EqualFold(block.DocID, queryText):
			return wikiSourceIdentity{docID: block.DocID, blockID: block.BlockID, path: block.DocPath, kind: "canonical_indexed_id"}, true, nil
		case strings.EqualFold(block.BlockID, queryText):
			return wikiSourceIdentity{docID: block.DocID, blockID: block.BlockID, path: block.DocPath, kind: "canonical_indexed_id"}, true, nil
		}
	}
	records, err := store.ListDocSourceRecords(ctx, db)
	if err != nil {
		return wikiSourceIdentity{}, false, err
	}
	for _, record := range records {
		if strings.EqualFold(record.RecordID, queryText) {
			return wikiSourceIdentity{blockID: record.BlockID, recordID: record.RecordID, path: record.DocPath, kind: "canonical_indexed_id"}, true, nil
		}
	}
	return wikiSourceIdentity{}, false, nil
}

func applySourceIdentity(status *model.WikiLookupStatus, identity wikiSourceIdentity) {
	if status == nil {
		return
	}
	if identity.kind != "" {
		status.MatchKind = identity.kind
	}
	if identity.docID != "" {
		status.DocID = identity.docID
	}
	if identity.blockID != "" {
		status.BlockID = identity.blockID
	}
	if identity.recordID != "" {
		status.RecordID = identity.recordID
	}
	if identity.path != "" {
		status.Path = identity.path
	}
}

func wikiExpansionHint(operation string, workspaceName string, queryText string, shown int, total int) string {
	if total <= shown || total == 0 {
		return ""
	}
	switch operation {
	case "nav.wiki.search":
		return fmt.Sprintf("rerun mi-lsp nav wiki search %q --workspace %s --format toon --full for all %d matches", queryText, workspaceName, total)
	case "nav.wiki.route":
		return fmt.Sprintf("rerun mi-lsp nav wiki route %q --workspace %s --format toon --full for expanded route", queryText, workspaceName)
	case "nav.wiki.pack":
		return fmt.Sprintf("rerun mi-lsp nav wiki pack %q --workspace %s --format toon --full for expanded pack", queryText, workspaceName)
	default:
		return ""
	}
}

func packLookupStatus(ctx context.Context, query *docQueryContext, workspaceName string, task string, result model.PackResult) *model.WikiLookupStatus {
	total := len(query.ranked)
	shown := len(result.Docs)
	nextHint := wikiExpansionHint("nav.wiki.pack", workspaceName, task, shown, total)
	doc := model.DocRecord{}
	reasons := []string{}
	if primary, ok := packLookupPrimaryDoc(result); ok {
		doc = primary
		reasons = append(reasons, "primary_doc="+primary.Path)
	}
	status := wikiLookupStatusForDoc(workspaceName, task, doc, reasons, total, shown, nextHint)
	if len(query.docs) == 0 {
		status.IndexFreshness = "empty_index"
		status.Reason = "empty_index"
	}
	if shown == 0 {
		status.MatchKind = "true_absence"
		status.Reason = "no_pack_candidates"
	}
	if identity, ok, _ := sourceIdentityForQuery(ctx, query.db, task); ok {
		applySourceIdentity(&status, identity)
	}
	return &status
}

func packLookupPrimaryDoc(result model.PackResult) (model.DocRecord, bool) {
	if strings.TrimSpace(result.PrimaryDoc) != "" {
		for _, packDoc := range result.Docs {
			if packDoc.Path == result.PrimaryDoc {
				return model.DocRecord{
					Path:   packDoc.Path,
					Title:  packDoc.Title,
					DocID:  packDoc.DocID,
					Layer:  packDoc.Layer,
					Family: packDoc.Family,
				}, true
			}
		}
		return model.DocRecord{Path: result.PrimaryDoc}, true
	}
	if len(result.Docs) == 0 {
		return model.DocRecord{}, false
	}
	first := result.Docs[0]
	return model.DocRecord{
		Path:   first.Path,
		Title:  first.Title,
		DocID:  first.DocID,
		Layer:  first.Layer,
		Family: first.Family,
	}, true
}

func traceLookupStatus(ctx context.Context, db *sql.DB, workspaceName string, queryText string, result *model.TraceResult) *model.WikiLookupStatus {
	status := baseWikiLookupStatus(workspaceName, queryText, nil)
	status.TotalMatches = 0
	status.ShownMatches = 0
	if result == nil {
		status.MatchKind = "true_absence"
		status.Reason = "true_absence"
		return &status
	}
	status.TotalMatches = 1
	status.ShownMatches = 1
	status.DocID = result.DocID
	status.Path = traceResultPrimaryPath(result)
	status.Layer = result.Layer
	status.Stage = result.Stage
	status.MatchKind = "canonical_indexed_id"
	status.RankReason = result.Status
	if result.Status == "found_but_trace_incomplete" {
		status.Reason = "found_but_trace_incomplete"
	}
	if status.Path == "" || strings.EqualFold(status.Path, result.DocID) || strings.EqualFold(status.Path, result.RF) {
		if doc, ok := traceLookupDoc(ctx, db, queryText); ok {
			status.Path = doc.Path
			if status.DocID == "" {
				status.DocID = doc.DocID
			}
			if status.Layer == "" {
				status.Layer = doc.Layer
			}
			if status.Stage == "" {
				status.Stage = wikiStageForDoc(doc)
			}
		}
	}
	if identity, ok, _ := sourceIdentityForQuery(ctx, db, queryText); ok {
		applySourceIdentity(&status, identity)
	}
	return &status
}

func traceLookupDoc(ctx context.Context, db *sql.DB, queryText string) (model.DocRecord, bool) {
	if db == nil || strings.TrimSpace(queryText) == "" {
		return model.DocRecord{}, false
	}
	if docs, err := store.ListDocRecords(ctx, db); err == nil {
		for _, doc := range docs {
			if strings.EqualFold(doc.DocID, queryText) {
				return doc, true
			}
		}
	}
	if docs, err := store.FindDocRecordsByMention(ctx, db, "doc_id", queryText); err == nil && len(docs) > 0 {
		return docs[0], true
	}
	if docs, err := store.FindDocRecordsBySourceID(ctx, db, queryText); err == nil && len(docs) > 0 {
		return docs[0], true
	}
	return model.DocRecord{}, false
}

func traceResultPrimaryPath(result *model.TraceResult) string {
	if result == nil {
		return ""
	}
	for _, link := range result.Explicit {
		if link.Source == "doc_source" && link.File != "" {
			return link.File
		}
	}
	if result.DocID != "" && strings.HasPrefix(strings.ToUpper(result.DocID), "RF-") {
		return result.RF
	}
	return ""
}
