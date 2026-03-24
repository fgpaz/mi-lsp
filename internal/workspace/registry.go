package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"

	"github.com/fgpaz/mi-lsp/internal/model"
)

const registryDirName = ".mi-lsp"

func GlobalDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, registryDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func RegistryPath() (string, error) {
	dir, err := GlobalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "registry.toml"), nil
}

func LoadRegistry() (model.RegistryFile, error) {
	registry := model.RegistryFile{Workspaces: map[string]model.WorkspaceRegistration{}}
	path, err := RegistryPath()
	if err != nil {
		return registry, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return registry, nil
	} else if err != nil {
		return registry, err
	}
	if _, err := toml.DecodeFile(path, &registry); err != nil {
		return registry, err
	}
	if registry.Workspaces == nil {
		registry.Workspaces = map[string]model.WorkspaceRegistration{}
	}
	for name, ws := range registry.Workspaces {
		ws.Name = name
		registry.Workspaces[name] = ws
	}
	return registry, nil
}

func SaveRegistry(registry model.RegistryFile) error {
	if registry.Workspaces == nil {
		registry.Workspaces = map[string]model.WorkspaceRegistration{}
	}
	path, err := RegistryPath()
	if err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return toml.NewEncoder(file).Encode(registry)
}

func RegisterWorkspace(name string, registration model.WorkspaceRegistration) (model.RegistryFile, error) {
	registry, err := LoadRegistry()
	if err != nil {
		return registry, err
	}
	if registry.Workspaces == nil {
		registry.Workspaces = map[string]model.WorkspaceRegistration{}
	}
	registration.Name = name
	registry.Workspaces[name] = registration
	registry.Defaults.LastWorkspace = name
	return registry, SaveRegistry(registry)
}

func RemoveWorkspace(name string) error {
	registry, err := LoadRegistry()
	if err != nil {
		return err
	}
	if _, ok := registry.Workspaces[name]; !ok {
		return fmt.Errorf("workspace %q is not registered", name)
	}
	delete(registry.Workspaces, name)
	if registry.Defaults.LastWorkspace == name {
		registry.Defaults.LastWorkspace = ""
	}
	return SaveRegistry(registry)
}

func ResolveWorkspace(nameOrPath string) (model.WorkspaceRegistration, error) {
	if nameOrPath == "" {
		registry, err := LoadRegistry()
		if err != nil {
			return model.WorkspaceRegistration{}, err
		}
		if registry.Defaults.LastWorkspace != "" {
			if ws, ok := registry.Workspaces[registry.Defaults.LastWorkspace]; ok {
				ws.Name = registry.Defaults.LastWorkspace
				return ws, nil
			}
		}
		return model.WorkspaceRegistration{}, errors.New("no workspace specified and no default workspace configured")
	}
	registry, err := LoadRegistry()
	if err != nil {
		return model.WorkspaceRegistration{}, err
	}
	if ws, ok := registry.Workspaces[nameOrPath]; ok {
		ws.Name = nameOrPath
		return ws, nil
	}
	if _, err := os.Stat(nameOrPath); err == nil {
		return DetectWorkspace(nameOrPath)
	}
	return model.WorkspaceRegistration{}, errors.New("workspace not found in registry and path does not exist")
}

func ListWorkspaces() ([]model.WorkspaceRegistration, error) {
	registry, err := LoadRegistry()
	if err != nil {
		return nil, err
	}
	items := make([]model.WorkspaceRegistration, 0, len(registry.Workspaces))
	for name, ws := range registry.Workspaces {
		ws.Name = name
		items = append(items, ws)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func WorkspaceStateDir(root string) string {
	return filepath.Join(root, registryDirName)
}

func ProjectConfigPath(root string) string {
	return filepath.Join(WorkspaceStateDir(root), "project.toml")
}

func LoadProjectFile(root string) (model.ProjectFile, error) {
	project := model.ProjectFile{}
	path := ProjectConfigPath(root)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return project, nil
	} else if err != nil {
		return project, err
	}
	_, err := toml.DecodeFile(path, &project)
	return project, err
}

func SaveProjectFile(root string, project model.ProjectFile) error {
	stateDir := WorkspaceStateDir(root)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	file, err := os.Create(ProjectConfigPath(root))
	if err != nil {
		return err
	}
	defer file.Close()
	return toml.NewEncoder(file).Encode(project)
}
