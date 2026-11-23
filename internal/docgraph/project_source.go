package docgraph

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
	"gopkg.in/yaml.v3"
)

// ProjectSourceEnv names the private invocation file prepared by a trusted
// launcher. It is process-local, not a registry, an RPC input or a grant.
const ProjectSourceEnv = "MI_LSP_SETUP_VIEW_FILE"
const projectSourceSchema = "mi-lsp-project-source/v1"
const projectSourceMaxBytes = 256 * 1024

var errProjectPolicyInvalid = errors.New("invalid repository policy YAML")
var projectSourceReason = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

type projectSourceEnvelope struct {
	Schema     string             `json:"schema"`
	Status     string             `json:"status"`
	ReasonCode string             `json:"reason_code"`
	View       *projectSourceView `json:"view"`
}

type projectSourceView struct {
	RepoRoot         string          `json:"repo_root"`
	WikiRoot         string          `json:"wiki_root"`
	GovernanceFile   string          `json:"governance_file"`
	WorkspaceRoot    string          `json:"workspace_root"`
	ProjectionOutput string          `json:"projection_output"`
	WorkspaceAlias   string          `json:"workspace_alias"`
	RepoPolicy       json.RawMessage `json:"repo_policy"`
}

func resolveProjectSource(root, file string) SetupWikiSource {
	source := SetupWikiSource{
		Configured: true, External: true, Source: "project_source", State: "pending",
		ReasonCode: "project_source_invalid",
	}
	envelope, ok := loadProjectSourceEnvelope(file)
	if !ok || envelope.Schema != projectSourceSchema {
		return source
	}
	if envelope.Status != "resolved" || envelope.View == nil {
		if envelope.View == nil && (envelope.Status == "pending" || envelope.Status == "invalid") && projectSourceReason.MatchString(envelope.ReasonCode) {
			source.ReasonCode = envelope.ReasonCode
		}
		return source
	}
	view := envelope.View
	if !filepath.IsAbs(view.WorkspaceRoot) || !filepath.IsAbs(view.RepoRoot) {
		return source
	}
	rel, err := filepath.Rel(root, view.WorkspaceRoot)
	if err != nil || rel != "." || !safeDirectoryRoot(view.WorkspaceRoot) || !safeDirectoryRoot(view.RepoRoot) ||
		!pathHasDirPrefix(view.RepoRoot, view.WorkspaceRoot) || !safeGitRoot(view.RepoRoot, view.WorkspaceRoot) {
		source.ReasonCode = "project_source_workspace_mismatch"
		return source
	}
	if !filepath.IsAbs(view.WikiRoot) || !safeDirectoryRoot(view.WikiRoot) || pathHasDirPrefix(view.RepoRoot, view.WikiRoot) || pathHasDirPrefix(view.WikiRoot, view.RepoRoot) {
		source.ReasonCode = "project_source_wiki_unavailable"
		return source
	}
	governanceFile, err := projectSourceFile(view.WikiRoot, view.GovernanceFile)
	if err != nil {
		source.ReasonCode = "project_source_governance_unavailable"
		return source
	}
	output := filepath.ToSlash(filepath.Clean(filepath.FromSlash(view.ProjectionOutput)))
	if !safeSetupRelativePath(view.ProjectionOutput) || !strings.EqualFold(filepath.Ext(output), ".toml") {
		source.ReasonCode = "project_source_projection_invalid"
		return source
	}
	_, projectionFile, err := relativeFromWorkspace(root, output)
	if err != nil || pathInsideAnyCanon(projectionFile, tryResolvedCanons(root)) ||
		!safeProjectOutputParents(root, projectionFile) {
		source.ReasonCode = "project_source_projection_invalid"
		return source
	}
	var policy map[string]any
	if len(view.RepoPolicy) != 0 && yaml.Unmarshal(view.RepoPolicy, &policy) != nil {
		source.ReasonCode = "project_source_policy_invalid"
		return source
	}
	source.State, source.ReasonCode = "resolved", ""
	source.Root, source.GovernanceFile, source.ProjectionFile, source.RepoPolicy = filepath.Clean(view.WikiRoot), governanceFile, projectionFile, policy
	return source
}

