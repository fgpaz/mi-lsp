package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
)

func (a *App) wikiIndex(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	if blockedEnv, err := a.governanceGateEnvelope(ctx, request, "nav.wiki.index"); err != nil {
		return model.Envelope{}, err
	} else if blockedEnv != nil {
		return *blockedEnv, nil
	}
	kind, _ := request.Payload["kind"].(string)
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return model.Envelope{}, fmt.Errorf("kind is required; example: mi-lsp nav wiki index requirement --workspace <alias>")
	}
	registration, project, err := a.resolveWorkspaceWithProjectForNavigation(request)
	if err != nil {
		return model.Envelope{}, err
	}
	wikiRoots, err := harnessWikiRoots(registration, project)
	if err != nil {
		return model.Envelope{}, err
	}
	memory, _ := loadReentryMemory(ctx, registration.Root)
	query := loadDocQueryContext(ctx, registration, "wiki-index")
	defer query.Close()
	if query.dbErr != nil {
		return model.Envelope{}, query.dbErr
	}
	warnings := append([]string{}, query.profileWarnings...)
	viewDocs := loadWikiViewDocs(registration.Root, filterGovernedWikiDocRecordsInRoots(registration.Root, query.docs, map[string]any{}, wikiRoots))
	result := docgraph.BuildWikiIndex(viewDocs, kind)
	warnings = append(warnings, result.Warnings...)
	env := model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "wiki.index",
		Items:     []any{result},
		Warnings:  warnings,
	}
	return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
}

func (a *App) wikiMatrix(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	if blockedEnv, err := a.governanceGateEnvelope(ctx, request, "nav.wiki.matrix"); err != nil {
		return model.Envelope{}, err
	} else if blockedEnv != nil {
		return *blockedEnv, nil
	}
	registration, project, err := a.resolveWorkspaceWithProjectForNavigation(request)
	if err != nil {
		return model.Envelope{}, err
	}
	wikiRoots, err := harnessWikiRoots(registration, project)
	if err != nil {
		return model.Envelope{}, err
	}
	memory, _ := loadReentryMemory(ctx, registration.Root)
	query := loadDocQueryContext(ctx, registration, "wiki-matrix")
	defer query.Close()
	if query.dbErr != nil {
		return model.Envelope{}, query.dbErr
	}
	warnings := append([]string{}, query.profileWarnings...)
	if len(query.docs) == 0 {
		hint := fmt.Sprintf("documentation index is empty; rerun 'mi-lsp index --workspace %s --docs-only' before wiki matrix", registration.Name)
		warnings = append(warnings, hint)
	}
	viewDocs := loadWikiViewDocs(registration.Root, filterGovernedWikiDocRecordsInRoots(registration.Root, query.docs, map[string]any{}, wikiRoots))
	result := docgraph.BuildWikiMatrix(viewDocs)
	warnings = append(warnings, result.Warnings...)
	env := model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "wiki.matrix",
		Items:     []any{result},
		Warnings:  warnings,
	}
	return applyCoachPolicy(attachMemoryPointer(env, memory), request.Context), nil
}

func loadWikiViewDocs(root string, records []model.DocRecord) []docgraph.WikiViewDoc {
	docs := make([]docgraph.WikiViewDoc, 0, len(records))
	seen := map[string]struct{}{}
	for _, record := range records {
		if strings.TrimSpace(record.Path) == "" {
			continue
		}
		key := strings.ToLower(filepath.ToSlash(record.Path))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		path := filepath.Join(root, filepath.FromSlash(record.Path))
		body, err := os.ReadFile(path)
		content := ""
		if err == nil {
			content = string(body)
		}
		docs = append(docs, docgraph.WikiViewDoc{
			Path:    record.Path,
			DocID:   record.DocID,
			Title:   record.Title,
			Content: content,
		})
	}
	return docs
}
