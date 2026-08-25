package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestWikiRootResolvesCanonRelativeToWorkspaceRoot(t *testing.T) {
	ensureWritableTestHome(t)
	parent := t.TempDir()
	code := filepath.Join(parent, "code")
	canon := filepath.Join(parent, "wiki-repo", "Ingenieria")
	if err := os.MkdirAll(code, 0o755); err != nil {
		t.Fatalf("mkdir code: %v", err)
	}
	if err := os.MkdirAll(canon, 0o755); err != nil {
		t.Fatalf("mkdir canon: %v", err)
	}
	if err := os.WriteFile(filepath.Join(canon, "00_gobierno_documental.md"), []byte("# Gobierno\n"), 0o644); err != nil {
		t.Fatalf("write gobierno: %v", err)
	}
	if err := workspace.SaveProjectFile(code, model.ProjectFile{
		Project: model.ProjectBlock{Name: "code", Kind: model.WorkspaceKindSingle},
		Canons: []model.WorkspaceCanon{{
			ID:   "wiki",
			Root: "../wiki-repo/Ingenieria",
			Role: "producto",
			Mode: "read-only",
		}},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	alias := "wiki-root-canon-" + filepath.Base(parent)
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name: alias,
		Root: code,
		Kind: model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })

	env, err := New(code, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki-root",
		Context:   model.QueryOptions{Workspace: alias},
	})
	if err != nil {
		t.Fatalf("nav.wiki-root: %v", err)
	}
	if !env.Ok || env.Backend != "wiki-root" || env.Workspace != alias {
		t.Fatalf("envelope = %#v", env)
	}
	items, ok := env.Items.([]model.WikiRootResolution)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %#v", env.Items)
	}
	item := items[0]
	if item.WikiRoot != "../wiki-repo/Ingenieria" {
		t.Fatalf("wiki_root = %q, want ../wiki-repo/Ingenieria", item.WikiRoot)
	}
	if item.ResolvedFrom != "canon.wiki" {
		t.Fatalf("resolved_from = %q, want canon.wiki", item.ResolvedFrom)
	}
	if item.ID != "wiki" || item.Role != "producto" || item.Workspace != alias {
		t.Fatalf("id/role/workspace = %#v", item)
	}
	if item.GovernanceDoc != "../wiki-repo/Ingenieria/00_gobierno_documental.md" {
		t.Fatalf("governance_doc = %q", item.GovernanceDoc)
	}
	assertPortableWikiRootItems(t, parent, items)
}

func TestWikiRootNoCanonDefaults(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	alias := "wiki-root-default-" + filepath.Base(root)
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name: alias,
		Root: root,
		Kind: model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })

	env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki-root",
		Context:   model.QueryOptions{Workspace: alias},
	})
	if err != nil {
		t.Fatalf("nav.wiki-root: %v", err)
	}
	if !env.Ok || env.Backend != "wiki-root" {
		t.Fatalf("envelope = %#v", env)
	}
	items, ok := env.Items.([]model.WikiRootResolution)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %#v", env.Items)
	}
	item := items[0]
	if item.ResolvedFrom != "default" || item.WikiRoot != ".docs/wiki" {
		t.Fatalf("item = %#v, want resolved_from=default wiki_root=.docs/wiki", item)
	}
	if item.GovernanceDoc != ".docs/wiki/00_gobierno_documental.md" {
		t.Fatalf("governance_doc = %q", item.GovernanceDoc)
	}
	if item.Role != "" || item.ID != "" {
		t.Fatalf("default role/id should be empty, got %#v", item)
	}
	if item.Workspace != alias {
		t.Fatalf("workspace = %q, want %q", item.Workspace, alias)
	}
	assertPortableWikiRootItems(t, root, items)
}

