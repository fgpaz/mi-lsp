package service

import (
	"context"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/wikisource"
)

// traceAnchorPayloadKey carries the anchor split off a `<DOC-ID>#<ANCLA>` target. Enrichment after the
// operation reads payload["rf"], so that key must keep holding a plain document ID.
const traceAnchorPayloadKey = "trace_anchor"

// splitTraceAnchorPayload moves the anchor out of a trace target before dispatch. Other operations and
// targets without a complete `#` pair pass through unchanged, and the caller's payload is never mutated.
func splitTraceAnchorPayload(request model.CommandRequest) model.CommandRequest {
	if request.Operation != "nav.trace" && request.Operation != "nav.wiki.trace" {
		return request
	}
	docID, anchor, ok := splitTraceAnchorRef(stringPayload(request.Payload, "rf"))
	if !ok {
		return request
	}
	payload := make(map[string]any, len(request.Payload)+1)
	for key, value := range request.Payload {
		payload[key] = value
	}
	payload["rf"] = docID
	payload[traceAnchorPayloadKey] = anchor
	request.Payload = payload
	return request
}

// splitTraceAnchorRef reads `<DOC-ID>#<ANCLA>`. Both parts must be non-empty; a bare `DOC#` stays a
// plain document target, so the existing not-found path reports it.
func splitTraceAnchorRef(value string) (string, string, bool) {
	docID, anchor, found := strings.Cut(strings.TrimSpace(value), "#")
	docID = strings.ToUpper(strings.TrimSpace(docID))
	anchor = strings.ToUpper(strings.TrimSpace(anchor))
	if !found || docID == "" || anchor == "" {
		return "", "", false
	}
	return docID, anchor, true
}

// traceWithAnchor runs the document trace and, when the target named an anchor, adds where that anchor
// is declared and which citations point at it or leave its document. It reuses the validator's anchor
// declarations, so trace and validate-contract cannot disagree about where an anchor lives.
func (a *App) traceWithAnchor(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	envelope, err := a.trace(ctx, request)
	anchor := stringPayload(request.Payload, traceAnchorPayloadKey)
	if err != nil || anchor == "" {
		return envelope, err
	}
	docID := stringPayload(request.Payload, "rf")
	registration, project, err := a.resolveWorkspaceWithProjectForNavigation(request)
	if err != nil {
		return model.Envelope{}, err
	}
	wikiRoots, err := harnessWikiRoots(registration, project)
	if err != nil {
		return model.Envelope{}, err
	}
	query := loadDocQueryContext(ctx, registration, wikiContractProtocol)
	defer query.Close()
	if query.dbErr != nil {
		envelope.Warnings = appendStringIfMissing(envelope.Warnings, "anchor_index_unavailable:"+docID+"#"+anchor)
		return envelope, nil
	}
	corpus := filterGovernedWikiRecordsInRoots(registration.Root, query.docs, wikiRoots)
	language := ResolveWikiContractLanguageAtRoot(registration.Root)
	bodies := readWikiContractIndexBodies(registration.Root, corpus)
	focus, refs, warnings := buildWikiAnchorTraceView(docID, anchor, corpus, bodies, language.Language)
	envelope.AnchorFocus = focus
	envelope.AnchorRefs = refs
	for _, warning := range warnings {
		envelope.Warnings = appendStringIfMissing(envelope.Warnings, warning)
	}
	return envelope, nil
}

// buildWikiAnchorTraceView is the pure half of the anchor trace. Given the governed corpus and its bodies,
// it resolves the focus anchor and keeps the citations that point at it (incoming) or leave its document
// (outgoing), sorted by source so the output is stable. A missing anchor is a warning, never a failure.
func buildWikiAnchorTraceView(docID, anchor string, corpus []model.DocRecord, bodies []wikisource.ContractDocBody, language model.WikiContractLanguage) (*model.WikiAnchorFocus, []model.WikiAnchorRef, []string) {
	ref := docID + "#" + anchor
	targets := buildWikiAnchorTargets(corpus, bodies, language)
	target := targets[docID]
	focus := &model.WikiAnchorFocus{Ref: ref, Path: target.path, Status: model.WikiAnchorStatusMissing}
	if line, declared := target.anchors[anchor]; declared {
		focus.Line = line
		focus.Status = model.WikiAnchorStatusResolved
	}
	var warnings []string
	if focus.Status != model.WikiAnchorStatusResolved {
		warnings = append(warnings, "anchor_missing:"+ref)
	}
	contents := make(map[string]string, len(bodies))
	for _, body := range bodies {
		contents[body.Path] = body.Content
	}
	refs := make([]model.WikiAnchorRef, 0)
	for _, record := range corpus {
		content, ok := contents[record.Path]
		if !ok {
			continue
		}
		citations, _ := resolveWikiAnchorRefs(record.Path, strings.ToUpper(record.DocID), content, targets)
		for _, citation := range citations {
			incoming := citation.Ref == ref && citation.TargetPath == focus.Path
			outgoing := citation.SourcePath == focus.Path
			if incoming || outgoing {
				refs = append(refs, citation)
			}
		}
	}
	sort.SliceStable(refs, func(i, j int) bool {
		if refs[i].SourcePath != refs[j].SourcePath {
			return refs[i].SourcePath < refs[j].SourcePath
		}
		if refs[i].SourceLine != refs[j].SourceLine {
			return refs[i].SourceLine < refs[j].SourceLine
		}
		return refs[i].Ref < refs[j].Ref
	})
	return focus, refs, warnings
}
