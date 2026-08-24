package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

type wikiMapHub struct {
	ID    string       `json:"id"`
	Title string       `json:"title"`
	Docs  []wikiMapDoc `json:"docs"`
}

type wikiMapDoc struct {
	Path  string `json:"path"`
	Title string `json:"title,omitempty"`
}

var wikiMapHubOrder = []struct {
	id    string
	title string
}{
	{id: "persona", title: "Persona"},
	{id: "proyectos", title: "Proyectos"},
	{id: "sistema", title: "Sistema"},
	{id: "materia", title: "Materia"},
}

func (a *App) wikiMap(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	started := time.Now()
	registration, _, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}

	docs, source, warnings := loadWikiMapDocs(ctx, registration)
	hubs := groupWikiMapDocs(docs)
	hubs = trimWikiMapHubs(hubs, request.Context.MaxItems, request.Context.TokenBudget)

	env := model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "wiki.map",
		Items:     hubs,
		Warnings:  warnings,
		Stats: model.Stats{
			Files:          wikiMapDocCount(hubs),
			Ms:             time.Since(started).Milliseconds(),
			TokensEstimate: estimateWikiMapTokens(hubs),
		},
	}
	if source == "walk" {
		env.Warnings = appendStringIfMissing(env.Warnings, "wiki map used filesystem walk because the documentation index is empty")
	}
	if len(hubs) == 0 {
		env.Hint = "no se encontraron hubs de wiki en wiki/ o bibliotecas/"
	}
	return applyCoachPolicy(env, request.Context), nil
}

func loadWikiMapDocs(ctx context.Context, registration model.WorkspaceRegistration) ([]wikiMapDoc, string, []string) {
	var warnings []string
	db, err := openWorkspaceDB(registration, "wiki.map", true)
	if err == nil {
		defer db.Close()
		records, listErr := store.ListDocRecords(ctx, db)
		if listErr == nil && len(records) > 0 {
			docs := make([]wikiMapDoc, 0, len(records))
			for _, record := range records {
				if classifyWikiMapHub(record.Path) == "" {
					continue
				}
				docs = append(docs, wikiMapDoc{Path: record.Path, Title: record.Title})
			}
			if len(docs) > 0 {
				return docs, "index", warnings
			}
		}
	}
	return walkWikiMapDocs(registration.Root), "walk", warnings
}

