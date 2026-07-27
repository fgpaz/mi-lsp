package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/docgraph"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestNavGovernanceReportsEffectiveProfileAndSync(t *testing.T) {
	alias := "gov-ok-" + filepath.Base(t.TempDir())
	root := createIndexedWorkspaceFixture(t, alias)
	app := New(root, nil)

	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	}); err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.governance",
		Context:   model.QueryOptions{Workspace: alias},
	})
	if err != nil {
		t.Fatalf("nav.governance: %v", err)
	}
	if env.Backend != "governance" {
		t.Fatalf("backend = %q, want governance", env.Backend)
	}
	items := env.Items.([]model.GovernanceStatus)
	if len(items) != 1 {
		t.Fatalf("expected one governance status, got %#v", env.Items)
	}
	status := items[0]
	if status.Blocked {
		t.Fatalf("expected governance to pass, got %#v", status)
	}
	if status.Profile != "spec_backend" {
		t.Fatalf("profile = %q, want spec_backend", status.Profile)
	}
	if status.EffectiveBase != "ordered_wiki" {
		t.Fatalf("effective base = %q, want ordered_wiki", status.EffectiveBase)
	}
	if status.Sync != "in_sync" {
		t.Fatalf("sync = %q, want in_sync", status.Sync)
	}
}

func TestNavGovernanceRejectsDeclaredLegacyAECanonRoots(t *testing.T) {
	for _, aeRoot := range []string{".docs/wiki/ae", "wiki/ae", ".docs/ae"} {
		t.Run(aeRoot, func(t *testing.T) {
			alias := "gov-ae-" + filepath.Base(t.TempDir())
			ensureWritableTestHome(t)
			root := t.TempDir()
			writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
			writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# 07. Baseline tecnica\n")
			writeAECanonModules(t, root, aeRoot)
			if aeRoot == ".docs/wiki/ae" {
				writeWorkspaceFile(t, root, ".docs/wiki/ae/README.md", "# AE\n\nCanon lives in `.docs/wiki/ae/README.md`.\n")
			}
			writeSpecBackendGovernanceFixtureWithAE(t, root, aeRoot)

			app := New(root, nil)
			if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
				Name:      alias,
				Root:      root,
				Languages: []string{"csharp"},
				Kind:      model.WorkspaceKindSingle,
			}); err != nil {
				t.Fatalf("RegisterWorkspace: %v", err)
			}
			if err := workspace.SaveProjectFile(root, model.ProjectFile{
				Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "main"},
				Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
			}); err != nil {
				t.Fatalf("SaveProjectFile: %v", err)
			}
			defer func() { _ = workspace.RemoveWorkspace(alias) }()

			env, err := app.Execute(context.Background(), model.CommandRequest{
				Operation: "nav.governance",
				Context:   model.QueryOptions{Workspace: alias},
			})
			if err != nil {
				t.Fatalf("nav.governance: %v", err)
			}
			status := env.Items.([]model.GovernanceStatus)[0]
			if !status.Blocked {
				t.Fatalf("expected legacy AE declaration to block governance, got %#v", status)
			}
			if status.AECanon.Status != "mismatch" || status.AECanon.Reason != "ae_canon_legacy_mode_rejected" {
				t.Fatalf("expected legacy AE rejection, got %#v", status.AECanon)
			}
			if len(status.AECanon.Roots) != 1 || status.AECanon.Roots[0] != aeRoot {
				t.Fatalf("ae_canon.roots = %#v, want [%s]", status.AECanon.Roots, aeRoot)
			}
		})
	}
}

func TestNavGovernanceSupportsKernelV2ExternalCanon(t *testing.T) {
	alias := "gov-ae-kernel-v2-" + filepath.Base(t.TempDir())
	ensureWritableTestHome(t)
	root := t.TempDir()
	kernelHome := t.TempDir()
	t.Setenv("AE_KERNEL_HOME", "  "+kernelHome+"  ")
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# 07. Baseline tecnica\n")
	writeSpecBackendGovernanceFixture(t, root)
	addKernelV2AECanonToGovernanceFixture(t, root)
	writeKernelV2CanonModules(t, kernelHome)
	writeKernelV2RepoPolicy(t, root)

	app := New(root, nil)
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "main"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.governance",
		Context:   model.QueryOptions{Workspace: alias},
	})
	if err != nil {
		t.Fatalf("nav.governance: %v", err)
	}
	status := env.Items.([]model.GovernanceStatus)[0]
	if status.Blocked {
		t.Fatalf("expected kernel v2 governance to pass, got %#v", status)
	}
	if status.AECanon.Status != "valid" || status.AECanon.Source != "kernel_v2" {
		t.Fatalf("expected valid kernel v2 ae_canon, got %#v", status.AECanon)
	}
	wantRoots := []string{"<kernel_home>/canon", ".docs/ae/repo-policy.yaml"}
	if len(status.AECanon.Roots) != len(wantRoots) || status.AECanon.Roots[0] != wantRoots[0] || status.AECanon.Roots[1] != wantRoots[1] {
		t.Fatalf("ae_canon.roots = %#v, want %#v", status.AECanon.Roots, wantRoots)
	}
	if _, err := os.Stat(filepath.Join(root, ".docs", "ae", "AE-KERNEL-V2.md")); !os.IsNotExist(err) {
		t.Fatalf("kernel v2 validation must not require repo-local canon modules, stat err=%v", err)
	}
	projection, err := os.ReadFile(filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml"))
	if err != nil {
		t.Fatalf("read projected model: %v", err)
	}
	for _, expected := range []string{
		"[governance.ae_canon]",
		"mode = \"kernel_v2\"",
		"source = \"<kernel_home>/canon\"",
		"repo_policy = \".docs/ae/repo-policy.yaml\"",
	} {
		if !strings.Contains(string(projection), expected) {
			t.Fatalf("projected read model is missing %q:\n%s", expected, projection)
		}
	}
	profile, source, warnings := docgraph.LoadProfile(root)
	if source != "project" || len(warnings) != 0 {
		t.Fatalf("projected read model did not round-trip: source=%q warnings=%#v", source, warnings)
	}
	if profile.Governance.AECanon.Mode != "kernel_v2" || profile.Governance.AECanon.Source != "<kernel_home>/canon" || profile.Governance.AECanon.RepoPolicy != ".docs/ae/repo-policy.yaml" {
		t.Fatalf("projected ae_canon did not round-trip: %#v", profile.Governance.AECanon)
	}
}

