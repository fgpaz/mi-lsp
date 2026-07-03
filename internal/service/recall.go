package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/embed"
	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/rerank"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/wikichunk"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

type wikiEmbeddingPlan struct {
	docPaths []string
	chunks   []wikiEmbeddingChunkPlan
}

type wikiEmbeddingChunkPlan struct {
	DocPath     string
	ChunkID     string
	Heading     string
	Snippet     string
	ContentHash string
	StartLine   int
	EndLine     int
	Text        string
	Model       string
	Dim         int
	Reusable    bool
	Existing    model.WikiChunkEmbedding
}

func (p wikiEmbeddingChunkPlan) embeddingWith(vector []byte, indexedAt int64) model.WikiChunkEmbedding {
	return model.WikiChunkEmbedding{
		DocPath:        p.DocPath,
		ChunkID:        p.ChunkID,
		Heading:        p.Heading,
		Snippet:        p.Snippet,
		ContentHash:    p.ContentHash,
		EmbeddingModel: p.Model,
		StartLine:      p.StartLine,
		EndLine:        p.EndLine,
		EmbeddingDim:   p.Dim,
		Embedding:      vector,
		IndexedAt:      indexedAt,
	}
}

type embeddingIndexError struct {
	err       error
	completed int
	total     int
}

func (e embeddingIndexError) Error() string {
	return fmt.Sprintf("embeddings: %v after %d/%d chunks embedded", e.err, e.completed, e.total)
}

func (e embeddingIndexError) Unwrap() error { return e.err }

