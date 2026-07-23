package indexer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	ProjectOrModule    string
	EntrypointKind     string
	Backend            string
}

// GraphObserver is the adapter boundary for external semantic compilers.
type GraphObserver func(context.Context, GraphObservationRequest) (model.GraphObservationBatch, error)

type GraphIndexOptions struct {
	RoslynObserver GraphObserver
}

type graphObservationTarget struct {
	Repo                    model.WorkspaceRepo
	Entrypoint              model.WorkspaceEntrypoint
	RepoRoot                string
	ProjectOrModule         string
	WorkspaceEntrypointPath string
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
	targets, ok := graphObservationTargets(root, project, configErr == nil)
	if project.Project.Kind == model.WorkspaceKindContainer && !ok {
		return nil, []model.GraphObservationOmission{{Backend: "roslyn", Capability: "declarations", ReasonCode: "ambiguous_repository", RecoveryHintCode: "select_repo"}}, []string{"graph omitted: container topology does not explicitly select one authoritative repository"}, nil
	}
	if !ok {
		return nil, nil, nil, errGraphTopologyAmbiguous
	}
	goRepos := graphGoRepos(root, project, targets)
	csharpTargets := make([]graphObservationTarget, 0)
	csharpDeclared := false
	csharpRepos := project.Repos
	if project.Project.Kind != model.WorkspaceKindContainer {
		selected := strings.TrimSpace(project.Project.DefaultRepo)
		if selected == "" && len(project.Repos) == 1 {
			selected = project.Repos[0].ID
		}
		csharpRepos = nil
		for _, repo := range project.Repos {
			if repo.ID == selected {
				csharpRepos = append(csharpRepos, repo)
				break
			}
		}
	}
	for _, repo := range csharpRepos {
		if hasLanguage(repo, "csharp") || hasLanguage(repo, "cs") || hasLanguage(repo, "dotnet") {
			csharpDeclared = true
		}
	}
	for _, target := range targets {
		if hasLanguage(target.Repo, "csharp") || hasLanguage(target.Repo, "cs") || hasLanguage(target.Repo, "dotnet") {
			csharpTargets = append(csharpTargets, target)
		}
	}
	if csharpDeclared && len(csharpTargets) == 0 {
		return nil, nil, nil, errors.New("no eligible C# project entrypoint was declared")
	}

	identity := ""
	if len(goRepos) > 0 || len(csharpTargets) > 0 {
		var err error
		identity, err = workspace.ResolveRepositoryIdentity(ctx, root, project.Repos)
		if err != nil {
			return nil, nil, nil, &model.GraphObservationError{Code: "GPH_IDENTITY_UNAVAILABLE", Field: "repository_identity", Message: "repository identity could not be resolved"}
		}
	}

	batches := make([]model.GraphObservationBatch, 0, len(targets)+1)
	omissions := make([]model.GraphObservationOmission, 0)
	warnings := make([]string, 0)

	if len(goRepos) > 0 {
		module := ""
		if project.Project.Kind == model.WorkspaceKindContainer {
			module = graphRootGoModule(root)
		} else {
			selected := goRepos[0]
			module = graphGoModule(selected.root, selected.repo.DefaultEntrypoint)
		}
		if module == "" {
			omissions = append(omissions, graphOmissionForRepo("go", "declarations", "go_module_missing", "declare_go_module", goRepos[0].repo))
			warnings = append(warnings, "graph omitted Go: no explicit go.mod or go.work entrypoint")
		} else {
			requestRoot := root
			if project.Project.Kind != model.WorkspaceKindContainer {
				requestRoot = goRepos[0].root
				module = graphGoModule(requestRoot, goRepos[0].repo.DefaultEntrypoint)
			}
			batch, observeErr := ObserveGoGraph(ctx, GoGraphObservationRequest{Root: requestRoot, RepositoryIdentity: identity, ProjectOrModule: module})
			if observeErr != nil {
				return nil, omissions, warnings, fmt.Errorf("go graph observation failed: %w", observeErr)
			}
			if err := batch.ReadyForStaging(); err != nil {
				return nil, omissions, warnings, fmt.Errorf("go graph observation is not stageable: %w", err)
			}
			batches = append(batches, batch)
		}
	}

	if len(csharpTargets) > 0 {
		if options.RoslynObserver == nil {
			return nil, omissions, warnings, errors.New("roslyn graph observer is not configured")
		}
		for _, target := range csharpTargets {
			request := GraphObservationRequest{
				WorkspaceRoot:      root,
				WorkspaceIdentity:  identity,
				RepositoryIdentity: identity,
				RepoID:             target.Repo.ID,
				RepoName:           target.Repo.Name,
				RepoRoot:           target.RepoRoot,
				EntrypointID:       target.Entrypoint.ID,
				EntrypointPath:     target.WorkspaceEntrypointPath,
				ProjectOrModule:    target.ProjectOrModule,
				EntrypointKind:     target.Entrypoint.Kind,
				Backend:            "roslyn",
			}
			batch, observeErr := options.RoslynObserver(ctx, request)
			if observeErr != nil {
				return nil, omissions, warnings, observeErr
			}
			if batch.Backend != "roslyn" {
				return nil, omissions, warnings, errors.New("roslyn graph observer returned an unexpected backend")
			}
			batch.WorkspaceIdentity = identity
			batch.RepositoryIdentity = identity
			for i := range batch.Nodes {
				batch.Nodes[i].Key.RepositoryIdentity = identity
				batch.Nodes[i].Key.BackendType = "roslyn"
			}
			if err := rebaseGraphObservationBatch(&batch, target.Repo.Root); err != nil {
				return nil, omissions, warnings, fmt.Errorf("roslyn graph observation rebase failed: %w", err)
			}
			if err := model.SealGraphObservationBatch(&batch); err != nil {
				return nil, omissions, warnings, fmt.Errorf("roslyn graph observation invalid: %w", err)
			}
			if err := batch.ReadyForStaging(); err != nil {
				if batch.Completeness == model.GraphCompletenessPartial {
					omissions = append(omissions, graphPartialOmission(target, batch))
					warnings = append(warnings, "graph omitted partial Roslyn observation for "+target.WorkspaceEntrypointPath)
					continue
				}
				return nil, omissions, warnings, fmt.Errorf("roslyn graph observation is not stageable: %w", err)
			}
			batches = append(batches, batch)
		}
	}

	for _, repo := range project.Repos {
		for _, language := range []string{"typescript", "python"} {
			if hasLanguage(repo, language) {
				backend := map[string]string{"typescript": "tsserver", "python": "pyright"}[language]
				omissions = append(omissions, graphOmissionForRepo(backend, "declarations", "backend_gated", "enable_semantic_backend", repo))
				warnings = append(warnings, "graph backend gated for "+language+"; no semantic claims were emitted")
			}
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

func graphObservationTargets(root string, project model.ProjectFile, projectConfigExplicit bool) ([]graphObservationTarget, bool) {
	if len(project.Repos) == 0 {
		return nil, false
	}
	if project.Project.Kind == model.WorkspaceKindContainer && !projectConfigExplicit {
		return nil, false
	}
	selected := strings.TrimSpace(project.Project.DefaultRepo)
	if project.Project.Kind != model.WorkspaceKindContainer {
		if selected == "" && len(project.Repos) == 1 {
			selected = project.Repos[0].ID
		}
		if selected == "" {
			return nil, false
		}
		for _, repo := range project.Repos {
			if repo.ID != selected {
				continue
			}
			return graphCSharpProjects(root, repo, project.Entrypoints), true
		}
		return nil, false
	}

	targets := make([]graphObservationTarget, 0)
	for _, repo := range project.Repos {
		targets = append(targets, graphCSharpProjects(root, repo, project.Entrypoints)...)
	}
	sort.Slice(targets, func(i, j int) bool {
		left := strings.ToLower(targets[i].WorkspaceEntrypointPath)
		right := strings.ToLower(targets[j].WorkspaceEntrypointPath)
		if left != right {
			return left < right
		}
		return targets[i].Entrypoint.ID < targets[j].Entrypoint.ID
	})
	return targets, true
}

func graphCSharpProjects(root string, repo model.WorkspaceRepo, entrypoints []model.WorkspaceEntrypoint) []graphObservationTarget {
	repoRoot := filepath.Join(root, filepath.FromSlash(normalizeRepoRoot(repo.Root)))
	seen := map[string]struct{}{}
	projects := make([]graphObservationTarget, 0)
	for _, entrypoint := range entrypoints {
		if entrypoint.RepoID != repo.ID || entrypoint.Kind != model.EntrypointKindProject || !strings.EqualFold(filepath.Ext(entrypoint.Path), ".csproj") {
			continue
		}
		workspacePath := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(entrypoint.Path))))
		if workspacePath == "." || filepath.IsAbs(filepath.FromSlash(workspacePath)) {
			continue
		}
		candidate := filepath.Join(root, filepath.FromSlash(workspacePath))
		info, err := os.Stat(candidate)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		projectPath, err := filepath.Rel(repoRoot, candidate)
		if err != nil || projectPath == ".." || strings.HasPrefix(projectPath, ".."+string(filepath.Separator)) || filepath.IsAbs(projectPath) {
			continue
		}
		projectPath = filepath.ToSlash(filepath.Clean(projectPath))
		key := strings.ToLower(filepath.Clean(projectPath))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		projects = append(projects, graphObservationTarget{
			Repo:                    repo,
			Entrypoint:              entrypoint,
			RepoRoot:                repoRoot,
			ProjectOrModule:         projectPath,
			WorkspaceEntrypointPath: workspacePath,
		})
	}
	sort.Slice(projects, func(i, j int) bool {
		left := strings.ToLower(projects[i].ProjectOrModule)
		right := strings.ToLower(projects[j].ProjectOrModule)
		if left != right {
			return left < right
		}
		return projects[i].Entrypoint.ID < projects[j].Entrypoint.ID
	})
	return projects
}

