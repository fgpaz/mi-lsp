package service

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

const (
	wikiCodeDefaultLimit      = 50
	wikiCodeMaxLimit          = 500
	wikiCodeDefaultTokenBudget = 4000
	wikiCodeMaxTokenBudget    = 20000
)

// WikiCodeResolveRequest is the bounded, direction-aware input to the shared
// wiki/code resolver. DB is expected to be a read-only handle when supplied;
// the resolver never executes a write or a schema migration.
type WikiCodeResolveRequest struct {
	Direction      string
	WorkspaceRoot  string
	DB             *sql.DB
	ReadOnlyDB     *sql.DB
	DocSelectors   []string
	DocIDs         []string
	DocPaths       []string
	BlockIDs       []string
	TargetPath     string
	TargetSymbol   string
	CurrentBlockID string
	Limit          int
	TokenBudget    int
	Cursor         string

	// ExplicitHistorical is required to expose a retired declaration as a
	// bounded superseded_by redirect. AllowRetired is retained as an explicit
	// compatibility spelling; neither flag permits retired direct evidence.
	ExplicitHistorical bool
	AllowRetired       bool
	Historical         bool

	GraphSnapshot    *store.GraphQuerySnapshot
	GraphFreshness   model.GraphFreshness
	GraphFreshnessPtr *model.GraphFreshness
}

type wikiCodeResolver struct {
	root            string
	db              *sql.DB
	req             WikiCodeResolveRequest
	docs             []model.DocRecord
	docsByPath       map[string]model.DocRecord
	docsByID         map[string][]model.DocRecord
	docsLoaded       bool
	catalogAvailable bool
	ignoreMatcher    *workspace.IgnoreMatcher
	graphState      string
	graphGeneration string
	result          *model.WikiCodeContext
}

// ResolveWikiCodeContext is the package-level entry point used by adapters that
// already hold a workspace root and a pinned read-only DB handle.
func ResolveWikiCodeContext(ctx context.Context, workspaceRoot string, db *sql.DB, req WikiCodeResolveRequest, overlay model.WikiCodeOverlay) (model.WikiCodeContext, error) {
	if strings.TrimSpace(workspaceRoot) != "" {
		req.WorkspaceRoot = workspaceRoot
	}
	if db != nil {
		req.DB = db
	}
	return resolveWikiCodeContext(ctx, req, overlay)
}

// ResolveWikiCodeContext exposes the same shared resolver to service callers.
// It is intentionally additive; existing command handlers remain unchanged in
// this lane and can adopt this one API during command integration.
func (a *App) ResolveWikiCodeContext(ctx context.Context, req WikiCodeResolveRequest, overlay model.WikiCodeOverlay) (model.WikiCodeContext, error) {
	return a.resolveWikiCodeContext(ctx, req, overlay)
}

// resolveWikiCodeContext is the single bounded implementation for both
// wiki→code and code→wiki lookup. It consumes an ephemeral overlay and never
// repairs, writes, or promotes the derived store.
func (a *App) resolveWikiCodeContext(ctx context.Context, req WikiCodeResolveRequest, overlay model.WikiCodeOverlay) (model.WikiCodeContext, error) {
	if strings.TrimSpace(req.WorkspaceRoot) == "" && a != nil {
		req.WorkspaceRoot = a.RepoRoot
	}
	return resolveWikiCodeContext(ctx, req, overlay)
}

