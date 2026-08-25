package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestValidateCanonRole(t *testing.T) {
	t.Parallel()
	if err := ValidateCanonRole("producto"); err != nil {
		t.Fatalf("producto should be valid: %v", err)
	}
	if err := ValidateCanonRole("docs"); err == nil {
		t.Fatal("docs should be invalid")
	}
	if err := ValidateCanonRole(""); err == nil {
		t.Fatal("empty role should be invalid")
	}
}

func TestUpsertCanonLinkIdempotentAndReplace(t *testing.T) {
	t.Parallel()
	links, replaced, idempotent := UpsertCanonLink(nil, "wiki", "producto")
	if replaced != "" || !(!idempotent) || len(links) != 1 {
		t.Fatalf("insert = %#v replaced=%q idempotent=%v", links, replaced, idempotent)
	}
	same, replaced, idempotent := UpsertCanonLink(links, "wiki", "producto")
	if !idempotent || replaced != "" || len(same) != 1 {
		t.Fatalf("duplicate should be idempotent, got %#v replaced=%q idempotent=%v", same, replaced, idempotent)
	}
	replacedLinks, replaced, idempotent := UpsertCanonLink(links, "other-wiki", "producto")
	if idempotent || replaced != "wiki" || len(replacedLinks) != 1 || replacedLinks[0].Alias != "other-wiki" {
		t.Fatalf("same-role replace = %#v replaced=%q idempotent=%v", replacedLinks, replaced, idempotent)
	}
	two, _, _ := UpsertCanonLink(links, "gov", "gobierno_local")
	if len(two) != 2 {
		t.Fatalf("different role should append, got %#v", two)
	}
}

func TestRegistrationHasCanonLink(t *testing.T) {
	t.Parallel()
	reg := model.WorkspaceRegistration{
		CanonLinks: []model.WorkspaceCanonLink{{Alias: "wiki", Role: "producto"}},
	}
	if !RegistrationHasCanonLink(reg, "wiki", "") {
		t.Fatal("any-role match should succeed")
	}
	if !RegistrationHasCanonLink(reg, "WIKI", "producto") {
		t.Fatal("case-insensitive alias should match")
	}
	if RegistrationHasCanonLink(reg, "wiki", "ecosistema") {
		t.Fatal("mismatched role should not match")
	}
	if RegistrationHasCanonLink(reg, "other", "") {
		t.Fatal("unknown alias should not match")
	}
}

func TestCanonLinksRoundTripSaveLoadRegistry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := t.TempDir()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := RegisterWorkspace("code", model.WorkspaceRegistration{
		Name: "code",
		Root: root,
		Kind: model.WorkspaceKindSingle,
		CanonLinks: []model.WorkspaceCanonLink{{
			Alias: "wiki-canon",
			Role:  "producto",
		}},
	}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	got, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	ws := got.Workspaces["code"]
	if len(ws.CanonLinks) != 1 || ws.CanonLinks[0].Alias != "wiki-canon" || ws.CanonLinks[0].Role != "producto" {
		t.Fatalf("CanonLinks = %#v", ws.CanonLinks)
	}
	if filepath.IsAbs(ws.CanonLinks[0].Alias) {
		t.Fatalf("link alias leaked an absolute path: %q", ws.CanonLinks[0].Alias)
	}
}

func TestRegisterWorkspacePreservesCanonLinks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := t.TempDir()
	if _, err := RegisterWorkspace("code", model.WorkspaceRegistration{
		Name:       "code",
		Root:       root,
		Kind:       model.WorkspaceKindSingle,
		CanonLinks: []model.WorkspaceCanonLink{{Alias: "wiki-canon", Role: "producto"}},
	}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	if _, err := RegisterWorkspace("code", model.WorkspaceRegistration{
		Name: "code",
		Root: root,
		Kind: model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("re-register: %v", err)
	}
	got, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	links := got.Workspaces["code"].CanonLinks
	if len(links) != 1 || links[0].Alias != "wiki-canon" || links[0].Role != "producto" {
		t.Fatalf("CanonLinks not preserved: %#v", links)
	}
}