func walkWikiMapDocs(root string) []wikiMapDoc {
	docs := make([]wikiMapDoc, 0)
	for _, dir := range []string{"wiki", "bibliotecas"} {
		base := filepath.Join(root, dir)
		_ = filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry.IsDir() {
				if skipWikiMapDir(entry.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if !strings.EqualFold(filepath.Ext(rel), ".md") {
				return nil
			}
			if classifyWikiMapHub(rel) == "" {
				return nil
			}
			title := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
			if content, readErr := os.ReadFile(path); readErr == nil {
				if extracted := firstMarkdownHeading(content); extracted != "" {
					title = extracted
				}
			}
			docs = append(docs, wikiMapDoc{Path: rel, Title: title})
			return nil
		})
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	return docs
}

func groupWikiMapDocs(docs []wikiMapDoc) []wikiMapHub {
	grouped := map[string][]wikiMapDoc{}
	for _, doc := range docs {
		hub := classifyWikiMapHub(doc.Path)
		if hub == "" {
			continue
		}
		grouped[hub] = append(grouped[hub], doc)
	}
	hubs := make([]wikiMapHub, 0, len(wikiMapHubOrder))
	for _, meta := range wikiMapHubOrder {
		items := grouped[meta.id]
		if len(items) == 0 {
			continue
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
		hubs = append(hubs, wikiMapHub{ID: meta.id, Title: meta.title, Docs: items})
	}
	return hubs
}

func classifyWikiMapHub(path string) string {
	p := filepath.ToSlash(strings.TrimSpace(path))
	lower := strings.ToLower(p)
	if strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") {
		return ""
	}
	if wikiMapExcludedPath(p) {
		return ""
	}
	if p == "bibliotecas" || strings.HasPrefix(p, "bibliotecas/") {
		return "materia"
	}
	if !strings.HasPrefix(p, "wiki/") {
		return ""
	}
	rest := strings.TrimPrefix(p, "wiki/")
	first := rest
	firstLevel := true
	if i := strings.Index(rest, "/"); i >= 0 {
		first = rest[:i]
		firstLevel = false
	} else {
		first = strings.TrimSuffix(rest, filepath.Ext(rest))
	}
	lowerFirst := strings.ToLower(first)
	if strings.HasPrefix(lowerFirst, "31-workers") || strings.HasPrefix(lowerFirst, "32-contratos") {
		return ""
	}
	if len(first) < 2 || first[0] < '0' || first[0] > '9' || first[1] < '0' || first[1] > '9' {
		return ""
	}
	nn := int(first[0]-'0')*10 + int(first[1]-'0')
	switch {
	case nn >= 0 && nn <= 9:
		if firstLevel {
			return "persona"
		}
	case nn >= 10 && nn <= 19:
		return "proyectos"
	case nn >= 20 && nn <= 30:
		if firstLevel || strings.HasPrefix(lowerFirst, "30-dashboard") {
			return "sistema"
		}
	}
	return ""
}

func wikiMapExcludedPath(path string) bool {
	lower := "/" + strings.ToLower(filepath.ToSlash(path)) + "/"
	for _, seg := range []string{"/old/", "/archive/", "/deprecated/", "/historico/", "/legacy/"} {
		if strings.Contains(lower, seg) {
			return true
		}
	}
	return false
}

func skipWikiMapDir(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch lower {
	case "old", "archive", "deprecated", "historico", "legacy":
		return true
	}
	return strings.HasPrefix(lower, "31-workers") || strings.HasPrefix(lower, "32-contratos")
}

func trimWikiMapHubs(hubs []wikiMapHub, maxItems, tokenBudget int) []wikiMapHub {
	if len(hubs) == 0 {
		return hubs
	}
	total := wikiMapDocCount(hubs)
	limit := total
	if maxItems > 0 && maxItems < limit {
		limit = maxItems
	}
	for limit > 0 {
		trimmed := copyWikiMapHubs(hubs)
		kept := 0
		for i := range trimmed {
			if kept >= limit {
				trimmed[i].Docs = nil
				continue
			}
			remain := limit - kept
			if remain < len(trimmed[i].Docs) {
				trimmed[i].Docs = trimmed[i].Docs[:remain]
			}
			kept += len(trimmed[i].Docs)
		}
		trimmed = dropEmptyWikiMapHubs(trimmed)
		if tokenBudget <= 0 || estimateWikiMapTokens(trimmed) <= tokenBudget {
			return trimmed
		}
		limit--
	}
	return nil
}

func copyWikiMapHubs(hubs []wikiMapHub) []wikiMapHub {
	out := make([]wikiMapHub, len(hubs))
	for i, hub := range hubs {
		out[i] = wikiMapHub{ID: hub.ID, Title: hub.Title, Docs: append([]wikiMapDoc(nil), hub.Docs...)}
	}
	return out
}

func dropEmptyWikiMapHubs(hubs []wikiMapHub) []wikiMapHub {
	out := make([]wikiMapHub, 0, len(hubs))
	for _, hub := range hubs {
		if len(hub.Docs) == 0 {
			continue
		}
		out = append(out, hub)
	}
	return out
}

func wikiMapDocCount(hubs []wikiMapHub) int {
	n := 0
	for _, hub := range hubs {
		n += len(hub.Docs)
	}
	return n
}

func estimateWikiMapTokens(hubs []wikiMapHub) int {
	payload, err := json.Marshal(hubs)
	if err != nil {
		return 0
	}
	return (len(payload) + 3) / 4
}

func firstMarkdownHeading(content []byte) string {
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			return strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
	}
	return ""
}
