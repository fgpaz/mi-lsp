package workspace

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

type repoDetection struct {
	repo        model.WorkspaceRepo
	entrypoints []model.WorkspaceEntrypoint
	hasMarkers  bool
}

func DetectWorkspace(path string) (model.WorkspaceRegistration, error) {
	registration, _, err := DetectWorkspaceLayout(path, "")
	return registration, err
}

func DetectWorkspaceLayout(path string, explicitName string) (model.WorkspaceRegistration, model.ProjectFile, error) {
	root, err := normalizeWorkspaceRoot(path)
	if err != nil {
		return model.WorkspaceRegistration{}, model.ProjectFile{}, err
	}

	rootHasGit := hasGitDir(root)
	rootDetection, err := detectRepo(root, root)
	if err != nil {
		return model.WorkspaceRegistration{}, model.ProjectFile{}, err
	}
	childDetections, err := detectChildRepos(root)
	if err != nil {
		return model.WorkspaceRegistration{}, model.ProjectFile{}, err
	}

	kind := model.WorkspaceKindSingle
	detections := []repoDetection{}
	switch {
	case rootHasGit || (rootDetection.hasMarkers && len(childDetections) == 0):
		detections = append(detections, rootDetection)
	case len(childDetections) > 0:
		kind = model.WorkspaceKindContainer
		detections = append(detections, childDetections...)
	case rootDetection.hasMarkers:
		detections = append(detections, rootDetection)
	default:
		return model.WorkspaceRegistration{}, model.ProjectFile{}, errors.New("no supported project markers found")
	}

	project := buildProjectFile(root, explicitName, kind, detections)
	registration := model.WorkspaceRegistration{
		Name:      project.Project.Name,
		Root:      root,
		Languages: append([]string{}, project.Project.Languages...),
		Kind:      project.Project.Kind,
		Solution:  defaultSolutionPath(project),
	}
	return registration, project, nil
}

func ScanCandidates(startDir string) ([]model.WorkspaceRegistration, error) {
	root, err := filepath.Abs(startDir)
	if err != nil {
		return nil, err
	}
	candidates := []string{root}
	parent := filepath.Dir(root)
	if parent != root {
		candidates = append(candidates, parent)
	}

	seen := map[string]struct{}{}
	results := make([]model.WorkspaceRegistration, 0)
	for _, candidateRoot := range candidates {
		entries, err := os.ReadDir(candidateRoot)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || isIgnoredDirName(entry.Name()) {
				continue
			}
			path := filepath.Join(candidateRoot, entry.Name())
			if _, ok := seen[path]; ok {
				continue
			}
			registration, _, err := DetectWorkspaceLayout(path, "")
			if err != nil {
				continue
			}
			seen[path] = struct{}{}
			results = append(results, registration)
		}
	}
	slices.SortFunc(results, func(left, right model.WorkspaceRegistration) int {
		return strings.Compare(strings.ToLower(left.Name), strings.ToLower(right.Name))
	})
	return results, nil
}

func LoadProjectTopology(root string, registration model.WorkspaceRegistration) (model.ProjectFile, error) {
	project, err := LoadProjectFile(root)
	if err != nil {
		return model.ProjectFile{}, err
	}
	if project.Project.Name == "" || len(project.Repos) == 0 {
		_, detected, detectErr := DetectWorkspaceLayout(root, registration.Name)
		if detectErr == nil {
			project = mergeProjectFile(project, detected)
		}
	}
	return normalizeProjectFile(root, registration, project), nil
}

func mergeProjectFile(existing model.ProjectFile, detected model.ProjectFile) model.ProjectFile {
	merged := existing
	if merged.Project.Name == "" {
		merged.Project.Name = detected.Project.Name
	}
	if len(merged.Project.Languages) == 0 {
		merged.Project.Languages = detected.Project.Languages
	}
	if merged.Project.Kind == "" {
		merged.Project.Kind = detected.Project.Kind
	}
	if merged.Project.DefaultRepo == "" {
		merged.Project.DefaultRepo = detected.Project.DefaultRepo
	}
	if merged.Project.DefaultEntrypoint == "" {
		merged.Project.DefaultEntrypoint = detected.Project.DefaultEntrypoint
	}
	if len(merged.Repos) == 0 {
		merged.Repos = detected.Repos
	}
	if len(merged.Entrypoints) == 0 {
		merged.Entrypoints = detected.Entrypoints
	}
	return merged
}