// embedWorkspaceWiki indexes wiki chunks with embeddings after a successful doc publish.
func (a *App) embedWorkspaceWiki(ctx context.Context, root string, progress func(context.Context, indexer.Progress) error) ([]string, error) {
	var warnings []string

	// Load project file for embeddings config
	project, err := workspace.LoadProjectFile(root)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("embeddings: failed to load project config: %v", err))
		return warnings, nil
	}

	// Check if embeddings are active and configured.
	if !project.Embeddings.Active() {
		// No-op: embeddings not configured
		return nil, nil
	}

	// Open the repo-local store
	db, err := store.Open(root)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("embeddings: failed to open store: %v", err))
		return warnings, nil
	}
	defer db.Close()

	// Load all doc records to get the indexed docs
	docs, err := store.ListDocRecords(ctx, db)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("embeddings: failed to load doc records: %v", err))
		return warnings, nil
	}

	// For each doc, chunk and embed
	existingEmbeddings, err := store.LoadWikiChunkEmbeddings(ctx, db)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("embeddings: failed to load existing embeddings: %v", err))
		return warnings, nil
	}

	// Build embeddings client
	cfg := embed.Config{
		Provider:       project.Embeddings.Provider,
		BaseURL:        project.Embeddings.BaseURL,
		Model:          project.Embeddings.Model,
		APIKeyEnv:      project.Embeddings.APIKeyEnv,
		Dim:            project.Embeddings.Dim,
		BatchSize:      project.Embeddings.BatchSize,
		TimeoutMS:      project.Embeddings.TimeoutMS,
		EncodingFormat: project.Embeddings.EncodingFormat,
		UserAgent:      project.Embeddings.UserAgent,
	}
	if cfg.APIKeyEnv == "" {
		cfg.APIKeyEnv = "MI_LSP_EMBEDDINGS_API_KEY"
	}
	client := embed.New(cfg)

	plan, planWarnings := buildWikiEmbeddingPlan(ctx, root, docs, existingEmbeddings, cfg)
	warnings = append(warnings, planWarnings...)
	total := len(plan.chunks)
	completed := 0
	if err := ctx.Err(); err != nil {
		return warnings, embeddingPhaseError(err, completed, total)
	}
	if total == 0 {
		return warnings, nil
	}
	if err := store.PruneInvalidWikiChunkEmbeddings(ctx, db); err != nil {
		warnings = append(warnings, fmt.Sprintf("embeddings: failed to prune invalid embeddings: %v", err))
		return warnings, nil
	}

	batchSize := effectiveEmbeddingBatchSize(cfg.BatchSize)
	pending := make([]model.WikiChunkEmbedding, 0, batchSize)
	texts := make([]string, 0, batchSize)
	textPlans := make([]wikiEmbeddingChunkPlan, 0, batchSize)
	storedKeys := make(map[string]struct{}, total)

	report := func(force bool, currentPath string) error {
		if progress == nil {
			return nil
		}
		return progress(ctx, indexer.Progress{
			Stage:      "embeddings",
			Path:       embeddingProgressPath(currentPath, completed, total),
			Docs:       completed,
			FilesTotal: total,
			Force:      force,
		})
	}
	flushStored := func(force bool, currentPath string) error {
		if len(pending) == 0 {
			return report(force, currentPath)
		}
		if err := store.UpsertWikiChunkEmbeddings(ctx, db, pending); err != nil {
			return embeddingPhaseError(fmt.Errorf("failed to store embeddings: %w", err), completed, total)
		}
		for _, chunk := range pending {
			storedKeys[chunk.DocPath+"\x00"+chunk.ChunkID] = struct{}{}
		}
		completed += len(pending)
		pending = pending[:0]
		return report(force, currentPath)
	}
	flushEmbeddings := func(force bool) error {
		if len(texts) == 0 {
			return nil
		}
		currentPath := textPlans[len(textPlans)-1].DocPath
		vectors, err := client.Embed(ctx, texts)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return embeddingPhaseError(ctxErr, completed, total)
			}
			warnings = append(warnings, fmt.Sprintf("embeddings: failed to embed %d chunks ending at %s: %v", len(texts), currentPath, err))
			texts = texts[:0]
			textPlans = textPlans[:0]
			return report(force, currentPath)
		}
		now := time.Now().Unix()
		for i, vector := range vectors {
			pending = append(pending, textPlans[i].embeddingWith(embed.EncodeVector(vector), now))
		}
		texts = texts[:0]
		textPlans = textPlans[:0]
		return flushStored(force, currentPath)
	}

	if err := report(true, ""); err != nil {
		return warnings, embeddingPhaseError(err, completed, total)
	}
	for _, planned := range plan.chunks {
		if err := ctx.Err(); err != nil {
			return warnings, embeddingPhaseError(err, completed, total)
		}
		if planned.Reusable {
			pending = append(pending, planned.embeddingWith(planned.Existing.Embedding, planned.Existing.IndexedAt))
			if len(pending) >= batchSize {
				if err := flushStored(false, planned.DocPath); err != nil {
					return warnings, err
				}
			}
			continue
		}
		if len(pending) > 0 {
			if err := flushStored(false, planned.DocPath); err != nil {
				return warnings, err
			}
		}
		texts = append(texts, planned.Text)
		textPlans = append(textPlans, planned)
		if len(texts) >= batchSize {
			if err := flushEmbeddings(false); err != nil {
				return warnings, err
			}
		}
	}
	if err := flushEmbeddings(true); err != nil {
		return warnings, err
	}
	if err := flushStored(true, ""); err != nil {
		return warnings, err
	}
	if err := store.DeleteStaleWikiChunkEmbeddingsForDocs(ctx, db, plan.docPaths, storedKeys); err != nil {
		return warnings, embeddingPhaseError(fmt.Errorf("failed to delete stale embeddings: %w", err), completed, total)
	}
	if completed < total {
		warnings = append(warnings, fmt.Sprintf("embeddings: stored %d/%d chunks; skipped %d chunks after provider errors", completed, total, total-completed))
	}

	return warnings, nil
}

func (a *App) appendWikiEmbeddingWarnings(ctx context.Context, root string, warnings []string, progress func(context.Context, indexer.Progress) error) ([]string, error) {
	embedWarnings, err := a.embedWorkspaceWiki(ctx, root, progress)
	if len(embedWarnings) > 0 {
		warnings = append(warnings, embedWarnings...)
	}
	return warnings, err
}

