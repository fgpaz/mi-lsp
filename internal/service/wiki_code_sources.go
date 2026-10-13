package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/livecontext"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

// resolveSourceContext is the opt-in branch of the existing bridge. It does
// not run scored document selection, change q stages, or mutate publication.
func (r *wikiCodeResolver) resolveSourceContext(ctx context.Context, overlay model.WikiCodeOverlay) (model.WikiCodeContext, error) {
	p := &model.WikiCodeSourceProjection{
		Version: model.DocBindingSourceVersion, Availability: model.DocBindingSourceAvailable,
		Resolution: "unresolved", Historical: r.req.ExplicitHistorical || r.req.Historical || r.req.AllowRetired,
		Declarations: []model.WikiCodeSourceDeclaration{},
	}
	r.result.BindingSources = p
	r.setFreshness(overlay)
	p.Truncated = liveOverlayWasTruncated(overlay)
	limit := r.req.Limit
	if limit > 200 {
		limit = 200
	}
	path := strings.TrimSpace(r.req.SourceDocPath)
	id := strings.TrimSpace(r.req.SourceDocID)
	bindingID := strings.TrimSpace(r.req.BindingID)
	owners := map[string]bool{}
	if path != "" {
		// Validate before normalization; unsafe selectors must not alias canon.
		if sanitizeLivePublicPath(path) != path || !model.CanonicalWikiAuthority(path) {
			p.Omission = "unsafe_doc_path"
			return r.finishSourceContext(p), nil
		}
		owners[path] = true
	}
	if id != "" {
		matches := map[string]bool{}
		for _, doc := range r.docs {
			if doc.DocID == id && model.CanonicalWikiAuthority(doc.Path) && !doc.IsSnapshot {
				matches[doc.Path] = true
			}
		}
		for owner, c := range overlay.SourceCollections {
			for _, d := range c.Declarations {
				if d.DocID == id {
					matches[owner] = true
				}
			}
		}
		if path != "" {
			if !matches[path] {
				owners = map[string]bool{}
			}
		} else {
			owners = matches
		}
		if len(owners) == 0 {
			p.Omission = "missing_doc_target"
			if !r.docsLoaded {
				p.Availability = model.DocBindingSourceIncomplete
				p.Reason = "document_catalog_unavailable"
			}
			return r.finishSourceContext(p), nil
		}
	}
	if path == "" && id == "" && bindingID == "" {
		p.Omission = "selector_required"
		return r.finishSourceContext(p), nil
	}
	if path == "" && id == "" {
		// Naked binding IDs use Store's exact bounded lookup. RAM only replaces
		// owners that are actually within the normalized overlay scope.
		c, err := r.readSourceCollection(ctx, "", bindingID, limit)
		if err != nil {
			return model.WikiCodeContext{}, err
		}
		r.copySourceAvailability(p, c)
		for _, d := range c.Declarations {
			owners[d.DocPath] = true
		}
		for owner := range overlay.SourceCollections {
			if sourceOwnerInScope(owner, overlay, r.docs) {
				owners[owner] = true
			}
		}
	}
	paths := make([]string, 0, len(owners))
	for owner := range owners {
		paths = append(paths, owner)
	}
	sort.Strings(paths)
	if len(paths) > 200 {
		paths = paths[:200]
		p.Truncated = true
	}
	if id != "" && len(owners) > 1 {
		p.Resolution = "ambiguous"
		p.Omission = "ambiguous_doc_target"
		p.Candidates = append([]string(nil), paths...)
	}
	all := []model.DocBindingSource{}
	for _, owner := range paths {
		if sanitizeLivePublicPath(owner) != owner || !model.CanonicalWikiAuthority(owner) {
			p.Availability = model.DocBindingSourceIncomplete
			p.Reason = "unsafe_source_owner"
			continue
		}
		base, err := r.readSourceCollection(ctx, owner, bindingID, 200)
		if err != nil {
			return model.WikiCodeContext{}, err
		}
		if base.Availability != model.DocBindingSourceAvailable {
			p.Availability = model.DocBindingSourceIncomplete
			p.Reason = base.Reason
		}
		c := base
		if replacement, ok := overlay.SourceCollections[owner]; ok && sourceOwnerInScope(owner, overlay, r.docs) {
			c = replacement
			// Retired lifecycle must be checked against the persisted collection,
			// even when artifact-state rows are absent or legacy. Also guard ID
			// reuse at a new owner path. Never publish a RAM reactivation.
			if base.DocumentLifecycle == model.DocLifecycleRetired && c.DocumentLifecycle != model.DocLifecycleRetired {
				c.Availability = model.DocBindingSourceIncomplete
				c.Reason = "retired_source_reactivation_rejected"
				c.Declarations = nil
			}
			for _, d := range replacement.Declarations {
				for _, prior := range r.docs {
					if prior.DocID != d.DocID || prior.Path == owner {
						continue
					}
					old, err := r.readSourceCollection(ctx, prior.Path, "", 1)
					if err != nil {
						return model.WikiCodeContext{}, err
					}
					if old.DocumentLifecycle == model.DocLifecycleRetired {
						c.Availability = model.DocBindingSourceIncomplete
						c.Reason = "retired_source_identity_reuse_rejected"
						c.Declarations = nil
					}
				}
			}
		} else {
			filtered := []model.DocBindingSource{}
			for _, d := range c.Declarations {
				masked := false
				for _, t := range overlay.Tombstones {
					// BindingRef is association, not declaration identity.
					if t.DocPath == owner && (t.BlockID == "" || t.BlockID == d.BlockID) {
						// Positive live disk absence does not erase a published
						// retired inventory explicitly requested as history. Other
						// masks and the fail-closed checks below still apply.
						if p.Historical && base.Availability == model.DocBindingSourceAvailable && base.DocumentLifecycle == model.DocLifecycleRetired && t.Reason == "disk-absent" && t.Proof == "disk-absent" {
							continue
						}
						masked = true
					}
				}
				if !masked {
					filtered = append(filtered, d)
				}
			}
			c.Declarations = filtered
		}
		for _, o := range overlay.Omissions {
			if o.Path != owner && o.DocPath != owner {
				continue
			}
			if o.Code == model.OmissionReadFailed || o.Code == model.OmissionParseFailed || o.Code == model.OmissionConcurrentChange || o.Code == model.OmissionUnsafeTarget || o.Code == model.OmissionUnknownDocument {
				c.Availability = model.DocBindingSourceIncomplete
				c.Reason = o.Code
				c.Declarations = nil
			}
		}
		if overlay.Status == model.OverlayStatusConcurrentChange {
			c.Availability = model.DocBindingSourceIncomplete
			c.Reason = "concurrent_change"
			c.Declarations = nil
		}
		r.copySourceAvailability(p, c)
		if len(paths) == 1 {
			p.DocumentLifecycle = c.DocumentLifecycle
		}
		if c.Availability != model.DocBindingSourceAvailable {
			continue
		}
		for _, d := range c.Declarations {
			if !sourceDeclarationInScope(d, overlay.Scope) {
				continue
			}
			if d.DocPath != owner || d.Provenance != "declared" {
				p.Availability = model.DocBindingSourceIncomplete
				p.Reason = "source_owner_mismatch"
				continue
			}
			if bindingID != "" && d.Binding.BindingID != bindingID {
				continue
			}
			if id != "" && d.DocID != id {
				continue
			}
			all = append(all, d)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].DocPath != all[j].DocPath {
			return all[i].DocPath < all[j].DocPath
		}
		return all[i].Binding.BindingID < all[j].Binding.BindingID
	})
	matchedOwners := map[string]bool{}
	for _, d := range all {
		matchedOwners[d.DocPath] = true
	}
	if bindingID != "" && len(matchedOwners) > 1 {
		p.Resolution = "ambiguous"
		p.Omission = "ambiguous_binding_id"
		p.Candidates = nil
		for owner := range matchedOwners {
			p.Candidates = append(p.Candidates, owner)
		}
		sort.Strings(p.Candidates)
	}
	if len(all) > limit {
		all = all[:limit]
		p.Truncated = true
	}
	if p.Truncated && p.Resolution != "ambiguous" {
		p.Omission = "source_lookup_truncated"
	}
	if p.Resolution != "ambiguous" && !p.Truncated && p.Availability == model.DocBindingSourceAvailable {
		if len(all) > 0 || (bindingID == "" && len(paths) == 1) {
			p.Resolution = "resolved"
		} else {
			p.Omission = "missing_binding_target"
		}
	}
	// One WCB evidence per association, never one per source ID/surface. The
	// existing conservative bridge lifecycle gate still wins over any source.
	var bindings []model.DocArtifactBinding
	if p.Resolution == "resolved" && len(paths) == 1 && p.Availability == model.DocBindingSourceAvailable {
		r.req.DocSelectors = nil
		r.req.DocIDs = nil
		r.req.DocPaths = []string{paths[0]}
		var err error
		bindings, err = r.effectiveBindings(ctx, overlay)
		if err != nil {
			p.Availability = model.DocBindingSourceIncomplete
			p.Reason = "binding_catalog_unavailable"
			bindings = nil
		}
		activeRefs := map[string]bool{}
		for _, d := range all {
			if sourceDeclarationActive(d) {
				activeRefs[d.BindingRef] = true
			}
		}
		active := []model.DocArtifactBinding{}
		for _, b := range bindings {
			if activeRefs[b.BindingRef] && r.bindingNavigable(b) {
				active = append(active, b)
			}
		}
		if len(active) > 0 {
			r.setPrimaryDocument()
			r.setFreshness(overlay)
			r.resolveForward(ctx, active)
		}
	}
	for _, d := range all {
		historical := !sourceDeclarationActive(d)
		if historical && !p.Historical {
			if p.Omission == "" {
				p.Omission = "historical_declarations_excluded"
			}
			continue
		}
		out := model.WikiCodeSourceDeclaration{DocBindingSource: d, Resolution: "unresolved", ExcludedFromActive: historical}
		if historical {
			out.Omission = "historical_target_not_evaluated"
		}
		for _, b := range bindings {
			if b.BindingRef == d.BindingRef {
				out.SupersededBy = b.SupersededBy
			}
		}
		if !historical {
			out.Omission = "source_target_not_resolved"
			for _, omission := range r.result.Omissions {
				if omission.BindingRef != d.BindingRef || d.BindingRef == "" {
					continue
				}
				out.Omission = omission.Code
				if omission.Code == model.WikiCodeStatusConcurrentChange {
					out.Resolution = "stale"
				}
				if omission.Code == model.WikiCodeStatusAmbiguousSymbol {
					out.Resolution = "ambiguous"
				}
			}
			for _, evidence := range append(append([]model.WikiCodeEvidence{}, r.result.DirectCode...), r.result.Tests...) {
				if evidence.BindingRef == d.BindingRef && (d.Binding.Symbol == "" || evidence.Status == model.WikiCodeStatusResolvedSymbol) {
					out.Resolution = "resolved"
					out.Omission = ""
				}
			}
		}
		if p.Resolution == "ambiguous" {
			out.Resolution = "ambiguous"
			out.Omission = p.Omission
		}
		if p.Truncated {
			out.Resolution = "unresolved"
			out.Omission = "source_lookup_truncated"
		}
		p.Declarations = append(p.Declarations, out)
	}
	if len(p.Candidates) > limit {
		p.Candidates = p.Candidates[:limit]
		p.Truncated = true
	}
	for len(p.Candidates) > 0 {
		encoded, _ := json.Marshal(p)
		if len(encoded)/4 <= r.req.TokenBudget/2 {
			break
		}
		p.Candidates = p.Candidates[:len(p.Candidates)-1]
		p.Truncated = true
	}
	// Source output is separately bounded and has no bridge/q cursor. Removing
	// rows for output budget must not turn a partial lookup into a winner.
	for len(p.Declarations) > 0 {
		encoded, _ := json.Marshal(p)
		if len(encoded)/4 <= r.req.TokenBudget/2 {
			break
		}
		p.Declarations = p.Declarations[:len(p.Declarations)-1]
		p.Truncated = true
		if p.Resolution != "ambiguous" {
			p.Resolution = "unresolved"
		}
		p.Omission = "source_output_budget_exceeded"
	}
	if p.Truncated || p.Availability != model.DocBindingSourceAvailable {
		// Partial source selection is never sufficient to authorize active
		// evidence, even if a bounded prefix happened to contain one owner.
		r.clearSourceSelection()
		for i := range p.Declarations {
			p.Declarations[i].Resolution = "unresolved"
			p.Declarations[i].Omission = "source_lookup_incomplete"
		}
	}
	return r.finishSourceContext(p), nil
}