func normalizeProjectFile(root string, registration model.WorkspaceRegistration, project model.ProjectFile) model.ProjectFile {
	name := strings.TrimSpace(project.Project.Name)
	if name == "" {
		name = strings.TrimSpace(registration.Name)
	}
	if name == "" {
		name = filepath.Base(root)
	}
	project.Project.Name = name
	if len(project.Project.Languages) == 0 {
		project.Project.Languages = append([]string{}, registration.Languages...)
	}
	if project.Project.Kind == "" {
		project.Project.Kind = firstNonEmpty(registration.Kind, model.WorkspaceKindSingle)
	}
	if len(project.Repos) == 0 {
		repoID := defaultRepoID(root)
		project.Repos = []model.WorkspaceRepo{{
			ID:                repoID,
			Name:              filepath.Base(root),
			Root:              ".",
			Languages:         append([]string{}, project.Project.Languages...),
			DefaultEntrypoint: repoID,
		}}
	}
	if project.Project.DefaultRepo == "" {
		project.Project.DefaultRepo = project.Repos[0].ID
	}
	if project.Project.DefaultEntrypoint == "" {
		for _, repo := range project.Repos {
			if repo.DefaultEntrypoint != "" {
				project.Project.DefaultEntrypoint = repo.DefaultEntrypoint
				break
			}
		}
	}
	return project
}

func ApplyProjectTopology(registration model.WorkspaceRegistration, project model.ProjectFile) model.WorkspaceRegistration {
	updated := registration
	if strings.TrimSpace(updated.Name) == "" && project.Project.Name != "" {
		updated.Name = project.Project.Name
	}
	if len(project.Project.Languages) > 0 {
		updated.Languages = append([]string{}, project.Project.Languages...)
	}
	if project.Project.Kind != "" {
		updated.Kind = project.Project.Kind
	}
	updated.Solution = defaultSolutionPath(project)
	return updated
}

func FindRepo(project model.ProjectFile, selector string) (model.WorkspaceRepo, bool) {
	needle := strings.TrimSpace(selector)
	if needle == "" {
		for _, repo := range project.Repos {
			if repo.ID == project.Project.DefaultRepo {
				return repo, true
			}
		}
		if len(project.Repos) > 0 {
			return project.Repos[0], true
		}
		return model.WorkspaceRepo{}, false
	}
	for _, repo := range project.Repos {
		if strings.EqualFold(repo.ID, needle) || strings.EqualFold(repo.Name, needle) || strings.EqualFold(repo.Root, filepath.ToSlash(needle)) {
			return repo, true
		}
	}
	return model.WorkspaceRepo{}, false
}

func FindEntrypoint(project model.ProjectFile, selector string) (model.WorkspaceEntrypoint, bool) {
	needle := strings.TrimSpace(selector)
	if needle == "" {
		for _, entrypoint := range project.Entrypoints {
			if entrypoint.ID == project.Project.DefaultEntrypoint {
				return entrypoint, true
			}
		}
		return model.WorkspaceEntrypoint{}, false
	}
	normalizedNeedle := filepath.ToSlash(needle)
	for _, entrypoint := range project.Entrypoints {
		if strings.EqualFold(entrypoint.ID, needle) || strings.EqualFold(entrypoint.Path, normalizedNeedle) {
			return entrypoint, true
		}
	}
	return model.WorkspaceEntrypoint{}, false
}

func FindRepoByFile(project model.ProjectFile, workspaceRoot string, filePath string) (model.WorkspaceRepo, bool) {
	absoluteFile := filePath
	if !filepath.IsAbs(absoluteFile) {
		absoluteFile = filepath.Join(workspaceRoot, filepath.FromSlash(filePath))
	}
	rel, err := filepath.Rel(workspaceRoot, absoluteFile)
	if err != nil {
		return model.WorkspaceRepo{}, false
	}
	normalizedRel := filepath.ToSlash(rel)
	var best model.WorkspaceRepo
	bestLen := -1
	for _, repo := range project.Repos {
		rootPrefix := strings.Trim(strings.TrimSpace(filepath.ToSlash(repo.Root)), "/")
		if rootPrefix == "" || rootPrefix == "." {
			if bestLen < 0 {
				best = repo
				bestLen = 0
			}
			continue
		}
		if normalizedRel == rootPrefix || strings.HasPrefix(normalizedRel, rootPrefix+"/") {
			if len(rootPrefix) > bestLen {
				best = repo
				bestLen = len(rootPrefix)
			}
		}
	}
	if bestLen >= 0 {
		return best, true
	}
	return model.WorkspaceRepo{}, false
}

func EntrypointsForRepo(project model.ProjectFile, repoID string) []model.WorkspaceEntrypoint {
	items := make([]model.WorkspaceEntrypoint, 0)
	for _, entrypoint := range project.Entrypoints {
		if strings.EqualFold(entrypoint.RepoID, repoID) {
			items = append(items, entrypoint)
		}
	}
	return items
}