func TestInspectGovernanceBlocksIncompleteKernelV2Sources(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	kernelHome := t.TempDir()
	t.Setenv("AE_KERNEL_HOME", kernelHome)
	writeSpecBackendGovernanceFixture(t, root)
	addKernelV2AECanonToGovernanceFixture(t, root)
	writeKernelV2CanonModules(t, kernelHome)

	status := docgraph.InspectGovernance(root, true)
	if !status.Blocked {
		t.Fatalf("expected missing repo policy to block governance, got %#v", status)
	}
	if status.AECanon.Status != "missing" || status.AECanon.Reason != "kernel_v2_sources_missing" {
		t.Fatalf("expected missing kernel v2 source status, got %#v", status.AECanon)
	}
	if len(status.AECanon.MissingModules) != 1 || status.AECanon.MissingModules[0] != ".docs/ae/repo-policy.yaml" {
		t.Fatalf("ae_canon.missing_modules = %#v, want repo policy", status.AECanon.MissingModules)
	}
}

func TestInspectGovernanceRejectsInvalidKernelV2Configuration(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeSpecBackendGovernanceFixture(t, root)
	path := filepath.Join(root, ".docs", "wiki", "00_gobierno_documental.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read governance fixture: %v", err)
	}
	config := strings.Join([]string{
		"ae_canon:",
		"  mode: kernel_v2",
		"  source: .docs/wiki/ae",
		"  repo_policy: ../repo-policy.yaml",
	}, "\n")
	updated := strings.Replace(string(content), "hierarchy:", config+"\nhierarchy:", 1)
	writeWorkspaceFile(t, root, ".docs/wiki/00_gobierno_documental.md", updated)

	status := docgraph.InspectGovernance(root, true)
	if !status.Blocked {
		t.Fatalf("expected invalid kernel v2 configuration to block governance, got %#v", status)
	}
	if status.AECanon.Status != "mismatch" || status.AECanon.Reason != "kernel_v2_config_invalid" {
		t.Fatalf("expected kernel v2 config mismatch, got %#v", status.AECanon)
	}
	if len(status.AECanon.MissingModules) != 2 {
		t.Fatalf("expected source and repo policy validation issues, got %#v", status.AECanon.MissingModules)
	}
}

func TestInspectGovernanceRejectsUnknownAECanonMode(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeSpecBackendGovernanceFixture(t, root)
	path := filepath.Join(root, ".docs", "wiki", "00_gobierno_documental.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read governance fixture: %v", err)
	}
	config := strings.Join([]string{
		"ae_canon:",
		"  mode: kernel_v3",
		"  source: <kernel_home>/canon",
		"  repo_policy: .docs/ae/repo-policy.yaml",
	}, "\n")
	updated := strings.Replace(string(content), "hierarchy:", config+"\nhierarchy:", 1)
	writeWorkspaceFile(t, root, ".docs/wiki/00_gobierno_documental.md", updated)

	status := docgraph.InspectGovernance(root, true)
	if !status.Blocked {
		t.Fatalf("expected unknown AE canon mode to block governance, got %#v", status)
	}
	if status.AECanon.Status != "mismatch" || status.AECanon.Reason != "ae_canon_mode_invalid" {
		t.Fatalf("expected invalid AE canon mode status, got %#v", status.AECanon)
	}
}