func sourceDeclarationInScope(d model.DocBindingSource, scope model.WikiCodeScope) bool {
	// The source request selects an owner, but a block-scoped RAM view must
	// never widen that scope just because the parser carried a complete owner.
	if len(scope.Blocks) == 0 {
		return true
	}
	for _, path := range scope.DocPaths {
		if path == d.DocPath {
			return true
		}
	}
	for _, id := range scope.DocIDs {
		if id == d.DocID {
			return true
		}
	}
	for _, block := range scope.Blocks {
		if (block.DocPath == d.DocPath || block.DocID == d.DocID) && block.BlockID == d.BlockID {
			return true
		}
	}
	return false
}

func sourceDeclarationActive(d model.DocBindingSource) bool {
	return d.DocumentLifecycle == model.DocLifecycleActive && d.Binding.BindingLifecycle == model.DocLifecycleActive && !isRetiredWikiPath(d.DocPath)
}

func sourceOwnerInScope(owner string, overlay model.WikiCodeOverlay, docs []model.DocRecord) bool {
	scope := overlay.Scope
	for _, d := range overlay.SourceCollections[owner].Declarations {
		for _, id := range scope.DocIDs {
			if d.DocID == id {
				return true
			}
		}
	}
	for _, p := range scope.DocPaths {
		if p == owner {
			return true
		}
	}
	for _, b := range scope.Blocks {
		if b.DocPath == owner {
			return true
		}
	}
	for _, doc := range docs {
		if doc.Path == owner {
			for _, id := range scope.DocIDs {
				if id == doc.DocID {
					return true
				}
			}
		}
	}
	return false
}