func DefaultEntrypointForRepo(project model.ProjectFile, repoID string) (model.WorkspaceEntrypoint, bool) {
	for _, entrypoint := range project.Entrypoints {
		if strings.EqualFold(entrypoint.RepoID, repoID) && entrypoint.Default {
			return entrypoint, true
		}
	}
	for _, entrypoint := range project.Entrypoints {
		if strings.EqualFold(entrypoint.RepoID, repoID) {
			return entrypoint, true
		}
	}
	return model.WorkspaceEntrypoint{}, false
}

func detectChildRepos(root string) ([]repoDetection, error) {
	matcher, err := LoadIgnoreMatcher(root, nil)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	results := make([]repoDetection, 0)
	for _, entry := range entries {
		if !entry.IsDir() || isIgnoredDirName(entry.Name()) {
			continue
		}
		candidate := filepath.Join(root, entry.Name())
		if matcher.ShouldIgnore(root, candidate) {
			continue
		}
		detection, err := detectRepo(root, candidate)
		if err != nil {
			return nil, err
		}
		if detection.hasMarkers || hasGitDir(candidate) {
			results = append(results, detection)
		}
	}
	slices.SortFunc(results, func(left, right repoDetection) int {
		return strings.Compare(strings.ToLower(left.repo.Name), strings.ToLower(right.repo.Name))
	})
	return results, nil
}

func detectRepo(workspaceRoot string, repoRoot string) (repoDetection, error) {
	matcher, err := LoadIgnoreMatcher(repoRoot, nil)
	if err != nil {
		return repoDetection{}, err
	}
	relRoot, err := filepath.Rel(workspaceRoot, repoRoot)
	if err != nil {
		return repoDetection{}, err
	}
	relRoot = filepath.ToSlash(relRoot)
	if relRoot == "" {
		relRoot = "."
	}
	repoID := makeStableID(relRoot, defaultRepoID(repoRoot))
	languages := map[string]struct{}{}
	solutions := make([]string, 0)
	projects := make([]string, 0)
	maxDepth := strings.Count(repoRoot, string(os.PathSeparator)) + 4

	err = filepath.WalkDir(repoRoot, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if matcher.ShouldIgnore(repoRoot, current) && current != repoRoot {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if strings.Count(current, string(os.PathSeparator)) > maxDepth {
				return fs.SkipDir
			}
			return nil
		}
		relPath, relErr := filepath.Rel(workspaceRoot, current)
		if relErr != nil {
			return relErr
		}
		relPath = filepath.ToSlash(relPath)
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".sln":
			solutions = append(solutions, relPath)
			languages["csharp"] = struct{}{}
		case ".csproj":
			projects = append(projects, relPath)
			languages["csharp"] = struct{}{}
		case ".ts", ".tsx", ".js", ".jsx":
			languages["typescript"] = struct{}{}
		case ".py", ".pyi":
			languages["python"] = struct{}{}
		}
		switch strings.ToLower(entry.Name()) {
		case "package.json", "tsconfig.json", "next.config.js", "next.config.ts", "vite.config.ts":
			languages["typescript"] = struct{}{}
		case "pyproject.toml", "setup.py", "setup.cfg", "requirements.txt", "poetry.lock", "pipfile", "pipfile.lock":
			languages["python"] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return repoDetection{}, err
	}

	languageList := mapKeys(languages)
	slices.Sort(languageList)
	entrypoints := buildEntrypoints(repoID, relRoot, solutions, projects)
	defaultEntrypoint := defaultEntrypointID(entrypoints)
	return repoDetection{
		repo: model.WorkspaceRepo{
			ID:                repoID,
			Name:              filepath.Base(repoRoot),
			Root:              relRoot,
			Languages:         languageList,
			DefaultEntrypoint: defaultEntrypoint,
		},
		entrypoints: entrypoints,
		hasMarkers:  len(languageList) > 0 || len(entrypoints) > 0,
	}, nil
}

func buildProjectFile(root string, explicitName string, kind string, detections []repoDetection) model.ProjectFile {
	repos := make([]model.WorkspaceRepo, 0, len(detections))
	entrypoints := make([]model.WorkspaceEntrypoint, 0)
	languageSet := map[string]struct{}{}
	defaultRepo := ""
	defaultEntrypoint := ""
	for idx, detection := range detections {
		repos = append(repos, detection.repo)
		entrypoints = append(entrypoints, detection.entrypoints...)
		for _, language := range detection.repo.Languages {
			languageSet[language] = struct{}{}
		}
		if idx == 0 {
			defaultRepo = detection.repo.ID
			defaultEntrypoint = detection.repo.DefaultEntrypoint
		}
	}
	languages := mapKeys(languageSet)
	slices.Sort(languages)
	name := strings.TrimSpace(explicitName)
	if name == "" {
		name = filepath.Base(root)
	}
	return model.ProjectFile{
		Project: model.ProjectBlock{
			Name:              name,
			Languages:         languages,
			Kind:              firstNonEmpty(kind, model.WorkspaceKindSingle),
			DefaultRepo:       defaultRepo,
			DefaultEntrypoint: defaultEntrypoint,
		},
		Repos:       repos,
		Entrypoints: entrypoints,
	}
}

