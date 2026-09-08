package service

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/docidentity"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/nav"
	"github.com/fgpaz/mi-lsp/internal/store"
)

func (a *App) wikiSearch(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	if blockedEnv, err := a.governanceGateEnvelope(ctx, request, "nav.wiki.search"); err != nil {
		return model.Envelope{}, err
	} else if blockedEnv != nil {
		return *blockedEnv, nil
	}

	// Check if --all-workspaces mode is requested
	allWorkspaces, _ := request.Payload["all_workspaces"].(bool)
	if allWorkspaces {
		return a.wikiSearchAllWorkspaces(ctx, request)
	}

	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}
	memory, _ := loadReentryMemory(ctx, registration.Root)

	queryText := strings.TrimSpace(firstNonEmpty(
		stringPayload(request.Payload, "query"),
		stringPayload(request.Payload, "pattern"),
		stringPayload(request.Payload, "task"),
	))
	if queryText == "" {
		return model.Envelope{}, fmt.Errorf("query is required")
	}

	query := loadDocQueryContext(ctx, registration, queryText)
	defer query.Close()
	if query.dbErr != nil {
		return model.Envelope{}, query.dbErr
	}

	warnings := append([]string{}, query.profileWarnings...)
	warnings = append(warnings, fmt.Sprintf("read_model=%s", query.profileSource))
	if len(query.docs) == 0 {
		hint := fmt.Sprintf("documentation index is empty; rerun 'mi-lsp index --workspace %s --docs-only' before wiki search", registration.Name)
		warnings = appendStringIfMissing(warnings, hint)
		env := model.Envelope{
			Ok:        true,
			Workspace: registration.Name,
			Backend:   "wiki.search",
			Items:     []map[string]any{},
			Warnings:  warnings,
			Hint:      hint,
		}
		return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
	}

	layerFilter, unknownLayers := parseWikiLayerFilter(stringPayload(request.Payload, "layer"))
	for _, layer := range unknownLayers {
		warnings = appendStringIfMissing(warnings, fmt.Sprintf("unknown wiki layer %q ignored; valid layers: RS, RF, FL, TP, CT, TECH, DB", layer))
	}

	topFromPayload := intFromAny(request.Payload["top"], 0)
	offset := intFromAny(request.Payload["offset"], request.Context.Offset)
	includeContent, _ := request.Payload["include_content"].(bool)

	exactDocs, err := store.FindDocRecordsBySourceID(ctx, query.db, queryText)
	if err != nil {
		return model.Envelope{}, err
	}
	identitySnapshotCurrent, err := store.DocIdentitySnapshotCurrent(ctx, query.db)
	if err != nil {
		return model.Envelope{}, err
	}
	if !identitySnapshotCurrent {
		warnings = appendStringIfMissing(warnings, "legacy doc_id mentions are unverified; only source-confirmed references are eligible")
	}
	ranked := append([]scoredDoc(nil), query.ranked...)
	mentionCandidates, mentionWarnings, mentionErr := wikiSearchExactMentionCandidatesWithWarnings(ctx, query.db, registration.Root, queryText)
	if mentionErr != nil {
		return model.Envelope{}, mentionErr
	}
	for _, warning := range mentionWarnings {
		warnings = appendStringIfMissing(warnings, warning)
	}
	ranked = append(ranked, mentionCandidates...)
	eligible := orderWikiSearchCandidates(queryText, exactDocs, ranked, layerFilter)
	for _, warning := range wikiSearchAmbiguityWarnings(eligible) {
		warnings = appendStringIfMissing(warnings, warning)
	}
	top := topFromPayload
	if top <= 0 {
		if request.Context.Full {
			top = len(eligible)
		} else {
			top = request.Context.MaxItems
		}
	}
	if top <= 0 {
		top = 10
	}

	selected := wikiSearchCandidatePage(eligible, offset, top)
	candidates := make([]model.WikiSearchResult, 0, len(selected))
	for _, candidate := range selected {
		layer := wikiLayerForDoc(candidate.record)
		candidates = append(candidates, wikiSearchResult(registration.Name, registration.Root, queryText, candidate, layer, includeContent, request.Context.MaxChars))
	}
	totalMatches := len(eligible)
	nextHint := wikiExpansionHint("nav.wiki.search", registration.Name, queryText, len(candidates), totalMatches)
	sourceIdentity, hasSourceIdentity, _ := sourceIdentityForQuery(ctx, query.db, queryText)
	for i := range candidates {
		status := wikiLookupStatusForDoc(registration.Name, queryText, model.DocRecord{
			Path:   candidates[i].Path,
			Title:  candidates[i].Title,
			DocID:  candidates[i].DocID,
			Layer:  candidates[i].Layer,
			Family: candidates[i].Family,
		}, candidates[i].Why, totalMatches, len(candidates), nextHint)
		if hasSourceIdentity && candidates[i].Path == sourceIdentity.path {
			applySourceIdentity(&status, sourceIdentity)
		}
		candidates[i].LookupStatus = &status
	}

	hint := ""
	if len(candidates) == 0 {
		if len(layerFilter) > 0 {
			hint = "0 wiki matches for selected layers; broaden --layer or try nav wiki route"
		} else {
			hint = "0 wiki matches; try a doc id like RS-*, RF-*, FL-*, CT-*, TECH-*, DB-*, AE-* or run nav wiki route"
		}
	}

	env := model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "wiki.search",
		Items:     candidates,
		Warnings:  warnings,
		Hint:      hint,
		Stats:     model.Stats{Files: len(candidates)},
	}
	if nextHint != "" {
		env.NextHint = &nextHint
	}
	return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
}

