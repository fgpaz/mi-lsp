package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestFindOverlayScopedRepoPagesPastOtherRepos(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	selected := model.WorkspaceRepo{ID: "selected", Name: "Selected", Root: "selected"}
	other := model.WorkspaceRepo{ID: "other", Name: "Other", Root: "other"}
	project := model.ProjectFile{Repos: []model.WorkspaceRepo{selected, other}}
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	var files []model.FileRecord
	var symbols []model.SymbolRecord
	add := func(repo model.WorkspaceRepo, path, source string) {
		t.Helper()
		items, file := indexer.ExtractCatalog(root, repo, filepath.Join(root, path), []byte(source))
		files = append(files, file)
		symbols = append(symbols, items...)
	}
	for i := 0; i < 220; i++ {
		add(other, fmt.Sprintf("other/f%03d.go", i), fmt.Sprintf("package p\nfunc Match%03d() {}\n", i))
	}
	add(selected, "selected/base.go", "package p\nfunc MatchZBase() {}\nfunc MatchZSecond() {}\n")
	add(selected, "selected/masked.go", "package p\nfunc MatchZMasked() {}\n")
	if err := store.ReplaceCatalog(ctx, db, project, files, symbols); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "selected"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "selected", "delta.go"), []byte("package p\nfunc MatchZDelta() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	overlay := workspace.WorktreeOverlay{Root: root, Changed: []string{"selected/delta.go"}, Deleted: []string{"selected/masked.go"}}
	registration := model.WorkspaceRegistration{Name: "fixture", Root: root}
	scope := model.WorkspaceRepo{ID: "SELECTED", Name: "unmatched"}
	find := func(pattern, kind string, exact bool, limit, offset int) []model.SymbolRecord {
		t.Helper()
		got, err := findSymbolsWithWorktreeOverlay(ctx, db, registration, project, overlay, pattern, kind, exact, limit, offset, &scope)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	for _, tc := range []struct {
		offset int
		want   string
	}{{0, "MatchZBase"}, {1, "MatchZDelta"}, {2, "MatchZSecond"}} {
		got := find("Match", "", false, 1, tc.offset)
		if len(got) != 1 || got[0].Name != tc.want {
			t.Errorf("offset %d: got %#v, want %s", tc.offset, got, tc.want)
		}
	}
	if got := find("MatchZMasked", "", true, 1, 0); len(got) != 0 {
		t.Errorf("masked: %#v", got)
	}
	if got := find("no-such-symbol", "", false, 1, 0); len(got) != 0 {
		t.Errorf("no match: %#v", got)
	}
	if got := find("MatchZBase", "", true, 1, 0); len(got) != 1 || got[0].Name != "MatchZBase" {
		t.Errorf("exact: %#v", got)
	}
	if got := find("Match", "not-a-kind", false, 1, 0); len(got) != 0 {
		t.Errorf("kind: %#v", got)
	}
	scope = model.WorkspaceRepo{ID: "unmatched", Name: "sELECTED"}
	if got := find("Match", "", false, 1, 0); len(got) != 1 || got[0].Name != "MatchZBase" {
		t.Errorf("name scope: %#v", got)
	}
}
