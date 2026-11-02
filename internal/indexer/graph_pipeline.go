package indexer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
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
	// AffectedPaths is nil for legacy observation, otherwise repo-relative.
	AffectedPaths []string
	// AppliedPaths receives the worker's exact acknowledged repo-relative paths.
	AppliedPaths *[]string
}

// GraphObserver is the adapter boundary for external semantic compilers.
type GraphObserver func(context.Context, GraphObservationRequest) (model.GraphObservationBatch, error)

// GoGraphDeltaObserver executes the exact Go request selected by canonical
// planning and returns its raw compiler receipt for canonical sealing.
type GoGraphDeltaObserver func(context.Context, GraphObservationRequest, GoGraphObservationRequest, []string) (model.GraphObservationBatch, GoGraphDeltaReceipt, error)

type GraphIndexOptions struct {
	// TopologyWitnessOut receives the generation-owned native Go topology captured
	// by the same observation request. It is never reconstructed from later files.
	TopologyWitnessOut **model.GraphTopologyWitness
	RoslynObserver     GraphObserver
	GoDeltaObserver    GoGraphDeltaObserver
	EntrypointSelector string
	GoOffline          bool
	// AffectedPaths is nil for legacy observation, otherwise workspace-relative.
	AffectedPaths []string
	// PreviousGoModulePaths maps workspace-relative go.mod paths to prior source
	// owners, also workspace-relative. Root must populate these from the fixed base.
	PreviousGoModulePaths map[string][]string
	// PreviousGoPackageOwners holds pinned package-owner candidates keyed by workspace-relative go.mod.
	PreviousGoPackageOwners map[string][]string
	DeltaCoverage           *GraphDeltaCoverage
	RoslynDeltaMembers      GraphDeltaMembership
}

type graphObservationTarget struct {
	Repo                    model.WorkspaceRepo
	Entrypoint              model.WorkspaceEntrypoint
	RepoRoot                string
	ProjectOrModule         string
	WorkspaceEntrypointPath string
}

// graphIdentityUnavailableWarning is the sanitized, actionable warning emitted
// when the graph is omitted because no repository identity can be resolved.
const graphIdentityUnavailableWarning = "graph not published: repository identity unavailable (set exactly one git remote origin or declare repository_identity)"

var errGraphTopologyAmbiguous = errors.New("graph topology is ambiguous; no repository was selected")

