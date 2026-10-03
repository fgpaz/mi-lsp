package indexer

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

// gitRepoWithoutOrigin builds a committed Go repository with no remote, so the
// repository identity cannot be resolved (RF-GPH-007).
func gitRepoWithoutOrigin(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWriteIncrementalFile(t, filepath.Join(root, "go.mod"), "module example.com/no-origin\n\ngo 1.23\n")
	mustWriteIncrementalFile(t, filepath.Join(root, "main.go"), "package main\nfunc target() {}\nfunc main() { target() }\n")
	mustWriteIncrementalFile(t, filepath.Join(root, ".gitignore"), ".mi-lsp/\n")
	for _, args := range [][]string{{"init"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "mi-lsp-test"}, {"add", "."}, {"commit", "-m", "fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	return root
}

func TestIndexWorkspaceWithoutRepositoryIdentityPublishesCatalogAndOmitsGraph(t *testing.T) {
	root := gitRepoWithoutOrigin(t)

	result, err := IndexWorkspaceWithGeneration(context.Background(), root, true, "gen-no-origin")
	if err != nil {
		t.Fatalf("IndexWorkspace without origin must succeed, got: %v", err)
	}
	if !containsString(result.Warnings, graphIdentityUnavailableWarning) {
		t.Fatalf("warnings = %v, want %q", result.Warnings, graphIdentityUnavailableWarning)
	}
	if result.GraphGenerationID != "" {
		t.Fatalf("graph generation %q published without identity", result.GraphGenerationID)
	}
	omitted := false
	for _, omission := range result.GraphOmissions {
		if omission.ReasonCode == "identity_unavailable" && omission.RecoveryHintCode == "declare_repository_identity" {
			omitted = true
		}
	}
	if !omitted {
		t.Fatalf("graph omissions = %+v, want identity_unavailable", result.GraphOmissions)
	}
	ready, err := store.WorkspaceCatalogReady(context.Background(), root)
	if err != nil || !ready {
		t.Fatalf("catalog ready=%v err=%v, want a published catalog", ready, err)
	}
	if len(result.Symbols) == 0 {
		t.Fatal("catalog must still index symbols")
	}
	if strings.Contains(strings.Join(result.Warnings, "\n"), root) {
		t.Fatal("warnings must not leak the local path")
	}
}

func TestExplicitEntrypointWithoutRepositoryIdentityStaysAnError(t *testing.T) {
	root := gitRepoWithoutOrigin(t)

	_, err := IndexWorkspaceWithGraphProgress(context.Background(), root, true, "gen-explicit", nil, GraphIndexOptions{EntrypointSelector: "go.mod"})
	var observationErr *model.GraphObservationError
	if !errors.As(err, &observationErr) {
		t.Fatalf("explicit entrypoint error = %v, want a typed GraphObservationError", err)
	}
	if _, statErr := os.Stat(store.WorkspaceDBPath(root)); statErr == nil {
		if ready, readyErr := store.WorkspaceCatalogReady(context.Background(), root); readyErr == nil && ready {
			t.Fatal("a failed explicit graph selection must not publish the catalog")
		}
	}
}
