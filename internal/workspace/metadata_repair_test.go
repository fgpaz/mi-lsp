package workspace

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func metadataRepairWorkspaceFixture(t *testing.T) (string, model.WorkspaceRegistration) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":  "module example.invalid/root-pin\n\ngo 1.24\n",
		"Main.kt": "package rootpin\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	alias := "root-pin-fixture"
	registration, project, err := DetectWorkspaceLayout(root, alias)
	if err != nil {
		t.Fatalf("DetectWorkspaceLayout: %v", err)
	}
	project.Project.Name = alias
	project.Project.Languages = []string{"go"}
	for i := range project.Repos {
		project.Repos[i].Languages = []string{"go"}
	}
	if err := SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	registration.Name = alias
	registration.Languages = []string{"go"}
	if _, err := RegisterWorkspace(alias, registration); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	registry, err := LoadRegistryReadOnly()
	if err != nil {
		t.Fatalf("LoadRegistryReadOnly: %v", err)
	}
	return root, registry.Workspaces[alias]
}

func TestOpenPinnedWorkspaceRootRejectsSymlink(t *testing.T) {
	target := t.TempDir()
	parent := t.TempDir()
	link := filepath.Join(parent, "workspace-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := openPinnedWorkspaceRoot(link); !errors.Is(err, ErrWorkspaceMetadataUnsafe) {
		t.Fatalf("open symlink root error = %v, want unsafe", err)
	}
}

func TestApplyWorkspaceMetadataRepairRejectsSelectionToPinSwap(t *testing.T) {
	root, registration := metadataRepairWorkspaceFixture(t)
	selectedRoot, err := CaptureWorkspaceRootIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	originalProjectPath := ProjectConfigPath(root)
	originalProject, err := os.ReadFile(originalProjectPath)
	if err != nil {
		t.Fatal(err)
	}
	registryPath, err := RegistryPathReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	originalRegistry, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}

	movedRoot := root + "-selected-original"
	t.Cleanup(func() { _ = os.RemoveAll(movedRoot) })
	decoyProjectPath := ProjectConfigPath(root)
	var decoyProject []byte
	swapped := false
	_, changed, err := applyWorkspaceMetadataRepairSelected(
		WorkspaceMetadataSelection{Registration: registration, RootInfo: selectedRoot},
		metadataRepairHooks{
			beforeOpenPinned: func() error {
				swapped = true
				if err := os.Rename(root, movedRoot); err != nil {
					return err
				}
				if err := os.MkdirAll(root, 0700); err != nil {
					return err
				}
				for _, name := range []string{"go.mod", "Main.kt"} {
					contents, err := os.ReadFile(filepath.Join(movedRoot, name))
					if err != nil {
						return err
					}
					if err := os.WriteFile(filepath.Join(root, name), contents, 0600); err != nil {
						return err
					}
				}
				project, err := LoadProjectFile(movedRoot)
				if err != nil {
					return err
				}
				project.Project.Languages = []string{"python"}
				if err := SaveProjectFile(root, project); err != nil {
					return err
				}
				decoyProject, err = os.ReadFile(decoyProjectPath)
				return err
			},
		},
	)
	if !swapped {
		t.Fatal("pre-pin swap hook did not run")
	}
	if !errors.Is(err, ErrWorkspaceMetadataRootChanged) || changed {
		t.Fatalf("apply changed=%v err=%v; want root-changed with no mutation", changed, err)
	}
	originalAfter, err := os.ReadFile(filepath.Join(movedRoot, ".mi-lsp", "project.toml"))
	if err != nil || !bytes.Equal(originalAfter, originalProject) {
		t.Fatalf("selected original project changed: read error=%v", err)
	}
	decoyAfter, err := os.ReadFile(decoyProjectPath)
	if err != nil || !bytes.Equal(decoyAfter, decoyProject) {
		t.Fatalf("replacement destination changed: read error=%v", err)
	}
	gotRegistry, err := os.ReadFile(registryPath)
	if err != nil || !bytes.Equal(gotRegistry, originalRegistry) {
		t.Fatalf("registry changed on selection-to-pin swap: read error=%v", err)
	}
}

