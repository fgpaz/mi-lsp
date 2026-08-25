package service

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

type wikiMapHub struct {
	ID        string       `json:"id"`
	Title     string       `json:"title"`
	Docs      []wikiMapDoc `json:"docs"`
	TotalDocs int          `json:"total_docs,omitempty"`
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

	profile, _, warnings := docgraph.LoadProfile(registration.Root)
	docs, source, loadWarnings, loadErr := loadWikiMapDocs(ctx, registration, profile)
	if loadErr != nil {
		return model.Envelope{}, loadErr
	}
	warnings = append(warnings, loadWarnings...)
	hubs := groupWikiMapDocs(docs, &profile)
	hubs, trimStats := trimWikiMapHubs(hubs, request.Context.MaxItems, request.Context.TokenBudget)

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
	if trimStats != nil {
		env.Stats.TotalDocs = trimStats.totalDocs
		env.Stats.TotalReturned = trimStats.totalReturned
		env.Stats.TruncationReason = trimStats.reason
		if trimStats.hint != "" {
			env.NextHint = &trimStats.hint
		}
		env.Truncated = trimStats.totalReturned < trimStats.totalDocs
	}
	if source == "disabled" {
		env.Hint = "wiki map está deshabilitado por read-model.toml"
	} else if len(hubs) == 0 {
		env.Hint = "no se encontraron hubs en las raíces configuradas de wiki map"
	}
	return applyCoachPolicy(env, request.Context), nil
}

type wikiMapTrimStats struct {
	totalDocs, totalReturned int
	reason, hint             string
}

func loadWikiMapDocs(ctx context.Context, registration model.WorkspaceRegistration, profile model.DocsReadProfile) ([]wikiMapDoc, string, []string, error) {
	if !profile.IsWikiMapEnabled() {
		return nil, "disabled", nil, nil
	}
	warnings := []string{}
	roots := profile.EffectiveWikiMapRoots()
	db, err := openWorkspaceDB(registration, "wiki.map", true)
	if err != nil || db == nil {
		warnings = append(warnings, "wiki_map_db_open_failed_fallback_walk")
	} else {
		defer db.Close()
		records, queryErr := store.ListDocRecordsPaths(ctx, db, roots...)
		if queryErr != nil {
			if ctx.Err() != nil {
				return nil, "index", warnings, ctx.Err()
			}
			warnings = append(warnings, "wiki_map_db_query_failed_fallback_walk")
		} else {
			docs := make([]wikiMapDoc, 0, len(records))
			for _, record := range records {
				if classifyWikiMapHub(record.Path, profile) != "" {
					docs = append(docs, wikiMapDoc{Path: record.Path, Title: record.Title})
				}
			}
			if len(docs) > 0 {
				return docs, "index", warnings, nil
			}
		}
	}
	docs, walkWarnings, walkErr := walkWikiMapDocs(ctx, registration.Root, profile)
	warnings = append(warnings, walkWarnings...)
	return docs, "walk", warnings, walkErr
}

func walkWikiMapDocs(ctx context.Context, root string, profile model.DocsReadProfile) ([]wikiMapDoc, []string, error) {
	if !profile.IsWikiMapEnabled() {
		return nil, nil, nil
	}
	matcher, matcherErr := workspace.LoadIgnoreMatcher(root, nil)
	warnings := []string{}
	if matcherErr != nil {
		warnings = append(warnings, "wiki_map_ignore_matcher_unavailable")
	}
	byPath := map[string]wikiMapDoc{}
	for _, configuredRoot := range profile.EffectiveWikiMapRoots() {
		if err := ctx.Err(); err != nil {
			return nil, warnings, err
		}
		base := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(configuredRoot, "/")))
		info, statErr := os.Lstat(base)
		if statErr != nil || !info.IsDir() {
			warnings = appendStringIfMissing(warnings, "wiki_map_root_unavailable")
			continue
		}
		if reparse, reparseErr := preparationPathReparse(base); reparseErr != nil || reparse {
			warnings = appendStringIfMissing(warnings, "wiki_map_root_reparse_skipped")
			continue
		}
		walkErr := filepath.WalkDir(base, func(path string, entry os.DirEntry, entryErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if entryErr != nil {
				warnings = appendStringIfMissing(warnings, "wiki_map_walk_entry_unavailable")
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				warnings = appendStringIfMissing(warnings, "wiki_map_symlink_skipped")
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if reparse, reparseErr := preparationPathReparse(path); reparseErr != nil || reparse {
				warnings = appendStringIfMissing(warnings, "wiki_map_reparse_skipped")
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.IsDir() {
				if matcher != nil && matcher.ShouldIgnore(root, path) {
					return filepath.SkipDir
				}
				return nil
			}
			if matcher != nil && matcher.ShouldIgnore(root, path) {
				return nil
			}
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil || !strings.EqualFold(filepath.Ext(relative), ".md") {
				return nil
			}
			relative = filepath.ToSlash(relative)
			if classifyWikiMapHub(relative, profile) == "" {
				return nil
			}
			title, titleErr := readWikiMapTitle(path, relative)
			if titleErr != nil {
				warnings = appendStringIfMissing(warnings, "wiki_map_title_read_failed")
			}
			byPath[relative] = wikiMapDoc{Path: relative, Title: title}
			return nil
		})
		if walkErr != nil {
			if ctx.Err() != nil {
				return nil, warnings, ctx.Err()
			}
			warnings = appendStringIfMissing(warnings, "wiki_map_walk_failed")
		}
	}
	docs := make([]wikiMapDoc, 0, len(byPath))
	for _, doc := range byPath {
		docs = append(docs, doc)
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	return docs, warnings, nil
}

func readWikiMapTitle(path, relative string) (string, error) {
	fallback := strings.TrimSuffix(filepath.Base(relative), filepath.Ext(relative))
	file, err := os.Open(path)
	if err != nil {
		return fallback, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, 64*1024))
	if err != nil {
		return fallback, err
	}
	if title := wikiMapFrontMatterTitle(content); title != "" {
		return title, nil
	}
	if heading := firstMarkdownHeading(content); heading != "" {
		return heading, nil
	}
	return fallback, nil
}

