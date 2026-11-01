package actions

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

type metadataRepairFixture struct {
	alias         string
	root          string
	projectPath   string
	registryPath  string
	project       model.ProjectFile
	registration  model.WorkspaceRegistration
	lastWorkspace string
}

func newMetadataRepairFixture(t *testing.T) metadataRepairFixture {
	t.Helper()
	_ = isolateHome(t)
	root := t.TempDir()
	for name, contents := range map[string]string{
		"go.mod":         "module example.invalid/tantalo\n\ngo 1.24\n",
		"Main.kt":        "package sample\n",
		"pyproject.toml": "[project]\nname = 'sample'\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	alias := "tantalo-fixture"
	registration, detected, err := workspace.DetectWorkspaceLayout(root, alias)
	if err != nil {
		t.Fatalf("DetectWorkspaceLayout: %v", err)
	}
	if !containsLanguage(detected.Project.Languages, "kotlin") || !containsLanguage(detected.Project.Languages, "python") {
		t.Fatalf("fixture detection languages = %v", detected.Project.Languages)
	}

	project := detected
	project.Project.Name = alias
	project.Project.Languages = []string{"python"}
	project.Project.Kind = "preserved-kind"
	project.Project.DefaultRepo = "preserved-default-repo"
	project.Project.DefaultEntrypoint = "preserved-default-entrypoint"
	project.Ignore.ExtraPatterns = []string{"preserved-ignore"}
	for i := range project.Repos {
		project.Repos[i].Languages = []string{"python"}
		project.Repos[i].Name = "preserved-repo-name"
		project.Repos[i].RepositoryIdentity = "preserved-repository-identity"
		project.Repos[i].DefaultEntrypoint = "preserved-repo-entrypoint"
	}
	project.Canons = []model.WorkspaceCanon{{ID: "preserved-canon", Root: "wiki", Role: "product"}}
	escapeMax := 7
	project.CanonPolicy = &model.CanonPolicyBlock{EscapeMax: &escapeMax}
	embeddingsEnabled := false
	project.Embeddings = &model.EmbeddingsBlock{Enabled: &embeddingsEnabled, Provider: "preserved-provider"}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile fixture: %v", err)
	}

	registration.Name = alias
	registration.Languages = []string{"python"}
	registration.Kind = "preserved-registry-kind"
	registration.Solution = "preserved.sln"
	registration.CanonLinks = []model.WorkspaceCanonLink{{Alias: "preserved-canon", Role: "product"}}
	if _, err := workspace.RegisterWorkspace(alias, registration); err != nil {
		t.Fatalf("RegisterWorkspace fixture: %v", err)
	}
	registry, err := workspace.LoadRegistryReadOnly()
	if err != nil {
		t.Fatalf("LoadRegistryReadOnly fixture: %v", err)
	}
	registry.Defaults.LastWorkspace = "preserved-last-workspace"
	if err := workspace.SaveRegistry(registry); err != nil {
		t.Fatalf("SaveRegistry fixture: %v", err)
	}
	projectPath := workspace.ProjectConfigPath(root)
	registryPath, err := workspace.RegistryPathReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	project, err = workspace.LoadProjectFile(root)
	if err != nil {
		t.Fatalf("LoadProjectFile fixture: %v", err)
	}
	return metadataRepairFixture{
		alias: alias, root: root, projectPath: projectPath,
		registryPath: registryPath, project: project, registration: registration,
		lastWorkspace: registry.Defaults.LastWorkspace,
	}
}

func TestResolveSelectorWithIdentityRetainsSelectedWorkspaceObject(t *testing.T) {
	fixture := newMetadataRepairFixture(t)
	registration, registered, selectedRoot, err := resolveSelectorWithIdentity(fixture.alias)
	if err != nil || !registered || selectedRoot == nil {
		t.Fatalf("resolveSelectorWithIdentity = registered %v root %v err %v", registered, selectedRoot, err)
	}
	currentRoot, err := os.Stat(fixture.root)
	if err != nil || !os.SameFile(selectedRoot, currentRoot) {
		t.Fatalf("selected identity does not match selected root: stat error=%v", err)
	}
	if registration.Name != fixture.alias || registration.Root != fixture.registration.Root {
		t.Fatalf("registration = %+v; want alias/root %+v", registration, fixture.registration)
	}
}