type graphGoRepo struct {
	repo model.WorkspaceRepo
	root string
}

func graphGoRepos(root string, project model.ProjectFile, targets []graphObservationTarget) []graphGoRepo {
	if project.Project.Kind != model.WorkspaceKindContainer {
		selected := strings.TrimSpace(project.Project.DefaultRepo)
		for _, repo := range project.Repos {
			if (repo.ID == selected || (selected == "" && len(project.Repos) == 1)) && hasLanguage(repo, "go") {
				return []graphGoRepo{{repo: repo, root: filepath.Join(root, filepath.FromSlash(normalizeRepoRoot(repo.Root)))}}
			}
		}
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]graphGoRepo, 0)
	for _, target := range targets {
		if !hasLanguage(target.Repo, "go") || target.Repo.Root == "" {
			continue
		}
		if _, ok := seen[strings.ToLower(target.Repo.ID)]; ok {
			continue
		}
		seen[strings.ToLower(target.Repo.ID)] = struct{}{}
		result = append(result, graphGoRepo{repo: target.Repo, root: target.Repo.Root})
	}
	for _, repo := range project.Repos {
		if !hasLanguage(repo, "go") {
			continue
		}
		if _, ok := seen[strings.ToLower(repo.ID)]; ok {
			continue
		}
		result = append(result, graphGoRepo{repo: repo, root: repo.Root})
	}
	return result
}

