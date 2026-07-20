package indexer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// GraphObservationRequest identifies one exact repository and topology entrypoint.
type GraphObservationRequest struct {
	WorkspaceRoot      string
	WorkspaceIdentity  string
	RepositoryIdentity string
	RepoID             string
	RepoName           string
	RepoRoot           string
	EntrypointID       string
	EntrypointPath     string
	EntrypointKind     string
	Backend            string
}

// GraphObserver is the adapter boundary for external semantic compilers.
type GraphObserver func(context.Context, GraphObservationRequest) (model.GraphObservationBatch, error)

type GraphIndexOptions struct {
	RoslynObserver GraphObserver
}

var errGraphTopologyAmbiguous = errors.New("graph topology is ambiguous; no repository was selected")

// ObserveGraph runs local Go observation and the externally supplied Roslyn
// observer. Unsupported languages are recorded as gated omissions, never as
// semantic graph claims.
func ObserveGraph(ctx context.Context, root string, project model.ProjectFile, options GraphIndexOptions, progress ProgressFunc) ([]model.GraphObservationBatch, []model.GraphObservationOmission, []string, error) {
	if err := reportProgress(ctx, progress, Progress{Stage: "graph.observe", Force: true}); err != nil {
		return nil, nil, nil, err
	}
	_, configErr := os.Stat(workspace.ProjectConfigPath(root))
	repo, entrypoint, ok := graphTopologyTarget(project, configErr == nil)
	if !ok {
		return nil, []model.GraphObservationOmission{{Backend: "roslyn", Capability: "declarations", ReasonCode: "ambiguous_repository", RecoveryHintCode: "select_repo"}}, []string{"graph omitted: container topology does not explicitly select one authoritative repository"}, nil
	}
	if strings.TrimSpace(repo.RepositoryIdentity) == "" {
		return nil, []model.GraphObservationOmission{{Backend: "roslyn", Capability: "declarations", ReasonCode: "repository_identity_missing", RecoveryHintCode: "declare_repository_identity"}}, []string{"graph omitted: repository_identity is required and was not guessed"}, nil
	}
	identity, err := model.NormalizeRepositoryIdentity(strings.TrimSpace(repo.RepositoryIdentity))
	if err != nil {
		return nil, nil, nil, errors.New("graph repository identity is invalid")
	}
	repoRoot := filepath.Join(root, filepath.FromSlash(repo.Root))
	batches := make([]model.GraphObservationBatch, 0, 2)
	omissions := make([]model.GraphObservationOmission, 0)
	warnings := make([]string, 0)

	if hasLanguage(repo, "go") {
		module := graphGoModule(repoRoot, repo.DefaultEntrypoint)
		if module == "" {
			omissions = append(omissions, graphOmission("go", "declarations", "go_module_missing", "declare_go_module"))
			warnings = append(warnings, "graph omitted Go: no explicit go.mod or go.work entrypoint")
		} else {
			batch, err := ObserveGoGraph(ctx, GoGraphObservationRequest{Root: repoRoot, RepositoryIdentity: identity, ProjectOrModule: module})
			if err != nil {
				return nil, omissions, warnings, fmt.Errorf("go graph observation failed: %w", err)
			}
			if err := batch.ReadyForStaging(); err != nil {
				return nil, omissions, warnings, fmt.Errorf("go graph observation is not stageable: %w", err)
			}
			batches = append(batches, batch)
		}
	}

	if hasLanguage(repo, "csharp") {
		if options.RoslynObserver == nil {
			return nil, omissions, warnings, errors.New("roslyn graph observer is not configured")
		}
		if entrypoint.Path == "" || entrypoint.Kind != model.EntrypointKindProject || !strings.HasSuffix(strings.ToLower(filepath.ToSlash(entrypoint.Path)), ".csproj") {
			return nil, omissions, warnings, errors.New("roslyn graph entrypoint is missing or invalid")
		}
		request := GraphObservationRequest{WorkspaceRoot: root, WorkspaceIdentity: identity, RepositoryIdentity: identity, RepoID: repo.ID, RepoName: repo.Name, RepoRoot: repoRoot, EntrypointID: entrypoint.ID, EntrypointPath: relativeToRepo(repoRoot, root, entrypoint.Path), EntrypointKind: entrypoint.Kind, Backend: "roslyn"}
		batch, err := options.RoslynObserver(ctx, request)
		if err != nil {
			return nil, omissions, warnings, err
		}
		if batch.Backend != "roslyn" {
			return nil, omissions, warnings, errors.New("roslyn graph observer returned an unexpected backend")
		}
		batch.WorkspaceIdentity = identity
		batch.RepositoryIdentity = identity
		if err := batch.Validate(); err != nil {
			return nil, omissions, warnings, fmt.Errorf("roslyn graph observation invalid: %w", err)
		}
		if err := batch.ReadyForStaging(); err != nil {
			return nil, omissions, warnings, fmt.Errorf("roslyn graph observation is not stageable: %w", err)
		}
		batches = append(batches, batch)
	}

	for _, language := range []string{"typescript", "python"} {
		if hasLanguage(repo, language) {
			backend := map[string]string{"typescript": "tsserver", "python": "pyright"}[language]
			omissions = append(omissions, graphOmission(backend, "declarations", "backend_gated", "enable_semantic_backend"))
			warnings = append(warnings, "graph backend gated for "+language+"; no semantic claims were emitted")
		}
	}
	if len(batches) == 0 {
		warnings = append(warnings, "graph not published: no eligible graph backend observation")
	}
	if err := reportProgress(ctx, progress, Progress{Stage: "graph.stage", Force: true}); err != nil {
		return nil, omissions, warnings, err
	}
	return batches, omissions, warnings, nil
}