func TestInspectGovernanceReportsKernelV2SourceDefects(t *testing.T) {
	t.Run("missing canon module", func(t *testing.T) {
		ensureWritableTestHome(t)
		root := t.TempDir()
		kernelHome := t.TempDir()
		t.Setenv("AE_KERNEL_HOME", kernelHome)
		writeSpecBackendGovernanceFixture(t, root)
		addKernelV2AECanonToGovernanceFixture(t, root)
		writeKernelV2RepoPolicy(t, root)
		writeWorkspaceFile(t, kernelHome, "canon/AE-KERNEL-V2.md", "# AE Kernel v2\n")

		status := docgraph.InspectGovernance(root, true)
		if !status.Blocked || status.AECanon.Status != "missing" {
			t.Fatalf("expected incomplete canon to block governance, got %#v", status)
		}
		if len(status.AECanon.MissingModules) != 4 {
			t.Fatalf("expected four missing canon modules, got %#v", status.AECanon.MissingModules)
		}
	})

	t.Run("canon module is not a regular file", func(t *testing.T) {
		ensureWritableTestHome(t)
		root := t.TempDir()
		kernelHome := t.TempDir()
		t.Setenv("AE_KERNEL_HOME", kernelHome)
		writeSpecBackendGovernanceFixture(t, root)
		addKernelV2AECanonToGovernanceFixture(t, root)
		writeKernelV2CanonModules(t, kernelHome)
		writeKernelV2RepoPolicy(t, root)
		modulePath := filepath.Join(kernelHome, "canon", "AE-PHASES.md")
		if err := os.Remove(modulePath); err != nil {
			t.Fatalf("remove canon fixture module: %v", err)
		}
		if err := os.Mkdir(modulePath, 0o755); err != nil {
			t.Fatalf("replace canon module with directory: %v", err)
		}

		status := docgraph.InspectGovernance(root, true)
		if !status.Blocked || status.AECanon.Status != "mismatch" || status.AECanon.Reason != "kernel_v2_source_path_invalid" {
			t.Fatalf("expected non-file canon module to block governance, got %#v", status)
		}
		if len(status.AECanon.MissingModules) != 1 || status.AECanon.MissingModules[0] != "<kernel_home>/canon/AE-PHASES.md#unsafe_path" {
			t.Fatalf("expected unsafe canon module marker, got %#v", status.AECanon.MissingModules)
		}
	})

	t.Run("repo policy symbolic link escapes workspace", func(t *testing.T) {
		ensureWritableTestHome(t)
		root := t.TempDir()
		kernelHome := t.TempDir()
		t.Setenv("AE_KERNEL_HOME", kernelHome)
		writeSpecBackendGovernanceFixture(t, root)
		addKernelV2AECanonToGovernanceFixture(t, root)
		writeKernelV2CanonModules(t, kernelHome)
		externalRoot := t.TempDir()
		writeKernelV2RepoPolicy(t, externalRoot)
		policyDir := filepath.Join(root, ".docs", "ae")
		if err := os.MkdirAll(policyDir, 0o755); err != nil {
			t.Fatalf("create repo policy directory: %v", err)
		}
		if err := os.Symlink(filepath.Join(externalRoot, ".docs", "ae", "repo-policy.yaml"), filepath.Join(policyDir, "repo-policy.yaml")); err != nil {
			t.Skipf("symbolic links unavailable in this test environment: %v", err)
		}

		status := docgraph.InspectGovernance(root, true)
		if !status.Blocked || status.AECanon.Status != "mismatch" || status.AECanon.Reason != "kernel_v2_source_path_invalid" {
			t.Fatalf("expected external policy symlink to block governance, got %#v", status)
		}
	})

	t.Run("invalid repo policy yaml", func(t *testing.T) {
		ensureWritableTestHome(t)
		root := t.TempDir()
		kernelHome := t.TempDir()
		t.Setenv("AE_KERNEL_HOME", kernelHome)
		writeSpecBackendGovernanceFixture(t, root)
		addKernelV2AECanonToGovernanceFixture(t, root)
		writeKernelV2CanonModules(t, kernelHome)
		writeWorkspaceFile(t, root, ".docs/ae/repo-policy.yaml", "repo: [\n")

		status := docgraph.InspectGovernance(root, true)
		if !status.Blocked || status.AECanon.Status != "missing" {
			t.Fatalf("expected invalid repo policy to block governance, got %#v", status)
		}
		if len(status.AECanon.MissingModules) != 1 || status.AECanon.MissingModules[0] != ".docs/ae/repo-policy.yaml#invalid_yaml" {
			t.Fatalf("expected invalid yaml marker, got %#v", status.AECanon.MissingModules)
		}
	})

	t.Run("missing repo policy sections", func(t *testing.T) {
		ensureWritableTestHome(t)
		root := t.TempDir()
		kernelHome := t.TempDir()
		t.Setenv("AE_KERNEL_HOME", kernelHome)
		writeSpecBackendGovernanceFixture(t, root)
		addKernelV2AECanonToGovernanceFixture(t, root)
		writeKernelV2CanonModules(t, kernelHome)
		writeWorkspaceFile(t, root, ".docs/ae/repo-policy.yaml", "repo:\n  name: test-repo\n")

		status := docgraph.InspectGovernance(root, true)
		if !status.Blocked || status.AECanon.Status != "missing" {
			t.Fatalf("expected incomplete repo policy to block governance, got %#v", status)
		}
		missing := strings.Join(status.AECanon.MissingModules, "\n")
		for _, slot := range []string{
			"#repo.description",
			"#tracker.provider",
			"#wrappers[]",
			"#qa.canon_paths[]",
			"#last_updated",
		} {
			if !strings.Contains(missing, slot) {
				t.Fatalf("expected missing policy slot %q, got %#v", slot, status.AECanon.MissingModules)
			}
		}
	})

	t.Run("empty repo policy sections", func(t *testing.T) {
		ensureWritableTestHome(t)
		root := t.TempDir()
		kernelHome := t.TempDir()
		t.Setenv("AE_KERNEL_HOME", kernelHome)
		writeSpecBackendGovernanceFixture(t, root)
		addKernelV2AECanonToGovernanceFixture(t, root)
		writeKernelV2CanonModules(t, kernelHome)
		writeWorkspaceFile(t, root, ".docs/ae/repo-policy.yaml", strings.Join([]string{
			"repo: {}",
			"language: {}",
			"tracker: {}",
			"secrets: {}",
			"wrappers: []",
			"wiki: {}",
			"qa: {}",
			"subagents: {}",
			"last_updated: \"\"",
		}, "\n")+"\n")

		status := docgraph.InspectGovernance(root, true)
		if !status.Blocked || status.AECanon.Status != "missing" {
			t.Fatalf("expected empty repo policy sections to block governance, got %#v", status)
		}
		missing := strings.Join(status.AECanon.MissingModules, "\n")
		for _, slot := range []string{
			"#repo.name",
			"#tracker.provider",
			"#wrappers[]",
			"#qa.canon_paths[]",
			"#last_updated",
		} {
			if !strings.Contains(missing, slot) {
				t.Fatalf("expected empty policy slot %q to be rejected, got %#v", slot, status.AECanon.MissingModules)
			}
		}
	})

	for _, tc := range []struct {
		name        string
		oldValue    string
		newValue    string
		missingSlot string
	}{
		{
			name:        "placeholder map does not satisfy repo fields",
			oldValue:    "  name: test-repo\n  description: Test repository",
			newValue:    "  placeholder: null",
			missingSlot: "#repo.name",
		},
		{
			name:        "null wrapper is rejected",
			oldValue:    "  - name: test-wrapper\n    script: scripts/test-wrapper.ps1",
			newValue:    "  - null",
			missingSlot: "#wrappers[]",
		},
		{
			name:        "incorrect scalar type is rejected",
			oldValue:    "  docs_lang: Spanish",
			newValue:    "  docs_lang:\n    - Spanish",
			missingSlot: "#language.docs_lang",
		},
		{
			name:        "empty structure rules are rejected",
			oldValue:    "  structure_rules:\n    - Keep code under internal",
			newValue:    "  structure_rules: []",
			missingSlot: "#repo.structure_rules[]",
		},
		{
			name:        "provider key env is required",
			oldValue:    "    key_env: TEST_LINEAR_API_KEY",
			newValue:    "    key_env: \"\"",
			missingSlot: "#tracker.linear.key_env",
		},
		{
			name:        "provider key env must be an env name",
			oldValue:    "    key_env: TEST_LINEAR_API_KEY",
			newValue:    "    key_env: key-env-with-dash",
			missingSlot: "#tracker.linear.key_env",
		},
		{
			name:        "invalid update date is rejected",
			oldValue:    "last_updated: \"2026-07-13\"",
			newValue:    "last_updated: \"not-a-date\"",
			missingSlot: "#last_updated",
		},
		{
			name:        "first tracker project requires key",
			oldValue:    "      - key: TEST",
			newValue:    "      - name: Test",
			missingSlot: "#tracker.linear.projects[].key",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ensureWritableTestHome(t)
			root := t.TempDir()
			kernelHome := t.TempDir()
			t.Setenv("AE_KERNEL_HOME", kernelHome)
			writeSpecBackendGovernanceFixture(t, root)
			addKernelV2AECanonToGovernanceFixture(t, root)
			writeKernelV2CanonModules(t, kernelHome)
			writeKernelV2RepoPolicy(t, root)

			policyPath := filepath.Join(root, ".docs", "ae", "repo-policy.yaml")
			content, err := os.ReadFile(policyPath)
			if err != nil {
				t.Fatalf("read repo policy fixture: %v", err)
			}
			updated := strings.Replace(string(content), tc.oldValue, tc.newValue, 1)
			if updated == string(content) {
				t.Fatalf("repo policy fixture did not contain %q", tc.oldValue)
			}
			writeWorkspaceFile(t, root, ".docs/ae/repo-policy.yaml", updated)

			status := docgraph.InspectGovernance(root, true)
			if !status.Blocked || status.AECanon.Status != "missing" {
				t.Fatalf("expected semantically invalid repo policy to block governance, got %#v", status)
			}
			if !strings.Contains(strings.Join(status.AECanon.MissingModules, "\n"), tc.missingSlot) {
				t.Fatalf("expected missing policy slot %q, got %#v", tc.missingSlot, status.AECanon.MissingModules)
			}
		})
	}
}