func (r *wikiCodeResolver) readSourceCollection(ctx context.Context, path, binding string, limit int) (model.DocBindingSourceCollection, error) {
	if r.db == nil {
		return model.DocBindingSourceCollection{Version: model.DocBindingSourceVersion, Availability: model.DocBindingSourceIncomplete, Reason: "database_unavailable"}, nil
	}
	return store.ReadDocBindingSources(ctx, r.db, path, binding, limit)
}

func (r *wikiCodeResolver) copySourceAvailability(p *model.WikiCodeSourceProjection, c model.DocBindingSourceCollection) {
	if c.Availability != model.DocBindingSourceAvailable {
		p.Availability = model.DocBindingSourceIncomplete
		p.Reason = c.Reason
	}
	p.Truncated = p.Truncated || c.Truncated
}

// A partial source view cannot retain privileged document selection, even
// when incompleteness is discovered after forward resolution or budgeting.
func (r *wikiCodeResolver) clearSourceSelection() {
	r.result.PrimaryDoc = model.DocRecord{}
	r.result.AuthorityChain = nil
	r.result.DirectCode = nil
	r.result.Tests = nil
	r.result.SupportingCode = nil
}

func (r *wikiCodeResolver) finishSourceContext(p *model.WikiCodeSourceProjection) model.WikiCodeContext {
	p.DeterminismDigest = ""
	encoded, _ := json.Marshal(p)
	sourceTokens := (len(encoded) + 3) / 4
	if sourceTokens > r.result.TokenBudget {
		p.Truncated = true
		if p.Resolution == "resolved" {
			p.Resolution = "unresolved"
		}
		p.Omission = "source_output_budget_exceeded"
		encoded, _ = json.Marshal(p)
		sourceTokens = (len(encoded) + 3) / 4
	}
	sum := sha256.Sum256(encoded)
	p.DeterminismDigest = hex.EncodeToString(sum[:])
	model.SortWikiCodeContext(r.result)
	applyWikiCodeResolverBounds(r.result, r.req.Limit, "", r.req.Direction)
	budget := r.result.TokenBudget
	r.result.TokenBudget = budget - sourceTokens
	if r.result.TokenBudget < 1 {
		r.result.TokenBudget = 1
	}
	applyWikiCodeResolverBudget(r.result, r.req.Direction)
	r.result.TokenBudget = budget
	r.result.TokenUsed += sourceTokens
	r.result.Truncated = r.result.Truncated || p.Truncated || r.result.TokenUsed > budget
	if r.result.Truncated || p.Availability != model.DocBindingSourceAvailable {
		r.clearSourceSelection()
	}
	// No source pagination is part of v1 cursors. Narrow the exact selectors.
	r.result.NextCursor = ""
	r.result.Continuation = nil
	r.result.DeterminismDigest = model.WikiCodeContextDigest(*r.result)
	return *r.result
}

