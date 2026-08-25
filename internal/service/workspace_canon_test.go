package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestWorkspaceInitPreservesCanonsAndCanonPolicy(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "go.mod", "module example.com/canon\n")
	escape := 2
	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{Name: "keep-canon", Kind: model.WorkspaceKindSingle},
		Canons: []model.WorkspaceCanon{{
			ID:   "wiki",
			Root: "../wiki-repo/Ingenieria",
			Role: "producto",
		}},
		CanonPolicy: &model.CanonPolicyBlock{EscapeMax: &escape},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	alias := "canon-init-" + filepath.Base(root)
	app := New(root, nil)
	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Payload:   map[string]any{"path": root, "alias": alias, "no_index": true},
	}); err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })

	got, err := workspace.LoadProjectFile(root)
	if err != nil {
		t.Fatalf("LoadProjectFile: %v", err)
	}
	if len(got.Canons) != 1 || got.Canons[0].ID != "wiki" || got.Canons[0].Root != "../wiki-repo/Ingenieria" || got.Canons[0].Role != "producto" {
		t.Fatalf("canons not preserved: %#v", got.Canons)
	}
	if filepath.IsAbs(got.Canons[0].Root) {
		t.Fatalf("preserved root leaked an absolute path: %q", got.Canons[0].Root)
	}
	if got.CanonPolicy == nil || got.CanonPolicy.EscapeMax == nil || *got.CanonPolicy.EscapeMax != 2 {
		t.Fatalf("canon policy not preserved: %#v", got.CanonPolicy)
	}
}

func TestWorkspaceLinkStoresCanonLinkAndIsIdempotent(t *testing.T) {
	ensureWritableTestHome(t)
	code := t.TempDir()
	wiki := t.TempDir()
	writeWorkspaceFile(t, code, "go.mod", "module example.com/code\n")
	writeWorkspaceFile(t, wiki, "go.mod", "module example.com/wiki\n")
	codeAlias := "link-code-" + filepath.Base(code)
	wikiAlias := "link-wiki-" + filepath.Base(wiki)
	registerServiceWorkspace(t, codeAlias, code)
	registerServiceWorkspace(t, wikiAlias, wiki)

	app := New(code, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.link",
		Context:   model.QueryOptions{Workspace: codeAlias},
		Payload:   map[string]any{"alias": wikiAlias, "role": "producto"},
	})
	if err != nil {
		t.Fatalf("workspace.link: %v", err)
	}
	if !env.Ok || env.Workspace != codeAlias {
		t.Fatalf("envelope = %#v", env)
	}

	registry, err := workspace.LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	links := registry.Workspaces[codeAlias].CanonLinks
	if len(links) != 1 || links[0].Alias != wikiAlias || links[0].Role != "producto" {
		t.Fatalf("CanonLinks = %#v", links)
	}
	if filepath.IsAbs(links[0].Alias) {
		t.Fatalf("link stored an absolute alias: %q", links[0].Alias)
	}

	again, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.link",
		Context:   model.QueryOptions{Workspace: codeAlias},
		Payload:   map[string]any{"alias": wikiAlias, "role": "producto"},
	})
	if err != nil {
		t.Fatalf("idempotent workspace.link: %v", err)
	}
	if !again.Ok {
		t.Fatalf("idempotent envelope = %#v", again)
	}
	registry, err = workspace.LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if got := len(registry.Workspaces[codeAlias].CanonLinks); got != 1 {
		t.Fatalf("idempotent CanonLinks count = %d", got)
	}
}

func TestWorkspaceLinkReplacesSameRoleDifferentAlias(t *testing.T) {
	ensureWritableTestHome(t)
	code := t.TempDir()
	wikiA := t.TempDir()
	wikiB := t.TempDir()
	writeWorkspaceFile(t, code, "go.mod", "module example.com/code\n")
	codeAlias := "link-replace-" + filepath.Base(code)
	aliasA := "link-wiki-a-" + filepath.Base(wikiA)
	aliasB := "link-wiki-b-" + filepath.Base(wikiB)
	registerServiceWorkspace(t, codeAlias, code)
	registerServiceWorkspace(t, aliasA, wikiA)
	registerServiceWorkspace(t, aliasB, wikiB)

	app := New(code, nil)
	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.link",
		Context:   model.QueryOptions{Workspace: codeAlias},
		Payload:   map[string]any{"alias": aliasA, "role": "producto"},
	}); err != nil {
		t.Fatalf("first link: %v", err)
	}
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.link",
		Context:   model.QueryOptions{Workspace: codeAlias},
		Payload:   map[string]any{"alias": aliasB, "role": "producto"},
	})
	if err != nil {
		t.Fatalf("replace link: %v", err)
	}
	if !strings.Contains(strings.Join(env.Warnings, " "), aliasA) {
		t.Fatalf("warnings = %v, want replaced alias %s", env.Warnings, aliasA)
	}
	registry, err := workspace.LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	links := registry.Workspaces[codeAlias].CanonLinks
	if len(links) != 1 || links[0].Alias != aliasB || links[0].Role != "producto" {
		t.Fatalf("CanonLinks after replace = %#v", links)
	}
}

func TestWorkspaceLinkRequiresRegisteredTargetAndValidRole(t *testing.T) {
	ensureWritableTestHome(t)
	code := t.TempDir()
	writeWorkspaceFile(t, code, "go.mod", "module example.com/code\n")
	codeAlias := "link-missing-" + filepath.Base(code)
	registerServiceWorkspace(t, codeAlias, code)
	app := New(code, nil)

	_, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.link",
		Context:   model.QueryOptions{Workspace: codeAlias},
		Payload:   map[string]any{"alias": "does-not-exist", "role": "producto"},
	})
	if err == nil {
		t.Fatal("missing target should fail closed")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Fatalf("error %q should mention the missing alias", err)
	}

	_, err = app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.link",
		Context:   model.QueryOptions{Workspace: codeAlias},
		Payload:   map[string]any{"alias": codeAlias, "role": "docs"},
	})
	if err == nil {
		t.Fatal("invalid role should fail closed")
	}
	if !strings.Contains(err.Error(), "docs") {
		t.Fatalf("error %q should mention the invalid role", err)
	}
}

func TestWorkspaceInitPreservesCanonLinks(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "go.mod", "module example.com/canon-link\n")
	alias := "canon-link-init-" + filepath.Base(root)
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:       alias,
		Root:       root,
		Kind:       model.WorkspaceKindSingle,
		CanonLinks: []model.WorkspaceCanonLink{{Alias: "wiki-canon", Role: "producto"}},
	}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })

	if _, err := New(root, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Payload:   map[string]any{"path": root, "alias": alias, "no_index": true},
	}); err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	got, err := workspace.LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	links := got.Workspaces[alias].CanonLinks
	if len(links) != 1 || links[0].Alias != "wiki-canon" || links[0].Role != "producto" {
		t.Fatalf("CanonLinks wiped by init: %#v", links)
	}
}


