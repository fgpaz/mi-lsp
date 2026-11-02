package model

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode"
)

const (
	GraphTopologyWitnessVersion = 1
	GraphTopologyMaxRepos       = 256
	GraphTopologyMaxSelectors   = 1024
	GraphTopologyMaxModules     = 256
	GraphTopologyMaxJSONBytes   = 64 * 1024
)

var ErrGraphTopologyWitnessInvalid = errors.New("GPH_QUERY_TOPOLOGY_INCOMPLETE")

// GraphTopologyWitness is the minimal, generation-bound projection of the
// normalized workspace topology consumed by native Go graph observation.
// It deliberately excludes unrelated ProjectFile settings.
type GraphTopologyWitness struct {
	SchemaVersion         int                     `json:"schema_version"`
	WorkspaceIdentity     string                  `json:"workspace_identity"`
	RepositoryIdentity    string                  `json:"repository_identity"`
	Project               GraphTopologyProject    `json:"project"`
	Modules               []GraphTopologyGoModule `json:"go_modules"`
	ProjectDigest         GraphDigest             `json:"project_digest"`
	BaseConfigFingerprint GraphDigest             `json:"base_config_fingerprint"`
	Digest                GraphDigest             `json:"topology_digest"`
}

type GraphTopologyProject struct {
	Kind              string                    `json:"kind"`
	Languages         []string                  `json:"languages"`
	DefaultRepo       string                    `json:"default_repo,omitempty"`
	DefaultEntrypoint string                    `json:"default_entrypoint,omitempty"`
	Repos             []GraphTopologyRepo       `json:"repos"`
	Entrypoints       []GraphTopologyEntrypoint `json:"entrypoints"`
}

type GraphTopologyRepo struct {
	ID                 string   `json:"id"`
	RepositoryIdentity string   `json:"repository_identity,omitempty"`
	Root               string   `json:"workspace_prefix"`
	Languages          []string `json:"languages"`
	DefaultEntrypoint  string   `json:"default_entrypoint,omitempty"`
}

type GraphTopologyEntrypoint struct {
	ID      string `json:"id"`
	RepoID  string `json:"repo_id"`
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Default bool   `json:"default,omitempty"`
}

type GraphTopologyGoModule struct {
	RepositoryIDs          []string    `json:"repository_ids"`
	SelectorID             string      `json:"selector_id"`
	SelectorKind           string      `json:"selector_kind"`
	SelectorPath           string      `json:"selector_path"`
	RequestRoot            string      `json:"request_root"`
	ProjectOrModule        string      `json:"project_or_module"`
	GoModPath              string      `json:"go_mod_path"`
	ModulePath             string      `json:"module_path"`
	GoModDigest            GraphDigest `json:"go_mod_digest"`
	BatchConfigFingerprint GraphDigest `json:"batch_config_fingerprint"`
}

func topologyPath(value string, allowDot bool) bool {
	if value == "" || strings.ContainsAny(value, "\\\x00") || strings.HasPrefix(value, "/") || (len(value) > 1 && value[1] == ':') {
		return false
	}
	clean := path.Clean(value)
	return clean == value && clean != ".." && !strings.HasPrefix(clean, "../") && (allowDot || clean != ".")
}