func TestWikiRootRoleMissingMatchErrors(t *testing.T) {
	ensureWritableTestHome(t)
	parent := t.TempDir()
	code := filepath.Join(parent, "code")
	canon := filepath.Join(parent, "wiki-repo", "Ingenieria")
	if err := os.MkdirAll(code, 0o755); err != nil {
		t.Fatalf("mkdir code: %v", err)
	}
	if err := os.MkdirAll(canon, 0o755); err != nil {
		t.Fatalf("mkdir canon: %v", err)
	}
	if err := os.WriteFile(filepath.Join(canon, "00_gobierno_documental.md"), []byte("# Gobierno\n"), 0o644); err != nil {
		t.Fatalf("write gobierno: %v", err)
	}
	if err := workspace.SaveProjectFile(code, model.ProjectFile{
		Project: model.ProjectBlock{Name: "code", Kind: model.WorkspaceKindSingle},
		Canons: []model.WorkspaceCanon{{
			ID:   "wiki",
			Root: "../wiki-repo/Ingenieria",
			Role: "producto",
			Mode: "read-only",
		}},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	alias := "wiki-root-role-" + filepath.Base(parent)
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name: alias,
		Root: code,
		Kind: model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })

	_, err := New(code, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki-root",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"role": "ecosistema"},
	})
	if err == nil {
		t.Fatal("expected role mismatch to fail closed")
	}
	message := err.Error()
	if !strings.Contains(message, "ecosistema") {
		t.Fatalf("error %q should mention the requested role", message)
	}
	if !strings.Contains(message, "wiki") || !strings.Contains(message, "producto") {
		t.Fatalf("error %q should list available ids/roles", message)
	}
}

func TestWikiRootMultipleCanonsSortedByID(t *testing.T) {
	ensureWritableTestHome(t)
	parent := t.TempDir()
	code := filepath.Join(parent, "code")
	producto := filepath.Join(parent, "wiki-repo", "Ingenieria")
	gobierno := filepath.Join(parent, "gobierno")
	if err := os.MkdirAll(code, 0o755); err != nil {
		t.Fatalf("mkdir code: %v", err)
	}
	if err := os.MkdirAll(producto, 0o755); err != nil {
		t.Fatalf("mkdir producto: %v", err)
	}
	if err := os.MkdirAll(gobierno, 0o755); err != nil {
		t.Fatalf("mkdir gobierno: %v", err)
	}
	if err := workspace.SaveProjectFile(code, model.ProjectFile{
		Project: model.ProjectBlock{Name: "code", Kind: model.WorkspaceKindSingle},
		Canons: []model.WorkspaceCanon{
			{ID: "zeta", Root: "../wiki-repo/Ingenieria", Role: "producto", Mode: "read-only"},
			{ID: "alpha", Root: "../gobierno", Role: "gobierno_local"},
		},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	alias := "wiki-root-multi-" + filepath.Base(parent)
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name: alias,
		Root: code,
		Kind: model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })

	env, err := New(code, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki-root",
		Context:   model.QueryOptions{Workspace: alias},
	})
	if err != nil {
		t.Fatalf("nav.wiki-root: %v", err)
	}
	items, ok := env.Items.([]model.WikiRootResolution)
	if !ok || len(items) != 2 {
		t.Fatalf("items = %#v", env.Items)
	}
	if items[0].ID != "alpha" || items[1].ID != "zeta" {
		t.Fatalf("sort order = %q,%q want alpha,zeta", items[0].ID, items[1].ID)
	}
	if items[0].ResolvedFrom != "canon.alpha" || items[1].ResolvedFrom != "canon.zeta" {
		t.Fatalf("resolved_from = %q,%q", items[0].ResolvedFrom, items[1].ResolvedFrom)
	}
	assertPortableWikiRootItems(t, parent, items)
}

func TestWikiRootInvalidCanonFailsClosed(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{Name: "code", Kind: model.WorkspaceKindSingle},
		Canons: []model.WorkspaceCanon{{
			ID:   "wiki",
			Root: "/abs/wiki",
			Role: "producto",
		}},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	alias := "wiki-root-invalid-" + filepath.Base(root)
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name: alias,
		Root: root,
		Kind: model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })

	_, err := New(root, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki-root",
		Context:   model.QueryOptions{Workspace: alias},
	})
	if err == nil {
		t.Fatal("invalid [[canon]] must fail closed")
	}
	if !strings.Contains(err.Error(), "/abs/wiki") {
		t.Fatalf("error %q should include the declared path", err)
	}
}

