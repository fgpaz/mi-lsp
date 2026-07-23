package workspace

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

var errRepositoryIdentityResolution = errors.New("repository identity could not be resolved")

// ResolveRepositoryIdentity returns the one canonical identity shared by every
// graph observation in a workspace. Explicit topology values take precedence;
// otherwise only local VCS metadata is consulted. No network or path-based
// fallback is permitted.
func ResolveRepositoryIdentity(ctx context.Context, workspaceRoot string, repos []model.WorkspaceRepo) (string, error) {
	if ctx == nil {
		return "", errRepositoryIdentityResolution
	}

	var explicit string
	for _, repo := range repos {
		raw := strings.TrimSpace(repo.RepositoryIdentity)
		if raw == "" {
			continue
		}
		identity, err := model.NormalizeRepositoryIdentity(raw)
		if err != nil {
			return "", errRepositoryIdentityResolution
		}
		if explicit == "" {
			explicit = identity
			continue
		}
		if explicit != identity {
			return "", errRepositoryIdentityResolution
		}
	}
	if explicit != "" {
		return explicit, nil
	}

	gitRoot, err := localGitCommand(ctx, "-C", workspaceRoot, "rev-parse", "--show-toplevel")
	if err != nil || strings.TrimSpace(gitRoot) == "" {
		return "", errRepositoryIdentityResolution
	}
	gitRoot = strings.TrimSpace(gitRoot)
	origins, err := localGitCommand(ctx, "-C", gitRoot, "config", "--local", "--get-all", "remote.origin.url")
	if err != nil {
		return "", errRepositoryIdentityResolution
	}
	values := make([]string, 0, 1)
	for _, value := range strings.Split(origins, "\n") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	if len(values) != 1 {
		return "", errRepositoryIdentityResolution
	}
	identity, err := model.NormalizeRepositoryIdentity(values[0])
	if err != nil {
		return "", errRepositoryIdentityResolution
	}
	return identity, nil
}

func localGitCommand(ctx context.Context, args ...string) (string, error) {
	output, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}
