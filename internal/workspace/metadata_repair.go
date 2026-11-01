package workspace

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/fgpaz/mi-lsp/internal/model"
)

var (
	ErrWorkspaceMetadataUnsafe              = errors.New("workspace metadata contains unsupported TOML or file shape")
	ErrWorkspaceMetadataNotConfigured       = errors.New("workspace has no local project metadata")
	ErrWorkspaceMetadataUnavailable         = errors.New("workspace metadata cannot be read or detected")
	ErrWorkspaceMetadataRegistrationChanged = errors.New("workspace registration changed")
	ErrWorkspaceMetadataRootChanged         = errors.New("workspace root identity changed")
	ErrWorkspaceMetadataReadback            = errors.New("workspace metadata readback failed")
)

type PartialMutationError struct {
	Cause         error
	RecoveryPaths []string
}

func (e *PartialMutationError) Error() string {
	return "workspace metadata mutation may be partial; rollback could not be verified"
}

func (e *PartialMutationError) Unwrap() error { return e.Cause }

type WorkspaceMetadataRepairPlan struct {
	Registration        model.WorkspaceRegistration
	Project             model.ProjectFile
	MetadataPresent     bool
	ProjectChanged      bool
	RegistrationChanged bool
	Drift               bool
}

// WorkspaceMetadataSelection carries the physical root identity captured when
// Apply resolves its selector, so path equality cannot retarget the repair.
type WorkspaceMetadataSelection struct {
	Registration model.WorkspaceRegistration
	RootInfo     os.FileInfo
}

type projectMetadataSnapshot struct {
	Project model.ProjectFile
	Raw     []byte
	Mode    os.FileMode
	Info    os.FileInfo
}

type registrySnapshot struct {
	Registry model.RegistryFile
	Path     string
	Raw      []byte
	Mode     os.FileMode
	Info     os.FileInfo
}

type pinnedWorkspaceRoot struct {
	path      string
	root      *os.Root
	rootInfo  os.FileInfo
	state     *os.Root
	stateInfo os.FileInfo
}

type metadataRepairHooks struct {
	beforeOpenPinned    func() error
	beforeProjectCommit func(*pinnedWorkspaceRoot) error
	saveRegistry        func(model.RegistryFile) error
	restoreProject      func(*pinnedWorkspaceRoot, projectMetadataSnapshot, os.FileInfo) error
}

// PlanWorkspaceMetadataRepair compares a registered workspace with fresh
// detection. It never writes the registry, project metadata, or index.
func PlanWorkspaceMetadataRepair(expected model.WorkspaceRegistration) (WorkspaceMetadataRepairPlan, error) {
	registry, err := readStrictRegistry()
	if err != nil {
		return WorkspaceMetadataRepairPlan{}, err
	}
	registration, err := registeredMetadataTarget(registry.Registry, expected)
	if err != nil {
		return WorkspaceMetadataRepairPlan{}, err
	}
	pinned, err := openPinnedWorkspaceRoot(registration.Root)
	if err != nil {
		return WorkspaceMetadataRepairPlan{}, err
	}
	defer pinned.Close()
	plan, _, err := planWorkspaceMetadata(pinned, registration)
	if err == nil {
		err = pinned.verifyPath()
	}
	return plan, err
}

// ApplyWorkspaceMetadataRepair updates only languages in an existing project
// file and its existing registry entry. Project reads, atomic replacement,
// readback, and rollback stay relative to a pinned workspace directory handle.
func ApplyWorkspaceMetadataRepair(expected model.WorkspaceRegistration) (WorkspaceMetadataRepairPlan, bool, error) {
	rootInfo, err := os.Stat(expected.Root)
	if err != nil || !rootInfo.IsDir() {
		return WorkspaceMetadataRepairPlan{}, false, ErrWorkspaceMetadataRootChanged
	}
	return ApplyWorkspaceMetadataRepairSelected(WorkspaceMetadataSelection{
		Registration: expected,
		RootInfo:     rootInfo,
	})
}

// ApplyWorkspaceMetadataRepairSelected validates the selection token captured by
// the caller before reading or mutating anything through the pinned descriptor.
func ApplyWorkspaceMetadataRepairSelected(selection WorkspaceMetadataSelection) (WorkspaceMetadataRepairPlan, bool, error) {
	return applyWorkspaceMetadataRepairSelected(selection, metadataRepairHooks{})
}

