package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

const (
	defaultWikiCodeTokenBudget = 4000
	maxWikiCodeTokenBudget     = 20000
)

var wikiCodeEnrichedOperations = map[string]bool{
	"nav.ask": true, "nav.route": true, "nav.wiki.route": true,
	"nav.pack": true, "nav.wiki.pack": true, "nav.context": true,
	"nav.affected": true, "nav.diff-context": true, "nav.workspace-map": true,
}

// enrichWikiCodeContext adds the query-only docs/code projection without changing
// the command's items, ordering, backend, or primary document selection.
func (a *App) enrichWikiCodeContext(ctx context.Context, request model.CommandRequest, env model.Envelope) model.Envelope {
	if !env.Ok || env.Backend == "governance" || !wikiCodeEnrichedOperations[request.Operation] {
		return env
	}
	if all, _ := request.Payload["all_workspaces"].(bool); all || strings.TrimSpace(request.Context.Workspace) == "" {
		return env
	}

	budget := intFromAny(request.Payload["wiki_code_token_budget"], request.Context.TokenBudget)
	if budget < 1 {
		budget = defaultWikiCodeTokenBudget
		env.Warnings = appendStringIfMissing(env.Warnings, "wiki-code context token budget invalid; using default 4000")
	} else if budget > maxWikiCodeTokenBudget {
		budget = maxWikiCodeTokenBudget
		env.Warnings = appendStringIfMissing(env.Warnings, "wiki-code context token budget capped at 20000")
	}

	path := extractWikiPrimaryPath(request.Operation, env.Items)
	if path == "" && request.Operation == "nav.workspace-map" {
		path = ".docs/wiki/02_arquitectura.md"
	}
	if path == "" {
		return env
	}

	registration, err := a.ResolveWorkspace(request.Context.Workspace)
	if err != nil {
		env.Warnings = appendStringIfMissing(env.Warnings, "wiki-code context unavailable: "+err.Error())
		return env
	}
	// Context-producing commands may legitimately serve their legacy fallback
	// while governance is blocked; never attach code evidence in that state.
	if docgraph.InspectGovernance(registration.Root, true).Blocked {
		return env
	}
	db, err := openWorkspaceDB(registration, "wiki-code-enrich", true)
	if err != nil {
		env.Warnings = appendStringIfMissing(env.Warnings, "wiki-code context unavailable: "+err.Error())
		return env
	}
	defer db.Close()

	var primary model.DocRecord
	docs, docsErr := store.ListDocRecords(ctx, db)
	if docsErr != nil {
		env.Warnings = appendStringIfMissing(env.Warnings, "wiki-code context unavailable: "+docsErr.Error())
		return env
	}
	for _, doc := range docs {
		if doc.Path == path && model.CanonicalWikiAuthority(doc.Path) && !doc.IsSnapshot {
			primary = doc
			break
		}
	}
	if primary.Path == "" {
		env.Warnings = appendStringIfMissing(env.Warnings, "wiki-code context primary document is not indexed: "+path)
		return env
	}

	codeContext, err := BuildWikiCodeContext(ctx, db, primary, budget)
	if err != nil {
		env.Warnings = appendStringIfMissing(env.Warnings, "wiki-code context unavailable: "+err.Error())
		return env
	}
	env.WikiCodeContext = &codeContext
	for _, omission := range codeContext.Omissions {
		if omission.Code != "" {
			env.Warnings = appendStringIfMissing(env.Warnings, fmt.Sprintf("wiki-code context omission: %s", omission.Code))
		}
	}
	return env
}

func extractWikiPrimaryPath(operation string, items any) string {
	data, err := json.Marshal(items)
	if err != nil {
		return ""
	}
	var values any
	if json.Unmarshal(data, &values) != nil {
		return ""
	}
	var preferred []string
	switch operation {
	case "nav.ask":
		preferred = []string{"primary_doc"}
	case "nav.route", "nav.wiki.route":
		preferred = []string{"canonical", "anchor_doc"}
	case "nav.pack", "nav.wiki.pack":
		preferred = []string{"primary_doc"}
	}
	if path := findCanonicalPath(values, preferred); path != "" {
		return path
	}
	return findCanonicalPath(values, nil)
}

func findCanonicalPath(value any, preferred []string) string {
	if path, ok := value.(string); ok && model.CanonicalWikiAuthority(path) && !strings.HasPrefix(path, ".docs/raw/") && !strings.HasPrefix(path, ".docs/auditoria/") {
		return path
	}
	if list, ok := value.([]any); ok {
		for _, item := range list {
			if path := findCanonicalPath(item, preferred); path != "" {
				return path
			}
		}
		return ""
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	keys := append([]string(nil), preferred...)
	seen := map[string]bool{}
	for _, key := range keys {
		seen[key] = true
		if child, exists := obj[key]; exists {
			if path := findCanonicalPath(child, nil); path != "" {
				return path
			}
		}
	}
	keys = keys[:0]
	for key := range obj {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		child := obj[key]
		if key == "path" || key == "file" {
			if path, ok := child.(string); ok && model.CanonicalWikiAuthority(path) && !strings.HasPrefix(path, ".docs/raw/") && !strings.HasPrefix(path, ".docs/auditoria/") {
				return path
			}
		}
		if path := findCanonicalPath(child, nil); path != "" {
			return path
		}
	}
	return ""
}
