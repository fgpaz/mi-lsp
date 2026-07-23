package workspace

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestResolveRepositoryIdentityNormalizesLocalHTTPSOrigin(t *testing.T) {
	root := initIdentityGitRepo(t, "https://GitHub.com/acme/mi-lsp.git")
	got, err := ResolveRepositoryIdentity(context.Background(), root, []model.WorkspaceRepo{{ID: "repo"}})
	if err != nil || got != "github.com/acme/mi-lsp" {
		t.Fatalf("identity=%q err=%v", got, err)
	}
}

func TestResolveRepositoryIdentityNormalizesLocalSSHOrigin(t *testing.T) {
	root := initIdentityGitRepo(t, "git@GitHub.com:acme/mi-lsp.git")
	got, err := ResolveRepositoryIdentity(context.Background(), root, []model.WorkspaceRepo{{ID: "repo"}})
	if err != nil || got != "github.com/acme/mi-lsp" {
		t.Fatalf("identity=%q err=%v", got, err)
	}
}

func TestResolveRepositoryIdentityFailsWithoutOrigin(t *testing.T) {
	root := initIdentityGitRepo(t, "")
	if _, err := ResolveRepositoryIdentity(context.Background(), root, []model.WorkspaceRepo{{ID: "repo"}}); err == nil {
		t.Fatal("expected missing origin to fail closed")
	}
}

func TestResolveRepositoryIdentityFailsWithMultipleOrigins(t *testing.T) {
	root := initIdentityGitRepo(t, "https://example.com/one.git")
	runIdentityGit(t, root, "config", "--local", "--add", "remote.origin.url", "git@example.com:two.git")
	if _, err := ResolveRepositoryIdentity(context.Background(), root, []model.WorkspaceRepo{{ID: "repo"}}); err == nil {
		t.Fatal("expected multiple origins to fail closed")
	}
}

func TestResolveRepositoryIdentityRejectsUnsupportedOriginForms(t *testing.T) {
	for _, origin := range []string{
		"/local/repo.git",
		"C:/local/repo.git",
		"https://user:secret@example.com/repo.git",
		"https://example.com/repo.git?token=secret",
		"https://example.com/repo.git#fragment",
		"file:///tmp/repo.git",
	} {
		t.Run(origin, func(t *testing.T) {
			root := initIdentityGitRepo(t, origin)
			if _, err := ResolveRepositoryIdentity(context.Background(), root, []model.WorkspaceRepo{{ID: "repo"}}); err == nil {
				t.Fatal("expected origin to fail closed")
			}
		})
	}
}

func TestResolveRepositoryIdentityExplicitAvoidsGit(t *testing.T) {
	got, err := ResolveRepositoryIdentity(context.Background(), filepath.Join(t.TempDir(), "not-a-repo"), []model.WorkspaceRepo{{ID: "one", RepositoryIdentity: "HTTPS://Example.com/acme/repo.git"}, {ID: "two"}})
	if err != nil || got != "example.com/acme/repo" {
		t.Fatalf("identity=%q err=%v", got, err)
	}
}

func TestResolveRepositoryIdentityRejectsConflictingExplicitIdentities(t *testing.T) {
	_, err := ResolveRepositoryIdentity(context.Background(), t.TempDir(), []model.WorkspaceRepo{{ID: "one", RepositoryIdentity: "https://example.com/one.git"}, {ID: "two", RepositoryIdentity: "https://example.com/two.git"}})
	if err == nil {
		t.Fatal("expected conflicting explicit identities to fail closed")
	}
}

func initIdentityGitRepo(t *testing.T, origin string) string {
	t.Helper()
	root := t.TempDir()
	runIdentityGit(t, root, "init")
	if strings.TrimSpace(origin) != "" {
		runIdentityGit(t, root, "config", "--local", "remote.origin.url", origin)
	}
	return root
}

func runIdentityGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, strings.TrimSpace(string(output)))
	}
	return string(output)
}