func applyWorkspaceMetadataRepair(expected model.WorkspaceRegistration, hooks metadataRepairHooks) (WorkspaceMetadataRepairPlan, bool, error) {
	rootInfo, err := os.Stat(expected.Root)
	if err != nil || !rootInfo.IsDir() {
		return WorkspaceMetadataRepairPlan{}, false, ErrWorkspaceMetadataRootChanged
	}
	return applyWorkspaceMetadataRepairSelected(WorkspaceMetadataSelection{
		Registration: expected,
		RootInfo:     rootInfo,
	}, hooks)
}

func applyWorkspaceMetadataRepairSelected(selection WorkspaceMetadataSelection, hooks metadataRepairHooks) (WorkspaceMetadataRepairPlan, bool, error) {
	expected := selection.Registration
	if selection.RootInfo == nil || !selection.RootInfo.IsDir() {
		return WorkspaceMetadataRepairPlan{}, false, ErrWorkspaceMetadataRootChanged
	}
	var plan WorkspaceMetadataRepairPlan
	changed := false
	err := WithRegistryLock(func() error {
		registry, err := readStrictRegistry()
		if err != nil {
			return err
		}
		registration, err := registeredMetadataTarget(registry.Registry, expected)
		if err != nil {
			return err
		}
		if hooks.beforeOpenPinned != nil {
			if err := hooks.beforeOpenPinned(); err != nil {
				return err
			}
		}
		pinned, err := openPinnedWorkspaceRoot(registration.Root)
		if err != nil {
			return err
		}
		defer pinned.Close()
		if !os.SameFile(selection.RootInfo, pinned.rootInfo) {
			return ErrWorkspaceMetadataRootChanged
		}

		var project projectMetadataSnapshot
		plan, project, err = planWorkspaceMetadata(pinned, registration)
		if err != nil || !plan.Drift {
			return err
		}

		var projectData []byte
		if plan.ProjectChanged {
			projectData, err = encodeProjectMetadata(plan.Project)
			if err != nil {
				return err
			}
		}
		updatedRegistry := registry.Registry
		updated := plan.Registration
		updated.Languages = append([]string(nil), plan.Project.Project.Languages...)
		updated.Name = registration.Name
		updatedRegistry.Workspaces[registration.Name] = updated

		projectWritten := false
		var projectCurrentInfo os.FileInfo
		if plan.ProjectChanged {
			if hooks.beforeProjectCommit != nil {
				if err := hooks.beforeProjectCommit(pinned); err != nil {
					return err
				}
			}
			if err := pinned.verifyPath(); err != nil {
				return err
			}
			projectCurrentInfo, projectWritten, err = pinned.replaceProject(projectData, project.Mode, project.Info, true)
			if err != nil {
				if !projectWritten {
					return err
				}
				rollbackErr, partial := rollbackMetadataFailure(err, pinned, project, projectCurrentInfo, projectWritten, false, registry, nil, hooks)
				changed = changed || partial
				return rollbackErr
			}
		}

		if err := pinned.verifyPath(); err != nil {
			if !projectWritten {
				return err
			}
			rollbackErr, partial := rollbackMetadataFailure(err, pinned, project, projectCurrentInfo, projectWritten, false, registry, nil, hooks)
			changed = changed || partial
			return rollbackErr
		}

		registryAttempted := false
		var registryCurrentInfo os.FileInfo
		if plan.RegistrationChanged {
			if err := pinned.verifyPath(); err != nil {
				if !projectWritten {
					return err
				}
				rollbackErr, partial := rollbackMetadataFailure(err, pinned, project, projectCurrentInfo, projectWritten, false, registry, nil, hooks)
				changed = changed || partial
				return rollbackErr
			}
			saveRegistry := hooks.saveRegistry
			if saveRegistry == nil {
				saveRegistry = SaveRegistry
			}
			registryAttempted = true
			if err := saveRegistry(updatedRegistry); err != nil {
				_, registryCurrentInfo, _ = readRawRegularFile(registry.Path)
				rollbackErr, partial := rollbackMetadataFailure(err, pinned, project, projectCurrentInfo, projectWritten, true, registry, registryCurrentInfo, hooks)
				changed = changed || partial
				return rollbackErr
			}
			_, registryCurrentInfo, err = readRawRegularFile(registry.Path)
			if err != nil {
				rollbackErr, partial := rollbackMetadataFailure(ErrWorkspaceMetadataReadback, pinned, project, projectCurrentInfo, projectWritten, true, registry, registryCurrentInfo, hooks)
				changed = changed || partial
				return rollbackErr
			}
		}

		if err := pinned.verifyPath(); err != nil {
			rollbackErr, partial := rollbackMetadataFailure(err, pinned, project, projectCurrentInfo, projectWritten, registryAttempted, registry, registryCurrentInfo, hooks)
			changed = changed || partial
			return rollbackErr
		}

		readbackProject, readbackErr := readStrictProjectFile(pinned)
		readbackRegistry, registryErr := readStrictRegistry()
		readbackRegistration, targetErr := registeredMetadataTarget(readbackRegistry.Registry, registration)
		if readbackErr != nil || registryErr != nil || targetErr != nil ||
			!reflect.DeepEqual(readbackProject.Project, plan.Project) ||
			!reflect.DeepEqual(readbackRegistry.Registry, updatedRegistry) ||
			!reflect.DeepEqual(readbackRegistration.Languages, updated.Languages) {
			cause := errors.Join(ErrWorkspaceMetadataReadback, readbackErr, registryErr, targetErr)
			if registryAttempted {
				_, registryCurrentInfo, _ = readRawRegularFile(registry.Path)
			}
			rollbackErr, partial := rollbackMetadataFailure(cause, pinned, project, projectCurrentInfo, projectWritten, registryAttempted, registry, registryCurrentInfo, hooks)
			changed = changed || partial
			return rollbackErr
		}
		if err := pinned.verifyPath(); err != nil {
			if registryAttempted {
				_, registryCurrentInfo, _ = readRawRegularFile(registry.Path)
			}
			rollbackErr, partial := rollbackMetadataFailure(err, pinned, project, projectCurrentInfo, projectWritten, registryAttempted, registry, registryCurrentInfo, hooks)
			changed = changed || partial
			return rollbackErr
		}

		plan.Registration = readbackRegistration
		plan.Project = readbackProject.Project
		changed = true
		return nil
	})
	return plan, changed, err
}

