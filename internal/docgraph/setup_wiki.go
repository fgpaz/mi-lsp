package docgraph

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SetupWikiSource is the local, read-only projection of Setup's shared and
// machine-local project configuration. Root is populated when the configured
// path can be resolved, even if its checkout is currently missing.
type SetupWikiSource struct {
	Configured bool
	State      string
	ReasonCode string
	Source     string
	Root       string
}

// ResolveSetupWikiSource resolves exactly the wiki selected by Setup v1/v2.
// It never discovers sibling directories, contacts Azure, or falls back to
// .docs/wiki when a Setup contract is present.
func ResolveSetupWikiSource(projectRoot string) SetupWikiSource {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return SetupWikiSource{Configured: true, State: "pending", ReasonCode: "invalid_workspace"}
	}
	shared, exists, err := readSetupConfig(filepath.Join(root, ".sandinas", "project.yaml"))
	if !exists {
		return SetupWikiSource{}
	}
	pending := func(source, reason, path string) SetupWikiSource {
		return SetupWikiSource{Configured: true, State: "pending", ReasonCode: reason, Source: source, Root: path}
	}
	if err != nil {
		return pending("", "setup_contract_unreadable", "")
	}
	schema, _ := setupString(shared, "schema")
	if schema != "sandinas-project/v1" && schema != "sandinas-project/v2" {
		return pending("", "setup_schema_unsupported", "")
	}

	local, localExists, localErr := readSetupConfig(filepath.Join(root, ".sandinas", "project.local.yaml"))
	if localErr != nil {
		return pending("", "local_paths_unreadable", "")
	}
	if !localExists {
		local = map[string]any{}
	}

	wiki := setupMap(shared["wiki"])
	source := "azure_wiki"
	if value, present := wiki["setup_source"]; present && value != nil {
		configured, ok := value.(string)
		if !ok || strings.TrimSpace(configured) == "" {
			return pending("", "wiki_source_invalid", "")
		}
		source = strings.TrimSpace(configured)
	}
	if source != "azure_wiki" && source != "repo_folder" {
		return pending(source, "wiki_source_invalid", "")
	}

	localPaths := setupMap(local["paths"])
	workspaceRoot := filepath.Dir(root)
	if configured, ok := setupString(localPaths, "workspace_root"); ok && configured != "" && configured != "." && configured != "./" {
		workspaceRoot = resolveSetupPath(root, configured)
	}
	project := setupMap(shared["project"])
	projectName, ok := setupString(project, "name")
	if !ok || projectName == "" {
		projectName = filepath.Base(root)
	}

	checkout := configuredWikiCheckout(root, localPaths)
	var wikiRoot string
	if source == "repo_folder" {
		if setupPath, present := wiki["setup_path"]; present && setupPath != nil {
			value, ok := setupPath.(string)
			if !ok {
				return pending(source, "wiki_setup_path_invalid", "")
			}
			if strings.TrimSpace(value) != "" {
				if !safeSetupRelativePath(value) {
					return pending(source, "wiki_setup_path_invalid", "")
				}
				wikiRoot = resolveSetupPath(root, value)
			}
		}
	}
	if wikiRoot == "" {
		wikiRoot = checkout
	}
	if wikiRoot == "" {
		if projectName == "." || projectName == ".." || strings.ContainsAny(projectName, `/\\`) {
			return pending(source, "setup_project_name_invalid", "")
		}
		wikiRoot = filepath.Join(workspaceRoot, projectName+".wiki")
	}

	wikiRoot, err = filepath.Abs(wikiRoot)
	if err != nil {
		return pending(source, "wiki_source_path_invalid", "")
	}
	info, statErr := os.Stat(wikiRoot)
	if statErr != nil || !info.IsDir() {
		reason := "wiki_source_unavailable"
		if source == "azure_wiki" {
			reason = "wiki_checkout_unavailable"
		}
		return pending(source, reason, filepath.Clean(wikiRoot))
	}
	if !safeDirectoryRoot(wikiRoot) {
		return pending(source, "wiki_source_unsafe", filepath.Clean(wikiRoot))
	}
	return SetupWikiSource{Configured: true, State: "resolved", Source: source, Root: filepath.Clean(wikiRoot)}
}