func buildWikiEmbeddingPlan(ctx context.Context, root string, docs []model.DocRecord, existingEmbeddings map[string]model.WikiChunkEmbedding, cfg embed.Config) (wikiEmbeddingPlan, []string) {
	var (
		plan       wikiEmbeddingPlan
		warnings   []string
		seenDoc    = make(map[string]struct{})
		configured = cfg
	)
	if configured.BatchSize <= 0 {
		configured.BatchSize = 32
	}
	for _, doc := range docs {
		if ctx.Err() != nil {
			return plan, warnings
		}
		docPath := doc.Path
		filePath := filepath.Join(root, filepath.FromSlash(docPath))
		content, err := os.ReadFile(filePath)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("embeddings: failed to read %s: %v", docPath, err))
			continue
		}
		if _, ok := seenDoc[docPath]; !ok {
			seenDoc[docPath] = struct{}{}
			plan.docPaths = append(plan.docPaths, docPath)
		}
		docMeta := parseWikiDocMetadata(string(content), doc)
		for _, chunk := range wikichunk.ChunkByHeading(string(content)) {
			embeddingText := embeddingTextForChunk(doc, docMeta, chunk)
			embeddingHash := hashEmbeddingText(chunk.ContentHash, embeddingText)
			snippet := chunk.Text
			if len(snippet) > 200 {
				snippet = snippet[:200]
			}
			item := wikiEmbeddingChunkPlan{
				DocPath:     docPath,
				ChunkID:     chunk.ChunkID,
				Heading:     chunk.Heading,
				Snippet:     snippet,
				ContentHash: embeddingHash,
				StartLine:   chunk.StartLine,
				EndLine:     chunk.EndLine,
				Text:        embeddingText,
				Model:       configured.Model,
				Dim:         configured.Dim,
			}
			if existing, ok := existingEmbeddings[docPath+"\x00"+chunk.ChunkID]; ok && existing.ContentHash == embeddingHash && existing.EmbeddingModel == configured.Model && existing.EmbeddingDim == configured.Dim && len(existing.Embedding) > 0 {
				item.Reusable = true
				item.Existing = existing
			}
			plan.chunks = append(plan.chunks, item)
		}
	}
	sort.Strings(plan.docPaths)
	return plan, warnings
}

func effectiveEmbeddingBatchSize(batchSize int) int {
	if batchSize <= 0 {
		return 32
	}
	return batchSize
}

func embeddingProgressPath(currentPath string, completed int, total int) string {
	progress := fmt.Sprintf("%d/%d chunks embedded", completed, total)
	if strings.TrimSpace(currentPath) == "" {
		return progress
	}
	return fmt.Sprintf("%s (%s)", currentPath, progress)
}

func embeddingPhaseError(err error, completed int, total int) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, errIndexJobCanceled) {
		return err
	}
	return embeddingIndexError{err: err, completed: completed, total: total}
}

type scoredChunk struct {
	embedding model.WikiChunkEmbedding
	score     float64
	reranked  bool
}