func rollbackMetadataFailure(cause error, pinned *pinnedWorkspaceRoot, project projectMetadataSnapshot, currentProjectInfo os.FileInfo, projectWritten, registryAttempted bool, registry registrySnapshot, currentRegistryInfo os.FileInfo, hooks metadataRepairHooks) (error, bool) {
	var recoveryPaths []string
	var rollbackErrors []error
	if registryAttempted {
		if err := restoreRegistrySnapshot(registry, currentRegistryInfo); err != nil {
			recoveryPaths = append(recoveryPaths, displayRecoveryPath(registry.Path))
			rollbackErrors = append(rollbackErrors, err)
		}
	}
	if projectWritten {
		restoreProject := hooks.restoreProject
		if restoreProject == nil {
			restoreProject = func(root *pinnedWorkspaceRoot, snapshot projectMetadataSnapshot, current os.FileInfo) error {
				return root.restoreProject(snapshot, current)
			}
		}
		if err := restoreProject(pinned, project, currentProjectInfo); err != nil {
			recoveryPaths = append(recoveryPaths, workspaceProjectRecoveryPath(pinned))
			rollbackErrors = append(rollbackErrors, err)
		}
	}
	if len(recoveryPaths) != 0 {
		return &PartialMutationError{Cause: errors.Join(cause, errors.Join(rollbackErrors...)), RecoveryPaths: recoveryPaths}, true
	}
	return cause, false
}