func TestInspectGovernanceRejectsInvalidKernelV2AuthorityBinding(t *testing.T) {
	for _, tc := range []struct {
		name        string
		oldValue    string
		newValue    string
		missingSlot string
	}{
		{name: "policy schema", oldValue: "schema: ae-repo-policy/v1", newValue: "schema: ae-repo-policy/v2", missingSlot: "#schema"},
		{name: "policy version", oldValue: "version: 1", newValue: "version: 2", missingSlot: "#version"},
		{name: "authority schema", oldValue: "  schema: ae-authority/v1", newValue: "  schema: ae-authority/v2", missingSlot: "#authority.schema"},
		{name: "authority version", oldValue: "  version: 1", newValue: "  version: 2", missingSlot: "#authority.version"},
		{name: "authority model", oldValue: "    - Kernel", newValue: "    - Organization", missingSlot: "#authority.model"},
		{name: "authority entity id", oldValue: "    id: test-repo", newValue: "    id: \"\"", missingSlot: "#authority.repository.id"},
		{name: "forbidden organization layer", oldValue: "authority:\n  schema:", newValue: "authority:\n  org:\n    id: forbidden\n  schema:", missingSlot: "#authority.org#forbidden"},
		{name: "unknown authority key", oldValue: "authority:\n  schema:", newValue: "authority:\n  extra: forbidden\n  schema:", missingSlot: "#authority.extra#forbidden"},
		{name: "allowlist version", oldValue: "    version: v1", newValue: "    version: v2", missingSlot: "#authority.allowlist.version"},
		{name: "allowlist must include policy", oldValue: "      - .docs/ae/repo-policy.yaml", newValue: "      - .docs/ae/other.yaml", missingSlot: "#authority.allowlist.paths#policy_missing"},
		{name: "allowlist duplicate", oldValue: "      - AGENTS.md", newValue: "      - AGENTS.md\n      - AGENTS.md", missingSlot: "#authority.allowlist.paths#duplicate"},
		{name: "allowlist unsafe path", oldValue: "      - AGENTS.md", newValue: "      - ../AGENTS.md", missingSlot: "#authority.allowlist.paths#unsafe_path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ensureWritableTestHome(t)
			root := t.TempDir()
			kernelHome := t.TempDir()
			t.Setenv("AE_KERNEL_HOME", kernelHome)
			writeSpecBackendGovernanceFixture(t, root)
			addKernelV2AECanonToGovernanceFixture(t, root)
			writeKernelV2CanonModules(t, kernelHome)
			writeKernelV2RepoPolicy(t, root)

			policyPath := filepath.Join(root, ".docs", "ae", "repo-policy.yaml")
			content, err := os.ReadFile(policyPath)
			if err != nil {
				t.Fatalf("read repo policy fixture: %v", err)
			}
			updated := strings.Replace(string(content), tc.oldValue, tc.newValue, 1)
			if updated == string(content) {
				t.Fatalf("repo policy fixture did not contain %q", tc.oldValue)
			}
			writeWorkspaceFile(t, root, ".docs/ae/repo-policy.yaml", updated)

			status := docgraph.InspectGovernance(root, true)
			if !status.Blocked || status.AECanon.Status != "missing" {
				t.Fatalf("expected invalid authority binding to block governance, got %#v", status)
			}
			if !strings.Contains(strings.Join(status.AECanon.MissingModules, "\n"), tc.missingSlot) {
				t.Fatalf("expected policy defect %q, got %#v", tc.missingSlot, status.AECanon.MissingModules)
			}
		})
	}
}