func loadProjectSourceEnvelope(file string) (projectSourceEnvelope, bool) {
	var envelope projectSourceEnvelope
	file, err := projectSourceFile(filepath.Dir(file), file)
	if err != nil {
		return envelope, false
	}
	f, err := os.Open(file)
	if err != nil {
		return envelope, false
	}
	defer f.Close()
	payload, err := io.ReadAll(io.LimitReader(f, projectSourceMaxBytes+1))
	if err != nil || len(payload) > projectSourceMaxBytes {
		return envelope, false
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&envelope) != nil || decoder.Decode(new(any)) != io.EOF {
		return projectSourceEnvelope{}, false
	}
	return envelope, true
}

// A nested Git checkout is a different repository, even inside the declared root.
func safeGitRoot(repo, workspaceRoot string) bool {
	for dir := filepath.Clean(workspaceRoot); ; dir = filepath.Dir(dir) {
		info, err := os.Lstat(filepath.Join(dir, ".git"))
		rel, _ := filepath.Rel(dir, repo)
		if rel == "." {
			return err == nil && (info.IsDir() || info.Mode().IsRegular())
		}
		if !os.IsNotExist(err) || filepath.Dir(dir) == dir {
			return false
		}
	}
}

func projectSourceFile(root, file string) (string, error) {
	if !filepath.IsAbs(file) || !safeDirectoryRoot(root) {
		return "", errSafeFileInvalid
	}
	rel, err := filepath.Rel(root, file)
	if err != nil {
		return "", errSafeFileInvalid
	}
	return safeRegularFileWithin(root, rel)
}

func readRepoPolicy(root, declared string) (map[string]any, error) {
	if hasProjectSource() {
		source := ResolveSetupWikiSource(root)
		if source.External {
			if source.State != "resolved" {
				return nil, errSafeFileInvalid
			}
			if source.RepoPolicy == nil {
				return nil, errSafeFileMissing
			}
			return source.RepoPolicy, nil
		}
	}
	path, err := safeRegularFileWithin(root, filepath.FromSlash(declared))
	if err != nil {
		return nil, err
	}
	policy, _, err := readSetupConfig(path)
	if err != nil {
		return nil, errProjectPolicyInvalid
	}
	return policy, nil
}

func safeProjectOutputParents(root, target string) bool {
	for path := target; path != root; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 ||
			(path == target && !info.Mode().IsRegular()) || (path != target && !info.IsDir()) {
			return false
		}
	}
	return true
}

// Only an explicit index may create a missing projection. Existing projections
// must never be replaced and remain subject to normal governance validation.
func compareProjectProjection(root string, source SetupWikiSource, status model.GovernanceStatus, expected []byte, initialize bool) model.GovernanceStatus {
	status.ProjectionDoc = WorkspacePathForSource(root, source.ProjectionFile)
	current, err := os.ReadFile(source.ProjectionFile)
	if errors.Is(err, fs.ErrNotExist) && initialize && !status.Blocked && !status.AECanon.Blocking {
		err = initializeProjectProjection(root, status.ProjectionDoc, expected)
		if err == nil {
			current = expected
		}
	}
	status.IndexSync, status.IndexSyncDetails = inspectIndexSync(root)
	switch {
	case err != nil:
		status.Sync = "missing"
		status.ReasonCode = "project_source_projection_unavailable"
	case normalizedText(current) != normalizedText(expected):
		status.Sync = "stale"
		status.ReasonCode = "project_source_projection_stale"
	default:
		status.Sync = "in_sync"
	}
	if status.ReasonCode == "" && status.AECanon.Blocking {
		status.ReasonCode = status.AECanon.Reason
	}
	if status.ReasonCode == "" && status.IndexSync == "stale" {
		status.ReasonCode = "workspace_index_stale"
	}
	if status.ReasonCode != "" {
		status.Issues = []string{status.ReasonCode}
		status.Blocked = true
	}
	status.Summary = "Project governance: projection " + status.Sync + "; index " + status.IndexSync + "."
	return status
}

func initializeProjectProjection(root, relative string, content []byte) error {
	if linked, err := workspace.IsLinkedWorktree(root); err != nil || linked {
		return errSafeFileInvalid
	}
	confined, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer confined.Close()
	if err := confined.MkdirAll(filepath.Dir(relative), 0o755); err != nil {
		return err
	}
	f, err := confined.OpenFile(relative, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(content)
	if err := errors.Join(writeErr, f.Close()); err != nil {
		return errors.Join(err, confined.Remove(relative))
	}
	return nil
}

func hasProjectSource() bool {
	_, exists := os.LookupEnv(ProjectSourceEnv)
	return exists
}