func planWorkspaceMetadata(pinned *pinnedWorkspaceRoot, registration model.WorkspaceRegistration) (WorkspaceMetadataRepairPlan, projectMetadataSnapshot, error) {
	if err := pinned.verifyPath(); err != nil {
		return WorkspaceMetadataRepairPlan{}, projectMetadataSnapshot{}, err
	}
	project, err := readStrictProjectFile(pinned)
	if errors.Is(err, ErrWorkspaceMetadataNotConfigured) {
		return WorkspaceMetadataRepairPlan{Registration: registration}, projectMetadataSnapshot{}, nil
	}
	if err != nil {
		return WorkspaceMetadataRepairPlan{}, projectMetadataSnapshot{}, err
	}
	if err := pinned.verifyPath(); err != nil {
		return WorkspaceMetadataRepairPlan{}, projectMetadataSnapshot{}, err
	}
	_, detected, err := DetectWorkspaceLayout(registration.Root, registration.Name)
	if err != nil {
		return WorkspaceMetadataRepairPlan{}, projectMetadataSnapshot{}, ErrWorkspaceMetadataUnavailable
	}
	if err := pinned.verifyPath(); err != nil {
		return WorkspaceMetadataRepairPlan{}, projectMetadataSnapshot{}, err
	}
	candidate := project.Project
	candidate.Project.Languages = mergeDetectedLanguages(project.Project.Project.Languages, detected.Project.Languages)
	candidate.Repos = mergeDetectedRepoLanguages(project.Project.Repos, detected.Repos)
	updatedRegistration := registration
	updatedRegistration.Languages = append([]string(nil), candidate.Project.Languages...)
	projectChanged := !reflect.DeepEqual(project.Project, candidate)
	registrationChanged := !reflect.DeepEqual(registration.Languages, updatedRegistration.Languages)
	plan := WorkspaceMetadataRepairPlan{
		Registration:        updatedRegistration,
		Project:             candidate,
		MetadataPresent:     true,
		ProjectChanged:      projectChanged,
		RegistrationChanged: registrationChanged,
		Drift:               projectChanged || registrationChanged,
	}
	return plan, project, nil
}

func encodeProjectMetadata(project model.ProjectFile) ([]byte, error) {
	var contents bytes.Buffer
	if err := toml.NewEncoder(&contents).Encode(project); err != nil {
		return nil, err
	}
	return contents.Bytes(), nil
}

func mergeDetectedLanguages(existing, detected []string) []string {
	merged := append([]string(nil), existing...)
	seen := make(map[string]struct{}, len(existing)+len(detected))
	for _, language := range existing {
		key := strings.ToLower(strings.TrimSpace(language))
		if key != "" {
			seen[key] = struct{}{}
		}
	}
	for _, language := range detected {
		language = strings.TrimSpace(language)
		key := strings.ToLower(language)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		merged = append(merged, language)
		seen[key] = struct{}{}
	}
	return merged
}

func mergeDetectedRepoLanguages(existing, detected []model.WorkspaceRepo) []model.WorkspaceRepo {
	merged := append([]model.WorkspaceRepo(nil), existing...)
	for _, detectedRepo := range detected {
		match := -1
		if id := strings.ToLower(strings.TrimSpace(detectedRepo.ID)); id != "" {
			for i := range merged {
				if strings.ToLower(strings.TrimSpace(merged[i].ID)) == id {
					match = i
					break
				}
			}
		}
		if match < 0 {
			root := strings.TrimSpace(detectedRepo.Root)
			if root != "" {
				root = strings.ToLower(filepath.ToSlash(filepath.Clean(root)))
				for i := range merged {
					otherRoot := strings.TrimSpace(merged[i].Root)
					if otherRoot != "" && strings.ToLower(filepath.ToSlash(filepath.Clean(otherRoot))) == root {
						match = i
						break
					}
				}
			}
		}
		if match >= 0 {
			merged[match].Languages = mergeDetectedLanguages(merged[match].Languages, detectedRepo.Languages)
		}
	}
	return merged
}