func TestRegisteredWorkspaceMetadataRepairPlansWithoutWriting(t *testing.T) {
	fixture := newMetadataRepairFixture(t)
	projectBefore, err := os.ReadFile(fixture.projectPath)
	if err != nil {
		t.Fatal(err)
	}
	registryBefore, err := os.ReadFile(fixture.registryPath)
	if err != nil {
		t.Fatal(err)
	}

	result, err := Inspect("workspace", fixture.alias, "", "")
	if err != nil || !result.Needed || result.ReasonCode != "WORKSPACE_METADATA_DRIFT" {
		t.Fatalf("Inspect = %+v, %v", result, err)
	}
	for _, disclosed := range []string{"repair state", "runtime receipt", "--reindex", "solo orientativa", "vuelve a seleccionar"} {
		if !strings.Contains(result.NextAction, disclosed) {
			t.Fatalf("NextAction %q does not disclose %q", result.NextAction, disclosed)
		}
	}
	projectAfter, err := os.ReadFile(fixture.projectPath)
	if err != nil {
		t.Fatal(err)
	}
	registryAfter, err := os.ReadFile(fixture.registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(projectBefore, projectAfter) || !bytes.Equal(registryBefore, registryAfter) {
		t.Fatal("plan modified project.toml or registry.toml")
	}
	if _, err := os.Stat(filepath.Join(fixture.root, ".mi-lsp", "index.db")); !os.IsNotExist(err) {
		t.Fatalf("plan touched index: %v", err)
	}
}

func TestRegisteredWorkspaceMetadataRepairApplyPreservesNonLanguageFields(t *testing.T) {
	fixture := newMetadataRepairFixture(t)
	originalRegistry, err := workspace.LoadRegistryReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	originalRegistration := originalRegistry.Workspaces[fixture.alias]
	_, detected, err := workspace.DetectWorkspaceLayout(fixture.root, fixture.alias)
	if err != nil {
		t.Fatal(err)
	}
	wantProject := fixture.project
	wantProject.Project.Languages = append([]string(nil), fixture.project.Project.Languages...)
	for _, language := range detected.Project.Languages {
		if !containsLanguage(wantProject.Project.Languages, language) {
			wantProject.Project.Languages = append(wantProject.Project.Languages, language)
		}
	}
	wantProject.Repos = append([]model.WorkspaceRepo(nil), fixture.project.Repos...)
	for i := range wantProject.Repos {
		wantProject.Repos[i].Languages = append([]string(nil), fixture.project.Repos[i].Languages...)
		for _, detectedRepo := range detected.Repos {
			if wantProject.Repos[i].ID == detectedRepo.ID || wantProject.Repos[i].Root == detectedRepo.Root {
				for _, language := range detectedRepo.Languages {
					if !containsLanguage(wantProject.Repos[i].Languages, language) {
						wantProject.Repos[i].Languages = append(wantProject.Repos[i].Languages, language)
					}
				}
				break
			}
		}
	}

	result, err := Apply("workspace", fixture.alias, "", "")
	if err != nil || !result.Changed {
		t.Fatalf("Apply = %+v, %v", result, err)
	}
	gotProject, err := workspace.LoadProjectFile(fixture.root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotProject, wantProject) {
		t.Fatalf("project metadata mismatch\n got: %#v\nwant: %#v", gotProject, wantProject)
	}
	gotRegistry, err := workspace.LoadRegistryReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	gotRegistration := gotRegistry.Workspaces[fixture.alias]
	if !reflect.DeepEqual(gotRegistration.CanonLinks, originalRegistration.CanonLinks) ||
		gotRegistration.Root != originalRegistration.Root ||
		gotRegistration.Kind != originalRegistration.Kind ||
		gotRegistration.Solution != originalRegistration.Solution {
		t.Fatalf("registration identity/metadata changed: got %+v want %+v", gotRegistration, originalRegistration)
	}
	if !reflect.DeepEqual(gotRegistration.Languages, gotProject.Project.Languages) {
		t.Fatalf("registry languages = %v; project languages = %v", gotRegistration.Languages, gotProject.Project.Languages)
	}
	if gotRegistry.Defaults.LastWorkspace != fixture.lastWorkspace {
		t.Fatalf("last_workspace = %q, want %q", gotRegistry.Defaults.LastWorkspace, fixture.lastWorkspace)
	}
	for _, name := range []string{"index.db", "graph.db", "catalog.db"} {
		if _, err := os.Stat(filepath.Join(fixture.root, ".mi-lsp", name)); !os.IsNotExist(err) {
			t.Fatalf("workspace metadata repair created %s: %v", name, err)
		}
	}
}

func TestRegisteredWorkspaceMetadataRepairFailsClosedOnUnknownTOML(t *testing.T) {
	for _, target := range []string{"project", "registry"} {
		t.Run(target, func(t *testing.T) {
			fixture := newMetadataRepairFixture(t)
			path := fixture.projectPath
			if target == "registry" {
				path = fixture.registryPath
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := file.WriteString("\npublic_extension = 'preserve-me'\n"); err != nil {
				_ = file.Close()
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			before, err = os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			for _, apply := range []bool{false, true} {
				var result Result
				if apply {
					result, err = Apply("workspace", fixture.alias, "", "")
				} else {
					result, err = Inspect("workspace", fixture.alias, "", "")
				}
				if err == nil || result.ReasonCode != "WORKSPACE_METADATA_UNSAFE" {
					t.Fatalf("apply=%v result=%+v err=%v", apply, result, err)
				}
				after, readErr := os.ReadFile(path)
				if readErr != nil || !bytes.Equal(before, after) {
					t.Fatalf("apply=%v overwrote unknown TOML: readErr=%v", apply, readErr)
				}
			}
		})
	}
}

func TestWorkspaceMetadataPartialMutationIsReportedAsChanged(t *testing.T) {
	path := "~/.mi-lsp/registry.toml"
	result, err := workspaceMetadataFailure(&workspace.PartialMutationError{
		Cause:         errors.New("rollback not verified"),
		RecoveryPaths: []string{path},
	})
	if err == nil || !result.Changed || result.ReasonCode != "WORKSPACE_METADATA_PARTIAL_MUTATION" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if !strings.Contains(result.NextAction, path) {
		t.Fatalf("next action %q omits recovery path %q", result.NextAction, path)
	}
}

func containsLanguage(languages []string, want string) bool {
	for _, language := range languages {
		if strings.EqualFold(language, want) {
			return true
		}
	}
	return false
}