func graphTopologyTarget(project model.ProjectFile, explicitSelection bool) (model.WorkspaceRepo, model.WorkspaceEntrypoint, bool) {
	if len(project.Repos) == 0 {
		return model.WorkspaceRepo{}, model.WorkspaceEntrypoint{}, false
	}
	selected := strings.TrimSpace(project.Project.DefaultRepo)
	if project.Project.Kind == model.WorkspaceKindContainer && !explicitSelection {
		return model.WorkspaceRepo{}, model.WorkspaceEntrypoint{}, false
	}
	if selected == "" && len(project.Repos) == 1 {
		selected = project.Repos[0].ID
	}
	if selected == "" {
		return model.WorkspaceRepo{}, model.WorkspaceEntrypoint{}, false
	}
	var repo model.WorkspaceRepo
	found := false
	for _, candidate := range project.Repos {
		if candidate.ID == selected {
			repo, found = candidate, true
			break
		}
	}
	if !found {
		return model.WorkspaceRepo{}, model.WorkspaceEntrypoint{}, false
	}
	var entrypoint model.WorkspaceEntrypoint
	for _, candidate := range project.Entrypoints {
		if candidate.ID == project.Project.DefaultEntrypoint && candidate.RepoID == repo.ID {
			entrypoint = candidate
			break
		}
	}
	if entrypoint.ID == "" {
		for _, candidate := range project.Entrypoints {
			if candidate.RepoID == repo.ID && candidate.Default {
				entrypoint = candidate
				break
			}
		}
	}
	return repo, entrypoint, true
}

func graphGoModule(repoRoot, configured string) string {
	configured = filepath.ToSlash(strings.TrimSpace(configured))
	if configured == "go.mod" || configured == "go.work" {
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(configured))); err == nil {
			return configured
		}
	}
	for _, name := range []string{"go.mod", "go.work"} {
		if _, err := os.Stat(filepath.Join(repoRoot, name)); err == nil {
			return name
		}
	}
	return ""
}

func relativeToRepo(repoRoot, workspaceRoot, path string) string {
	path = filepath.FromSlash(strings.TrimSpace(path))
	candidate := filepath.Join(workspaceRoot, path)
	if rel, err := filepath.Rel(repoRoot, candidate); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

func hasLanguage(repo model.WorkspaceRepo, language string) bool {
	for _, candidate := range repo.Languages {
		if strings.EqualFold(candidate, language) {
			return true
		}
	}
	return false
}

func graphOmission(backend, capability, reason, recovery string) model.GraphObservationOmission {
	return model.GraphObservationOmission{Ref: "omission:" + backend + ":" + reason, Backend: backend, Capability: capability, ReasonCode: reason, RecoveryHintCode: recovery}
}
