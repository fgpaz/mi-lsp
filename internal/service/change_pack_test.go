package service

import "testing"

func TestChangePackInputPathsExplicitPrecedeDiffSeeds(t *testing.T) {
	diffPaths := []string{".mi-lsp/index.db", "internal/service/affected.go"}
	explicitPaths := []string{"internal/service/change_pack.go", "internal/service/affected.go"}

	got := changePackInputPaths(diffPaths, explicitPaths)
	want := []string{"internal/service/affected.go", "internal/service/change_pack.go"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	for _, path := range got {
		if path == ".mi-lsp/index.db" {
			t.Fatalf("explicit paths must exclude diff/runtime seed %q", path)
		}
	}

	withoutExplicit := changePackInputPaths(diffPaths, nil)
	if len(withoutExplicit) != len(diffPaths) || withoutExplicit[0] != ".mi-lsp/index.db" {
		t.Fatalf("without explicit paths, diff discovery must remain unchanged: %v", withoutExplicit)
	}
}