func openPinnedWorkspaceRoot(path string) (*pinnedWorkspaceRoot, error) {
	if !filepath.IsAbs(path) {
		return nil, ErrWorkspaceMetadataUnsafe
	}
	path = filepath.Clean(path)
	if err := rejectSymlinkPathComponents(path); err != nil {
		return nil, err
	}
	before, err := os.Lstat(path)
	if err != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return nil, ErrWorkspaceMetadataUnsafe
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, ErrWorkspaceMetadataUnavailable
	}
	opened, openErr := root.Stat(".")
	after, statErr := os.Lstat(path)
	if openErr != nil || statErr != nil || after.Mode()&os.ModeSymlink != 0 || !after.IsDir() ||
		!os.SameFile(before, opened) || !os.SameFile(after, opened) {
		_ = root.Close()
		return nil, ErrWorkspaceMetadataRootChanged
	}
	if err := rejectSymlinkPathComponents(path); err != nil {
		_ = root.Close()
		return nil, ErrWorkspaceMetadataRootChanged
	}
	pinned := &pinnedWorkspaceRoot{path: path, root: root, rootInfo: opened}
	stateInfo, err := root.Lstat(".mi-lsp")
	if errors.Is(err, os.ErrNotExist) {
		return pinned, nil
	}
	if err != nil || !stateInfo.IsDir() || stateInfo.Mode()&os.ModeSymlink != 0 {
		_ = root.Close()
		return nil, ErrWorkspaceMetadataUnsafe
	}
	state, err := root.OpenRoot(".mi-lsp")
	if err != nil {
		_ = root.Close()
		return nil, ErrWorkspaceMetadataUnsafe
	}
	stateOpened, stateOpenErr := state.Stat(".")
	stateAfter, stateStatErr := root.Lstat(".mi-lsp")
	if stateOpenErr != nil || stateStatErr != nil || stateAfter.Mode()&os.ModeSymlink != 0 ||
		!stateAfter.IsDir() || !os.SameFile(stateInfo, stateOpened) || !os.SameFile(stateAfter, stateOpened) {
		_ = state.Close()
		_ = root.Close()
		return nil, ErrWorkspaceMetadataRootChanged
	}
	pinned.state = state
	pinned.stateInfo = stateOpened
	return pinned, nil
}

func (pinned *pinnedWorkspaceRoot) Close() error {
	var stateErr error
	if pinned.state != nil {
		stateErr = pinned.state.Close()
	}
	return errors.Join(stateErr, pinned.root.Close())
}

func (pinned *pinnedWorkspaceRoot) verifyDescriptor() error {
	rootInfo, err := pinned.root.Stat(".")
	if err != nil || !os.SameFile(rootInfo, pinned.rootInfo) {
		return ErrWorkspaceMetadataRootChanged
	}
	if pinned.state != nil {
		stateInfo, err := pinned.state.Stat(".")
		currentState, stateErr := pinned.root.Lstat(".mi-lsp")
		if err != nil || stateErr != nil || currentState.Mode()&os.ModeSymlink != 0 ||
			!currentState.IsDir() || !os.SameFile(stateInfo, pinned.stateInfo) || !os.SameFile(currentState, pinned.stateInfo) {
			return ErrWorkspaceMetadataRootChanged
		}
	}
	return nil
}

func (pinned *pinnedWorkspaceRoot) verifyPath() error {
	if err := pinned.verifyDescriptor(); err != nil {
		return err
	}
	if err := rejectSymlinkPathComponents(pinned.path); err != nil {
		return ErrWorkspaceMetadataRootChanged
	}
	current, err := os.Lstat(pinned.path)
	if err != nil || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(current, pinned.rootInfo) {
		return ErrWorkspaceMetadataRootChanged
	}
	return nil
}

func rejectSymlinkPathComponents(path string) error {
	volume := filepath.VolumeName(path)
	separator := string(os.PathSeparator)
	prefix := volume + separator
	if volume == "" {
		prefix = separator
	}
	remainder := strings.TrimPrefix(path, prefix)
	current := filepath.Clean(prefix)
	for _, component := range strings.Split(remainder, separator) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return ErrWorkspaceMetadataUnavailable
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrWorkspaceMetadataUnsafe
		}
		if current != path && !info.IsDir() {
			return ErrWorkspaceMetadataUnsafe
		}
	}
	return nil
}

