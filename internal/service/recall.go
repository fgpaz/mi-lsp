package service

import (
	"context"
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

	// Check if embeddings are enabled and configured
	if project.Embeddings == nil || !project.Embeddings.Enabled || project.Embeddings.BaseURL == "" || project.Embeddings.Model == "" {
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
		Provider:    project.Embeddings.Provider,
		BaseURL:     project.Embeddings.BaseURL,
		Model:       project.Embeddings.Model,
		APIKeyEnv:   project.Embeddings.APIKeyEnv,
		Dim:         project.Embeddings.Dim,
		BatchSize:   project.Embeddings.BatchSize,
		TimeoutMS:   project.Embeddings.TimeoutMS,
	}
	if cfg.APIKeyEnv == "" {
		cfg.APIKeyEnv = "MI_LSP_EMBEDDINGS_API_KEY"
	}
	client := embed.New(cfg)

	// Process each doc
	for _, doc := range docs {
		docPath := doc.Path
		// Skip if not under .docs (only wiki docs)
		if !strings.HasPrefix(docPath, ".docs"+string(filepath.Separator)) && docPath != ".docs" {
			continue
		}

		// Read file
		filePath := filepath.Join(root, docPath)
		content, err := os.ReadFile(filePath)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("embeddings: failed to read %s: %v", docPath, err))
			continue
		}

		// Chunk by heading
		chunks := wikichunk.ChunkByHeading(string(content))
		indexedDocPaths = append(indexedDocPaths, docPath)

		// Build candidates and check if re-embedding is needed
		var textsToEmbed []string
		var chunkIndicesForEmbedding []int

		for chunkIdx, chunk := range chunks {
			key := docPath + "\x00" + chunk.ChunkID
			existing, exists := existingEmbeddings[key]

			// Check if we can reuse existing embedding
			if exists && existing.ContentHash == chunk.ContentHash && existing.EmbeddingModel == cfg.Model && existing.EmbeddingDim == cfg.Dim && existing.Embedding != nil {
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
					ContentHash:    chunk.ContentHash,
					EmbeddingModel: cfg.Model,
					StartLine:      chunk.StartLine,
					EndLine:        chunk.EndLine,
					EmbeddingDim:   cfg.Dim,
					Embedding:      existing.Embedding,
					IndexedAt:      existing.IndexedAt,
				})
			} else {
				// Need to embed
				textsToEmbed = append(textsToEmbed, chunk.Text)
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
				snippet := c.Text
				if len(snippet) > 200 {
					snippet = snippet[:200]
				}
				allChunks[len(allChunks)-len(textsToEmbed)+i] = model.WikiChunkEmbedding{
					DocPath:        docPath,
					ChunkID:        c.ChunkID,
					Heading:        c.Heading,
					Snippet:        snippet,
					ContentHash:    c.ContentHash,
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

	// Check if embeddings are configured
	if project.Embeddings == nil || !project.Embeddings.Enabled || project.Embeddings.BaseURL == "" || project.Embeddings.Model == "" {
		// Return hint without calling embeddings
		hint := "embeddings not configured; configure [embeddings] section in .mi-lsp/project.toml or use 'mi-lsp nav search' for lexical search"
		return model.Envelope{
			Ok:       true,
			Workspace: registration.Name,
			Backend:  "recall",
			Items:    []model.RecallResult{},
			Hint:     hint,
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
		Provider:    project.Embeddings.Provider,
		BaseURL:     project.Embeddings.BaseURL,
		Model:       project.Embeddings.Model,
		APIKeyEnv:   project.Embeddings.APIKeyEnv,
		Dim:         project.Embeddings.Dim,
		BatchSize:   project.Embeddings.BatchSize,
		TimeoutMS:   project.Embeddings.TimeoutMS,
	}
	if cfg.APIKeyEnv == "" {
		cfg.APIKeyEnv = "MI_LSP_EMBEDDINGS_API_KEY"
	}
	client := embed.New(cfg)

	// Try to embed the query
	queryVector, err := client.EmbedOne(ctx, query)
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
				Query:     query,
				Archivo:   path,
				Snippet:   snippet,
				Score:     0,
				Why:       []string{"lexical_fallback"},
			})
		}

		warnings := []string{fmt.Sprintf("embeddings unavailable (%v); served lexical results", err)}
		hint := "embeddings endpoint offline; results are from lexical search. Fix embeddings config to enable semantic search."

		return model.Envelope{
			Ok:       true,
			Workspace: registration.Name,
			Backend:  "recall+lexical",
			Items:    results,
			Warnings: warnings,
			Hint:     hint,
			Stats:    model.Stats{Files: len(results)},
		}, nil
	}

	// Load all embeddings
	allEmbeddings, err := store.AllWikiChunkEmbeddings(ctx, db)
	if err != nil {
		return model.Envelope{}, err
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
		results = append(results, model.RecallResult{
			Query:     query,
			Archivo:   sc.embedding.DocPath,
			Heading:   sc.embedding.Heading,
			Score:     sc.score,
			Snippet:   sc.embedding.Snippet,
			StartLine: sc.embedding.StartLine,
			EndLine:   sc.embedding.EndLine,
			Why:       []string{"semantic_match"},
		})
	}

	// Apply map mode if requested
	if mapMode {
		// Group by archivo and heading for compact display
		results = compactRecallResults(results)
	}

	return model.Envelope{
		Ok:       true,
		Workspace: registration.Name,
		Backend:  "recall",
		Mode:     "semantic",
		Items:    results,
		Stats:    model.Stats{Files: len(results)},
	}, nil
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