func readSetupConfig(path string) (map[string]any, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, true, errors.New("Setup config is not a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, true, err
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	var config map[string]any
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, true, err
	}
	if config == nil {
		config = map[string]any{}
	}
	return config, true, nil
}

func setupMap(value any) map[string]any {
	if result, ok := value.(map[string]any); ok {
		return result
	}
	return map[string]any{}
}

func setupString(values map[string]any, key string) (string, bool) {
	value, ok := values[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(text), true
}

func configuredWikiCheckout(root string, paths map[string]any) string {
	value, ok := paths["wiki_checkout"]
	if !ok {
		return ""
	}
	if values, ok := value.([]any); ok {
		if len(values) == 0 {
			return ""
		}
		value = values[0]
	}
	path, ok := value.(string)
	if !ok || strings.TrimSpace(path) == "" || strings.TrimSpace(path) == "." || strings.TrimSpace(path) == "./" {
		return ""
	}
	return resolveSetupPath(root, path)
}

func resolveSetupPath(root, value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, `\`, "/"))
	path := filepath.FromSlash(value)
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(root, path))
}

func safeSetupRelativePath(value string) bool {
	value = strings.TrimSpace(strings.ReplaceAll(value, `\`, "/"))
	if value == "" || filepath.IsAbs(filepath.FromSlash(value)) || (len(value) >= 2 && value[1] == ':') || strings.Contains(value, "://") {
		return false
	}
	for _, component := range strings.Split(value, "/") {
		if component == ".." {
			return false
		}
	}
	return true
}

// WorkspacePathForSource preserves portable workspace-relative paths when
// possible and returns an absolute path for configured roots on another volume.
func WorkspacePathForSource(workspaceRoot, sourcePath string) string {
	root, rootErr := filepath.Abs(workspaceRoot)
	target, targetErr := filepath.Abs(sourcePath)
	if rootErr != nil || targetErr != nil {
		return filepath.ToSlash(filepath.Clean(sourcePath))
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || filepath.IsAbs(rel) || (len(rel) >= 2 && rel[1] == ':') {
		return filepath.ToSlash(filepath.Clean(target))
	}
	return filepath.ToSlash(filepath.Clean(rel))
}

// ResolveDocumentPath maps an indexed workspace-relative path (including an
// explicitly configured sibling wiki) or an authorized absolute wiki path to disk.
func ResolveDocumentPath(workspaceRoot, documentPath string) string {
	documentPath = strings.TrimSpace(strings.ReplaceAll(documentPath, `\`, "/"))
	if isAbsoluteDeclaredPath(documentPath) {
		return filepath.Clean(filepath.FromSlash(documentPath))
	}
	return filepath.Clean(filepath.Join(workspaceRoot, filepath.FromSlash(documentPath)))
}

func setupWikiGovernanceDocument(workspaceRoot string, source SetupWikiSource) (path, displayPath, sourceKind, reasonCode string) {
	if source.State != "resolved" || strings.TrimSpace(source.Root) == "" {
		return "", "", "", firstNonEmptySetupReason(source.ReasonCode, "setup_wiki_source_unavailable")
	}
	for _, candidate := range []struct {
		name       string
		sourceKind string
	}{
		{name: canonGovernanceFileName, sourceKind: "setup"},
		{name: legacyGovernanceFileName, sourceKind: "setup_legacy"},
	} {
		path := filepath.Join(source.Root, candidate.name)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", "", "", "setup_governance_source_unreadable"
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return "", "", "", "setup_governance_source_unsafe"
		}
		return filepath.Clean(path), WorkspacePathForSource(workspaceRoot, path), candidate.sourceKind, ""
	}
	path = filepath.Join(source.Root, canonGovernanceFileName)
	return filepath.Clean(path), WorkspacePathForSource(workspaceRoot, path), "setup", ""
}

func firstNonEmptySetupReason(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "setup_wiki_source_unavailable"
}