func TestInspectGovernanceDoesNotAllowReadmeToOverrideKernelV2Authority(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	kernelHome := t.TempDir()
	t.Setenv("AE_KERNEL_HOME", kernelHome)
	writeSpecBackendGovernanceFixture(t, root)
	addKernelV2AECanonToGovernanceFixture(t, root)
	writeKernelV2CanonModules(t, kernelHome)
	writeKernelV2RepoPolicy(t, root)
	writeWorkspaceFile(t, root, ".docs/ae/README.md", "```yaml\nae_canon:\n  mode: kernel_v3\n  source: local\n```\n")

	status := docgraph.InspectGovernance(root, true)
	if status.Blocked || status.AECanon.Status != "valid" {
		t.Fatalf("README content must not override repository policy authority, got %#v", status)
	}
}

func TestInspectGovernanceDoesNotDeriveKernelV2AuthorityFromReadme(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	kernelHome := t.TempDir()
	t.Setenv("AE_KERNEL_HOME", kernelHome)
	writeSpecBackendGovernanceFixture(t, root)
	addAEDeclarationToGovernanceFixture(t, root, ".docs/ae")
	writeKernelV2CanonModules(t, kernelHome)
	writeKernelV2RepoPolicy(t, root)
	writeWorkspaceFile(t, root, ".docs/ae/README.md", "```yaml\nae_canon:\n  mode: kernel_v2\n  source: <kernel_home>/canon\n  repo_policy: .docs/ae/repo-policy.yaml\n```\n")

	status := docgraph.InspectGovernance(root, true)
	if !status.Blocked || status.AECanon.Reason != "ae_canon_legacy_mode_rejected" {
		t.Fatalf("README must not create Kernel v2 authority, got %#v", status)
	}
}

func TestInspectGovernanceAcceptsKernelV2TrackerProviders(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider string
		block    string
	}{
		{name: "linear lowercase", provider: "linear", block: "linear"},
		{name: "linear canonical", provider: "Linear", block: "linear"},
		{name: "linear uppercase", provider: "LINEAR", block: "linear"},
		{name: "plane lowercase", provider: "plane", block: "plane"},
		{name: "plane canonical", provider: "Plane", block: "plane"},
		{name: "plane uppercase", provider: "PLANE", block: "plane"},
		{name: "azure underscore", provider: "azure_boards", block: "azure_boards"},
		{name: "azure hyphen", provider: "azure-boards", block: "azure_boards"},
		{name: "azure compact", provider: "AzureBoards", block: "azure_boards"},
		{name: "azure canonical", provider: "Azure Boards", block: "azure_boards"},
		{name: "jira lowercase", provider: "jira", block: "jira"},
		{name: "jira canonical", provider: "Jira", block: "jira"},
		{name: "jira uppercase", provider: "JIRA", block: "jira"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ensureWritableTestHome(t)
			root := t.TempDir()
			kernelHome := t.TempDir()
			t.Setenv("AE_KERNEL_HOME", kernelHome)
			writeSpecBackendGovernanceFixture(t, root)
			addKernelV2AECanonToGovernanceFixture(t, root)
			writeKernelV2CanonModules(t, kernelHome)
			writeKernelV2RepoPolicy(t, root)

			policyPath := filepath.Join(root, ".docs", "ae", "repo-policy.yaml")
			content, err := os.ReadFile(policyPath)
			if err != nil {
				t.Fatalf("read repo policy fixture: %v", err)
			}
			updated := strings.Replace(string(content), "  provider: Linear", "  provider: "+tc.provider, 1)
			updated = strings.Replace(updated, "  linear:", "  "+tc.block+":", 1)
			writeWorkspaceFile(t, root, ".docs/ae/repo-policy.yaml", updated)

			status := docgraph.InspectGovernance(root, true)
			if status.Blocked || status.AECanon.Status != "valid" {
				t.Fatalf("expected %s tracker policy to pass, got %#v", tc.provider, status)
			}
		})
	}
}

func TestInspectGovernanceAcceptsKernelV2TrackerNone(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	kernelHome := t.TempDir()
	t.Setenv("AE_KERNEL_HOME", kernelHome)
	writeSpecBackendGovernanceFixture(t, root)
	addKernelV2AECanonToGovernanceFixture(t, root)
	writeKernelV2CanonModules(t, kernelHome)
	writeKernelV2RepoPolicy(t, root)

	policyPath := filepath.Join(root, ".docs", "ae", "repo-policy.yaml")
	content, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("read repo policy fixture: %v", err)
	}
	updated := strings.Replace(string(content), strings.Join([]string{
		"  provider: Linear",
		"  linear:",
		"    base_url: https://api.linear.app/graphql",
		"    workspace: test",
		"    key_env: TEST_LINEAR_API_KEY",
		"    projects:",
		"      - key: TEST",
	}, "\n"), strings.Join([]string{
		"  provider: None",
		"  none:",
		"    mode: local-only",
	}, "\n"), 1)
	if updated == string(content) {
		t.Fatal("tracker fixture did not contain the expected Linear block")
	}
	writeWorkspaceFile(t, root, ".docs/ae/repo-policy.yaml", updated)

	status := docgraph.InspectGovernance(root, true)
	if status.Blocked || status.AECanon.Status != "valid" {
		t.Fatalf("expected tracker None policy to pass, got %#v", status)
	}
}

