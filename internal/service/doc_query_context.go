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
	"github.com/fgpaz/mi-lsp/internal/workspace"
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
	// worktreeBase names the registered checkout whose docs index answers for a
	// linked worktree. It is empty when the query reads its own index.
	worktreeBase string
	// worktreeMasked holds the worktree paths whose base records are not answers.
	worktreeMasked map[string]bool
}

// worktreeBaseDocsReason marks a docs answer served from a registered base
// index with the linked worktree's physical delta applied.
const worktreeBaseDocsReason = "worktree_base_docs"

// maxWorktreeDocBytes bounds one physical markdown file parsed for a worktree.
const maxWorktreeDocBytes = 4 * 1024 * 1024

func loadDocQueryContext(ctx context.Context, registration model.WorkspaceRegistration, task string) *docQueryContext {
	profile, profileSource, profileWarnings := docgraph.LoadProfile(registration.Root)
	profile = docgraph.RouteReadProfile(registration.Root, profile)
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
	// A linked worktree reads the registered base docs index; it never builds one.
	indexRegistration, worktree, err := docsIndexForRegistration(registration)
	if err != nil {
		query.dbErr = err
		return query
	}
	db, err := openWorkspaceDB(indexRegistration, "doc.query", true) // readOnly=true for doc queries
	if err != nil {
		query.dbErr = err
		return query
	}
	query.db = db

	// PERF-02/03: read the active docs generation once; doc records and FTS scores are
	// cached per generation and invalidate structurally on reindex. recentChanges and
	// ranking always run fresh (no cache-hit shortcut), so no stale tiebreak data.
	generation := docsGeneration(ctx, db)
	docs, err := loadDocRecordsCached(ctx, db, indexRegistration.Root, generation)
	if err != nil {
		query.dbErr = err
		return query
	}
	if worktree != nil {
		docs = query.applyWorktreeOverlay(docs, *worktree, indexRegistration)
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

	query.ftsScores = query.unmaskedFTSScores(ftsScoresCached(ctx, db, indexRegistration.Root, rankingTask, generation))
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

// worktreeBaseUnregisteredReason names a linked worktree whose main checkout is
// not registered. Such a worktree has no base index to answer from, and nothing
// may create an index inside it.
const worktreeBaseUnregisteredReason = "worktree_base_unregistered"

// worktreeBaseUnregisteredError is returned for a linked worktree whose main
// checkout is not registered. MainRoot is the checkout the operator should register.
type worktreeBaseUnregisteredError struct {
	Worktree string
	MainRoot string
}

func (e *worktreeBaseUnregisteredError) Error() string {
	return worktreeBaseUnregisteredReason
}

// worktreeNavSearchCommand is the next executable step for a linked worktree
// answer: a nav search over the physical worktree for the question's identifier,
// or for the whole query when it names none. It never repeats the question's own
// command, so a continuation cannot point the caller back at what just ran.
func worktreeNavSearchCommand(root string, query string) string {
	term := strings.TrimSpace(query)
	if identifiers := intentWorktreeIdentifiers(query); len(identifiers) > 0 {
		term = identifiers[0]
	}
	if term == "" {
		return ""
	}
	return fmt.Sprintf("mi-lsp nav search %s --workspace %s", shellQuoteArg(term), shellQuoteArg(root))
}

// worktreeNavSearchContinuation wraps worktreeNavSearchCommand as the
// continuation of a partial linked worktree answer. The Op is set so the
// continuation policy keeps it; an Op-less target is dropped and the envelope
// then repeats the failed command.
func worktreeNavSearchContinuation(root string, query string, reason string) *model.Continuation {
	command := worktreeNavSearchCommand(root, query)
	if command == "" {
		return nil
	}
	return &model.Continuation{Reason: reason, Next: model.ContinuationTarget{Op: "nav.search", Command: command}}
}

// worktreeBaseUnregisteredEnvelope answers a docs question from a linked worktree
// whose main checkout is not registered. Nothing is read from or created in the
// worktree index. The next hint names the registration that would enable docs
// answers, and the continuation is the nav search that works on the physical
// worktree now.
func worktreeBaseUnregisteredEnvelope(registration model.WorkspaceRegistration, backend string, query string, unregistered *worktreeBaseUnregisteredError) model.Envelope {
	env := model.Envelope{
		Ok:        true,
		Partial:   true,
		Reason:    worktreeBaseUnregisteredReason,
		Workspace: registration.Name,
		Backend:   backend,
		Items:     []map[string]any{},
		Warnings:  []string{worktreeBaseUnregisteredReason + ": the main checkout of this linked worktree is not registered, so no docs index answers for it; nothing was created inside the worktree"},
		Hint:      "register the main checkout to answer docs from its index; nav search still reads this worktree as text",
	}
	if unregistered != nil && unregistered.MainRoot != "" {
		next := fmt.Sprintf("mi-lsp workspace add %s", shellQuoteArg(unregistered.MainRoot))
		env.NextHint = &next
	}
	env.Continuation = worktreeNavSearchContinuation(registration.Root, query, worktreeBaseUnregisteredReason)
	return env
}

// docsIndexForRegistration picks the registration whose docs index answers. A
// linked worktree with a registered base reads that base index and carries the
// worktree overlay; every other registration reads its own index unchanged.
// A linked worktree whose main checkout is not registered has no index to read,
// and it never gets one of its own from this path.
func docsIndexForRegistration(registration model.WorkspaceRegistration) (model.WorkspaceRegistration, *workspace.WorktreeOverlay, error) {
	linked, err := workspace.IsLinkedWorktree(registration.Root)
	if err != nil || !linked {
		return registration, nil, err
	}
	overlay, err := workspace.ResolveWorktreeOverlay(registration.Root)
	if err != nil {
		return registration, nil, err
	}
	base, found, err := workspace.RegisteredWorktreeBase(overlay)
	if err != nil {
		return registration, nil, err
	}
	if !found {
		return registration, nil, &worktreeBaseUnregisteredError{Worktree: registration.Root, MainRoot: overlay.BaseRoot}
	}
	return base, &overlay, nil
}

// applyWorktreeOverlay answers a linked worktree from its base docs index. Base
// records for paths the worktree changed, deleted or edited are masked, and the
// worktree's own changed markdown files are parsed from the physical tree so they
// still answer. It returns a new slice because base records are cached and shared.
func (q *docQueryContext) applyWorktreeOverlay(docs []model.DocRecord, overlay workspace.WorktreeOverlay, base model.WorkspaceRegistration) []model.DocRecord {
	q.worktreeBase = base.Name
	q.worktreeMasked = make(map[string]bool)
	for _, rel := range overlay.MaskedPaths() {
		q.worktreeMasked[rel] = true
	}
	deleted := make(map[string]bool, len(overlay.Deleted))
	for _, rel := range overlay.Deleted {
		deleted[rel] = true
	}
	out := make([]model.DocRecord, 0, len(docs))
	masked := 0
	for _, doc := range docs {
		if q.worktreeMasked[filepath.ToSlash(doc.Path)] {
			masked++
			continue
		}
		out = append(out, doc)
	}
	// "parsed" counts only the worktree's own delta. Base checkout edits are read
	// physically too; the readable ones stay uncounted, the unreadable ones are skipped.
	changed := make(map[string]bool, len(overlay.Changed))
	for _, rel := range overlay.Changed {
		changed[rel] = true
	}
	parsed, skipped := 0, 0
	for _, rel := range overlay.ReadPaths() {
		if deleted[rel] || !strings.EqualFold(path.Ext(rel), ".md") {
			continue
		}
		doc, ok := readWorktreeDoc(overlay.Root, rel, q.profile)
		if !ok {
			// A base checkout edit that cannot be read is masked from the base
			// index too, so it is reported as skipped even though it is not parsed.
			skipped++
			continue
		}
		out = append(out, doc)
		if changed[rel] {
			parsed++
		}
	}
	note := fmt.Sprintf("%s: docs answered from registered base %q; %d base records masked by the worktree delta, %d worktree markdown files parsed physically", worktreeBaseDocsReason, base.Name, masked, parsed)
	if skipped > 0 {
		note += fmt.Sprintf(", %d skipped", skipped)
	}
	q.profileWarnings = append(q.profileWarnings, note)
	return out
}

// unmaskedFTSScores drops the base FTS score of every path the worktree delta
// masks. That score was earned by base content, so a physical doc that replaced
// it is judged by its own text, like a new doc. It returns a new map because
// the input is the shared FTS cache entry, which must not change.
func (q *docQueryContext) unmaskedFTSScores(scores map[string]float64) map[string]float64 {
	if q == nil || q.worktreeBase == "" || len(q.worktreeMasked) == 0 || len(scores) == 0 {
		return scores
	}
	out := make(map[string]float64, len(scores))
	for docPath, score := range scores {
		if q.hidesPath(docPath) {
			continue
		}
		out[docPath] = score
	}
	return out
}

// hidesPath reports whether a base answer for docPath is masked by the worktree delta.
func (q *docQueryContext) hidesPath(docPath string) bool {
	return q != nil && q.worktreeMasked[filepath.ToSlash(docPath)]
}

// visibleDocs drops index records that the worktree delta masks. Without a
// worktree overlay it returns docs unchanged.
func (q *docQueryContext) visibleDocs(docs []model.DocRecord) []model.DocRecord {
	if q == nil || q.worktreeBase == "" {
		return docs
	}
	out := make([]model.DocRecord, 0, len(docs))
	for _, doc := range docs {
		if !q.hidesPath(doc.Path) {
			out = append(out, doc)
		}
	}
	return out
}

// visibleScoredDocs is visibleDocs for ranked candidates.
func (q *docQueryContext) visibleScoredDocs(docs []scoredDoc) []scoredDoc {
	if q == nil || q.worktreeBase == "" {
		return docs
	}
	out := make([]scoredDoc, 0, len(docs))
	for _, doc := range docs {
		if !q.hidesPath(doc.record.Path) {
			out = append(out, doc)
		}
	}
	return out
}

// readWorktreeDoc parses one markdown file from the physical worktree. It refuses
// paths that resolve outside the root and files over maxWorktreeDocBytes.
func readWorktreeDoc(root string, rel string, profile model.DocsReadProfile) (model.DocRecord, bool) {
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return model.DocRecord{}, false
	}
	physical, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return model.DocRecord{}, false
	}
	relReal, err := filepath.Rel(rootReal, physical)
	if err != nil || relReal == ".." || strings.HasPrefix(relReal, ".."+string(filepath.Separator)) || filepath.IsAbs(relReal) {
		return model.DocRecord{}, false
	}
	info, err := os.Stat(physical)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxWorktreeDocBytes {
		return model.DocRecord{}, false
	}
	content, err := os.ReadFile(physical)
	if err != nil {
		return model.DocRecord{}, false
	}
	doc, _, _, _, _, _, _, parseErr := docgraph.ParseSingleDoc(root, rel, content, profile)
	if parseErr != nil {
		return model.DocRecord{}, false
	}
	return doc, true
}

