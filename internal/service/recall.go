package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/embed"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/wikichunk"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// embedWorkspaceWiki indexes wiki chunks with embeddings after a successful doc publish.
// Returns warnings (never fails the caller).
func (a *App) embedWorkspaceWiki(ctx context.Context, root string) []string {
	var warnings []string

	// Load project file for embeddings config
	project, err := workspace.LoadProjectFile(root)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("embeddings: failed to load project config: %v", err))
		return warnings
	}

	// Check if embeddings are active and configured.
	if !project.Embeddings.Active() {
		// No-op: embeddings not configured
		return nil
	}

	// Open the repo-local store
	db, err := store.Open(root)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("embeddings: failed to open store: %v", err))
		return warnings
	}
	defer db.Close()

	// Load all doc records to get the indexed docs
	docs, err := store.ListDocRecords(ctx, db)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("embeddings: failed to load doc records: %v", err))
		return warnings
	}

	var indexedDocPaths []string
	var allChunks []model.WikiChunkEmbedding

	// For each doc, chunk and embed
	existingEmbeddings, err := store.LoadWikiChunkEmbeddings(ctx, db)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("embeddings: failed to load existing embeddings: %v", err))
		return warnings
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

	// Process each doc. doc records are the markdown docs the indexer selected
	// (any knowledge-wiki layout: .docs/wiki, .library, docs/, README, ...). We embed
	// all of them. NOTE: doc.Path uses forward slashes; do not filter with
	// filepath.Separator (that broke on Windows and skipped every doc).
	for _, doc := range docs {
		docPath := doc.Path

		// Read file
		filePath := filepath.Join(root, filepath.FromSlash(docPath))
		content, err := os.ReadFile(filePath)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("embeddings: failed to read %s: %v", docPath, err))
			continue
		}

		docMeta := parseWikiDocMetadata(string(content), doc)

		// Chunk by heading
		chunks := wikichunk.ChunkByHeading(string(content))
		indexedDocPaths = append(indexedDocPaths, docPath)

		// Build candidates and check if re-embedding is needed
		var textsToEmbed []string
		var chunkIndicesForEmbedding []int

		for chunkIdx, chunk := range chunks {
			embeddingText := embeddingTextForChunk(doc, docMeta, chunk)
			embeddingHash := hashEmbeddingText(chunk.ContentHash, embeddingText)
			key := docPath + "\x00" + chunk.ChunkID
			existing, exists := existingEmbeddings[key]

			// Check if we can reuse existing embedding
			if exists && existing.ContentHash == embeddingHash && existing.EmbeddingModel == cfg.Model && existing.EmbeddingDim == cfg.Dim && existing.Embedding != nil {
				// Reuse
				snippet := chunk.Text
				if len(snippet) > 200 {
					snippet = snippet[:200]
				}
				allChunks = append(allChunks, model.WikiChunkEmbedding{
					DocPath:        docPath,
					ChunkID:        chunk.ChunkID,
					Heading:        chunk.Heading,
					Snippet:        snippet,
					ContentHash:    embeddingHash,
					EmbeddingModel: cfg.Model,
					StartLine:      chunk.StartLine,
					EndLine:        chunk.EndLine,
					EmbeddingDim:   cfg.Dim,
					Embedding:      existing.Embedding,
					IndexedAt:      existing.IndexedAt,
				})
			} else {
				// Need to embed
				textsToEmbed = append(textsToEmbed, embeddingText)
				chunkIndicesForEmbedding = append(chunkIndicesForEmbedding, chunkIdx)
				// Reserve space
				allChunks = append(allChunks, model.WikiChunkEmbedding{})
			}
		}

		// If there are texts to embed, call the API
		if len(textsToEmbed) > 0 {
			embeddings, err := client.Embed(ctx, textsToEmbed)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("embeddings: failed to embed chunks from %s: %v", docPath, err))
				// Remove the reserved spaces
				allChunks = allChunks[:len(allChunks)-len(textsToEmbed)]
				continue
			}

			// Populate the embeddings
			for i, emb := range embeddings {
				chunkIdx := chunkIndicesForEmbedding[i]
				c := chunks[chunkIdx]
				embeddingText := embeddingTextForChunk(doc, docMeta, c)
				snippet := c.Text
				if len(snippet) > 200 {
					snippet = snippet[:200]
				}
				allChunks[len(allChunks)-len(textsToEmbed)+i] = model.WikiChunkEmbedding{
					DocPath:        docPath,
					ChunkID:        c.ChunkID,
					Heading:        c.Heading,
					Snippet:        snippet,
					ContentHash:    hashEmbeddingText(c.ContentHash, embeddingText),
					EmbeddingModel: cfg.Model,
					StartLine:      c.StartLine,
					EndLine:        c.EndLine,
					EmbeddingDim:   cfg.Dim,
					Embedding:      embed.EncodeVector(emb),
					IndexedAt:      time.Now().Unix(),
				}
			}
		}
	}

	// Replace embeddings in DB
	if err := store.ReplaceWikiChunkEmbeddingsForDocs(ctx, db, indexedDocPaths, allChunks); err != nil {
		warnings = append(warnings, fmt.Sprintf("embeddings: failed to store embeddings: %v", err))
		return warnings
	}

	return warnings
}

func (a *App) appendWikiEmbeddingWarnings(ctx context.Context, root string, warnings []string) []string {
	if embedWarnings := a.embedWorkspaceWiki(ctx, root); len(embedWarnings) > 0 {
		warnings = append(warnings, embedWarnings...)
	}
	return warnings
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
		hint := "embeddings not configured; configure [embeddings] section in .mi-lsp/project.toml or use 'mi-lsp nav search' for lexical search"
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

		warnings := []string{fmt.Sprintf("embeddings unavailable (%v); served lexical results", err)}
		hint := "embeddings endpoint offline; results are from lexical search. Fix embeddings config to enable semantic search."

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
	type scoredChunk struct {
		embedding model.WikiChunkEmbedding
		score     float64
	}
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
			Why:       recallWhy(intent, sc.embedding, doc),
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
		Hint:      recallIntentHint(intent),
	}, nil
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