// recall handles semantic search over wiki chunks via embeddings or lexical fallback.
func (a *App) recall(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	// Resolve workspace (same pattern as search/ask, NO governance gate)
	registration, project, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}

	query, _ := request.Payload["query"].(string)
	query = strings.TrimSpace(query)
	if query == "" {
		return model.Envelope{}, fmt.Errorf("query is required")
	}

	mapMode, _ := request.Payload["map"].(bool)
	intent := normalizeRecallIntent(stringPayload(request.Payload, "intent"))

	// Check if embeddings are active and configured.
	if !project.Embeddings.Active() {
		// Return hint without calling embeddings
		hint := "embeddings not configured; configure [embeddings] section in .mi-lsp/project.toml or use 'mi-lsp nav wiki search' for lexical search"
		return model.Envelope{
			Ok:        true,
			Workspace: registration.Name,
			Backend:   "recall",
			Items:     []model.RecallResult{},
			Hint:      hint,
		}, nil
	}

	// Open store
	db, err := store.Open(registration.Root)
	if err != nil {
		return model.Envelope{}, err
	}
	defer db.Close()

	// Build embeddings client
	cfg := embed.Config{
		Provider:       project.Embeddings.Provider,
		BaseURL:        project.Embeddings.BaseURL,
		Model:          project.Embeddings.Model,
		APIKeyEnv:      project.Embeddings.APIKeyEnv,
		Dim:            project.Embeddings.Dim,
		BatchSize:      project.Embeddings.BatchSize,
		TimeoutMS:      project.Embeddings.TimeoutMS,
		EncodingFormat: project.Embeddings.EncodingFormat,
		UserAgent:      project.Embeddings.UserAgent,
	}
	if cfg.APIKeyEnv == "" {
		cfg.APIKeyEnv = "MI_LSP_EMBEDDINGS_API_KEY"
	}
	client := embed.New(cfg)

	// Try to embed the query
	queryVector, err := client.EmbedOne(ctx, recallQueryText(query, intent))
	if err != nil {
		// Fall back to lexical search
		items, searchErr := searchPatternHelper(ctx, registration.Root, project, query, false, askLimit(request.Context.MaxItems, 10, 10))
		if searchErr != nil {
			return model.Envelope{}, searchErr
		}

		// Map search results to RecallResults
		var results []model.RecallResult
		for _, item := range items {
			path, _ := item["path"].(string)
			snippet, _ := item["snippet"].(string)
			results = append(results, model.RecallResult{
				Query:   query,
				Intent:  intent,
				Archivo: path,
				Snippet: snippet,
				Score:   0,
				Why:     []string{"lexical_fallback"},
			})
		}

		warnings := []string{"embeddings unavailable; served lexical results"}
		hint := "embeddings endpoint offline; results are from lexical search. Fix embeddings config to enable semantic search or use 'mi-lsp nav wiki search'."

		return model.Envelope{
			Ok:        true,
			Workspace: registration.Name,
			Backend:   "recall+lexical",
			Items:     results,
			Warnings:  warnings,
			Hint:      hint,
			Stats:     model.Stats{Files: len(results)},
		}, nil
	}

	// Load all embeddings
	allEmbeddings, err := store.AllWikiChunkEmbeddings(ctx, db)
	if err != nil {
		return model.Envelope{}, err
	}
	docRecords, _ := store.ListDocRecords(ctx, db)
	docByPath := make(map[string]model.DocRecord, len(docRecords))
	for _, doc := range docRecords {
		docByPath[doc.Path] = doc
	}

	// Score each embedding
	var scored []scoredChunk

	for _, emb := range allEmbeddings {
		vector := embed.DecodeVector(emb.Embedding)
		score := embed.Cosine(queryVector, vector)
		if doc, ok := docByPath[emb.DocPath]; ok {
			score += recallIntentBoost(intent, emb, doc)
		} else {
			score += recallIntentBoost(intent, emb, model.DocRecord{Path: emb.DocPath})
		}
		scored = append(scored, scoredChunk{embedding: emb, score: score})
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Take top-k
	maxItems := request.Context.MaxItems
	if maxItems <= 0 {
		maxItems = 10
	}
	var warnings []string
	scored = applyRecallRerank(ctx, project, query, intent, scored, maxItems, docByPath, &warnings)
	if len(scored) > maxItems {
		scored = scored[:maxItems]
	}

	// Build results
	var results []model.RecallResult
	for _, sc := range scored {
		doc := docByPath[sc.embedding.DocPath]
		results = append(results, model.RecallResult{
			Query:     query,
			Intent:    intent,
			Archivo:   sc.embedding.DocPath,
			Heading:   sc.embedding.Heading,
			Score:     sc.score,
			Snippet:   sc.embedding.Snippet,
			StartLine: sc.embedding.StartLine,
			EndLine:   sc.embedding.EndLine,
			Why:       recallWhyWithRerank(intent, sc.embedding, doc, sc.reranked),
		})
	}

	// Apply map mode if requested
	if mapMode {
		// Group by archivo and heading for compact display
		results = compactRecallResults(results)
	}

	return model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "recall",
		Mode:      "semantic",
		Items:     results,
		Stats:     model.Stats{Files: len(results)},
		Warnings:  warnings,
		Hint:      recallIntentHint(intent),
	}, nil
}