func buildEntrypoints(repoID string, repoRoot string, solutions []string, projects []string) []model.WorkspaceEntrypoint {
	items := make([]model.WorkspaceEntrypoint, 0, len(solutions)+len(projects))
	for _, path := range solutions {
		items = append(items, model.WorkspaceEntrypoint{ID: makeEntrypointID(repoID, path), RepoID: repoID, Path: path, Kind: model.EntrypointKindSolution})
	}
	for _, path := range projects {
		items = append(items, model.WorkspaceEntrypoint{ID: makeEntrypointID(repoID, path), RepoID: repoID, Path: path, Kind: model.EntrypointKindProject})
	}
	defaultID := chooseDefaultEntrypoint(repoRoot, items)
	for idx := range items {
		items[idx].Default = items[idx].ID == defaultID
	}
	slices.SortFunc(items, func(left, right model.WorkspaceEntrypoint) int {
		if left.Default != right.Default {
			if left.Default {
				return -1
			}
			return 1
		}
		if left.Kind != right.Kind {
			if left.Kind == model.EntrypointKindSolution {
				return -1
			}
			if right.Kind == model.EntrypointKindSolution {
				return 1
			}
		}
		return strings.Compare(strings.ToLower(left.Path), strings.ToLower(right.Path))
	})
	return items
}

func chooseDefaultEntrypoint(repoRoot string, items []model.WorkspaceEntrypoint) string {
	if len(items) == 0 {
		return ""
	}
	normalizedRoot := strings.Trim(strings.TrimSpace(filepath.ToSlash(repoRoot)), "/")
	for _, item := range items {
		if item.Kind != model.EntrypointKindSolution || isAuxiliaryEntrypointPath(item.Path) {
			continue
		}
		candidateDir := strings.Trim(filepath.ToSlash(filepath.Dir(item.Path)), "/")
		if candidateDir == normalizedRoot {
			return item.ID
		}
	}
	for _, item := range items {
		if item.Kind == model.EntrypointKindSolution && !isAuxiliaryEntrypointPath(item.Path) {
			return item.ID
		}
	}
	for _, item := range items {
		if item.Kind == model.EntrypointKindProject && !isAuxiliaryEntrypointPath(item.Path) {
			return item.ID
		}
	}
	if len(items) == 1 {
		return items[0].ID
	}
	for _, item := range items {
		if item.Kind == model.EntrypointKindSolution {
			return item.ID
		}
	}
	return items[0].ID
}

func isAuxiliaryEntrypointPath(path string) bool {
	normalized := "/" + strings.Trim(strings.ToLower(filepath.ToSlash(path)), "/") + "/"
	auxiliaryMarkers := []string{
		"/.docs/",
		"/docs/",
		"/template/",
		"/templates/",
	}
	for _, marker := range auxiliaryMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func defaultEntrypointID(items []model.WorkspaceEntrypoint) string {
	for _, item := range items {
		if item.Default {
			return item.ID
		}
	}
	if len(items) > 0 {
		return items[0].ID
	}
	return ""
}

func defaultSolutionPath(project model.ProjectFile) string {
	for _, entrypoint := range project.Entrypoints {
		if entrypoint.Default && entrypoint.Kind == model.EntrypointKindSolution {
			return entrypoint.Path
		}
	}
	for _, entrypoint := range project.Entrypoints {
		if entrypoint.Kind == model.EntrypointKindSolution {
			return entrypoint.Path
		}
	}
	return ""
}

func normalizeWorkspaceRoot(path string) (string, error) {
	root, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		root = filepath.Dir(root)
	}
	return root, nil
}

func hasGitDir(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}

func isIgnoredDirName(name string) bool {
	for _, pattern := range DefaultIgnorePatterns() {
		if strings.TrimSuffix(pattern, "/") == name {
			return true
		}
	}
	return false
}

func mapKeys(items map[string]struct{}) []string {
	keys := make([]string, 0, len(items))
	for item := range items {
		keys = append(keys, item)
	}
	return keys
}

func makeStableID(value string, fallback string) string {
	normalized := strings.ToLower(strings.Trim(filepath.ToSlash(value), "/"))
	if normalized == "" || normalized == "." {
		return fallback
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", ".", "-", "_", "-")
	normalized = replacer.Replace(normalized)
	normalized = strings.Trim(normalized, "-")
	if normalized == "" {
		return fallback
	}
	return normalized
}

func defaultRepoID(root string) string {
	return makeStableID(filepath.Base(root), "root")
}

func makeEntrypointID(repoID string, path string) string {
	return repoID + "::" + makeStableID(path, filepath.Base(path))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