func resolveWikiCodeContext(ctx context.Context, req WikiCodeResolveRequest, overlay model.WikiCodeOverlay) (model.WikiCodeContext, error) {
	if ctx == nil {
		return model.WikiCodeContext{}, model.NewWikiCodeContextError("GPH_WIKI_BACKEND_UNAVAILABLE", "resolver context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return model.WikiCodeContext{}, err
	}

	direction, err := normalizeWikiCodeDirection(req)
	if err != nil {
		return model.WikiCodeContext{}, err
	}
	req.Direction = direction
	if req.Limit < 1 {
		req.Limit = wikiCodeDefaultLimit
	}
	if req.Limit > wikiCodeMaxLimit {
		req.Limit = wikiCodeMaxLimit
	}
	if req.TokenBudget < 1 {
		req.TokenBudget = wikiCodeDefaultTokenBudget
	}
	if req.TokenBudget > wikiCodeMaxTokenBudget {
		req.TokenBudget = wikiCodeMaxTokenBudget
	}

	result := model.WikiCodeContext{
		DirectCode:     make([]model.WikiCodeEvidence, 0),
		Tests:          make([]model.WikiCodeEvidence, 0),
		SupportingCode: make([]model.WikiCodeEvidence, 0),
		Candidates:     make([]model.WikiCodeEvidence, 0),
		WikiContext:    make([]model.WikiCodeWikiContextItem, 0),
		Direction:      direction,
		CodeEvidence:   make([]model.WikiCodeEvidence, 0),
		GraphPaths:     make([]model.WikiCodeGraphPath, 0),
		Drift:          make([]model.WikiCodeDrift, 0),
		Omissions:      make([]model.WikiCodeContextOmission, 0),
		TokenBudget:    req.TokenBudget,
		OverlayDigest:  fmt.Sprint(overlay.Digest),
		Provenance: model.WikiCodeProvenance{
			Backend:       "wiki-code-resolver",
			OverlayDigest: fmt.Sprint(overlay.Digest),
			QueryOnly:     true,
		},
	}
	database := req.DB
	if database == nil {
		database = req.ReadOnlyDB
	}
	ownedDB := false
	if database == nil && strings.TrimSpace(req.WorkspaceRoot) != "" {
		if opened, openErr := store.OpenReadOnlyExisting(req.WorkspaceRoot, store.WorkspaceDBPath(req.WorkspaceRoot)); openErr == nil {
			database = opened
			ownedDB = true
		}
	}
	if ownedDB {
		defer database.Close()
	}
	copyOverlayCost(overlay, &result.Cost)
	resolver := &wikiCodeResolver{root: req.WorkspaceRoot, db: database, req: req, result: &result}
	if strings.TrimSpace(req.WorkspaceRoot) != "" {
		resolver.ignoreMatcher, _ = workspace.LoadIgnoreMatcher(req.WorkspaceRoot, nil)
	}
	resolver.graphState, resolver.graphGeneration = graphFreshness(req)
	resolver.appendOverlayOmissions(overlay)

	if resolver.db != nil {
		if generation, _, generationErr := store.WorkspaceMetaValue(ctx, resolver.db, store.WorkspaceMetaActiveDocsGeneration); generationErr == nil {
			result.DocGenerationID = generation
			result.Provenance.DocsGeneration = generation
		}
		if resolver.catalogProbe(ctx) == nil {
			resolver.catalogAvailable = true
		}
	}
	if result.DocGenerationID == "" && strings.TrimSpace(fmt.Sprint(overlay.BaseGeneration)) != "" {
		result.DocGenerationID = fmt.Sprint(overlay.BaseGeneration)
		result.Provenance.DocsGeneration = fmt.Sprint(overlay.BaseGeneration)
	}
	resolver.loadDocs(ctx)
	if resolver.docsByPath == nil {
		resolver.docsByPath = make(map[string]model.DocRecord)
	}
	if resolver.docsByID == nil {
		resolver.docsByID = make(map[string][]model.DocRecord)
	}
	if err := resolver.validateDocumentIDs(); err != nil {
		return model.WikiCodeContext{}, err
	}
	resolver.setPrimaryDocument()
	resolver.setFreshness(overlay)

	bindings, bindingErr := resolver.effectiveBindings(ctx, overlay)
	if bindingErr != nil {
		if errors.Is(bindingErr, context.Canceled) || errors.Is(bindingErr, context.DeadlineExceeded) {
			return model.WikiCodeContext{}, bindingErr
		}
		var bindingContextErr *model.WikiCodeContextError
		if errors.As(bindingErr, &bindingContextErr) && (bindingContextErr.Code == "GPH_WIKI_DUPLICATE_BINDING_REF" || bindingContextErr.Code == "GPH_WIKI_DUPLICATE_DOC_ID") {
			return model.WikiCodeContext{}, bindingErr
		}
		setWikiFreshnessDomain(&result.Freshness, "bindings", "unknown")
		resolver.result.Omissions = append(resolver.result.Omissions, model.WikiCodeContextOmission{
			Code:   model.WikiCodeStatusCatalogUnavailable,
			Status: model.WikiCodeStatusCatalogUnavailable,
			Source: "doc_artifact_bindings",
			Reason: "persisted binding catalog could not be read",
		})
		bindings = resolver.overlayBindings(overlay)
	}
	resolver.result.Cost.BindingsExamined = len(bindings)
	if err := validateBindingDocumentIDs(bindings); err != nil {
		return model.WikiCodeContext{}, err
	}

	switch direction {
	case model.WikiCodeDirectionWikiToCode:
		resolver.resolveForward(ctx, bindings)
	case model.WikiCodeDirectionCodeToWiki:
		resolver.resolveReverse(ctx, bindings)
	}

	model.SortWikiCodeContext(&result)
	resolver.setClassification()
	applyWikiCodeResolverBounds(&result, req.Limit, req.Cursor, req.Direction)
	applyWikiCodeResolverBudget(&result, req.Direction)
	model.SortWikiCodeContext(&result)
	result.DeterminismDigest = model.WikiCodeContextDigest(result)
	return result, nil
}

func normalizeWikiCodeDirection(req WikiCodeResolveRequest) (string, error) {
	direction := strings.ToLower(strings.TrimSpace(req.Direction))
	switch direction {
	case "", "wiki_to_code", "wiki-code", "wiki→code", "wiki->code", "forward":
		if direction == "" && strings.TrimSpace(req.TargetPath) != "" {
			return model.WikiCodeDirectionCodeToWiki, nil
		}
		return model.WikiCodeDirectionWikiToCode, nil
	case "code_to_wiki", "code-wiki", "code→wiki", "code->wiki", "reverse":
		return model.WikiCodeDirectionCodeToWiki, nil
	default:
		return "", model.NewWikiCodeContextError("GPH_WIKI_DIRECTION_INVALID", "direction must be wiki_to_code or code_to_wiki")
	}
}

func graphFreshness(req WikiCodeResolveRequest) (string, string) {
	freshness := req.GraphFreshness
	if freshness.State == "" && req.GraphFreshnessPtr != nil {
		freshness = *req.GraphFreshnessPtr
	}
	if freshness.State != "" {
		switch freshness.State {
		case model.GraphFreshnessCurrent, "fresh":
			if req.GraphSnapshot != nil {
				if generation, err := req.GraphSnapshot.Generation(); err == nil {
					return model.GraphFreshnessCurrent, generation.GenerationID.String()
				}
			}
			return model.GraphFreshnessUnknown, ""
		case model.GraphFreshnessStale, model.GraphFreshnessLagging:
			return model.GraphFreshnessStale, freshness.GenerationID
		case model.GraphFreshnessInvalid:
			return model.GraphFreshnessUnknown, freshness.GenerationID
		default:
			return model.GraphFreshnessUnknown, freshness.GenerationID
		}
	}
	if req.GraphSnapshot != nil {
		generation, err := req.GraphSnapshot.Generation()
		if err != nil {
			return model.GraphFreshnessUnknown, ""
		}
		return model.GraphFreshnessCurrent, generation.GenerationID.String()
	}
	return model.GraphFreshnessUnknown, ""
}

func (r *wikiCodeResolver) loadDocs(ctx context.Context) {
	r.docsByPath = make(map[string]model.DocRecord)
	r.docsByID = make(map[string][]model.DocRecord)
	if r.db == nil {
		return
	}
	docs, err := store.ListDocRecords(ctx, r.db)
	if err != nil {
		return
	}
	r.docsLoaded = true
	sort.SliceStable(docs, func(i, j int) bool {
		left := []string{normalizeWikiPath(docs[i].Path), strings.ToUpper(strings.TrimSpace(docs[i].DocID))}
		right := []string{normalizeWikiPath(docs[j].Path), strings.ToUpper(strings.TrimSpace(docs[j].DocID))}
		return compareWikiStringTuple(left, right) < 0
	})
	r.docs = docs
	for _, doc := range docs {
		path := normalizeWikiPath(doc.Path)
		if !canonicalActiveWikiPath(path, doc.IsSnapshot) {
			continue
		}
		r.docsByPath[path] = doc
		if id := strings.ToUpper(strings.TrimSpace(doc.DocID)); id != "" {
			r.docsByID[id] = append(r.docsByID[id], doc)
		}
	}
}

func (r *wikiCodeResolver) validateDocumentIDs() error {
	pathsByID := make(map[string][]string)
	for _, doc := range r.docs {
		if !model.CanonicalWikiAuthority(doc.Path) || doc.IsSnapshot {
			continue
		}
		id := strings.ToUpper(strings.TrimSpace(doc.DocID))
		if id == "" {
			continue
		}
		pathsByID[id] = append(pathsByID[id], normalizeWikiPath(doc.Path))
	}
	for id, paths := range pathsByID {
		if len(paths) > 1 {
			sort.Strings(paths)
			return model.NewWikiCodeContextError("GPH_WIKI_DUPLICATE_DOC_ID", "document ID "+id+" is declared by multiple canonical paths: "+strings.Join(paths, ","))
		}
	}
	return nil
}

func (r *wikiCodeResolver) setPrimaryDocument() {
	selectors := resolverDocSelectors(r.req)
	for _, selector := range selectors {
		if doc, ok := r.docsByPath[normalizeWikiPath(selector)]; ok {
			r.result.PrimaryDoc = doc
			break
		}
		if docs := r.docsByID[strings.ToUpper(strings.TrimSpace(selector))]; len(docs) == 1 {
			r.result.PrimaryDoc = docs[0]
			break
		}
	}
	if r.result.PrimaryDoc.Path == "" && len(r.docs) > 0 && r.req.Direction == model.WikiCodeDirectionWikiToCode {
		for _, doc := range r.docs {
			if canonicalActiveWikiPath(doc.Path, doc.IsSnapshot) {
				r.result.PrimaryDoc = doc
				break
			}
		}
	}
	if r.result.PrimaryDoc.Path != "" {
		r.result.AuthorityChain = append(r.result.AuthorityChain, model.WikiCodeAuthorityEntry{
			DocID: r.result.PrimaryDoc.DocID, Path: r.result.PrimaryDoc.Path,
			Layer: r.result.PrimaryDoc.Layer, Role: "primary", ContentHash: r.result.PrimaryDoc.ContentHash,
		})
	}
}

func (r *wikiCodeResolver) setFreshness(overlay model.WikiCodeOverlay) {
	docsStatus := "current"
	bindingsStatus := "current"
	switch strings.ToLower(strings.TrimSpace(fmt.Sprint(overlay.Status))) {
	case "concurrent_change":
		docsStatus = "concurrent_change"
		bindingsStatus = "concurrent_change"
	case "overlay":
		docsStatus = "overlay"
		bindingsStatus = "overlay"
	case "stale":
		docsStatus = "stale"
		bindingsStatus = "stale"
	case "unknown":
		docsStatus = "unknown"
		bindingsStatus = "unknown"
	}
	if (len(overlay.Additions) > 0 || len(overlay.Tombstones) > 0) && docsStatus != "concurrent_change" {
		docsStatus = "overlay"
		bindingsStatus = "overlay"
	}
	overlayActive := docsStatus == "overlay" || bindingsStatus == "overlay"
	if (r.db == nil || !r.docsLoaded) && !overlayActive && docsStatus == "current" {
		docsStatus = "unknown"
	}
	if r.db == nil && !overlayActive && bindingsStatus == "current" {
		bindingsStatus = "unknown"
	}
	catalogStatus := "unknown"
	if r.catalogAvailable {
		catalogStatus = "current"
	}
	graphStatus := "unknown"
	switch r.graphState {
	case model.GraphFreshnessCurrent:
		graphStatus = "current"
	case model.GraphFreshnessStale:
		graphStatus = "stale"
	}
	authorityStatus := "current"
	if r.result.PrimaryDoc.Path != "" && !model.CanonicalWikiAuthority(r.result.PrimaryDoc.Path) {
		authorityStatus = "unknown"
	}
	setWikiFreshnessDomain(&r.result.Freshness, "docs_manifest", docsStatus)
	setWikiFreshnessDomain(&r.result.Freshness, "bindings", bindingsStatus)
	setWikiFreshnessDomain(&r.result.Freshness, "catalog", catalogStatus)
	setWikiFreshnessDomain(&r.result.Freshness, "graph", graphStatus)
	setWikiFreshnessDomain(&r.result.Freshness, "authority", authorityStatus)
	if r.graphGeneration != "" {
		r.result.CodeGenerationID = r.graphGeneration
		r.result.Provenance.GraphGeneration = r.graphGeneration
	}
}

func copyOverlayCost(overlay any, cost *model.WikiCodeResolveCost) {
	if cost == nil || overlay == nil {
		return
	}
	encoded, err := json.Marshal(overlay)
	if err != nil {
		return
	}
	var object map[string]any
	if json.Unmarshal(encoded, &object) != nil {
		return
	}
	for key, raw := range object {
		value, ok := raw.(float64)
		if !ok {
			continue
		}
		switch strings.ToLower(strings.ReplaceAll(key, "_", "")) {
		case "metadatachecked":
			cost.MetadataChecked += int(value)
		case "fileshashed":
			cost.FilesHashed += int(value)
		case "filesparsed":
			cost.FilesParsed += int(value)
		case "unchangedreused":
			cost.UnchangedReused += int(value)
		case "bytesread":
			cost.BytesRead += int64(value)
		}
	}
	if nested, ok := object["cost"].(map[string]any); ok {
		copyOverlayCost(nested, cost)
	}
}

func (r *wikiCodeResolver) catalogProbe(ctx context.Context) error {
	if r.db == nil {
		return model.NewWikiCodeContextError(model.WikiCodeStatusCatalogUnavailable, "catalog handle is unavailable")
	}
	r.result.Cost.CatalogQueries++
	var count int
	return r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM files").Scan(&count)
}

func (r *wikiCodeResolver) appendOverlayOmissions(overlay model.WikiCodeOverlay) {
	for _, item := range overlay.Omissions {
		encoded, err := json.Marshal(item)
		if err != nil {
			continue
		}
		var omission model.WikiCodeContextOmission
		if json.Unmarshal(encoded, &omission) != nil {
			continue
		}
		if omission.Code == "" {
			omission.Code = reflectedStringField(item, "Code", "Status", "ReasonCode")
		}
		if omission.Code == "" {
			omission.Code = model.WikiCodeStatusConcurrentChange
		}
		if omission.Status == "" {
			omission.Status = omission.Code
		}
		r.result.Omissions = append(r.result.Omissions, omission)
	}
}

func (r *wikiCodeResolver) effectiveBindings(ctx context.Context, overlay model.WikiCodeOverlay) ([]model.DocArtifactBinding, error) {
	var persisted []model.DocArtifactBinding
	var err error
	if r.db != nil {
		persisted, err = r.loadPersistedBindings(ctx)
		if err != nil {
			return nil, err
		}
	}

	byRef := make(map[string]model.DocArtifactBinding, len(persisted)+len(overlay.Additions))
	for _, binding := range persisted {
		binding = normalizeBinding(binding)
		if binding.BindingRef == "" {
			continue
		}
		if _, exists := byRef[binding.BindingRef]; exists {
			return nil, model.NewWikiCodeContextError("GPH_WIKI_DUPLICATE_BINDING_REF", "binding_ref is not unique: "+binding.BindingRef)
		}
		byRef[binding.BindingRef] = binding
	}
	for _, tombstone := range overlay.Tombstones {
		ref, docPath, blockID := tombstoneFields(tombstone)
		for key, binding := range byRef {
			if (ref != "" && key == ref) || (docPath != "" && normalizeWikiPath(binding.DocPath) == normalizeWikiPath(docPath) && (blockID == "" || binding.BlockID == blockID)) {
				delete(byRef, key)
			}
		}
	}
	seenAddition := make(map[string]struct{}, len(overlay.Additions))
	for _, binding := range overlay.Additions {
		binding = normalizeBinding(binding)
		if binding.BindingRef == "" {
			continue
		}
		if _, exists := seenAddition[binding.BindingRef]; exists {
			return nil, model.NewWikiCodeContextError("GPH_WIKI_DUPLICATE_BINDING_REF", "overlay binding_ref is not unique: "+binding.BindingRef)
		}
		seenAddition[binding.BindingRef] = struct{}{}
		byRef[binding.BindingRef] = binding
	}

	bindings := make([]model.DocArtifactBinding, 0, len(byRef))
	for _, binding := range byRef {
		if r.bindingInScope(binding) {
			bindings = append(bindings, binding)
		}
	}
	sort.SliceStable(bindings, func(i, j int) bool { return bindingLess(bindings[i], bindings[j]) })
	return bindings, nil
}

func (r *wikiCodeResolver) overlayBindings(overlay model.WikiCodeOverlay) []model.DocArtifactBinding {
	bindings := make([]model.DocArtifactBinding, 0, len(overlay.Additions))
	for _, binding := range overlay.Additions {
		binding = normalizeBinding(binding)
		if binding.BindingRef != "" && r.bindingInScope(binding) {
			bindings = append(bindings, binding)
		}
	}
	sort.SliceStable(bindings, func(i, j int) bool { return bindingLess(bindings[i], bindings[j]) })
	return bindings
}

func validateBindingDocumentIDs(bindings []model.DocArtifactBinding) error {
	pathsByID := make(map[string]string)
	for _, binding := range bindings {
		if !model.CanonicalWikiAuthority(binding.DocPath) || isRetiredWikiPath(binding.DocPath) || strings.EqualFold(strings.TrimSpace(binding.DocLifecycle), model.DocLifecycleRetired) {
			continue
		}
		id := strings.ToUpper(strings.TrimSpace(binding.DocID))
		if id == "" {
			continue
		}
		path := normalizeWikiPath(binding.DocPath)
		if previous, exists := pathsByID[id]; exists && previous != path {
			return model.NewWikiCodeContextError("GPH_WIKI_DUPLICATE_DOC_ID", "document ID "+id+" is declared by multiple active canonical paths: "+previous+", "+path)
		}
		pathsByID[id] = path
	}
	return nil
}

func (r *wikiCodeResolver) loadPersistedBindings(ctx context.Context) ([]model.DocArtifactBinding, error) {
	// Selecting all rows keeps explicit old ID/path lookups available for the
	// bounded retirement redirect while scope filtering remains local and
	// deterministic. T1's indexed queries remain available to future adapters.
	return store.ListDocArtifactBindings(ctx, r.db)
}

func (r *wikiCodeResolver) bindingInScope(binding model.DocArtifactBinding) bool {
	if r.req.Direction == model.WikiCodeDirectionCodeToWiki {
		targetPath := canonicalBridgePath(r.req.TargetPath)
		if targetPath == "" || canonicalBridgePath(binding.TargetPath) != targetPath {
			return false
		}
		if symbol := strings.TrimSpace(r.req.TargetSymbol); symbol != "" && strings.TrimSpace(binding.TargetSymbol) != symbol {
			return false
		}
		return true
	}
	selectors := resolverDocSelectors(r.req)
	if len(selectors) == 0 {
		return true
	}
	for _, selector := range selectors {
		selector = strings.TrimSpace(selector)
		if selector == "" {
			continue
		}
		if strings.Contains(selector, "#") {
			parts := strings.SplitN(selector, "#", 2)
			if normalizeWikiPath(binding.DocPath) == normalizeWikiPath(parts[0]) && binding.BlockID == parts[1] {
				return true
			}
		}
		if normalizeWikiPath(binding.DocPath) == normalizeWikiPath(selector) || strings.EqualFold(binding.DocID, selector) || binding.BlockID == selector {
			return true
		}
	}
	if block := strings.TrimSpace(r.req.CurrentBlockID); block != "" && binding.BlockID == block {
		return true
	}
	return false
}

func resolverDocSelectors(req WikiCodeResolveRequest) []string {
	values := make([]string, 0, len(req.DocSelectors)+len(req.DocIDs)+len(req.DocPaths)+len(req.BlockIDs))
	values = append(values, req.DocSelectors...)
	values = append(values, req.DocIDs...)
	values = append(values, req.DocPaths...)
	values = append(values, req.BlockIDs...)
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(strings.ReplaceAll(value, "\\", "/"))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeBinding(binding model.DocArtifactBinding) model.DocArtifactBinding {
	binding.DocPath = normalizeWikiPath(binding.DocPath)
	binding.TargetPath = normalizeWikiPath(binding.TargetPath)
	if binding.BindingRef == "" && binding.DocPath != "" && binding.TargetPath != "" {
		binding.BindingRef = model.WikiCodeBindingRef(binding.DocPath, binding.BlockID, binding.DocID, binding.Relation, binding.TargetPath, binding.TargetSymbol, binding.TargetKind)
	}
	if strings.TrimSpace(binding.BindingStatus) == "" {
		binding.BindingStatus = model.BindingStatusExact
	}
	if strings.TrimSpace(binding.DocLifecycle) == "" {
		binding.DocLifecycle = model.DocLifecycleActive
	}
	return binding
}

func bindingLess(left, right model.DocArtifactBinding) bool {
	leftKey := []string{
		fmt.Sprintf("%03d", model.WikiCodeRelationOrder(left.Relation)), normalizeWikiPath(left.DocPath), left.BlockID,
		fmt.Sprintf("%09d", left.StartLine), normalizeWikiPath(left.TargetPath), left.TargetSymbol, left.Role,
		left.BindingRef,
	}
	rightKey := []string{
		fmt.Sprintf("%03d", model.WikiCodeRelationOrder(right.Relation)), normalizeWikiPath(right.DocPath), right.BlockID,
		fmt.Sprintf("%09d", right.StartLine), normalizeWikiPath(right.TargetPath), right.TargetSymbol, right.Role,
		right.BindingRef,
	}
	return compareWikiStringTuple(leftKey, rightKey) < 0
}

func compareWikiStringTuple(left, right []string) int {
	for i := 0; i < len(left) && i < len(right); i++ {
		if left[i] < right[i] {
			return -1
		}
		if left[i] > right[i] {
			return 1
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func (r *wikiCodeResolver) resolveForward(ctx context.Context, bindings []model.DocArtifactBinding) {
	active := 0
	for _, binding := range bindings {
		if !r.bindingNavigable(binding) {
			r.handleHistoricalBinding(binding)
			continue
		}
		active++
		evidence, omission, candidates := r.resolveBinding(ctx, binding)
		if omission != nil {
			r.result.Omissions = append(r.result.Omissions, *omission)
		}
		if len(candidates) > 0 {
			r.result.Candidates = append(r.result.Candidates, candidates...)
		}
		if evidence.Status == "" {
			continue
		}
		if evidence.Relation == model.RelationTests || evidence.TargetKind == model.TargetKindTest {
			r.result.Tests = append(r.result.Tests, evidence)
		} else {
			r.result.DirectCode = append(r.result.DirectCode, evidence)
		}
		r.addClassification(reverseBindingClassification(binding))
	}
	if active > 0 && r.req.Direction == model.WikiCodeDirectionWikiToCode {
		r.resolveSupporting(ctx)
	}
	if active == 0 {
		r.addClassification(model.WikiCodeClassificationUnmappedChangedCode)
	}
}

func (r *wikiCodeResolver) bindingNavigable(binding model.DocArtifactBinding) bool {
	if !closedBindingRelation(binding.Relation) || !closedBindingTargetKind(binding.TargetKind) {
		return false
	}
	origin := strings.TrimSpace(binding.AuthoringOrigin)
	if origin != "" && !strings.EqualFold(origin, model.AuthoringOriginCanonical) && !strings.EqualFold(origin, model.AuthoringOriginLegacy) {
		return false
	}
	if !model.CanonicalWikiAuthority(binding.DocPath) || isRetiredWikiPath(binding.DocPath) {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(binding.BindingStatus), model.BindingStatusPlanned) {
		return false
	}
	status := strings.TrimSpace(binding.BindingStatus)
	if status != "" && !strings.EqualFold(status, model.BindingStatusExact) {
		return false
	}
	lifecycle := strings.TrimSpace(binding.DocLifecycle)
	return lifecycle == "" || strings.EqualFold(lifecycle, model.DocLifecycleActive)
}

func (r *wikiCodeResolver) bindingIsRetired(binding model.DocArtifactBinding) bool {
	return isRetiredWikiPath(binding.DocPath) || strings.EqualFold(strings.TrimSpace(binding.DocLifecycle), model.DocLifecycleRetired)
}

func closedBindingRelation(relation string) bool {
	switch strings.ToLower(strings.TrimSpace(relation)) {
	case model.RelationImplements, model.RelationTests, model.RelationConfigures, model.RelationOperates:
		return true
	default:
		return false
	}
}

func closedBindingTargetKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case model.TargetKindFile, model.TargetKindSymbol, model.TargetKindTest, model.TargetKindConfig:
		return true
	default:
		return false
	}
}

func bindingExclusionCode(binding model.DocArtifactBinding) string {
	if !closedBindingRelation(binding.Relation) || !closedBindingTargetKind(binding.TargetKind) {
		return "invalid_binding"
	}
	if strings.EqualFold(strings.TrimSpace(binding.BindingStatus), model.BindingStatusPlanned) {
		return "planned_binding_excluded"
	}
	return "retired_binding_rejected"
}

func (r *wikiCodeResolver) historicalRequested(binding model.DocArtifactBinding) bool {
	if r.req.ExplicitHistorical || r.req.AllowRetired || r.req.Historical {
		return true
	}
	for _, selector := range resolverDocSelectors(r.req) {
		if normalizeWikiPath(selector) == normalizeWikiPath(binding.DocPath) || strings.EqualFold(strings.TrimSpace(selector), strings.TrimSpace(binding.DocID)) {
			return true
		}
	}
	return false
}

func (r *wikiCodeResolver) handleHistoricalBinding(binding model.DocArtifactBinding) {
	if r.bindingIsRetired(binding) && r.historicalRequested(binding) && strings.TrimSpace(binding.SupersededBy) != "" {
		r.result.WikiContext = append(r.result.WikiContext, model.WikiCodeWikiContextItem{
			DocID: binding.DocID, Path: binding.DocPath, BlockID: binding.BlockID,
			Relation: binding.Relation, Role: binding.Role, BindingRef: binding.BindingRef,
			Status: "redirect", SupersededBy: binding.SupersededBy,
			StartLine: binding.StartLine, EndLine: binding.EndLine, Origin: "wiki_declared_path", AuthoringOrigin: binding.AuthoringOrigin,
		})
		return
	}
	code := bindingExclusionCode(binding)
	r.result.Omissions = append(r.result.Omissions, model.WikiCodeContextOmission{
		Code: code, Status: code, Source: binding.TargetPath, DocID: binding.DocID,
		DocPath: binding.DocPath, BlockID: binding.BlockID, BindingRef: binding.BindingRef,
		Reason: "binding is not active exact evidence",
	})
}

func (r *wikiCodeResolver) resolveBinding(ctx context.Context, binding model.DocArtifactBinding) (model.WikiCodeEvidence, *model.WikiCodeContextOmission, []model.WikiCodeEvidence) {
	binding = normalizeBinding(binding)
	evidence := model.WikiCodeEvidence{
		Path: binding.TargetPath, Symbol: binding.TargetSymbol, Kind: binding.TargetKind,
		ClaimStatus: model.GraphRecordExact,
		Origin: "wiki_declared_path", AuthoringOrigin: binding.AuthoringOrigin, ObservedOrigin: "",
		DocID: binding.DocID, DocPath: binding.DocPath, BlockID: binding.BlockID,
		Relation: binding.Relation, Role: binding.Role, BindingRef: binding.BindingRef,
		StartLine: binding.StartLine, EndLine: binding.EndLine, SourceDoc: binding.DocPath,
		SourceBlock: binding.BlockID, SourceLine: binding.StartLine, TargetKind: binding.TargetKind,
		Classification: model.WikiCodeClassificationDirectSDDBinding,
	}
	omission := func(status, reason string, candidates []string) *model.WikiCodeContextOmission {
		return &model.WikiCodeContextOmission{
			Code: status, Status: status, Source: binding.TargetPath, Reason: reason,
			Candidates: candidates, DocID: binding.DocID, DocPath: binding.DocPath,
			BlockID: binding.BlockID, BindingRef: binding.BindingRef,
		}
	}

	targetPath, err := normalizeTargetPath(binding.TargetPath)
	if err != nil {
		return model.WikiCodeEvidence{}, omission(model.WikiCodeStatusUnsafeTarget, "declared target path is not a safe repo-relative path", nil), nil
	}
	if targetPath == normalizeWikiPath(binding.DocPath) && (strings.TrimSpace(binding.TargetSymbol) == "" || strings.TrimSpace(binding.TargetSymbol) == strings.TrimSpace(binding.BlockID)) {
		return model.WikiCodeEvidence{}, omission(model.WikiCodeStatusSelfEdgeRejected, "self-export to the current document block is materialization metadata", nil), nil
	}
	if r.ignoreMatcher != nil {
		rootAbs, rootErr := filepath.Abs(r.root)
		if rootErr != nil || r.ignoreMatcher.ShouldIgnore(rootAbs, filepath.Join(rootAbs, filepath.FromSlash(targetPath))) {
			return model.WikiCodeEvidence{}, omission(model.WikiCodeStatusUnsafeTarget, "declared target is excluded by the workspace ignore policy", nil), nil
		}
	}
	r.result.Cost.FilesChecked++
	resolvedPath, exists, status := resolveSafeBridgeTarget(r.root, targetPath)
	if status == model.WikiCodeStatusUnsafeTarget {
		return model.WikiCodeEvidence{}, omission(status, "declared target escapes the workspace or follows an unsafe symlink", nil), nil
	}
	if !exists {
		candidates := r.lexicalBridgeCandidates(ctx, binding.TargetSymbol)
		return model.WikiCodeEvidence{}, omission(model.WikiCodeStatusMissingPath, "declared target path does not exist", evidenceStrings(candidates)), candidates
	}

	hashes, raced, readErr := hashBridgeTarget(resolvedPath)
	if raced {
		return model.WikiCodeEvidence{}, omission(model.WikiCodeStatusConcurrentChange, "target changed while its current hash was read", nil), nil
	}
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return model.WikiCodeEvidence{}, omission(model.WikiCodeStatusMissingPath, "declared target disappeared during resolution", nil), nil
		}
		return model.WikiCodeEvidence{}, omission(model.WikiCodeStatusConcurrentChange, "target could not be read as a current file", nil), nil
	}
	r.result.Cost.FilesHashed++
	evidence.TargetHash = hashes.sha256
	evidence.SourceDigest = hashes.sha256
	r.result.Cost.BytesRead += hashes.bytes

	if r.db != nil {
		r.result.Cost.CatalogQueries++
	}
	catalogHash, catalogFound, catalogErr := lookupCatalogFileHash(ctx, r.db, targetPath)
	if catalogErr != nil && strings.TrimSpace(binding.TargetSymbol) != "" {
		evidence.Kind = model.TargetKindFile
		evidence.Status = model.WikiCodeStatusResolvedFile
		evidence.ResolutionStatus = evidence.Status
		return evidence, omission(model.WikiCodeStatusCatalogUnavailable, "symbol resolution requires the catalog but it is unavailable", nil), nil
	}
	if catalogFound && catalogHash != "" && !bridgeHashMatches(catalogHash, hashes) {
		return model.WikiCodeEvidence{}, omission(model.WikiCodeStatusConcurrentChange, "target hash differs from the published catalog hash", nil), nil
	}
	if strings.TrimSpace(binding.TargetSymbol) == "" {
		evidence.Status = model.WikiCodeStatusResolvedFile
		evidence.ResolutionStatus = evidence.Status
		if catalogFound {
			evidence.ObservedOrigin = "catalog_observed"
		}
		return evidence, nil, nil
	}
	if r.db == nil {
		evidence.Kind = model.TargetKindFile
		evidence.Status = model.WikiCodeStatusResolvedFile
		evidence.ResolutionStatus = evidence.Status
		return evidence, omission(model.WikiCodeStatusCatalogUnavailable, "exact symbol resolution requires a catalog", nil), nil
	}

	r.result.Cost.CatalogQueries++
	symbols, symbolsErr := store.SymbolsByFile(ctx, r.db, targetPath, 512, 0)
	if symbolsErr != nil {
		evidence.Kind = model.TargetKindFile
		evidence.Status = model.WikiCodeStatusResolvedFile
		evidence.ResolutionStatus = evidence.Status
		return evidence, omission(model.WikiCodeStatusCatalogUnavailable, "symbol catalog query failed", nil), nil
	}
	r.result.Cost.SymbolsChecked += len(symbols)
	matched := exactBridgeSymbols(symbols, binding.TargetSymbol)
	if len(matched) == 1 {
		symbol := matched[0]
		if symbol.FileHash != "" && !bridgeHashMatches(symbol.FileHash, hashes) {
			return model.WikiCodeEvidence{}, omission(model.WikiCodeStatusConcurrentChange, "symbol belongs to a file whose catalog hash is no longer current", nil), nil
		}
		evidence.Status = model.WikiCodeStatusResolvedSymbol
		evidence.ResolutionStatus = evidence.Status
		// Keep the declared identity authoritative; catalog data only enriches
		// kind/language and current-hash evidence.
		evidence.Symbol = binding.TargetSymbol
		evidence.Kind = symbol.Kind
		evidence.Language = symbol.Language
		evidence.ObservedOrigin = "catalog_observed"
		return evidence, nil, nil
	}
	if len(matched) > 1 {
		labels := make([]string, 0, len(matched))
		candidateEvidence := make([]model.WikiCodeEvidence, 0, len(matched))
		for _, symbol := range matched {
			labels = append(labels, bridgeSymbolLabel(symbol))
			candidateEvidence = append(candidateEvidence, model.WikiCodeEvidence{
				Path: symbol.FilePath, Symbol: symbol.Name, Kind: symbol.Kind, Language: symbol.Language,
				Origin: "catalog_observed", ObservedOrigin: "catalog_observed", Status: model.WikiCodeStatusAmbiguousSymbol,
				ResolutionStatus: model.WikiCodeStatusAmbiguousSymbol,
			})
		}
		sort.Strings(labels)
		return model.WikiCodeEvidence{}, omission(model.WikiCodeStatusAmbiguousSymbol, "declared symbol has multiple exact catalog definitions", labels), candidateEvidence
	}

	candidates := r.lexicalBridgeCandidates(ctx, binding.TargetSymbol)
	// The file is current even when its declared symbol is missing. Preserve
	// the path as resolved_file and surface missing_symbol as a typed omission.
	evidence.Kind = model.TargetKindFile
	evidence.Status = model.WikiCodeStatusResolvedFile
	evidence.ResolutionStatus = evidence.Status
	return evidence, omission(model.WikiCodeStatusMissingSymbol, "declared file exists but the exact symbol is absent from the current catalog", evidenceStrings(candidates)), candidates
}

func (r *wikiCodeResolver) lexicalBridgeCandidates(ctx context.Context, symbol string) []model.WikiCodeEvidence {
	if r.db == nil || strings.TrimSpace(symbol) == "" {
		return nil
	}
	items, err := store.FindSymbols(ctx, r.db, symbol, "", false, 32, 0)
	if err != nil {
		return nil
	}
	r.result.Cost.CatalogQueries++
	candidates := make([]model.WikiCodeEvidence, 0, len(items))
	for _, item := range items {
		candidates = append(candidates, model.WikiCodeEvidence{
			Path: item.FilePath, Symbol: item.Name, Kind: item.Kind, Language: item.Language,
			Origin: "text_candidate", ObservedOrigin: "text_candidate", Status: model.WikiCodeStatusMissingSymbol,
			ResolutionStatus: model.WikiCodeStatusMissingSymbol,
		})
	}
	return candidates
}

func (r *wikiCodeResolver) resolveSupporting(ctx context.Context) {
	if r.graphState == model.GraphFreshnessStale {
		r.appendGraphOmission(model.WikiCodeStatusGraphStale, "supporting graph expansion is disabled for a stale graph")
		return
	}
	if r.graphState != model.GraphFreshnessCurrent || r.req.GraphSnapshot == nil {
		r.appendGraphOmission(model.WikiCodeStatusGraphUnavailable, "supporting graph expansion requires a current graph snapshot")
		return
	}

	seen := make(map[string]struct{})
	directItems := append(append([]model.WikiCodeEvidence{}, r.result.DirectCode...), r.result.Tests...)
	for _, direct := range directItems {
		seen[fmt.Sprintf("%s\x00%s", normalizeWikiPath(direct.Path), direct.Symbol)] = struct{}{}
	}
	for _, direct := range directItems {
		if direct.Status == model.WikiCodeStatusResolvedFile && strings.TrimSpace(direct.Symbol) != "" {
			// The file is current, but the declared symbol is unresolved;
			// do not expand a graph claim from that uncertain identity.
			continue
		}
		selectors := make([]string, 0, 2)
		if strings.TrimSpace(direct.Symbol) != "" {
			selectors = append(selectors, direct.Symbol)
		}
		selectors = append(selectors, direct.Path)
		var sourceNodes []model.GraphNodeRecord
		for _, selector := range selectors {
			nodes, _, err := r.req.GraphSnapshot.ResolveGraphSelector(ctx, selector)
			if err != nil {
				continue
			}
			r.result.Cost.GraphNodesVisited += len(nodes)
			for _, node := range nodes {
				if normalizeWikiPath(node.Identity.OwnerPath) == normalizeWikiPath(direct.Path) && (direct.Symbol == "" || node.Identity.SemanticIdentity == direct.Symbol || node.DisplayName == direct.Symbol) {
					sourceNodes = append(sourceNodes, node)
				}
			}
			if len(sourceNodes) > 0 {
				break
			}
		}
		for _, source := range sourceNodes {
			edges, err := r.req.GraphSnapshot.Edges(ctx, []int{source.NodeID}, "both", []string{"imports", "calls", "references", "tests"}, 64)
			if err != nil {
				continue
			}
			r.result.Cost.GraphEdgesVisited += len(edges)
			ids := make([]int, 0, len(edges))
			for _, edge := range edges {
				ids = append(ids, edge.ToNodeID)
				if edge.FromNodeID != source.NodeID {
					ids = append(ids, edge.FromNodeID)
				}
			}
			targets, err := r.req.GraphSnapshot.NodesByIDs(ctx, ids)
			if err != nil {
				continue
			}
			for _, edge := range edges {
				targetID := edge.ToNodeID
				if edge.ToNodeID == source.NodeID {
					targetID = edge.FromNodeID
				}
				target, ok := targets[targetID]
				if !ok || bridgeGraphSelfEdge(source, target, direct) {
					continue
				}
				path := normalizeWikiPath(target.Identity.OwnerPath)
				if path == "" {
					continue
				}
				symbol := target.Identity.SemanticIdentity
				baseKey := fmt.Sprintf("%s\x00%s", path, symbol)
				if _, exists := seen[baseKey]; exists {
					continue
				}
				key := fmt.Sprintf("%s\x00%s\x00%s", path, symbol, edge.Relation)
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				r.result.SupportingCode = append(r.result.SupportingCode, model.WikiCodeEvidence{
					Path: path, Symbol: symbol, Kind: target.Identity.SymbolKind, Language: target.Identity.Language,
					ClaimStatus: target.ClaimStatus, SourceDigest: target.SourceDigest.String(),
					Origin: "graph_observed", ObservedOrigin: "graph_observed", Status: "supporting",
					ResolutionStatus: "supporting", Relation: edge.Relation, Classification: model.WikiCodeClassificationSupportingCode,
				})
				r.result.GraphPaths = append(r.result.GraphPaths, model.WikiCodeGraphPath{
					From: direct.Path, To: path, Relation: edge.Relation, ClaimStatus: edge.ClaimStatus, EdgeRef: edge.CrossRID,
				})
			}
		}
	}
	if len(r.result.SupportingCode) > 0 {
		r.addClassification(model.WikiCodeClassificationSupportingCode)
	}
}

func bridgeGraphSelfEdge(source, target model.GraphNodeRecord, direct model.WikiCodeEvidence) bool {
	if source.NodeID == target.NodeID {
		return true
	}
	targetPath := normalizeWikiPath(target.Identity.OwnerPath)
	if targetPath == normalizeWikiPath(direct.Path) && (target.Identity.SemanticIdentity == "" || target.Identity.SemanticIdentity == direct.Symbol) {
		return true
	}
	return targetPath == normalizeWikiPath(direct.DocPath) && target.Identity.SemanticIdentity == direct.BlockID
}

func (r *wikiCodeResolver) appendGraphOmission(status, reason string) {
	for _, omission := range r.result.Omissions {
		if omission.Code == status {
			return
		}
	}
	r.result.Omissions = append(r.result.Omissions, model.WikiCodeContextOmission{
		Code: status, Status: status, Source: r.result.PrimaryDoc.Path, Reason: reason,
	})
}

func (r *wikiCodeResolver) resolveReverse(ctx context.Context, bindings []model.DocArtifactBinding) {
	targetPath, err := normalizeTargetPath(r.req.TargetPath)
	if err != nil {
		r.result.Omissions = append(r.result.Omissions, model.WikiCodeContextOmission{Code: model.WikiCodeStatusUnsafeTarget, Status: model.WikiCodeStatusUnsafeTarget, Source: r.req.TargetPath, Reason: "reverse target path is not repo-relative"})
		r.addClassification(model.WikiCodeClassificationUnmappedChangedCode)
		return
	}
	targetSymbol := strings.TrimSpace(r.req.TargetSymbol)
	if classify := bridgePathClassification(targetPath); classify != "" {
		r.addClassification(classify)
	}
	for _, binding := range bindings {
		if canonicalBridgePath(binding.TargetPath) != targetPath {
			continue
		}
		if targetSymbol != "" && strings.TrimSpace(binding.TargetSymbol) != targetSymbol {
			continue
		}
		if !r.bindingNavigable(binding) {
			if r.bindingIsRetired(binding) && r.historicalRequested(binding) && binding.SupersededBy != "" {
				r.result.WikiContext = append(r.result.WikiContext, model.WikiCodeWikiContextItem{DocID: binding.DocID, Path: binding.DocPath, BlockID: binding.BlockID, Relation: binding.Relation, Role: binding.Role, BindingRef: binding.BindingRef, Status: "redirect", SupersededBy: binding.SupersededBy, StartLine: binding.StartLine, EndLine: binding.EndLine, Origin: "wiki_declared_path", AuthoringOrigin: binding.AuthoringOrigin})
			} else {
				code := bindingExclusionCode(binding)
				r.result.Omissions = append(r.result.Omissions, model.WikiCodeContextOmission{Code: code, Status: code, Source: binding.TargetPath, DocID: binding.DocID, DocPath: binding.DocPath, BlockID: binding.BlockID, BindingRef: binding.BindingRef, Reason: "binding is not active exact evidence"})
			}
			continue
		}
		resolutionStatus := model.WikiCodeStatusResolvedFile
		if strings.TrimSpace(binding.TargetSymbol) != "" {
			resolutionStatus = model.WikiCodeStatusResolvedSymbol
		}
		item := model.WikiCodeWikiContextItem{
			DocID: binding.DocID, Path: binding.DocPath, BlockID: binding.BlockID, Relation: binding.Relation,
			Role: binding.Role, BindingRef: binding.BindingRef, Status: resolutionStatus, StartLine: binding.StartLine,
			EndLine: binding.EndLine, Origin: "wiki_declared_path", AuthoringOrigin: binding.AuthoringOrigin,
			Classification: reverseBindingClassification(binding),
		}
		item.Parents = r.linkedDocumentaryParents(ctx, binding.DocPath)
		r.result.WikiContext = append(r.result.WikiContext, item)
		r.addClassification(item.Classification)
	}
	if len(r.result.WikiContext) == 0 {
		r.addClassification(model.WikiCodeClassificationUnmappedChangedCode)
		r.result.NextQueries = append(r.result.NextQueries,
			"nav related "+targetPath,
		)
		if targetSymbol != "" {
			r.result.NextQueries = append(r.result.NextQueries, "nav related "+targetSymbol)
		}
	}
}

func (r *wikiCodeResolver) linkedDocumentaryParents(ctx context.Context, docPath string) []model.WikiCodeWikiContextItem {
	if r.db == nil {
		return nil
	}
	// Documentary parents can point to the declaring RF from either direction
	// (for example, an FL may link forward to its RF). Read both endpoints from
	// the existing doc graph; never infer a parent from the code target.
	rows, err := store.QueryContextWithRetry(ctx, r.db, `
		SELECT from_path, to_path, to_doc_id, kind, label
		FROM doc_edges
		WHERE from_path = ? OR to_path = ?
		ORDER BY kind ASC, from_path ASC, to_path ASC, to_doc_id ASC
		LIMIT 64
	`, docPath, docPath)
	if err != nil {
		return nil
	}
	defer rows.Close()
	parents := make([]model.WikiCodeWikiContextItem, 0, 8)
	seen := make(map[string]struct{})
	for rows.Next() {
		var edge model.DocEdge
		if err := rows.Scan(&edge.FromPath, &edge.ToPath, &edge.ToDocID, &edge.Kind, &edge.Label); err != nil {
			return nil
		}
		candidatePath := edge.ToPath
		if normalizeWikiPath(edge.ToPath) == normalizeWikiPath(docPath) {
			candidatePath = edge.FromPath
		}
		doc, ok := r.docsByPath[normalizeWikiPath(candidatePath)]
		if !ok && normalizeWikiPath(candidatePath) == normalizeWikiPath(edge.ToPath) && strings.TrimSpace(edge.ToDocID) != "" {
			items := r.docsByID[strings.ToUpper(strings.TrimSpace(edge.ToDocID))]
			if len(items) == 1 {
				doc, ok = items[0], true
			}
		}
		if !ok || normalizeWikiPath(doc.Path) == normalizeWikiPath(docPath) || !canonicalActiveWikiPath(doc.Path, doc.IsSnapshot) || !isRFOrFLDoc(doc) {
			continue
		}
		path := normalizeWikiPath(doc.Path)
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		parents = append(parents, model.WikiCodeWikiContextItem{DocID: doc.DocID, Path: doc.Path, Relation: edge.Kind, Status: "linked", Origin: "doc_edge"})
		if len(parents) >= 32 {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	sort.SliceStable(parents, func(i, j int) bool {
		return compareWikiStringTuple([]string{parents[i].Path, parents[i].DocID, parents[i].Relation}, []string{parents[j].Path, parents[j].DocID, parents[j].Relation}) < 0
	})
	return parents
}

func reverseBindingClassification(binding model.DocArtifactBinding) string {
	switch binding.Relation {
	case model.RelationImplements, model.RelationTests:
		return model.WikiCodeClassificationDirectSDDBinding
	case model.RelationConfigures, model.RelationOperates:
		return model.WikiCodeClassificationSharedTechnical
	default:
		return model.WikiCodeClassificationUnmappedChangedCode
	}
}

func (r *wikiCodeResolver) setClassification() {
	resolvedWikiContext := false
	for _, item := range r.result.WikiContext {
		if item.Status != "redirect" {
			resolvedWikiContext = true
			break
		}
	}
	if len(r.result.Classifications) == 0 {
		if len(r.result.DirectCode) == 0 && len(r.result.Tests) == 0 && !resolvedWikiContext {
			r.result.Classifications = []string{model.WikiCodeClassificationUnmappedChangedCode}
		} else {
			r.result.Classifications = []string{model.WikiCodeClassificationDirectSDDBinding}
		}
	}
	for _, item := range append(append(append([]model.WikiCodeEvidence{}, r.result.DirectCode...), r.result.Tests...), append(r.result.SupportingCode, r.result.Candidates...)...) {
		if classify := bridgePathClassification(item.Path); classify != "" {
			r.addClassification(classify)
		}
	}
	if r.result.Classification == "" && len(r.result.Classifications) > 0 {
		r.result.Classification = r.result.Classifications[0]
	}
	if r.result.Classification == "" {
		r.result.Classification = model.WikiCodeClassificationUnmappedChangedCode
	}
}

func (r *wikiCodeResolver) addClassification(classification string) {
	switch classification {
	case model.WikiCodeClassificationDirectSDDBinding,
		model.WikiCodeClassificationSupportingCode,
		model.WikiCodeClassificationSharedTechnical,
		model.WikiCodeClassificationGeneratedOrVendor,
		model.WikiCodeClassificationMechanicalNoImpact,
		model.WikiCodeClassificationUnmappedChangedCode:
	default:
		return
	}
	for _, existing := range r.result.Classifications {
		if existing == classification {
			return
		}
	}
	r.result.Classifications = append(r.result.Classifications, classification)
	sort.SliceStable(r.result.Classifications, func(i, j int) bool { return classificationOrder(r.result.Classifications[i]) < classificationOrder(r.result.Classifications[j]) })
}

func classificationOrder(value string) int {
	switch value {
	case model.WikiCodeClassificationDirectSDDBinding:
		return 10
	case model.WikiCodeClassificationSupportingCode:
		return 20
	case model.WikiCodeClassificationSharedTechnical:
		return 30
	case model.WikiCodeClassificationGeneratedOrVendor:
		return 40
	case model.WikiCodeClassificationMechanicalNoImpact:
		return 50
	case model.WikiCodeClassificationUnmappedChangedCode:
		return 60
	default:
		return 100
	}
}

func bridgePathClassification(path string) string {
	lower := strings.ToLower(normalizeWikiPath(path))
	for _, marker := range []string{"/generated/", "/vendor/", "/bin/", "/obj/"} {
		if strings.Contains("/"+lower+"/", marker) {
			return model.WikiCodeClassificationGeneratedOrVendor
		}
	}
	for _, name := range []string{".gitattributes", ".gitignore", ".editorconfig", "package-lock.json", "pnpm-lock.yaml", "yarn.lock"} {
		if strings.HasSuffix(lower, name) {
			return model.WikiCodeClassificationMechanicalNoImpact
		}
	}
	return ""
}

func applyWikiCodeResolverBounds(result *model.WikiCodeContext, limit int, cursor, direction string) {
	if result == nil || limit < 1 {
		return
	}
	remaining := limit
	truncated := false
	trim := func(items *[]model.WikiCodeEvidence) {
		if len(*items) > remaining {
			*items = (*items)[:remaining]
			remaining = 0
			truncated = true
			return
		}
		remaining -= len(*items)
	}
	trim(&result.DirectCode)
	trim(&result.Tests)
	trim(&result.SupportingCode)
	trim(&result.Candidates)
	if len(result.WikiContext) > remaining {
		result.WikiContext = result.WikiContext[:remaining]
		remaining = 0
		truncated = true
	} else {
		remaining -= len(result.WikiContext)
	}
	if truncated {
		result.Truncated = true
		result.NextCursor = fmt.Sprintf("%s:%d", direction, limit)
		result.Continuation = &model.WikiCodeContextContinuation{Direction: direction, Cursor: result.NextCursor, Remaining: len(result.DirectCode) + len(result.Tests) + len(result.SupportingCode) + len(result.Candidates) + len(result.WikiContext)}
	} else if strings.TrimSpace(cursor) != "" {
		result.NextCursor = cursor
	}
}

func applyWikiCodeResolverBudget(result *model.WikiCodeContext, direction string) {
	if result == nil || result.TokenBudget < 1 {
		return
	}
	mandatory := struct {
		Direct []model.WikiCodeEvidence `json:"direct_code"`
		Tests  []model.WikiCodeEvidence `json:"tests"`
	}{result.DirectCode, result.Tests}
	mandatoryBytes, _ := json.Marshal(mandatory)
	used := (len(mandatoryBytes) + 3) / 4
	remaining := result.TokenBudget - used
	for _, items := range []*[]model.WikiCodeEvidence{&result.SupportingCode, &result.Candidates} {
		for len(*items) > 0 {
			b, _ := json.Marshal(*items)
			cost := (len(b) + 3) / 4
			if cost <= remaining {
				remaining -= cost
				break
			}
			*items = (*items)[:len(*items)-1]
			result.Truncated = true
		}
	}
	for len(result.WikiContext) > 0 {
		b, _ := json.Marshal(result.WikiContext)
		cost := (len(b) + 3) / 4
		if cost <= remaining {
			remaining -= cost
			break
		}
		result.WikiContext = result.WikiContext[:len(result.WikiContext)-1]
		result.Truncated = true
	}
	for len(result.GraphPaths) > 0 {
		b, _ := json.Marshal(result.GraphPaths)
		cost := (len(b) + 3) / 4
		if cost <= remaining {
			remaining -= cost
			break
		}
		result.GraphPaths = result.GraphPaths[:len(result.GraphPaths)-1]
		result.Truncated = true
	}
	if used > result.TokenBudget {
		result.Truncated = true
	}
	result.TokenUsed = result.TokenBudget - remaining
	if result.TokenUsed < used {
		result.TokenUsed = used
	}
	if result.Truncated && result.NextCursor == "" {
		result.NextCursor = fmt.Sprintf("%s:budget", direction)
		result.Continuation = &model.WikiCodeContextContinuation{Direction: direction, Cursor: result.NextCursor}
	}
}

func evidenceStrings(items []model.WikiCodeEvidence) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, bridgeSymbolLabel(model.SymbolRecord{FilePath: item.Path, Name: item.Symbol}))
	}
	sort.Strings(out)
	return out
}