func TestApplyWorkspaceMetadataRepairRejectsRootSwapBeforeCommit(t *testing.T) {
	root, registration := metadataRepairWorkspaceFixture(t)
	originalProjectPath := ProjectConfigPath(root)
	originalProject, err := os.ReadFile(originalProjectPath)
	if err != nil {
		t.Fatal(err)
	}
	registryPath, err := RegistryPathReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	originalRegistry, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	movedRoot := root + "-pinned-original"
	t.Cleanup(func() { _ = os.RemoveAll(movedRoot) })
	decoyProject := []byte("decoy destination must stay untouched\n")

	_, changed, err := applyWorkspaceMetadataRepair(registration, metadataRepairHooks{
		beforeProjectCommit: func(*pinnedWorkspaceRoot) error {
			if err := os.Rename(root, movedRoot); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(originalProjectPath), 0700); err != nil {
				return err
			}
			return os.WriteFile(originalProjectPath, decoyProject, 0600)
		},
	})
	if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(32)) {
		if changed {
			t.Fatal("sharing violation reported a mutation")
		}
		if _, statErr := os.Stat(movedRoot); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("rename unexpectedly created destination: %v", statErr)
		}
		projectAfter, readErr := os.ReadFile(originalProjectPath)
		if readErr != nil || !bytes.Equal(projectAfter, originalProject) {
			t.Fatalf("original project changed after blocked rename: %v", readErr)
		}
		registryAfter, readErr := os.ReadFile(registryPath)
		if readErr != nil || !bytes.Equal(registryAfter, originalRegistry) {
			t.Fatalf("registry changed after blocked rename: %v", readErr)
		}
		return
	}
	if !errors.Is(err, ErrWorkspaceMetadataRootChanged) || changed {
		t.Fatalf("apply changed=%v err=%v; want root-changed with no mutation", changed, err)
	}
	movedProject, err := os.ReadFile(filepath.Join(movedRoot, ".mi-lsp", "project.toml"))
	if err != nil || !bytes.Equal(movedProject, originalProject) {
		t.Fatalf("pinned original project changed: read error=%v", err)
	}
	gotDecoy, err := os.ReadFile(originalProjectPath)
	if err != nil || !bytes.Equal(gotDecoy, decoyProject) {
		t.Fatalf("replacement destination changed: read error=%v", err)
	}
	gotRegistry, err := os.ReadFile(registryPath)
	if err != nil || !bytes.Equal(gotRegistry, originalRegistry) {
		t.Fatalf("registry changed on root swap: read error=%v", err)
	}
}

func TestApplyWorkspaceMetadataRepairReportsUnverifiedRollbackAsPartialMutation(t *testing.T) {
	root, registration := metadataRepairWorkspaceFixture(t)
	projectPath := ProjectConfigPath(root)
	original, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	_, changed, err := applyWorkspaceMetadataRepair(registration, metadataRepairHooks{
		saveRegistry: func(model.RegistryFile) error {
			return errors.New("injected registry save failure")
		},
		restoreProject: func(*pinnedWorkspaceRoot, projectMetadataSnapshot, os.FileInfo) error {
			return errors.New("injected rollback failure")
		},
	})
	var partial *PartialMutationError
	if !errors.As(err, &partial) || !changed {
		t.Fatalf("apply changed=%v err=%v; want typed partial mutation", changed, err)
	}
	if len(partial.RecoveryPaths) != 1 || !bytes.Contains([]byte(partial.RecoveryPaths[0]), []byte(".mi-lsp")) {
		t.Fatalf("recovery paths = %v", partial.RecoveryPaths)
	}
	current, err := os.ReadFile(projectPath)
	if err != nil || bytes.Equal(current, original) {
		t.Fatalf("injected rollback should leave changed project for recovery; read error=%v", err)
	}
}
