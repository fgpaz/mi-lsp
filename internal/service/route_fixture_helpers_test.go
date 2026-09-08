package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// routeFixtureProfile returns a minimal governance-declared read model that
// declares the canon/ and scripts/ path families used by the route fixtures.
// The canon hierarchy entry uses layer "AE" so Tier1's explicit-ID search
// resolves canon/** the same way the ae-kernel read model does.
func routeFixtureProfile() model.DocsReadProfile {
	return model.DocsReadProfile{
		Families: []model.DocsReadFamily{{
			Name:  "technical",
			Paths: []string{"canon/**", "scripts/**"},
		}},
		Governance: model.DocsGovernanceProfile{
			SourceDoc: ".docs/wiki/00_gobierno_documental.md",
			Hierarchy: []model.GovernanceHierarchyItem{{
				ID:     "canon",
				Layer:  "AE",
				Family: "technical",
				Paths:  []string{"canon/**"},
			}},
		},
	}
}

// makeRouteFixtureRoot writes a workspace root that contains:
//   - the canonical governance anchor canon/AE-POLICY-PROJECTION.md carrying
//     its declared doc id (the established governance owner), and
//   - a mention-bearing artifact under scripts/ whose body only asserts the
//     ID (a test file, never document identity).
//
// The canonical doc is intentionally NOT indexed by the fixtures: they index
// only the artifact, mimicking the source-proven observation where the docs
// index held mention-bearing records but not the canonical owner.
func makeRouteFixtureRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeWorkspaceFile(t, root, "canon/AE-POLICY-PROJECTION.md", strings.Join([]string{
		"# AE-POLICY-PROJECTION.md — Deterministic Policy Kernel Rendering",
		"",
		"doc_id: AE-POLICY-PROJECTION-V2",
		"",
		"## Projection Contract",
		"",
		"Deterministic rendering contract for the policy kernel.",
	}, "\n"))
	writeWorkspaceFile(t, root, "scripts/kernel-contract-docs.test.mjs", strings.Join([]string{
		"// Regression fixture: asserts AE-POLICY-PROJECTION projection stays in sync.",
		"test('AE-POLICY-PROJECTION contract', () => {});",
	}, "\n"))
	if err := os.MkdirAll(filepath.Join(root, ".docs", "wiki"), 0o755); err != nil {
		t.Fatalf("mkdir .docs/wiki: %v", err)
	}
	return root
}