func exactBridgeSymbols(items []model.SymbolRecord, target string) []model.SymbolRecord {
	target = strings.TrimSpace(target)
	matched := make([]model.SymbolRecord, 0)
	for _, item := range items {
		if item.Name == target || item.QualifiedName == target {
			matched = append(matched, item)
		}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		return compareWikiStringTuple([]string{matched[i].FilePath, matched[i].QualifiedName, matched[i].Name, fmt.Sprintf("%09d", matched[i].StartLine)}, []string{matched[j].FilePath, matched[j].QualifiedName, matched[j].Name, fmt.Sprintf("%09d", matched[j].StartLine)}) < 0
	})
	return matched
}

func bridgeSymbolLabel(item model.SymbolRecord) string {
	if strings.TrimSpace(item.QualifiedName) != "" {
		return item.FilePath + "#" + item.QualifiedName
	}
	return item.FilePath + "#" + item.Name
}

func normalizeWikiPath(path string) string {
	return strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
}

func canonicalBridgePath(path string) string {
	if normalized, err := normalizeTargetPath(path); err == nil {
		return normalized
	}
	return normalizeWikiPath(path)
}

func normalizeTargetPath(path string) (string, error) {
	path = normalizeWikiPath(path)
	if path == "" || strings.ContainsAny(path, "\x00\r\n") || filepath.IsAbs(path) || strings.HasPrefix(path, "/") || (len(path) > 1 && path[1] == ':') {
		return "", errors.New("unsafe target path")
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	if clean == "." || clean == "" || strings.HasPrefix(clean, "../") || clean == ".." || strings.Contains(clean, "/../") || strings.HasSuffix(clean, "/..") {
		return "", errors.New("unsafe target path")
	}
	return clean, nil
}

func resolveSafeBridgeTarget(root, relative string) (string, bool, string) {
	if strings.TrimSpace(root) == "" {
		return "", false, model.WikiCodeStatusUnsafeTarget
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", false, model.WikiCodeStatusUnsafeTarget
	}
	rootEval, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		rootEval = filepath.Clean(rootAbs)
	}
	candidate := filepath.Join(rootAbs, filepath.FromSlash(relative))
	if !bridgePathWithin(rootAbs, candidate) {
		return "", false, model.WikiCodeStatusUnsafeTarget
	}
	probe := candidate
	for {
		info, statErr := os.Lstat(probe)
		if statErr == nil {
			evaluated, evalErr := filepath.EvalSymlinks(probe)
			if evalErr != nil {
				return "", false, model.WikiCodeStatusUnsafeTarget
			}
			if !bridgePathWithin(rootEval, evaluated) {
				return "", false, model.WikiCodeStatusUnsafeTarget
			}
			evaluatedInfo, evaluatedStatErr := os.Stat(evaluated)
			if evaluatedStatErr != nil {
				return "", false, model.WikiCodeStatusUnsafeTarget
			}
			if !info.Mode().IsRegular() && (info.Mode()&os.ModeSymlink) != os.ModeSymlink {
				return "", false, model.WikiCodeStatusMissingPath
			}
			if !evaluatedInfo.Mode().IsRegular() {
				return "", false, model.WikiCodeStatusMissingPath
			}
			return evaluated, true, ""
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", false, model.WikiCodeStatusUnsafeTarget
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return candidate, false, model.WikiCodeStatusMissingPath
		}
		probe = parent
		if !bridgePathWithin(rootAbs, probe) {
			return "", false, model.WikiCodeStatusUnsafeTarget
		}
	}
}

func bridgePathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return false
	}
	return true
}

type bridgeTargetHashes struct {
	md5    string
	sha1   string
	sha256 string
	bytes  int64
}

func hashBridgeTarget(path string) (bridgeTargetHashes, bool, error) {
	var zero bridgeTargetHashes
	for attempt := 1; attempt <= 2; attempt++ {
		before, err := os.Stat(path)
		if err != nil {
			return zero, false, err
		}
		md5Hash := md5.New()
		sha1Hash := sha1.New()
		sha256Hash := sha256.New()
		file, err := os.Open(path)
		if err != nil {
			return zero, false, err
		}
		bytesRead, copyErr := io.Copy(io.MultiWriter(md5Hash, sha1Hash, sha256Hash), file)
		closeErr := file.Close()
		if copyErr != nil {
			return zero, false, copyErr
		}
		if closeErr != nil {
			return zero, false, closeErr
		}
		after, err := os.Stat(path)
		if err != nil {
			return zero, false, err
		}
		if before.Size() == after.Size() && before.ModTime().Equal(after.ModTime()) {
			return bridgeTargetHashes{
				md5: hex.EncodeToString(md5Hash.Sum(nil)), sha1: hex.EncodeToString(sha1Hash.Sum(nil)), sha256: hex.EncodeToString(sha256Hash.Sum(nil)), bytes: bytesRead,
			}, false, nil
		}
		if attempt == 2 {
			return zero, true, nil
		}
	}
	return zero, true, nil
}