// traceBindingSources extends nav.trace/nav.wiki.trace, not the provider's q
// grammar. Source selectors are exact and never passed to the legacy scored
// trace lookup. Naked binding IDs may discover only bounded published owners.
func (a *App) traceBindingSources(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	for _, key := range []string{"all", "all_workspaces", "summary"} {
		if enabled, _ := request.Payload[key].(bool); enabled {
			return model.Envelope{}, model.NewWikiCodeContextError("GPH_WIKI_SOURCE_SCOPE_INVALID", "source lookup requires one workspace and exact selectors, not all/summary")
		}
	}
	if stringPayload(request.Payload, "cursor") != "" {
		return model.Envelope{}, model.NewWikiCodeContextError("GPH_WIKI_SOURCE_CURSOR_UNSUPPORTED", "source lookup has no cursor; narrow exact selectors")
	}
	registration, _, err := a.resolveWorkspaceWithProjectForNavigation(request)
	if err != nil {
		return model.Envelope{}, err
	}
	db, _ := openLiveWikiCodeDB(registration)
	if db != nil {
		defer db.Close()
	}
	id := strings.TrimSpace(stringPayload(request.Payload, "rf"))
	path := strings.TrimSpace(stringPayload(request.Payload, "doc"))
	binding := strings.TrimSpace(stringPayload(request.Payload, "binding_id"))
	scope := livecontext.WikiCodeScope{Kind: model.ScopeExactWiki}
	if path != "" {
		scope.DocPaths = []string{path}
	}
	if id != "" {
		scope.DocIDs = []string{id}
	}
	if path == "" && id == "" && binding != "" && db != nil {
		c, err := store.ReadDocBindingSources(ctx, db, "", binding, 200)
		if err != nil {
			return model.Envelope{}, err
		}
		for _, d := range c.Declarations {
			scope.DocPaths = appendLiveUnique(scope.DocPaths, d.DocPath)
		}
	}
	var result model.WikiCodeContext
	if len(scope.DocIDs)+len(scope.DocPaths) > 0 {
		result, err = a.buildLiveWikiCodeContextForRequest(ctx, request, scope, nil)
	} else {
		historical, _ := request.Payload["historical"].(bool)
		result, err = ResolveWikiCodeContext(ctx, registration.Root, db, WikiCodeResolveRequest{
			IncludeBindingSources: true, SourceDocID: id, SourceDocPath: path, BindingID: binding,
			ExplicitHistorical: historical, Limit: request.Context.MaxItems, TokenBudget: request.Context.TokenBudget,
		}, model.WikiCodeOverlay{})
	}
	if err != nil {
		return model.Envelope{}, err
	}
	return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "trace", Items: []model.TraceResult{}, WikiCodeContext: &result, Truncated: result.Truncated}, nil
}