func applyRecallRerank(ctx context.Context, project model.ProjectFile, query string, intent string, scored []scoredChunk, maxItems int, docByPath map[string]model.DocRecord, warnings *[]string) []scoredChunk {
	if project.Recall == nil || !project.Recall.RerankExtension.Active() || len(scored) == 0 {
		return scored
	}
	cfgBlock := project.Recall.RerankExtension
	cfg := rerank.Config{
		Command:         cfgBlock.Command,
		Args:            append([]string(nil), cfgBlock.Args...),
		TimeoutMS:       cfgBlock.TimeoutMS,
		CandidateCount:  cfgBlock.CandidateCount,
		TopN:            cfgBlock.TopN,
		MaxSnippetChars: cfgBlock.MaxSnippetChars,
	}
	candidateCount := cfg.CandidateCount
	if candidateCount <= 0 {
		candidateCount = 50
		if maxItems > candidateCount {
			candidateCount = maxItems
		}
	}
	if candidateCount > len(scored) {
		candidateCount = len(scored)
	}
	if candidateCount <= 0 {
		return scored
	}

	candidates := make([]rerank.Candidate, 0, candidateCount)
	for i, sc := range scored[:candidateCount] {
		doc := docByPath[sc.embedding.DocPath]
		candidates = append(candidates, rerank.Candidate{
			Index:     i,
			Archivo:   sc.embedding.DocPath,
			Heading:   sc.embedding.Heading,
			Snippet:   sc.embedding.Snippet,
			Score:     sc.score,
			StartLine: sc.embedding.StartLine,
			EndLine:   sc.embedding.EndLine,
			Why:       recallWhy(intent, sc.embedding, doc),
		})
	}

	outcome, err := rerank.Execute(ctx, cfg, query, candidates)
	if err != nil {
		kind := "failed"
		if safeErr, ok := err.(*rerank.SafeError); ok && safeErr.Kind != "" {
			kind = safeErr.Kind
		}
		*warnings = append(*warnings, fmt.Sprintf("rerank extension %s; preserved semantic order", kind))
		return scored
	}
	if len(outcome.Warnings) > 0 {
		*warnings = append(*warnings, outcome.Warnings...)
	}
	if len(outcome.Order) == 0 {
		return scored
	}

	ordered := make([]scoredChunk, 0, len(scored))
	for _, index := range outcome.Order {
		if index < 0 || index >= candidateCount {
			continue
		}
		next := scored[index]
		if outcome.Applied[index] {
			next.reranked = true
		}
		ordered = append(ordered, next)
	}
	if len(scored) > candidateCount {
		ordered = append(ordered, scored[candidateCount:]...)
	}
	return ordered
}

type wikiDocMetadata struct {
	DocumentKey string
	BodyRole    string
	Tags        string
}

func parseWikiDocMetadata(content string, doc model.DocRecord) wikiDocMetadata {
	meta := wikiDocMetadata{DocumentKey: firstNonEmpty(doc.DocID, doc.Title), BodyRole: doc.Family}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return meta
	}
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "documentkey", "document_key":
			meta.DocumentKey = value
		case "body_role", "bodyrole":
			meta.BodyRole = value
		case "tags":
			meta.Tags = value
		}
	}
	return meta
}

func embeddingTextForChunk(doc model.DocRecord, meta wikiDocMetadata, chunk wikichunk.Chunk) string {
	prefix := []string{
		"mi-lsp retrieval document metadata:",
		"documentKey: " + firstNonEmpty(meta.DocumentKey, doc.DocID, doc.Title, doc.Path),
		"body_role: " + firstNonEmpty(meta.BodyRole, doc.Family, doc.Layer),
		"tags: " + meta.Tags,
		"path: " + doc.Path,
		"title: " + doc.Title,
		"layer: " + doc.Layer,
		"family: " + doc.Family,
		"heading: " + chunk.Heading,
		"",
		"content:",
	}
	return strings.Join(prefix, "\n") + "\n" + chunk.Text
}

func hashEmbeddingText(chunkHash string, embeddingText string) string {
	sum := sha256.Sum256([]byte(chunkHash + "\x00qwen-metadata-v1\x00" + embeddingText))
	return hex.EncodeToString(sum[:])
}

func normalizeRecallIntent(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "formula", "calculation", "calc":
		return "formula"
	case "evidence", "source", "sources":
		return "evidence"
	case "route", "routing", "worker":
		return "route"
	case "learning", "memory", "aprendizaje":
		return "learning"
	default:
		return "explore"
	}
}

func recallQueryText(query string, intent string) string {
	switch normalizeRecallIntent(intent) {
	case "formula":
		return "Retrieve source-grounded formula and calculation contract passages. Prefer validated formula contracts, evidence matrices, worked examples, fixtures, units, ranges and stop conditions. Avoid aliases, roadmaps and worker profiles unless no source exists.\nQuery: " + query
	case "evidence":
		return "Retrieve canonical source evidence, document keys, source ids, page or section pointers, and evidence matrices. Prefer .library source-grounded notes and manifests.\nQuery: " + query
	case "route":
		return "Retrieve canonical worker profiles and domain routing notes that identify which Kraal worker should handle the request. Prefer worker profile documents and routing contracts.\nQuery: " + query
	case "learning":
		return "Retrieve durable learning, prior decisions, operational notes, memory quality rules and assistant improvement guidance.\nQuery: " + query
	default:
		return "Retrieve broad Kraal context, synthesis notes, indexes, prior decisions and relevant canonical project knowledge.\nQuery: " + query
	}
}