func routeDocCandidatePaths(query *docQueryContext, route model.RouteResult, limit int) []string {
	if limit <= 0 {
		limit = 5
	}
	paths := make([]string, 0, limit)
	seen := make(map[string]struct{}, limit)
	add := func(path string) {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path == "" || !strings.EqualFold(filepath.Ext(path), ".md") || len(paths) >= limit {
			return
		}
		if _, exists := seen[path]; exists {
			return
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	add(route.Canonical.AnchorDoc.Path)
	for _, doc := range route.Canonical.PreviewPack {
		add(doc.Path)
	}
	if route.Discovery != nil {
		for _, doc := range route.Discovery.Docs {
			add(doc.Path)
		}
	}
	if query != nil {
		for _, item := range query.ranked {
			add(item.record.Path)
		}
	}
	return paths
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
	owners := exactPackOwners(q.docs, strings.TrimSpace(q.task))
	if len(owners) == 1 && !docgraph.DeclaresDocumentID(q.registration.Root, owners[0].Path, owners[0].DocID) {
		// An inferred legacy index ID must not displace a governed aggregate
		// that contains the requested embedded record.
		owners = nil
	}
	canonical := model.RouteCanonicalLane{}
	tier1Why := []string{}
	if len(owners) == 1 {
		owner := owners[0]
		canonical = model.RouteCanonicalLane{
			AnchorDoc: model.RouteDoc{Path: owner.Path, DocID: owner.DocID, Title: owner.Title, Family: owner.Family, Layer: owner.Layer, Stage: "anchor", Why: "exact_indexed_owner"},
			Family:    owner.Family, Authoritative: true,
		}
		tier1Why = append(tier1Why, "anchor=exact_indexed_owner")
	} else if len(owners) > 1 {
		tier1Why = append(tier1Why, "ambiguous_doc_id="+strings.TrimSpace(q.task))
	} else {
		canonical, tier1Why = docgraph.Tier1CanonicalRoute(q.routeTask(), q.profile, q.registration.Root)
	}
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
	if q.dbErr != nil || len(q.ranked) == 0 || len(owners) > 1 {
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
	if canonical.AnchorDoc.Path != "" && (len(owners) == 1 || tier1AnchorIsGovernanceOwned(q.registration.Root, q.profile, canonical.AnchorDoc.Path)) && !rankedDocIsDeclaredOwner(q, primary, canonical) {
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
				Path:   anchorPath,
				Title:  routeResult.Canonical.AnchorDoc.Title,
				DocID:  routeResult.Canonical.AnchorDoc.DocID,
				Layer:  routeResult.Canonical.AnchorDoc.Layer,
				Family: routeResult.Canonical.AnchorDoc.Family,
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
	if strings.EqualFold(normalized, ".docs/wiki/00_governance.md") || strings.EqualFold(normalized, "readme.md") {
		declared = true
	}
	if !declared {
		return false
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(trimmed)))
	return err == nil && !info.IsDir()
}