func TestInspectGovernanceRejectsKernelV2TrackerNoneDefects(t *testing.T) {
	for _, tc := range []struct {
		name        string
		tracker     string
		missingSlot string
	}{
		{
			name:        "missing local-only mode",
			tracker:     "  provider: None\n  none: {}",
			missingSlot: "#tracker.none.mode",
		},
		{
			name:        "external field is forbidden",
			tracker:     "  provider: None\n  none:\n    mode: local-only\n  linear:\n    key_env: SHOULD_NOT_EXIST",
			missingSlot: "#tracker.linear#forbidden",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ensureWritableTestHome(t)
			root := t.TempDir()
			kernelHome := t.TempDir()
			t.Setenv("AE_KERNEL_HOME", kernelHome)
			writeSpecBackendGovernanceFixture(t, root)
			addKernelV2AECanonToGovernanceFixture(t, root)
			writeKernelV2CanonModules(t, kernelHome)
			writeKernelV2RepoPolicy(t, root)

			policyPath := filepath.Join(root, ".docs", "ae", "repo-policy.yaml")
			content, err := os.ReadFile(policyPath)
			if err != nil {
				t.Fatalf("read repo policy fixture: %v", err)
			}
			updated := strings.Replace(string(content), strings.Join([]string{
				"  provider: Linear",
				"  linear:",
				"    base_url: https://api.linear.app/graphql",
				"    workspace: test",
				"    key_env: TEST_LINEAR_API_KEY",
				"    projects:",
				"      - key: TEST",
			}, "\n"), tc.tracker, 1)
			if updated == string(content) {
				t.Fatal("tracker fixture did not contain the expected Linear block")
			}
			writeWorkspaceFile(t, root, ".docs/ae/repo-policy.yaml", updated)

			status := docgraph.InspectGovernance(root, true)
			if !status.Blocked || status.AECanon.Status != "missing" {
				t.Fatalf("expected invalid tracker None policy to block, got %#v", status)
			}
			if !strings.Contains(strings.Join(status.AECanon.MissingModules, "\n"), tc.missingSlot) {
				t.Fatalf("expected tracker defect %q, got %#v", tc.missingSlot, status.AECanon.MissingModules)
			}
		})
	}
}

func TestInspectGovernanceAcceptsRealBuhoNoneRepoPolicy(t *testing.T) {
	const realPolicyPath = `C:\repos\buho\assets\.docs\ae\repo-policy.yaml`
	content, err := os.ReadFile(realPolicyPath)
	if os.IsNotExist(err) {
		t.Skipf("real Buho policy is not available at %s", realPolicyPath)
	}
	if err != nil {
		t.Fatalf("read real Buho policy: %v", err)
	}
	ensureWritableTestHome(t)
	root := t.TempDir()
	kernelHome := t.TempDir()
	t.Setenv("AE_KERNEL_HOME", kernelHome)
	writeSpecBackendGovernanceFixture(t, root)
	addKernelV2AECanonToGovernanceFixture(t, root)
	writeKernelV2CanonModules(t, kernelHome)
	writeWorkspaceFile(t, root, ".docs/ae/repo-policy.yaml", string(content))

	status := docgraph.InspectGovernance(root, true)
	if status.Blocked || status.AECanon.Status != "valid" {
		t.Fatalf("expected real Buho None policy to pass without tracker secrets, got %#v", status)
	}
}

func TestInspectGovernanceRejectsKernelV2TrackerProviderOutsideRendererAliases(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	kernelHome := t.TempDir()
	t.Setenv("AE_KERNEL_HOME", kernelHome)
	writeSpecBackendGovernanceFixture(t, root)
	addKernelV2AECanonToGovernanceFixture(t, root)
	writeKernelV2CanonModules(t, kernelHome)
	writeKernelV2RepoPolicy(t, root)

	policyPath := filepath.Join(root, ".docs", "ae", "repo-policy.yaml")
	content, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("read repo policy fixture: %v", err)
	}
	updated := strings.Replace(string(content), "  provider: Linear", "  provider: \" Linear \"", 1)
	writeWorkspaceFile(t, root, ".docs/ae/repo-policy.yaml", updated)

	status := docgraph.InspectGovernance(root, true)
	if !status.Blocked || status.AECanon.Status != "missing" {
		t.Fatalf("expected provider outside renderer aliases to block, got %#v", status)
	}
	if !strings.Contains(strings.Join(status.AECanon.MissingModules, "\n"), "#tracker.provider") {
		t.Fatalf("expected invalid tracker provider marker, got %#v", status.AECanon.MissingModules)
	}
}

func TestInspectGovernanceRejectsKernelV2TrackerNeutralAliases(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()
	kernelHome := t.TempDir()
	t.Setenv("AE_KERNEL_HOME", kernelHome)
	writeSpecBackendGovernanceFixture(t, root)
	addKernelV2AECanonToGovernanceFixture(t, root)
	writeKernelV2CanonModules(t, kernelHome)
	writeKernelV2RepoPolicy(t, root)

	policyPath := filepath.Join(root, ".docs", "ae", "repo-policy.yaml")
	content, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("read repo policy fixture: %v", err)
	}
	updated := strings.Replace(string(content), "  provider: Linear", "  provider: Linear\n  key_env: LEGACY_LINEAR_API_KEY", 1)
	writeWorkspaceFile(t, root, ".docs/ae/repo-policy.yaml", updated)

	status := docgraph.InspectGovernance(root, true)
	if !status.Blocked || status.AECanon.Status != "missing" {
		t.Fatalf("expected neutral tracker alias to block, got %#v", status)
	}
	if !strings.Contains(strings.Join(status.AECanon.MissingModules, "\n"), "#tracker.key_env#forbidden") {
		t.Fatalf("expected forbidden neutral alias marker, got %#v", status.AECanon.MissingModules)
	}
}

func TestInspectGovernanceResolvesKernelHomeFromConfigWhenEnvIsBlank(t *testing.T) {
	root := t.TempDir()
	kernelHome := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("AE_KERNEL_HOME", "   ")
	writeWorkspaceFile(t, home, ".ae/config.yaml", "kernel_home: \""+filepath.ToSlash(kernelHome)+"\"\n")
	writeSpecBackendGovernanceFixture(t, root)
	addKernelV2AECanonToGovernanceFixture(t, root)
	writeKernelV2CanonModules(t, kernelHome)
	writeKernelV2RepoPolicy(t, root)

	status := docgraph.InspectGovernance(root, true)
	if status.Blocked || status.AECanon.Status != "valid" || status.AECanon.Source != "kernel_v2" {
		t.Fatalf("expected kernel home config fallback to pass, got %#v", status)
	}
}