func lookupCatalogFileHash(ctx context.Context, db *sql.DB, path string) (string, bool, error) {
	if db == nil {
		return "", false, model.NewWikiCodeContextError(model.WikiCodeStatusCatalogUnavailable, "catalog handle is unavailable")
	}
	rows, err := db.QueryContext(ctx, "SELECT content_hash FROM files WHERE file_path=? ORDER BY repo_id, repo_name LIMIT 2", path)
	if err != nil {
		return "", false, err
	}
	defer rows.Close()
	var hashes []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return "", false, err
		}
		hashes = append(hashes, value)
	}
	if err := rows.Err(); err != nil {
		return "", false, err
	}
	if len(hashes) == 0 {
		return "", false, nil
	}
	if len(hashes) > 1 && hashes[0] != hashes[1] {
		return "", false, model.NewWikiCodeContextError("GPH_WIKI_CATALOG_AMBIGUOUS", "target path has multiple catalog hashes")
	}
	return hashes[0], true, nil
}

func bridgeHashMatches(expected string, hashes bridgeTargetHashes) bool {
	expected = strings.ToLower(strings.TrimSpace(expected))
	return expected != "" && (expected == hashes.md5 || expected == hashes.sha1 || expected == hashes.sha256 || expected == "sha256:"+hashes.sha256 || expected == "sha1:"+hashes.sha1 || expected == "md5:"+hashes.md5)
}