type wikiSearchEvidence struct {
	line      int
	startLine int
	endLine   int
	text      string
}

func wikiSearchLineEvidence(root string, relPath string, queryText string) (wikiSearchEvidence, bool) {
	content, ok := wikiSearchReadSafeSource(root, relPath)
	if !ok {
		return wikiSearchEvidence{}, false
	}
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 {
		return wikiSearchEvidence{}, false
	}
	tokens := docgraph.QuestionTokens(queryText)
	normalizedQuery := normalizeWikiSearchNaturalText(queryText)
	metadataLines := wikiSearchMetadataLines(lines)
	lineNo := 0
	bestScore := -1
	for i, line := range lines {
		score := wikiSearchEvidenceLineScoreWithMetadata(line, tokens, normalizedQuery, metadataLines[i])
		if score > bestScore {
			bestScore = score
			lineNo = i
		}
	}
	if bestScore <= 0 {
		return wikiSearchEvidence{}, false
	}
	startLine := max(0, lineNo-1)
	endLine := min(len(lines)-1, lineNo+1)
	parts := make([]string, 0, endLine-startLine+1)
	for _, line := range lines[startLine : endLine+1] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts = append(parts, line)
	}
	if len(parts) == 0 {
		parts = append(parts, strings.TrimSpace(lines[lineNo]))
	}
	text := strings.Join(parts, "\n")
	if len(text) > 480 {
		text = text[:480]
	}
	return wikiSearchEvidence{line: lineNo + 1, startLine: startLine + 1, endLine: endLine + 1, text: text}, true
}

func wikiSearchEvidenceLineScore(line string, tokens []string, normalizedQuery string) int {
	return wikiSearchEvidenceLineScoreWithMetadata(line, tokens, normalizedQuery, false)
}

func wikiSearchEvidenceLineScoreWithMetadata(line string, tokens []string, normalizedQuery string, metadata bool) int {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return 0
	}
	normalized := normalizeWikiSearchNaturalText(trimmed)
	score := 0
	coverage := 0
	for _, token := range tokens {
		if wikiSearchHasLexicalToken(normalized, token) {
			coverage++
		}
	}
	if coverage > 0 {
		score += coverage * 100
		// Prefer a useful prose passage over an equally-covered heading or
		// metadata line; headings are still retained as a neighboring line.
		if !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "---") && !strings.HasPrefix(trimmed, "```") {
			score += 30
		}
	}
	if normalizedQuery != "" && strings.Contains(normalized, normalizedQuery) {
		score += 20
	}
	if strings.HasPrefix(trimmed, "---") || strings.HasPrefix(trimmed, "```") ||
		strings.HasPrefix(normalized, "doc id ") || strings.HasPrefix(normalized, "id ") ||
		strings.HasPrefix(normalized, "imports ") || strings.HasPrefix(normalized, "exports ") ||
		strings.HasPrefix(normalized, "harness protocol ") {
		score -= 45
	}
	if metadata && coverage > 0 {
		// Frontmatter and the Harness header are useful fallback metadata, but
		// should lose to a matching body passage whenever one exists.
		score -= 110
		if score < 1 {
			score = 1
		}
	}
	return score
}

func wikiSearchMetadataLines(lines []string) []bool {
	metadata := make([]bool, len(lines))
	if len(lines) == 0 {
		return metadata
	}
	first := 0
	for first < len(lines) && strings.TrimSpace(lines[first]) == "" {
		first++
	}
	if first < len(lines) && strings.TrimSpace(lines[first]) == "---" {
		for i := first; i < len(lines); i++ {
			metadata[i] = true
			if i > first && strings.TrimSpace(lines[i]) == "---" {
				break
			}
		}
	}

	inFence := false
	fenceStart := -1
	harnessFence := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inFence {
			if !strings.HasPrefix(trimmed, "```") {
				continue
			}
			language := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trimmed, "```")))
			inFence = true
			fenceStart = i
			harnessFence = language == "yaml" || language == "yml"
			continue
		}
		if harnessFence && strings.HasPrefix(trimmed, "harness_protocol:") {
			for j := fenceStart; j <= i; j++ {
				metadata[j] = true
			}
		}
		if strings.HasPrefix(trimmed, "```") {
			if harnessFence {
				for j := fenceStart; j <= i; j++ {
					metadata[j] = true
				}
			}
			inFence = false
			fenceStart = -1
			harnessFence = false
		}
	}
	return metadata
}

// normalizeWikiSearchNaturalText is scoped to natural search lexical matching.
// Declared identifier equality continues to use docidentity without folding.
func normalizeWikiSearchNaturalText(value string) string {
	value = norm.NFD.String(strings.ToLower(value))
	var folded strings.Builder
	folded.Grow(len(value))
	for _, r := range value {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		folded.WriteRune(r)
	}
	value = folded.String()
	replacer := strings.NewReplacer(
		"\r", " ",
		"\n", " ",
		"_", " ",
		"-", " ",
		"/", " ",
		"\\", " ",
		".", " ",
		":", " ",
		",", " ",
		";", " ",
		"?", " ",
		"!", " ",
		"(", " ",
		")", " ",
	)
	return strings.Join(strings.Fields(replacer.Replace(value)), " ")
}