func TestWikiRootResolvesRegistryCanonLink(t *testing.T) {
	ensureWritableTestHome(t)
	code := t.TempDir()
	wiki := t.TempDir()
	writeWorkspaceFile(t, wiki, ".docs/wiki/00_gobierno_documental.md", "# Gobierno\n")
	codeAlias := "wiki-root-link-code-" + filepath.Base(code)
	wikiAlias := "wiki-root-link-wiki-" + filepath.Base(wiki)
	if _, err := workspace.RegisterWorkspace(codeAlias, model.WorkspaceRegistration{
		Name:       codeAlias,
		Root:       code,
		Kind:       model.WorkspaceKindSingle,
		CanonLinks: []model.WorkspaceCanonLink{{Alias: wikiAlias, Role: "producto"}},
	}); err != nil {
		t.Fatalf("RegisterWorkspace(code): %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(codeAlias) })
	if _, err := workspace.RegisterWorkspace(wikiAlias, model.WorkspaceRegistration{
		Name: wikiAlias,
		Root: wiki,
		Kind: model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace(wiki): %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(wikiAlias) })

	env, err := New(code, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki-root",
		Context:   model.QueryOptions{Workspace: codeAlias},
	})
	if err != nil {
		t.Fatalf("nav.wiki-root: %v", err)
	}
	items, ok := env.Items.([]model.WikiRootResolution)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %#v", env.Items)
	}
	item := items[0]
	if item.ResolvedFrom != "registry.link" {
		t.Fatalf("resolved_from = %q, want registry.link", item.ResolvedFrom)
	}
	if item.WikiRoot != ".docs/wiki" || item.GovernanceDoc != ".docs/wiki/00_gobierno_documental.md" {
		t.Fatalf("paths = %#v", item)
	}
	if item.Workspace != codeAlias || item.ID != wikiAlias || item.Role != "producto" {
		t.Fatalf("id/role/workspace = %#v", item)
	}
	assertPortableWikiRootItems(t, code, items)
}