func tombstoneFields(value any) (string, string, string) {
	return reflectedStringField(value, "BindingRef", "Ref"), normalizeWikiPath(reflectedStringField(value, "DocPath", "OwnerDocPath", "OwnerPath", "Path")), reflectedStringField(value, "BlockID", "OwnerBlockID", "OwnerBlock", "Block")
}

func reflectedStringField(value any, names ...string) string {
	if value == nil {
		return ""
	}
	rv := reflect.ValueOf(value)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return ""
		}
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Struct {
		for _, name := range names {
			field := rv.FieldByName(name)
			if field.IsValid() && field.Kind() == reflect.String {
				return strings.TrimSpace(field.String())
			}
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	var object map[string]any
	if json.Unmarshal(encoded, &object) != nil {
		return ""
	}
	for _, name := range names {
		for key, raw := range object {
			if strings.EqualFold(strings.ReplaceAll(key, "_", ""), strings.ReplaceAll(name, "_", "")) {
				if text, ok := raw.(string); ok {
					return strings.TrimSpace(text)
				}
			}
		}
	}
	return ""
}

func setWikiFreshnessDomain(freshness *model.WikiCodeFreshness, domain, status string) {
	if freshness == nil {
		return
	}
	value := reflect.ValueOf(freshness)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return
	}
	value = value.Elem()
	setFreshnessReflectValue(value, domain, status)
}