func TestNavGovernanceUsesReadModelSourceDocForKnowledgeWiki(t *testing.T) {
	alias := "gov-knowledge-wiki-" + filepath.Base(t.TempDir())
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/00-gobierno.md", "# 00 - Gobierno\n\nKraal-style knowledge wiki governance.\n")
	writeWorkspaceFile(t, root, ".docs/wiki/01-alcance.md", "# 01 - Alcance\n")
	writeAECanonModules(t, root, ".docs/wiki/ae")
	writeWorkspaceFile(t, root, ".docs/wiki/_mi-lsp/read-model.toml", `version = 1

[[family]]
  name = "kraal-canon"
  intent_keywords = ["gobierno", "ae"]
  paths = [".docs/wiki/*.md", ".docs/wiki/ae/"]

[generic_docs]
  paths = ["README.md", ".docs/wiki/"]

[reading_pack]
  max_docs = 8
  functional_stage_order = ["kraal-canon"]
  technical_stage_order = ["kraal-canon"]
  ux_stage_order = ["kraal-canon"]

[governance]
  source_doc = ".docs/wiki/00-gobierno.md"
  source_format = "markdown"
  profile = "knowledge-wiki"
  effective_base = "knowledge-wiki"
  context_chain = [".docs/wiki/00-gobierno.md", ".docs/wiki/01-alcance.md"]
  closure_chain = ["tools/validate_kraal.py"]
  audit_chain = [".docs/auditoria/"]
  blocking_rules = ["Do not treat .docs/raw/** as canonical truth."]
`)

	app := New(root, nil)
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "main"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.governance",
		Context:   model.QueryOptions{Workspace: alias},
	})
	if err != nil {
		t.Fatalf("nav.governance: %v", err)
	}
	status := env.Items.([]model.GovernanceStatus)[0]
	if status.Blocked {
		t.Fatalf("expected knowledge-wiki governance to pass, got %#v", status)
	}
	if status.HumanDoc != ".docs/wiki/00-gobierno.md" {
		t.Fatalf("human_doc = %q, want .docs/wiki/00-gobierno.md", status.HumanDoc)
	}
	if status.Profile != "knowledge-wiki" || status.Sync != "in_sync" {
		t.Fatalf("expected knowledge-wiki in_sync, got profile=%q sync=%q", status.Profile, status.Sync)
	}
	if status.AECanon.Status == "valid" {
		t.Fatalf("knowledge-wiki without kernel_v2 must not promote repo-local AE as authority, got %#v", status.AECanon)
	}
}

func TestNavGovernanceRejectsExplicitAECanonReadmeRedirect(t *testing.T) {
	alias := "gov-ae-redirect-" + filepath.Base(t.TempDir())
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# 07. Baseline tecnica\n")
	writeAECanonModules(t, root, ".docs/ae")
	writeSpecBackendGovernanceFixture(t, root)
	addAEDeclarationToGovernanceFixture(t, root, ".docs/wiki/ae")
	writeWorkspaceFile(t, root, ".docs/wiki/ae/README.md", "# AE redirect\n\nCanon moved to `.docs/ae/README.md`.\n")
	if status := docgraph.InspectGovernance(root, true); !status.Blocked || status.AECanon.Reason != "ae_canon_legacy_mode_rejected" {
		t.Fatalf("expected redirected legacy AE canon to be rejected, got %#v", status)
	}

	app := New(root, nil)
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "main"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.governance",
		Context:   model.QueryOptions{Workspace: alias},
	})
	if err != nil {
		t.Fatalf("nav.governance: %v", err)
	}
	status := env.Items.([]model.GovernanceStatus)[0]
	if !status.Blocked || status.AECanon.Reason != "ae_canon_legacy_mode_rejected" {
		t.Fatalf("expected legacy redirect rejection, got %#v", status)
	}
}

func TestNavGovernanceDoesNotPromoteUndeclaredDocsAE(t *testing.T) {
	alias := "gov-ae-undeclared-" + filepath.Base(t.TempDir())
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# 07. Baseline tecnica\n")
	writeAECanonModules(t, root, ".docs/ae")
	writeSpecBackendGovernanceFixture(t, root)

	app := New(root, nil)
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "main"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.governance",
		Context:   model.QueryOptions{Workspace: alias},
	})
	if err != nil {
		t.Fatalf("nav.governance: %v", err)
	}
	status := env.Items.([]model.GovernanceStatus)[0]
	if status.AECanon.Status == "valid" || len(status.AECanon.Roots) != 0 {
		t.Fatalf("undeclared .docs/ae should not become authority, got %#v", status.AECanon)
	}
	if status.Blocked {
		t.Fatalf("undeclared .docs/ae should not block a non-AE governance profile, got %#v", status)
	}
}

func TestNavGovernanceAutoSyncsProjectionWhenMissing(t *testing.T) {
	alias := "gov-sync-" + filepath.Base(t.TempDir())
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# 07. Baseline tecnica\n")
	writeSpecBackendGovernanceFixture(t, root)
	if err := os.Remove(filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml")); err != nil {
		t.Fatalf("remove read-model: %v", err)
	}

	app := New(root, nil)
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "main"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.governance",
		Context:   model.QueryOptions{Workspace: alias},
	})
	if err != nil {
		t.Fatalf("nav.governance: %v", err)
	}
	items := env.Items.([]model.GovernanceStatus)
	if len(items) != 1 {
		t.Fatalf("expected one governance status, got %#v", env.Items)
	}
	if items[0].Sync != "auto_synced" && items[0].Sync != "in_sync" {
		t.Fatalf("expected auto sync or in_sync, got %#v", items[0])
	}
	if _, err := os.Stat(filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml")); err != nil {
		t.Fatalf("expected projected read-model to exist, got %v", err)
	}
}