// ObserveGraph runs local Go observation and the externally supplied Roslyn
// observer. Unsupported languages are recorded as gated omissions, never as
// semantic graph claims.
func ObserveGraph(ctx context.Context, root string, project model.ProjectFile, options GraphIndexOptions, progress ProgressFunc) ([]model.GraphObservationBatch, []model.GraphObservationOmission, []string, error) {
	if options.TopologyWitnessOut != nil {
		*options.TopologyWitnessOut = nil
	}
	if options.AffectedPaths != nil && options.DeltaCoverage != nil {
		options.DeltaCoverage.reset(options.AffectedPaths)
	}
	if err := reportProgress(ctx, progress, Progress{Stage: "graph.observe", Force: true}); err != nil {
		return nil, nil, nil, err
	}
	if options.AffectedPaths != nil && len(options.AffectedPaths) == 0 {
		return nil, nil, nil, nil
	}
	if options.AffectedPaths != nil {
		if err := validateGraphDeltaPaths(options.AffectedPaths); err != nil {
			return nil, nil, nil, err
		}
	}
	_, configErr := os.Stat(workspace.ProjectConfigPath(root))
	targets, ok := graphObservationTargets(root, project, configErr == nil)
	if !ok && options.AffectedPaths != nil && options.DeltaCoverage != nil {
		options.DeltaCoverage.TopologyAmbiguous = true
	}
	if project.Project.Kind == model.WorkspaceKindContainer && !ok && strings.TrimSpace(options.EntrypointSelector) == "" {
		return nil, []model.GraphObservationOmission{{Backend: "roslyn", Capability: "declarations", ReasonCode: "ambiguous_repository", RecoveryHintCode: "select_repo"}}, []string{"graph omitted: container topology does not explicitly select one authoritative repository"}, nil
	}
	if !ok && strings.TrimSpace(options.EntrypointSelector) == "" {
		return nil, nil, nil, errGraphTopologyAmbiguous
	}

	var explicitSelection *graphExplicitSelection
	if selector := strings.TrimSpace(options.EntrypointSelector); selector != "" {
		selection, err := resolveGraphExplicitSelection(root, project, selector)
		if err != nil {
			return nil, nil, nil, err
		}
		explicitSelection = &selection
		targets = graphTargetsForExplicitSelection(root, selection)
		if len(targets) == 0 && selection.Backend == "roslyn" {
			return nil, nil, nil, &model.GraphObservationError{Code: "GPH_ENTRYPOINT_UNAVAILABLE", Field: "entrypoint", Message: "explicit C# entrypoint has no eligible project file"}
		}
	}

	if explicitSelection != nil && explicitSelection.Backend == "kotlin-language-server" {
		if options.AffectedPaths != nil {
			return nil, nil, nil, &model.GraphObservationError{Code: "GPH_DELTA_UNAVAILABLE", Field: "affected_paths", Message: "explicit_incomplete: Kotlin selective observation is not stageable"}
		}
		selected := *explicitSelection
		identity, err := workspace.ResolveRepositoryIdentity(ctx, root, project.Repos)
		if err != nil {
			return nil, nil, nil, &model.GraphObservationError{Code: "GPH_IDENTITY_UNAVAILABLE", Field: "repository_identity", Message: "repository identity could not be resolved"}
		}
		if options.RoslynObserver == nil {
			return nil, nil, nil, &model.GraphObservationError{Code: "GPH_BACKEND_UNAVAILABLE", Field: "backend", Message: "external graph observer is not configured"}
		}
		module, err := filepath.Rel(selected.RepoRoot, filepath.Join(root, filepath.FromSlash(selected.Entrypoint.Path)))
		if err != nil {
			return nil, nil, nil, err
		}
		batch, observeErr := options.RoslynObserver(ctx, GraphObservationRequest{
			WorkspaceRoot: root, WorkspaceIdentity: identity, RepositoryIdentity: identity,
			RepoID: selected.Repo.ID, RepoName: selected.Repo.Name, RepoRoot: selected.RepoRoot,
			EntrypointID: selected.Entrypoint.ID, EntrypointPath: selected.Entrypoint.Path,
			ProjectOrModule: filepath.ToSlash(module), EntrypointKind: selected.Entrypoint.Kind, Backend: selected.Backend,
		})
		if observeErr != nil {
			return nil, nil, nil, observeErr
		}
		if batch.Backend != selected.Backend || batch.RepositoryIdentity != identity || batch.WorkspaceIdentity != identity {
			return nil, nil, nil, &model.GraphObservationError{Code: "GPH_BACKEND_MISMATCH", Field: "backend", Message: "Kotlin observer returned mismatched identity/backend"}
		}
		if err := batch.ValidateCanonical(); err != nil {
			return nil, nil, nil, err
		}
		if err := rebaseGraphObservationBatch(&batch, selected.Repo.Root); err != nil {
			return nil, nil, nil, err
		}
		if err := model.SealGraphObservationBatch(&batch); err != nil {
			return nil, nil, nil, err
		}
		warnings := []string{"Kotlin calls unsupported; graph domain is named main-jvm declarations and local compiler references only"}
		if err := batch.ReadyForStaging(); err != nil {
			if batch.Completeness != model.GraphCompletenessPartial {
				return nil, nil, warnings, err
			}
			warnings = append(warnings, "explicit_incomplete: Kotlin partial observation not staged: "+graphObservationBatchDiagnostics(batch))
			return nil, append(batch.Omissions, graphOmissionForRepo(selected.Backend, "declarations", "backend_partial", "satisfy_rf_gph_008_promotion_oracles", selected.Repo)), warnings, nil
		}
		return []model.GraphObservationBatch{batch}, nil, warnings, nil
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
	if explicitSelection != nil {
		if explicitSelection.Backend == "go" {
			goRepos = []graphGoRepo{{repo: explicitSelection.Repo, root: explicitSelection.RepoRoot}}
			csharpTargets = nil
		} else {
			goRepos = nil
			csharpTargets = targets
		}
	}
	if csharpDeclared && len(csharpTargets) == 0 && explicitSelection == nil {
		return nil, nil, nil, errors.New("no eligible C# project entrypoint was declared")
	}

	identity := ""
	if len(goRepos) > 0 || len(csharpTargets) > 0 {
		var err error
		identity, err = workspace.ResolveRepositoryIdentity(ctx, root, project.Repos)
		if err != nil {
			if explicitSelection != nil {
				return nil, nil, nil, &model.GraphObservationError{Code: "GPH_IDENTITY_UNAVAILABLE", Field: "repository_identity", Message: "repository identity could not be resolved"}
			}
			// RF-GPH-007: without an explicit repository_identity or exactly one
			// origin, the catalog/docs index continues and only the graph
			// publication is omitted. Identity is never derived from the path.
			backend := "go"
			if len(goRepos) == 0 {
				backend = "roslyn"
			}
			return nil, []model.GraphObservationOmission{graphOmission(backend, "declarations", "identity_unavailable", "declare_repository_identity")}, []string{graphIdentityUnavailableWarning}, nil
		}
	}

	batches := make([]model.GraphObservationBatch, 0, len(targets)+1)
	omissions := make([]model.GraphObservationOmission, 0)
	warnings := make([]string, 0)

	if len(goRepos) > 0 {
		if options.AffectedPaths != nil && project.Project.Kind == model.WorkspaceKindContainer && explicitSelection == nil && len(goRepos) > 1 {
			warnings = append(warnings, "explicit_incomplete: Go container has multiple repositories and a workspace-root legacy observation; select an entrypoint before masking Go paths")
			if options.DeltaCoverage != nil {
				options.DeltaCoverage.TopologyAmbiguous = true
			}
			goto skipGoObservation
		}
		module := ""
		if explicitSelection != nil {
			selected := goRepos[0]
			configured, selectorOK := graphGoModuleSelector(root, project, selected.repo, &explicitSelection.Entrypoint)
			if selectorOK {
				module = graphGoModule(selected.root, configured)
			}
		} else if project.Project.Kind == model.WorkspaceKindContainer {
			module = graphRootGoModule(root)
		} else {
			selected := goRepos[0]
			configured, selectorOK := graphGoModuleSelector(root, project, selected.repo, nil)
			if selectorOK {
				module = graphGoModule(selected.root, configured)
			}
		}
		if explicitSelection != nil && module == "" {
			return nil, nil, nil, &model.GraphObservationError{Code: "GPH_ENTRYPOINT_INVALID", Field: "entrypoint", Message: "explicit entrypoint must resolve to a safe regular go.mod within the selected repository"}
		}
		if module == "" {
			omissions = append(omissions, graphOmissionForRepo("go", "declarations", "go_module_missing", "declare_go_module", goRepos[0].repo))
			warnings = append(warnings, "graph omitted Go: no explicit go.mod entrypoint")
		} else {
			requestRoot := root
			if project.Project.Kind != model.WorkspaceKindContainer || explicitSelection != nil {
				requestRoot = goRepos[0].root
			}
			if options.GoOffline {
				warnings = append(warnings, "go_offline=true; Go observation uses local toolchain/cache, readonly module metadata and no workspace mode; inherited GOFLAGS ignored")
			}
			var topologyCapture GoGraphTopologyCapture
			req := GoGraphObservationRequest{Root: requestRoot, RepositoryIdentity: identity, ProjectOrModule: module, GoOffline: options.GoOffline}
			if options.TopologyWitnessOut != nil {
				req.TopologyCapture = &topologyCapture
			}
			var selectedPaths []string
			var goReceipt GoGraphDeltaReceipt
			var goAppliedPaths []string
			var batch model.GraphObservationBatch
			var observeErr error
			if options.AffectedPaths == nil {
				batch, observeErr = ObserveGoGraph(ctx, req)
			} else {
				paths, err := graphDeltaPathsForRoot(root, requestRoot, options.AffectedPaths, "")
				if err != nil {
					return nil, omissions, warnings, err
				}
				moduleWorkspacePath, relErr := filepath.Rel(root, filepath.Join(requestRoot, filepath.FromSlash(module)))
				if relErr != nil {
					return nil, omissions, warnings, relErr
				}
				moduleWorkspacePath = filepath.ToSlash(moduleWorkspacePath)
				moduleRequestPath := filepath.ToSlash(module)
				controlPaths := map[string]bool{filepath.ToSlash(filepath.Join(filepath.Dir(moduleRequestPath), "go.sum")): true}
				controlPaths[moduleRequestPath] = true
				for _, path := range paths {
					if strings.EqualFold(filepath.Ext(path), ".go") || controlPaths[path] {
						selectedPaths = append(selectedPaths, path)
					}
				}
				if len(selectedPaths) == 0 {
					goto skipGoObservation
				}
				if previous := options.PreviousGoModulePaths[moduleWorkspacePath]; len(previous) > 0 {
					prior, err := graphDeltaPathsForRoot(root, requestRoot, previous, ".go")
					if err != nil {
						return nil, omissions, warnings, err
					}
					req.PreviousModulePaths = prior
				}
				req.PreviousPackageOwners = append([]string(nil), options.PreviousGoPackageOwners[moduleWorkspacePath]...)
				if options.GoDeltaObserver != nil {
					request := GraphObservationRequest{WorkspaceRoot: root, WorkspaceIdentity: identity, RepositoryIdentity: identity, RepoID: goRepos[0].repo.ID, RepoName: goRepos[0].repo.Name, RepoRoot: requestRoot, ProjectOrModule: module, Backend: "go", AffectedPaths: append([]string(nil), selectedPaths...)}
					batch, goReceipt, observeErr = options.GoDeltaObserver(ctx, request, req, selectedPaths)
				} else {
					batch, goReceipt, observeErr = ObserveGoGraphDeltaWithReceipt(ctx, req, selectedPaths)
				}
				if observeErr == nil && options.DeltaCoverage != nil {
					bridge, bridgeErr := CanonicalizeGoGraphDelta(root, requestRoot, module, &batch, goReceipt)
					if bridgeErr != nil {
						return nil, omissions, warnings, bridgeErr
					}
					goAppliedPaths = append([]string(nil), bridge.RawReceipt.Applied...)
					goReceipt = bridge.CanonicalReceipt
					if options.DeltaCoverage != nil {
						options.DeltaCoverage.GoReceiptBridges = append(options.DeltaCoverage.GoReceiptBridges, bridge)
					}
				}
			}
			if observeErr != nil {
				return nil, omissions, warnings, fmt.Errorf("go graph observation failed: %w", observeErr)
			}
			if err := batch.ReadyForStaging(); err != nil {
				return nil, omissions, warnings, wrapGoObservationStageError(batch, err)
			}
			if options.TopologyWitnessOut != nil {
				requestPrefix, prefixErr := graphTopologyWorkspacePrefix(root, requestRoot)
				if prefixErr != nil {
					return nil, omissions, warnings, prefixErr
				}
				repoIDs := make([]string, 0, len(goRepos))
				for _, selectedRepo := range goRepos {
					if _, prefixErr := graphTopologyWorkspacePrefix(root, selectedRepo.root); prefixErr != nil {
						return nil, omissions, warnings, prefixErr
					}
					repoIDs = append(repoIDs, selectedRepo.repo.ID)
				}
				modulePath := filepath.ToSlash(module)
				goModPath := path.Join(requestPrefix, modulePath)
				observedGoModPath, modPathErr := graphTopologyWorkspacePrefix(root, filepath.Join(requestRoot, filepath.FromSlash(modulePath)))
				if modPathErr != nil || observedGoModPath != goModPath {
					return nil, omissions, warnings, fmt.Errorf("explicit_incomplete: consumed go.mod has an unproven physical workspace projection")
				}
				witness, witnessErr := model.NewGraphTopologyWitness(project, identity)
				if witnessErr != nil || topologyCapture.ProjectOrModule != modulePath || topologyCapture.ModulePath == "" || topologyCapture.GoModDigest == (model.GraphDigest{}) || topologyCapture.BatchConfigFingerprint != batch.ConfigFingerprint {
					return nil, omissions, warnings, fmt.Errorf("explicit_incomplete: native Go topology capture is incomplete")
				}
				selectorKind, selectorID := "implicit_repo", "implicit:go.mod"
				if project.Project.Kind == model.WorkspaceKindContainer && explicitSelection == nil {
					selectorKind, selectorID = "container_root", "workspace-root:go.mod"
				} else if explicitSelection != nil {
					selectorKind, selectorID = "explicit", explicitSelection.Entrypoint.ID
				} else if configured := strings.TrimSpace(goRepos[0].repo.DefaultEntrypoint); configured != "" {
					selectorKind, selectorID = "configured", configured
					for _, entry := range project.Entrypoints {
						if entry.RepoID == goRepos[0].repo.ID && strings.EqualFold(entry.ID, configured) {
							selectorID = entry.ID
							break
						}
					}
				}
				witness.Modules = append(witness.Modules, model.GraphTopologyGoModule{
					RepositoryIDs: repoIDs, SelectorID: selectorID, SelectorKind: selectorKind, SelectorPath: goModPath,
					RequestRoot: requestPrefix, ProjectOrModule: modulePath,
					GoModPath: goModPath, ModulePath: topologyCapture.ModulePath,
					GoModDigest: topologyCapture.GoModDigest, BatchConfigFingerprint: topologyCapture.BatchConfigFingerprint,
				})
				*options.TopologyWitnessOut = witness
			}
			if options.DeltaCoverage != nil && options.AffectedPaths != nil {
				projected, err := graphGoControlCoverage(root, requestRoot, module, goReceipt)
				if err != nil {
					return nil, omissions, warnings, err
				}
				options.DeltaCoverage.GoReceipts = append(options.DeltaCoverage.GoReceipts, goReceipt)
				options.DeltaCoverage.GoControlCertificates = append(options.DeltaCoverage.GoControlCertificates, projected)
				for _, certificate := range goReceipt.PackageOwnerCertificates {
					certificate.Module = goReceipt.ProjectOrModule
					options.DeltaCoverage.GoPackageCertificates = append(options.DeltaCoverage.GoPackageCertificates, certificate)
				}
				options.DeltaCoverage.record(root, requestRoot, "go", module, goAppliedPaths)
			}
			batches = append(batches, batch)
		}
	}

skipGoObservation:
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
			if options.AffectedPaths != nil {
				paths, err := graphDeltaPathsForRoot(root, target.RepoRoot, options.AffectedPaths, ".cs")
				if err != nil {
					return nil, omissions, warnings, err
				}
				if len(paths) == 0 {
					continue
				}
				if options.RoslynDeltaMembers == nil {
					warnings = append(warnings, "explicit_incomplete: Roslyn project membership unavailable for "+target.WorkspaceEntrypointPath)
					continue
				}
				paths, err = options.RoslynDeltaMembers(ctx, request, paths)
				if err != nil {
					return nil, omissions, warnings, err
				}
				if err := graphDeltaValidateSubset(paths, options.AffectedPaths, root, target.RepoRoot); err != nil {
					return nil, omissions, warnings, err
				}
				if len(paths) == 0 {
					continue
				}
				request.AffectedPaths = paths
				request.AppliedPaths = new([]string)
			}
			batch, observeErr := options.RoslynObserver(ctx, request)
			if observeErr != nil {
				return nil, omissions, warnings, observeErr
			}
			if options.AffectedPaths != nil && (request.AppliedPaths == nil || *request.AppliedPaths == nil || !graphDeltaSamePaths(request.AffectedPaths, *request.AppliedPaths)) {
				return nil, omissions, warnings, errors.New("explicit_incomplete: Roslyn worker did not acknowledge the exact project paths")
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
			if options.DeltaCoverage != nil && options.AffectedPaths != nil {
				options.DeltaCoverage.record(root, target.RepoRoot, "roslyn", target.ProjectOrModule, request.AffectedPaths)
			}
		}
	}

	for _, repo := range project.Repos {
		for _, language := range []string{"typescript", "python", "kotlin"} {
			if hasLanguage(repo, language) {
				backend := map[string]string{"typescript": "tsserver", "python": "pyright", "kotlin": "kotlin-language-server"}[language]
				omissions = append(omissions, graphOmissionForRepo(backend, "declarations", "backend_gated", "enable_semantic_backend", repo))
				warnings = append(warnings, "graph backend gated for "+language+"; no semantic claims were emitted")
			}
		}
	}
	if options.AffectedPaths != nil && options.DeltaCoverage != nil {
		for _, path := range options.DeltaCoverage.Paths {
			if len(path.Owners) == 0 {
				warnings = append(warnings, "explicit_incomplete: uncovered affected path "+path.WorkspacePath)
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

type graphExplicitSelection struct {
	Repo       model.WorkspaceRepo
	RepoRoot   string
	Entrypoint model.WorkspaceEntrypoint
	Backend    string
}

func resolveGraphExplicitSelection(root string, project model.ProjectFile, selector string) (graphExplicitSelection, error) {
	entrypoint, found := workspace.FindEntrypoint(project, selector)
	if !found {
		workspacePath, safe := graphSafeExplicitPath(root, selector)
		if !safe {
			matches := make([]string, 0, len(project.Repos))
			for _, candidateRepo := range project.Repos {
				localPath, localSafe := graphSafeExplicitPath(filepath.Join(root, filepath.FromSlash(normalizeRepoRoot(candidateRepo.Root))), selector)
				if !localSafe {
					continue
				}
				matches = append(matches, filepath.ToSlash(filepath.Join(filepath.FromSlash(normalizeRepoRoot(candidateRepo.Root)), filepath.FromSlash(localPath))))
			}
			if len(matches) == 1 {
				workspacePath, safe = matches[0], true
			} else if len(matches) > 1 {
				return graphExplicitSelection{}, &model.GraphObservationError{Code: "GPH_ENTRYPOINT_AMBIGUOUS", Field: "entrypoint", Message: "explicit repository-relative entrypoint matches multiple repositories; use a declared entrypoint ID or workspace-relative path"}
			}
		}
		if !safe {
			return graphExplicitSelection{}, &model.GraphObservationError{Code: "GPH_ENTRYPOINT_NOT_FOUND", Field: "entrypoint", Message: "explicit entrypoint was not found; select a declared entrypoint or an existing safe repository-relative Go/C#/Gradle module path"}
		}
		repo, repoFound := workspace.FindRepoByFile(project, root, workspacePath)
		if !repoFound {
			return graphExplicitSelection{}, &model.GraphObservationError{Code: "GPH_ENTRYPOINT_REPO_NOT_FOUND", Field: "entrypoint", Message: "explicit entrypoint is outside every selected repository"}
		}
		kind := model.EntrypointKindProject
		if strings.EqualFold(filepath.Ext(filepath.FromSlash(workspacePath)), ".sln") {
			kind = model.EntrypointKindSolution
		}
		entrypoint = model.WorkspaceEntrypoint{ID: workspacePath, RepoID: repo.ID, Path: workspacePath, Kind: kind}
	}

	repo, repoFound := workspace.FindRepo(project, entrypoint.RepoID)
	if !repoFound {
		return graphExplicitSelection{}, &model.GraphObservationError{Code: "GPH_ENTRYPOINT_REPO_NOT_FOUND", Field: "entrypoint", Message: "explicit entrypoint references an undeclared repository"}
	}
	repoRoot := filepath.Join(root, filepath.FromSlash(normalizeRepoRoot(repo.Root)))
	if entrypoint.Kind == model.EntrypointKindSolution || strings.EqualFold(filepath.Ext(filepath.FromSlash(entrypoint.Path)), ".sln") {
		return graphExplicitSelection{}, &model.GraphObservationError{Code: "GPH_ENTRYPOINT_SOLUTION_UNAVAILABLE", Field: "entrypoint", Message: "explicit C# solution selection is unavailable; select a declared or safe repository-relative .csproj"}
	}
	if !graphExplicitPathBelongsToRepo(root, repoRoot, entrypoint.Path) {
		return graphExplicitSelection{}, &model.GraphObservationError{Code: "GPH_ENTRYPOINT_REPO_MISMATCH", Field: "entrypoint", Message: "explicit entrypoint path does not physically belong to its declared repository"}
	}
	backend := graphEntrypointBackend(entrypoint)
	if backend == "" {
		return graphExplicitSelection{}, &model.GraphObservationError{Code: "GPH_ENTRYPOINT_UNSUPPORTED", Field: "entrypoint", Message: "explicit entrypoint is not a supported Go module, C# project or experimental Kotlin Gradle module; explicit solutions are unavailable"}
	}
	if backend == "kotlin-language-server" && !hasLanguage(repo, "kotlin") {
		return graphExplicitSelection{}, &model.GraphObservationError{Code: "GPH_ENTRYPOINT_LANGUAGE_MISMATCH", Field: "entrypoint", Message: "explicit Gradle entrypoint belongs to a repository without Kotlin support"}
	}
	if backend == "go" && !hasLanguage(repo, "go") {
		return graphExplicitSelection{}, &model.GraphObservationError{Code: "GPH_ENTRYPOINT_LANGUAGE_MISMATCH", Field: "entrypoint", Message: "explicit Go entrypoint belongs to a repository without Go support"}
	}
	if backend == "roslyn" && !hasLanguage(repo, "csharp") && !hasLanguage(repo, "cs") && !hasLanguage(repo, "dotnet") {
		return graphExplicitSelection{}, &model.GraphObservationError{Code: "GPH_ENTRYPOINT_LANGUAGE_MISMATCH", Field: "entrypoint", Message: "explicit C# entrypoint belongs to a repository without C# support"}
	}
	return graphExplicitSelection{Repo: repo, RepoRoot: repoRoot, Entrypoint: entrypoint, Backend: backend}, nil
}

func graphExplicitPathBelongsToRepo(workspaceRoot, repoRoot, configured string) bool {
	workspacePath, safe := graphSafeExplicitPath(workspaceRoot, configured)
	if !safe {
		return false
	}
	workspaceRootReal, err := filepath.EvalSymlinks(workspaceRoot)
	if err != nil {
		return false
	}
	repoRootReal, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return false
	}
	candidateReal, err := filepath.EvalSymlinks(filepath.Join(workspaceRoot, filepath.FromSlash(workspacePath)))
	if err != nil {
		return false
	}
	if _, err := filepath.Rel(workspaceRootReal, candidateReal); err != nil {
		return false
	}
	relative, err := filepath.Rel(repoRootReal, candidateReal)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func graphSafeExplicitPath(root, configured string) (string, bool) {
	configured = strings.TrimSpace(configured)
	driveRelative := len(configured) >= 2 &&
		((configured[0] >= 'A' && configured[0] <= 'Z') || (configured[0] >= 'a' && configured[0] <= 'z')) &&
		configured[1] == ':' && (len(configured) == 2 || (configured[2] != '/' && configured[2] != '\\'))
	if configured == "" || strings.ContainsRune(configured, 0) || strings.Contains(configured, "\\") || filepath.IsAbs(configured) || driveRelative {
		return "", false
	}
	for _, part := range strings.Split(configured, "/") {
		if part == ".." {
			return "", false
		}
	}
	clean := filepath.Clean(configured)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	candidate := filepath.Join(rootAbs, clean)
	relative, err := filepath.Rel(rootAbs, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", false
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", false
	}
	candidateReal, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", false
	}
	relativeReal, err := filepath.Rel(rootReal, candidateReal)
	if err != nil || relativeReal == ".." || strings.HasPrefix(relativeReal, ".."+string(filepath.Separator)) || filepath.IsAbs(relativeReal) {
		return "", false
	}
	info, err := os.Lstat(candidate)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	ext := strings.ToLower(filepath.Ext(clean))
	base := filepath.Base(clean)
	if ext != ".csproj" && ext != ".sln" && strings.ToLower(base) != "go.mod" && base != "build.gradle" && base != "build.gradle.kts" {
		return "", false
	}
	return filepath.ToSlash(clean), true
}

func graphEntrypointBackend(entrypoint model.WorkspaceEntrypoint) string {
	path := filepath.ToSlash(strings.TrimSpace(entrypoint.Path))
	base := filepath.Base(filepath.FromSlash(path))
	if base == "build.gradle" || base == "build.gradle.kts" {
		return "kotlin-language-server"
	}
	if strings.EqualFold(filepath.Base(filepath.FromSlash(path)), "go.mod") {
		if entrypoint.Kind != model.EntrypointKindProject {
			return ""
		}
		return "go"
	}
	if strings.EqualFold(filepath.Ext(filepath.FromSlash(path)), ".csproj") {
		return "roslyn"
	}
	return ""
}

func graphTargetsForExplicitSelection(root string, selection graphExplicitSelection) []graphObservationTarget {
	if selection.Backend != "roslyn" {
		return nil
	}
	return graphCSharpProjects(root, selection.Repo, []model.WorkspaceEntrypoint{selection.Entrypoint})
}

func graphTopologyWorkspacePrefix(workspaceRoot, selectedRoot string) (string, error) {
	rootAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", fmt.Errorf("explicit_incomplete: workspace topology root is unavailable")
	}
	selectedAbs := selectedRoot
	if !filepath.IsAbs(selectedAbs) {
		selectedAbs = filepath.Join(rootAbs, selectedAbs)
	}
	selectedAbs, err = filepath.Abs(selectedAbs)
	if err != nil {
		return "", fmt.Errorf("explicit_incomplete: selected topology root is unavailable")
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", fmt.Errorf("explicit_incomplete: workspace topology root cannot be resolved")
	}
	selectedReal, err := filepath.EvalSymlinks(selectedAbs)
	if err != nil {
		return "", fmt.Errorf("explicit_incomplete: selected topology root cannot be resolved")
	}
	lexical, err := filepath.Rel(rootAbs, selectedAbs)
	if err != nil || lexical == ".." || strings.HasPrefix(lexical, ".."+string(filepath.Separator)) || filepath.IsAbs(lexical) {
		return "", fmt.Errorf("explicit_incomplete: selected topology root is outside the workspace")
	}
	physical, err := filepath.Rel(rootReal, selectedReal)
	if err != nil || physical == ".." || strings.HasPrefix(physical, ".."+string(filepath.Separator)) || filepath.IsAbs(physical) || filepath.Clean(physical) != filepath.Clean(lexical) {
		return "", fmt.Errorf("explicit_incomplete: selected topology root has an unproven symlink mapping")
	}
	return filepath.ToSlash(filepath.Clean(lexical)), nil
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
func graphGoModuleSelector(workspaceRoot string, project model.ProjectFile, repo model.WorkspaceRepo, explicit *model.WorkspaceEntrypoint) (string, bool) {
	configured := strings.TrimSpace(repo.DefaultEntrypoint)
	if explicit != nil {
		workspacePath, safe := graphSafeRelativeModule(workspaceRoot, explicit.Path)
		if !safe {
			return "", false
		}
		repoRoot := filepath.Join(workspaceRoot, filepath.FromSlash(normalizeRepoRoot(repo.Root)))
		repoRootAbs, err := filepath.Abs(repoRoot)
		if err != nil {
			return "", false
		}
		workspaceRootAbs, err := filepath.Abs(workspaceRoot)
		if err != nil {
			return "", false
		}
		modulePath := filepath.Join(workspaceRootAbs, filepath.FromSlash(workspacePath))
		localPath, err := filepath.Rel(repoRootAbs, modulePath)
		if err != nil || localPath == ".." || strings.HasPrefix(localPath, ".."+string(filepath.Separator)) || filepath.IsAbs(localPath) {
			return "", false
		}
		localPath = filepath.ToSlash(filepath.Clean(localPath))
		if _, safe := graphSafeRelativeModule(repoRoot, localPath); !safe {
			return "", false
		}
		return localPath, true
	}
	if configured == "" {
		return "", true
	}
	for _, entrypoint := range project.Entrypoints {
		if !strings.EqualFold(strings.TrimSpace(entrypoint.RepoID), strings.TrimSpace(repo.ID)) || !strings.EqualFold(strings.TrimSpace(entrypoint.ID), configured) {
			continue
		}
		workspacePath, safe := graphSafeRelativeModule(workspaceRoot, entrypoint.Path)
		if !safe {
			return "", false
		}
		workspaceRootAbs, err := filepath.Abs(workspaceRoot)
		if err != nil {
			return "", false
		}
		repoRoot := filepath.Join(workspaceRoot, filepath.FromSlash(normalizeRepoRoot(repo.Root)))
		repoRootAbs, err := filepath.Abs(repoRoot)
		if err != nil {
			return "", false
		}
		modulePath := filepath.Join(workspaceRootAbs, filepath.FromSlash(workspacePath))
		localPath, err := filepath.Rel(repoRootAbs, modulePath)
		if err != nil || localPath == ".." || strings.HasPrefix(localPath, ".."+string(filepath.Separator)) || filepath.IsAbs(localPath) {
			return "", false
		}
		localPath = filepath.ToSlash(filepath.Clean(localPath))
		if _, safe := graphSafeRelativeModule(repoRoot, localPath); !safe {
			return "", false
		}
		return localPath, true
	}
	if strings.Contains(configured, "::") {
		return "", false
	}
	return configured, true
}

func graphRootGoModule(root string) string {
	return graphGoModule(root, "")
}

func graphGoModule(repoRoot, configured string) string {
	repoRoot = filepath.Clean(strings.TrimSpace(repoRoot))
	rawConfigured := strings.TrimSpace(configured)
	if rawConfigured != "" {
		normalized, safe := graphSafeRelativeModule(repoRoot, rawConfigured)
		if !safe || filepath.Base(normalized) != "go.mod" {
			return ""
		}
		path := filepath.Join(repoRoot, filepath.FromSlash(normalized))
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
			return normalized
		}
		return ""
	}
	path := filepath.Join(repoRoot, "go.mod")
	if _, safe := graphSafeRelativeModule(repoRoot, "go.mod"); !safe {
		return ""
	}
	if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
		return "go.mod"
	}
	return ""
}

func graphSafeRelativeModule(repoRoot, configured string) (string, bool) {
	configured = strings.TrimSpace(configured)
	driveRelative := len(configured) >= 2 &&
		((configured[0] >= 'A' && configured[0] <= 'Z') || (configured[0] >= 'a' && configured[0] <= 'z')) &&
		configured[1] == ':' &&
		(len(configured) == 2 || (configured[2] != '/' && configured[2] != '\\'))
	if configured == "" || strings.ContainsRune(configured, 0) || strings.Contains(configured, "\\") || filepath.IsAbs(configured) || driveRelative {
		return "", false
	}
	for _, part := range strings.Split(configured, "/") {
		if part == ".." {
			return "", false
		}
	}
	clean := filepath.Clean(configured)
	if filepath.Base(clean) != "go.mod" {
		return "", false
	}
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", false
	}
	path := filepath.Join(root, clean)
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", false
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", false
	}
	pathReal, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	relativeReal, err := filepath.Rel(rootReal, pathReal)
	if err != nil || relativeReal == ".." || strings.HasPrefix(relativeReal, ".."+string(filepath.Separator)) || filepath.IsAbs(relativeReal) {
		return "", false
	}
	return filepath.ToSlash(clean), true
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

func graphObservationBatchDiagnostics(batch model.GraphObservationBatch) string {
	reasons := make(map[string]struct{}, len(batch.Omissions)+len(batch.Unresolved))
	for _, omission := range batch.Omissions {
		if omission.ReasonCode != "" {
			reasons[omission.ReasonCode] = struct{}{}
		}
	}
	for _, unresolved := range batch.Unresolved {
		if unresolved.ReasonCode != "" {
			reasons[unresolved.ReasonCode] = struct{}{}
		}
	}
	codes := make([]string, 0, len(reasons))
	for code := range reasons {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	declarations := "missing"
	for _, coverage := range batch.Coverage {
		if coverage.Capability == "declarations" {
			declarations = fmt.Sprintf("eligible=%d observed=%d omitted=%d unresolved=%d", coverage.Eligible, coverage.Observed, coverage.Omitted, coverage.Unresolved)
			break
		}
	}
	return fmt.Sprintf("completeness=%q nodes=%d unresolved=%d omissions=%d reasons=%s declarations=%s", batch.Completeness, len(batch.Nodes), len(batch.Unresolved), len(batch.Omissions), strings.Join(codes, ","), declarations)
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