func setFreshnessReflectValue(value reflect.Value, domain, status string) bool {
	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.Kind() == reflect.Interface {
			if value.IsNil() {
				if !value.CanSet() {
					return false
				}
				value.Set(reflect.ValueOf(status))
				return true
			}
			value = value.Elem()
			continue
		}
		if value.IsNil() {
			if !value.CanSet() {
				return false
			}
			value.Set(reflect.New(value.Type().Elem()))
		}
		value = value.Elem()
	}
	if value.Kind() == reflect.Map && value.Type().Key().Kind() == reflect.String {
		if value.IsNil() && value.CanSet() {
			value.Set(reflect.MakeMap(value.Type()))
		}
		if value.CanSet() || !value.IsNil() {
			item := reflect.ValueOf(status)
			if item.Type().AssignableTo(value.Type().Elem()) {
				value.SetMapIndex(reflect.ValueOf(domain).Convert(value.Type().Key()), item)
				return true
			}
			if item.Type().ConvertibleTo(value.Type().Elem()) {
				value.SetMapIndex(reflect.ValueOf(domain).Convert(value.Type().Key()), item.Convert(value.Type().Elem()))
				return true
			}
			if value.Type().Elem().Kind() == reflect.Struct {
				entry := reflect.New(value.Type().Elem()).Elem()
				if setFreshnessReflectString(entry, status) {
					value.SetMapIndex(reflect.ValueOf(domain).Convert(value.Type().Key()), entry)
					return true
				}
			}
		}
		return false
	}
	if value.Kind() != reflect.Struct {
		if value.Kind() == reflect.String && value.CanSet() {
			value.SetString(status)
			return true
		}
		return false
	}
	fieldNames := []string{domain, domain + "_status"}
	parts := strings.Split(domain, "_")
	pascal := ""
	for _, part := range parts {
		if part != "" {
			pascal += strings.ToUpper(part[:1]) + part[1:]
		}
	}
	fieldNames = append(fieldNames, pascal, pascal+"Status")
	for _, name := range fieldNames {
		field := value.FieldByName(name)
		if field.IsValid() && field.CanSet() {
			if setFreshnessReflectString(field, status) {
				return true
			}
		}
	}
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		fieldType := value.Type().Field(i)
		jsonName := strings.Split(fieldType.Tag.Get("json"), ",")[0]
		if strings.EqualFold(strings.ReplaceAll(jsonName, "_", ""), strings.ReplaceAll(domain, "_", "")) || strings.EqualFold(fieldType.Name, pascal) {
			if setFreshnessReflectString(field, status) {
				return true
			}
		}
		if strings.EqualFold(fieldType.Name, "Domains") || strings.EqualFold(jsonName, "domains") {
			if setFreshnessReflectValue(field, domain, status) {
				return true
			}
		}
	}
	return false
}