func TestNavAskBlocksWhenGovernanceDocumentIsMissing(t *testing.T) {
	alias := "gov-block-" + filepath.Base(t.TempDir())
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# 07. Baseline tecnica\n")
	writeWorkspaceFile(t, root, ".docs/wiki/_mi-lsp/read-model.toml", "version = 1\n")

	app := New(root, nil)
	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	}); err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.ask",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"question": "how does daemon routing work?"},
	})
	if err != nil {
		t.Fatalf("nav.ask: %v", err)
	}
	if env.Backend != "governance" {
		t.Fatalf("backend = %q, want governance", env.Backend)
	}
	items := env.Items.([]model.GovernanceStatus)
	if len(items) != 1 || !items[0].Blocked {
		t.Fatalf("expected blocked governance status, got %#v", env.Items)
	}
}

func TestWorkspaceStatusIncludesGovernanceFields(t *testing.T) {
	alias := "gov-status-" + filepath.Base(t.TempDir())
	root := createIndexedWorkspaceFixture(t, alias)
	app := New(root, nil)

	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	}); err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.status",
		Context:   model.QueryOptions{Workspace: alias},
	})
	if err != nil {
		t.Fatalf("workspace.status: %v", err)
	}
	items := env.Items.([]any)
	item := items[0].(map[string]any)
	if item["governance_profile"] != "spec_backend" {
		t.Fatalf("governance_profile = %#v, want spec_backend", item["governance_profile"])
	}
	if item["governance_blocked"] != false {
		t.Fatalf("governance_blocked = %#v, want false", item["governance_blocked"])
	}
	if item["governance_sync"] != "in_sync" {
		t.Fatalf("governance_sync = %#v, want in_sync", item["governance_sync"])
	}
	aeCanon, ok := item["ae_canon"].(model.AECanonStatus)
	if !ok || aeCanon.Status == "" {
		t.Fatalf("expected ae_canon in workspace.status, got %#v", item["ae_canon"])
	}
}

func TestWorkspaceStatusBlocksKapsitoStyleMissingGovernanceEmptyDocs(t *testing.T) {
	alias := "gov-kapsito-regression-" + filepath.Base(t.TempDir())
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	app := New(root, nil)
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "main"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.status",
		Context:   model.QueryOptions{Workspace: alias},
	})
	if err != nil {
		t.Fatalf("workspace.status: %v", err)
	}
	items := env.Items.([]any)
	item := items[0].(map[string]any)
	if item["governance_blocked"] != true {
		t.Fatalf("governance_blocked = %#v, want true", item["governance_blocked"])
	}
	if item["docs_ready"] != false || item["doc_count"] != 0 {
		t.Fatalf("expected empty docs to be not ready, got docs_ready=%#v doc_count=%#v", item["docs_ready"], item["doc_count"])
	}
	aeCanon, ok := item["ae_canon"].(model.AECanonStatus)
	if !ok {
		t.Fatalf("expected ae_canon status, got %#v", item["ae_canon"])
	}
	// AECanon.Blocking should only be true if the workspace declares AE in its governance.
	// When governance is missing, Blocking should be false (workspace must create governance first).
	if aeCanon.Status != "missing" || aeCanon.Blocking {
		t.Fatalf("expected missing non-blocking ae_canon when governance is missing, got %#v", aeCanon)
	}
}

func TestWorkspaceStatusCanSkipReadModelAutoSync(t *testing.T) {
	alias := "gov-status-readonly-" + filepath.Base(t.TempDir())
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# 07. Baseline tecnica\n")
	writeSpecBackendGovernanceFixture(t, root)
	projectionPath := filepath.Join(root, ".docs", "wiki", "_mi-lsp", "read-model.toml")
	if err := os.Remove(projectionPath); err != nil {
		t.Fatalf("remove read-model: %v", err)
	}

	app := New(root, nil)
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "main"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: "."}},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.status",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"auto_sync": false},
	})
	if err != nil {
		t.Fatalf("workspace.status: %v", err)
	}
	items := env.Items.([]any)
	item := items[0].(map[string]any)
	if item["governance_sync"] != "stale" {
		t.Fatalf("governance_sync = %#v, want stale", item["governance_sync"])
	}
	if item["governance_blocked"] != true {
		t.Fatalf("governance_blocked = %#v, want true", item["governance_blocked"])
	}
	if _, err := os.Stat(projectionPath); !os.IsNotExist(err) {
		t.Fatalf("read-model should not be auto-synced, stat err=%v", err)
	}
}

func TestNavPackBlockedWhenGovernanceIsInvalid(t *testing.T) {
	alias := "gov-block-pack-" + filepath.Base(t.TempDir())
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# 07. Baseline tecnica\n")
	writeWorkspaceFile(t, root, ".docs/wiki/_mi-lsp/read-model.toml", "version = 1\n")

	app := New(root, nil)
	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	}); err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.pack",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"task": "understand how login works"},
	})
	if err != nil {
		t.Fatalf("nav.pack: %v", err)
	}
	if env.Backend != "governance" {
		t.Fatalf("backend = %q, want governance", env.Backend)
	}
	items := env.Items.([]model.GovernanceStatus)
	if len(items) != 1 || !items[0].Blocked {
		t.Fatalf("expected blocked governance status, got %#v", env.Items)
	}
}

func TestNavRouteBlockedWhenGovernanceIsInvalid(t *testing.T) {
	alias := "gov-block-route-" + filepath.Base(t.TempDir())
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# 07. Baseline tecnica\n")
	writeWorkspaceFile(t, root, ".docs/wiki/_mi-lsp/read-model.toml", "version = 1\n")

	app := New(root, nil)
	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	}); err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.route",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"task": "understand how login works"},
	})
	if err != nil {
		t.Fatalf("nav.route: %v", err)
	}
	if env.Backend != "governance" {
		t.Fatalf("backend = %q, want governance", env.Backend)
	}
	items := env.Items.([]model.GovernanceStatus)
	if len(items) != 1 || !items[0].Blocked {
		t.Fatalf("expected blocked governance status, got %#v", env.Items)
	}
}