func readStrictProjectFile(pinned *pinnedWorkspaceRoot) (projectMetadataSnapshot, error) {
	if pinned.state == nil {
		return projectMetadataSnapshot{}, ErrWorkspaceMetadataNotConfigured
	}
	info, err := pinned.state.Lstat("project.toml")
	if errors.Is(err, os.ErrNotExist) {
		return projectMetadataSnapshot{}, ErrWorkspaceMetadataNotConfigured
	}
	if err != nil {
		return projectMetadataSnapshot{}, ErrWorkspaceMetadataUnavailable
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return projectMetadataSnapshot{}, ErrWorkspaceMetadataUnsafe
	}
	file, err := pinned.state.Open("project.toml")
	if err != nil {
		return projectMetadataSnapshot{}, ErrWorkspaceMetadataUnavailable
	}
	openedInfo, statErr := file.Stat()
	if statErr != nil || !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return projectMetadataSnapshot{}, ErrWorkspaceMetadataRootChanged
	}
	contents, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return projectMetadataSnapshot{}, ErrWorkspaceMetadataUnavailable
	}
	current, err := pinned.state.Lstat("project.toml")
	if err != nil || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, current) {
		return projectMetadataSnapshot{}, ErrWorkspaceMetadataRootChanged
	}
	project := model.ProjectFile{}
	metadata, err := toml.Decode(string(contents), &project)
	if err != nil {
		return projectMetadataSnapshot{}, ErrWorkspaceMetadataUnavailable
	}
	if len(metadata.Undecoded()) != 0 {
		return projectMetadataSnapshot{}, ErrWorkspaceMetadataUnsafe
	}
	return projectMetadataSnapshot{Project: project, Raw: contents, Mode: info.Mode().Perm(), Info: info}, nil
}

func readStrictRegistry() (registrySnapshot, error) {
	path, err := RegistryPathReadOnly()
	if err != nil {
		return registrySnapshot{}, ErrWorkspaceMetadataUnavailable
	}
	contents, info, err := readRawRegularFile(path)
	if err != nil {
		return registrySnapshot{}, ErrWorkspaceMetadataUnsafe
	}
	registry := model.RegistryFile{}
	metadata, err := toml.Decode(string(contents), &registry)
	if err != nil {
		return registrySnapshot{}, ErrWorkspaceMetadataUnavailable
	}
	if len(metadata.Undecoded()) != 0 {
		return registrySnapshot{}, ErrWorkspaceMetadataUnsafe
	}
	if registry.Workspaces == nil {
		registry.Workspaces = map[string]model.WorkspaceRegistration{}
	}
	for name, registration := range registry.Workspaces {
		registration.Name = name
		registry.Workspaces[name] = registration
	}
	return registrySnapshot{Registry: registry, Path: path, Raw: contents, Mode: info.Mode().Perm(), Info: info}, nil
}

func readRawRegularFile(path string) ([]byte, os.FileInfo, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 {
		return nil, nil, ErrWorkspaceMetadataUnsafe
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	after, err := os.Lstat(path)
	if err != nil || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, after) {
		return nil, nil, ErrWorkspaceMetadataRootChanged
	}
	return contents, after, nil
}

func registeredMetadataTarget(registry model.RegistryFile, expected model.WorkspaceRegistration) (model.WorkspaceRegistration, error) {
	name := strings.TrimSpace(expected.Name)
	registration, ok := registry.Workspaces[name]
	if !ok || !sameRegisteredRoot(registration.Root, expected.Root) {
		return model.WorkspaceRegistration{}, ErrWorkspaceMetadataRegistrationChanged
	}
	registration.Name = name
	return registration, nil
}