func graphRootGoModule(root string) string {
	for _, name := range []string{"go.mod", "go.work"} {
		path := filepath.Join(root, name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return name
		}
	}
	return ""
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

func rebaseGraphObservationBatch(batch *model.GraphObservationBatch, repoRoot string) error {
	if batch == nil {
		return errors.New("graph observation batch is nil")
	}
	prefix := normalizeRepoRoot(repoRoot)
	join := func(path string) string {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if prefix == "." || prefix == "" {
			return path
		}
		return filepath.ToSlash(filepath.Join(filepath.FromSlash(prefix), filepath.FromSlash(path)))
	}
	batch.ProjectOrModule = join(batch.ProjectOrModule)
	for i := range batch.Nodes {
		batch.Nodes[i].Key.ProjectOrModule = join(batch.Nodes[i].Key.ProjectOrModule)
		batch.Nodes[i].Key.OwnerPath = join(batch.Nodes[i].Key.OwnerPath)
	}
	for i := range batch.Edges {
		batch.Edges[i].OwnerPath = join(batch.Edges[i].OwnerPath)
	}
	for i := range batch.Evidence {
		batch.Evidence[i].SourceURI = join(batch.Evidence[i].SourceURI)
	}
	for i := range batch.Unresolved {
		batch.Unresolved[i].OwnerPath = join(batch.Unresolved[i].OwnerPath)
	}
	for i := range batch.Omissions {
		batch.Omissions[i].OwnerPath = join(batch.Omissions[i].OwnerPath)
	}
	return nil
}

func normalizeRepoRoot(root string) string {
	root = filepath.ToSlash(strings.TrimSpace(root))
	if root == "" || root == "." {
		return "."
	}
	root = strings.TrimPrefix(root, "./")
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(root)))
	if clean == "" {
		return "."
	}
	return clean
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
	return model.GraphObservationOmission{Ref: "omission:" + backend + ":" + reason, OwnerPath: ".", SubjectKind: "repository", Backend: backend, Capability: capability, ReasonCode: reason, RecoveryHintCode: recovery}
}

func graphOmissionForRepo(backend, capability, reason, recovery string, repo model.WorkspaceRepo) model.GraphObservationOmission {
	omission := graphOmission(backend, capability, reason, recovery)
	omission.Ref = "omission:" + repo.ID + ":" + backend + ":" + reason
	omission.OwnerPath = normalizeRepoRoot(repo.Root)
	return omission
}

func graphPartialOmission(target graphObservationTarget, batch model.GraphObservationBatch) model.GraphObservationOmission {
	return model.GraphObservationOmission{
		Ref:              "omission:" + target.Repo.ID + ":roslyn:partial:" + batch.ProjectOrModule,
		OwnerPath:        normalizeRepoRoot(target.Repo.Root),
		SubjectKind:      "project",
		Backend:          "roslyn",
		Capability:       "declarations",
		ReasonCode:       "backend_partial",
		RecoveryHintCode: "repair_project_or_retry",
	}
}