func TestWikiRootRegistryLinkUsesTargetCanon(t *testing.T) {
	ensureWritableTestHome(t)
	parent := t.TempDir()
	code := filepath.Join(parent, "code")
	wikiWS := filepath.Join(parent, "wiki-ws")
	canon := filepath.Join(wikiWS, "Ingenieria")
	if err := os.MkdirAll(code, 0o755); err != nil {
		t.Fatalf("mkdir code: %v", err)
	}
	if err := os.MkdirAll(canon, 0o755); err != nil {
		t.Fatalf("mkdir canon: %v", err)
	}
	if err := workspace.SaveProjectFile(wikiWS, model.ProjectFile{
		Project: model.ProjectBlock{Name: "wiki-ws", Kind: model.WorkspaceKindSingle},
		Canons:  []model.WorkspaceCanon{{ID: "ingenieria", Root: "Ingenieria", Role: "producto"}},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	codeAlias := "wiki-root-link-canon-code-" + filepath.Base(parent)
	wikiAlias := "wiki-root-link-canon-wiki-" + filepath.Base(parent)
	if _, err := workspace.RegisterWorkspace(codeAlias, model.WorkspaceRegistration{
		Name:       codeAlias,
		Root:       code,
		Kind:       model.WorkspaceKindSingle,
		CanonLinks: []model.WorkspaceCanonLink{{Alias: wikiAlias, Role: "producto"}},
	}); err != nil {
		t.Fatalf("RegisterWorkspace(code): %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(codeAlias) })
	if _, err := workspace.RegisterWorkspace(wikiAlias, model.WorkspaceRegistration{
		Name: wikiAlias,
		Root: wikiWS,
		Kind: model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace(wiki): %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(wikiAlias) })

	env, err := New(code, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki-root",
		Context:   model.QueryOptions{Workspace: codeAlias},
	})
	if err != nil {
		t.Fatalf("nav.wiki-root: %v", err)
	}
	items, ok := env.Items.([]model.WikiRootResolution)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %#v", env.Items)
	}
	if items[0].WikiRoot != "Ingenieria" || items[0].ResolvedFrom != "registry.link" || items[0].ID != wikiAlias {
		t.Fatalf("item = %#v", items[0])
	}
	if items[0].GovernanceDoc != "Ingenieria/00_gobierno_documental.md" {
		t.Fatalf("governance_doc = %q", items[0].GovernanceDoc)
	}
	assertPortableWikiRootItems(t, parent, items)
}

func TestWikiRootRegistryLinkRoleMismatchErrors(t *testing.T) {
	ensureWritableTestHome(t)
	code := t.TempDir()
	wiki := t.TempDir()
	codeAlias := "wiki-root-link-role-code-" + filepath.Base(code)
	wikiAlias := "wiki-root-link-role-wiki-" + filepath.Base(wiki)
	if _, err := workspace.RegisterWorkspace(codeAlias, model.WorkspaceRegistration{
		Name:       codeAlias,
		Root:       code,
		Kind:       model.WorkspaceKindSingle,
		CanonLinks: []model.WorkspaceCanonLink{{Alias: wikiAlias, Role: "producto"}},
	}); err != nil {
		t.Fatalf("RegisterWorkspace(code): %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(codeAlias) })
	if _, err := workspace.RegisterWorkspace(wikiAlias, model.WorkspaceRegistration{
		Name: wikiAlias,
		Root: wiki,
		Kind: model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace(wiki): %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(wikiAlias) })

	_, err := New(code, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.wiki-root",
		Context:   model.QueryOptions{Workspace: codeAlias},
		Payload:   map[string]any{"role": "ecosistema"},
	})
	if err == nil {
		t.Fatal("expected role mismatch to fail closed")
	}
	message := err.Error()
	if !strings.Contains(message, "ecosistema") || !strings.Contains(message, wikiAlias) || !strings.Contains(message, "producto") {
		t.Fatalf("error %q should list requested role and available ids/roles", message)
	}
}

func TestGovernanceInspectsProductoCanonLink(t *testing.T) {
	ensureWritableTestHome(t)
	code := t.TempDir()
	wiki := t.TempDir()
	writeSpecBackendGovernanceFixture(t, wiki)
	writeWorkspaceFile(t, code, "go.mod", "module example.com/code\n")
	codeAlias := "gov-link-code-" + filepath.Base(code)
	wikiAlias := "gov-link-wiki-" + filepath.Base(wiki)
	if _, err := workspace.RegisterWorkspace(codeAlias, model.WorkspaceRegistration{
		Name:       codeAlias,
		Root:       code,
		Kind:       model.WorkspaceKindSingle,
		CanonLinks: []model.WorkspaceCanonLink{{Alias: wikiAlias, Role: "producto"}},
	}); err != nil {
		t.Fatalf("RegisterWorkspace(code): %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(codeAlias) })
	if _, err := workspace.RegisterWorkspace(wikiAlias, model.WorkspaceRegistration{
		Name: wikiAlias,
		Root: wiki,
		Kind: model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace(wiki): %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(wikiAlias) })

	wikiProjection := filepath.Join(wiki, ".docs", "wiki", "_mi-lsp", "read-model.toml")
	before, err := os.Stat(wikiProjection)
	if err != nil {
		t.Fatalf("stat wiki projection: %v", err)
	}

	env, err := New(code, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.governance",
		Context:   model.QueryOptions{Workspace: codeAlias},
	})
	if err != nil {
		t.Fatalf("nav.governance: %v", err)
	}
	if !env.Ok {
		t.Fatalf("envelope = %#v", env)
	}
	items, ok := env.Items.([]model.GovernanceStatus)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %#v", env.Items)
	}
	if items[0].Blocked && strings.Contains(strings.Join(items[0].Issues, " "), "missing") {
		t.Fatalf("code workspace should inspect product canon governance, got %#v", items[0])
	}
	after, err := os.Stat(wikiProjection)
	if err != nil {
		t.Fatalf("stat wiki projection after: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("governance auto_sync wrote into the foreign canon workspace")
	}
}

func assertPortableWikiRootItems(t *testing.T, forbiddenRoot string, items []model.WikiRootResolution) {
	t.Helper()
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("marshal items: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"wiki_root"`) || !strings.Contains(text, `"resolved_from"`) || !strings.Contains(text, `"governance_doc"`) {
		t.Fatalf("json tags missing: %s", text)
	}
	absRoot, err := filepath.Abs(forbiddenRoot)
	if err != nil {
		t.Fatalf("abs forbidden root: %v", err)
	}
	if strings.Contains(text, filepath.ToSlash(absRoot)) || strings.Contains(text, absRoot) {
		t.Fatalf("absolute workspace path leaked into items JSON: %s", text)
	}
	for _, item := range items {
		for _, path := range []string{item.WikiRoot, item.GovernanceDoc} {
			if path == "" {
				t.Fatalf("empty portable path in %#v", item)
			}
			if filepath.IsAbs(path) || strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\\`) {
				t.Fatalf("absolute path in item: %#v", item)
			}
			if len(path) >= 2 && path[1] == ':' {
				t.Fatalf("volume path in item: %#v", item)
			}
		}
	}
}