func wikiMapFrontMatterTitle(content []byte) string {
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break
		}
		key, value, found := strings.Cut(trimmed, ":")
		if found && strings.EqualFold(strings.TrimSpace(key), "title") {
			return strings.Trim(strings.TrimSpace(value), "\"'")
		}
	}
	return ""
}

func groupWikiMapDocs(docs []wikiMapDoc, profile *model.DocsReadProfile) []wikiMapHub {
	if profile == nil || !profile.IsWikiMapEnabled() {
		return nil
	}
	grouped := map[string][]wikiMapDoc{}
	for _, doc := range docs {
		hub := classifyWikiMapHub(doc.Path, *profile)
		if hub != "" {
			grouped[hub] = append(grouped[hub], doc)
		}
	}
	hubs := make([]wikiMapHub, 0)
	appendHub := func(id, title string) {
		items := grouped[id]
		if len(items) == 0 {
			return
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
		hubs = append(hubs, wikiMapHub{ID: id, Title: title, Docs: items, TotalDocs: len(items)})
	}
	if profile.HasCustomHubs() {
		for _, hub := range profile.WikiMap.Hubs {
			appendHub(hub.ID, hub.Title)
		}
		return hubs
	}
	for _, hub := range wikiMapHubOrder {
		appendHub(hub.id, hub.title)
	}
	return hubs
}

func classifyWikiMapHub(path string, profile model.DocsReadProfile) string {
	if !profile.IsWikiMapEnabled() {
		return ""
	}
	if profile.HasCustomHubs() {
		return profile.ClassifyWikiMapHubClassified(path)
	}
	return classifyWikiMapHubLocked(path)
}

func classifyWikiMapHubLocked(path string) string {
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

func trimWikiMapHubs(hubs []wikiMapHub, maxItems, tokenBudget int) ([]wikiMapHub, *wikiMapTrimStats) {
	total := wikiMapDocCount(hubs)
	limit := total
	maxLimited := maxItems > 0 && maxItems < limit
	if maxLimited {
		limit = maxItems
	}
	tokenLimited := false
	if tokenBudget > 0 && estimateWikiMapTokens(fairTrimByCount(hubs, limit)) > tokenBudget {
		tokenLimited = true
		low, high := 0, limit
		for low < high {
			mid := (low + high + 1) / 2
			if estimateWikiMapTokens(fairTrimByCount(hubs, mid)) <= tokenBudget {
				low = mid
			} else {
				high = mid - 1
			}
		}
		limit = low
	}
	out := fairTrimByCount(hubs, limit)
	stats := &wikiMapTrimStats{totalDocs: total, totalReturned: wikiMapDocCount(out)}
	if stats.totalReturned < total {
		switch {
		case tokenLimited:
			stats.reason = "token_budget"
			stats.hint = "aumente --token-budget para incluir más documentos"
		case maxLimited:
			stats.reason = "max_items"
			stats.hint = "aumente --max-items para incluir más documentos"
		}
	}
	return out, stats
}

func fairTrimByCount(hubs []wikiMapHub, count int) []wikiMapHub {
	if count < 0 {
		count = 0
	}
	out := make([]wikiMapHub, len(hubs))
	positions := make([]int, len(hubs))
	for i, hub := range hubs {
		total := len(hub.Docs)
		if hub.TotalDocs > total {
			total = hub.TotalDocs
		}
		out[i] = wikiMapHub{ID: hub.ID, Title: hub.Title, TotalDocs: total}
	}
	added := 0
	for added < count {
		progressed := false
		for i := range hubs {
			if added >= count {
				break
			}
			if positions[i] >= len(hubs[i].Docs) {
				continue
			}
			out[i].Docs = append(out[i].Docs, hubs[i].Docs[positions[i]])
			positions[i]++
			added++
			progressed = true
		}
		if !progressed {
			break
		}
	}
	return dropEmptyWikiMapHubs(out)
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