func setFreshnessReflectString(value reflect.Value, status string) bool {
	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.Kind() == reflect.Interface {
			if value.IsNil() {
				if !value.CanSet() {
					return false
				}
				value.Set(reflect.ValueOf(status))
				return true
			}
			value = value.Elem()
			continue
		}
		if value.IsNil() {
			if !value.CanSet() {
				return false
			}
			value.Set(reflect.New(value.Type().Elem()))
		}
		value = value.Elem()
	}
	if value.Kind() == reflect.String && value.CanSet() {
		value.SetString(status)
		return true
	}
	if value.Kind() == reflect.Struct {
		for _, name := range []string{"Status", "State", "Value"} {
			field := value.FieldByName(name)
			if field.IsValid() && field.CanSet() && setFreshnessReflectString(field, status) {
				return true
			}
		}
	}
	return false
}

func canonicalActiveWikiPath(path string, snapshot bool) bool {
	return model.CanonicalWikiAuthority(path) && !snapshot && !isRetiredWikiPath(path)
}

func isRetiredWikiPath(path string) bool {
	path = normalizeWikiPath(path)
	return strings.HasPrefix(path, ".docs/wiki/_retired/") || strings.Contains(path, "/_retired/")
}

func isRFOrFLDoc(doc model.DocRecord) bool {
	family := strings.ToLower(strings.TrimSpace(doc.Family))
	if family == "rf" || family == "fl" || family == "functional" {
		return strings.Contains(strings.ToLower(doc.Path), "/04_rf/") || strings.Contains(strings.ToLower(doc.Path), "/03_fl/") || family == "rf" || family == "fl"
	}
	lower := strings.ToLower(normalizeWikiPath(doc.Path))
	return strings.Contains(lower, "/04_rf/") || strings.Contains(lower, "/03_fl/")
}