func sameRegisteredRoot(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func (pinned *pinnedWorkspaceRoot) replaceProject(contents []byte, mode os.FileMode, expected os.FileInfo, requirePath bool) (os.FileInfo, bool, error) {
	if pinned.state == nil || expected == nil {
		return nil, false, ErrWorkspaceMetadataNotConfigured
	}
	if requirePath {
		if err := pinned.verifyPath(); err != nil {
			return nil, false, err
		}
	} else if err := pinned.verifyDescriptor(); err != nil {
		return nil, false, err
	}
	current, err := pinned.state.Lstat("project.toml")
	if err != nil || current.Mode()&os.ModeSymlink != 0 || !current.Mode().IsRegular() || !os.SameFile(current, expected) {
		return nil, false, ErrWorkspaceMetadataRootChanged
	}
	tempName, err := metadataTempName()
	if err != nil {
		return nil, false, err
	}
	file, err := pinned.state.OpenFile(tempName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, false, err
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
		_ = pinned.state.Remove(tempName)
	}()
	if _, err := io.Copy(file, bytes.NewReader(contents)); err != nil {
		return nil, false, err
	}
	if err := file.Sync(); err != nil {
		return nil, false, err
	}
	if err := file.Chmod(mode.Perm()); err != nil {
		return nil, false, err
	}
	if err := file.Close(); err != nil {
		closed = true
		return nil, false, err
	}
	closed = true
	if requirePath {
		if err := pinned.verifyPath(); err != nil {
			return nil, false, err
		}
	} else if err := pinned.verifyDescriptor(); err != nil {
		return nil, false, err
	}
	current, err = pinned.state.Lstat("project.toml")
	if err != nil || current.Mode()&os.ModeSymlink != 0 || !current.Mode().IsRegular() || !os.SameFile(current, expected) {
		return nil, false, ErrWorkspaceMetadataRootChanged
	}
	if err := pinned.state.Rename(tempName, "project.toml"); err != nil {
		return nil, false, err
	}
	newInfo, err := pinned.state.Lstat("project.toml")
	if err != nil {
		return nil, true, ErrWorkspaceMetadataReadback
	}
	return newInfo, true, nil
}

func (pinned *pinnedWorkspaceRoot) restoreProject(snapshot projectMetadataSnapshot, current os.FileInfo) error {
	if current == nil {
		return ErrWorkspaceMetadataReadback
	}
	if _, replaced, err := pinned.replaceProject(snapshot.Raw, snapshot.Mode, current, false); err != nil || !replaced {
		return errors.Join(ErrWorkspaceMetadataReadback, err)
	}
	if err := pinned.verifyDescriptor(); err != nil {
		return errors.Join(ErrWorkspaceMetadataReadback, err)
	}
	contents, info, err := readRawRootFile(pinned.state, "project.toml")
	if err != nil || !bytes.Equal(contents, snapshot.Raw) || info.Mode().Perm() != snapshot.Mode.Perm() {
		return errors.Join(ErrWorkspaceMetadataReadback, err)
	}
	return nil
}

func readRawRootFile(root *os.Root, path string) ([]byte, os.FileInfo, error) {
	info, err := root.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, ErrWorkspaceMetadataUnsafe
	}
	file, err := root.Open(path)
	if err != nil {
		return nil, nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return nil, nil, ErrWorkspaceMetadataRootChanged
	}
	contents, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return nil, nil, errors.Join(readErr, closeErr)
	}
	return contents, info, nil
}

func restoreRegistrySnapshot(snapshot registrySnapshot, expectedCurrent os.FileInfo) error {
	contents, current, err := readRawRegularFile(snapshot.Path)
	if err != nil {
		return err
	}
	if bytes.Equal(contents, snapshot.Raw) && current.Mode().Perm() == snapshot.Mode.Perm() {
		return nil
	}
	if expectedCurrent == nil || !os.SameFile(current, expectedCurrent) {
		return ErrWorkspaceMetadataReadback
	}
	if err := writeFileAtomic(snapshot.Path, snapshot.Mode, func(w io.Writer) error {
		_, writeErr := w.Write(snapshot.Raw)
		return writeErr
	}); err != nil {
		return err
	}
	after, info, err := readRawRegularFile(snapshot.Path)
	if err != nil || !bytes.Equal(after, snapshot.Raw) || info.Mode().Perm() != snapshot.Mode.Perm() {
		return errors.Join(ErrWorkspaceMetadataReadback, err)
	}
	invalidateRegistryCache(snapshot.Path)
	return nil
}

func metadataTempName() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return ".project-repair-" + hex.EncodeToString(random[:]) + ".tmp", nil
}

func workspaceProjectRecoveryPath(pinned *pinnedWorkspaceRoot) string {
	selected := displayRecoveryPath(pinned.path)
	if err := pinned.verifyPath(); err != nil {
		return "original pinned workspace object opened at " + selected + " (path identity changed); .mi-lsp/project.toml"
	}
	return filepath.Join(selected, ".mi-lsp", "project.toml")
}

func displayRecoveryPath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil {
		if relative, relErr := filepath.Rel(home, path); relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			return filepath.Join("~", relative)
		}
	}
	return path
}

func writeFileAtomic(path string, mode os.FileMode, write func(io.Writer) error) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".metadata-repair-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpPath)
	}()
	if err := write(tmp); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		closed = true
		return err
	}
	closed = true
	if err := os.Chmod(tmpPath, mode.Perm()); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return nil
}