var wikiSearchSupportedIdentifierPattern = regexp.MustCompile(`(?i)^(?:FL|RS|RF|TP|TECH|CT|DB|AE)-[A-Za-z0-9_.-]+$`)
var wikiSearchCompoundIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9]+(?:[-._][A-Za-z0-9]+)+$`)

type wikiSearchIdentifierMatcher struct {
	identifier    string
	identifierLow string
}

func newWikiSearchIdentifierMatcher(identifier string) *wikiSearchIdentifierMatcher {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" || strings.ContainsAny(identifier, "\r\n") {
		return nil
	}
	return &wikiSearchIdentifierMatcher{identifier: identifier, identifierLow: strings.ToLower(identifier)}
}

func (m *wikiSearchIdentifierMatcher) matches(text string) bool {
	if m == nil || m.identifierLow == "" {
		return false
	}
	return docidentity.Match(text, m.identifier)
}

func wikiSearchIdentifierQuery(queryText string) bool {
	queryText = strings.TrimSpace(queryText)
	if queryText == "" {
		return false
	}
	if wikiSearchSupportedIdentifierPattern.MatchString(queryText) {
		return true
	}
	if !wikiSearchCompoundIdentifierPattern.MatchString(queryText) {
		return false
	}
	for _, r := range queryText {
		if unicode.IsUpper(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func wikiSearchIdentifierMode(queryText string, exactDocs []model.DocRecord, ranked []scoredDoc) bool {
	for _, doc := range exactDocs {
		if strings.TrimSpace(doc.Path) != "" {
			return true
		}
	}
	for _, candidate := range ranked {
		if strings.EqualFold(strings.TrimSpace(candidate.record.DocID), strings.TrimSpace(queryText)) {
			return true
		}
	}
	return wikiSearchIdentifierQuery(queryText)
}

func wikiSearchHasCompleteIdentifier(text string, identifier string) bool {
	return newWikiSearchIdentifierMatcher(identifier).matches(text)
}

func wikiSearchHasLexicalToken(text string, token string) bool {
	token = normalizeWikiSearchNaturalText(token)
	if token == "" {
		return false
	}
	normalized := normalizeWikiSearchNaturalText(text)
	return normalized == token || strings.Contains(" "+normalized+" ", " "+token+" ")
}

func wikiSearchCandidateHasTextEvidence(queryText string, candidate scoredDoc) bool {
	for _, reason := range candidate.reason {
		if reason == "fts5=match" {
			return true
		}
	}
	for _, token := range docgraph.QuestionTokens(queryText) {
		if wikiSearchHasLexicalToken(candidate.record.DocID, token) ||
			wikiSearchHasLexicalToken(candidate.record.Title, token) ||
			wikiSearchHasLexicalToken(candidate.record.SearchText, token) ||
			wikiSearchHasLexicalToken(candidate.record.Path, token) {
			return true
		}
	}
	return false
}

type wikiSearchSourceFailure string

const (
	wikiSearchSourceFailureInvalidPath   wikiSearchSourceFailure = "invalid_path"
	wikiSearchSourceFailureMissing       wikiSearchSourceFailure = "missing_source"
	wikiSearchSourceFailureOversize      wikiSearchSourceFailure = "oversize_source"
	wikiSearchSourceFailureRead          wikiSearchSourceFailure = "source_read"
	wikiSearchSourceFailureMissingHash   wikiSearchSourceFailure = "missing_hash"
	wikiSearchSourceFailureStaleHash     wikiSearchSourceFailure = "stale_hash"
	wikiSearchSourceFailureSymlinkEscape wikiSearchSourceFailure = "symlink_escape"
)

// wikiSearchExactMentionCandidates recovers complete identifier references from
// persisted mentions only when the current extractor version is trusted. Legacy
// rows are admitted only after bounded raw-source confirmation.
func wikiSearchExactMentionCandidates(ctx context.Context, db *sql.DB, root, queryText string) ([]scoredDoc, error) {
	candidates, _, err := wikiSearchExactMentionCandidatesWithWarnings(ctx, db, root, queryText)
	return candidates, err
}

func wikiSearchExactMentionCandidatesWithWarnings(ctx context.Context, db *sql.DB, root, queryText string) ([]scoredDoc, []string, error) {
	if db == nil || strings.TrimSpace(queryText) == "" {
		return nil, nil, nil
	}
	docs, err := store.FindDocRecordsByMention(ctx, db, model.DocMentionTypeDocID, queryText)
	if err != nil {
		return nil, nil, err
	}
	if len(docs) == 0 {
		return nil, nil, nil
	}
	current, err := store.DocIdentitySnapshotCurrent(ctx, db)
	if err != nil {
		return nil, nil, err
	}
	sort.SliceStable(docs, func(i, j int) bool {
		iOwner := strings.EqualFold(strings.TrimSpace(docs[i].DocID), strings.TrimSpace(queryText))
		jOwner := strings.EqualFold(strings.TrimSpace(docs[j].DocID), strings.TrimSpace(queryText))
		if iOwner != jOwner {
			return iOwner
		}
		return wikiSearchPathKey(docs[i].Path) < wikiSearchPathKey(docs[j].Path)
	})
	if len(docs) > wikiSearchLegacySourceMaxCandidates {
		docs = docs[:wikiSearchLegacySourceMaxCandidates]
	}
	candidates := make([]scoredDoc, 0, len(docs))
	failures := make(map[wikiSearchSourceFailure]int)
	for _, doc := range docs {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		reason := "mention_exact"
		if !current {
			content, failure := wikiSearchReadCanonicalSourceDetailed(root, doc)
			if failure != "" {
				failures[failure]++
				continue
			}
			if !docidentity.Match(string(content), queryText) {
				continue
			}
			reason = "mention_exact_legacy_source"
		}
		candidates = append(candidates, scoredDoc{record: doc, score: 900, reason: []string{reason}})
	}
	return candidates, wikiSearchLegacySourceWarnings(failures), nil
}

const (
	wikiSearchLegacySourceMaxBytes      = 1 << 20
	wikiSearchLegacySourceMaxCandidates = 256
)

func wikiSearchLegacySourceWarnings(failures map[wikiSearchSourceFailure]int) []string {
	ordered := []wikiSearchSourceFailure{
		wikiSearchSourceFailureInvalidPath,
		wikiSearchSourceFailureMissing,
		wikiSearchSourceFailureOversize,
		wikiSearchSourceFailureStaleHash,
		wikiSearchSourceFailureMissingHash,
		wikiSearchSourceFailureSymlinkEscape,
		wikiSearchSourceFailureRead,
	}
	parts := make([]string, 0, len(ordered))
	for _, failure := range ordered {
		if count := failures[failure]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", failure, count))
		}
	}
	if len(parts) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("legacy doc_id mention source confirmation skipped: %s", strings.Join(parts, ", "))}
}

func wikiSearchReadCanonicalSource(root string, doc model.DocRecord) ([]byte, bool) {
	content, failure := wikiSearchReadCanonicalSourceDetailed(root, doc)
	return content, failure == ""
}

func wikiSearchReadCanonicalSourceDetailed(root string, doc model.DocRecord) ([]byte, wikiSearchSourceFailure) {
	if strings.TrimSpace(doc.ContentHash) == "" {
		return nil, wikiSearchSourceFailureMissingHash
	}
	canonicalPath, failure := wikiSearchCanonicalSourcePath(root, doc.Path)
	if failure != "" {
		return nil, failure
	}
	info, err := os.Stat(canonicalPath)
	if err != nil {
		return nil, wikiSearchSourceFailureMissing
	}
	if info.Size() < 0 || info.Size() > wikiSearchLegacySourceMaxBytes {
		return nil, wikiSearchSourceFailureOversize
	}
	content, err := os.ReadFile(canonicalPath)
	if err != nil {
		return nil, wikiSearchSourceFailureRead
	}
	if len(content) > wikiSearchLegacySourceMaxBytes {
		return nil, wikiSearchSourceFailureOversize
	}
	digest := sha1.Sum(content)
	if hex.EncodeToString(digest[:]) != doc.ContentHash {
		return nil, wikiSearchSourceFailureStaleHash
	}
	return content, ""
}

func wikiSearchCanonicalSourcePath(root string, relPath string) (string, wikiSearchSourceFailure) {
	if strings.TrimSpace(root) == "" {
		return "", wikiSearchSourceFailureInvalidPath
	}
	relPath = strings.TrimSpace(relPath)
	if relPath == "" || strings.ContainsRune(relPath, '\x00') {
		return "", wikiSearchSourceFailureInvalidPath
	}
	rel := filepath.Clean(filepath.FromSlash(relPath))
	if filepath.IsAbs(rel) || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", wikiSearchSourceFailureInvalidPath
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", wikiSearchSourceFailureMissing
	}
	canonicalPath, err := filepath.EvalSymlinks(filepath.Join(root, rel))
	if err != nil {
		return "", wikiSearchSourceFailureMissing
	}
	relCanonical, err := filepath.Rel(canonicalRoot, canonicalPath)
	if err != nil {
		return "", wikiSearchSourceFailureRead
	}
	if relCanonical == ".." || strings.HasPrefix(relCanonical, ".."+string(filepath.Separator)) {
		return "", wikiSearchSourceFailureSymlinkEscape
	}
	return canonicalPath, ""
}

func wikiSearchReadSafeSource(root string, relPath string) ([]byte, bool) {
	canonicalPath, failure := wikiSearchCanonicalSourcePath(root, relPath)
	if failure != "" {
		return nil, false
	}
	content, err := os.ReadFile(canonicalPath)
	if err != nil {
		return nil, false
	}
	return content, true
}

func wikiSearchHasReason(candidate scoredDoc, want string) bool {
	for _, reason := range candidate.reason {
		if reason == want {
			return true
		}
	}
	return false
}

// wikiSearchCandidateAdmitted is deliberately search-local: route/ask/pack keep
// their owner-aware ranking, while literal search must not turn routing priors into
// relevance evidence.
func wikiSearchCandidateAdmitted(queryText string, identifierMode bool, identifierMatcher *wikiSearchIdentifierMatcher, candidate scoredDoc, exactSourcePaths map[string]struct{}) bool {
	if _, exactSource := exactSourcePaths[wikiSearchPathKey(candidate.record.Path)]; exactSource {
		return true
	}
	if wikiSearchHasReason(candidate, "mention_exact") || wikiSearchHasReason(candidate, "mention_exact_legacy_source") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(candidate.record.DocID), strings.TrimSpace(queryText)) {
		return true
	}
	if identifierMode {
		return identifierMatcher.matches(candidate.record.Title) ||
			identifierMatcher.matches(candidate.record.Path)
	}
	return wikiSearchCandidateHasTextEvidence(queryText, candidate)
}

func sanitizeWikiSearchCandidate(queryText string, identifierMode bool, candidate scoredDoc) scoredDoc {
	if !identifierMode || strings.EqualFold(strings.TrimSpace(candidate.record.DocID), strings.TrimSpace(queryText)) {
		return candidate
	}
	reasons := make([]string, 0, len(candidate.reason))
	for _, reason := range candidate.reason {
		if strings.HasPrefix(reason, "doc_id=") {
			continue
		}
		reasons = append(reasons, reason)
	}
	candidate.reason = reasons
	return candidate
}

func orderWikiSearchCandidates(queryText string, exactDocs []model.DocRecord, ranked []scoredDoc, layerFilter map[string]struct{}) []scoredDoc {
	candidates := assembleWikiSearchCandidates(queryText, exactDocs, ranked, layerFilter)
	if wikiSearchIdentifierMode(queryText, exactDocs, ranked) {
		return candidates
	}
	return rerankWikiSearchNaturalCandidates(queryText, candidates)
}

func rerankWikiSearchNaturalCandidates(queryText string, candidates []scoredDoc) []scoredDoc {
	tokens := docgraph.QuestionTokens(queryText)
	for i := range candidates {
		score, reasons := wikiSearchNaturalEvidenceScore(candidates[i].record, tokens, queryText)
		candidates[i].score = score
		candidates[i].reason = append(candidates[i].reason, reasons...)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		leftPath := wikiSearchPathKey(candidates[i].record.Path)
		rightPath := wikiSearchPathKey(candidates[j].record.Path)
		if leftPath != rightPath {
			return leftPath < rightPath
		}
		return candidates[i].record.DocID < candidates[j].record.DocID
	})
	return candidates
}

func wikiSearchNaturalEvidenceScore(doc model.DocRecord, tokens []string, queryText string) (int, []string) {
	title := normalizeWikiSearchNaturalText(doc.Title)
	snippet := normalizeWikiSearchNaturalText(doc.Snippet)
	searchText := normalizeWikiSearchNaturalText(doc.SearchText)
	path := normalizeWikiSearchNaturalText(doc.Path)
	docID := normalizeWikiSearchNaturalText(doc.DocID)
	coverage := 0
	titleHits := 0
	snippetHits := 0
	bodyHits := 0
	pathHits := 0
	for _, token := range tokens {
		matched := false
		if wikiSearchHasLexicalToken(title, token) {
			titleHits++
			matched = true
		}
		if wikiSearchHasLexicalToken(snippet, token) {
			snippetHits++
			matched = true
		}
		if wikiSearchHasLexicalToken(searchText, token) {
			bodyHits++
			matched = true
		}
		if wikiSearchHasLexicalToken(path, token) || wikiSearchHasLexicalToken(docID, token) {
			pathHits++
			matched = true
		}
		if matched {
			coverage++
		}
	}
	score := coverage * 100
	score += titleHits * 24
	score += snippetHits * 28
	score += bodyHits * 8
	score += pathHits * 3
	if normalizedQuery := normalizeWikiSearchNaturalText(queryText); normalizedQuery != "" &&
		(strings.Contains(title, normalizedQuery) || strings.Contains(snippet, normalizedQuery) || strings.Contains(searchText, normalizedQuery)) {
		score += 20
	}
	reasons := []string{fmt.Sprintf("natural_coverage=%d/%d", coverage, len(tokens))}
	if titleHits > 0 {
		reasons = append(reasons, fmt.Sprintf("natural_heading=%d", titleHits))
	}
	if snippetHits > 0 {
		reasons = append(reasons, fmt.Sprintf("natural_passage=%d", snippetHits))
	}
	if bodyHits > 0 {
		reasons = append(reasons, fmt.Sprintf("natural_body=%d", bodyHits))
	}
	if pathHits > 0 {
		reasons = append(reasons, fmt.Sprintf("natural_identity=%d", pathHits))
	}
	return max(score, 1), reasons
}

func assembleWikiSearchCandidates(queryText string, exactDocs []model.DocRecord, ranked []scoredDoc, layerFilter map[string]struct{}) []scoredDoc {
	queryText = strings.TrimSpace(queryText)
	sortedExactDocs := append([]model.DocRecord(nil), exactDocs...)
	sort.SliceStable(sortedExactDocs, func(i, j int) bool {
		if sortedExactDocs[i].Layer != sortedExactDocs[j].Layer {
			return sortedExactDocs[i].Layer < sortedExactDocs[j].Layer
		}
		if wikiSearchPathKey(sortedExactDocs[i].Path) != wikiSearchPathKey(sortedExactDocs[j].Path) {
			return wikiSearchPathKey(sortedExactDocs[i].Path) < wikiSearchPathKey(sortedExactDocs[j].Path)
		}
		return sortedExactDocs[i].DocID < sortedExactDocs[j].DocID
	})
	exactSourcePaths := make(map[string]struct{}, len(sortedExactDocs))
	for _, doc := range sortedExactDocs {
		exactSourcePaths[wikiSearchPathKey(doc.Path)] = struct{}{}
	}
	identifierMode := wikiSearchIdentifierMode(queryText, sortedExactDocs, ranked)
	identifierMatcher := newWikiSearchIdentifierMatcher(queryText)

	candidates := make([]scoredDoc, 0, len(sortedExactDocs)+len(ranked))
	seenPaths := make(map[string]struct{}, len(sortedExactDocs)+len(ranked))
	appendCandidate := func(candidate scoredDoc) {
		candidate = sanitizeWikiSearchCandidate(queryText, identifierMode, candidate)
		if !wikiSearchCandidateAdmitted(queryText, identifierMode, identifierMatcher, candidate, exactSourcePaths) {
			return
		}
		layer := wikiLayerForDoc(candidate.record)
		if len(layerFilter) > 0 {
			if _, ok := layerFilter[layer]; !ok {
				return
			}
		}
		pathKey := wikiSearchPathKey(candidate.record.Path)
		if _, seen := seenPaths[pathKey]; seen {
			return
		}
		seenPaths[pathKey] = struct{}{}
		candidates = append(candidates, candidate)
	}

	// An explicit DocRecord.DocID is the strongest owner signal. Keep this
	// precedence separate from source declarations so a source block/record
	// cannot displace its owning document in the result sequence.
	for _, candidate := range ranked {
		if !strings.EqualFold(strings.TrimSpace(candidate.record.DocID), queryText) {
			continue
		}
		if _, sourceMatch := exactSourcePaths[wikiSearchPathKey(candidate.record.Path)]; sourceMatch {
			candidate = withWikiSearchReason(candidate, "source_id_exact")
			candidate.score = max(candidate.score, 1000)
		}
		appendCandidate(candidate)
	}
	for _, doc := range sortedExactDocs {
		if strings.EqualFold(strings.TrimSpace(doc.DocID), queryText) {
			appendCandidate(scoredDoc{record: doc, score: 1000, reason: []string{"source_id_exact"}})
		}
	}

	for _, doc := range sortedExactDocs {
		appendCandidate(scoredDoc{record: doc, score: 1000, reason: []string{"source_id_exact"}})
	}
	for _, candidate := range ranked {
		appendCandidate(candidate)
	}
	return candidates
}

func withWikiSearchReason(candidate scoredDoc, reason string) scoredDoc {
	for _, existing := range candidate.reason {
		if existing == reason {
			return candidate
		}
	}
	candidate.reason = append(append([]string{}, candidate.reason...), reason)
	return candidate
}

func wikiSearchPathKey(path string) string {
	return filepath.ToSlash(strings.TrimSpace(path))
}

func wikiSearchAmbiguityWarnings(eligible []scoredDoc) []string {
	pathsByDocID := map[string]map[string]struct{}{}
	for _, candidate := range eligible {
		docID := strings.ToUpper(strings.TrimSpace(candidate.record.DocID))
		if docID == "" {
			continue
		}
		if _, ok := pathsByDocID[docID]; !ok {
			pathsByDocID[docID] = map[string]struct{}{}
		}
		pathsByDocID[docID][wikiSearchPathKey(candidate.record.Path)] = struct{}{}
	}
	warnings := make([]string, 0)
	for docID, paths := range pathsByDocID {
		if len(paths) <= 1 {
			continue
		}
		ordered := make([]string, 0, len(paths))
		for path := range paths {
			ordered = append(ordered, path)
		}
		sort.Strings(ordered)
		warnings = append(warnings, fmt.Sprintf("ambiguous wiki doc_id %q has multiple eligible owner paths: %s", docID, strings.Join(ordered, ", ")))
	}
	sort.Strings(warnings)
	return warnings
}

func wikiSearchCandidatePage(eligible []scoredDoc, offset int, top int) []scoredDoc {
	start := min(max(offset, 0), len(eligible))
	if top <= 0 {
		return eligible[start:]
	}
	end := start + min(top, len(eligible)-start)
	return eligible[start:end]
}

func wikiSearchResult(workspaceName string, root string, queryText string, candidate scoredDoc, layer string, includeContent bool, maxChars int) model.WikiSearchResult {
	doc := candidate.record
	item := model.WikiSearchResult{
		DocID:       doc.DocID,
		Path:        doc.Path,
		Title:       doc.Title,
		Layer:       layer,
		Family:      doc.Family,
		Stage:       wikiStageForDoc(doc),
		Score:       candidate.score,
		Why:         append([]string{}, candidate.reason...),
		Snippet:     doc.Snippet,
		NextQueries: buildWikiSearchNextQueries(workspaceName, queryText, doc, layer),
	}
	if includeContent {
		item.Content = readWikiSearchContent(root, doc.Path, maxChars)
	}
	if evidence, ok := wikiSearchLineEvidence(root, doc.Path, queryText); ok {
		item.Line = evidence.line
		item.StartLine = evidence.startLine
		item.EndLine = evidence.endLine
		item.Evidence = evidence.text
	} else if strings.TrimSpace(doc.Snippet) != "" {
		item.Why = append(item.Why, "source_evidence_unavailable=indexed_snippet_fallback")
	}
	return item
}

func parseWikiLayerFilter(raw string) (map[string]struct{}, []string) {
	filter := map[string]struct{}{}
	unknown := []string{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}) {
		layer := strings.ToUpper(strings.TrimSpace(part))
		if layer == "" {
			continue
		}
		switch layer {
		case "RS", "RF", "FL", "TP", "CT", "TECH", "DB", "AE":
			filter[layer] = struct{}{}
		default:
			unknown = append(unknown, layer)
		}
	}
	if len(filter) == 0 {
		return nil, unknown
	}
	return filter, unknown
}

func wikiLayerForDoc(doc model.DocRecord) string {
	docID := strings.ToUpper(strings.TrimSpace(doc.DocID))
	path := filepath.ToSlash(doc.Path)
	switch {
	case strings.HasPrefix(docID, "RS-") || doc.Layer == "RS" || strings.Contains(path, "/02_resultados/") || strings.HasSuffix(path, "/02_resultados_soluciones_usuario.md"):
		return "RS"
	case strings.HasPrefix(docID, "FL-") || doc.Layer == "03" || strings.Contains(path, "/03_FL/"):
		return "FL"
	case strings.HasPrefix(docID, "RF-") || doc.Layer == "04" || strings.Contains(path, "/04_RF/"):
		return "RF"
	case strings.HasPrefix(docID, "TP-") || doc.Layer == "06" || strings.Contains(path, "/06_pruebas/"):
		return "TP"
	case strings.HasPrefix(docID, "CT-") || doc.Layer == "09" || strings.Contains(path, "/09_contratos/"):
		return "CT"
	case strings.HasPrefix(docID, "TECH-") || doc.Layer == "07" || strings.Contains(path, "/07_tech/"):
		return "TECH"
	case strings.HasPrefix(docID, "DB-") || doc.Layer == "08" || strings.Contains(path, "/08_db/"):
		return "DB"
	case strings.HasPrefix(docID, "AE-") || doc.Layer == "AE" || strings.Contains(path, "/ae/"):
		return "AE"
	default:
		return strings.ToUpper(strings.TrimSpace(doc.Layer))
	}
}

func wikiStageForDoc(doc model.DocRecord) string {
	path := filepath.ToSlash(doc.Path)
	switch {
	case strings.Contains(path, "/02_resultados/") || strings.HasSuffix(path, "/02_resultados_soluciones_usuario.md") || doc.Layer == "RS":
		return "outcome"
	case strings.Contains(path, "/03_FL/") || doc.Layer == "03":
		return "flow"
	case strings.Contains(path, "/04_RF/") || doc.Layer == "04":
		return "requirements"
	case strings.Contains(path, "/06_pruebas/") || doc.Layer == "06":
		return "tests"
	case strings.Contains(path, "/07_tech/"):
		return "technical_detail"
	case doc.Layer == "07":
		return "technical_baseline"
	case strings.Contains(path, "/08_db/") || doc.Layer == "08":
		return "physical_data"
	case strings.Contains(path, "/09_contratos/") || doc.Layer == "09":
		return "contracts"
	case strings.Contains(path, "/ae/") || doc.Layer == "AE":
		return "technical_detail"
	case doc.Layer == "01":
		return "scope"
	case doc.Layer == "02":
		return "architecture"
	default:
		return ""
	}
}

func buildWikiSearchNextQueries(workspaceName string, queryText string, doc model.DocRecord, layer string) []string {
	queries := make([]string, 0, 4)
	queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav wiki pack %q --workspace %s --doc %s --format toon", queryText, workspaceName, doc.Path))
	if (layer == "RS" && strings.HasPrefix(strings.ToUpper(doc.DocID), "RS-")) ||
		(layer == "RF" && strings.HasPrefix(strings.ToUpper(doc.DocID), "RF-")) {
		queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav wiki trace %s --workspace %s --format toon", doc.DocID, workspaceName))
	}
	if doc.DocID != "" {
		queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav wiki search %q --workspace %s --format toon", doc.DocID, workspaceName))
	}
	if doc.Path != "" {
		queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav multi-read %s:1-120 --workspace %s --format toon", doc.Path, workspaceName))
	}
	queries = appendUniqueQuery(queries, fmt.Sprintf("mi-lsp nav ask %q --workspace %s --format toon", queryText, workspaceName))
	if len(queries) > 4 {
		return queries[:4]
	}
	return queries
}

func readWikiSearchContent(root string, relPath string, maxChars int) string {
	content, ok := wikiSearchReadSafeSource(root, relPath)
	if !ok {
		return ""
	}
	limit := 4096
	if maxChars > 0 && maxChars < limit {
		limit = maxChars
	}
	text := string(content)
	if len(text) > limit {
		return text[:limit]
	}
	return text
}

// wikiSearchAllWorkspaces handles --all-workspaces fan-out using FanOutWiki.
func (a *App) wikiSearchAllWorkspaces(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	queryText := strings.TrimSpace(firstNonEmpty(
		stringPayload(request.Payload, "query"),
		stringPayload(request.Payload, "pattern"),
		stringPayload(request.Payload, "task"),
	))
	if queryText == "" {
		return model.Envelope{}, fmt.Errorf("query is required")
	}

	// Extract search parameters from payload
	layerFilterRaw := stringPayload(request.Payload, "layer")
	layerFilter, _ := parseWikiLayerFilter(layerFilterRaw)
	topGlobalFromPayload := intFromAny(request.Payload["top_global"], 50)
	topGlobal := topGlobalFromPayload
	if topGlobal <= 0 {
		topGlobal = 50
	}
	searchTop := intFromAny(request.Payload["top"], 0)
	searchOffset := intFromAny(request.Payload["offset"], 0)
	includeContent, _ := request.Payload["include_content"].(bool)

	// Fan-out across workspaces
	fanOutOpts := nav.WikiFanOutOptions{
		Timeout:  0, // Use default (30s)
		Parallel: 0, // Use default (4)
	}

	fanOutResult, err := nav.FanOutWiki(ctx, fanOutOpts, func(subCtx context.Context, ws model.WorkspaceRegistration) ([]any, map[string]any, error) {
		// Query the doc index for this workspace
		query := loadDocQueryContext(subCtx, ws, queryText)
		defer query.Close()
		if query.dbErr != nil {
			return nil, map[string]any{}, query.dbErr
		}

		if len(query.docs) == 0 {
			return []any{}, map[string]any{}, nil
		}

		// Assemble the complete deterministic sequence before applying offset/limit.
		exactDocs, exactErr := store.FindDocRecordsBySourceID(subCtx, query.db, queryText)
		if exactErr != nil {
			return nil, map[string]any{}, exactErr
		}
		identitySnapshotCurrent, identityErr := store.DocIdentitySnapshotCurrent(subCtx, query.db)
		if identityErr != nil {
			return nil, map[string]any{}, identityErr
		}
		mentionCandidates, mentionWarnings, mentionErr := wikiSearchExactMentionCandidatesWithWarnings(subCtx, query.db, ws.Root, queryText)
		if mentionErr != nil {
			return nil, map[string]any{}, mentionErr
		}
		ranked := append([]scoredDoc(nil), query.ranked...)
		ranked = append(ranked, mentionCandidates...)
		eligible := orderWikiSearchCandidates(queryText, exactDocs, ranked, layerFilter)
		workspaceTop := searchTop
		if workspaceTop <= 0 {
			workspaceTop = 10
		}
		selected := wikiSearchCandidatePage(eligible, searchOffset, workspaceTop)

		candidates := make([]model.WikiSearchResult, 0, len(selected))
		for _, candidate := range selected {
			layer := wikiLayerForDoc(candidate.record)
			item := wikiSearchResult(ws.Name, ws.Root, queryText, candidate, layer, includeContent, request.Context.MaxChars)
			item.Workspace = ws.Name // Add workspace label
			item.Host = ""
			candidates = append(candidates, item)
		}

		// Convert candidates to []any
		itemsAny := make([]any, len(candidates))
		for i := range candidates {
			itemsAny[i] = candidates[i]
		}

		workspaceWarnings := wikiSearchAmbiguityWarnings(eligible)
		for _, warning := range mentionWarnings {
			workspaceWarnings = appendStringIfMissing(workspaceWarnings, warning)
		}
		if !identitySnapshotCurrent {
			workspaceWarnings = appendStringIfMissing(workspaceWarnings, "legacy doc_id mentions are unverified; only source-confirmed references are eligible")
		}
		stats := map[string]any{
			"warnings": workspaceWarnings,
		}
		return itemsAny, stats, nil
	})

	if err != nil {
		return model.Envelope{}, err
	}

	// Aggregate results from all workspaces
	var allResults []model.WikiSearchResult
	fanoutWarnings := []string{}
	for _, wsResult := range fanOutResult.Items {
		if wsResult.Err != nil {
			// Skip workspaces with errors but continue with others
			continue
		}
		if rawWarnings, ok := wsResult.Stats["warnings"].([]string); ok {
			for _, warning := range rawWarnings {
				fanoutWarnings = appendStringIfMissing(fanoutWarnings, fmt.Sprintf("%s: %s", wsResult.Workspace, warning))
			}
		}
		for _, item := range wsResult.Items {
			if wsItem, ok := item.(model.WikiSearchResult); ok {
				allResults = append(allResults, wsItem)
			}
		}
	}

	// Sort by (score DESC, workspace ASC, doc_id ASC)
	sort.Slice(allResults, func(i, j int) bool {
		if allResults[i].Score != allResults[j].Score {
			return allResults[i].Score > allResults[j].Score
		}
		if allResults[i].Workspace != allResults[j].Workspace {
			return allResults[i].Workspace < allResults[j].Workspace
		}
		return allResults[i].DocID < allResults[j].DocID
	})

	// Truncate to topGlobal
	if len(allResults) > topGlobal {
		allResults = allResults[:topGlobal]
	}

	// Build envelope with federated stats
	warnings := fanoutWarnings
	failureStrs := []string{}
	if len(fanOutResult.WorkspacesFailed) > 0 {
		for _, f := range fanOutResult.WorkspacesFailed {
			failureStrs = append(failureStrs, fmt.Sprintf("%s: %s", f.Alias, f.Reason))
		}
		warnings = append(warnings, fmt.Sprintf("%d workspace(s) failed during query", len(fanOutResult.WorkspacesFailed)))
	}

	stats := model.Stats{
		Files:                 len(allResults),
		WorkspacesQueried:     fanOutResult.WorkspacesQueried,
		WorkspacesFailed:      failureStrs,
		TruncatedPerWorkspace: fanOutResult.TruncatedPerWS,
	}

	hint := ""
	if len(allResults) == 0 {
		if len(layerFilter) > 0 {
			hint = "0 wiki matches across workspaces for selected layers; broaden --layer or try nav wiki route"
		} else {
			hint = "0 wiki matches across workspaces; try a doc id like RS-*, RF-*, FL-*, CT-*, TECH-*, DB-*, AE-*"
		}
	}

	// Convert results to []any for envelope
	itemsAny := make([]any, len(allResults))
	for i := range allResults {
		itemsAny[i] = allResults[i]
	}

	env := model.Envelope{
		Ok:        true,
		Workspace: "all",
		Backend:   "wiki.search",
		Items:     itemsAny,
		Warnings:  warnings,
		Hint:      hint,
		Stats:     stats,
	}
	return env, nil
}