func boundedTopologyText(value string, limit int, allowEmpty bool) bool {
	if len(value) > limit || (!allowEmpty && strings.TrimSpace(value) == "") || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func normalizedTopologyProject(project ProjectFile) GraphTopologyProject {
	p := GraphTopologyProject{
		Kind:        project.Project.Kind,
		Languages:   append([]string(nil), project.Project.Languages...),
		DefaultRepo: project.Project.DefaultRepo, DefaultEntrypoint: project.Project.DefaultEntrypoint,
		Repos:       make([]GraphTopologyRepo, 0, len(project.Repos)),
		Entrypoints: make([]GraphTopologyEntrypoint, 0, len(project.Entrypoints)),
	}
	for _, repo := range project.Repos {
		root := strings.ReplaceAll(strings.TrimSpace(repo.Root), "\\", "/")
		if root == "" {
			root = "."
		}
		p.Repos = append(p.Repos, GraphTopologyRepo{
			ID: repo.ID, RepositoryIdentity: repo.RepositoryIdentity, Root: path.Clean(root),
			Languages: append([]string(nil), repo.Languages...), DefaultEntrypoint: repo.DefaultEntrypoint,
		})
	}
	for _, entry := range project.Entrypoints {
		entryPath := strings.ReplaceAll(strings.TrimSpace(entry.Path), "\\", "/")
		p.Entrypoints = append(p.Entrypoints, GraphTopologyEntrypoint{
			ID: entry.ID, RepoID: entry.RepoID, Path: path.Clean(entryPath), Kind: entry.Kind, Default: entry.Default,
		})
	}
	p.Languages = lowerUniqueTopologyStrings(p.Languages)
	for i := range p.Repos {
		p.Repos[i].Languages = lowerUniqueTopologyStrings(p.Repos[i].Languages)
	}
	sort.Slice(p.Repos, func(i, j int) bool { return p.Repos[i].ID < p.Repos[j].ID })
	sort.Slice(p.Entrypoints, func(i, j int) bool {
		if p.Entrypoints[i].ID != p.Entrypoints[j].ID {
			return p.Entrypoints[i].ID < p.Entrypoints[j].ID
		}
		return p.Entrypoints[i].Path < p.Entrypoints[j].Path
	})
	return p
}

func uniqueTopologyStrings(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

func lowerUniqueTopologyStrings(values []string) []string {
	for i := range values {
		values[i] = strings.ToLower(strings.TrimSpace(values[i]))
	}
	sort.Strings(values)
	return uniqueTopologyStrings(values)
}

func topologyProjectBytes(project GraphTopologyProject) ([]byte, error) {
	return json.Marshal(project)
}

// GraphTopologyProjectFingerprint binds the normalized selection/config input
// independently of the filesystem or a later ProjectFile load.
func GraphTopologyProjectFingerprint(project ProjectFile) (GraphDigest, error) {
	canonical := normalizedTopologyProject(project)
	if err := validateTopologyProject(canonical); err != nil {
		return GraphDigest{}, err
	}
	encoded, err := topologyProjectBytes(canonical)
	if err != nil {
		return GraphDigest{}, err
	}
	if len(encoded) > GraphTopologyMaxJSONBytes {
		return GraphDigest{}, ErrGraphTopologyWitnessInvalid
	}
	return topologyHash("MILSP-GO-TOPOLOGY-PROJECT/v1\x00", encoded), nil
}

func NewGraphTopologyWitness(project ProjectFile, identity string) (*GraphTopologyWitness, error) {
	canonical := normalizedTopologyProject(project)
	if err := validateTopologyProject(canonical); err != nil {
		return nil, err
	}
	normalizedIdentity, err := NormalizeRepositoryIdentity(identity)
	if err != nil || strings.TrimSpace(identity) == "" {
		return nil, ErrGraphTopologyWitnessInvalid
	}
	w := &GraphTopologyWitness{
		SchemaVersion: GraphTopologyWitnessVersion, WorkspaceIdentity: normalizedIdentity,
		RepositoryIdentity: normalizedIdentity, Project: canonical, Modules: []GraphTopologyGoModule{},
	}
	projectBytes, err := topologyProjectBytes(canonical)
	if err != nil {
		return nil, err
	}
	w.ProjectDigest = topologyHash("MILSP-GO-TOPOLOGY-PROJECT/v1\x00", projectBytes)
	return w, nil
}

func validateTopologyProject(p GraphTopologyProject) error {
	if !boundedTopologyText(p.Kind, 64, false) || !boundedTopologyText(p.DefaultRepo, 256, true) || !boundedTopologyText(p.DefaultEntrypoint, 512, true) || len(p.Repos) == 0 || len(p.Repos) > GraphTopologyMaxRepos || len(p.Entrypoints) > GraphTopologyMaxSelectors || len(p.Languages) > 64 {
		return ErrGraphTopologyWitnessInvalid
	}
	seenRepos := make(map[string]bool, len(p.Repos))
	for i, repo := range p.Repos {
		if !boundedTopologyText(repo.ID, 256, false) || !topologyPath(repo.Root, true) || len(repo.Languages) > 64 || (i > 0 && p.Repos[i-1].ID >= repo.ID) {
			return ErrGraphTopologyWitnessInvalid
		}
		if repo.RepositoryIdentity != "" {
			if normalized, err := NormalizeRepositoryIdentity(repo.RepositoryIdentity); err != nil || normalized != repo.RepositoryIdentity {
				return ErrGraphTopologyWitnessInvalid
			}
		}
		for j, language := range repo.Languages {
			if !boundedTopologyText(language, 64, false) || language != strings.ToLower(language) || (j > 0 && repo.Languages[j-1] >= language) {
				return ErrGraphTopologyWitnessInvalid
			}
		}
		seenRepos[repo.ID] = true
	}
	for i, language := range p.Languages {
		if !boundedTopologyText(language, 64, false) || language != strings.ToLower(language) || (i > 0 && p.Languages[i-1] >= language) {
			return ErrGraphTopologyWitnessInvalid
		}
	}
	seenEntrypoints := make(map[string]bool, len(p.Entrypoints))
	for i, entry := range p.Entrypoints {
		if !boundedTopologyText(entry.ID, 512, false) || !boundedTopologyText(entry.RepoID, 256, false) || !topologyPath(entry.Path, false) || !boundedTopologyText(entry.Kind, 64, false) || !seenRepos[entry.RepoID] {
			return ErrGraphTopologyWitnessInvalid
		}
		if i > 0 && (p.Entrypoints[i-1].ID > entry.ID || (p.Entrypoints[i-1].ID == entry.ID && p.Entrypoints[i-1].Path >= entry.Path)) {
			return ErrGraphTopologyWitnessInvalid
		}
		key := entry.ID + "\x00" + entry.RepoID + "\x00" + entry.Path + "\x00" + entry.Kind
		if seenEntrypoints[key] {
			return ErrGraphTopologyWitnessInvalid
		}
		seenEntrypoints[key] = true
	}
	return nil
}

func topologyHash(domain string, data []byte) GraphDigest {
	h := sha256.New()
	_, _ = h.Write([]byte(domain))
	_, _ = h.Write(data)
	var digest GraphDigest
	copy(digest[:], h.Sum(nil))
	return digest
}

func (w *GraphTopologyWitness) Seal() error {
	if w == nil || w.SchemaVersion != GraphTopologyWitnessVersion {
		return ErrGraphTopologyWitnessInvalid
	}
	w.Project.Languages = lowerUniqueTopologyStrings(append([]string(nil), w.Project.Languages...))
	w.Project.Repos = append([]GraphTopologyRepo(nil), w.Project.Repos...)
	for i := range w.Project.Repos {
		w.Project.Repos[i].Languages = lowerUniqueTopologyStrings(append([]string(nil), w.Project.Repos[i].Languages...))
	}
	sort.Slice(w.Project.Repos, func(i, j int) bool { return w.Project.Repos[i].ID < w.Project.Repos[j].ID })
	w.Project.Entrypoints = append([]GraphTopologyEntrypoint(nil), w.Project.Entrypoints...)
	sort.Slice(w.Project.Entrypoints, func(i, j int) bool {
		if w.Project.Entrypoints[i].ID != w.Project.Entrypoints[j].ID {
			return w.Project.Entrypoints[i].ID < w.Project.Entrypoints[j].ID
		}
		return w.Project.Entrypoints[i].Path < w.Project.Entrypoints[j].Path
	})
	if err := validateTopologyProject(w.Project); err != nil {
		return err
	}
	identity, err := NormalizeRepositoryIdentity(w.RepositoryIdentity)
	if err != nil || identity != w.RepositoryIdentity || w.WorkspaceIdentity != identity {
		return ErrGraphTopologyWitnessInvalid
	}
	if len(w.Modules) == 0 || len(w.Modules) > GraphTopologyMaxModules {
		return ErrGraphTopologyWitnessInvalid
	}
	w.Modules = append([]GraphTopologyGoModule(nil), w.Modules...)
	for i := range w.Modules {
		w.Modules[i].RepositoryIDs = append([]string(nil), w.Modules[i].RepositoryIDs...)
		sort.Strings(w.Modules[i].RepositoryIDs)
		w.Modules[i].RepositoryIDs = uniqueTopologyStrings(w.Modules[i].RepositoryIDs)
	}
	sort.Slice(w.Modules, func(i, j int) bool {
		if w.Modules[i].RequestRoot != w.Modules[j].RequestRoot {
			return w.Modules[i].RequestRoot < w.Modules[j].RequestRoot
		}
		return w.Modules[i].ProjectOrModule < w.Modules[j].ProjectOrModule
	})
	w.ProjectDigest, err = projectDigest(w.Project)
	if err != nil {
		return err
	}
	if w.BaseConfigFingerprint == (GraphDigest{}) {
		return ErrGraphTopologyWitnessInvalid
	}
	if err := w.validateModules(); err != nil {
		return err
	}
	payload, err := w.payloadBytes()
	if err != nil || len(payload) > GraphTopologyMaxJSONBytes {
		return ErrGraphTopologyWitnessInvalid
	}
	w.Digest = topologyHash("MILSP-GO-TOPOLOGY-WITNESS/v1\x00", payload)
	return w.Validate()
}

func projectDigest(project GraphTopologyProject) (GraphDigest, error) {
	encoded, err := topologyProjectBytes(project)
	if err != nil {
		return GraphDigest{}, err
	}
	return topologyHash("MILSP-GO-TOPOLOGY-PROJECT/v1\x00", encoded), nil
}

func (w GraphTopologyWitness) payloadBytes() ([]byte, error) {
	return json.Marshal(struct {
		SchemaVersion         int                     `json:"schema_version"`
		WorkspaceIdentity     string                  `json:"workspace_identity"`
		RepositoryIdentity    string                  `json:"repository_identity"`
		Project               GraphTopologyProject    `json:"project"`
		Modules               []GraphTopologyGoModule `json:"go_modules"`
		ProjectDigest         GraphDigest             `json:"project_digest"`
		BaseConfigFingerprint GraphDigest             `json:"base_config_fingerprint"`
	}{w.SchemaVersion, w.WorkspaceIdentity, w.RepositoryIdentity, w.Project, w.Modules, w.ProjectDigest, w.BaseConfigFingerprint})
}

func (w GraphTopologyWitness) CanonicalJSON() ([]byte, error) {
	if err := w.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(w)
	if err != nil || len(encoded) > GraphTopologyMaxJSONBytes {
		return nil, ErrGraphTopologyWitnessInvalid
	}
	return encoded, nil
}

func (w GraphTopologyWitness) Validate() error {
	identity, identityErr := NormalizeRepositoryIdentity(w.RepositoryIdentity)
	if w.SchemaVersion != GraphTopologyWitnessVersion || w.Digest == (GraphDigest{}) || w.ProjectDigest == (GraphDigest{}) || w.BaseConfigFingerprint == (GraphDigest{}) || identityErr != nil || identity != w.RepositoryIdentity || w.WorkspaceIdentity != identity {
		return ErrGraphTopologyWitnessInvalid
	}
	if err := validateTopologyProject(w.Project); err != nil {
		return err
	}
	project, err := projectDigest(w.Project)
	if err != nil || project != w.ProjectDigest {
		return ErrGraphTopologyWitnessInvalid
	}
	if len(w.Modules) == 0 || len(w.Modules) > GraphTopologyMaxModules {
		return ErrGraphTopologyWitnessInvalid
	}
	for i, module := range w.Modules {
		if i > 0 && (w.Modules[i-1].RequestRoot > module.RequestRoot || (w.Modules[i-1].RequestRoot == module.RequestRoot && w.Modules[i-1].ProjectOrModule >= module.ProjectOrModule)) {
			return ErrGraphTopologyWitnessInvalid
		}
	}
	if err := w.validateModules(); err != nil {
		return err
	}
	payload, err := w.payloadBytes()
	encoded, marshalErr := json.Marshal(w)
	if err != nil || marshalErr != nil || len(payload) > GraphTopologyMaxJSONBytes || len(encoded) > GraphTopologyMaxJSONBytes || topologyHash("MILSP-GO-TOPOLOGY-WITNESS/v1\x00", payload) != w.Digest {
		return ErrGraphTopologyWitnessInvalid
	}
	return nil
}

func (w GraphTopologyWitness) validateModules() error {
	if len(w.Modules) == 0 || len(w.Modules) > GraphTopologyMaxModules {
		return ErrGraphTopologyWitnessInvalid
	}
	repos := make(map[string]GraphTopologyRepo, len(w.Project.Repos))
	for _, repo := range w.Project.Repos {
		repos[repo.ID] = repo
	}
	seen := map[string]bool{}
	for _, module := range w.Modules {
		if !boundedTopologyText(module.SelectorID, 512, false) || (module.SelectorKind != "explicit" && module.SelectorKind != "configured" && module.SelectorKind != "implicit_repo" && module.SelectorKind != "container_root") || !topologyPath(module.SelectorPath, false) || module.SelectorPath != module.GoModPath || !topologyPath(module.RequestRoot, true) || !topologyPath(module.ProjectOrModule, false) || path.Base(module.ProjectOrModule) != "go.mod" || !topologyPath(module.GoModPath, false) || module.GoModPath != path.Join(module.RequestRoot, module.ProjectOrModule) || !boundedTopologyText(module.ModulePath, 512, false) || module.GoModDigest == (GraphDigest{}) || module.BatchConfigFingerprint == (GraphDigest{}) || len(module.RepositoryIDs) == 0 || len(module.RepositoryIDs) > GraphTopologyMaxRepos {
			return ErrGraphTopologyWitnessInvalid
		}
		key := module.RequestRoot + "\x00" + module.ProjectOrModule
		if seen[key] {
			return ErrGraphTopologyWitnessInvalid
		}
		seen[key] = true
		for i, repoID := range module.RepositoryIDs {
			repo, ok := repos[repoID]
			if !ok || !topologyRepoHasGo(repo) || (i > 0 && module.RepositoryIDs[i-1] >= repoID) {
				return ErrGraphTopologyWitnessInvalid
			}
		}
	}
	return nil
}

func topologyRepoHasGo(repo GraphTopologyRepo) bool {
	for _, language := range repo.Languages {
		if language == "go" {
			return true
		}
	}
	return false
}

func (w GraphTopologyWitness) ProjectFile() ProjectFile {
	p := ProjectFile{Project: ProjectBlock{
		Kind: w.Project.Kind, Languages: append([]string(nil), w.Project.Languages...),
		DefaultRepo: w.Project.DefaultRepo, DefaultEntrypoint: w.Project.DefaultEntrypoint,
	}}
	for _, repo := range w.Project.Repos {
		p.Repos = append(p.Repos, WorkspaceRepo{ID: repo.ID, Root: repo.Root, RepositoryIdentity: repo.RepositoryIdentity, Languages: append([]string(nil), repo.Languages...), DefaultEntrypoint: repo.DefaultEntrypoint})
	}
	for _, entry := range w.Project.Entrypoints {
		p.Entrypoints = append(p.Entrypoints, WorkspaceEntrypoint{ID: entry.ID, RepoID: entry.RepoID, Path: entry.Path, Kind: entry.Kind, Default: entry.Default})
	}
	return p
}

func (w GraphTopologyWitness) ValidateConfigFingerprint(config GraphDigest) error {
	if err := w.Validate(); err != nil {
		return err
	}
	if bindGraphTopologyConfig(w.BaseConfigFingerprint, w.Digest) != config {
		return fmt.Errorf("%w: topology is not bound to generation config fingerprint", ErrGraphTopologyWitnessInvalid)
	}
	return nil
}

func BindGraphTopologyConfig(base, topology GraphDigest) GraphDigest {
	return bindGraphTopologyConfig(base, topology)
}

func bindGraphTopologyConfig(base, topology GraphDigest) GraphDigest {
	var input [64]byte
	copy(input[:32], base[:])
	copy(input[32:], topology[:])
	return topologyHash("MILSP-G3-CONFIG-TOPOLOGY/v1\x00", input[:])
}
