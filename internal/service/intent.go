package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

type intentMatch struct {
	Symbol   model.SymbolRecord
	Score    float64
	Evidence string
}

func (a *App) intent(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	registration, project, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}

	question, _ := request.Payload["question"].(string)
	question = strings.TrimSpace(question)
	if question == "" {
		return model.Envelope{}, fmt.Errorf("question is required for nav intent")
	}

	topN := intFromAny(request.Payload["top"], 10)
	if topN <= 0 {
		topN = 10
	}
	offset := intFromAny(request.Payload["offset"], 0)

	scopedRepo, scopeEnvelope := resolveCatalogRepoScope(registration, project, request.Payload)
	if scopeEnvelope != nil {
		return *scopeEnvelope, nil
	}

	db, err := store.Open(registration.Root)
	if err != nil {
		return model.Envelope{}, err
	}
	defer db.Close()

	tokens := docgraph.QuestionTokens(question)
	if len(tokens) == 0 {
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "intent", Items: []map[string]any{}, Warnings: []string{"query produced no tokens after normalization"}}, nil
	}

	queryLimit := topN * 5
	sqlOffset := offset
	if scopedRepo != nil {
		queryLimit = max((offset+topN)*10, 100)
		sqlOffset = 0
	}
	candidates, err := store.IntentSearch(ctx, db, tokens, queryLimit, sqlOffset)
	if err != nil {
		return model.Envelope{}, err
	}
	candidates = filterSymbolsByRepo(candidates, scopedRepo)
	if offset > 0 {
		if offset >= len(candidates) {
			candidates = []model.SymbolRecord{}
		} else {
			candidates = candidates[offset:]
		}
	}

	if len(candidates) == 0 {
		return model.Envelope{Ok: true, Workspace: registration.Name, Backend: "intent", Items: []map[string]any{}, Warnings: []string{"no symbols matched intent tokens"}}, nil
	}

	scored := scoreBM25(candidates, tokens)
	if len(scored) > topN {
		scored = scored[:topN]
	}

	items := make([]map[string]any, len(scored))
	for i, match := range scored {
		items[i] = map[string]any{
			"file":           match.Symbol.FilePath,
			"line":           match.Symbol.StartLine,
			"symbol":         match.Symbol.Name,
			"kind":           match.Symbol.Kind,
			"qualified_name": match.Symbol.QualifiedName,
			"score":          fmt.Sprintf("%.2f", match.Score),
			"evidence":       match.Evidence,
			"snippet":        intentSnippet(match.Symbol),
		}
	}

	return model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "intent",
		Items:     items,
		Stats:     model.Stats{Symbols: len(items)},
	}, nil
}

func scoreBM25(symbols []model.SymbolRecord, tokens []string) []intentMatch {
	// Compute document frequency per token
	docFreq := make(map[string]int)
	for _, sym := range symbols {
		seen := make(map[string]struct{})
		searchLower := strings.ToLower(sym.SearchText)
		for _, token := range tokens {
			if strings.Contains(searchLower, token) {
				if _, ok := seen[token]; !ok {
					docFreq[token]++
					seen[token] = struct{}{}
				}
			}
		}
	}

	totalDocs := float64(len(symbols))
	scored := make([]intentMatch, 0, len(symbols))

	for _, sym := range symbols {
		score := 0.0
		evidence := ""
		searchLower := strings.ToLower(sym.SearchText)
		nameLower := strings.ToLower(sym.Name)

		for _, token := range tokens {
			if !strings.Contains(searchLower, token) {
				continue
			}

			count := float64(strings.Count(searchLower, token))

			// IDF: log(N / df)
			idf := 1.0
			if df, ok := docFreq[token]; ok && df > 0 {
				idf = 1.0 + math.Log(totalDocs/float64(df))
			}

			termScore := count * idf

			// Positional boosts
			if strings.Contains(nameLower, token) {
				termScore *= 3.0
				if evidence == "" {
					evidence = "name_match"
				}
			}
			kindLower := strings.ToLower(sym.Kind)
			if strings.Contains(kindLower, token) {
				termScore *= 2.0
			}
			if sym.Parent != "" && strings.Contains(strings.ToLower(sym.Parent), token) {
				termScore *= 1.5
			}

			score += termScore
		}

		if score > 0 {
			if evidence == "" {
				evidence = "search_text_match"
			}
			scored = append(scored, intentMatch{
				Symbol:   sym,
				Score:    score,
				Evidence: evidence,
			})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	return scored
}

func intentSnippet(sym model.SymbolRecord) string {
	if sym.Signature != "" {
		return sym.Signature
	}
	if sym.Parent != "" {
		return sym.Parent + "." + sym.Name
	}
	return sym.Name
}