func recallIntentBoost(intent string, emb model.WikiChunkEmbedding, doc model.DocRecord) float64 {
	haystack := strings.ToLower(strings.Join([]string{doc.Path, doc.Title, doc.DocID, doc.Layer, doc.Family, doc.Snippet, emb.Heading, emb.Snippet}, "\n"))
	path := strings.ToLower(doc.Path)
	score := 0.0

	isWorker := strings.Contains(path, ".docs/wiki/workers/") || strings.Contains(path, "skills/brewing/")
	isLibrary := strings.Contains(path, ".library/")
	isContract := strings.Contains(haystack, "contract") || strings.Contains(haystack, "contrato")
	isEvidence := strings.Contains(haystack, "evidence") || strings.Contains(haystack, "source-grounded") || strings.Contains(haystack, "source note")
	isFormula := strings.Contains(haystack, "formula") || strings.Contains(haystack, "calculation") || strings.Contains(haystack, "fixture")
	isAlias := strings.Contains(haystack, "search aliases") || strings.Contains(haystack, "aliases")
	isRoadmap := strings.Contains(haystack, "roadmap") || strings.Contains(haystack, "future")
	isSynthesis := strings.Contains(haystack, "synthesis") || strings.Contains(haystack, "index")
	isLearning := strings.Contains(haystack, "learning") || strings.Contains(haystack, "aprendizaje")

	switch normalizeRecallIntent(intent) {
	case "formula":
		if isLibrary {
			score += 0.08
		}
		if isContract || isEvidence || isFormula {
			score += 0.18
		}
		if isAlias {
			score -= 0.20
		}
		if isWorker {
			score -= 0.25
		}
		if isRoadmap && !(isContract || isEvidence || isFormula) {
			score -= 0.08
		}
	case "evidence":
		if isLibrary {
			score += 0.10
		}
		if isEvidence || isContract {
			score += 0.12
		}
		if isWorker {
			score -= 0.08
		}
	case "route":
		if isWorker {
			score += 0.28
		}
		if isLibrary && !isWorker {
			score -= 0.04
		}
	case "learning":
		if isLearning {
			score += 0.18
		}
		if isWorker {
			score -= 0.04
		}
	default:
		if isSynthesis {
			score += 0.06
		}
	}
	return score
}

func recallWhy(intent string, emb model.WikiChunkEmbedding, doc model.DocRecord) []string {
	why := []string{"semantic_match", "intent_" + normalizeRecallIntent(intent)}
	boost := recallIntentBoost(intent, emb, doc)
	if boost > 0 {
		why = append(why, "intent_boost")
	}
	if boost < 0 {
		why = append(why, "intent_penalty")
	}
	return why
}

func recallWhyWithRerank(intent string, emb model.WikiChunkEmbedding, doc model.DocRecord, reranked bool) []string {
	why := recallWhy(intent, emb, doc)
	if reranked {
		why = append(why, "external_rerank")
	}
	return why
}

func recallIntentHint(intent string) string {
	switch normalizeRecallIntent(intent) {
	case "formula":
		return "intent=formula prioritizes source-grounded contracts/evidence and penalizes aliases or worker profiles; final numbers still require validated contracts."
	case "route":
		return "intent=route prioritizes worker profiles for dispatch; route hits are not final source evidence."
	case "learning":
		return "intent=learning prioritizes durable learning and operational memory."
	case "evidence":
		return "intent=evidence prioritizes .library source-grounded notes, source ids and evidence matrices."
	default:
		return "intent=explore prioritizes broad synthesis and project context."
	}
}

// compactRecallResults groups results for --map mode, keeping them as RecallResult but marking why as map-relevant.
func compactRecallResults(results []model.RecallResult) []model.RecallResult {
	// For now, just reuse the ranking but update Why to indicate map context
	for i := range results {
		results[i].Why = []string{"map_context"}
	}
	return results
}

// searchPatternHelper is a helper to do lexical search; mirrors patterns from ask.go
func searchPatternHelper(ctx context.Context, root string, project model.ProjectFile, pattern string, useRegex bool, limit int) ([]map[string]any, error) {
	searchCtx, cancel := withSearchTimeout(ctx, 10*time.Second)
	defer cancel()
	return searchPatternScoped(searchCtx, root, root, project, pattern, useRegex, limit)
}
